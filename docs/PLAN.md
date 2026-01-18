# Monitor - Architecture Specification

Personal infrastructure monitoring system for tracking diverse digital systems with flexible, self-describing probes.

## System Overview

```
                                    ┌─────────────────────────────────────┐
                                    │         Web Service (central)       │
                                    │                                     │
  ┌─────────────────────┐           │  - REST API for SPA                 │
  │     PostgreSQL      │◄──────────│  - Push API for watchers/alerts     │
  │                     │           │  - Serves React static files        │
  │  - watchers         │           │  - Auth middleware                  │
  │  - probe_types      │           │  - Notification dispatcher          │
  │  - probe_configs    │           │                                     │
  │  - probe_results    │           └──────────────┬──────────────────────┘
  │  - notifications    │                          │
  └─────────────────────┘                          │ HTTPS
                                                   │
          ┌────────────────────────────────────────┼────────────────────────┐
          │                                        │                        │
          ▼                                        ▼                        ▼
┌─────────────────────┐              ┌─────────────────────┐    ┌──────────────────┐
│   Watcher (NAS)     │              │   Watcher (Mac)     │    │ External Systems │
│   name: "nas"       │              │   name: "macbook"   │    │                  │
│                     │──POST───────▶│                     │    │  GitHub Actions  │
│   Probes:           │   results    │   Probes:           │    │  UptimeRobot     │
│   - disk-space      │              │   - disk-space      │    │  Custom scripts  │
│   - backup-check    │              │   - git-status      │    │                  │
└─────────────────────┘              └─────────────────────┘    └────────┬─────────┘
                                                                         │
                                                                    POST alerts
                                                                         │
                                                                         ▼
                                                              ┌─────────────────────┐
                                                              │   Browser (SPA)     │
                                                              │   - Dashboard       │
                                                              │   - Config UI       │
                                                              │   - History/Trends  │
                                                              └─────────────────────┘
```

**Key design principles:**
- Web service is the central hub; all writes go through its API
- Watchers push results via HTTP (no direct database access)
- External systems can push alerts directly via webhook
- Multiple watchers supported, each with its own local probes
- Probe types shared by name+version across watchers
- Single Go binary with `watcher` and `web` modes

## Components

### 1. Go Binary (`monitor`)

Single binary with two operational modes:

**`monitor watcher --name <name>`** - Background scheduler service
- Registers itself with web service on startup
- Discovers local probes and registers their types
- Fetches probe configs assigned to this watcher from web service
- Schedules and executes probes as subprocesses (async, parallel)
- Global concurrency limit (configurable)
- Timeout enforcement: SIGTERM, then SIGKILL after grace period
- Pushes results to web service via HTTP
- Sends heartbeat to web service periodically
- Minimal local HTTP API (optional, for debugging):
  - `GET /health` - Liveness check

**`monitor web`** - Central web service
- REST API for SPA (CRUD operations)
- Push API for watchers (results, heartbeats, probe type registration)
- Push API for external alerts (webhooks)
- Notification dispatcher (triggers on status changes)
- Serves embedded React static files
- Token-based authentication (shared token for watchers and UI)
- Tracks watcher health via heartbeat freshness

### 2. Database (PostgreSQL)

```sql
-- Registered watchers
watchers (
  id SERIAL PRIMARY KEY,
  name TEXT UNIQUE NOT NULL,         -- e.g., "nas", "macbook"
  last_seen_at TIMESTAMPTZ,
  version TEXT,                      -- monitor binary version
  registered_at TIMESTAMPTZ DEFAULT NOW()
)

-- Registered probe types (from --describe)
-- Shared by name+version; different versions coexist
probe_types (
  id SERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  version TEXT NOT NULL,
  description TEXT,
  arguments JSONB,                   -- {required: {...}, optional: {...}}
  registered_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ,
  UNIQUE(name, version)
)

-- Which watchers have which probe types available
watcher_probe_types (
  watcher_id INTEGER REFERENCES watchers(id) ON DELETE CASCADE,
  probe_type_id INTEGER REFERENCES probe_types(id) ON DELETE CASCADE,
  executable_path TEXT NOT NULL,     -- path on that watcher
  PRIMARY KEY (watcher_id, probe_type_id)
)

-- Configured probe instances
probe_configs (
  id SERIAL PRIMARY KEY,
  probe_type_id INTEGER REFERENCES probe_types(id),
  watcher_id INTEGER REFERENCES watchers(id),  -- which watcher runs this
  name TEXT NOT NULL,
  enabled BOOLEAN DEFAULT true,
  arguments JSONB,                   -- configured argument values
  interval TEXT NOT NULL,            -- '1m', '5m', '1h', '1d'
  timeout_seconds INTEGER DEFAULT 60,
  next_run_at TIMESTAMPTZ,           -- when to run next (probe can override)
  group_path TEXT,                   -- e.g., "Backups", "Backups/Photos"
  keywords TEXT[],                   -- e.g., ["personal", "nas"]
  notification_channels INTEGER[],
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ
)

-- Historical results
probe_results (
  id SERIAL PRIMARY KEY,
  probe_config_id INTEGER REFERENCES probe_configs(id),
  watcher_id INTEGER REFERENCES watchers(id),  -- which watcher ran it
  status TEXT NOT NULL,              -- 'ok', 'warning', 'critical', 'unknown'
  message TEXT,
  metrics JSONB,                     -- numeric values for trends
  data JSONB,                        -- arbitrary probe-specific data
  duration_ms INTEGER,
  next_run_at TIMESTAMPTZ,           -- probe-requested next run time
  scheduled_at TIMESTAMPTZ,          -- when it was supposed to run
  executed_at TIMESTAMPTZ,           -- when it actually ran
  recorded_at TIMESTAMPTZ DEFAULT NOW()
)

-- Track missed runs
missed_runs (
  id SERIAL PRIMARY KEY,
  probe_config_id INTEGER REFERENCES probe_configs(id),
  scheduled_at TIMESTAMPTZ,
  reason TEXT                        -- 'watcher_down', 'timeout', 'error'
)

-- Notification channels
notification_channels (
  id SERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  type TEXT NOT NULL,                -- 'pushover', 'ntfy', 'email'
  config JSONB,                      -- channel-specific settings
  enabled BOOLEAN DEFAULT true
)

-- Indexes
CREATE INDEX idx_results_config_time ON probe_results(probe_config_id, executed_at DESC);
CREATE INDEX idx_results_status ON probe_results(status) WHERE status != 'ok';
CREATE INDEX idx_configs_watcher ON probe_configs(watcher_id) WHERE enabled;
CREATE INDEX idx_configs_group ON probe_configs(group_path);
CREATE INDEX idx_configs_keywords ON probe_configs USING GIN(keywords);
```

### 3. Probes

Self-describing executables executed as subprocesses. Can be any language (Go, Python, Bash, etc.) - watcher just executes them and parses stdout.

**Self-description interface:**
```bash
$ disk-space --describe
{
  "name": "disk-space",
  "description": "Check available disk space on a path",
  "version": "1.0.0",
  "arguments": {
    "required": {
      "path": {"type": "string", "description": "Path to check"}
    },
    "optional": {
      "min_free_gb": {"type": "number", "default": 10, "description": "Minimum free GB"},
      "min_free_percent": {"type": "number", "description": "Minimum free %"}
    }
  }
}
```

**Execution interface:**
```bash
$ disk-space --path /volume1 --min_free_gb 100
# stdout (final JSON only):
{
  "status": "ok",
  "message": "532 GB free on /volume1 (53%)",
  "metrics": {
    "free_bytes": 571230126080,
    "total_bytes": 1077895069696,
    "free_percent": 53.0
  },
  "data": {
    "filesystem": "btrfs",
    "mount_point": "/volume1"
  },
  "next_run": "2024-01-19T06:00:00Z"   // optional: override next run time
}
```

**Execution model:**
- Watcher spawns probe as subprocess with configured arguments
- Probe writes final JSON to stdout when complete
- Watcher captures stdout, parses JSON, pushes result to web service
- Timeout: watcher sends SIGTERM, waits grace period, then SIGKILL
- No streaming; probe must complete and return single JSON object
- If probe returns `next_run`, it overrides the interval-based schedule

**Status values:**
- `ok` - Normal operation
- `warning` - Attention needed, not critical
- `critical` - Immediate attention required
- `unknown` - Could not determine status (probe error)

**Initial probes to build:**
- `disk-space` - Check free space on paths
- `backup-age` - Check when last backup completed
- `file-exists` - Verify file exists, optionally check age
- `git-status` - Check for uncommitted changes
- `github-action` - Check last GitHub Action run
- `http-health` - HTTP endpoint health check
- `command` - Run command, check exit code

### 4. Web Frontend (React SPA)

TypeScript/React single-page application served by Go backend.

**Features:**
- Dashboard: all probe statuses at a glance
- Detail view: probe history, metrics charts
- Configuration: add/edit probe instances using self-described arguments
- Notifications: configure channels
- System health: watcher status, missed runs

**Tech stack:**
- React 18+ with TypeScript
- React Query for data fetching/caching
- Recharts for trend visualization
- Tailwind CSS

### 5. REST API (Web Service)

```
# Watchers
GET    /api/watchers                 # List registered watchers with health status
GET    /api/watchers/:id             # Get single watcher with its probe types

# Probe types
GET    /api/probe-types              # List all probe types
GET    /api/probe-types?watcher=:id  # List probe types available on a watcher

# Probe configs
GET    /api/probe-configs            # List all (supports ?group=, ?keywords=, ?watcher=)
POST   /api/probe-configs            # Create new probe config
GET    /api/probe-configs/:id        # Get single config with latest result
PUT    /api/probe-configs/:id        # Update config
DELETE /api/probe-configs/:id        # Delete config
POST   /api/probe-configs/:id/run    # Set next_run_at to now (watcher picks up on next poll)

# Results
GET    /api/results                  # Query results (filters: config_id, status, since)
GET    /api/results/:config_id       # Results for specific probe
GET    /api/results/stats            # Aggregate stats for dashboard

# Notifications
GET    /api/notification-channels
POST   /api/notification-channels
PUT    /api/notification-channels/:id
DELETE /api/notification-channels/:id
POST   /api/notification-channels/:id/test

# System
GET    /api/health                   # Web backend health
GET    /api/status                   # Overall system status (watcher health, recent failures)

# Push API (used by watchers and external systems)
POST   /api/push/register            # Watcher registers itself and its probe types
POST   /api/push/heartbeat           # Watcher heartbeat
POST   /api/push/result              # Watcher submits probe result
POST   /api/push/alert               # External system posts an alert (creates result)
GET    /api/push/configs/:watcher    # Watcher fetches its assigned configs
```

### 6. Push API Details

**Watcher registration** (`POST /api/push/register`):
```json
{
  "name": "nas",
  "version": "1.0.0",
  "probe_types": [
    {
      "name": "disk-space",
      "version": "1.0.0",
      "description": "Check disk space",
      "arguments": {...},
      "executable_path": "/app/probes/disk-space"
    }
  ]
}
```

**Result submission** (`POST /api/push/result`):
```json
{
  "watcher": "nas",
  "probe_config_id": 123,
  "status": "ok",
  "message": "532 GB free",
  "metrics": {...},
  "data": {...},
  "duration_ms": 42,
  "next_run": "2024-01-19T06:00:00Z",
  "executed_at": "2024-01-18T12:00:00Z"
}
```

**External alert** (`POST /api/push/alert`):
```json
{
  "source": "github-actions",          // creates/uses probe config with this name
  "status": "critical",
  "message": "Build failed: main branch",
  "data": {
    "repo": "user/repo",
    "run_id": 12345,
    "url": "https://github.com/..."
  }
}
```
External alerts auto-create a probe config if one doesn't exist for the source.

### 7. Notification System

**Channels (initial):**
- **Pushover** - iOS/Android push
- **ntfy** - Self-hosted alternative
- **Email** - SMTP

**Triggers:**
- Status change (ok→warning, ok→critical, etc.)
- Recovery (critical→ok, warning→ok)
- External alerts (always notify on critical)
- Future: consecutive failure escalation

## Scheduling

**Interval format:** `<number><unit>` where unit is `m` (minutes), `h` (hours), `d` (days)
- Valid: `1m`, `5m`, `15m`, `30m`, `1h`, `2h`, `6h`, `12h`, `1d`, `7d`

**Next run calculation:**
1. If probe result includes `next_run` timestamp, use that
2. Otherwise, calculate from interval: `last_executed_at + interval`
3. Store in `probe_configs.next_run_at` for the watcher to fetch

**Rerun now:** The UI can set `next_run_at` to the current time (or past), causing the watcher to pick up the probe on its next poll. This replaces the previous approach of calling the watcher's trigger endpoint directly.

**Dynamic scheduling example:**
- Daily backup probe runs every 30 minutes checking if backup completed
- Once backup confirmed, probe returns `next_run: "tomorrow 2am"` (as ISO timestamp)
- Watcher skips this probe until the specified time

**Missed run tracking:** if watcher starts and finds scheduled runs that didn't happen, it logs them to `missed_runs` table.

## Deployment

**Central services (NAS):**
```yaml
services:
  postgres:
    image: postgres:16
    volumes:
      - ./data/postgres:/var/lib/postgresql/data

  web:
    image: monitor:latest
    command: web
    ports:
      - "8080:8080"
    environment:
      - DATABASE_URL=postgres://...
      - AUTH_TOKEN=...

  watcher-nas:
    image: monitor:latest
    command: watcher --name nas
    environment:
      - PUSH_URL=http://web:8080
      - AUTH_TOKEN=...
```

**Remote watcher (Mac):**
```bash
# Run as launchd service or manually
monitor watcher --name macbook \
  --push-url https://monitor.example.com \
  --auth-token $TOKEN \
  --probes-dir ~/probes
```

## Authentication

Token-based auth:
- Shared token for watchers and web UI (via `Authorization: Bearer <token>`)
- Future: multiple tokens with visibility filters (e.g., hide personal probes for demo)

## Grouping and Filtering

**Group path:** hierarchical category, e.g., "Backups", "Backups/Photos"
- Use path semantics for future nesting: "Backups/Photos" is under "Backups"
- UI can filter by group or show group tree

**Keywords:** free-form tags, e.g., ["personal", "nas", "critical"]
- UI can filter by one or more keywords
- Useful for cross-cutting concerns (all "personal" probes across groups)

## Future Considerations

- **Escalation rules**: "alert after N consecutive failures"
- **Token scopes**: filter visible probes by token (for demos, focused views)
- **macOS menu bar app**: native notifications
- **iOS app**: React Native, share components
- **Data retention**: pruning/aggregation policies
- **Trend alerts**: "alert if metric trending toward threshold"

## Migration from Current Implementation

The current implementation has:
- Single watcher with direct DB access
- probe_types without version or watcher association
- probe_configs without watcher_id, group, keywords, next_run
- watcher_heartbeat table (will become watchers table)

**Migration steps:**
1. Add new database migration for schema changes
2. Update web service with push API endpoints
3. Update watcher to use push API instead of direct DB
4. Add watcher --name flag and registration
5. Update probe type registration to include version
6. Add group/keywords to probe config forms
7. Update probe result parsing for next_run
8. Update UI for watcher selection and filtering

## Implementation Order (from current state)

0. **Store plan in repo**: Copy this plan to `docs/PLAN.md` for tracking
1. **Database migration**: new schema with watchers, watcher_probe_types, updated columns
2. **Push API**: `/api/push/*` endpoints in web service
3. **Watcher refactor**: use HTTP client instead of DB, add --name flag
4. **Probe type versioning**: update discovery to track name+version
5. **Group/keywords**: add to probe_configs, update API and UI
6. **next_run support**: parse from probe output, store in config
7. **External alerts**: webhook endpoint that creates results
8. **UI updates**: watcher selection, group/keyword filtering

## Verification

**Multi-watcher setup:**
1. Start web service and postgres
2. Start watcher with `--name nas`, verify it registers via push API
3. Start second watcher with `--name macbook`
4. Verify both watchers appear in `/api/watchers`
5. Create probe config assigned to "nas" watcher
6. Verify probe runs and result appears

**Dynamic scheduling:**
1. Create probe with 1-minute interval
2. Modify probe to return `next_run` 5 minutes in future
3. Verify probe doesn't run again until specified time

**External alerts:**
1. POST to `/api/push/alert` with test data
2. Verify result created and notification triggered
