# OpenCode Documentation Standards

> **Purpose**: Standards, review triggers, and validation criteria for OpenCode docs  
> **Used by**: `scripts/validate-opencode-docs.sh` and agents

---

## Directory Standards

| Directory | Purpose | Review Trigger |
|-----------|---------|----------------|
| `reference/` | Stable, evergreen docs | When OpenCode behavior changes |
| `design/` | Roadmaps, implementation strategies | When plans are updated or completed |
| `archive/` | Point-in-time snapshots | Rarely (historical record) |

---

## File Standards

### Required Frontmatter

All docs should have a header with:
```markdown
# Title

> **Purpose**: What this document is for  
> **Other metadata as needed**
```

### Link Format

All internal links should be relative:
```markdown
# ✅ Good
[events.md](reference/events.md)
[reference/plugin-implementation.md](reference/plugin-implementation.md)

# ❌ Bad  
[events.md](/absolute/path/to/events.md)
```


### Date Format

Use ISO 8601 in archive docs:
```markdown
> **Date**: 2026-01-17
```

---

## Review Triggers

### When Code Changes

| Code Path | Docs to Review |
|-----------|----------------|
| `internal/opencode/**` | `reference/integration-guide.md`, `reference/plugin-implementation.md` |
| `internal/opencode/plugin/**` | `reference/events.md`, `reference/plugin-implementation.md` |
| `internal/config/agents.go` (OpenCode section) | `reference/configuration.md`, `design/role-permissions.md` |

### When OpenCode Updates

| Change Type | Docs to Review |
|-------------|----------------|
| New event types | `reference/events.md` |
| New tools | `reference/tools.md` |
| Config schema changes | `reference/configuration.md`, `reference/customization.md` |
| Breaking changes | `reference/maintenance.md`, `HISTORY.md` |

### When Docs Change

| Doc Changed | Also Update |
|-------------|-------------|
| Any `design/**` file | `design/README.md` (inventory) |
| Any doc created/moved | `README.md` (directory structure) |
| Major changes | `HISTORY.md` |

---

## Validation Checks

The validation script checks:

### 1. Broken Links

All `[text](file.md)` links should resolve to existing files.

### 2. Missing README Updates

When files are added/removed in `reference/` or `design/`, their READMEs should be updated.

### 3. Stale Code References

Compare timestamps:
- If `internal/opencode/**` modified more recently than docs → flag for review

### 4. History Log

If significant doc changes exist, `HISTORY.md` should have a recent entry.

### 5. Design README Freshness

`design/README.md` should be updated when design docs change.

---

## Agent Review Checklist

When an agent modifies OpenCode integration code:

- [ ] Check if behavior changes affect docs in `reference/`
- [ ] If new features added, update relevant reference docs
- [ ] If config changes, update `configuration.md`
- [ ] If event/hook changes, update `events.md`
- [ ] Add entry to `HISTORY.md` with date and summary
- [ ] Run `scripts/validate-opencode-docs.sh` before commit

When an agent modifies docs:

- [ ] Update `design/README.md` if design docs changed
- [ ] Update `README.md` directory structure if files added/moved
- [ ] Add entry to `HISTORY.md`
- [ ] Check all internal links still work

---

## Running Validation

```bash
# Full validation
./scripts/validate-opencode-docs.sh

# Check specific aspects
./scripts/validate-opencode-docs.sh --links      # Broken links only
./scripts/validate-opencode-docs.sh --stale      # Stale docs only
./scripts/validate-opencode-docs.sh --history    # History check only
```

---

## Integration Points

### Pre-commit (optional)

Add to lefthook or git hooks:
```yaml
pre-commit:
  commands:
    opencode-docs:
      glob: "docs/opencode/**/*.md"
      run: ./scripts/validate-opencode-docs.sh --quick
```

### CI (optional)

```yaml
- name: Validate OpenCode Docs
  run: ./scripts/validate-opencode-docs.sh
```

---

## Adding New Docs

When creating a new doc:

1. Determine correct directory (`reference/` vs `design/` vs `archive/`)
2. Add frontmatter with Purpose
3. Update parent README if applicable
4. Add links from related docs
5. Add entry to `HISTORY.md`
6. Run validation
