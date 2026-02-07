package framework

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
)

type BusClient struct {
	conn        net.Conn
	subscribers map[string][]chan Event
	mu          sync.RWMutex
	moduleID    string
	logLevel    int
}

const (
	LevelDebug = iota
	LevelInfo
	LevelWarn
	LevelError
)

func NewBusClient(socketPath, moduleID, logLevelStr string) (*BusClient, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to bus: %w", err)
	}

	level := LevelInfo
	switch logLevelStr {
	case "DEBUG": level = LevelDebug
	case "WARN":  level = LevelWarn
	case "ERROR": level = LevelError
	}

	bc := &BusClient{
		conn:        conn,
		subscribers: make(map[string][]chan Event),
		moduleID:    moduleID,
		logLevel:    level,
	}

	go bc.listen()
	return bc, nil
}

func (bc *BusClient) Close() error {
	return bc.conn.Close()
}

func (bc *BusClient) listen() {
	decoder := json.NewDecoder(bc.conn)
	for {
		var ev Event
		if err := decoder.Decode(&ev); err != nil {
			// Quietly exit on EOF/closed connection
			return
		}

		bc.mu.RLock()
		subs := bc.subscribers[ev.Topic]
		wildcards := bc.subscribers["*"]
		bc.mu.RUnlock()

		// Route to specific subscribers
		if len(subs) > 0 {
			for _, ch := range subs {
				select {
				case ch <- ev:
				default:
				}
			}
		}

		// Route to wildcard subscribers
		if len(wildcards) > 0 {
			for _, ch := range wildcards {
				select {
				case ch <- ev:
				default:
				}
			}
		}
	}
}

func (bc *BusClient) Publish(topic, eventType string, data map[string]any) {
	ev := Event{
		Topic:  topic,
		Type:   eventType,
		Source: bc.moduleID,
		Data:   data,
	}
	json.NewEncoder(bc.conn).Encode(ev)
}

func (bc *BusClient) Subscribe(topic string) <-chan Event {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	ch := make(chan Event, 100)
	bc.subscribers[topic] = append(bc.subscribers[topic], ch)
	
	// Notify core of subscription? (Optional implementation detail)
	return ch
}

func (bc *BusClient) Info(msg string, args ...any) {
	if bc.logLevel <= LevelInfo {
		log.Printf("INFO  [%s] "+msg, append([]any{bc.moduleID}, args...)...)
	}
}

func (bc *BusClient) Debug(msg string, args ...any) {
	if bc.logLevel <= LevelDebug {
		log.Printf("DEBUG [%s] "+msg, append([]any{bc.moduleID}, args...)...)
	}
}

func (bc *BusClient) Warn(msg string, args ...any) {
	if bc.logLevel <= LevelWarn {
		log.Printf("WARN  [%s] "+msg, append([]any{bc.moduleID}, args...)...)
	}
}

func (bc *BusClient) Error(msg string, args ...any) {
	if bc.logLevel <= LevelError {
		log.Printf("ERROR [%s] "+msg, append([]any{bc.moduleID}, args...)...)
	}
}
