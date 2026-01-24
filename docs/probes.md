# Probe SDK

A probe is an executable that checks something and reports its status.

## Quick Start

A probe must:

1. Support `--describe` to output its metadata as JSON
2. Accept arguments as command-line flags (`--name=value`)
3. Output results as JSON to stdout

## Protocol

### Self-Description

When invoked with `--describe`, a probe outputs its metadata as JSON:

```json
{
  "name": "my-probe",
  "description": "Check something important",
  "version": "1.0.0",
  "arguments": {
    "required": {
      "target": {
        "type": "string",
        "description": "Target to check"
      }
    },
    "optional": {
      "timeout": {
        "type": "number",
        "description": "Timeout in seconds",
        "default": 30
      }
    }
  }
}
```

#### Description Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Unique probe identifier (lowercase, hyphens allowed) |
| `description` | Yes | Human-readable summary |
| `version` | Yes | Semantic version (e.g., "1.0.0") |
| `arguments` | Yes | Object with `required` and/or `optional` maps |
| `subcommand` | No | If set, executor runs: `binary <subcommand> --args` |

#### Argument Specification

Each argument has:

| Field | Required | Description |
|-------|----------|-------------|
| `type` | Yes | One of: `string`, `number`, `boolean` |
| `description` | Yes | Human-readable explanation |
| `default` | No | Default value (omit for required arguments) |
| `enum` | No | Array of allowed values |

### Execution

The executor invokes probes as:

```
/path/to/probe --arg1=value1 --arg2=value2
```

Arguments are also available as `PROBE_`-prefixed environment variables:

| Argument | Environment Variable |
|----------|---------------------|
| `my_arg` | `PROBE_MY_ARG` |
| `max-retries` | `PROBE_MAX_RETRIES` |

Convert hyphens to underscores and uppercase the names.

### Result Format

Probes output JSON to stdout:

```json
{
  "status": "ok",
  "message": "All systems operational",
  "metrics": {
    "response_time_ms": 42,
    "items_checked": 100
  },
  "data": {
    "version": "1.2.3",
    "last_updated": "2024-01-15T10:30:00Z"
  }
}
```

#### Result Fields

| Field | Required | Description |
|-------|----------|-------------|
| `status` | Yes | One of: `ok`, `warning`, `critical`, `unknown` |
| `message` | Yes | Human-readable status summary |
| `metrics` | No | Numeric values for graphing/alerting |
| `data` | No | Additional context (strings, objects, etc.) |
| `next_run` | No | ISO 8601 timestamp to override next scheduled run |

#### Status Values

| Status | Meaning |
|--------|---------|
| `ok` | Check passed |
| `warning` | Degraded state, attention needed |
| `critical` | Failure, immediate action required |
| `unknown` | Check could not complete (error, timeout) |

### Error Handling

On failure, return `status: "unknown"` with an error message. Always exit 0; any other exit code signals a broken probe, not a failed check.

```json
{
  "status": "unknown",
  "message": "Failed to connect to database: connection refused"
}
```

## Writing Probes in Go

Use Go's `flag` package to parse arguments and `encoding/json` for output.

### Example

```go
package main

import (
    "encoding/json"
    "flag"
    "os"
)

type Description struct {
    Name        string    `json:"name"`
    Description string    `json:"description"`
    Version     string    `json:"version"`
    Arguments   Arguments `json:"arguments"`
}

type Arguments struct {
    Required map[string]ArgSpec `json:"required,omitempty"`
    Optional map[string]ArgSpec `json:"optional,omitempty"`
}

type ArgSpec struct {
    Type        string `json:"type"`
    Description string `json:"description"`
    Default     any    `json:"default,omitempty"`
}

type Result struct {
    Status  string         `json:"status"`
    Message string         `json:"message"`
    Metrics map[string]any `json:"metrics,omitempty"`
    Data    map[string]any `json:"data,omitempty"`
}

func main() {
    describe := flag.Bool("describe", false, "Print probe description")
    target := flag.String("target", "", "Target to check")
    timeout := flag.Int("timeout", 30, "Timeout in seconds")
    flag.Parse()

    if *describe {
        json.NewEncoder(os.Stdout).Encode(Description{
            Name:        "example",
            Description: "Example probe",
            Version:     "1.0.0",
            Arguments: Arguments{
                Required: map[string]ArgSpec{
                    "target": {Type: "string", Description: "Target to check"},
                },
                Optional: map[string]ArgSpec{
                    "timeout": {Type: "number", Description: "Timeout in seconds", Default: 30},
                },
            },
        })
        return
    }

    if *target == "" {
        json.NewEncoder(os.Stdout).Encode(Result{
            Status:  "unknown",
            Message: "target argument is required",
        })
        return
    }

    // Perform check...
    json.NewEncoder(os.Stdout).Encode(Result{
        Status:  "ok",
        Message: "Check passed",
        Metrics: map[string]any{"duration_ms": 42},
        Data:    map[string]any{"target": *target, "timeout": *timeout},
    })
}
```

### Building

For Docker, cross-compile for Linux:

```bash
CGO_ENABLED=0 GOOS=linux go build -o my-probe .
```

## Writing Probes in Shell

Shell probes access arguments via environment variables (`PROBE_*`). Use `jq` to generate JSON output.

### Example

```bash
#!/bin/bash
set -euo pipefail

if [[ "${1:-}" == "--describe" ]]; then
    cat <<'EOF'
{
  "name": "example",
  "description": "Example shell probe",
  "version": "1.0.0",
  "arguments": {
    "required": {
      "url": {
        "type": "string",
        "description": "URL to check"
      }
    },
    "optional": {
      "timeout": {
        "type": "number",
        "description": "Timeout in seconds",
        "default": 10
      }
    }
  }
}
EOF
    exit 0
fi

url="${PROBE_URL:-}"
timeout="${PROBE_TIMEOUT:-10}"

if [[ -z "$url" ]]; then
    jq --null-input '{status: "unknown", message: "url argument is required"}'
    exit 0
fi

# Perform check
if response=$(curl --silent --fail --max-time "$timeout" "$url"); then
    jq --null-input \
        --arg url "$url" \
        '{status: "ok", message: "URL is reachable", data: {url: $url}}'
else
    jq --null-input \
        --arg url "$url" \
        '{status: "critical", message: "URL is unreachable", data: {url: $url}}'
fi
```

### Dependencies

Shell probes in the Docker image have access to:

- `bash`, `curl`, `jq`
- `git` (for repository checks)
- Standard POSIX utilities

### Tips

- Always `exit 0`, even on check failure
- Use `jq --null-input` to generate JSON (handles escaping)
- Access arguments via `PROBE_*` environment variables
- Provide sensible defaults with `${PROBE_VAR:-default}`

## Deployment

Place probes in the `probes/` directory:

```
probes/
  my-probe/
    my-probe      # executable (same name as directory)
```

The watcher discovers probes on startup by calling `--describe` on each executable.

## Testing

Test your probe locally:

```bash
# Test self-description
./my-probe --describe | jq .

# Test execution
./my-probe --target=example.com | jq .

# Test with environment variables (shell probes)
PROBE_TARGET=example.com ./my-probe | jq .
```
