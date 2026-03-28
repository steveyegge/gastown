+++
name = "tool-updater"
description = "Upgrade beads (bd) and dolt via Homebrew when updates are available"
version = 1

[gate]
type = "cooldown"
duration = "168h"

[tracking]
labels = ["plugin:tool-updater", "category:maintenance"]
digest = true

[execution]
timeout = "10m"
notify_on_failure = true
severity = "medium"
+++

# Tool Updater

Checks for and applies Homebrew updates to `beads` (bd) and `dolt`.

gt is rebuilt separately by the `rebuild-gt` plugin (it builds from source, not Homebrew).

## Run

```bash
cd /Users/jeremy/gt/plugins/tool-updater && bash run.sh
```
