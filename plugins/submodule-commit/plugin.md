+++
name = "submodule-commit"
description = "Auto-commit accumulated changes inside git submodules and update parent pointer"
version = 1

[gate]
type = "cooldown"
duration = "2h"

[tracking]
labels = ["plugin:submodule-commit", "category:git-hygiene"]
digest = true

[execution]
timeout = "15m"
notify_on_failure = true
severity = "low"

# Opt-in per rig via plugin frontmatter:
# [plugin.submodule-commit]
# enabled = true
# commit_branch = "main"          # branch to commit on in each submodule
# push_enabled = false            # push submodule commits (false = local only)
# allowlist = []                  # empty = all submodules; ["path/to/sub"] = only those
+++

# Submodule Commit

Auto-commits accumulated changes inside git submodules and updates the parent
repo's submodule pointer. Polecats only operate on parent repo worktrees and
have no commit mandate for submodule repos — this plugin fills that gap.

**Opt-in only.** Rigs must enable this plugin in their `plugin.md` frontmatter.
Current enabled rigs: `lilypad_chat` (3 Bitbucket submodules).

## Step 1: Find opt-in rigs with submodules

```bash
RIG_JSON=$(gt rig list --json 2>/dev/null || true)
if [ -z "$RIG_JSON" ]; then
  echo "SKIP: could not get rig list"
  exit 0
fi

# Find rigs that have .gitmodules
ENABLED_RIGS=()
while IFS= read -r REPO_PATH; do
  [ -z "$REPO_PATH" ] && continue
  [ ! -f "$REPO_PATH/.gitmodules" ] && continue
  # Check rig plugin config for opt-in
  RIG_NAME=$(basename "$REPO_PATH")
  PLUGIN_CONFIG=$(gt rig show "$RIG_NAME" --json 2>/dev/null | jq -r '.plugins["submodule-commit"].enabled // false' 2>/dev/null || echo "false")
  if [ "$PLUGIN_CONFIG" = "true" ]; then
    ENABLED_RIGS+=("$REPO_PATH")
  fi
done < <(echo "$RIG_JSON" | jq -r '.[] | select(.repo_path != null) | .repo_path // empty' 2>/dev/null)

if [ ${#ENABLED_RIGS[@]} -eq 0 ]; then
  echo "SKIP: no opt-in rigs with submodules found"
  exit 0
fi

echo "Processing ${#ENABLED_RIGS[@]} rig(s) with submodules"
```

## Step 2: For each opt-in rig, process its submodules

```bash
TOTAL_COMMITTED=0
TOTAL_PUSHED=0
TOTAL_PARENT_UPDATED=0
ERRORS=""

for REPO_PATH in "${ENABLED_RIGS[@]}"; do
  echo ""
  echo "=== $REPO_PATH ==="

  RIG_NAME=$(basename "$REPO_PATH")

  # Get plugin config
  RIG_CONFIG=$(gt rig show "$RIG_NAME" --json 2>/dev/null | jq -r '.plugins["submodule-commit"] // {}' 2>/dev/null || echo "{}")
  COMMIT_BRANCH=$(echo "$RIG_CONFIG" | jq -r '.commit_branch // "main"')
  PUSH_ENABLED=$(echo "$RIG_CONFIG" | jq -r '.push_enabled // false')
  ALLOWLIST=$(echo "$RIG_CONFIG" | jq -r '.allowlist // [] | .[]' 2>/dev/null || true)

  # Parse .gitmodules for submodule paths
  SUBMODULE_PATHS=$(git -C "$REPO_PATH" config --file .gitmodules --get-regexp 'submodule\..*\.path' 2>/dev/null | awk '{print $2}' || true)

  PARENT_CHANGED=false

  while IFS= read -r SUB_PATH; do
    [ -z "$SUB_PATH" ] && continue

    # Apply allowlist filter if set
    if [ -n "$ALLOWLIST" ]; then
      MATCH=false
      while IFS= read -r ALLOWED; do
        [ "$SUB_PATH" = "$ALLOWED" ] && MATCH=true && break
      done <<< "$ALLOWLIST"
      $MATCH || continue
    fi

    FULL_SUB="$REPO_PATH/$SUB_PATH"
    if [ ! -d "$FULL_SUB/.git" ] && [ ! -f "$FULL_SUB/.git" ]; then
      echo "  SKIP: $SUB_PATH — not initialized"
      continue
    fi

    # Check for uncommitted changes in submodule
    SUB_DIRTY=$(git -C "$FULL_SUB" status --porcelain 2>/dev/null | head -1 || true)
    if [ -z "$SUB_DIRTY" ]; then
      echo "  $SUB_PATH: clean"
      continue
    fi

    SUB_BRANCH=$(git -C "$FULL_SUB" branch --show-current 2>/dev/null || true)
    if [ -z "$SUB_BRANCH" ]; then
      echo "  SKIP: $SUB_PATH — detached HEAD, skipping"
      continue
    fi

    echo "  $SUB_PATH: dirty (branch=$SUB_BRANCH), committing..."

    # Commit changes
    git -C "$FULL_SUB" add -A 2>/dev/null || true
    STAGED=$(git -C "$FULL_SUB" diff --cached --name-only 2>/dev/null | wc -l | tr -d ' ')
    if [ "$STAGED" -gt 0 ]; then
      git -C "$FULL_SUB" commit -m "chore: accumulated changes [skip ci]

Auto-committed by submodule-commit plugin ($STAGED file(s))." \
        --author="Gas Town <gastown@local>" 2>/dev/null && \
        echo "    Committed $STAGED file(s)" && \
        TOTAL_COMMITTED=$((TOTAL_COMMITTED + 1)) || \
        { echo "    WARN: commit failed"; continue; }

      # Push (best effort, || true)
      if [ "$PUSH_ENABLED" = "true" ]; then
        git -C "$FULL_SUB" push origin "$SUB_BRANCH" 2>/dev/null && \
          TOTAL_PUSHED=$((TOTAL_PUSHED + 1)) || \
          echo "    WARN: push failed (local commit preserved)"
      fi

      PARENT_CHANGED=true
    fi
  done <<< "$SUBMODULE_PATHS"

  # Update parent repo submodule pointer if any submodule changed
  if $PARENT_CHANGED; then
    PARENT_BRANCH=$(git -C "$REPO_PATH" branch --show-current 2>/dev/null || true)
    if [ "$PARENT_BRANCH" = "main" ]; then
      PARENT_DIRTY=$(git -C "$REPO_PATH" status --porcelain 2>/dev/null | grep -v "^??" | head -1 || true)
      if [ -z "$PARENT_DIRTY" ]; then
        git -C "$REPO_PATH" add -A -- '*.gitmodules' $(git -C "$REPO_PATH" status --short 2>/dev/null | awk '{print $2}') 2>/dev/null || true
        PARENT_STAGED=$(git -C "$REPO_PATH" diff --cached --name-only 2>/dev/null | head -1 || true)
        if [ -n "$PARENT_STAGED" ]; then
          git -C "$REPO_PATH" commit -m "chore: update submodule pointers [skip ci]

Auto-committed by submodule-commit plugin." \
            --author="Gas Town <gastown@local>" 2>/dev/null && \
            TOTAL_PARENT_UPDATED=$((TOTAL_PARENT_UPDATED + 1)) || true
          git -C "$REPO_PATH" push origin main 2>/dev/null || echo "  WARN: parent push failed (local commit preserved)"
        fi
      else
        echo "  SKIP: parent repo dirty, not updating submodule pointer"
      fi
    else
      echo "  SKIP: parent repo on $PARENT_BRANCH (not main), not updating pointer"
    fi
  fi
done
```

## Record Result

```bash
SUMMARY="submodule-commit: $TOTAL_COMMITTED submodule(s) committed, $TOTAL_PUSHED pushed, $TOTAL_PARENT_UPDATED parent pointer(s) updated"
echo ""
echo "=== Submodule Commit Summary ==="
echo "$SUMMARY"

RESULT="success"
[ -n "$ERRORS" ] && RESULT="warning"

bd create "$SUMMARY" -t chore --ephemeral \
  -l "type:plugin-run,plugin:submodule-commit,result:$RESULT" \
  -d "$SUMMARY" --silent 2>/dev/null || true
```
