+++
name = "doc-freshness"
description = "Detect stale documentation that has drifted from the code it describes"
version = 1

[gate]
type = "cooldown"
duration = "24h"

[tracking]
labels = ["plugin:doc-freshness", "category:quality"]
digest = true

[execution]
timeout = "5m"
notify_on_failure = true
severity = "low"
+++

# Doc Freshness

Detects documentation that has drifted from the code it describes by tracking
code-doc coupling. When code changes significantly but the docs that reference
it haven't been updated, those docs are flagged as potentially stale.

Detection strategies:
1. Code-doc coupling: .md files that reference .go files/functions
2. CLI help drift: command --help vs docs that describe those commands
3. Dead references: docs mentioning files, functions, or flags that no longer exist

Creates beads labeled `doc-stale` for docs that need updating.
