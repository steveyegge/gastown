+++
name = "code-quality-scanner"
description = "Scan codebase for refactoring opportunities using golangci-lint and churn analysis"
version = 1

[gate]
type = "cooldown"
duration = "12h"

[tracking]
labels = ["plugin:code-quality-scanner", "category:quality"]
digest = true

[execution]
timeout = "10m"
notify_on_failure = true
severity = "low"
+++

# Code Quality Scanner

Periodically scans the codebase for refactoring opportunities by combining
static analysis (golangci-lint with aggressive linters) and git churn data
(files changed most often). The intersection — high complexity + high churn —
identifies the highest-value refactoring targets.

Creates beads labeled `refactor-opportunity` for actionable items.
Deduplicates against existing beads. Closes beads when code is cleaned up.
