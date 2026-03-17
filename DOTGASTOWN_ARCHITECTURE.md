# .gastown Layout: Separating Infrastructure from Code

## Problem

Gas Town infrastructure files live alongside user code in the rig directory tree.
A rig at `~/gt/myproject/` mixes agent directories (`polecats/`, `crew/`, `witness/`,
`refinery/`, `artisans/`, `conductor/`, `architect/`), runtime state (`.runtime/`,
`.beads/`, `.repo.git/`), and configuration (`config.json`, `settings/`, `roles/`)
with the actual project code (inside `mayor/rig/`, `crew/<name>/`, etc.).

This makes it hard to:
- Know what's "your code" vs. "gastown infrastructure" at a glance
- Add gastown to an existing project without restructuring
- Reason about what's safe to delete, back up, or ignore
- Keep `.gitignore` clean (currently needs ~20 patterns)

## Proposal

Move all gastown infrastructure into a single `.gastown/` directory, either:
- **Per-project**: `myproject/.gastown/` (recommended)
- **Global**: `~/.gastown/` (alternative)

The project root IS the code. One `.gitignore` entry (`.gastown/`) hides everything.

## Current Layout vs. Proposed Layout

### Current (rig = container)
```
~/gt/myrig/                        # rig root (NOT the code)
├── config.json                    # rig identity
├── .beads/                        # beads DB redirect
├── .repo.git/                     # shared bare repo
├── .runtime/                      # locks, ephemeral state
│   ├── locks/
│   ├── overlay/
│   └── namepool-state.json
├── settings/
│   └── config.json
├── roles/
│   └── witness.toml               # role overrides
├── mayor/rig/                     # canonical clone (actual code)
│   ├── .beads/
│   ├── src/
│   └── go.mod
├── refinery/rig/                  # merge worktree
├── witness/                       # no clone, just state
├── crew/
│   └── mel/                       # full clone
├── polecats/
│   └── nux/                       # worktree
├── artisans/
│   ├── frontend-1/
│   └── backend-1/
├── conductor/
│   ├── specialties.toml
│   └── features/
└── architect/
```

### Proposed (project root = code)
```
~/projects/myproject/              # project root IS the code
├── .gastown/                      # ALL gastown infrastructure
│   ├── config.json                # rig identity
│   ├── beads/                     # beads DB (no leading dot needed)
│   │   └── redirect
│   ├── repo.git/                  # shared bare repo
│   ├── runtime/
│   │   ├── locks/
│   │   ├── overlay/
│   │   └── namepool-state.json
│   ├── settings/
│   │   └── config.json
│   ├── roles/
│   │   └── witness.toml
│   ├── agents/
│   │   ├── witness/
│   │   ├── refinery/              # worktree at agents/refinery/clone/
│   │   │   └── clone/
│   │   ├── conductor/
│   │   │   ├── specialties.toml
│   │   │   └── features/
│   │   ├── architect/
│   │   ├── artisans/
│   │   │   ├── frontend-1/
│   │   │   └── backend-1/
│   │   ├── crew/
│   │   │   └── mel/               # full clone
│   │   └── polecats/
│   │       └── nux/               # worktree
│   └── clones/                    # (alternative: clones at top level)
├── src/                           # actual project code
├── internal/
├── go.mod
└── .gitignore                     # just needs: .gastown/
```

### Key differences

| Aspect | Current | Proposed |
|--------|---------|----------|
| Project code location | `mayor/rig/` subdirectory | Project root itself |
| Infrastructure visibility | 10+ dirs at rig root | Single `.gastown/` dir |
| `.gitignore` entries | ~20 patterns | 1 pattern: `.gastown/` |
| Adding to existing project | Must restructure into rig | `gt init` in project root |
| `ls` in project root | Agent dirs mixed with nothing | Clean code + `.gastown/` |

## The Canonical Clone Problem

The biggest design question: where does the "real" code live?

**Current**: The canonical clone is `mayor/rig/`. The rig root is a container, not code.
Crew and polecats get separate clones/worktrees inside the rig.

**Proposed**: The project root IS the canonical clone. The user works directly in their
project directory. Crew, polecats, artisans get clones/worktrees inside `.gastown/agents/`.

This means:
- `git status` in the project root shows the user's code changes (not gastown's)
- The user's existing repo structure is untouched
- `gt init` can be run in any existing git repo
- The bare repo (`.gastown/repo.git/`) is created from the project's `.git/`

## RigLayout Interface

To migrate incrementally, introduce an abstraction over path resolution:

```go
// RigLayout encapsulates all rig directory conventions.
// Two implementations: ClassicLayout (current) and DotfileLayout (new).
type RigLayout interface {
    // Root paths
    RigRoot() string              // container root (classic) or project root (dotfile)
    InfraRoot() string            // rig root (classic) or .gastown/ (dotfile)

    // Agent home directories
    WitnessDir() string
    RefineryDir() string
    ConductorDir() string
    ArchitectDir() string
    ArtisanDir(name string) string
    CrewDir(name string) string
    PolecatDir(name string) string

    // Code checkouts
    CanonicalClone() string            // mayor/rig (classic) or project root (dotfile)
    CrewClone(name string) string
    PolecatWorktree(name string) string
    RefineryWorktree() string

    // Git infrastructure
    BareRepo() string

    // Beads
    BeadsDir() string

    // Runtime
    RuntimeDir() string
    LocksDir() string
    OverlayDir() string

    // Configuration
    ConfigFile() string
    SettingsDir() string
    RolesDir() string
    SpecialtiesFile() string

    // Agent settings
    AgentClaudeDir(role, name string) string
}
```

### ClassicLayout (current behavior)

```go
type ClassicLayout struct {
    rigPath string
    rigName string
}

func (l *ClassicLayout) InfraRoot() string            { return l.rigPath }
func (l *ClassicLayout) CanonicalClone() string        { return filepath.Join(l.rigPath, "mayor", "rig") }
func (l *ClassicLayout) ArtisanDir(name string) string { return filepath.Join(l.rigPath, "artisans", name) }
func (l *ClassicLayout) BareRepo() string              { return filepath.Join(l.rigPath, ".repo.git") }
func (l *ClassicLayout) RuntimeDir() string            { return filepath.Join(l.rigPath, ".runtime") }
func (l *ClassicLayout) ConfigFile() string            { return filepath.Join(l.rigPath, "config.json") }
// ... etc
```

### DotfileLayout (new behavior)

```go
type DotfileLayout struct {
    projectRoot string
    rigName     string
}

func (l *DotfileLayout) InfraRoot() string            { return filepath.Join(l.projectRoot, ".gastown") }
func (l *DotfileLayout) CanonicalClone() string        { return l.projectRoot }
func (l *DotfileLayout) ArtisanDir(name string) string { return filepath.Join(l.InfraRoot(), "agents", "artisans", name) }
func (l *DotfileLayout) BareRepo() string              { return filepath.Join(l.InfraRoot(), "repo.git") }
func (l *DotfileLayout) RuntimeDir() string            { return filepath.Join(l.InfraRoot(), "runtime") }
func (l *DotfileLayout) ConfigFile() string            { return filepath.Join(l.InfraRoot(), "config.json") }
// ... etc
```

### Layout Detection

```go
func DetectLayout(startPath string) (RigLayout, error) {
    // Walk up from startPath looking for markers:
    // 1. .gastown/config.json → DotfileLayout (project root = parent of .gastown/)
    // 2. mayor/town.json → ClassicLayout (rig root = parent dirs contain mayor/)
    // 3. config.json + mayor/rig/ → ClassicLayout (we're at rig root)
}
```

## Town-Level Considerations

The town root (`~/gt/`) also has infrastructure: `mayor/`, `deacon/`, `daemon/`,
`.beads/`, `.runtime/`, `settings/`. Options:

**Option A: Town stays classic, rigs go dotfile**
- Town root remains `~/gt/` with current structure
- Individual rigs use `.gastown/` inside their project dirs
- Simplest migration path — town infra is already separate from code

**Option B: Global `~/.gastown/` for town, per-project `.gastown/` for rigs**
- `~/.gastown/town.json` replaces `~/gt/mayor/town.json`
- `~/.gastown/agents/mayor/`, `~/.gastown/agents/deacon/`
- Projects register with: `gt init --town ~/.gastown`

**Recommendation: Option A first.** The town root is already a dedicated directory.
The pain point is rigs, where code and infrastructure mix. Solve that first.

## Migration Strategy

### Phase 1: RigLayout Interface (non-breaking)

Create the `RigLayout` interface and `ClassicLayout` implementation.
Migrate our new code (artisan, conductor, architect) to use it.
No behavior change — just indirection.

**Files to create:**
| File | Purpose |
|------|---------|
| `internal/rig/layout.go` | Interface definition |
| `internal/rig/layout_classic.go` | Current behavior implementation |
| `internal/rig/layout_test.go` | Tests for both layouts |

**Files to migrate (our new code — ~10 call sites):**
| File | Current | After |
|------|---------|-------|
| `internal/artisan/manager.go` | `filepath.Join(m.rigPath, "artisans", name)` | `m.layout.ArtisanDir(name)` |
| `internal/conductor/state.go` | `filepath.Join(s.rigPath, "conductor", "features")` | `s.layout.ConductorFeaturesDir()` |
| `internal/config/specialty.go` | `filepath.Join(rigPath, "conductor", "specialties.toml")` | `layout.SpecialtiesFile()` |
| `internal/cmd/artisan_add.go` | `fmt.Sprintf("%s/%s", townRoot, rigName)` | `layout.RigRoot()` |
| `internal/cmd/artisan_list.go` | `fmt.Sprintf("%s/%s", townRoot, rigName)` | `layout.RigRoot()` |
| `internal/cmd/artisan_remove.go` | `fmt.Sprintf("%s/%s", townRoot, rigName)` | `layout.RigRoot()` |
| `internal/cmd/artisan_specialties.go` | `fmt.Sprintf("%s/%s", townRoot, rigName)` | `layout.RigRoot()` |

### Phase 2: DotfileLayout Implementation

Add `DotfileLayout` and `DetectLayout()`. Both layouts coexist.
New rigs can opt into dotfile layout. Existing rigs continue working.

**New command:**
```
gt init                      # initialize .gastown/ in current project
gt init --layout classic     # use traditional rig layout
```

### Phase 3: Migrate Existing Code (gradual)

Replace `filepath.Join` calls throughout the existing codebase with layout methods.
This is the long tail — ~50+ call sites across polecat, crew, witness, refinery,
rig manager, config loader, etc.

**Priority order:**
1. `internal/artisan/` (our code — Phase 1)
2. `internal/conductor/` (our code — Phase 1)
3. `internal/config/specialty.go` (our code — Phase 1)
4. `internal/rig/manager.go` (creates directories — high impact)
5. `internal/polecat/manager.go` (polecat paths)
6. `internal/crew/manager.go` (crew paths)
7. `internal/witness/manager.go` (witness paths)
8. `internal/config/loader.go` (settings paths)
9. `internal/cmd/sling*.go` (dispatch paths)
10. Everything else

### Phase 4: Workspace Detection Update

Update `workspace.FindFromCwd()` to detect `.gastown/config.json` as an
alternative marker to `mayor/town.json`.

### Phase 5: `gt init` for Existing Repos

New workflow:
```bash
cd ~/projects/myapp
gt init                           # creates .gastown/, registers with town
echo ".gastown/" >> .gitignore
gt artisan add frontend-1 --specialty frontend
gt sling gt-abc --artisan frontend-1
```

## Scope of Change

| Category | Call sites | Difficulty |
|----------|-----------|------------|
| Our new code (artisan/conductor/architect) | ~10 | Easy |
| Rig manager (AddRig, directory creation) | ~15 | Medium |
| Polecat manager (spawn, worktree, cleanup) | ~12 | Medium |
| Crew manager (add, clone, state) | ~8 | Easy |
| Config loader (settings, roles, overlays) | ~10 | Medium |
| Witness/refinery (monitoring, merge paths) | ~8 | Medium |
| Sling/dispatch (target resolution) | ~6 | Easy |
| Beads routing (redirect resolution) | ~5 | Hard (many edge cases) |
| Workspace detection | ~3 | Medium |
| **Total** | **~77** | |

## Non-Goals

- Changing the beads database format or Dolt storage
- Changing the mail system
- Changing tmux session naming
- Changing the role definition TOML system
- Breaking backward compatibility with existing towns

## Open Questions

1. **Clone location**: Should crew/artisan full clones live inside `.gastown/agents/crew/<name>/`
   or alongside the project (e.g., `../myproject-crew-mel/`)? Inside `.gastown/` keeps
   things tidy but means a longer path for users who `cd` into crew workspaces.

2. **Worktree anchoring**: Git worktrees need a `.git` file pointing to the bare repo.
   If the project root IS the canonical clone, the bare repo at `.gastown/repo.git/`
   needs to be created FROM the project's existing `.git/`. Doable but needs care.

3. **Multi-rig towns**: If one project can belong to multiple rigs, or one town
   has multiple projects, does each project get its own `.gastown/`? Yes — each
   `.gastown/config.json` references the town and rig name.

4. **Upstream acceptance**: This is a significant structural change. If proposed to
   steveyegge/gastown, it would need buy-in as an RFC before implementation.
   The interface-first approach (Phase 1-2) is low-risk and could be proposed
   independently.
