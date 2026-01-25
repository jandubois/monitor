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
	Summary string         `json:"summary,omitempty"` // One-line for collapsed view; falls back to first line of Message
	Message string         `json:"message"`
	Metrics map[string]any `json:"metrics,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
	NextRun string         `json:"next_run,omitempty"` // ISO timestamp to override next scheduled run
}

// Description is the self-description format for probes.
// The system treats (Name, Version) as a unique identifier.
// Increment Version when Output schema changes to avoid conflicts between watchers.
type Description struct {
	Name            string       `json:"name"`
	Description     string       `json:"description"`
	Version         string       `json:"version"` // Increment when Output schema changes
	Arguments       Arguments    `json:"arguments"`
	Output          OutputSchema `json:"output,omitempty"`           // Documents expected metrics/data fields
	DefaultName     string       `json:"default_name,omitempty"`     // Go template for default config name (e.g., "Disk: {{.path}}")
	DefaultInterval string       `json:"default_interval,omitempty"` // Suggested run frequency (e.g., "1h", "5m")
	Subcommand      string       `json:"subcommand,omitempty"`       // If set, execute as: binary <subcommand> --args
}

// Arguments describes required and optional probe arguments.
type Arguments struct {
	Required map[string]ArgumentSpec `json:"required,omitempty"`
	Optional map[string]ArgumentSpec `json:"optional,omitempty"`
}

// ArgumentSpec describes a single argument.
type ArgumentSpec struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Default     any      `json:"default,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

// OutputSchema documents the expected probe output fields.
type OutputSchema struct {
	Metrics map[string]MetricSpec `json:"metrics,omitempty"`
	Data    map[string]DataSpec   `json:"data,omitempty"`
}

// MetricSpec describes a metric field in probe output.
type MetricSpec struct {
	Type        string `json:"type"`                  // "number" or "integer"
	Unit        string `json:"unit,omitempty"`        // "bytes", "percent", "hours", etc.
	Description string `json:"description,omitempty"` // Human-readable label
}

// DataSpec describes a data field in probe output.
type DataSpec struct {
	Type        string `json:"type"`                  // "string", "number", "boolean", "array", "object"
	Description string `json:"description,omitempty"` // Human-readable label
}
