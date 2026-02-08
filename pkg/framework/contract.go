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

// ControlSpec defines a first-class interactive UI component.
type ControlSpec struct {
	Type     string   `json:"type"`                // REQUIRED: "switch", "sensor", "binary_sensor", "light", "number", "image", "stream"
	Name     string   `json:"name"`                // REQUIRED: Human-readable label
	Unit     string   `json:"unit,omitempty"`      // OPTIONAL: "°C", "%", "lux"
	Min      float64  `json:"min,omitempty"`       // OPTIONAL: For range/number
	Max      float64  `json:"max,omitempty"`       // OPTIONAL: For range/number
	Step     float64  `json:"step,omitempty"`      // OPTIONAL: For range/number
	Options  []string `json:"options,omitempty"`   // OPTIONAL: For dropdowns/selects
	ReadOnly bool     `json:"read_only,omitempty"` // OPTIONAL: If the UI should prevent input
}

// InstanceConfig is the standardized container for any hardware device.
type InstanceConfig struct {
	ID       string                 `json:"id"`       // Unique hardware identifier (e.g. MAC)
	Name     string                 `json:"name"`     // Friendly human-readable name (from hardware)
	Alias    string                 `json:"alias"`    // User-defined override name
	Enabled  bool                   `json:"enabled"`  // If the logic should actively connect
	Config   map[string]any         `json:"config"`   // Static connection info (IP, Port, Keys)
	State    map[string]any         `json:"state"`    // Live values (Key matches Control key)
	Controls map[string]ControlSpec `json:"controls"` // UI Definition (Key matches State key)
	Meta     map[string]any         `json:"meta"`     // Informational (Model, FW version, Status)
}

// Event represents a system-wide message on the Bus.
type Event struct {
	Topic string         `json:"topic"` // e.g. "commands/device-id", "state/update"
	Type  string         `json:"type"`  // e.g. "power", "refresh", "register"
	Data  map[string]any `json:"data"`  // Payload
}