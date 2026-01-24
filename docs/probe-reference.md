# Probe Reference

This guide describes the available probes and their configuration options.

## disk-space

Check available disk space on a filesystem path.

**Use cases:**
- Monitor server disk usage
- Alert before a volume fills up
- Track storage trends over time

### Parameters

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `path` | Yes | — | Filesystem path to check |
| `min_free_gb` | No | 10 | Alert if free space falls below this (GB) |
| `min_free_percent` | No | 0 | Alert if free percentage falls below this |

The probe returns `critical` if either threshold is breached.

### Example

```json
{
  "path": "/volume1",
  "min_free_gb": 100,
  "min_free_percent": 10
}
```

### Metrics

- `free_bytes` — Available space in bytes
- `total_bytes` — Total filesystem size
- `free_percent` — Available space as percentage

---

## command

Run a shell command and check its exit code.

**Use cases:**
- Verify a service is running (`pgrep nginx`)
- Check a backup completed (`test -f /backup/latest.tar.gz`)
- Run custom health checks

### Parameters

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `command` | Yes | — | Shell command to execute |
| `shell` | No | `/bin/sh` | Shell interpreter |
| `ok_codes` | No | `"0"` | Exit codes that indicate success (comma-separated) |
| `warning_codes` | No | `""` | Exit codes that indicate warning (comma-separated) |
| `capture_output` | No | `true` | Include stdout/stderr in result data |

Exit codes not in `ok_codes` or `warning_codes` produce `critical` status.

### Example

```json
{
  "command": "pgrep -x nginx",
  "ok_codes": "0",
  "warning_codes": "",
  "capture_output": false
}
```

### Metrics

- `exit_code` — Command's exit code
- `duration_ms` — Execution time in milliseconds

---

## git-status

Scan a directory for Git repositories with uncommitted changes or unpushed commits.

**Use cases:**
- Remind yourself to commit work-in-progress
- Ensure changes are pushed before leaving a machine
- Monitor developer workstations

The probe recursively searches for `.git` directories and checks each repository's status.

### Parameters

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `path` | Yes | — | Directory to scan for repositories |
| `uncommitted_hours` | No | 1 | Hours before uncommitted changes trigger alert |
| `unpushed_hours` | No | 4 | Hours before unpushed commits trigger alert |

The probe returns `critical` if any repository exceeds these thresholds.

### Example

```json
{
  "path": "/Users/jan/git",
  "uncommitted_hours": 2,
  "unpushed_hours": 8
}
```

### Metrics

- `repos_scanned` — Number of repositories found
- `repos_dirty` — Repositories with uncommitted changes
- `repos_unpushed` — Repositories with unpushed commits

---

## github

Check GitHub repository activity (commits, file changes).

**Use cases:**
- Verify automated commits are happening (CI/CD pipelines)
- Monitor repository activity
- Alert if a scheduled job stops producing commits

Requires a GitHub token via `GH_TOKEN` or `GITHUB_TOKEN` environment variable.

### Parameters

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `repo` | Yes | — | Repository in `owner/name` format |
| `branch` | No | `main` | Branch to check |
| `max_age_hours` | No | 24 | Alert if latest commit is older than this (0 to disable) |
| `min_files` | No | 0 | Alert if latest commit changed fewer files (0 to disable) |
| `min_additions` | No | 0 | Alert if latest commit added fewer lines (0 to disable) |

### Example

```json
{
  "repo": "myorg/myrepo",
  "branch": "main",
  "max_age_hours": 48
}
```

### Metrics

- `commit_age_hours` — Hours since the latest commit
- `files_changed` — Files changed in the latest commit
- `additions` — Lines added in the latest commit
- `deletions` — Lines deleted in the latest commit

---

## rd-releases

Check if the latest Rancher Desktop release appears in the update channel.

**Use cases:**
- Monitor release pipeline health
- Alert if a release is stuck (published but not promoted)

The probe compares the latest GitHub release against the Rancher Desktop update channel API.

### Parameters

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `warn_days` | No | 4 | Days before warning if release missing from channel |
| `critical_days` | No | 7 | Days before critical alert |

### Example

```json
{
  "warn_days": 3,
  "critical_days": 5
}
```

### Metrics

- `days_since_release` — Days since the release was published
- `warn_days` — Configured warning threshold
- `critical_days` — Configured critical threshold

### Data

- `latest_version` — Latest GitHub release version
- `channel_version` — Version in the update channel
- `published_at` — Release publication timestamp

---

## debug

Test probe for debugging and development. Simulates various failure modes.

**Use cases:**
- Test alerting and notification pipelines
- Verify timeout handling
- Debug probe execution issues

### Parameters

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `mode` | No | `ok` | Behavior: `ok`, `warning`, `critical`, `timeout`, `crash`, `error` |
| `message` | No | — | Custom message to return |
| `delay_ms` | No | 0 | Milliseconds to wait before responding |

### Example

```json
{
  "mode": "timeout",
  "delay_ms": 120000
}
```

This configuration simulates a probe that hangs for 2 minutes, useful for testing timeout handling.
