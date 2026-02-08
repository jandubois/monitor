# Probe Reference

This guide describes the available probes and their configuration options.

Each probe provides:
- **Parameters** — Arguments to configure the check
- **Metrics** — Numeric values for graphing and alerting
- **Output schema** — Field descriptions for UI display (via `--describe`)
- **Default interval** — Suggested run frequency

## disk-space

Check available disk space on a filesystem path.

**Default interval:** 1h

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

| Metric | Unit | Description |
|--------|------|-------------|
| `free_bytes` | bytes | Available space |
| `total_bytes` | bytes | Total filesystem size |
| `free_gb` | gigabytes | Available space |
| `free_percent` | percent | Available percentage |

---

## command

**Default interval:** 5m

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

| Metric | Unit | Description |
|--------|------|-------------|
| `exit_code` | — | Command's exit code |
| `duration_ms` | milliseconds | Execution time |

---

## github

**Default interval:** 1h

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

| Metric | Unit | Description |
|--------|------|-------------|
| `age_hours` | hours | Commit age |
| `files_changed` | count | Files changed |
| `additions` | count | Lines added |
| `deletions` | count | Lines deleted |

---

## rd-releases

**Default interval:** 6h

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

| Metric | Unit | Description |
|--------|------|-------------|
| `days_since_release` | count | Days since release |
| `warn_days` | count | Warning threshold |
| `critical_days` | count | Critical threshold |

---

## debug

**Default interval:** 1m

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
