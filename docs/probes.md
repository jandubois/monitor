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
  "default_interval": "5m",
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
  },
  "output": {
    "metrics": {
      "response_time_ms": {"type": "integer", "unit": "milliseconds", "description": "Response time"}
    },
    "data": {
      "target": {"type": "string", "description": "Checked target"}
    }
  }
}
```

#### Description Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Unique probe identifier (lowercase, hyphens allowed) |
| `description` | Yes | Human-readable summary |
| `version` | Yes | Semantic version (e.g., "1.0.0"). **Increment when `output` schema changes.** |
| `arguments` | Yes | Object with `required` and/or `optional` maps |
| `output` | No | Schema documenting expected metrics and data fields |
| `default_interval` | No | Suggested run frequency (e.g., "1h", "5m", "1d") |
| `subcommand` | No | If set, executor runs: `binary <subcommand> --args` |

#### Argument Specification

Each argument has:

| Field | Required | Description |
|-------|----------|-------------|
| `type` | Yes | One of: `string`, `number`, `boolean` |
| `description` | Yes | Human-readable explanation |
| `default` | No | Default value (omit for required arguments) |
| `enum` | No | Array of allowed values |

#### Output Schema

Document expected output fields so the frontend can display labels, units, and tooltips:

```json
{
  "output": {
    "metrics": {
      "free_bytes": {
        "type": "integer",
        "unit": "bytes",
        "description": "Available space"
      },
      "free_percent": {
        "type": "number",
        "unit": "percent",
        "description": "Available percentage"
      }
    },
    "data": {
      "path": {
        "type": "string",
        "description": "Checked path"
      }
    }
  }
}
```

**Metric fields:**

| Field | Required | Description |
|-------|----------|-------------|
| `type` | Yes | `"number"` or `"integer"` |
| `unit` | No | Unit for formatting: `bytes`, `percent`, `hours`, `milliseconds`, `count` |
| `description` | No | Human-readable label |

**Data fields:**

| Field | Required | Description |
|-------|----------|-------------|
| `type` | Yes | `"string"`, `"number"`, `"boolean"`, `"array"`, or `"object"` |
| `description` | No | Human-readable label |

#### Versioning

**Increment the probe version when the output schema changes.** The system treats each (name, version) combination as a distinct probe type. If you change the schema without updating the version:

- Different watchers may register conflicting schemas
- Historical results may display incorrect labels
- UI formatting may be inconsistent

Bump the version for:
- Adding, removing, or renaming metrics or data fields
- Changing a metric's unit or type
- Significant changes to result format

Minor documentation updates (improving descriptions) don't require a version bump.

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
  "summary": "202.6 GB free (89%)",
  "message": "202.6 GB free on / (89.4%)",
  "metrics": {
    "free_bytes": 217538662400,
    "free_percent": 89.4
  },
  "data": {
    "path": "/"
  }
}
```

#### Result Fields

| Field | Required | Description |
|-------|----------|-------------|
| `status` | Yes | One of: `ok`, `warning`, `critical`, `unknown` |
| `summary` | No | One-line text for collapsed view; defaults to first line of `message` |
| `message` | Yes | Full status text for expanded view |
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
    Name            string       `json:"name"`
    Description     string       `json:"description"`
    Version         string       `json:"version"`
    Arguments       Arguments    `json:"arguments"`
    Output          OutputSchema `json:"output,omitempty"`
    DefaultInterval string       `json:"default_interval,omitempty"`
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

type OutputSchema struct {
    Metrics map[string]MetricSpec `json:"metrics,omitempty"`
    Data    map[string]DataSpec   `json:"data,omitempty"`
}

type MetricSpec struct {
    Type        string `json:"type"`
    Unit        string `json:"unit,omitempty"`
    Description string `json:"description,omitempty"`
}

type DataSpec struct {
    Type        string `json:"type"`
    Description string `json:"description,omitempty"`
}

type Result struct {
    Status  string         `json:"status"`
    Summary string         `json:"summary,omitempty"`
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
  "default_interval": "5m",
  "arguments": {
    "required": {
      "url": {"type": "string", "description": "URL to check"}
    },
    "optional": {
      "timeout": {"type": "number", "description": "Timeout in seconds", "default": 10}
    }
  },
  "output": {
    "data": {
      "url": {"type": "string", "description": "Checked URL"}
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
if curl --silent --fail --max-time "$timeout" "$url" >/dev/null; then
    jq --null-input --arg url "$url" \
        '{status: "ok", summary: "reachable", message: "URL is reachable", data: {url: $url}}'
else
    jq --null-input --arg url "$url" \
        '{status: "critical", summary: "unreachable", message: "URL is unreachable", data: {url: $url}}'
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
