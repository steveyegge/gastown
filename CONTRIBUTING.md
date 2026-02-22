# Contributing to Gas Town

Thanks for your interest in contributing! Gas Town is experimental software, and we welcome contributions that help explore these ideas.

## Getting Started

1. Fork the repository
2. Clone your fork
3. Install prerequisites (see README.md)
4. Build and test: `go build -o gt ./cmd/gt && go test ./...`

## Development Workflow

We use a direct-to-main workflow for trusted contributors. For external contributors:

1. Create a feature branch from `main`
2. Make your changes
3. Ensure tests pass: `go test ./...`
4. Submit a pull request

### PR Branch Naming

**Never create PRs from your fork's `main` branch.** Always create a dedicated branch for each PR:

```bash
# Good - dedicated branch per PR
git checkout -b fix/deacon-startup upstream/main
git checkout -b feat/auto-seance upstream/main

# Bad - PR from main accumulates unrelated commits
git checkout main  # Don't PR from here!
```

Why this matters:
- PRs from `main` accumulate ALL commits pushed to your fork
- Multiple contributors pushing to the same fork's `main` creates chaos
- Reviewers can't tell which commits belong to which PR
- You can't have multiple PRs open simultaneously

Branch naming conventions:
- `fix/*` - Bug fixes
- `feat/*` - New features
- `refactor/*` - Code restructuring
- `docs/*` - Documentation only

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Keep functions focused and small
- Add comments for non-obvious logic
- Include tests for new functionality

## Event-First Observability

Gas Town operations should be diagnosable through `gt feed` without requiring
tmux pane inspection or ad-hoc log spelunking.

When changing command behavior:
- Emit feed-visible events for major state transitions (start/stop, sling, handoff, done, escalation, merge).
- Use structured payload helpers from `internal/events`.
- Keep event logging best-effort (do not fail the command because event logging failed).
- Update `docs/feed.md` when adding new high-signal event types.

## Design Philosophy

Gas Town follows two core principles that shape every contribution. Understanding
these will save you (and reviewers) time.

### Zero Framework Cognition (ZFC)

**Go provides transport. Agents provide cognition.**

Gas Town's Go code handles plumbing: tmux sessions, message delivery, hooks,
nudges, file transport, and observability primitives (like `bd show --json`).
All reasoning, judgment calls, and decision-making happen in the AI agents via
molecule formulas and role templates.

This means:
- **No hardcoded thresholds in Go.** Don't write `if age > 5*time.Minute`
  to decide if an agent is stuck. Expose the age as data and let the agent decide.
- **No heuristics in Go.** Don't write detection logic that pattern-matches
  agent behavior. Give agents the tools to observe, and let them reason.
- **Formulas over subcommands.** If the feature is "detect X and do Y," it's
  probably a molecule step, not a new `gt` subcommand.

**The test:** Before adding Go code, ask yourself — *"Am I adding transport or
cognition?"* If the answer is cognition, it should be a molecule step or
formula instruction instead.

For the full rationale, see
[Zero Framework Cognition](https://steve-yegge.medium.com/zero-framework-cognition-a-way-to-build-resilient-ai-applications-56b090ed3e69).

### Bitter Lesson Alignment

Gas Town bets on models getting smarter, not on hand-crafted heuristics getting
more elaborate. If an AI agent can observe data and reason about it, we expose
the data (transport) rather than encoding the reasoning (cognition). Today's
clumsy heuristic is tomorrow's technical debt — but a clean observability
primitive ages well.

**Examples:**

| Good (transport) | Bad (cognition in Go) |
|---|---|
| `gt nudge <session> "message"` | Go code deciding *when* to nudge |
| `bd show --json` exposing step status | Go code deciding *what* step status means |
| `tmux has-session` checking liveness | Go code with hardcoded "stuck after N minutes" |

## What to Contribute

Good first contributions:
- Bug fixes with clear reproduction steps
- Documentation improvements
- Test coverage for untested code paths
- Small, focused features

For larger changes, please open an issue first to discuss the approach.

## Commit Messages

- Use present tense ("Add feature" not "Added feature")
- Keep the first line under 72 characters
- Reference issues when applicable: `Fix timeout bug (gt-xxx)`

## Testing

Run the full test suite before submitting:

```bash
go test ./...
```

For specific packages:

```bash
go test ./internal/wisp/...
go test ./cmd/gt/...
```

## Questions?

Open an issue for questions about contributing. We're happy to help!
