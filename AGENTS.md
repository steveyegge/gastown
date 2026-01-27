# Agent Instructions

See **CLAUDE.md** for complete agent context and instructions.

This file exists for compatibility with tools that look for AGENTS.md.

> **Recovery**: Run `gt prime` after compaction, clear, or new session

Full context is injected by `gt prime` at session start.

## Pull Requests

When creating PRs, always use `--repo` with the origin remote (gh CLI defaults to upstream for forks):

```bash
gh pr create --repo $(git remote get-url origin | sed 's/.*github.com[:/]\(.*\)\.git/\1/')
```
