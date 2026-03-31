+++
name = "faultline-health"
description = "Health-check the faultline error tracking service and restart if down"
version = 1

[gate]
type = "cooldown"
duration = "60s"

[tracking]
labels = ["plugin:faultline-health", "category:health"]
digest = true

[execution]
timeout = "30s"
notify_on_failure = true
severity = "high"
+++

# Faultline Health Check

Monitors the faultline error tracking service (self-managed daemon). Checks
the `/health` endpoint and restarts the service if it's unresponsive. Escalates
after repeated failures.

## Step 1: Check if faultline is expected to be running

If there's no pidfile and no process, faultline may not have been started yet.
Don't treat "never started" as a crash.

```bash
TOWN_ROOT="${GT_TOWN_ROOT:-$HOME/gt}"
RUNTIME_DIR="$TOWN_ROOT/.faultline"
PIDFILE="$RUNTIME_DIR/faultline.pid"
LOGFILE="$RUNTIME_DIR/faultline.log"
FAULTLINE_BIN="$RUNTIME_DIR/faultline"
ADDR="${FAULTLINE_ADDR:-:8080}"
FAILURE_COUNT_FILE="$RUNTIME_DIR/.health-failures"

# Normalize addr for curl
HOST="$ADDR"
if [[ "$HOST" == :* ]]; then
  HOST="localhost$HOST"
fi
HEALTH_URL="http://$HOST/health"

# Check if binary exists
if [ ! -f "$FAULTLINE_BIN" ]; then
  echo "SKIP: faultline binary not found at $FAULTLINE_BIN"
  echo "Build with: cd <faultline-repo> && go build -o $FAULTLINE_BIN ./cmd/faultline"
  exit 0
fi

# Check if pidfile exists (was it ever started?)
if [ ! -f "$PIDFILE" ]; then
  # Try health check anyway — might be running without pidfile
  if curl -sf --max-time 3 "$HEALTH_URL" >/dev/null 2>&1; then
    echo "OK: faultline responding (no pidfile)"
    exit 0
  fi
  echo "SKIP: faultline not started (no pidfile, no response)"
  exit 0
fi
```

## Step 2: Health check

```bash
echo "=== Faultline Health Check ==="
echo "  PID file: $PIDFILE"
echo "  Health URL: $HEALTH_URL"

PID=$(cat "$PIDFILE" 2>/dev/null | tr -d '[:space:]')

# Check if process is alive
if [ -n "$PID" ] && kill -0 "$PID" 2>/dev/null; then
  echo "  Process: alive (PID $PID)"
else
  echo "  Process: dead (PID $PID)"
  PROCESS_DEAD=true
fi

# Hit the health endpoint
HEALTH_RESPONSE=$(curl -sf --max-time 3 "$HEALTH_URL" 2>&1)
HEALTH_EXIT=$?

if [ $HEALTH_EXIT -eq 0 ]; then
  echo "  Health: OK — $HEALTH_RESPONSE"
  # Reset failure counter on success
  rm -f "$FAILURE_COUNT_FILE"
  exit 0
fi

echo "  Health: FAILED (curl exit=$HEALTH_EXIT)"
```

## Step 3: Count consecutive failures

```bash
# Track consecutive failures
FAILURES=1
if [ -f "$FAILURE_COUNT_FILE" ]; then
  PREV=$(cat "$FAILURE_COUNT_FILE" 2>/dev/null | tr -d '[:space:]')
  FAILURES=$(( ${PREV:-0} + 1 ))
fi
echo "$FAILURES" > "$FAILURE_COUNT_FILE"
echo "  Consecutive failures: $FAILURES"
```

## Step 4: Decide action

```bash
if [ "$FAILURES" -ge 5 ]; then
  # Too many failures — escalate, don't keep restarting
  echo "ESCALATE: faultline has failed $FAILURES consecutive health checks"
  gt escalate -s HIGH "Faultline service: $FAILURES consecutive health check failures. Manual intervention needed."
  exit 1
fi

if [ "$FAILURES" -ge 2 ] || [ "$PROCESS_DEAD" = "true" ]; then
  echo "RESTART: attempting to restart faultline"

  # Stop cleanly if process exists
  if [ -n "$PID" ] && kill -0 "$PID" 2>/dev/null; then
    kill "$PID" 2>/dev/null
    sleep 2
    # Force kill if still alive
    kill -0 "$PID" 2>/dev/null && kill -9 "$PID" 2>/dev/null
  fi
  rm -f "$PIDFILE"

  # Restart using the daemon's start command
  "$FAULTLINE_BIN" start
  RESTART_EXIT=$?

  if [ $RESTART_EXIT -eq 0 ]; then
    echo "  Restart: OK"
    # Don't reset failure counter — let the next health check confirm
  else
    echo "  Restart: FAILED (exit=$RESTART_EXIT)"
    gt escalate -s HIGH "Faultline service restart failed (exit=$RESTART_EXIT)"
  fi
else
  echo "  First failure — will retry next cycle before restarting"
fi
```

## Record Result

```bash
if [ "$FAILURES" -ge 2 ]; then
  echo "=== Faultline health check: RESTARTED (failure #$FAILURES) ==="
else
  echo "=== Faultline health check: WARNING (failure #$FAILURES, will retry) ==="
fi
```
