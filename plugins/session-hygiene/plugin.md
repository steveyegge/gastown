+++
name = "session-hygiene"
description = "Clean up zombie tmux sessions and orphaned dog sessions"
version = 1

[gate]
type = "cooldown"
duration = "30m"

[tracking]
labels = ["plugin:session-hygiene", "category:cleanup"]
digest = true

[execution]
timeout = "5m"
notify_on_failure = true
severity = "low"
+++

# Session Hygiene

Identifies and kills zombie tmux sessions (wrong prefix, no registered rig)
and orphaned dog sessions (tmux session exists but dog not in kennel).

## Step 1: Get valid rig prefixes

Fetch the rig registry to know which session prefixes are legitimate:

```bash
RIG_JSON=$(gt rig list --json 2>/dev/null)
if [ $? -ne 0 ] || [ -z "$RIG_JSON" ]; then
  echo "SKIP: could not get rig list"
  exit 0
fi

# Extract rig names as valid prefixes
VALID_PREFIXES=$(echo "$RIG_JSON" | jq -r '.[].name // empty' 2>/dev/null)
if [ -z "$VALID_PREFIXES" ]; then
  echo "SKIP: no rigs found in registry"
  exit 0
fi
```

## Step 2: List tmux sessions

```bash
SESSIONS=$(tmux list-sessions -F '#{session_name}' 2>/dev/null)
if [ $? -ne 0 ] || [ -z "$SESSIONS" ]; then
  echo "No tmux sessions running"
  exit 0
fi

SESSION_COUNT=$(echo "$SESSIONS" | wc -l | tr -d ' ')
```

## Step 3: Identify zombie sessions

A session is legitimate if its prefix matches a known rig or the `hq` namespace.
Gas Town sessions follow the pattern `<prefix>-<role>-<name>` (e.g., `hq-dog-alpha`,
`gastown-witness`, `gastown-polecat-slit`).

```bash
ZOMBIES=()

while IFS= read -r SESSION; do
  [ -z "$SESSION" ] && continue

  # Extract prefix (everything before the first dash)
  PREFIX=$(echo "$SESSION" | cut -d'-' -f1)

  # Allow hq prefix (town-level agents: deacon, dogs, mayor)
  if [ "$PREFIX" = "hq" ]; then
    continue
  fi

  # Check against valid rig prefixes
  VALID=false
  while IFS= read -r RIG; do
    if [ "$PREFIX" = "$RIG" ]; then
      VALID=true
      break
    fi
  done <<< "$VALID_PREFIXES"

  if [ "$VALID" = "false" ]; then
    ZOMBIES+=("$SESSION")
  fi
done <<< "$SESSIONS"
```

## Step 4: Kill zombie sessions

```bash
KILLED=0
for ZOMBIE in "${ZOMBIES[@]}"; do
  echo "Killing zombie session: $ZOMBIE"
  tmux kill-session -t "$ZOMBIE" 2>/dev/null && KILLED=$((KILLED + 1))
done
```

## Step 5: Check for orphaned dog sessions

Dog sessions follow the pattern `hq-dog-<name>`. Cross-reference against
the kennel to find sessions for dogs that no longer exist:

```bash
DOG_JSON=$(gt dog list --json 2>/dev/null || echo "[]")
KNOWN_DOGS=$(echo "$DOG_JSON" | jq -r '.[].name // empty' 2>/dev/null)

ORPHANED=0
while IFS= read -r SESSION; do
  [ -z "$SESSION" ] && continue

  # Match hq-dog-* pattern
  case "$SESSION" in
    hq-dog-*)
      DOG_NAME="${SESSION#hq-dog-}"

      # Check if this dog exists in the kennel
      FOUND=false
      while IFS= read -r DOG; do
        if [ "$DOG_NAME" = "$DOG" ]; then
          FOUND=true
          break
        fi
      done <<< "$KNOWN_DOGS"

      if [ "$FOUND" = "false" ]; then
        echo "Killing orphaned dog session: $SESSION (dog '$DOG_NAME' not in kennel)"
        tmux kill-session -t "$SESSION" 2>/dev/null && ORPHANED=$((ORPHANED + 1))
      fi
      ;;
  esac
done <<< "$SESSIONS"
```

## Record Result

```bash
SUMMARY="Checked $SESSION_COUNT sessions: $KILLED zombie(s) killed, $ORPHANED orphaned dog session(s) killed, ${#ZOMBIES[@]} zombie(s) found"
echo "$SUMMARY"
```

On success:
```bash
bd create "session-hygiene: $SUMMARY" -t chore --ephemeral \
  -l type:plugin-run,plugin:session-hygiene,result:success \
  -d "$SUMMARY" --silent 2>/dev/null || true
```

On failure:
```bash
bd create "session-hygiene: FAILED" -t chore --ephemeral \
  -l type:plugin-run,plugin:session-hygiene,result:failure \
  -d "Session hygiene failed: $ERROR" --silent 2>/dev/null || true

gt escalate "Plugin FAILED: session-hygiene" \
  --severity low \
  --reason "$ERROR"
```
