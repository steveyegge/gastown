# Formula Resolution Architecture

> Where formulas live, how they're found, and how they'll scale to Mol Mall

## The Problem (Solved)

Previously, formulas existed in multiple locations with no clear precedence:
- `.beads/formulas/` (was source of truth, synced via `go:generate`)
- `internal/formula/formulas/` (embedded copy for `go install`)
- Crew directories had their own `.beads/formulas/` (diverging copies)

This caused sync bugs where PRs edited `internal/` directly but `go generate` overwrote with old `.beads/` versions.

**The solution**: `internal/formula/formulas/` is now the single source of truth. Formulas are embedded in the binary and users override them on demand via `gt formula modify`. There is no more `go:generate` from `.beads/formulas/`, and no provisioning during `gt install`.

## Design Goals

1. **Predictable resolution** - Clear precedence rules
2. **Local customization** - Override system defaults without forking
3. **Project-specific formulas** - Committed workflows for collaborators
4. **Mol Mall ready** - Architecture supports remote formula installation
5. **Federation ready** - Formulas are shareable across towns via HOP (Highway Operations Protocol)

## Three-Tier Resolution

```
┌─────────────────────────────────────────────────────────────────┐
│                     FORMULA RESOLUTION ORDER                     │
│                    (most specific wins)                          │
└─────────────────────────────────────────────────────────────────┘

TIER 1: PROJECT (rig-level)
  Location: <project>/.beads/formulas/
  Source:   Committed to project repo
  Use case: Project-specific workflows (deploy, test, release)
  Example:  ~/gt/gastown/.beads/formulas/mol-gastown-release.formula.toml

TIER 2: TOWN (user-level)
  Location: ~/gt/.beads/formulas/
  Source:   Mol Mall installs, user customizations
  Use case: Cross-project workflows, personal preferences
  Example:  ~/gt/.beads/formulas/mol-polecat-work.formula.toml (customized)

TIER 3: SYSTEM (embedded)
  Location: Compiled into gt binary
  Source:   internal/formula/formulas/ (single source of truth in gastown repo)
  Use case: Defaults, blessed patterns, fallback
  Example:  mol-polecat-work.formula.toml (factory default)
```

### Resolution Algorithm

```go
func ResolveFormula(name string, cwd string) (Formula, Tier, error) {
    // Tier 1: Project-level (walk up from cwd to find .beads/formulas/)
    if projectDir := findProjectRoot(cwd); projectDir != "" {
        path := filepath.Join(projectDir, ".beads", "formulas", name+".formula.toml")
        if f, err := loadFormula(path); err == nil {
            return f, TierProject, nil
        }
    }

    // Tier 2: Town-level
    townDir := getTownRoot() // ~/gt or $GT_HOME
    path := filepath.Join(townDir, ".beads", "formulas", name+".formula.toml")
    if f, err := loadFormula(path); err == nil {
        return f, TierTown, nil
    }

    // Tier 3: Embedded (system)
    if f, err := loadEmbeddedFormula(name); err == nil {
        return f, TierSystem, nil
    }

    return nil, 0, ErrFormulaNotFound
}
```

### Why This Order

**Project wins** because:
- Project maintainers know their workflows best
- Collaborators get consistent behavior via git
- CI/CD uses the same formulas as developers

**Town is middle** because:
- User customizations override system defaults
- Mol Mall installs don't require project changes
- Cross-project consistency for the user

**System is fallback** because:
- Always available (compiled in)
- Factory reset target
- The "blessed" versions

## Formula Identity

### Current Format

```toml
formula = "mol-polecat-work"
version = 4
description = "..."
```

### Extended Format (Mol Mall Ready)

```toml
[formula]
name = "mol-polecat-work"
version = "4.0.0"                          # Semver
author = "steve@gastown.io"                # Author identity
license = "MIT"
repository = "https://github.com/steveyegge/gastown"

[formula.registry]
uri = "hop://molmall.gastown.io/formulas/mol-polecat-work@4.0.0"
checksum = "sha256:abc123..."              # Integrity verification
signed_by = "steve@gastown.io"             # Optional signing

[formula.capabilities]
# What capabilities does this formula exercise? Used for agent routing.
primary = ["go", "testing", "code-review"]
secondary = ["git", "ci-cd"]
```

### Version Resolution

When multiple versions exist:

```bash
bd cook mol-polecat-work          # Resolves per tier order
bd cook mol-polecat-work@4        # Specific major version
bd cook mol-polecat-work@4.0.0    # Exact version
bd cook mol-polecat-work@latest   # Explicit latest
```

## Crew Directory Problem

### Current State

Crew directories (`gastown/crew/max/`) are sparse checkouts of gastown. They have:
- Their own `.beads/formulas/` (from the checkout)
- These can diverge from `mayor/rig/.beads/formulas/`

### The Fix

Crew should NOT have their own formula copies. Options:

**Option A: Symlink/Redirect**
```bash
# crew/max/.beads/formulas -> ../../mayor/rig/.beads/formulas
```
All crew share the rig's formulas.

**Option B: Provision on Demand**
Crew directories don't have `.beads/formulas/`. Resolution falls through to:
1. Town-level (~/gt/.beads/formulas/)
2. System (embedded)

**Option C: Sparse Checkout Exclusion**
Exclude `.beads/formulas/` from crew sparse checkouts entirely.

**Recommendation: Option B** - Crew shouldn't need project-level formulas. They work on the project, they don't define its workflows.

## Commands

### Formula Management (gt)

```bash
gt formula list                  # Available formulas (with override indicators)
gt formula show <name>           # Formula details
gt formula modify <name>         # Copy embedded formula for customization
gt formula diff                  # Visual map of all overrides
gt formula diff <name>           # Detailed side-by-side diff
gt formula reset <name>          # Remove override, restore embedded
gt formula update <name>         # Agent-assisted merge after upgrade
gt formula run <name>            # Execute a formula
```

### Formula Data Operations (bd)

```bash
bd formula list                  # Available formulas
bd formula show <name>           # Formula details
bd cook <formula>                # Formula → Proto
```

### Enhanced List Output

```bash
gt formula list
  mol-polecat-work          v4    [project]  (override)
  mol-polecat-code-review   v1    [town]     (override)
  mol-witness-patrol        v2    [system]

gt formula diff
  mol-polecat-work          MODIFIED  (rig override)
  mol-polecat-code-review   MODIFIED  (town override)
  ... 28 formulas unchanged
```

### Future (Mol Mall)

```bash
# Install from Mol Mall
gt formula install mol-code-review-strict
gt formula install mol-code-review-strict@2.0.0
gt formula install hop://acme.corp/formulas/mol-deploy

# Manage installed formulas
gt formula list --installed              # What's in town-level
gt formula upgrade mol-polecat-work      # Update to latest
gt formula pin mol-polecat-work@4.0.0    # Lock version
gt formula uninstall mol-code-review-strict
```

## Migration Path

### Phase 1: Resolution Order (DONE)

1. ~~Implement three-tier resolution in `bd cook`~~ ✓
2. ~~Add `--resolve` flag to show resolution path~~ ✓
3. ~~Update `bd formula list` to show tiers~~ ✓
4. ~~Fix crew directories (Option B)~~ ✓

### Phase 2: Formula Override System (DONE)

1. ~~`internal/formula/formulas/` is now the single source of truth~~ ✓
2. ~~Removed `go:generate` from `.beads/formulas/`~~ ✓
3. ~~Added `gt formula` commands: `modify`, `diff`, `reset`, `update`, enhanced `list`~~ ✓
4. ~~Removed formula provisioning during `gt install`~~ ✓
5. ~~`gt doctor` detects legacy provisioned formulas~~ ✓

> **Current state**: Formulas are embedded in the `gt` binary from `internal/formula/formulas/`. Users customize via `gt formula modify`, view overrides via `gt formula diff`, and reset via `gt formula reset`. No sync or provisioning required.

### Phase 3: Mol Mall Integration

1. Define registry API (see mol-mall-design.md)
2. Implement `gt formula install` from remote
3. Add version pinning and upgrade flows
4. Add integrity verification (checksums, optional signing)

### Phase 4: Federation (HOP)

1. Add capability tags to formula schema
2. Track formula execution for agent accountability
3. Enable federation (cross-town formula sharing via Highway Operations Protocol)
4. Author attribution and validation records

## Related Documents

- [Mol Mall Design](mol-mall-design.md) - Registry architecture
- [molecules.md](molecules.md) - Formula → Proto → Mol lifecycle
- [understanding-gas-town.md](../../../docs/understanding-gas-town.md) - Gas Town architecture
