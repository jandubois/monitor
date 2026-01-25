package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

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
		database.Close()
		t.Fatalf("failed to create server: %v", err)
	}

	cleanup := func() {
		// Clean up test data
		_, _ = database.DB().ExecContext(ctx, "DELETE FROM probe_results")
		_, _ = database.DB().ExecContext(ctx, "DELETE FROM probe_configs")
		_, _ = database.DB().ExecContext(ctx, "DELETE FROM watcher_probe_types")
		_, _ = database.DB().ExecContext(ctx, "DELETE FROM probe_types")
		_, _ = database.DB().ExecContext(ctx, "DELETE FROM watcher_events")
		_, _ = database.DB().ExecContext(ctx, "DELETE FROM watchers")
		_, _ = database.DB().ExecContext(ctx, "DELETE FROM notification_channels")
		database.Close()
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

// TestWatcherEventRegistered verifies that a 'registered' event is created
// when a new watcher registers.
func TestWatcherEventRegistered(t *testing.T) {
	server, cleanup := testServer(t)
	if server == nil {
		return
	}
	defer cleanup()

	// Register a new watcher
	registerBody := `{
		"name": "new-watcher",
		"version": "1.0.0",
		"token": "new-watcher-token",
		"probe_types": [{
			"name": "test-probe",
			"version": "1.0.0",
			"description": "Test probe",
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

	// Check that a 'registered' event was created
	req = httptest.NewRequest("GET", "/api/watcher-events?watcher_id="+strconv.Itoa(watcherID), nil)
	w = httptest.NewRecorder()

	server.handleListWatcherEvents(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list events failed: %d: %s", w.Code, w.Body.String())
	}

	var events []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&events); err != nil {
		t.Fatalf("failed to decode events response: %v", err)
	}

	// Find the 'registered' event
	found := false
	for _, e := range events {
		if e["event_type"] == "registered" {
			found = true
			if e["severity"] != "info" {
				t.Errorf("expected severity 'info', got %v", e["severity"])
			}
			break
		}
	}

	if !found {
		t.Errorf("expected 'registered' event, got %v", events)
	}
}

// TestWatcherEventProbeVersionUpgrade verifies that a 'probe_version_upgrade'
// event is created when a probe version increases.
func TestWatcherEventProbeVersionUpgrade(t *testing.T) {
	server, cleanup := testServer(t)
	if server == nil {
		return
	}
	defer cleanup()

	// Step 1: Register watcher with probe v1.0.0
	registerBody := `{
		"name": "upgrade-watcher",
		"version": "1.0.0",
		"token": "upgrade-token",
		"probe_types": [{
			"name": "versioned-probe",
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
		t.Fatalf("initial registration failed: %d: %s", w.Code, w.Body.String())
	}

	var regResp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&regResp); err != nil {
		t.Fatalf("failed to decode registration response: %v", err)
	}
	watcherID := int(regResp["watcher_id"].(float64))

	// Step 2: Re-register with probe v1.1.0 (upgrade)
	upgradeBody := `{
		"name": "upgrade-watcher",
		"version": "1.0.0",
		"token": "upgrade-token",
		"probe_types": [{
			"name": "versioned-probe",
			"version": "1.1.0",
			"description": "Test probe v1.1",
			"arguments": {},
			"executable_path": "/bin/true"
		}]
	}`
	req = httptest.NewRequest("POST", "/api/push/register", strings.NewReader(upgradeBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	server.handlePushRegister(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("upgrade registration failed: %d: %s", w.Code, w.Body.String())
	}

	// Step 3: Check for 'probe_version_upgrade' event
	req = httptest.NewRequest("GET", "/api/watcher-events?watcher_id="+strconv.Itoa(watcherID)+"&type=probe_version_upgrade", nil)
	w = httptest.NewRecorder()

	server.handleListWatcherEvents(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list events failed: %d: %s", w.Code, w.Body.String())
	}

	var events []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&events); err != nil {
		t.Fatalf("failed to decode events response: %v", err)
	}

	if len(events) == 0 {
		t.Fatal("expected 'probe_version_upgrade' event, got none")
	}

	event := events[0]
	if event["severity"] != "info" {
		t.Errorf("expected severity 'info', got %v", event["severity"])
	}

	// Check the summary contains version info
	summary := event["summary"].(string)
	if !strings.Contains(summary, "1.0.0") || !strings.Contains(summary, "1.1.0") {
		t.Errorf("expected summary to mention version change, got %q", summary)
	}
}

// TestWatcherEventProbeVersionDowngrade verifies that a 'probe_version_downgrade'
// event is created when a probe version decreases.
func TestWatcherEventProbeVersionDowngrade(t *testing.T) {
	server, cleanup := testServer(t)
	if server == nil {
		return
	}
	defer cleanup()

	// Step 1: Register watcher with probe v2.0.0
	registerBody := `{
		"name": "downgrade-watcher",
		"version": "1.0.0",
		"token": "downgrade-token",
		"probe_types": [{
			"name": "versioned-probe",
			"version": "2.0.0",
			"description": "Test probe v2",
			"arguments": {},
			"executable_path": "/bin/true"
		}]
	}`
	req := httptest.NewRequest("POST", "/api/push/register", strings.NewReader(registerBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handlePushRegister(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("initial registration failed: %d: %s", w.Code, w.Body.String())
	}

	var regResp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&regResp); err != nil {
		t.Fatalf("failed to decode registration response: %v", err)
	}
	watcherID := int(regResp["watcher_id"].(float64))

	// Step 2: Re-register with probe v1.0.0 (downgrade)
	downgradeBody := `{
		"name": "downgrade-watcher",
		"version": "1.0.0",
		"token": "downgrade-token",
		"probe_types": [{
			"name": "versioned-probe",
			"version": "1.0.0",
			"description": "Test probe v1",
			"arguments": {},
			"executable_path": "/bin/true"
		}]
	}`
	req = httptest.NewRequest("POST", "/api/push/register", strings.NewReader(downgradeBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	server.handlePushRegister(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("downgrade registration failed: %d: %s", w.Code, w.Body.String())
	}

	// Step 3: Check for 'probe_version_downgrade' event
	req = httptest.NewRequest("GET", "/api/watcher-events?watcher_id="+strconv.Itoa(watcherID)+"&type=probe_version_downgrade", nil)
	w = httptest.NewRecorder()

	server.handleListWatcherEvents(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list events failed: %d: %s", w.Code, w.Body.String())
	}

	var events []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&events); err != nil {
		t.Fatalf("failed to decode events response: %v", err)
	}

	if len(events) == 0 {
		t.Fatal("expected 'probe_version_downgrade' event, got none")
	}

	event := events[0]
	if event["severity"] != "warning" {
		t.Errorf("expected severity 'warning', got %v", event["severity"])
	}
}

// TestWatcherEventAcknowledge verifies that events can be acknowledged.
func TestWatcherEventAcknowledge(t *testing.T) {
	server, cleanup := testServer(t)
	if server == nil {
		return
	}
	defer cleanup()

	// Register a watcher to create a 'registered' event
	registerBody := `{
		"name": "ack-watcher",
		"version": "1.0.0",
		"token": "ack-token",
		"probe_types": []
	}`
	req := httptest.NewRequest("POST", "/api/push/register", strings.NewReader(registerBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handlePushRegister(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("registration failed: %d: %s", w.Code, w.Body.String())
	}

	// Get the event
	req = httptest.NewRequest("GET", "/api/watcher-events?type=registered", nil)
	w = httptest.NewRecorder()

	server.handleListWatcherEvents(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list events failed: %d: %s", w.Code, w.Body.String())
	}

	var events []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&events); err != nil {
		t.Fatalf("failed to decode events response: %v", err)
	}

	if len(events) == 0 {
		t.Fatal("expected at least one event")
	}

	eventID := int(events[0]["id"].(float64))

	// Verify it's not acknowledged
	if events[0]["acknowledged"].(bool) {
		t.Error("expected event to be unacknowledged initially")
	}

	// Acknowledge the event
	req = httptest.NewRequest("PUT", "/api/watcher-events/"+strconv.Itoa(eventID)+"/acknowledge", nil)
	req.SetPathValue("id", strconv.Itoa(eventID))
	w = httptest.NewRecorder()

	server.handleAcknowledgeWatcherEvent(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("acknowledge failed: %d: %s", w.Code, w.Body.String())
	}

	// Verify unacknowledged filter excludes it
	req = httptest.NewRequest("GET", "/api/watcher-events?unacknowledged=true&type=registered", nil)
	w = httptest.NewRecorder()

	server.handleListWatcherEvents(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list events failed: %d: %s", w.Code, w.Body.String())
	}

	var unackEvents []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&unackEvents); err != nil {
		t.Fatalf("failed to decode events response: %v", err)
	}

	// The acknowledged event should be excluded
	for _, e := range unackEvents {
		if int(e["id"].(float64)) == eventID {
			t.Error("acknowledged event should be excluded from unacknowledged filter")
		}
	}
}

// TestOrphanedProbeConfig verifies that a probe config is marked as orphaned
// when no watcher provides a probe with the same name.
func TestOrphanedProbeConfig(t *testing.T) {
	server, cleanup := testServer(t)
	if server == nil {
		return
	}
	defer cleanup()

	ctx := server.db.DB()

	// Create a probe type without a watcher link
	_, err := ctx.Exec(`
		INSERT INTO probe_types (name, version, description, arguments, registered_at)
		VALUES ('orphaned-probe', '1.0.0', 'A probe that will be orphaned', '{}', datetime('now'))
	`)
	if err != nil {
		t.Fatalf("failed to create probe type: %v", err)
	}

	var probeTypeID int
	err = ctx.QueryRow(`SELECT id FROM probe_types WHERE name = 'orphaned-probe'`).Scan(&probeTypeID)
	if err != nil {
		t.Fatalf("failed to get probe type ID: %v", err)
	}

	// Create a config pointing to this orphaned probe type
	_, err = ctx.Exec(`
		INSERT INTO probe_configs (probe_type_id, name, enabled, arguments, interval, timeout_seconds, notification_channels)
		VALUES (?, 'Orphaned Test', 1, '{}', '1h', 60, '[]')
	`, probeTypeID)
	if err != nil {
		t.Fatalf("failed to create probe config: %v", err)
	}

	// List configs and verify orphaned flag
	req := httptest.NewRequest("GET", "/api/probe-configs", nil)
	w := httptest.NewRecorder()

	server.handleListProbeConfigs(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list configs failed: %d: %s", w.Code, w.Body.String())
	}

	var configs []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&configs); err != nil {
		t.Fatalf("failed to decode configs response: %v", err)
	}

	found := false
	for _, cfg := range configs {
		if cfg["name"] == "Orphaned Test" {
			found = true
			orphaned, ok := cfg["orphaned"].(bool)
			if !ok || !orphaned {
				t.Error("expected orphaned config to have orphaned=true")
			}
			break
		}
	}

	if !found {
		t.Fatal("orphaned config not found in response")
	}

	// Now register a watcher with this probe type
	registerBody := `{
		"name": "rescue-watcher",
		"version": "1.0.0",
		"token": "rescue-token",
		"probe_types": [{
			"name": "orphaned-probe",
			"version": "2.0.0",
			"description": "Rescued probe",
			"arguments": {},
			"executable_path": "/bin/true"
		}]
	}`
	req = httptest.NewRequest("POST", "/api/push/register", strings.NewReader(registerBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	server.handlePushRegister(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("registration failed: %d: %s", w.Code, w.Body.String())
	}

	// Verify config is no longer orphaned
	req = httptest.NewRequest("GET", "/api/probe-configs", nil)
	w = httptest.NewRecorder()

	server.handleListProbeConfigs(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list configs failed: %d: %s", w.Code, w.Body.String())
	}

	// Use a fresh slice to avoid Go's JSON decoder reusing map objects
	var configsAfter []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&configsAfter); err != nil {
		t.Fatalf("failed to decode configs response: %v", err)
	}

	for _, cfg := range configsAfter {
		if cfg["name"] == "Orphaned Test" {
			if orphaned, ok := cfg["orphaned"].(bool); ok && orphaned {
				t.Error("config should no longer be orphaned after watcher registration")
			}
			break
		}
	}
}

// TestOrphanedProbeNotDeliveredToWatcher verifies that an orphaned probe config
// is not delivered to the watcher when it polls for configs.
func TestOrphanedProbeNotDeliveredToWatcher(t *testing.T) {
	server, cleanup := testServer(t)
	if server == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()

	// Register a watcher first
	registerBody := `{
		"name": "orphan-test-watcher",
		"version": "1.0.0",
		"token": "orphan-watcher-token",
		"probe_types": [{
			"name": "active-probe",
			"version": "1.0.0",
			"description": "Active probe",
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

	// Approve the watcher
	_, err := server.db.DB().ExecContext(ctx,
		"UPDATE watchers SET approved = 1, paused = 0 WHERE id = ?", watcherID)
	if err != nil {
		t.Fatalf("failed to approve watcher: %v", err)
	}

	// Get the active probe type ID
	var activeProbeTypeID int
	err = server.db.DB().QueryRowContext(ctx,
		"SELECT id FROM probe_types WHERE name = 'active-probe'").Scan(&activeProbeTypeID)
	if err != nil {
		t.Fatalf("failed to get probe type ID: %v", err)
	}

	// Create a config for the active probe
	_, err = server.db.DB().ExecContext(ctx, `
		INSERT INTO probe_configs (probe_type_id, watcher_id, name, enabled, arguments, interval, timeout_seconds, notification_channels)
		VALUES (?, ?, 'Active Probe Config', 1, '{}', '1h', 60, '[]')
	`, activeProbeTypeID, watcherID)
	if err != nil {
		t.Fatalf("failed to create active probe config: %v", err)
	}

	// Create an orphaned probe type (no watcher link)
	_, err = server.db.DB().ExecContext(ctx, `
		INSERT INTO probe_types (name, version, description, arguments, registered_at)
		VALUES ('orphaned-probe', '1.0.0', 'Orphaned probe', '{}', datetime('now'))
	`)
	if err != nil {
		t.Fatalf("failed to create orphaned probe type: %v", err)
	}

	var orphanedProbeTypeID int
	err = server.db.DB().QueryRowContext(ctx,
		"SELECT id FROM probe_types WHERE name = 'orphaned-probe'").Scan(&orphanedProbeTypeID)
	if err != nil {
		t.Fatalf("failed to get orphaned probe type ID: %v", err)
	}

	// Create a config for the orphaned probe (assigned to the watcher)
	_, err = server.db.DB().ExecContext(ctx, `
		INSERT INTO probe_configs (probe_type_id, watcher_id, name, enabled, arguments, interval, timeout_seconds, notification_channels)
		VALUES (?, ?, 'Orphaned Probe Config', 1, '{}', '1h', 60, '[]')
	`, orphanedProbeTypeID, watcherID)
	if err != nil {
		t.Fatalf("failed to create orphaned probe config: %v", err)
	}

	// Now get configs for the watcher - should only return the active probe
	req = httptest.NewRequest("GET", "/api/push/configs/orphan-test-watcher", nil)
	req.Header.Set("Authorization", "Bearer orphan-watcher-token")
	req.SetPathValue("watcher", "orphan-test-watcher")

	// Set watcher context
	ctx = context.WithValue(ctx, watcherIDKey, watcherID)
	ctx = context.WithValue(ctx, watcherNameKey, "orphan-test-watcher")
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

	// Should only have the active probe config, not the orphaned one
	if len(configs) != 1 {
		t.Errorf("expected 1 config, got %d", len(configs))
	}

	if len(configs) > 0 && configs[0]["name"] != "Active Probe Config" {
		t.Errorf("expected 'Active Probe Config', got %v", configs[0]["name"])
	}

	// Verify the orphaned config exists in the API with orphaned=true
	req = httptest.NewRequest("GET", "/api/probe-configs", nil)
	w = httptest.NewRecorder()

	server.handleListProbeConfigs(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list configs failed: %d: %s", w.Code, w.Body.String())
	}

	var allConfigs []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&allConfigs); err != nil {
		t.Fatalf("failed to decode configs response: %v", err)
	}

	orphanedFound := false
	for _, cfg := range allConfigs {
		if cfg["name"] == "Orphaned Probe Config" {
			orphanedFound = true
			if orphaned, ok := cfg["orphaned"].(bool); !ok || !orphaned {
				t.Error("expected orphaned probe config to have orphaned=true")
			}
		}
	}

	if !orphanedFound {
		t.Error("orphaned probe config not found in list")
	}
}

// TestWatcherApprovalFlow tests the complete watcher approval flow:
// 1. Register watcher, then re-register with new token (creates token_changed event)
// 2. Verify watcher is unapproved with unacknowledged events
// 3. Approve watcher (should auto-acknowledge token_changed events and update last_seen_at)
// 4. Verify events are acknowledged and watcher is healthy
func TestWatcherApprovalFlow(t *testing.T) {
	server, cleanup := testServer(t)
	if server == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()

	// Create a mock watcher health endpoint
	mockWatcher := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockWatcher.Close()

	// Register a watcher with callback URL
	registerBody := fmt.Sprintf(`{
		"name": "approval-test-watcher",
		"version": "1.0.0",
		"token": "token-v1",
		"callback_url": %q,
		"probe_types": []
	}`, mockWatcher.URL)
	req := httptest.NewRequest("POST", "/api/push/register", strings.NewReader(registerBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.handlePushRegister(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("initial registration failed: %d: %s", w.Code, w.Body.String())
	}

	var regResp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&regResp); err != nil {
		t.Fatalf("failed to decode registration response: %v", err)
	}
	watcherID := int(regResp["watcher_id"].(float64))

	// Re-register with a different token to trigger token_changed event
	registerBody = fmt.Sprintf(`{
		"name": "approval-test-watcher",
		"version": "1.0.0",
		"token": "token-v2",
		"callback_url": %q,
		"probe_types": []
	}`, mockWatcher.URL)
	req = httptest.NewRequest("POST", "/api/push/register", strings.NewReader(registerBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.handlePushRegister(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("re-registration failed: %d: %s", w.Code, w.Body.String())
	}

	// Verify watcher is not approved
	var approved int
	err := server.db.DB().QueryRowContext(ctx, `SELECT approved FROM watchers WHERE id = ?`, watcherID).Scan(&approved)
	if err != nil {
		t.Fatalf("failed to query watcher: %v", err)
	}
	if approved != 0 {
		t.Error("expected watcher to be unapproved after token change")
	}

	// Verify there are unacknowledged token_changed events
	var unackCount int
	err = server.db.DB().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM watcher_events
		WHERE watcher_id = ? AND event_type = 'token_changed' AND acknowledged = 0
	`, watcherID).Scan(&unackCount)
	if err != nil {
		t.Fatalf("failed to count events: %v", err)
	}
	if unackCount == 0 {
		t.Error("expected unacknowledged token_changed event after re-registration")
	}

	// Set last_seen_at to an old time to verify it gets updated on approval
	_, err = server.db.DB().ExecContext(ctx, `
		UPDATE watchers SET last_seen_at = datetime('now', '-1 hour') WHERE id = ?
	`, watcherID)
	if err != nil {
		t.Fatalf("failed to update last_seen_at: %v", err)
	}

	// Approve the watcher
	approveBody := `{"paused": false}`
	req = httptest.NewRequest("PUT", "/api/watchers/"+strconv.Itoa(watcherID)+"/paused", strings.NewReader(approveBody))
	req.SetPathValue("id", strconv.Itoa(watcherID))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.handleSetWatcherPaused(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("approval failed: %d: %s", w.Code, w.Body.String())
	}

	// Verify watcher is now approved
	err = server.db.DB().QueryRowContext(ctx, `SELECT approved FROM watchers WHERE id = ?`, watcherID).Scan(&approved)
	if err != nil {
		t.Fatalf("failed to query watcher after approval: %v", err)
	}
	if approved != 1 {
		t.Error("expected watcher to be approved after approval")
	}

	// Verify token_changed events are now acknowledged
	err = server.db.DB().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM watcher_events
		WHERE watcher_id = ? AND event_type = 'token_changed' AND acknowledged = 0
	`, watcherID).Scan(&unackCount)
	if err != nil {
		t.Fatalf("failed to count events after approval: %v", err)
	}
	if unackCount != 0 {
		t.Errorf("expected token_changed events to be auto-acknowledged, but found %d unacknowledged", unackCount)
	}

	// Verify last_seen_at was updated (watcher should be healthy)
	var lastSeenAtStr string
	err = server.db.DB().QueryRowContext(ctx, `SELECT last_seen_at FROM watchers WHERE id = ?`, watcherID).Scan(&lastSeenAtStr)
	if err != nil {
		t.Fatalf("failed to query last_seen_at: %v", err)
	}
	lastSeenAt, err := time.Parse("2006-01-02 15:04:05", lastSeenAtStr)
	if err != nil {
		t.Fatalf("failed to parse last_seen_at: %v", err)
	}
	if time.Since(lastSeenAt) > 5*time.Second {
		t.Errorf("expected last_seen_at to be recent (within 5s), but it was %v ago", time.Since(lastSeenAt))
	}
}

