# Monitor

Personal infrastructure monitoring system with self-describing probes and multi-watcher support.

## Quick Start

```bash
# Start all services
docker compose up -d

# View logs
docker compose logs -f watcher

# Access web UI
open http://localhost:8080
```

Default auth token: `changeme` (set `AUTH_TOKEN` env var to change).

## Architecture

A central **web service** stores configuration and results in PostgreSQL. One or more **watchers** run on different machines, executing probes and pushing results via HTTP.

```
┌──────────────┐     HTTP      ┌──────────────┐
│   Watcher    │──────────────▶│  Web Service │◀── Browser (SPA)
│  (nas, mac)  │  push results │              │
│              │◀──────────────│   PostgreSQL │
│   Probes:    │  fetch config │              │
│  disk-space  │               └──────────────┘
│  command     │
└──────────────┘
```

Watchers have no direct database access. See [docs/PLAN.md](docs/PLAN.md) for full architecture details.

## Probes

Probes are self-describing executables. The watcher discovers them on startup via `--describe`:

```bash
$ ./probes/disk-space/disk-space --describe
{
  "name": "disk-space",
  "version": "1.0.0",
  "description": "Check available disk space on a path",
  "arguments": {
    "required": { "path": {"type": "string", "description": "Path to check"} },
    "optional": { "min_free_gb": {"type": "number", "default": 10} }
  }
}
```

When executed, probes return JSON with status, message, and optional metrics:

```bash
$ ./probes/disk-space/disk-space --path / --min_free_gb 10
{
  "status": "ok",
  "message": "87 GB free on / (89%)",
  "metrics": {"free_bytes": 93841408000, "free_percent": 89.4}
}
```

**Available probes:** disk-space, command

**Adding a probe:** Create an executable in `probes/<name>/` that implements `--describe` and returns JSON results. Restart the watcher to discover it.

## Running a Remote Watcher

```bash
./monitor watcher \
  --name macbook \
  --push-url https://monitor.example.com \
  --auth-token $TOKEN \
  --probes-dir ./probes
```

## API

All endpoints require `Authorization: Bearer <token>` header.

| Endpoint | Description |
|----------|-------------|
| `GET /api/status` | System health overview |
| `GET /api/watchers` | List registered watchers |
| `GET /api/probe-configs` | List probe configurations |
| `POST /api/probe-configs` | Create probe configuration |
| `GET /api/results?config_id=N` | Query probe results |
| `POST /api/push/alert` | External alert webhook |

See [docs/PLAN.md](docs/PLAN.md) for complete API reference.

## Development

```bash
# Run locally (requires Go 1.24+)
go run ./cmd/monitor web --database-url postgres://...
go run ./cmd/monitor watcher --name dev --push-url http://localhost:8080

# Build probes
cd probes/disk-space && go build -o disk-space .
```

## Project Structure

```
cmd/monitor/         CLI entry point (web, watcher, migrate commands)
internal/
  web/               Web service (handlers, push API, server)
  watcher/           Watcher (scheduler, executor, HTTP client)
  db/                Database connection and migrations
  notify/            Notification dispatcher
  probe/             Probe types and result structures
probes/              Probe executables (disk-space, command)
web/frontend/        React SPA (TypeScript, Tailwind)
docs/PLAN.md         Full architecture specification
```
