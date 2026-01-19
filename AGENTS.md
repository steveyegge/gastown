# Agent Instructions

This file provides key documentation references for AI agents working with Gastown.

> **Recovery**: Run `gt prime` after compaction, clear, or new session

Full context is injected by `gt prime` at session start.

## Key Documentation

| Topic | Location |
|-------|----------|
| **Gastown Overview** | `docs/overview.md` |
| **API Reference** | `docs/reference.md` |
| **Concepts Glossary** | `docs/glossary.md` |
| **Agent Feature Comparison** | `docs/agent-features.md` - Feature matrix across Claude, OpenCode, Codex, Gemini |

## OpenCode Integration

| Topic | Location |
|-------|----------|
| **Overview & Entry Point** | `docs/opencode/README.md` |
| **Implementation** | `internal/opencode/` - Plugin and tests |
| **Plugin Code** | `internal/opencode/plugin/gastown.js` |
| **Design & Plans** | `docs/opencode/design/` - Roadmaps and strategies |
| **Next Steps** | `docs/opencode/design/next-steps.md` - What to work on |
| **Change History** | `docs/opencode/HISTORY.md` - Chronological log |
| **Standards** | `docs/opencode/CONTRIBUTING.md` - Doc standards |
| **Unit Tests** | `internal/opencode/*_test.go` |
| **E2E Tests** | `scripts/test-runtime-e2e.sh`, `scripts/test-opencode-*.sh` |

