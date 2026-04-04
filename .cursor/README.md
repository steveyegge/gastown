# Cursor / `cursor-agent` in this repo

This directory holds **Cursor-specific** onboarding. For general Gas Town agent instructions, see [`../AGENTS.md`](../AGENTS.md) and [`../CLAUDE.md`](../CLAUDE.md).

## Prerequisites

1. **Build `gt`** from the repo root (`make build` or `go install ./cmd/gt`). Gas Town expects a working `gt` on your `PATH` for hooks and crew workflows.
2. **`bd` (beads)** — issue DB under `.beads/`; see [`../CONTRIBUTING.md`](../CONTRIBUTING.md) for workflow.
3. **Cursor Agent CLI** — install the `cursor-agent` binary per Cursor’s documentation. The Gas Town preset name is **`cursor`**; the process is typically **`cursor-agent`** (an **`agent`** symlink may exist).

## Preset vs binary

- **Preset:** `cursor` (`GT_AGENT=cursor`) — defined in `internal/config/agents.go`.
- **CLI:** `cursor-agent` (args include **`-f`** for auto-approve in autonomous flows).

## Hooks

Hooks are installed under **`.cursor/hooks.json`** when roles are provisioned (`EnsureSettingsForRole`). After template or hook changes, restart agents (e.g. **`gt up --restore`**) so sessions pick up new files.

## Skills

See [`.cursor/skills/gas-town-cursor/SKILL.md`](skills/gas-town-cursor/SKILL.md) for agent-facing workflow (gt, resume, pointers to code).

## Beads / plan tracking

Epic tasks for Cursor runtime parity are tracked in beads; coordination notes and script:

- [`docs/cursor-runtime-beads-tasks.md`](../docs/cursor-runtime-beads-tasks.md)
- [`scripts/cursor-runtime-bd-tasks.sh`](../scripts/cursor-runtime-bd-tasks.sh)

## Automated regression (local)

```bash
./scripts/cursor-runtime-test-gate.sh
```

Covers `internal/config`, `hooks`, `crew`, `tmux`, and `runtime` packages (see script for exact `go test` line).

## Manual smoke (short)

Run these only when changing behavior that tests do not cover end-to-end:

1. `make build` — `gt` binary builds.
2. `gt config agent list` — output includes the **`cursor`** preset.
3. Start or attach to a dev session with **`GT_AGENT=cursor`** (or `--agent cursor`) and confirm the pane command shows **`cursor-agent`** or **`agent`** and the session receives hooks/nudges as expected.

For full §9-style checklists, see the Cursor parity plan document in your planning folder if present; prefer adding **automated** tests in-repo when possible.
