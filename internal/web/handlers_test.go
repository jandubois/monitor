package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/jandubois/monitor/internal/config"
	"github.com/jandubois/monitor/internal/db"
)

// testServer creates a test server with a real database connection.
// Returns nil if TEST_DATABASE_PATH is not set.
func testServer(t *testing.T) (*Server, func()) {
	t.Helper()

	dbPath := os.Getenv("TEST_DATABASE_PATH")
	if dbPath == "" {
		t.Skip("TEST_DATABASE_PATH not set, skipping integration test")
		return nil, nil
	}

	// Run migrations first
	if err := db.RunMigrations(dbPath); err != nil {
		t.Skipf("failed to run migrations: %v", err)
		return nil, nil
	}

	ctx := context.Background()
	database, err := db.Connect(ctx, dbPath)
	if err != nil {
		t.Skipf("failed to connect to test database: %v", err)
		return nil, nil
	}

	cfg := &config.WebConfig{
		Port:      0, // Not used in tests
		AuthToken: "test-token",
		Name:      "test-server",
	}

	server, err := NewServer(database, cfg)
	if err != nil {
		_ = database.Close()
		t.Fatalf("failed to create server: %v", err)
	}

	cleanup := func() {
		// Clean up test data
		_, _ = database.DB().ExecContext(ctx, "DELETE FROM probe_results")
		_, _ = database.DB().ExecContext(ctx, "DELETE FROM probe_configs")
		_, _ = database.DB().ExecContext(ctx, "DELETE FROM watcher_probe_types")
		_, _ = database.DB().ExecContext(ctx, "DELETE FROM probe_types")
		_, _ = database.DB().ExecContext(ctx, "DELETE FROM watchers")
		_, _ = database.DB().ExecContext(ctx, "DELETE FROM notification_channels")
		_ = database.Close()
	}

	return server, cleanup
}

func TestHandleHealth(t *testing.T) {
	// Health endpoint doesn't require a database
	cfg := &config.WebConfig{
		Port:      0,
		AuthToken: "test-token",
		Name:      "test-server",
	}

	// Create a minimal server just for health check
	s := &Server{config: cfg}

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()

	s.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got %q", resp["status"])
	}
}

func TestHandleStatus(t *testing.T) {
	server, cleanup := testServer(t)
	if server == nil {
		return
	}
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()

	server.handleStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["server_name"] != "test-server" {
		t.Errorf("expected server_name 'test-server', got %v", resp["server_name"])
	}

	if _, ok := resp["watchers"]; !ok {
		t.Error("expected 'watchers' in response")
	}

	if _, ok := resp["all_healthy"]; !ok {
		t.Error("expected 'all_healthy' in response")
	}
}

func TestHandleListProbeTypes(t *testing.T) {
	server, cleanup := testServer(t)
	if server == nil {
		return
	}
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/probe-types", nil)
	w := httptest.NewRecorder()

	server.handleListProbeTypes(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Empty database should return empty list (or nil)
	// Just verify it doesn't error
}

func TestHandleListProbeConfigs(t *testing.T) {
	server, cleanup := testServer(t)
	if server == nil {
		return
	}
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/probe-configs", nil)
	w := httptest.NewRecorder()

	server.handleListProbeConfigs(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestHandleListWatchers(t *testing.T) {
	server, cleanup := testServer(t)
	if server == nil {
		return
	}
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/watchers", nil)
	w := httptest.NewRecorder()

	server.handleListWatchers(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
}

func TestRequireAuth(t *testing.T) {
	cfg := &config.WebConfig{
		Port:      0,
		AuthToken: "secret-token",
		Name:      "test-server",
	}
	s := &Server{config: cfg}

	handler := s.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	}))

	tests := []struct {
		name           string
		authHeader     string
		expectedStatus int
	}{
		{
			name:           "no auth header",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "wrong token",
			authHeader:     "Bearer wrong-token",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "correct token",
			authHeader:     "Bearer secret-token",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestRequireWatcherAuth(t *testing.T) {
	server, cleanup := testServer(t)
	if server == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()

	// Create a test watcher with a token
	_, err := server.db.DB().ExecContext(ctx, `
		INSERT INTO watchers (name, token, approved, paused, registered_at)
		VALUES ('test-watcher', 'watcher-secret-token', 1, 0, datetime('now'))
	`)
	if err != nil {
		t.Fatalf("failed to create test watcher: %v", err)
	}

	// Create an unapproved watcher
	_, err = server.db.DB().ExecContext(ctx, `
		INSERT INTO watchers (name, token, approved, paused, registered_at)
		VALUES ('unapproved-watcher', 'unapproved-token', 0, 1, datetime('now'))
	`)
	if err != nil {
		t.Fatalf("failed to create unapproved watcher: %v", err)
	}

	handler := server.requireWatcherAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		watcherID, ok := WatcherIDFromContext(r.Context())
		if !ok {
			t.Error("expected watcher ID in context")
		}
		watcherName, ok := WatcherNameFromContext(r.Context())
		if !ok {
			t.Error("expected watcher name in context")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success: " + watcherName + " " + strconv.Itoa(watcherID)))
	}))

	tests := []struct {
		name           string
		authHeader     string
		expectedStatus int
	}{
		{
			name:           "no auth header",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "invalid token",
			authHeader:     "Bearer invalid-token",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "unapproved watcher token",
			authHeader:     "Bearer unapproved-token",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "approved watcher token",
			authHeader:     "Bearer watcher-secret-token",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestHandleResultStats(t *testing.T) {
	server, cleanup := testServer(t)
	if server == nil {
		return
	}
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/results/stats", nil)
	w := httptest.NewRecorder()

	server.handleResultStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Check expected fields exist
	expectedFields := []string{"total_configs", "enabled_configs", "status_counts"}
	for _, field := range expectedFields {
		if _, ok := resp[field]; !ok {
			t.Errorf("expected %q in response", field)
		}
	}
}

func TestHandleListNotificationChannels(t *testing.T) {
	server, cleanup := testServer(t)
	if server == nil {
		return
	}
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/notification-channels", nil)
	w := httptest.NewRecorder()

	server.handleListNotificationChannels(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestHandleCreateAndDeleteNotificationChannel(t *testing.T) {
	server, cleanup := testServer(t)
	if server == nil {
		return
	}
	defer cleanup()

	// Create a notification channel
	createBody := `{"name":"test-channel","type":"webhook","config":{"url":"https://example.com/webhook"}}`
	req := httptest.NewRequest("POST", "/api/notification-channels", strings.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleCreateNotificationChannel(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	var created map[string]any
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	id, ok := created["id"].(float64)
	if !ok {
		t.Fatal("expected 'id' in response")
	}

	// Delete the channel
	req = httptest.NewRequest("DELETE", "/api/notification-channels/"+strconv.Itoa(int(id)), nil)
	req.SetPathValue("id", strconv.Itoa(int(id)))
	w = httptest.NewRecorder()

	server.handleDeleteNotificationChannel(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", w.Code)
	}
}

// TestProbeVersionUpgrade verifies that probe configs continue working
// when a watcher upgrades to a newer probe version.
func TestProbeVersionUpgrade(t *testing.T) {
	server, cleanup := testServer(t)
	if server == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()

	// Step 1: Register a watcher with probe type v1.0.0
	registerBody := `{
		"name": "test-watcher",
		"version": "1.0.0",
		"token": "watcher-token",
		"probe_types": [{
			"name": "test-probe",
			"version": "1.0.0",
			"description": "Test probe v1",
			"arguments": {},
			"executable_path": "/bin/true"
		}]
	}`
	req := httptest.NewRequest("POST", "/api/push/register", strings.NewReader(registerBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handlePushRegister(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("registration failed: %d: %s", w.Code, w.Body.String())
	}

	var regResp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&regResp); err != nil {
		t.Fatalf("failed to decode registration response: %v", err)
	}

	watcherID := int(regResp["watcher_id"].(float64))

	// Step 2: Approve the watcher
	_, err := server.db.DB().ExecContext(ctx,
		"UPDATE watchers SET approved = 1, paused = 0 WHERE id = ?", watcherID)
	if err != nil {
		t.Fatalf("failed to approve watcher: %v", err)
	}

	// Step 3: Get the probe type ID
	var probeTypeID int
	err = server.db.DB().QueryRowContext(ctx,
		"SELECT id FROM probe_types WHERE name = 'test-probe' AND version = '1.0.0'").Scan(&probeTypeID)
	if err != nil {
		t.Fatalf("failed to get probe type ID: %v", err)
	}

	// Step 4: Create a probe config using v1.0.0
	createConfigBody := `{
		"probe_type_id": ` + strconv.Itoa(probeTypeID) + `,
		"watcher_id": ` + strconv.Itoa(watcherID) + `,
		"name": "test-config",
		"enabled": true,
		"arguments": {},
		"interval": "1h",
		"timeout_seconds": 30
	}`
	req = httptest.NewRequest("POST", "/api/probe-configs", strings.NewReader(createConfigBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	server.handleCreateProbeConfig(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("config creation failed: %d: %s", w.Code, w.Body.String())
	}

	var configResp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&configResp); err != nil {
		t.Fatalf("failed to decode config response: %v", err)
	}
	configID := int(configResp["id"].(float64))

	// Step 5: Re-register the watcher with probe type v1.1.0 (simulating upgrade)
	upgradeBody := `{
		"name": "test-watcher",
		"version": "1.0.0",
		"token": "watcher-token",
		"probe_types": [{
			"name": "test-probe",
			"version": "1.1.0",
			"description": "Test probe v1.1 with new features",
			"arguments": {},
			"executable_path": "/bin/true-v1.1"
		}]
	}`
	req = httptest.NewRequest("POST", "/api/push/register", strings.NewReader(upgradeBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	server.handlePushRegister(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("re-registration failed: %d: %s", w.Code, w.Body.String())
	}

	// Step 6: Fetch configs for the watcher - should still return the config
	// with the upgraded version
	req = httptest.NewRequest("GET", "/api/push/configs/test-watcher", nil)
	req.Header.Set("Authorization", "Bearer watcher-token")
	req.SetPathValue("watcher", "test-watcher")

	// Set watcher context (normally done by middleware)
	ctx = context.WithValue(ctx, watcherIDKey, watcherID)
	ctx = context.WithValue(ctx, watcherNameKey, "test-watcher")
	req = req.WithContext(ctx)

	w = httptest.NewRecorder()

	server.handlePushGetConfigs(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("get configs failed: %d: %s", w.Code, w.Body.String())
	}

	var configs []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&configs); err != nil {
		t.Fatalf("failed to decode configs response: %v", err)
	}

	// Verify the config is returned
	if len(configs) != 1 {
		t.Fatalf("expected 1 config, got %d", len(configs))
	}

	cfg := configs[0]

	// Verify config ID matches
	if int(cfg["id"].(float64)) != configID {
		t.Errorf("expected config ID %d, got %v", configID, cfg["id"])
	}

	// Verify the probe name is correct
	if cfg["probe_type_name"] != "test-probe" {
		t.Errorf("expected probe_type_name 'test-probe', got %v", cfg["probe_type_name"])
	}

	// KEY CHECK: Verify the version is the NEW version (v1.1.0), not the original
	if cfg["probe_version"] != "1.1.0" {
		t.Errorf("expected probe_version '1.1.0' (upgraded), got %v", cfg["probe_version"])
	}

	// Verify the executable path is the new one
	if cfg["executable_path"] != "/bin/true-v1.1" {
		t.Errorf("expected executable_path '/bin/true-v1.1', got %v", cfg["executable_path"])
	}
}

