# Agent Instructions

See **CLAUDE.md** for complete agent context and instructions.

This file exists for compatibility with tools that look for AGENTS.md.

> **Recovery**: Run `gt prime` after compaction, clear, or new session

Full context is injected by `gt prime` at session start.

## Robot Mode CLI (Agent-Friendly)

For AI agents and automation, use robot mode for deterministic, parseable output:

- `--robot` (or `--json`) for machine-readable JSON envelopes
- `--robot-help` for stable, token-efficient help text
- Commands with minor syntax issues are accepted when intent is clear; responses include a correction note
- If intent is unclear, robot mode returns a detailed usage error with examples

Reference: `docs/design-robot-mode-cli-v_codex.md`.
