---
name: gas-town-cursor
description: >
  Develop and operate Gas Town with the Cursor agent preset (cursor-agent CLI):
  gt flags, hooks at .cursor/hooks.json, session resume, and how this repo differs from README marketing copy.
---

# Gas Town + Cursor Agent CLI

Use this skill when working **in this repository** with the **`cursor`** agent preset (CLI binary **`cursor-agent`**, sometimes symlinked as **`agent`**).

## Concepts

| Name | Meaning |
|------|---------|
| **Preset `cursor`** | Gas Town agent id (`GT_AGENT=cursor`). Config lives in `internal/config/agents.go` (`AgentCursor`). |
| **Binary `cursor-agent`** | The Cursor Agent CLI process name for pane/detection; install docs may also symlink `agent` → same binary. |
| **Hooks** | Cursor lifecycle hooks are configured at **`.cursor/hooks.json`** (see preset `HooksSettingsFile`). |

## Essential commands

- Build / run `gt` from repo root: `make build` or `go run ./cmd/gt …`.
- Point a session at the Cursor preset: spawn or config so the runtime uses **`--agent cursor`** (or set **`GT_AGENT=cursor`** where applicable).
- After changing hooks or settings: **`gt up --restore`** (or role-specific restart) so agents reload config.

## Resume semantics

The Cursor preset uses **`--resume <chatId>`** style resume (`ResumeStyle: flag`). Session identity is not carried in a dedicated env var in the same way as Claude’s `CLAUDE_SESSION_ID`; follow the preset fields in `internal/config/agents.go` (`ResumeFlag`, `ResumeStyle`).

## Read more

- Beads / plan handoff: [`docs/cursor-runtime-beads-tasks.md`](../../../docs/cursor-runtime-beads-tasks.md)
- Agent instructions for automation: [`AGENTS.md`](../../../AGENTS.md) and [`CLAUDE.md`](../../../CLAUDE.md) (project-wide, not Cursor-only)

## Boundary

Project **README** is user-facing product overview; **`.cursor/README.md`** is Cursor-specific onboarding for this repo. Prefer linking to those files instead of duplicating long install steps inside skills.
