# Scheduler Resource Classes

**Problem:** The scheduler limits concurrent polecats but treats all tasks equally. A polecat running `go test ./...` uses 10x more CPU than one editing a file. Result: CPU overload when multiple expensive tasks run in parallel.

**Solution:** Classify tasks by CPU cost and enforce per-class limits.

---

## Resource Classes

| Class | Examples | CPU | Memory | Typical Duration |
|-------|----------|-----|--------|------------------|
| `expensive` | Tests, builds, typecheck | High | High | 30s-5m |
| `moderate` | Lint, setup (npm install) | Medium | Medium | 10s-60s |
| `cheap` | Edit files, review, commit | Low | Low | <10s |

---

## Configuration

**Town-wide** (`mayor/daemon.json` or `settings/scheduler.json`):

```json
{
  "scheduler": {
    "max_polecats": 5,
    "resource_limits": {
      "expensive": 2,
      "moderate": 3,
      "cheap": 5
    }
  }
}
```

**Rules:**
- `max_polecats` = total concurrent polecats (hard ceiling)
- `resource_limits.*` = per-class limits (soft, sum can exceed max_polecats)
- Dispatcher checks BOTH: total slots AND class slot available

---

## Formula Declaration

Formulas declare their default resource class:

```toml
formula = "mol-polecat-work"
version = 5
resource_class = "cheap"  # Default for this formula

[vars]
[vars.resource_class]
description = "CPU resource class: expensive, moderate, cheap"
default = "cheap"
```

**Step-level override:** Individual steps can override:

```toml
[[steps]]
id = "run-tests"
title = "Run quality checks"
resource_class = "expensive"  # This step is expensive
```

---

## Dispatcher Logic

```go
type ResourceClass string
const (
    ClassExpensive ResourceClass = "expensive"
    ClassModerate  ResourceClass = "moderate"
    ClassCheap     ResourceClass = "cheap"
)

type ResourceLimits struct {
    Expensive int `json:"expensive"`
    Moderate  int `json:"moderate"`
    Cheap     int `json:"cheap"`
}

// Before dispatch:
func (d *Dispatcher) CanDispatch(class ResourceClass) bool {
    total := d.activePolecats()
    classCount := d.activeByClass[class]
    
    if total >= d.config.MaxPolecats {
        return false  // Hard ceiling
    }
    if classCount >= d.config.ResourceLimits[class] {
        return false  // Class limit reached
    }
    return true
}
```

---

## Bead Metadata

When slinging, capture resource class:

```go
type SlingParams struct {
    BeadID      string
    FormulaName string
    RigName     string
    ResourceClass ResourceClass  // NEW: captured from formula default
    // ...
}
```

**Inheritance:**
1. Explicit `--resource-class=expensive` flag (if provided)
2. Formula's `resource_class` field
3. Default: `cheap`

---

## Benefits

1. **CPU Control:** Never more than N expensive tasks running
2. **Fairness:** Cheap tasks never blocked by expensive queue
3. **Configurable:** Tune limits per-host capacity
4. **Backward Compatible:** Default `cheap` means existing formulas work unchanged

---

## Implementation Plan

### Phase 1: Config + Types
- [ ] Add `ResourceLimits` to `SchedulerConfig`
- [ ] Add `ResourceClass` type
- [ ] Add `resource_class` to `SlingParams`

### Phase 2: Dispatcher
- [ ] Track active polecats by class
- [ ] Add `CanDispatch(class)` check
- [ ] Update dispatch loop to check class limits

### Phase 3: Formula Integration
- [ ] Add `resource_class` field to formula schema
- [ ] Update `mol-polecat-work` to declare `cheap`
- [ ] Update `mol-refinery-patrol` to declare `expensive` (runs tests)

### Phase 4: Verification
- [ ] Test: 10 cheap tasks dispatch even with 2 expensive running
- [ ] Test: expensive tasks queue when limit reached
- [ ] Monitor: CPU usage stays within bounds

---

## Related

- [capacity/dispatch.go](../internal/scheduler/capacity/dispatch.go)
- [capacity/config.go](../internal/scheduler/capacity/config.go)
- [mol-polecat-work.formula.toml](../internal/formula/formulas/mol-polecat-work.formula.toml)
