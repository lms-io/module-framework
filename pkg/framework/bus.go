package framework

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
)

// BusClient handles low-level communication with the system Unix socket.
type BusClient struct {
	socketPath string
	id         string
	conn       net.Conn
	listeners  map[string]func(Event)
	mu         sync.Mutex
	done       chan struct{}
	seq        uint64
}

func NewBusClient(path, moduleID string) *BusClient {
	return &BusClient{
		socketPath: path,
		id:         moduleID,
		listeners:  make(map[string]func(Event)),
		done:       make(chan struct{}),
	}
}

func (b *BusClient) Start() error {
	conn, err := net.Dial("unix", b.socketPath)
	if err != nil {
		return fmt.Errorf("failed to connect to bus socket: %v", err)
	}
	b.conn = conn

	go b.readLoop()
	return nil
}

func (b *BusClient) readLoop() {
	scanner := bufio.NewScanner(b.conn)
	for scanner.Scan() {
		var ev Event
		if err := json.Unmarshal(scanner.Bytes(), &ev); err == nil {
			b.mu.Lock()
			for _, l := range b.listeners {
				go l(ev)
			}
			b.mu.Unlock()
		}
	}
}

func (b *BusClient) Publish(topic, eventType string, data map[string]any) {
	ev := Event{Topic: topic, Type: eventType, Data: data}
	payload, _ := json.Marshal(ev)
	b.mu.Lock()
	if b.conn != nil {
		fmt.Fprintf(b.conn, "%s\n", string(payload))
	}
	b.mu.Unlock()
}

func (b *BusClient) Subscribe(topic string) (<-chan Event, string) {
	ch := make(chan Event, 100)
	b.mu.Lock()
	b.seq++
	subID := fmt.Sprintf("%d", b.seq)
	b.listeners[subID] = func(ev Event) {
		if topicMatches(topic, ev.Topic) {
			select {
			case ch <- ev:
			default:
				// Buffer full, drop event
			}
		}
	}
	b.mu.Unlock()
	return ch, subID
}

func (b *BusClient) Unsubscribe(subID string) {
	b.mu.Lock()
	delete(b.listeners, subID)
	b.mu.Unlock()
}

func topicMatches(subscription, topic string) bool {
	if strings.HasSuffix(subscription, "*") {
		prefix := strings.TrimSuffix(subscription, "*")
		return strings.HasPrefix(topic, prefix)
	}
	return subscription == topic
}

func (b *BusClient) Close() {
	close(b.done)
	if b.conn != nil {
		b.conn.Close()
	}
}
