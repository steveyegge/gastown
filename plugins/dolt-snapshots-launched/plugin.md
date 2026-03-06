+++
name = "dolt-snapshots-launched"
description = "Snapshot Dolt databases when a convoy is launched"
version = 3

[gate]
type = "event"
on = "convoy.launched"

[tracking]
labels = ["plugin:dolt-snapshots", "category:data-safety"]
digest = true

[execution]
timeout = "2m"
notify_on_failure = true
severity = "low"
+++

# Dolt Snapshots — Launched Event

Fires when a convoy is launched (`convoy.launched` event). Creates `staged/` tags
and `convoy/` branches as the launch baseline.

Shares the Go binary from `dolt-snapshots/`. See that plugin for full documentation.

## Step 1: Run snapshot binary

```bash
BINARY_DIR="$(dirname "$0")/../dolt-snapshots"

# Build if binary missing or source is newer
if [ ! -f "$BINARY_DIR/snapshot" ] || [ "$BINARY_DIR/main.go" -nt "$BINARY_DIR/snapshot" ]; then
  echo "Building dolt-snapshots binary..."
  cd "$BINARY_DIR" && go build -o snapshot . 2>&1
  if [ $? -ne 0 ]; then
    echo "FATAL: Go build failed"
    exit 1
  fi
fi

"$BINARY_DIR/snapshot" --routes "$HOME/gt/.beads/routes.jsonl"
SNAPSHOT_EXIT=$?
```

## Step 2: Record result

```bash
RESULT="success"
if [ $SNAPSHOT_EXIT -ne 0 ]; then
  RESULT="failure"
fi

bd create "dolt-snapshots-launched: $RESULT" -t chore --ephemeral \
  -l type:plugin-run,plugin:dolt-snapshots-launched,result:$RESULT \
  -d "dolt-snapshots-launched plugin completed with exit code $SNAPSHOT_EXIT" --silent 2>/dev/null || true
```
