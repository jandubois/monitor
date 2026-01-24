# Monitor Architecture

Personal infrastructure monitoring system with self-describing probes and multi-watcher support.

## Overview

```
                                    ┌─────────────────────────────────────┐
                                    │         Web Service (central)       │
                                    │                                     │
  ┌─────────────────────┐           │  - REST API for SPA                 │
  │       SQLite        │◄──────────│  - Push API for watchers            │
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
│   Probes:           │   results    │   Probes:           │    │  Custom scripts  │
│   - disk-space      │              │   - disk-space      │    │                  │
│   - rd-releases     │              │   - git-status      │    │                  │
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

**Design principles:**
- Web service is the central hub; all writes go through its API
- Watchers push results via HTTP (no direct database access)
- External systems push alerts directly via webhook
- Multiple watchers supported, each with its own probes
- Single Go binary with `watcher` and `web` modes

## Components

### Go Binary (`monitor`)

Single binary with two operational modes:

**`monitor watcher --name <name>`** — Background scheduler service
- Registers with web service on startup
- Discovers local probes and registers their types
- Fetches probe configs assigned to this watcher
- Schedules and executes probes as subprocesses
- Pushes results to web service via HTTP
- Sends periodic heartbeat
- Local HTTP API for control:
  - `GET /health` — Liveness check (public)
  - `POST /reload` — Reload configs (requires auth)
  - `POST /trigger/{id}` — Trigger probe run (requires auth)
  - `POST /discover` — Re-discover probes (requires auth)

**`monitor web`** — Central web service
- REST API for SPA (CRUD operations)
- Push API for watchers (results, heartbeats, registration)
- Push API for external alerts (webhooks)
- Notification dispatcher (triggers on status changes)
- Serves embedded React static files

### Database (SQLite)

The system uses SQLite with WAL mode for concurrent reads.

```sql
-- Registered watchers
watchers (
    id INTEGER PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    token TEXT,                        -- per-watcher auth token
    approved INTEGER DEFAULT 0,        -- must be approved before active
    last_seen_at TEXT,
    version TEXT,
    callback_url TEXT,
    paused INTEGER DEFAULT 0,
    registered_at TEXT
)

-- Probe types (discovered via --describe)
probe_types (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    version TEXT NOT NULL,
    description TEXT,
    arguments TEXT,                    -- JSON
    registered_at TEXT,
    updated_at TEXT,
    UNIQUE(name, version)
)

-- Probe types available on each watcher
watcher_probe_types (
    watcher_id INTEGER REFERENCES watchers(id),
    probe_type_id INTEGER REFERENCES probe_types(id),
    executable_path TEXT NOT NULL,
    subcommand TEXT,
    PRIMARY KEY (watcher_id, probe_type_id)
)

-- Configured probe instances
probe_configs (
    id INTEGER PRIMARY KEY,
    probe_type_id INTEGER REFERENCES probe_types(id),
    watcher_id INTEGER REFERENCES watchers(id),
    name TEXT NOT NULL,
    enabled INTEGER DEFAULT 1,
    arguments TEXT,                    -- JSON
    interval TEXT NOT NULL,            -- '1m', '5m', '1h', '1d'
    timeout_seconds INTEGER DEFAULT 60,
    next_run_at TEXT,
    group_path TEXT,
    keywords TEXT,                     -- JSON array
    notification_channels TEXT,        -- JSON array of IDs
    created_at TEXT,
    updated_at TEXT
)

-- Probe results
probe_results (
    id INTEGER PRIMARY KEY,
    probe_config_id INTEGER REFERENCES probe_configs(id),
    watcher_id INTEGER REFERENCES watchers(id),
    status TEXT NOT NULL,              -- 'ok', 'warning', 'critical', 'unknown'
    message TEXT,
    metrics TEXT,                      -- JSON
    data TEXT,                         -- JSON
    duration_ms INTEGER,
    next_run_at TEXT,
    scheduled_at TEXT,
    executed_at TEXT,
    recorded_at TEXT
)

-- Notification channels
notification_channels (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL,                -- 'pushover', 'ntfy', 'email'
    config TEXT,                       -- JSON
    enabled INTEGER DEFAULT 1
)
```

### Probes

Self-describing executables run as subprocesses. See [probes.md](probes.md) for the SDK and [probe-reference.md](probe-reference.md) for available probes.

**Self-description:**
```bash
$ disk-space --describe
{
  "name": "disk-space",
  "version": "1.0.0",
  "description": "Check available disk space",
  "arguments": {
    "required": { "path": {"type": "string"} },
    "optional": { "min_free_gb": {"type": "number", "default": 10} }
  }
}
```

**Execution:**

Arguments are passed as command-line flags and environment variables:

```bash
# Command line
disk-space --path /volume1 --min_free_gb 100

# Environment variables (also available)
PROBE_PATH=/volume1 PROBE_MIN_FREE_GB=100 disk-space
```

**Result format:**
```json
{
  "status": "ok",
  "message": "532 GB free on /volume1 (53%)",
  "metrics": {"free_bytes": 571230126080, "free_percent": 53.0},
  "data": {"filesystem": "ext4"},
  "next_run": "2024-01-19T06:00:00Z"
}
```

**Status values:**
- `ok` — Normal operation
- `warning` — Attention needed
- `critical` — Immediate action required
- `unknown` — Could not determine status

**Available probes:** disk-space, command, git-status, github, rd-releases, debug

### Web Frontend

TypeScript/React SPA served by the Go backend.

**Stack:** React 18, TypeScript, React Query, Recharts, Tailwind CSS

**Features:**
- Dashboard with all probe statuses
- Detail view with history and metrics charts
- Configuration UI using self-described arguments
- Notification channel management
- Watcher health monitoring

## Authentication

The system uses separate tokens for users and watchers:

**User authentication** (`AUTH_TOKEN` environment variable)
- Single shared token for web UI and API access
- Required for all `/api/*` endpoints except `/api/health`
- Passed via `Authorization: Bearer <token>` header

**Watcher authentication** (per-watcher tokens)
- Each watcher generates a unique token on first run
- Stored in `~/.config/monitor/<name>.token`
- Token sent during registration and subsequent requests
- New watchers require approval before becoming active

**Watcher approval flow:**
1. Watcher starts and registers with web service
2. Web service creates watcher record with `approved = 0`
3. Admin approves watcher via UI or API
4. Watcher can now fetch configs and submit results

## API Reference

### User API

All endpoints require `Authorization: Bearer <token>` (user token).

```
GET    /api/health                    # Health check (no auth)
GET    /api/status                    # System overview

GET    /api/watchers                  # List watchers
GET    /api/watchers/{id}             # Get watcher
DELETE /api/watchers/{id}             # Delete watcher
PUT    /api/watchers/{id}/paused      # Pause/unpause (also approves)

GET    /api/probe-types               # List all probe types
GET    /api/probe-types?watcher={id}  # List types for watcher
POST   /api/probe-types/discover      # Trigger discovery

GET    /api/probe-configs             # List configs (?group=, ?keywords=, ?watcher=)
POST   /api/probe-configs             # Create config
GET    /api/probe-configs/{id}        # Get config
PUT    /api/probe-configs/{id}        # Update config
DELETE /api/probe-configs/{id}        # Delete config
POST   /api/probe-configs/{id}/run    # Trigger run
PUT    /api/probe-configs/{id}/enabled # Enable/disable

GET    /api/results                   # Query results (?config_id=, ?status=, ?since=)
GET    /api/results/{config_id}       # Results for config
GET    /api/results/stats             # Aggregate stats

GET    /api/notification-channels
POST   /api/notification-channels
PUT    /api/notification-channels/{id}
DELETE /api/notification-channels/{id}
POST   /api/notification-channels/{id}/test
```

### Push API (Watchers)

Watcher endpoints use per-watcher token authentication.

**Registration** (`POST /api/push/register`) — No auth required
```json
{
  "name": "nas",
  "version": "1.0.0",
  "token": "watcher-generated-token",
  "callback_url": "http://nas.local:8081",
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

**Heartbeat** (`POST /api/push/heartbeat`) — Watcher token required

**Result submission** (`POST /api/push/result`) — Watcher token required
```json
{
  "probe_config_id": 123,
  "status": "ok",
  "message": "532 GB free",
  "metrics": {...},
  "data": {...},
  "duration_ms": 42,
  "executed_at": "2024-01-18T12:00:00Z"
}
```

**Fetch configs** (`GET /api/push/configs/{watcher}`) — Watcher token required

**External alert** (`POST /api/push/alert`) — Watcher token required
```json
{
  "source": "github-actions",
  "status": "critical",
  "message": "Build failed: main branch",
  "data": {"repo": "user/repo", "url": "https://..."}
}
```

## Scheduling

**Interval format:** `<number><unit>` — m (minutes), h (hours), d (days)
- Examples: `1m`, `5m`, `15m`, `30m`, `1h`, `2h`, `6h`, `12h`, `1d`, `7d`

**Next run calculation:**
1. If probe returns `next_run` timestamp, use that
2. Otherwise: `last_executed_at + interval`

**Dynamic scheduling:** A probe can return `next_run` to override the interval. For example, a backup probe might check every 30 minutes until backup completes, then return `next_run` for tomorrow.

## Notifications

**Supported channels:** Pushover, ntfy, email (SMTP)

**Triggers:**
- Status change (ok→warning, ok→critical, etc.)
- Recovery (critical→ok, warning→ok)
- External alerts (always notify on critical)

## Deployment

**Docker Compose (recommended):**

```yaml
services:
  web:
    image: monitor:latest
    command: ["web", "--database-path", "/data/monitor.db"]
    ports:
      - "8080:8080"
    environment:
      AUTH_TOKEN: ${AUTH_TOKEN}
    volumes:
      - ./data:/data

  watcher:
    image: monitor:latest
    command: ["watcher", "--name", "default", "--push-url", "http://web:8080", "--probes-dir", "/app/probes"]
    volumes:
      - ${HOME}:/host-home:ro
    depends_on:
      - web
```

**Remote watcher (macOS):**

```bash
# Install as LaunchAgent
./monitor install \
  --push-url https://monitor.example.com \
  --auth-token $TOKEN

# Or run manually
./monitor watcher \
  --name macbook \
  --push-url https://monitor.example.com \
  --callback-url http://macbook.local:8081 \
  --probes-dir ./probes
```

The `--callback-url` enables direct probe triggering from the web UI.

## Project Structure

```
cmd/                 CLI commands (web, watcher, install)
internal/
  web/               Web service (handlers, push API)
  watcher/           Watcher (scheduler, executor, client)
  db/                SQLite connection and migrations
  notify/            Notification dispatcher
  probe/             Probe types and result structures
  probes/            Built-in probe implementations
probes/              External probe executables
web/frontend/        React SPA
docs/
  architecture.md    This document
  probes.md          Probe SDK
  probe-reference.md Probe user guide
```
