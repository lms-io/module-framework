package framework

import (
	"context"
	"log"
)

// ModuleAPI is the interface provided to the logic layer.
type ModuleAPI interface {
	ModuleID() string

	// Lifecycle & State
	SetBundleStatus(status BundleStatus)
	GetModuleConfig() map[string]any

	// Data Management
	RegisterInstance(payload InstanceConfig) error
	DeleteInstance(id string) error
	UpdateEntityState(instanceID string, state map[string]map[string]any) error
	GetInstances() []InstanceConfig

	// Communication
	Publish(topic, eventType string, data map[string]any)
	// Listen subscribes to any arbitrary topic (e.g. "commands/device-id", "state/*")
	Listen(topic string) <-chan Event
	// Subscribe listens to state updates for a device, or a specific entity when provided.
	Subscribe(deviceID string, entityID ...string) <-chan Event

	// Lifecycle
	Context() context.Context

	// Logging
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	Debug(msg string, args ...any)
}

// BaseModule is the standard implementation of ModuleAPI.
type BaseModule struct {
	id        string
	stateDir  string
	bus       *BusClient
	im        *InstanceManager
	ctx       context.Context
	modConfig map[string]any
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

func (m *BaseModule) SetBundleStatus(status BundleStatus) {
	m.bus.Publish("sys/bundle_status", "status", map[string]any{
		"bundle":  m.id,
		"state":   status.State,
		"message": status.Message,
		"config":  status.Config,
	})
}

func (m *BaseModule) RegisterInstance(payload InstanceConfig) error {
	if err := m.im.RegisterInstance(payload); err != nil {
		return err
	}
	m.bus.Publish("sys/register", "register", map[string]any{
		"id":           payload.ID,
		"name":         payload.Name,
		"alias":        payload.Alias,
		"bundle":       m.id,
		"config":       payload.Config,
		"raw_entities": payload.RawEntities,
		"raw_state":    payload.RawState,
		"entities":     payload.Entities,
		"entity_state": payload.EntityState,
		"meta":         payload.Meta,
	})
	return nil
}

func (m *BaseModule) DeleteInstance(id string) error {
	if err := m.im.DeleteInstance(id); err != nil {
		return err
	}
	m.bus.Publish("sys/unregister", "unregister", map[string]any{
		"id":     id,
		"bundle": m.id,
	})
	return nil
}

func (m *BaseModule) UpdateEntityState(id string, state map[string]map[string]any) error {
	m.im.UpdateEntityState(id, state)
	m.bus.Publish("state/"+id, "update", map[string]any{
		"id":           id,
		"entity_state": state,
	})
	for entityID, entityState := range state {
		m.bus.Publish("state/"+id+"/"+entityID, "update", map[string]any{
			"id":           id,
			"entity_id":    entityID,
			"entity_state": map[string]map[string]any{entityID: entityState},
		})
	}
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

func (m *BaseModule) Listen(topic string) <-chan Event {
	return m.bus.Subscribe(topic)
}

func (m *BaseModule) Subscribe(deviceID string, entityID ...string) <-chan Event {
	if deviceID == "" {
		return m.bus.Subscribe("")
	}
	if len(entityID) > 0 && entityID[0] != "" {
		return m.bus.Subscribe("state/" + deviceID + "/" + entityID[0])
	}
	return m.bus.Subscribe("state/" + deviceID)
}

func (m *BaseModule) Context() context.Context { return m.ctx }

func (m *BaseModule) Info(msg string, args ...any)  { log.Printf("INFO  ["+m.id+"] "+msg, args...) }
func (m *BaseModule) Warn(msg string, args ...any)  { log.Printf("WARN  ["+m.id+"] "+msg, args...) }
func (m *BaseModule) Error(msg string, args ...any) { log.Printf("ERROR ["+m.id+"] "+msg, args...) }
func (m *BaseModule) Debug(msg string, args ...any) { log.Printf("DEBUG ["+m.id+"] "+msg, args...) }
