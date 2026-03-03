# GitHub Copilot Instructions for Gas Town

- This is a Go monorepo. Use Go conventions and run `go test ./...` from the repo root when validating changes.
- Use existing `gt` and `bd` CLI patterns for agent, convoy, and beads workflows instead of inventing new commands.
- Keep changes minimal and consistent with existing runtime/agent abstractions (`RuntimeConfig`, `AgentPreset`, role templates).
- Preserve workspace boundaries (never write outside the current rig/worktree).
- Prefer updating existing tests to cover new behavior.
- **ai-marketplace deployment**: use the `.claude/skills/deploy-ai-marketplace/SKILL.md` skill — do not invent deployment steps from scratch.
