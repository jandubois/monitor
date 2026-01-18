package config

// WatcherConfig holds configuration for the watcher service.
type WatcherConfig struct {
	Name          string // Unique watcher name (e.g., "nas", "macbook")
	ProbesDir     string
	MaxConcurrent int
	APIPort       int
	PushURL       string // URL of web service push API
	AuthToken     string // Bearer token for authentication
}

// WebConfig holds configuration for the web server.
type WebConfig struct {
	Port      int
	AuthToken string
}
