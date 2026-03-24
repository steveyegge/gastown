# Rally Tavern ↔ Gas Town Phase 1 Integration Plan

**Bead:** gt-29z
**Phase:** 1 — BEFORE + DURING (planning & implementation knowledge injection)
**Date:** 2026-03-11

---

## Architectural Decisions (locked)

| # | Decision | Choice |
|---|----------|--------|
| Q1 | rally_tavern location | `$GT_ROOT/rally_tavern/` with graceful degradation if absent |
| Q2 | Go vs bash | Pure Go reimplementation (no dependency on rally_tavern bash scripts) |
| Q3 | Search scope | Knowledge-only in v1 (artifacts stay in existing bash scripts) |
| Q4 | AFTER trigger | Agent self-nomination during `gt done` (agent decides if bead is knowledge-worthy) |
| Q5 | Approval routing | Two-stage — rig Mayor nominates, rally_tavern Mayor accepts |

---

## Feature List

### 00 — Knowledge Index Loader

**Complexity:** 2/5
**Dependencies:** none

Load and parse the rally_tavern knowledge directory structure into an in-memory
index. The knowledge lives at `$GT_ROOT/rally_tavern/mayor/rig/knowledge/` in
three subdirectories:

- `practices/` — best practices YAML files (keyed by `codebase_type`, `tags`)
- `solutions/` — known solutions YAML files (keyed by `tags`)
- `learned/` — lessons learned YAML files (keyed by `tags`)

Each YAML file has a common schema: `id`, `title`, `summary`, `details`,
`tags []string`, `codebase_type string` (optional), `contributed_by`,
`created_at`, `verified_by []string`, plus type-specific fields (`gotchas`,
`examples`, `steps`, etc.).

**Implementation:**
- New package: `internal/rally/` with `knowledge.go`
- Struct: `KnowledgeEntry` with common fields + raw YAML for type-specific data
- Func: `LoadKnowledgeIndex(gtRoot string) (*KnowledgeIndex, error)`
  - Walks `$GT_ROOT/rally_tavern/mayor/rig/knowledge/{practices,solutions,learned}/`
  - Parses each `.yaml` file into `KnowledgeEntry`
  - Returns `nil, nil` if rally_tavern directory absent (graceful degradation)
- Func: `(idx *KnowledgeIndex) Search(query SearchQuery) []KnowledgeEntry`
  - Matches on tags (exact), codebase_type (exact), and title/summary (substring)
  - Returns sorted by relevance (exact tag match > codebase_type > substring)

**Files:**
- NEW: `internal/rally/knowledge.go`
- NEW: `internal/rally/knowledge_test.go`

---

### 01 — Tavern Profile Reader

**Complexity:** 1/5
**Dependencies:** none

Read the `tavern-profile.yaml` from the current project's repo root and extract
fields relevant for knowledge matching: `tags`, `facets.languages`,
`facets.frameworks`, `architecture.style`, `needs`, `constraints.must_use`.

**Implementation:**
- New file: `internal/rally/profile.go`
- Struct: `TavernProfile` with the subset of fields needed for matching
- Func: `LoadProfile(repoRoot string) (*TavernProfile, error)`
  - Reads `tavern-profile.yaml` from repo root
  - Returns `nil, nil` if file absent (graceful degradation)
- Func: `(p *TavernProfile) ToSearchQuery() SearchQuery`
  - Converts profile fields to a search query: tags → tag matches,
    languages/frameworks → codebase_type candidates, needs → keyword search

**Files:**
- NEW: `internal/rally/profile.go`
- NEW: `internal/rally/profile_test.go`

---

### 02 — `gt rally search <query>` Command

**Complexity:** 3/5
**Dependencies:** 00, 01

New `gt rally` parent command with `search` subcommand. Searches the
rally_tavern knowledge base by query string, optionally filtered by
tavern-profile context.

**Usage:**
```
gt rally search "dolt session management"
gt rally search --tags=security,auth
gt rally search --codebase-type=go-cobra
gt rally search --profile       # auto-query from tavern-profile.yaml
```

**Behavior:**
1. Resolve `$GT_ROOT` (from config or `~/gt`)
2. Call `LoadKnowledgeIndex(gtRoot)` — if nil, print info message and exit 0
3. If `--profile` flag: load tavern-profile.yaml, convert to SearchQuery
4. Otherwise: build SearchQuery from positional args + flags
5. Run `idx.Search(query)`
6. Print results as formatted list: title, summary, tags, source file
7. With `--json` flag: output JSON array for programmatic use
8. With `--full` flag: include `details` field in output

**Graceful degradation:** If `$GT_ROOT/rally_tavern/` doesn't exist, print
`"Rally Tavern not available (no $GT_ROOT/rally_tavern/ directory)"` and
exit 0. No error, no crash.

**Implementation:**
- New file: `internal/cmd/rally.go` — parent command
- New file: `internal/cmd/rally_search.go` — search subcommand
- Register in root command; add to beads-exempt list in `root.go`

**Files:**
- NEW: `internal/cmd/rally.go`
- NEW: `internal/cmd/rally_search.go`
- MODIFY: `internal/cmd/root.go` (register command, add to beads-exempt)

---

### 03 — `gt rally lookup <tag>` Command

**Complexity:** 2/5
**Dependencies:** 00

Targeted single-tag lookup for agent self-serve during implementation.
Optimized for the DURING phase: agent knows exactly what tag to look up.

**Usage:**
```
gt rally lookup security
gt rally lookup dolt
gt rally lookup ios-credentials
```

**Behavior:**
1. Load knowledge index (graceful degradation as in 02)
2. Exact-match on tag across all knowledge categories
3. Print matching entries with full details (agents need the content)
4. With `--json` flag: structured output
5. With `--summary` flag: titles + summaries only (compact)

**Implementation:**
- New file: `internal/cmd/rally_lookup.go`
- Reuses `internal/rally/` package from feature 00

**Files:**
- NEW: `internal/cmd/rally_lookup.go`

---

### 04 — Formula Integration: mol-idea-to-plan

**Complexity:** 2/5
**Dependencies:** 02

Update `mol-idea-to-plan.formula.toml` to inject relevant rally_tavern
knowledge into the planning context during the `intake` step.

**Changes to intake step:**
After loading the project profile, add a knowledge search step:

```
**Rally Tavern knowledge (optional):**
If `gt rally search --profile --json` succeeds and returns results, include
a `## Relevant Knowledge` section in the PRD draft summarizing the matching
practices, solutions, and lessons learned. This gives reviewers context about
known patterns that apply to this feature.

```bash
# Search for relevant knowledge based on project profile
gt rally search --profile --json > /tmp/rally-knowledge.json 2>/dev/null
# If results exist, include them in PRD context
```
```

**Implementation:**
- MODIFY: `internal/formula/formulas/mol-idea-to-plan.formula.toml`
  - Add knowledge search instructions to `intake` step description
  - Reference knowledge results in `prd-review` step context
- No Go code changes — formula is TOML instructions interpreted by agents

**Files:**
- MODIFY: `internal/formula/formulas/mol-idea-to-plan.formula.toml`

---

### 05 — Formula Integration: mol-polecat-work

**Complexity:** 2/5
**Dependencies:** 03

Update `mol-polecat-work.formula.toml` to append relevant practices/solutions
to polecat context during the `implement` step.

**Changes to implement step:**
Add a knowledge lookup instruction at the beginning of implementation:

```
**Rally Tavern knowledge (optional — run once at start):**
Before starting implementation, check if rally_tavern has relevant practices:

```bash
# Quick lookup based on issue tags or keywords from the issue description
gt rally search "<keywords from issue>" --summary 2>/dev/null
```

If results are found, read the relevant entries with `gt rally lookup <tag>`
and apply any applicable patterns. This is advisory — use judgment about
which practices apply to your specific task.
```

**Implementation:**
- MODIFY: `internal/formula/formulas/mol-polecat-work.formula.toml`
  - Add knowledge lookup instructions to `implement` step
  - Keep it lightweight — agents should not be blocked by rally_tavern absence

**Files:**
- MODIFY: `internal/formula/formulas/mol-polecat-work.formula.toml`

---

## Dependency Graph

```
00 (Knowledge Index Loader)
├──→ 02 (gt rally search) ──→ 04 (mol-idea-to-plan integration)
└──→ 03 (gt rally lookup)  ──→ 05 (mol-polecat-work integration)

01 (Tavern Profile Reader) ──→ 02 (gt rally search)
```

**Execution waves:**
- Wave 1: 00, 01 (no dependencies, can run in parallel)
- Wave 2: 02, 03 (depend on 00; 02 also on 01)
- Wave 3: 04, 05 (depend on 02, 03 respectively)

---

## Complexity Summary

| # | Feature | Complexity | Est. Files |
|---|---------|-----------|------------|
| 00 | Knowledge Index Loader | 2/5 | 2 new |
| 01 | Tavern Profile Reader | 1/5 | 2 new |
| 02 | `gt rally search` | 3/5 | 3 new + 1 modify |
| 03 | `gt rally lookup` | 2/5 | 1 new |
| 04 | mol-idea-to-plan formula | 2/5 | 1 modify |
| 05 | mol-polecat-work formula | 2/5 | 1 modify |
| **Total** | | **12/30** | **8 new + 2 modify** |

---

## Tech Stack Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| YAML parser | `gopkg.in/yaml.v3` | Already used in codebase for tavern-profile.yaml |
| Knowledge index | In-memory (no persistence) | Knowledge corpus is small (~20-50 files), read-only, loaded on demand |
| Search algorithm | Tag exact match + substring on title/summary | Simple, deterministic, no external dependencies; upgrade to fuzzy later if needed |
| Command structure | `gt rally {search,lookup}` | Follows existing pattern (`gt mail {inbox,read,send}`, `gt tap {guard,list}`) |
| Output format | Human-readable default + `--json` flag | Consistent with `bd` and `gt` CLI conventions |
| Graceful degradation | Return nil/empty, print info, exit 0 | Rally Tavern is optional; agents must never crash if it's absent |

---

## File Locations (All New/Modified)

### New Files
```
internal/rally/knowledge.go          # KnowledgeIndex, KnowledgeEntry, Search
internal/rally/knowledge_test.go     # Unit tests for knowledge loading + search
internal/rally/profile.go            # TavernProfile loader + SearchQuery converter
internal/rally/profile_test.go       # Unit tests for profile reading
internal/cmd/rally.go                # Parent command: gt rally
internal/cmd/rally_search.go         # Subcommand: gt rally search
internal/cmd/rally_lookup.go         # Subcommand: gt rally lookup
```

### Modified Files
```
internal/cmd/root.go                                    # Register rally command + beads-exempt
internal/formula/formulas/mol-idea-to-plan.formula.toml  # Add knowledge search to intake step
internal/formula/formulas/mol-polecat-work.formula.toml  # Add knowledge lookup to implement step
```

---

## Phase 2 — AFTER Pipeline (Complete, 2026-03-12)

The knowledge contribution loop: agents nominate what they learn, franklin reviews it.

| # | Feature | Status |
|---|---------|--------|
| 06 | `gt rally nominate` command | ✅ done |
| 07 | `Nomination` struct + wire format (`RALLY_NOMINATION_V1`) | ✅ done |
| 08 | franklin — knowledge curator agent in rally_tavern | ✅ done |
| 09 | Nomination prompt in `mol-polecat-work` self-clean step | ✅ done |

**Test case:** tmux mouse support tip nominated and delivered to `rally_tavern/franklin` inbox.

## Out of Scope (Phase 3+)

- **Artifact search**: Searching rally_tavern artifacts (code templates, starters)
- **Fuzzy search**: Full-text search, embeddings, or semantic matching
- **Knowledge caching**: Persistent index across sessions
- **MCP server integration**: rally_tavern MCP server for richer tool integration
