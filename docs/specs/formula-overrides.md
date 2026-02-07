[PRD]
# PRD: Formula Source Control & Override System

## Overview

Refactor the formula management system to:
1. **Fix the sync bug** between `.beads/formulas/` and `internal/formula/formulas/` in the gastown repo
2. **Simplify the codebase** by removing redundant provisioning during `gt install`
3. **Give users explicit control** via `gt formula modify` (copy-on-demand)
4. **Provide clear visualization** via `gt formula diff` showing override hierarchy

### Current State (Problematic)

```
.beads/formulas/           ──go generate──►  internal/formula/formulas/  ──embed──►  binary
   (source, tracked)                            (generated, also tracked)

   Problem: PRs edit internal/ directly, go generate overwrites with old .beads/ version
```

### New State (Proposed)

```
internal/formula/formulas/  ──embed──►  binary  ──gt formula modify──►  user overrides
   (single source of truth)              │                                │
                                         │                                ▼
                                         └──────────────────────►  ~/.beads/formulas/ (town)
                                                                   <rig>/.beads/formulas/ (project)
```

## Goals

- Eliminate the `.beads/formulas/` ↔ `internal/formula/formulas/` sync bug permanently
- Remove bulk formula provisioning during `gt install` (32 files users never asked for)
- Remove `.installed.json` checksum tracking and "outdated formula" warnings
- Provide explicit `gt formula modify <name>` for users who want to customize
- Create intuitive `gt formula diff` visualization of the override hierarchy
- Maintain backwards compatibility (existing overrides continue to work)
- Clean up erroneous `.beads/` gitignore rule in gastown repo

## Non-Goals (Out of Scope)

- Mol Mall integration (remote formula registry) - future work
- Formula versioning/pinning (`formula@version` syntax) - future work
- `gt formula create` changes - remains separate command
- Changes to formula resolution order (project → town → embedded stays the same)

## Quality Gates

These commands must pass for every user story:
- `make test` - Run all Go tests
- `make build` - Ensure binary compiles without formula sync diffs

For CLI behavior changes:
- Manual verification of command output format

## User Stories

### US-001: Make internal/formula/formulas/ the source of truth
**Description:** As a developer, I want `internal/formula/formulas/` to be the single source of truth so that I don't accidentally edit the wrong directory and have my changes overwritten.

**Acceptance Criteria:**
- [ ] Remove `//go:generate sh -c "rm -rf formulas && mkdir -p formulas && cp ..."` from `internal/formula/embed.go`
- [ ] Add comment in `embed.go`: `// Formulas in this directory are the source of truth. Edit here, not .beads/formulas/`
- [ ] Remove `.beads/formulas/*.formula.toml` from git tracking: `git rm --cached .beads/formulas/*.formula.toml`
- [ ] Remove the erroneous `.beads/` line from gastown's root `.gitignore` (line 56, was added by accident)
- [ ] Add `.beads/formulas/` to `.gitignore` to prevent accidental re-adding
- [ ] Running `make build` produces zero formula-related diffs
- [ ] All 32+ formulas remain in `internal/formula/formulas/` and compile into binary

### US-002: Add embedded formula read capability
**Description:** As a user, I want formulas to load directly from the embedded binary when no local override exists so that I don't need provisioned copies.

**Acceptance Criteria:**
- [ ] Add `GetEmbeddedFormula(name string) ([]byte, error)` function in `internal/formula/embed.go`
- [ ] Add `GetEmbeddedFormulaNames() ([]string, error)` function to list all embedded formulas
- [ ] Add `EmbeddedFormulaExists(name string) bool` helper function
- [ ] Functions read from `formulasFS` (the go:embed filesystem)
- [ ] Unit tests verify reading embedded formulas by name

### US-003: Update formula resolution to use embedded fallback
**Description:** As a user, I want the formula resolver to check embedded formulas as the final fallback so that formulas always resolve even without local copies.

**Acceptance Criteria:**
- [ ] Modify `findFormulaFile()` in `internal/cmd/formula.go` to add embedded fallback
- [ ] Resolution order: rig `.beads/formulas/` → town `$GT_ROOT/.beads/formulas/` → embedded
- [ ] When resolved from embedded, return a pseudo-path or marker indicating source
- [ ] `gt formula show <name>` works for embedded formulas without local copy
- [ ] `gt formula run <name>` works for embedded formulas without local copy
- [ ] Integration test: fresh install with no `.beads/formulas/` can run formulas

### US-004: Remove formula provisioning from gt install
**Description:** As a user, I want `gt install` to skip formula provisioning so that I don't get 32 files I never asked for.

**Acceptance Criteria:**
- [ ] Remove `formula.ProvisionFormulas()` call from `internal/cmd/install.go`
- [ ] Remove or deprecate `ProvisionFormulas()` function (keep `CopyFormulaTo()` for `modify`)
- [ ] Remove `.installed.json` file handling (no longer needed)
- [ ] Remove `loadInstalledFormulas()`, `saveInstalledFormulas()` functions
- [ ] `gt install` output no longer mentions "Provisioned N formulas"
- [ ] Integration test: `gt install` creates no files in `.beads/formulas/`

### US-005: Implement gt formula modify command
**Description:** As a user, I want to run `gt formula modify <name>` to copy an embedded formula for customization, with clear instructions on what to do next.

**Acceptance Criteria:**
- [ ] Add `gt formula modify <name>` subcommand
- [ ] Default: copies to town level (`$GT_ROOT/.beads/formulas/`)
- [ ] `--rig <name>` flag: copies to rig level (`<rig>/.beads/formulas/`)
- [ ] `--town <path>` flag: explicit town path override
- [ ] If override already exists, error with message: "Override already exists at <path>. Use 'gt formula reset <name>' to remove it first."
- [ ] On success, print:
  ```
  Formula copied to: /path/to/.beads/formulas/<name>.formula.toml
  
  == Formula Modification Guide ==
  
  Formula Structure:
    formula = "name"           # Formula identifier
    type = "workflow"          # workflow | convoy | aspect | expansion
    version = 1                # Increment when making breaking changes
    description = "..."        # What this formula does
  
  Steps (for workflow type):
    [[steps]]
    id = "step-id"             # Unique identifier
    title = "Step Title"       # Human-readable name
    needs = ["other-step"]     # Dependencies (optional)
    description = """          # Instructions for the agent
    What to do in this step...
    """
  
  Variables:
    [vars.myvar]
    description = "What this variable is for"
    required = true            # or false with default
    default = "value"          # Default if not required
  
  Resolution Order:
    1. Rig:   <rig>/.beads/formulas/     (most specific)
    2. Town:  $GT_ROOT/.beads/formulas/  (user customizations)
    3. Embedded: (compiled in binary)     (defaults)
  
  Commands:
    gt formula diff <name>     # See your changes vs embedded
    gt formula reset <name>    # Remove override, restore embedded
    gt formula show <name>     # View formula details
  ```
- [ ] Works when run from any directory (detects town from `$GT_ROOT` or cwd)

### US-006: Implement gt formula diff command (summary view)
**Description:** As a user, I want to run `gt formula diff` to see a visual map of all formula overrides across my town and rigs.

**Acceptance Criteria:**
- [ ] Add `gt formula diff` subcommand (no args = summary view)
- [ ] Scans ALL rigs in the town for overrides (like `git status` scans all files)
- [ ] Scans town-level `.beads/formulas/`
- [ ] Output format when no overrides:
  ```
  No formula overrides found.
  All formulas using embedded defaults (32 formulas available).

  Run 'gt formula modify <name>' to customize a formula.
  ```
- [ ] Output format with overrides:
  ```
  Formula Override Map
  ════════════════════

                            RESOLUTION ORDER
      ┌─────────────────────────────────────────────────────┐
      │  Rig Override  →  Town Override  →  Embedded        │
      └─────────────────────────────────────────────────────┘

  shiny
      embedded ──────────────────────────────────────── ✓ active
      
  mol-polecat-work
      embedded ─┬─► town override ─────────────────────  
                │   ~/gt/.beads/formulas/
                │
                └─► rig override (gastown) ──────────── ✓ active
                    ~/gt/gastown/.beads/formulas/
                    
      [rig vs embedded: 12 lines changed]

  custom-workflow
      (not in embedded) ─► rig (myproject) ──────────── ✓ custom
                          ~/gt/myproject/.beads/formulas/

  ───────────────────────────────────────────────────────────
  Summary: 1 using embedded, 1 with override, 1 custom
  Run 'gt formula diff <name>' for detailed diff
  ```
- [ ] Custom formulas (not in embedded) shown with "(not in embedded)" marker
- [ ] Shows which version is "active" based on resolution order

### US-007: Implement gt formula diff <name> (detailed view)
**Description:** As a user, I want to run `gt formula diff <name>` to see side-by-side diffs showing exactly what changed at each override level.

**Acceptance Criteria:**
- [ ] `gt formula diff <name>` shows detailed diff for specific formula
- [ ] Shows full resolution chain with diffs between each level
- [ ] Uses side-by-side diff format (like `diff -y`)
- [ ] Output format:
  ```
  mol-polecat-work
      ├─ embedded: (compiled in gt v0.5.0)
      ├─ town:     ~/gt/.beads/formulas/mol-polecat-work.formula.toml
      └─ rig:      ~/gt/gastown/.beads/formulas/mol-polecat-work.formula.toml  ◄ active

  [Embedded → Town]
  ────────────────────────────────────────────────────────────────────────────
  embedded                              │ town override
  ────────────────────────────────────────────────────────────────────────────
  timeout = 300                         │ timeout = 600
  ...                                   │ ...
  ────────────────────────────────────────────────────────────────────────────

  [Town → Rig (active)]
  ────────────────────────────────────────────────────────────────────────────
  town override                         │ rig override
  ────────────────────────────────────────────────────────────────────────────
  timeout = 600                         │ timeout = 900
  ...                                   │ ...
  ────────────────────────────────────────────────────────────────────────────
  ```
- [ ] If only one override exists, shows single diff (embedded → override)
- [ ] If formula not found anywhere, shows helpful error

### US-008: Implement gt formula reset command
**Description:** As a user, I want to run `gt formula reset <name>` to remove my local override and restore the embedded version.

**Acceptance Criteria:**
- [ ] Add `gt formula reset <name>` subcommand
- [ ] Default: removes from town level
- [ ] `--rig <name>` flag: removes from specific rig
- [ ] If no override exists, error: "No override found for '<name>'. Already using embedded version."
- [ ] On success, print: "Removed override. Now using embedded version."
- [ ] If both town and rig overrides exist and no flag specified, prompt or error with guidance

### US-009: Update gt formula list to show override status
**Description:** As a user, I want `gt formula list` to show which formulas have overrides so I can see at a glance what's customized.

**Acceptance Criteria:**
- [ ] `gt formula list` shows all embedded formulas
- [ ] Formulas with overrides marked with indicator
- [ ] Custom formulas (not in embedded) shown in separate section
- [ ] Output format:
  ```
  Embedded Formulas (32)
  ──────────────────────
    beads-release
    code-review
    design
    gastown-release
    mol-boot-triage
    ...
    shiny                    ◄ town override
    mol-polecat-work         ◄ rig override (gastown)

  Custom Formulas (1)
  ───────────────────
    custom-workflow          (rig: myproject)

  Run 'gt formula diff' to see differences.
  Run 'gt formula modify <name>' to customize a formula.
  ```

### US-010: Add gt doctor check for legacy provisioned formulas
**Description:** As a user upgrading from a previous version, I want `gt doctor` to detect and offer to clean up unmodified provisioned formulas.

**Acceptance Criteria:**
- [ ] `gt doctor` checks for formulas in `.beads/formulas/` that match embedded exactly
- [ ] Reports: "Found N legacy provisioned formulas that match embedded versions"
- [ ] `gt doctor --fix` removes unmodified formulas (keeps modified ones as overrides)
- [ ] Comparison uses content hash, not filename
- [ ] Lists which formulas were removed vs kept
- [ ] Does NOT remove formulas that differ from embedded (those are intentional overrides)

### US-011: Agent-assisted formula update for overrides
**Description:** As a user with customized formulas, I want to be notified when embedded versions are updated and offered an agent-assisted merge so that I can incorporate upstream improvements while preserving my customizations.

**Acceptance Criteria:**
- [ ] `gt formula diff` detects when override's base version differs from current embedded
- [ ] When update available, show notification: "Embedded version has been updated since you created this override"
- [ ] Add `gt formula update <name>` command that:
  - [ ] Detects user's configured agent (claude, opencode, etc.) from rig config
  - [ ] Invokes agent as one-shot command (e.g., `claude -p` or `opencode --run`)
  - [ ] Provides prompt with: old embedded, new embedded, user's override
  - [ ] Agent generates merged version preserving user customizations
  - [ ] Outputs proposed merge to stdout (user can review before applying)
- [ ] Add `--apply` flag to write merged result directly to override file
- [ ] `--apply` creates backup file (`.formula.toml.bak`) before overwriting
- [ ] Track which embedded version an override was based on (store hash in comment or sidecar)
- [ ] Output format:
  ```
  $ gt formula update mol-polecat-work
  
  Checking for updates to mol-polecat-work...
  
  Your override: ~/gt/.beads/formulas/mol-polecat-work.formula.toml
  Based on:      embedded v4 (hash: abc123)
  Current:       embedded v5 (hash: def456)
  
  Invoking claude to merge changes...
  
  ═══════════════════════════════════════════════════════════
  PROPOSED MERGE
  ═══════════════════════════════════════════════════════════
  
  [merged formula content here]
  
  ═══════════════════════════════════════════════════════════
  
  Review the proposed merge above.
  Run 'gt formula update mol-polecat-work --apply' to apply it.
  ```
- [ ] Remove `.installed.json` complexity (replace with simpler base-version tracking)
- [ ] Simplify `FormulaStatus` to: `current`, `update-available`, `custom` (not in embedded)

### US-012: Update documentation for formula override system
**Description:** As a user or contributor, I want all formula-related documentation to reflect the new override system so that I can understand and use the new commands.

**Acceptance Criteria:**
- [ ] README.md "Beads Formula Workflow" section updated to show `gt formula` commands instead of `bd formula` for user-facing operations
- [ ] README.md "Beads Integration" section updated with new formula commands (`modify`, `diff`, `reset`, `update`)
- [ ] README.md "Cooking Formulas" section updated to reference embedded formulas instead of `.beads/formulas/`
- [ ] `docs/formula-resolution.md` updated to reflect new resolution architecture (embedded as source of truth, override system, `gt formula modify/diff/reset/update` commands)
- [ ] `docs/reference.md` Formula Format section updated with `gt formula` commands and override workflow
- [ ] `docs/concepts/molecules.md` updated: remove references to reading from `.beads/formulas/` directly
- [ ] `internal/formula/README.md` updated: replace `ProvisionFormulas`/`CheckFormulaHealth`/`UpdateFormulas` API docs with new embedded API (`GetEmbeddedFormula`, `GetEmbeddedFormulaNames`, `CopyFormulaTo`, `GetEmbeddedFormulaHash`, `ExtractBaseHash`)
- [ ] `internal/formula/doc.go` updated if needed with new API references
- [ ] `.github/workflows/ci.yml` updated: remove "Verify formulas are in sync" step that checks `.beads/formulas/` vs `internal/formula/formulas/` (no longer relevant since `go:generate` is removed)
- [ ] All docs consistently describe the resolution order: rig → town → embedded

## Functional Requirements

- FR-1: `internal/formula/formulas/` is the single source of truth for embedded formulas
- FR-2: Formulas resolve in order: rig → town → embedded (most specific wins)
- FR-3: `gt formula modify` copies embedded formula to local path for customization
- FR-4: `gt formula diff` shows all overrides with visual hierarchy map
- FR-5: `gt formula diff <name>` shows side-by-side diff between resolution levels
- FR-6: `gt formula reset` removes local override, restoring embedded version
- FR-7: `gt formula list` indicates which formulas have overrides
- FR-8: `gt doctor --fix` cleans up legacy unmodified provisioned formulas
- FR-9: `gt install` does not provision any formulas to `.beads/formulas/`
- FR-10: Users can commit `.beads/formulas/` to their own projects (not affected by gastown's gitignore)
- FR-11: `gt formula modify` records the embedded version hash the override was based on
- FR-12: `gt formula update` detects when embedded has changed and offers agent-assisted merge
- FR-13: Agent invocation uses rig's configured default agent (claude, opencode, etc.)

## Technical Considerations

### Git Tracking Clarification

The `.gitignore` changes only affect the **gastown repository itself**, not user projects:

| Location | Tracking | Notes |
|----------|----------|-------|
| gastown repo `.beads/formulas/` | Remove from tracking | Source of truth is now `internal/formula/formulas/` |
| User's project `.beads/formulas/` | User's choice | Their `.gitignore`, can track if they want |
| User's town `.beads/formulas/` | User's choice | `gt git-init` doesn't ignore formulas |

### Migration for Existing Users

Users upgrading from previous versions may have:
1. **Provisioned formulas from old `gt install`**: These become redundant. `gt doctor --fix` cleans up unmodified ones.
2. **Intentionally modified formulas**: These are preserved as overrides (they differ from embedded).
3. **gastown clone with old tracked `.beads/formulas/`**: These files are removed from tracking in the new version.

### Base Version Tracking

When `gt formula modify` creates an override, it records the embedded version hash:

```toml
# Formula override created by gt formula modify
# Based on embedded version: sha256:abc123def456...
# To update: gt formula update <name>

formula = "mol-polecat-work"
...
```

This allows `gt formula update` to:
1. Detect if embedded version has changed since override was created
2. Provide three-way merge context to the agent (old embedded, new embedded, user's changes)

### Agent Invocation for Merge

The `gt formula update` command detects the configured agent from:
1. Rig config (`config.json` → `default_agent`)
2. Environment (`$GT_DEFAULT_AGENT`)
3. Fallback to `claude` if available

Invocation pattern:
```bash
# Claude
claude -p "Merge these formula versions: [prompt with context]"

# OpenCode  
opencode --run "Merge these formula versions: [prompt with context]"
```

### File Changes Summary

| File | Change |
|------|--------|
| `internal/formula/embed.go` | Remove go:generate, add GetEmbeddedFormula(), simplify tracking |
| `internal/cmd/formula.go` | Add modify/diff/reset/update commands, update findFormulaFile() |
| `internal/cmd/install.go` | Remove ProvisionFormulas() call |
| `internal/doctor/formula_check.go` | Simplify to legacy cleanup + update-available check |
| `.gitignore` | Remove erroneous `.beads/` line, add `.beads/formulas/` |
| `.beads/formulas/*` | Remove from git tracking |

## Success Metrics

- Zero formula sync bugs reported after release
- `make build` produces no formula-related diffs
- All existing formula commands (`run`, `show`, `list`) work with embedded fallback
- Users can customize formulas with clear `modify`/`diff`/`reset`/`update` workflow
- `gt doctor --fix` successfully cleans up legacy provisioned formulas
- Agent-assisted merge produces usable merged formulas

## Open Questions

1. Should `gt formula diff` use colors/highlighting for the side-by-side diff? (Likely yes, but need to handle non-TTY output)
2. Should we add `--json` output for `gt formula diff` for scripting? (Probably future work)
3. Should `gt formula modify` support `--force` to overwrite existing override? (Probably yes for convenience)
4. What prompt structure works best for agent-assisted formula merging? (Need to experiment)
5. ~~Should `gt formula update --apply` create a backup of the old override?~~ **Yes** - create `.formula.toml.bak` before overwriting

[/PRD]
