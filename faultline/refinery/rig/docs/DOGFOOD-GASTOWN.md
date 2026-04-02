# Dogfooding: gastown Integration (Wave 1)

Gas Town's own orchestration engine is the first Go-based dogfood target. gastown
is a monorepo (`github.com/steveyegge/gastown`) with three crew workspaces (amp,
beercan, imperator) all running the same codebase.

## Architecture

```
gastown (gtfaultline SDK → sentry-go)
    │
    ▼ POST /api/2/envelope/
faultline server
    │
    ├── fingerprint + group → Dolt
    ├── threshold check (3 events/5min, or fatal=1)
    │
    ▼ bd new --rig gastown
Gas Town (gt- beads)
    │
    ▼ polecat dispatched
autonomous fix → merge queue
```

## Faultline Server Configuration

```bash
# Add gastown as project 2, routed to the gastown rig
export FAULTLINE_PROJECTS="1:default_key:faultline,2:gastown_key:gastown"

# API URL for diagnostic links in beads
export FAULTLINE_API_URL="http://localhost:8080"
```

- Project ID `2`: gastown
- Public key: `gastown_key` (replace with generated key for production)
- Target rig: `gastown` (Gas Town rig with gt- prefix)

## DSN Discovery

```bash
curl http://localhost:8080/api/2/dsn/
```

Response:
```json
{
  "project_id": 2,
  "public_key": "gastown_key",
  "rig": "gastown",
  "dsn": "http://gastown_key@localhost:8080/2",
  "endpoints": {
    "envelope": "http://localhost:8080/api/2/envelope/",
    "store": "http://localhost:8080/api/2/store/"
  }
}
```

## gastown SDK Setup

gastown is Go, so it uses the `gtfaultline` package (thin sentry-go wrapper).

### 1. Add dependency

```bash
go get github.com/outdoorsea/faultline/pkg/gtfaultline
```

### 2. Initialize at startup

In `cmd/gt/main.go` or the command execution root:

```go
import "github.com/outdoorsea/faultline/pkg/gtfaultline"

func main() {
    // Initialize faultline error reporting.
    if err := gtfaultline.Init(gtfaultline.Config{
        DSN:         os.Getenv("FAULTLINE_DSN"),  // "http://gastown_key@localhost:8080/2"
        Release:     version,
        Environment: envOr("GT_ENV", "development"),
    }); err != nil {
        // Non-fatal: log and continue without error reporting.
        slog.Warn("faultline init failed", "err", err)
    }
    defer gtfaultline.Flush(2 * time.Second)
    defer gtfaultline.RecoverAndReport()

    os.Exit(cmd.Execute())
}
```

### 3. Hook into townlog

gastown uses a custom `townlog` logger (JSONL file-based). Wrap the error path
to forward error-level events:

```go
// In townlog or at the call sites that log errors:
gtfaultline.CaptureError(err)
```

### 4. Key integration points

| Location | Integration |
|----------|-------------|
| `cmd/gt/main.go` | `Init()`, `Flush()`, `RecoverAndReport()` |
| `internal/cmd/*.go` | `CaptureError()` on command failures |
| `internal/townlog/logger.go` | Forward error-level events |
| Witness crash handling | `CaptureError()` on polecat crashes |
| Deacon patrol failures | `CaptureError()` on patrol errors |

### 5. Environment variable

Set on each gastown crew workspace:

```bash
export FAULTLINE_DSN="http://gastown_key@localhost:8080/2"
```

## What Gets Captured

With the integration above, faultline captures:

- **Panics**: Via `RecoverAndReport()` in main
- **Command failures**: Errors from `cmd.Execute()` and subcommands
- **Townlog errors**: Error-level lifecycle events (crashes, session deaths, escalations)
- **Witness failures**: Polecat crash detection, patrol failures
- **Deacon errors**: Background patrol and maintenance failures

## Bead Labels

Beads created from gastown events include:

| Label | Meaning |
|-------|---------|
| `platform:go` | Go SDK event |
| `error:unhandled` | New error pattern detected |
| `error:regression` | Previously resolved error reappeared |
| `critical` | Fatal-level error (panics) |

## Validation Checklist

1. **SDK initialization**: `gtfaultline.Init()` succeeds at startup
2. **Error capture**: Deliberate test error appears in faultline dashboard
3. **Error-to-bead pipeline**: Events → fingerprint → threshold → `bd new --rig gastown`
4. **Agentic loop**: Polecat picks up gt- bead → diagnoses → fixes → merge queue
5. **Regression detection**: Fix merged → new event within 24h → regression bead filed

## Monitoring

```bash
# Check events from gastown
curl http://localhost:8080/api/2/issues/

# Check beads filed
bd list --rig gastown --label error:unhandled

# Dashboard
open http://localhost:8080/dashboard/projects/2/issues
```

## gt doctor Integration

Add faultline health checks to `gt doctor`:

| Check | What it verifies |
|-------|-----------------|
| `faultline_dsn_set` | `FAULTLINE_DSN` env var is configured |
| `faultline_reachable` | Faultline server responds to health check |
| `faultline_sdk_init` | SDK initialized without error |

These checks should be in the "Infrastructure" category of gt doctor.
