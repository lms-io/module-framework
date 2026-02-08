package framework

import (
	"context"
	"log"
)

// ModuleAPI is the interface provided to the logic layer.
type ModuleAPI interface {
	ModuleID() string
	
	// Data Management
	RegisterInstance(payload InstanceConfig) error
	UpdateState(instanceID string, state map[string]any) error
	GetInstances() []InstanceConfig
	GetModuleConfig() map[string]any
	
	// Communication
	Publish(topic, eventType string, data map[string]any)
	Subscribe(topic string) <-chan Event
	
	// Lifecycle
	Context() context.Context
	
	// Logging
	Info(msg string, args ...any)
	Error(msg string, args ...any)
	Debug(msg string, args ...any)
}

// BaseModule is the standard implementation of ModuleAPI.
type BaseModule struct {
	id         string
	stateDir   string
	bus        *BusClient
	im         *InstanceManager
	ctx        context.Context
	modConfig  map[string]any
}

func NewBaseModule(ctx context.Context, id, stateDir, busSocket string, config map[string]any) *BaseModule {
	return &BaseModule{
		id:        id,
		stateDir:  stateDir,
		bus:       NewBusClient(busSocket, id),
		im:        NewInstanceManager(stateDir, id),
		ctx:       ctx,
		modConfig: config,
	}
}

func (m *BaseModule) Start() error {
	return m.bus.Start()
}

func (m *BaseModule) ModuleID() string { return m.id }

func (m *BaseModule) RegisterInstance(payload InstanceConfig) error {
	// 1. Persist to Disk
	if err := m.im.RegisterInstance(payload); err != nil {
		return err
	}

	// 2. Announce to Bus (Core Registry)
	m.bus.Publish("sys/register", "register", map[string]any{
		"id":       payload.ID,
		"bundle":   m.id,
		"config":   payload.Config,
		"controls": payload.Controls,
		"meta":     payload.Meta,
	})
	return nil
}

func (m *BaseModule) UpdateState(id string, state map[string]any) error {
	m.im.UpdateState(id, state)
	m.bus.Publish("state/update", "update", map[string]any{
		"id":    id,
		"state": state,
	})
	return nil
}

func (m *BaseModule) GetInstances() []InstanceConfig {
	inst, _ := m.im.GetInstances()
	return inst
}

func (m *BaseModule) GetModuleConfig() map[string]any { return m.modConfig }

func (m *BaseModule) Publish(topic, eventType string, data map[string]any) {
	m.bus.Publish(topic, eventType, data)
}

func (m *BaseModule) Subscribe(topic string) <-chan Event {
	return m.bus.Subscribe(topic)
}

func (m *BaseModule) Context() context.Context { return m.ctx }

func (m *BaseModule) Info(msg string, args ...any) { log.Printf("INFO  ["+m.id+"] "+msg, args...) }
func (m *BaseModule) Error(msg string, args ...any) { log.Printf("ERROR ["+m.id+"] "+msg, args...) }
func (m *BaseModule) Debug(msg string, args ...any) { log.Printf("DEBUG ["+m.id+"] "+msg, args...) }
