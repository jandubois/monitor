# Project Rules

## Author

Jan Dubois (GitHub: jandubois)

## Code Quality

All lint errors must be fixed before committing changes to git.

## Testing with Docker Compose

Start the stack:

```bash
docker compose up -d
```

The default auth token is `changeme`. Set `AUTH_TOKEN` in the environment to override.

### API Endpoints

All API requests (except `/api/health` and `/api/push/register`) require the header:

```
Authorization: Bearer <token>
```

Key endpoints:

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/watchers` | List all watchers |
| GET | `/api/watchers/{id}` | Get watcher details |
| PUT | `/api/watchers/{id}/paused` | Set paused state (also approves when unpausing) |
| GET | `/api/probe-types?watcher={id}` | List probe types for a watcher |
| GET | `/api/probe-configs` | List probe configurations |
| POST | `/api/probe-configs` | Create probe configuration |
| POST | `/api/probe-configs/{id}/run` | Trigger probe execution |
| GET | `/api/results?config_id={id}` | Get results for a probe config |

### Common Operations

Approve a new watcher:

```bash
curl -X PUT http://localhost:8080/api/watchers/1/paused \
  -H "Authorization: Bearer changeme" \
  -H "Content-Type: application/json" \
  -d '{"paused": false}'
```

Create a probe configuration:

```bash
curl -X POST http://localhost:8080/api/probe-configs \
  -H "Authorization: Bearer changeme" \
  -H "Content-Type: application/json" \
  -d '{"probe_type_id": 1, "watcher_id": 1, "name": "My Probe", "enabled": true, "arguments": {}, "interval": "1h", "timeout_seconds": 30}'
```

Trigger a probe run:

```bash
curl -X POST http://localhost:8080/api/probe-configs/1/run \
  -H "Authorization: Bearer changeme"
```
