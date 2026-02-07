package harness

import (
	"encoding/json"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"module/internal/framework"
)

// Harness simulates the Core Bus
type Harness struct {
	SocketPath string
	listener   net.Listener
	Events     chan framework.Event
	conns      map[net.Conn]bool
	mu         sync.Mutex
}

func NewHarness() *Harness {
	tmpDir := os.TempDir()
	socketPath := filepath.Join(tmpDir, "module_test.sock")
	os.Remove(socketPath)

	l, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Fatalf("Failed to start harness: %v", err)
	}

	h := &Harness{
		SocketPath: socketPath,
		listener:   l,
		Events:     make(chan framework.Event, 100),
		conns:      make(map[net.Conn]bool),
	}

	go h.accept()
	return h
}

func (h *Harness) accept() {
	for {
		conn, err := h.listener.Accept()
		if err != nil {
			return
		}
		h.mu.Lock()
		h.conns[conn] = true
		h.mu.Unlock()
		go h.handleConn(conn)
	}
}

func (h *Harness) handleConn(conn net.Conn) {
	defer func() {
		h.mu.Lock()
		delete(h.conns, conn)
		h.mu.Unlock()
		conn.Close()
	}()

	decoder := json.NewDecoder(conn)
	for {
		var ev framework.Event
		if err := decoder.Decode(&ev); err != nil {
			return
		}
		
		// Log to the internal event channel for the test to see
		h.Events <- ev

		// Broadcast to all other clients
		h.mu.Lock()
		for c := range h.conns {
			if c == conn { continue } // Don't echo back to sender
			json.NewEncoder(c).Encode(ev)
		}
		h.mu.Unlock()
	}
}

func (h *Harness) Close() {
	h.listener.Close()
	os.Remove(h.SocketPath)
}
