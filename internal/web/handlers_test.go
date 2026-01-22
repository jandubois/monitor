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
		database.Close()
		t.Fatalf("failed to create server: %v", err)
	}

	cleanup := func() {
		// Clean up test data
		database.DB().ExecContext(ctx, "DELETE FROM probe_results")
		database.DB().ExecContext(ctx, "DELETE FROM probe_configs")
		database.DB().ExecContext(ctx, "DELETE FROM watcher_probe_types")
		database.DB().ExecContext(ctx, "DELETE FROM probe_types")
		database.DB().ExecContext(ctx, "DELETE FROM watchers")
		database.DB().ExecContext(ctx, "DELETE FROM notification_channels")
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
		w.Write([]byte("success"))
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

