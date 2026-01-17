package config

// WatcherConfig holds configuration for the watcher service.
type WatcherConfig struct {
	ProbesDir     string
	MaxConcurrent int
	APIPort       int
}

// WebConfig holds configuration for the web server.
type WebConfig struct {
	Port       int
	AuthToken  string
	WatcherURL string
}
