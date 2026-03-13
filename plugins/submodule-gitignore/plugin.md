+++
name = "submodule-gitignore"
description = "Inject Gas Town gitignore entries into rig project repos to prevent GT operational files from appearing as untracked"
version = 1

[gate]
type = "cooldown"
duration = "12h"

[tracking]
labels = ["plugin:submodule-gitignore", "category:git-hygiene"]
digest = true

[execution]
type = "script"
timeout = "5m"
notify_on_failure = true
severity = "low"
+++

# Submodule Gitignore

Scans all rig project repos and ensures Gas Town operational files are listed
in each repo's `.gitignore`. Prevents GT-created files (`.claude/`, `state.json`,
`.beads/` runtime artifacts, etc.) from appearing as untracked changes in
project repositories.

Idempotent: uses a guard marker comment to detect whether the block has already
been injected. Skips repos that already have the guard.

## Run

```bash
cd /Users/jeremy/gt/plugins/submodule-gitignore && bash run.sh
```
