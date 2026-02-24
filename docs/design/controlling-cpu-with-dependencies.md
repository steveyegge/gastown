# Controlling CPU Usage with Dependencies and Formulas

**Problem:** Multiple expensive tasks (tests, builds) run in parallel, overloading CPU.

**Solution:** Use task dependencies and convoy waves — NOT scheduler complexity.

---

## The Pattern

### 1. Polecats Don't Run Tests (Already Fixed)

`mol-polecat-work` formula:
- `test_command` = empty (polecat doesn't test)
- `build_command` = empty
- Polecats: implement → commit → submit
- **Refinery** runs tests after merge

### 2. Expensive Work Uses Dependencies

When filing expensive tasks, declare dependencies:

```bash
# Two expensive tasks that shouldn't run together
bd create "Run full test suite" --type=task
bd create "Build all binaries" --type=task

# Make them block each other
bd dep add gt-abc gt-def --type=blocks
```

Result: Only one runs at a time.

### 3. Convoy Waves Serialize Expensive Work

When staging a convoy, expensive tasks go in separate waves:

```
Convoy: "Release v2.0"

Wave 1 (parallel: 5):
  - Update docs
  - Fix typos
  - Review code
  - ...

Wave 2 (parallel: 1):  ← Only 1 expensive task at a time
  - Run full test suite

Wave 3 (parallel: 5):
  - Build artifacts
  - Deploy staging
  - ...
```

---

## Formula Enhancement: `parallel` Flag

Add a simple field to formula steps:

```toml
[[steps]]
id = "implement"
title = "Implement the solution"
parallel = true  # Multiple polecats can run this step

[[steps]]
id = "run-tests"
title = "Run quality checks"
parallel = false  # Only one polecat runs tests at a time
```

**Convoy stager reads this:**
- Groups `parallel = false` steps into separate waves
- Ensures only one executes at a time

---

## Implementation

### Phase 1: Document the Pattern
- [ ] Add docs/design/controlling-cpu-with-dependencies.md (this file)
- [ ] Add examples to formula docs
- [ ] Update convoy staging docs

### Phase 2: Add `parallel` Flag to Formulas
- [ ] Add `parallel` field to formula step schema
- [ ] Default: `true` (backward compatible)
- [ ] Update `mol-polecat-work` to mark test steps as `parallel = false`

### Phase 3: Convoy Stager Respects `parallel`
- [ ] Read `parallel` flag from formula
- [ ] Group non-parallel steps into separate waves
- [ ] Test: expensive tasks never run in parallel

---

## Benefits

1. **Simple:** No scheduler changes, no new config
2. **Explicit:** Dependencies are visible in `bd list`
3. **Flexible:** Per-task control, not global limits
4. **Backward Compatible:** Default `parallel = true` means existing formulas work unchanged

---

## Example: Test Suite Convoy

```bash
# Create convoy for release testing
gt convoy create "Release v2.0 Testing"

# Add cheap tasks (can run in parallel)
bd create "Review changelog" --type=task
bd create "Update version numbers" --type=task

# Add expensive tasks (must run serially)
bd create "Run unit tests" --type=task
bd create "Run integration tests" --type=task
bd create "Run E2E tests" --type=task

# Make expensive tasks block each other
bd dep add gt-unit gt-integration --type=blocks
bd dep add gt-integration gt-e2e --type=blocks

# Stage convoy - stager creates waves automatically
gt convoy stage <convoy-id>

# Result:
# Wave 1: Review changelog, Update versions (parallel: 2)
# Wave 2: Run unit tests (parallel: 1)
# Wave 3: Run integration tests (parallel: 1)
# Wave 4: Run E2E tests (parallel: 1)
```

---

## Related

- [convoy-lifecycle.md](convoy-lifecycle.md)
- [formula-resolution.md](formula-resolution.md)
- [mol-polecat-work.formula.toml](../internal/formula/formulas/mol-polecat-work.formula.toml)
