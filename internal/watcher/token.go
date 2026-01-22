package watcher

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// LoadOrCreateToken loads an existing token from disk or creates a new one.
// Tokens are stored in ~/.config/monitor/<watcherName>.token with 0600 permissions.
func LoadOrCreateToken(watcherName string) (string, error) {
	configDir, err := getConfigDir()
	if err != nil {
		return "", fmt.Errorf("get config directory: %w", err)
	}

	tokenPath := filepath.Join(configDir, watcherName+".token")

	// Try to read existing token
	data, err := os.ReadFile(tokenPath)
	if err == nil {
		token := string(data)
		if len(token) > 0 {
			return token, nil
		}
	}

	// Generate new token (32 bytes = 64 hex characters)
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)

	// Ensure config directory exists
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return "", fmt.Errorf("create config directory: %w", err)
	}

	// Write token to file
	if err := os.WriteFile(tokenPath, []byte(token), 0600); err != nil {
		return "", fmt.Errorf("write token file: %w", err)
	}

	return token, nil
}

// getConfigDir returns the path to the monitor config directory.
func getConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "monitor"), nil
}
