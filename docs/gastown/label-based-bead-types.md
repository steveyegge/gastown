# Migration Plan: Label-Based Bead Types

**Wanted item:** w-gt-003
**Status:** Migration plan
**Date:** 2026-03-04

## 1. Overview

Gas Town currently uses a single `Type` string field on beads to classify
them (e.g., "issue", "convoy", "agent", "molecule", "epic"). This migration
replaces the singleton type with a label-based system where a bead can have
multiple labels, using the convention `gt:<type>` for type labels.

The migration is **already partially complete**. The create and list paths
in `internal/beads/beads.go` already convert `Type` to `gt:<type>` labels
(lines 729-731, 785-787, 523-528). However, many consumers still read the
legacy `.Type` field from beads JSON output. This document catalogs all
remaining migration sites and proposes a phased approach to complete the
migration.

## 2. Current State

### What's already migrated

| Component | File | Status |
|-----------|------|--------|
| Bead creation | `beads.go:729-731` | Converts `opts.Type` to `--labels=gt:<type>` |
| Bead creation (with ID) | `beads.go:785-787` | Same conversion |
| Bead listing | `beads.go:523-528` | Prefers `--label=gt:<type>` over `--type` |
| Agent detection | `beads.go:122-135` | `IsAgentBead()` checks both `gt:agent` label and legacy `.Type` |
| Web fetcher filtering | `web/fetcher.go:1543-1554` | Checks both `.Type` and `gt:` labels for internal types |
| Agent beads | `beads_agent.go:225` | Creates with `--labels=gt:agent` |
| Channel beads | `beads_channel.go:159` | Creates with `--labels=gt:channel` |
| Dog beads | `beads_dog.go:16-31` | Creates with labels for metadata |
| Escalation beads | `beads_escalation.go:177` | Creates with `--labels=gt:escalation` |
| Group beads | `beads_group.go:164` | Creates with `--labels=gt:group` |
| ReadyWithType | `beads.go:631-632` | Already queries by `--label gt:<type>` |

### What still uses legacy `.Type` field

These are the sites that still read `issue.Type` / `bead.Type` directly:

#### Critical path (type validation)

| File | Line | Code | Impact |
|------|------|------|--------|
| `convoy.go` | 624, 742, 840, 999 | `convoy.Type != "convoy"` | Blocks convoy operations if type not set |
| `convoy_launch.go` | 287 | `result.IssueType == "convoy"` | Convoy staging filter |
| `convoy_stage.go` | 113-121 | `classifyBeadType(beadType)` | Convoy staging classification |
| `molecule.go` | 471-472 | `mol.Type != "molecule"` | Molecule validation |
| `synthesis.go` | 400 | `convoys[0].Type != "convoy"` | Synthesis validation |
| `daemon/lifecycle.go` | 785 | `issue.Type == "agent"` | Agent lifecycle detection |

#### Display and reporting

| File | Line | Code | Impact |
|------|------|------|--------|
| `ui/styles.go` | 406-421 | `RenderType(issueType)` | CLI type colorization |
| `cmd/bead.go` | 145, 167 | `source.Type` display and copy | Bead info display |
| `cmd/audit.go` | 289 | `issue.Type` in details | Audit reporting |
| `cmd/config.go` | 310 | `item.Type` | Config display |
| `cmd/mq_status.go` | 82, 144-145 | `issue.Type` | MQ status display |
| `cmd/wl_post.go` | 104-105 | `item.Type` | Wasteland post display |
| `cmd/polecat_identity.go` | 747, 837-839 | `extractWorkType()` | Work type for polecat naming |

#### Logic and classification

| File | Line | Code | Impact |
|------|------|------|--------|
| `beads.go` | 130 | `issue.Type == "agent"` | Legacy agent check (inside `IsAgentBead`) |
| `beads_agent.go` | 607 | `issue.Type` in error message | Agent validation error |
| `integration.go` | 194 | `issue.Type == "epic"` | Epic detection |
| `agents.go` | 672 | `issue.Type == "collision"` | Collision detection |
| `convoy/operations.go` | 166-167, 445 | `IsSlingableType()`, `.Type` | Sling type classification |
| `mail/router.go` | 771 | `a.Type == "agent"` | Mail routing |

## 3. Label Convention

### Namespace prefixes

Labels use namespaced prefixes to distinguish categories:

| Prefix | Meaning | Examples |
|--------|---------|---------|
| `gt:` | Bead type (replaces `.Type`) | `gt:convoy`, `gt:agent`, `gt:epic` |
| `severity:` | Escalation severity | `severity:critical`, `severity:warning` |
| `internal:` | Internal system labels | `internal:pinned` |
| (none) | User-defined labels | `bug`, `frontend`, `urgent` |

### Type-to-label mapping

Every existing `.Type` value maps to a `gt:` label:

| Legacy Type | Label | Notes |
|-------------|-------|-------|
| `agent` | `gt:agent` | Already migrated in create path |
| `convoy` | `gt:convoy` | High-traffic; 4 validation sites |
| `epic` | `gt:epic` | Used in integration and staging |
| `task` | `gt:task` | Default type for new beads |
| `bug` | `gt:bug` | Display only (RenderType) |
| `feature` | `gt:feature` | Display only |
| `chore` | `gt:chore` | Display only |
| `molecule` | `gt:molecule` | Molecule validation |
| `message` | `gt:message` | Mail system, pinned bead check |
| `merge-request` | `gt:merge-request` | Refinery processing |
| `collision` | `gt:collision` | Agent collision detection |
| `channel` | `gt:channel` | Already migrated in create path |
| `escalation` | `gt:escalation` | Already migrated in create path |
| `group` | `gt:group` | Already migrated in create path |
| `wisp` | `gt:wisp` | Ephemeral beads |
| `queue` | `gt:queue` | Message queues |
| `slot` | `gt:slot` | Scheduling slots |
| `role` | `gt:role` | Agent role definitions |
| `rig` | `gt:rig` | Rig registrations |

### Multi-label benefit

With labels, a bead can have multiple type-like attributes:

```
# A convoy bead that is also marked as an epic
Labels: ["gt:convoy", "gt:epic", "team:platform", "priority:high"]

# An agent bead with role information
Labels: ["gt:agent", "role:polecat", "pool:default"]
```

This is impossible with a single `Type` field.

## 4. Migration Strategy

### Phase 1: Add helper functions (low risk)

Create a set of label-based type-checking helpers in `internal/beads/beads.go`
that check both the label and the legacy field:

```go
// IsConvoyBead checks if a bead is a convoy.
func IsConvoyBead(issue *Issue) bool {
    if issue == nil {
        return false
    }
    if issue.Type == "convoy" {
        return true
    }
    return HasLabel(issue, "gt:convoy")
}

// IsEpicBead checks if a bead is an epic.
func IsEpicBead(issue *Issue) bool {
    if issue == nil {
        return false
    }
    if issue.Type == "epic" {
        return true
    }
    return HasLabel(issue, "gt:epic")
}

// IsMoleculeBead checks if a bead is a molecule.
func IsMoleculeBead(issue *Issue) bool {
    if issue == nil {
        return false
    }
    if issue.Type == "molecule" {
        return true
    }
    return HasLabel(issue, "gt:molecule")
}

// BeadTypeLabel returns the gt:<type> label for a bead,
// derived from labels first, falling back to the legacy Type field.
func BeadTypeLabel(issue *Issue) string {
    if issue == nil {
        return ""
    }
    for _, l := range issue.Labels {
        if strings.HasPrefix(l, "gt:") {
            return l
        }
    }
    if issue.Type != "" {
        return "gt:" + issue.Type
    }
    return ""
}

// BeadType returns the type string for a bead (without gt: prefix),
// derived from labels first, falling back to the legacy Type field.
func BeadType(issue *Issue) string {
    label := BeadTypeLabel(issue)
    return strings.TrimPrefix(label, "gt:")
}
```

`IsAgentBead()` already follows this pattern (line 122-135). The new helpers
use the same dual-check approach.

### Phase 2: Replace direct `.Type` reads (medium risk)

Replace all `issue.Type == "X"` checks with the helper functions:

| Before | After |
|--------|-------|
| `convoy.Type != "convoy"` | `!IsConvoyBead(convoy)` |
| `issue.Type == "agent"` | `IsAgentBead(issue)` |
| `issue.Type == "epic"` | `IsEpicBead(issue)` |
| `mol.Type != "molecule"` | `!IsMoleculeBead(mol)` |
| `issue.Type == "collision"` | `HasLabel(issue, "gt:collision")` |
| `issue.Type` (display) | `BeadType(issue)` |

For `classifyBeadType` in `convoy_stage.go`:

```go
func classifyBeadType(issue *Issue) StageInputKind {
    if IsEpicBead(issue) {
        return StageInputEpic
    }
    if IsConvoyBead(issue) {
        return StageInputConvoy
    }
    return StageInputTasks
}
```

For `RenderType` in `ui/styles.go`:

```go
func RenderType(issue *Issue) string {
    t := BeadType(issue)
    switch t {
    case "bug":
        return TypeBugStyle.Render(t)
    // ... same cases
    }
}
```

**Note:** `RenderType` currently takes a `string` parameter. If changing the
signature is too disruptive, keep the string version and add
`RenderTypeFromBead(issue *Issue)` that calls `BeadType(issue)` then
delegates to `RenderType`.

### Phase 3: Update display and reporting (low risk)

For display-only sites (`bead.go`, `audit.go`, `config.go`, `mq_status.go`,
`wl_post.go`, `polecat_identity.go`), replace `issue.Type` reads with
`BeadType(issue)`. These are safe because they only affect output formatting.

### Phase 4: Remove the Type field (future, high risk)

Once all consumers use label-based helpers:

1. Stop populating `Issue.Type` in beads JSON parsing.
2. Populate it from labels during deserialization for backward compatibility:
   ```go
   func (i *Issue) UnmarshalJSON(data []byte) error {
       type Alias Issue
       var raw Alias
       if err := json.Unmarshal(data, &raw); err != nil {
           return err
       }
       *i = Issue(raw)
       // Back-fill Type from labels if empty
       if i.Type == "" {
           i.Type = BeadType(i)
       }
       return nil
   }
   ```
3. Eventually remove the `Type` field entirely once all callers use helpers.

This phase depends on the beads (`bd`) binary also supporting label-based
type queries. If `bd show --json` still returns a `type` field, the
back-fill approach keeps things working.

## 5. Specific Migration Checklist

### Phase 1 — Helpers (1 file, ~50 lines)

- [ ] Add `IsConvoyBead()`, `IsEpicBead()`, `IsMoleculeBead()`, `IsMessageBead()` to `internal/beads/beads.go`
- [ ] Add `BeadType()` and `BeadTypeLabel()` to `internal/beads/beads.go`
- [ ] Add unit tests for all helpers (both label and legacy paths)

### Phase 2 — Critical path (6 files, ~30 changes)

- [ ] `internal/cmd/convoy.go:624,742,840,999` — `!IsConvoyBead(convoy)`
- [ ] `internal/cmd/convoy_launch.go:287` — `IsConvoyBead(&result)` or label check
- [ ] `internal/cmd/convoy_stage.go:113-121` — Refactor `classifyBeadType`
- [ ] `internal/beads/molecule.go:471` — `!IsMoleculeBead(mol)`
- [ ] `internal/cmd/synthesis.go:400` — `!IsConvoyBead(&convoys[0])`
- [ ] `internal/daemon/lifecycle.go:785,793` — `IsAgentBead(issue)` (already exists)
- [ ] `internal/beads/beads_agent.go:607` — Use `IsAgentBead(issue)`
- [ ] `internal/beads/integration.go:194` — `IsEpicBead(issue)`
- [ ] `internal/cmd/agents.go:672` — `HasLabel(issue, "gt:collision")`
- [ ] `internal/convoy/operations.go:166-167,445` — Label-based sling classification
- [ ] `internal/mail/router.go:771` — `IsAgentBead(a)`

### Phase 3 — Display (6 files, ~15 changes)

- [ ] `internal/ui/styles.go:406-421` — Use `BeadType()` or keep string API
- [ ] `internal/cmd/bead.go:145,167` — `BeadType(source)`
- [ ] `internal/cmd/audit.go:289` — `BeadType(issue)`
- [ ] `internal/cmd/config.go:310` — `BeadType(item)` or label from item
- [ ] `internal/cmd/mq_status.go:82,144-145` — `BeadType(issue)`
- [ ] `internal/cmd/wl_post.go:104-105` — `BeadType(item)` or equivalent
- [ ] `internal/cmd/polecat_identity.go:747,837-839` — `BeadType(issue)`

### Phase 4 — Field removal (future)

- [ ] Add `UnmarshalJSON` back-fill for `Issue.Type`
- [ ] Deprecation warning on direct `.Type` access (go vet check or linter)
- [ ] Remove `Type` field from `Issue` struct
- [ ] Remove `Type` field from `CreateOptions`
- [ ] Remove `Type` from `ListOptions` (already has `Label`)
- [ ] Update beads custom types config if needed

## 6. Testing Strategy

### Unit tests for helpers

```go
func TestIsConvoyBead(t *testing.T) {
    tests := []struct {
        name   string
        issue  *Issue
        expect bool
    }{
        {"nil issue", nil, false},
        {"legacy type", &Issue{Type: "convoy"}, true},
        {"gt:convoy label", &Issue{Labels: []string{"gt:convoy"}}, true},
        {"both", &Issue{Type: "convoy", Labels: []string{"gt:convoy"}}, true},
        {"wrong type", &Issue{Type: "epic"}, false},
        {"wrong label", &Issue{Labels: []string{"gt:epic"}}, false},
        {"no type info", &Issue{}, false},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            if got := IsConvoyBead(tt.issue); got != tt.expect {
                t.Errorf("IsConvoyBead() = %v, want %v", got, tt.expect)
            }
        })
    }
}
```

### Integration tests

- Create a bead with `--labels=gt:convoy` (no `--type`), verify convoy
  operations work.
- Create a bead with legacy `--type=convoy`, verify convoy operations
  still work.
- Verify `RenderType` produces correct styling from labels.

### Regression safety

Each phase should be merged separately with its own test coverage. Phase 2
(critical path) should include tests for every convoy/molecule/synthesis
code path that previously relied on `.Type`.

## 7. Risk Assessment

| Phase | Risk | Mitigation |
|-------|------|------------|
| Phase 1 (helpers) | Very low | Additive only, no behavior change |
| Phase 2 (critical path) | Medium | Dual-check pattern means both old and new beads work |
| Phase 3 (display) | Low | Output-only changes, no logic impact |
| Phase 4 (field removal) | High | Must ensure beads CLI compatibility; defer until stable |

The dual-check pattern (check label first, fall back to `.Type`) ensures
backward compatibility throughout the migration. Beads created before the
migration will still have their `.Type` field populated by `bd`, while new
beads will have `gt:` labels. Both work at every stage.

## 8. Constants Update

The `BeadsCustomTypes` constant in `internal/constants/constants.go` lists
types that are registered with beads. After the migration, these become
label prefixes rather than type constants, but the registration mechanism
stays the same since `bd` needs to know about custom types for its schema.

No change needed to `BeadsCustomTypesList()` — the list of valid type names
remains the same, they're just stored as labels instead of a type field.
