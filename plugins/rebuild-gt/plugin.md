+++
name = "rebuild-gt"
description = "Rebuild stale gt binary from gastown source"
version = 1

[gate]
type = "cooldown"
duration = "1h"

[tracking]
labels = ["plugin:rebuild-gt", "rig:gastown", "category:maintenance"]
digest = true

[execution]
timeout = "5m"
notify_on_failure = true
severity = "medium"
+++

# Rebuild gt Binary

Checks if the gt binary is stale (built from older commit than HEAD) and rebuilds.

## Gate Check

The Deacon evaluates this before dispatch. If gate closed, skip.

## Detection

Check binary staleness:

```bash
gt stale --json
```

If `"stale": false`, record success wisp and exit early.

## Action

Rebuild from source:

```bash
cd ~/gt/gastown/crew/george && make build && make install
```

## Record Result

On success:
```bash
bd wisp create \
  --label type:plugin-run \
  --label plugin:rebuild-gt \
  --label rig:gastown \
  --label result:success \
  --body "Rebuilt gt: $OLD → $NEW ($N commits)"
```

On failure:
```bash
bd wisp create \
  --label type:plugin-run \
  --label plugin:rebuild-gt \
  --label rig:gastown \
  --label result:failure \
  --body "Build failed: $ERROR"

gt escalate --severity=medium \
  --subject="Plugin FAILED: rebuild-gt" \
  --body="$ERROR" \
  --source="plugin:rebuild-gt"
```
