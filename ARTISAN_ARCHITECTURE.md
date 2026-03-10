# Artisan Architecture: Specialized Long-Lived Workers

## Overview

Three new rig-level roles that add specialized, long-lived workers with a structured
development methodology and an intelligent dispatch layer.

| Role | Type | Count | Purpose |
|------|------|-------|---------|
| **Architect** | LLM, long-lived | 1 per rig | Codebase oracle, examiner, spec designer |
| **Conductor** | LLM, long-lived | 1 per rig | Planner, splitter, router, phase enforcer |
| **Artisan** | LLM, long-lived | N per rig | Specialized workers (user-defined specialties) |

## Agent Hierarchy (Updated)

```
Mayor (strategic — human conversation)
  │
  ↓ creates high-level beads
  │
Conductor (tactical — plans phases, splits beads, routes)
  │
  ↔ Architect (oracle — examines code, answers questions, designs specs)
  │
  ↓ routes sub-beads to specialists
  │
Artisans (specialized execution — frontend, backend, tests, etc.)
  ↑ can consult Architect via mail
  │
  ↓ monitored by
  │
Witness (health, stuck detection — now covers artisans too)
Refinery (merge queue — unchanged)
```

## Directory Structure (Deployed)

```
~/gt/<rig>/
├── architect/              # NEW — codebase oracle
│   └── rig/                # Worktree from mayor/rig (read-only, always on main)
├── conductor/              # NEW — planner + router
│   └── rig/                # Worktree from mayor/rig
├── artisans/               # NEW — specialized workers
│   ├── frontend-1/
│   │   ├── rig/            # Full git clone
│   │   └── specialty.toml  # { specialty = "frontend" }
│   ├── frontend-2/
│   │   ├── rig/
│   │   └── specialty.toml
│   ├── backend-1/
│   │   ├── rig/
│   │   └── specialty.toml
│   ├── tests-1/
│   │   ├── rig/
│   │   └── specialty.toml
│   ├── security-1/
│   │   ├── rig/
│   │   └── specialty.toml
│   └── docs-1/
│       ├── rig/
│       └── specialty.toml
├── witness/                # Existing — now also monitors artisans
├── refinery/               # Existing — unchanged
├── polecats/               # Existing — still available for generic work
└── crew/                   # Existing — unchanged
```

## Development Methodology (7-Phase Pattern)

Every feature progresses through these phases in order. The Conductor enforces
this pattern and gates transitions. If any phase fails, the Conductor **escalates
to the user** rather than retrying or guessing.

| Phase | Name | Owner | Gate |
|-------|------|-------|------|
| 1 | **Examine** | Architect | Report delivered to Conductor |
| 2 | **Harden** | Tests artisan | PR merged (>90% coverage in target area) |
| 3 | **Modernize** | Specialty artisan(s) | PR merged (code matches best-practices refs) |
| 4 | **Specify** | Tests artisan + Architect | PR staged, **user approves** |
| 5 | **Implement** | Specialty artisan(s) | PR merged (all tests pass) |
| 6 | **Secure** | Security artisan | PR merged (security review passed) |
| 7 | **Document** | Docs artisan | PR merged |

### Failure Handling

If any phase fails (tests can't pass, artisan gets stuck, conflicts arise),
the Conductor **escalates to the user** with:
- Which phase failed and why
- What was attempted
- Suggested options for resolution

The Conductor does NOT retry, guess, or skip phases autonomously.

### Phase 1: Examine

The Architect analyzes the affected code area:
- Maps current architecture and dependencies
- Identifies test coverage gaps
- Notes patterns and anti-patterns
- Produces API contracts for any new/changed endpoints
- Produces model/database table change specifications
- Produces a structured report for the Conductor

### Phase 2: Harden

The Tests artisan brings **the target area of the proposed addition** to >90% coverage:
- Scoped to packages/files the Architect identified in Phase 1 as affected
- Does NOT attempt whole-codebase coverage — only the area the feature will touch
- Writes tests for existing behavior (not new features)
- Ensures the code that's about to change has a solid safety net first
- Communicates remaining holes to user
- Iterates until user is satisfied
- Submits PR

### Phase 3: Modernize

Specialty artisan(s) bring **the target area** up to best-practices:
- Scoped to the same packages/files identified by the Architect in Phase 1
- Best-practices reference files are stored in `~/.gastown/best-practices/` (shared across projects)
- Artisans refactor the target area to match the patterns in those reference files
- No new functionality — only structural improvements to code that's about to change
- Submits PR

### Phase 4: Specify (Test-Driven)

Tests artisan writes failing tests for the new feature:
- Architect designs the test contracts (what should be asserted)
- Architect produces API contracts and model/DB table change specs
- Tests artisan implements the test code based on Architect's specs
- PR is staged and **paused for user feedback**
- User reviews tests-as-spec to confirm intent
- No implementation code yet — only tests

### Phase 5: Implement

Specialty artisan(s) write code to make failing tests pass:
- Frontend, backend, etc. work on their specialty branches
- All Phase 4 tests must pass
- PR is completed and submitted to Refinery

### Phase 6: Secure

Security artisan reviews the implementation for vulnerabilities:
- OWASP top 10: injection, XSS, auth bypass, etc.
- Scoped to the target area — reviews new and changed code from Phase 5
- Checks for secrets/credentials in code
- Validates input sanitization at system boundaries
- Reviews auth/authz changes if applicable
- Can consult Architect for context on security-sensitive patterns
- Submits fixes as PR through normal Refinery flow

### Phase 7: Document

Docs artisan writes/updates documentation for the new feature:
- API docs, README updates, architecture docs, usage examples
- Scoped to the target area — documents what changed and why
- Can consult Architect for context on design decisions
- Submits PR through normal Refinery flow

## Best-Practices References

Best-practices reference files live in `~/.gastown/best-practices/` and are shared
across all projects. This allows a consistent coding standard without re-specifying
patterns per rig.

```
~/.gastown/best-practices/
├── go/
│   ├── error-handling.go       # How to handle errors
│   ├── api-handler.go          # HTTP handler patterns
│   └── test-structure.go       # Test organization
├── typescript/
│   ├── component.tsx           # React component patterns
│   └── api-client.ts           # API client patterns
└── general/
    ├── naming.md               # Naming conventions
    └── project-structure.md    # Directory organization
```

During Phase 3 (Modernize), artisans reference these files to refactor the target
area. The Architect also consults them when designing specs in Phase 4.

## Conductor: Planner + Router

### Responsibilities

1. **Receive** high-level beads from Mayor/humans
2. **Consult** Architect for Phase 1 analysis
3. **Plan** branch strategy and PR structure
4. **Split** beads into specialty sub-beads
5. **Route** sub-beads to available artisans by specialty
6. **Enforce** the 5-phase development pattern
7. **Track** progress through phases, gate transitions

### Conductor Patrol Cycle

```
1. Check for unrouted beads (new work from Mayor)
2. For each:
   a. Send to Architect for Phase 1 examination
   b. Receive architecture report
   c. Design PR plan:
      - Parent branch: integration/<feature-name>
      - Child branches per specialty
      - Dependency ordering
   d. Create sub-beads with specialty labels
   e. Link sub-beads to parent (convoy)
   f. Route Phase 2 work (harden) to tests artisan
   g. Wait for Phase 2 completion before routing Phase 3
   h. Continue through phases...
3. Monitor in-flight work, rebalance if needed
4. Sleep/repeat
```

### Branch Strategy (Example)

```
main
 └── integration/user-profile          (parent — convoy)
      ├── user-profile/harden          (Phase 2 — tests artisan)
      ├── user-profile/modernize       (Phase 3 — specialty artisans)
      ├── user-profile/backend         (Phase 5 — backend artisan)
      ├── user-profile/frontend        (Phase 5 — frontend artisan)
      ├── user-profile/tests           (Phase 4+5 — tests artisan)
      ├── user-profile/security        (Phase 6 — security artisan)
      └── user-profile/docs            (Phase 7 — docs artisan)
```

### What the User Sees (Conductor Log)

```
[Conductor] Received gt-abc12: "Add user profile page"
[Conductor] → Asking Architect to examine internal/user/ and related areas
[Architect] Report: 3 packages affected, 62% test coverage, auth middleware dependency
[Conductor] Planning phases for gt-abc12...
  Phase 2: Harden — gt-hrd01 → tests-1 (target area coverage: 62% → 90%+)
  Phase 3: Modernize — gt-mod01 → backend-1 (target area, ref: internal/refinery/engineer.go)
  Phase 4: Specify — gt-spc01 → tests-1 (Architect designs contracts)
  Phase 5: Implement:
    gt-imp01 "Profile API endpoints"     → backend-1
    gt-imp02 "Profile page components"   → frontend-1
    gt-imp03 "Profile integration tests" → tests-1
  Phase 6: Secure:
    gt-sec01 "Profile security review"   → security-1
  Phase 7: Document:
    gt-doc01 "Profile feature docs"      → docs-1
[Conductor] Starting Phase 2: slinging gt-hrd01 → tests-1
```

## Architect: Codebase Oracle

### Responsibilities

1. **Phase 1 examination** — deep architecture analysis on demand
2. **API contract design** — defines new/changed endpoint contracts for cross-specialty coordination
3. **Model/DB spec design** — specifies database table changes, model definitions
4. **Phase 4 spec design** — defines what tests should assert, based on API contracts and model specs
5. **On-demand consultation** — any agent can ask questions via mail

### How Agents Consult the Architect

```bash
# Conductor asks about a bead's scope
gt mail send architect "What areas does gt-abc12 touch? What specialties needed?"

# Frontend artisan asks about a pattern
gt mail send architect "How does the auth middleware work? I need to integrate."

# Tests artisan asks about coverage
gt mail send architect "What's untested in internal/refinery/?"

# Architect responds via mail
gt mail send frontend-1 "Auth uses middleware chain in internal/auth/chain.go..."
```

### Workspace: Read-Only Worktree

The Architect uses a worktree from `mayor/rig`, always on main. Since the
Architect never commits code — only reads and analyzes — it doesn't need an
isolated workspace. When the Refinery lands PRs to main, the Architect
automatically sees the latest merged code without needing to sync.

This avoids staleness: artisans land PRs, Refinery merges them, and the
Architect's view of the codebase stays current.

### Persistent Knowledge

The Architect is long-lived and builds up context about the codebase over time.
It doesn't need to re-examine from scratch each session.

## Artisan: Specialized Worker

### Properties

- **Long-lived** — persistent identity and workspace (survives across assignments)
- **Full git clone** — isolated workspace (like crew, not worktree)
- **Specialty-bound** — each artisan has a defined domain
- **Multiple per specialty** — frontend-1, frontend-2, etc. for parallelism
- **Monitored by Witness** — health checks, stuck detection, same as polecats
- **Can consult Architect** — ask questions via mail

### Naming Convention

`<specialty>-<number>`: `frontend-1`, `backend-1`, `tests-1`, `frontend-2`, etc.

### Specialty Definitions (Per-Rig)

User-defined in `<rig>/conductor/specialties.toml`:

```toml
[[specialty]]
name = "frontend"
description = "UI components, styling, browser interactions, accessibility"
prompt_template = "artisan-frontend.md.tmpl"
file_patterns = ["src/components/**", "src/pages/**", "*.css", "*.tsx"]
labels = ["frontend", "ui", "css"]

[[specialty]]
name = "backend"
description = "APIs, database, auth, server-side logic"
prompt_template = "artisan-backend.md.tmpl"
file_patterns = ["internal/**", "api/**", "*.go", "models/**"]
labels = ["backend", "api", "database"]

[[specialty]]
name = "tests"
description = "Test writing, coverage, fixtures, mocking"
prompt_template = "artisan-tests.md.tmpl"
file_patterns = ["*_test.go", "**/*.test.ts", "tests/**"]
labels = ["tests", "testing", "coverage"]

[[specialty]]
name = "security"
description = "Security review, vulnerability scanning, auth/authz, input validation"
prompt_template = "artisan-security.md.tmpl"
file_patterns = ["internal/auth/**", "internal/middleware/**", "**/*auth*", "**/*token*"]
labels = ["security", "auth", "vulnerability"]

[[specialty]]
name = "docs"
description = "Documentation, README files, API docs, architecture docs, inline comments"
prompt_template = "artisan-docs.md.tmpl"
file_patterns = ["docs/**", "*.md", "**/*.md"]
labels = ["docs", "documentation", "readme"]
```

The Conductor uses `file_patterns` and `labels` as hints when classifying work,
but as an LLM agent it also reasons about bead content directly.

## Files to Create/Modify

### New Files

| File | Purpose |
|------|---------|
| `internal/config/roles/artisan.toml` | Artisan role definition |
| `internal/config/roles/architect.toml` | Architect role definition |
| `internal/config/roles/conductor.toml` | Conductor role definition |
| `internal/cmd/artisan.go` | `gt artisan` command group |
| `internal/cmd/artisan_add.go` | `gt artisan add <specialty> [--rig]` |
| `internal/cmd/artisan_list.go` | `gt artisan list [--rig]` |
| `internal/cmd/artisan_remove.go` | `gt artisan remove <name> [--rig]` |
| `internal/cmd/conductor.go` | `gt conductor` command group |
| `internal/cmd/architect.go` | `gt architect` command group |
| `internal/formula/formulas/mol-conductor-patrol.formula.toml` | Conductor patrol formula |
| `internal/formula/formulas/mol-architect-examine.formula.toml` | Architect examination formula |
| Prompt templates for each specialty | Domain-specific context |

### Modified Files

| File | Change |
|------|--------|
| `internal/cmd/prime.go` | Add `RoleArtisan`, `RoleArchitect`, `RoleConductor` constants |
| `internal/constants/constants.go` | Add role constants |
| `internal/config/roles.go` | Add to `AllRoles()`, `RigRoles()` |
| `internal/cmd/role.go` | Add to `detectRole()`, `parseRoleString()`, `getRoleHome()`, `ActorString()`, `runRoleList()` |
| `internal/cmd/sling_target.go` | Support `<rig>/artisans/<name>` target |
| `internal/cmd/sling_dispatch.go` | Support artisan dispatch |
| `internal/cmd/polecat_spawn.go` | Artisan spawn logic (or separate artisan spawner) |
| Witness handlers | Add artisan monitoring |
| `internal/config/roles_test.go` | Update `AllRoles` count assertion |

## Implementation Order

1. **Artisan role** — TOML config, role registration, directory setup, `gt artisan add/list/remove`
2. **Architect role** — TOML config, role registration, directory setup
3. **Conductor role** — TOML config, role registration, directory setup
4. **Specialty system** — per-rig `specialties.toml` parsing, prompt template loading
5. **Conductor planning** — bead analysis, branch design, sub-bead creation, phase enforcement
6. **Conductor routing** — specialty matching, artisan selection, sling dispatch
7. **Architect consultation** — mail-based Q&A protocol, Phase 1 + Phase 4 formulas
8. **Witness integration** — extend monitoring to cover artisans
9. **Dependency ordering** — phase gating, sequencing sub-beads
10. **Sling integration** — `gt sling --artisan`, `gt sling --specialty` flags
