# Agent Instructions

See **CLAUDE.md** for complete agent context and instructions.

This file exists for compatibility with tools that look for AGENTS.md.

> **Recovery**: Run `gt prime` after compaction, clear, or new session

Full context is injected by `gt prime` at session start.

**Platforms**: Always write code that works on Unix (MacOS), Linux, and Windows.

**Validation**: Before closing the bd ticket, always check that the code works using the following commands:

```bash
golangci-lint run ./...
go test -v -timeout=4m ./...
go test -v -tags=integration -timeout=4m ./internal/cmd/...
```

When you fix errors, add a note in the "Lessons Learned" section of the `AGENTS.md` file how to prevent the problem, in order to ensure we do not repeat the same mistake

## Lessons Learned
