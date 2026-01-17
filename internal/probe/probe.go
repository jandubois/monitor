package probe

// Status represents the outcome of a probe execution.
type Status string

const (
	StatusOK       Status = "ok"
	StatusWarning  Status = "warning"
	StatusCritical Status = "critical"
	StatusUnknown  Status = "unknown"
)

// Result is the standard output format for probes.
type Result struct {
	Status  Status         `json:"status"`
	Message string         `json:"message"`
	Metrics map[string]any `json:"metrics,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
}

// Description is the self-description format for probes.
type Description struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Version     string    `json:"version"`
	Arguments   Arguments `json:"arguments"`
}

// Arguments describes required and optional probe arguments.
type Arguments struct {
	Required map[string]ArgumentSpec `json:"required,omitempty"`
	Optional map[string]ArgumentSpec `json:"optional,omitempty"`
}

// ArgumentSpec describes a single argument.
type ArgumentSpec struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Default     any    `json:"default,omitempty"`
}
