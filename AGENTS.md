# Agent Instructions

See **CLAUDE.md** for complete agent context and instructions.

This file exists for compatibility with tools that look for AGENTS.md.

> **Recovery**: Run `gt prime` after compaction, clear, or new session

Full context is injected by `gt prime` at session start.

<!-- beads-agent-instructions-v2 -->

---

## Beads Workflow Integration

This project uses [beads](https://github.com/steveyegge/beads) for issue tracking. Issues live in `.beads/` and are tracked in git.

Two CLIs: **bd** (issue CRUD) and **bv** (graph-aware triage, read-only).

### bd: Issue Management

```bash
bd ready              # Unblocked issues ready to work
bd list --status=open # All open issues
bd show <id>          # Full details with dependencies
bd create --title="..." --type=task --priority=2
bd update <id> --status=in_progress
bd close <id>         # Mark complete
bd close <id1> <id2>  # Close multiple
bd dep add <a> <b>    # a depends on b
bd sync               # Sync with git
```

### bv: Graph Analysis (read-only)

**NEVER run bare `bv`** — it launches interactive TUI. Always use `--robot-*` flags:

```bash
bv --robot-triage     # Ranked picks, quick wins, blockers, health
bv --robot-next       # Single top pick + claim command
bv --robot-plan       # Parallel execution tracks
bv --robot-alerts     # Stale issues, cascades, mismatches
bv --robot-insights   # Full graph metrics: PageRank, betweenness, cycles
```

### Workflow

1. **Start**: `bd ready` (or `bv --robot-triage` for graph analysis)
2. **Claim**: `bd update <id> --status=in_progress`
3. **Work**: Implement the task
4. **Complete**: `bd close <id>`
5. **Sync**: `bd sync` at session end

### Session Close Protocol

```bash
git status            # Check what changed
git add <files>       # Stage code changes
bd sync               # Commit beads changes
git commit -m "..."   # Commit code
bd sync               # Commit any new beads changes
git push              # Push to remote
```

### Key Concepts

- **Priority**: P0=critical, P1=high, P2=medium, P3=low, P4=backlog (numbers only)
- **Types**: task, bug, feature, epic, question, docs
- **Dependencies**: `bd ready` shows only unblocked work

<!-- end-beads-agent-instructions -->

<!-- gastown-agent-instructions-v1 -->

---

## Gas Town Multi-Agent Communication

This workspace is part of a **Gas Town** multi-agent environment. You communicate
with other agents using `gt` commands — never by printing text or using raw tmux.

### Nudging Agents (Immediate Delivery)

`gt nudge` sends a message directly to another agent's active session:

```bash
gt nudge mayor "Status update: PR review complete"
gt nudge laneassist/crew/dom "Check your mail — PR ready for review"
gt nudge witness "Polecat health check needed"
gt nudge refinery "Merge queue has items"
```

**Target formats:**
- Role shortcuts: `mayor`, `deacon`, `witness`, `refinery`
- Full path: `<rig>/crew/<name>`, `<rig>/polecats/<name>`

**Important:** `gt nudge` is the ONLY way to send text to another agent's session.
Never print "Hey @name" — the other agent cannot see your terminal output.

### Sending Mail (Persistent Messages)

`gt mail` sends messages that persist across session restarts:

```bash
# Reading
gt mail inbox                    # List messages
gt mail read <id>                # Read a specific message

# Sending (use --stdin for multi-line content)
gt mail send mayor/ -s "Subject" -m "Short message"
gt mail send laneassist/crew/dom -s "PR Review" --stdin <<'BODY'
Multi-line message content here.
Details about the PR and what to look for.
BODY
gt mail send --human -s "Subject" -m "Message to overseer"
```

### When to Use Which

| Want to... | Command | Why |
|------------|---------|-----|
| Wake a sleeping agent | `gt nudge <target> "msg"` | Immediate delivery |
| Send detailed task/info | `gt mail send <target> -s "..." --stdin` | Persists across restarts |
| Both: send + wake | `gt mail send` then `gt nudge` | Mail carries payload, nudge wakes |

### Context Recovery

After compaction or new session, run `gt prime` to reload your full role context,
identity, and any pending work.

```bash
gt prime              # Full context reload
gt hook               # Check for assigned work
gt mail inbox         # Check for messages
```

<!-- end-gastown-agent-instructions -->

<!-- gastown-quality-contract-v1 -->

---

## Quality Contract: What Every Polecat Must Deliver

Gastown enforces a 10-layer quality system. **CI is the minimum bar, not the goal.**

### Required: Test coverage rules

Every PR that changes behavior must include tests. The Refinery's coverage gate rejects PRs below 60% total coverage.

**Failure-mode tests (Layer 5)** — Use `//go:build failure` tag:

```go
//go:build failure

package mypackage_test

func TestMyFunc_TimeoutFromUpstream(t *testing.T) { ... }
func TestMyFunc_MalformedInput(t *testing.T) { ... }
func TestMyFunc_RetryIdempotency(t *testing.T) { ... }
```

Write failure-mode tests when your code:
- Calls an external service (timeout, unavailable, bad response)
- Parses input (malformed, empty, oversized)
- Has retry logic (verify idempotency)
- Handles auth (expired token, wrong credentials)
- Writes state (partial write, interrupted)

Run them locally: `./scripts/ci/verify.sh failure`

**Fuzz tests (Layer 5)** — Required for parsers, validators, and auth handlers:

```go
func FuzzParseMyFormat(f *testing.F) {
    f.Add([]byte("valid input"))
    f.Fuzz(func(t *testing.T, b []byte) {
        _, _ = ParseMyFormat(b) // must not panic
    })
}
```

Run locally: `./scripts/ci/verify.sh fuzz`

### Required: CI entrypoints

| Script | When | What |
|--------|------|------|
| `./scripts/ci/verify.sh pre-merge` | Before submitting with `gt done` | guard + build + unit + lint + coverage |
| `./scripts/ci/verify.sh integration` | Refinery post-squash gate | integration tests on merged result |
| `./scripts/ci/smoke.sh` | Post-merge (Witness triggers) | fast build + guard |
| `./scripts/ci/release-check.sh` | Release pipeline | smoke + vuln scan + version check |

### What the Refinery checks automatically

The Refinery runs these gates before merging your branch:
- `verify` (pre-merge): guard + build + unit + lint
- `coverage` (pre-merge): total coverage ≥ 60%
- `vuln` (pre-merge): `govulncheck ./...` — no known CVEs
- `integration` (post-squash): integration tests on the merged result

If any gate fails, the MR is rejected with `MERGE_FAILED` and you get a nudge.

### What happens after merge

The Witness runs `./scripts/ci/smoke.sh` after every merge. If it fails, a P0 bug bead is auto-created and escalated. You don't need to do anything — but if your merge caused it, the bead may be routed back to you.

<!-- end-gastown-quality-contract -->

