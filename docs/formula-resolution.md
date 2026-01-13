# Formula Resolution Architecture

> Where formulas live, how they're found, and how they'll scale to Mol Mall

## The Problem

Formulas currently exist in multiple locations with no clear precedence:
- `.beads/formulas/` (source of truth for a project)
- `internal/formula/formulas/` (embedded copy for `go install`)
- Crew directories have their own `.beads/formulas/` (diverging copies)

When an agent runs `bd cook mol-polecat-work`, which version do they get?

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
  Source:   gastown/mayor/rig/.beads/formulas/ at build time
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

### Existing

```bash
bd formula list              # Available formulas (should show tier)
bd formula show <name>       # Formula details
bd cook <formula>            # Formula → Proto
```

### Enhanced

```bash
# List with tier information
bd formula list
  mol-polecat-work          v4    [project]
  mol-polecat-code-review   v1    [town]
  mol-witness-patrol        v2    [system]

# Show resolution path
bd formula show mol-polecat-work --resolve
  Resolving: mol-polecat-work
  ✓ Found at: ~/gt/gastown/.beads/formulas/mol-polecat-work.formula.toml
  Tier: project
  Version: 4

  Resolution path checked:
  1. [project] ~/gt/gastown/.beads/formulas/ ← FOUND
  2. [town]    ~/gt/.beads/formulas/
  3. [system]  <embedded>

# Override tier for testing
bd cook mol-polecat-work --tier=system    # Force embedded version
bd cook mol-polecat-work --tier=town      # Force town version
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

### Phase 1: Resolution Order (Now)

1. Implement three-tier resolution in `bd cook`
2. Add `--resolve` flag to show resolution path
3. Update `bd formula list` to show tiers
4. Fix crew directories (Option B)

### Phase 2: Town-Level Formulas

1. Establish `~/gt/.beads/formulas/` as town formula location
2. Add `gt formula` commands for managing town formulas
3. Support manual installation (copy file, track in `.installed.json`)

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
