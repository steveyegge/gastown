# Agent Instructions

See **CLAUDE.md** for complete agent context and instructions.

This file exists for compatibility with tools that look for AGENTS.md.

## Landing the Plane (Session Completion)

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `gt done --exit` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work with `bd close <issue> --reason "..."`
4. **PUSH TO REMOTE** - This is MANDATORY before gt done:
   ```bash
   git pull --rebase
   bd sync
   git push -u origin HEAD
   git status  # MUST show branch is up to date
   ```
5. **SUBMIT TO MERGE QUEUE** - This is the FINAL step:
   ```bash
   gt done --exit
   ```
   This creates the MR bead, notifies the Refinery, and exits your session.

**CRITICAL RULES:**
- Work is NOT complete until `gt done --exit` succeeds
- You MUST push BEFORE running `gt done` (it will fail otherwise)
- NEVER stop after push - that leaves work stranded outside the merge queue
- NEVER say "ready when you are" - YOU must run the full protocol

## Dependency Management

Periodically check for outdated dependencies:

```bash
go list -m -u all | grep '\['
```

Update direct dependencies:

```bash
go get <package>@latest
go mod tidy
go build ./...
go test ./...
```

Check release notes for breaking changes before major version bumps.
