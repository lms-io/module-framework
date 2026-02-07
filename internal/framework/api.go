package framework

import (
	"context"
	"database/sql"
)

// Event represents a message on the bus
type Event struct {
	Topic  string         `json:"topic"`
	Type   string         `json:"type"`
	Source string         `json:"source"`
	Data   map[string]any `json:"data,omitempty"`
}

type ModuleAPI interface {
	Publish(topic, eventType string, data map[string]any)
	Subscribe(topic string) <-chan Event
	
	Info(msg string, args ...any)
	Debug(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	
	DB() *sql.DB
	GetInstances() []InstanceConfig
	GetModuleConfig() map[string]any
	RegisterInstance(config map[string]any) (string, error)
	UpdateState(id string, state map[string]any) error
	ExecInstance(id string, functionName string, args ...any) (any, error)
	
	Context() context.Context
}
