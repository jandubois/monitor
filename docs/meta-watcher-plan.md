# Meta-Watcher Plan

## Goal

Monitor the monitoring system itself. Detect and alert when watchers malfunction, disconnect, or have configuration issues like probe version downgrades.

## Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  Meta-Watcher   │────▶│   Web Service   │◀────│    Watchers     │
│  (privileged)   │     │   (database)    │     │   (monitored)   │
└─────────────────┘     └─────────────────┘     └─────────────────┘
        │                       │
        │  POST /api/push/*     │
        │  (report results)     │
        ▼                       │
   Uses AUTH_TOKEN              │
   to read /api/watchers        │
   and /api/watcher-events      │
```

**Key decisions:**
- Meta-watcher uses the privileged user token (AUTH_TOKEN), not a watcher token
- Accesses data via API only (no direct database access)
- Reports findings as probe results through normal watcher protocol
- Auto-creates probe configs for each monitored watcher

## Database Changes

### New table: `watcher_events`

```sql
CREATE TABLE watcher_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    watcher_id INTEGER NOT NULL REFERENCES watchers(id) ON DELETE CASCADE,
    timestamp TEXT NOT NULL DEFAULT (datetime('now')),
    event_type TEXT NOT NULL,
    severity TEXT NOT NULL DEFAULT 'info',  -- info, warning, error
    summary TEXT NOT NULL,                   -- one-line description
    details TEXT,                            -- JSON with event-specific data
    acknowledged INTEGER NOT NULL DEFAULT 0,
    acknowledged_at TEXT,
    INDEX idx_watcher_events_watcher (watcher_id),
    INDEX idx_watcher_events_timestamp (timestamp)
);
```

**Event types:**

| Type | Severity | Trigger |
|------|----------|---------|
| `registered` | info | New watcher registers |
| `connection_lost` | error | No heartbeat for threshold duration |
| `connection_restored` | info | Heartbeat received after connection_lost |
| `probe_version_upgrade` | info | Probe registered with higher version |
| `probe_version_downgrade` | warning | Probe registered with lower version |
| `watcher_version_changed` | info | Watcher binary version changed |
| `auth_failed` | error | Watcher token rejected |
| `registration_rejected` | error | Registration failed (other reason) |

### Migration: `004_watcher_events.up.sql`

```sql
CREATE TABLE watcher_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    watcher_id INTEGER NOT NULL REFERENCES watchers(id) ON DELETE CASCADE,
    timestamp TEXT NOT NULL DEFAULT (datetime('now')),
    event_type TEXT NOT NULL,
    severity TEXT NOT NULL DEFAULT 'info',
    summary TEXT NOT NULL,
    details TEXT,
    acknowledged INTEGER NOT NULL DEFAULT 0,
    acknowledged_at TEXT
);

CREATE INDEX idx_watcher_events_watcher ON watcher_events(watcher_id);
CREATE INDEX idx_watcher_events_timestamp ON watcher_events(timestamp);
```

## API Changes

### New endpoint: `GET /api/watcher-events`

Query watcher events with filtering.

**Parameters:**
- `watcher_id` - filter by watcher (optional)
- `type` - filter by event type (optional)
- `severity` - filter by severity (optional)
- `since` - events after timestamp (optional)
- `unacknowledged` - only unacknowledged events (optional)
- `limit` - max results (default 100)

**Response:**
```json
[
  {
    "id": 1,
    "watcher_id": 1,
    "watcher_name": "default",
    "timestamp": "2025-01-25T12:00:00Z",
    "event_type": "probe_version_downgrade",
    "severity": "warning",
    "summary": "Probe disk-space downgraded from 1.1.0 to 1.0.0",
    "details": {
      "probe_name": "disk-space",
      "old_version": "1.1.0",
      "new_version": "1.0.0"
    },
    "acknowledged": false
  }
]
```

### New endpoint: `PUT /api/watcher-events/{id}/acknowledge`

Mark an event as acknowledged.

**Response:**
```json
{"acknowledged": true}
```

### New endpoint: `GET /api/watchers/{id}/events`

Convenience endpoint to get events for a specific watcher.

## Web Service Changes

### Event generation

The web service generates events during normal operations:

1. **In `handlePushRegister`:**
   - `registered` - new watcher
   - `probe_version_upgrade` / `probe_version_downgrade` - version changes
   - `watcher_version_changed` - watcher binary version changes

2. **In `handlePushHeartbeat`:**
   - `connection_restored` - if previous state was disconnected

3. **In authentication middleware:**
   - `auth_failed` - invalid token

4. **Background goroutine (new):**
   - `connection_lost` - periodically check for stale watchers

### Connection monitoring goroutine

New background process in web service:

```go
func (s *Server) monitorWatcherConnections(ctx context.Context, threshold time.Duration) {
    ticker := time.NewTicker(threshold / 2)
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            s.checkStaleWatchers(threshold)
        }
    }
}
```

Checks each watcher's `last_seen_at` and generates `connection_lost` event if stale and no recent `connection_lost` event exists.

## Meta-Watcher Command

### Usage

```bash
monitor meta-watcher \
  --web-url=http://localhost:8080 \
  --token=AUTH_TOKEN \
  --threshold=1m \
  --interval=30s
```

**Flags:**
- `--web-url` - Web service URL (required)
- `--token` - AUTH_TOKEN for privileged API access (required)
- `--threshold` - Duration without heartbeat before unhealthy (default: 1m)
- `--interval` - How often to check (default: 30s)

### Behavior

1. **On startup:**
   - Register as a watcher named `meta-watcher`
   - Query `/api/watchers` to discover all watchers
   - Auto-create probe config for each watcher (excluding itself)

2. **On each interval:**
   - Query `/api/watchers` for current state
   - Query `/api/watcher-events?unacknowledged=true` for recent events
   - For each watcher, generate a probe result:
     - Status: `ok` if healthy, `critical` if connection_lost, `warning` if has warning events
     - Summary: "Healthy" or description of issue
     - Message: Full details including recent events
     - Metrics: `last_seen_seconds_ago`, `event_count`

3. **On watcher list change:**
   - Create probe config for new watchers
   - Delete probe config for removed watchers (or mark disabled)

### Probe result example

```json
{
  "status": "warning",
  "summary": "Probe version downgrade detected",
  "message": "Watcher 'production' has 1 warning event:\n- Probe disk-space downgraded from 1.1.0 to 1.0.0",
  "metrics": {
    "last_seen_seconds_ago": 15,
    "unacknowledged_events": 1
  },
  "data": {
    "watcher_name": "production",
    "watcher_version": "1.0.0",
    "probe_type_count": 5
  }
}
```

## UI Changes

### Watcher detail page

Add "Events" section showing recent events for that watcher:

- Event list with timestamp, type, severity, summary
- "Acknowledge" button for unacknowledged events
- Filter by severity/type

### Watcher list page

- Add event indicator (badge/icon) for watchers with unacknowledged events
- Color-code by severity

### New page: System Events (optional)

Global view of all watcher events across the system:

- Filterable table
- Bulk acknowledge
- Link to watcher detail

## Docker Compose Changes

Add meta-watcher service:

```yaml
meta-watcher:
  image: monitor:latest
  command: ["meta-watcher", "--web-url=http://web:8080", "--token=${AUTH_TOKEN}", "--threshold=1m"]
  depends_on:
    web:
      condition: service_healthy
  restart: unless-stopped
```

## Implementation Order

1. **Database migration** - Add `watcher_events` table
2. **Event generation** - Add event creation to web service handlers
3. **API endpoints** - Add event query and acknowledge endpoints
4. **Connection monitor** - Add background goroutine for connection_lost events
5. **Meta-watcher command** - Implement the new command
6. **UI: Watcher events** - Add events section to watcher detail page
7. **UI: Event indicators** - Add badges to watcher list
8. **Docker compose** - Add meta-watcher service

## Deferred

- Watcher diagnostic endpoints (`/health`, `/logs` on callback URL)
- Structured logging
- Event retention/cleanup policy
- System Events page (can use watcher detail pages initially)

## Testing

1. **Unit tests:**
   - Event generation logic
   - Event query filtering

2. **E2E tests:**
   - Watcher registers → `registered` event created
   - Probe version downgrade → `probe_version_downgrade` event created
   - Watcher stops heartbeating → `connection_lost` event after threshold
   - Watcher resumes → `connection_restored` event

3. **Manual testing:**
   - Start meta-watcher, verify probe configs created
   - Stop a watcher, verify critical status reported
   - Trigger version downgrade, verify warning in UI
