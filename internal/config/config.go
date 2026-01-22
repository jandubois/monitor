package config

// WatcherConfig holds configuration for the watcher service.
type WatcherConfig struct {
	Name          string // Unique watcher name (e.g., "nas", "macbook")
	ProbesDir     string
	MaxConcurrent int
	APIPort       int
	PushURL       string // URL of web service push API
	CallbackURL   string // URL where web service can reach this watcher (for triggers)
	AuthToken     string // Bearer token for authentication
}

// WebConfig holds configuration for the web server.
type WebConfig struct {
	Name      string // Server name for display in dashboard
	Port      int
	AuthToken string
}
