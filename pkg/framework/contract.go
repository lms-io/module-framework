package framework

// BundleState represents the lifecycle phase of a module.
type BundleState string

const (
	StateIdling     BundleState = "idling"     // No config provided yet
	StateValidating BundleState = "validating" // Checking credentials/connectivity
	StateReady      BundleState = "ready"      // Config proven, but Init not called yet
	StateStarting   BundleState = "starting"   // In the middle of initial discovery/sync
	StateActive     BundleState = "active"     // Fully operational, initial sync complete
	StateError      BundleState = "error"      // Config invalid or connection lost
)

// BundleStatus provides human-readable context for the current state.
type BundleStatus struct {
	State   BundleState    `json:"state"`
	Message string         `json:"message,omitempty"`
	Config  map[string]any `json:"config,omitempty"` // The active config
}

type RawEntitySpec struct {
	ID   string         `json:"id"`
	Kind string         `json:"kind"` // bundle-native kind, e.g. "esphome.cover"
	Name string         `json:"name"`
	Raw  map[string]any `json:"raw"` // raw descriptor payload
}

type EntitySpec struct {
	ID           string         `json:"id"`
	Kind         string         `json:"kind"` // abstract kind, e.g. "actuator", "sensor", "resource"
	Name         string         `json:"name"`
	Capabilities map[string]any `json:"capabilities,omitempty"`
	Links        []string       `json:"links,omitempty"` // references to raw entities
}

// InstanceConfig is the standardized container for any hardware device.
type InstanceConfig struct {
	ID          string                    `json:"id"`      // Unique hardware identifier (e.g. MAC)
	Name        string                    `json:"name"`    // Friendly human-readable name (from hardware)
	Alias       string                    `json:"alias"`   // User-defined override name
	Enabled     bool                      `json:"enabled"` // If the logic should actively connect
	Config      map[string]any            `json:"config"`  // Static connection info (IP, Port, Keys)
	RawEntities []RawEntitySpec           `json:"raw_entities,omitempty"`
	RawState    map[string]map[string]any `json:"raw_state,omitempty"`
	Entities    []EntitySpec              `json:"entities,omitempty"`
	EntityState map[string]map[string]any `json:"entity_state,omitempty"`
	Meta        map[string]any            `json:"meta"` // Informational (Model, FW version, Status)
}

// InstanceDeleter is an optional interface for bundles to react to instance deletion.
// If implemented, DeleteInstance will be called with the instance ID before removal.
type InstanceDeleter interface {
	DeleteInstance(id string)
}

// InstancePreprocessor is an optional interface for bundles to enrich/normalize
// register_instance payloads before persistence and publish.
type InstancePreprocessor interface {
	PrepareInstance(payload InstanceConfig) (InstanceConfig, error)
}

// InstanceLifecycleObserver is an optional interface for bundles to react when
// instances are registered/deleted through the framework command path.
type InstanceLifecycleObserver interface {
	OnInstanceRegistered(instance InstanceConfig)
	OnInstanceDeleted(id string)
}

// DeviceDiscoverer is an optional interface that LifecycleHandlers can implement
// to support on-demand single-device discovery via the "discover" command.
type DeviceDiscoverer interface {
	DiscoverDevice(config map[string]any)
}

type MCPTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Mutating    bool           `json:"mutating,omitempty"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
}

type MCPDescriptor struct {
	Instructions []string  `json:"instructions,omitempty"`
	Tools        []MCPTool `json:"tools,omitempty"`
}

// MCPProvider is optional. When implemented, bundles can extend framework-provided
// MCP tools with bundle-specific tools and handlers.
type MCPProvider interface {
	MCPDescribe() MCPDescriptor
	MCPInvoke(tool string, args map[string]any, api ModuleAPI) (map[string]any, error)
}

// Event represents a system-wide message on the Bus.
type Event struct {
	Topic string         `json:"topic"` // e.g. "commands/device-id", "state/device-id"
	Type  string         `json:"type"`  // e.g. "power", "refresh", "register"
	Data  map[string]any `json:"data"`  // Payload
}
