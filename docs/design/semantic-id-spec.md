# Semantic Issue ID Format Specification

**Status:** Draft v0.4
**Author:** gastown/crew/semantic_id_design
**Date:** 2026-01-29
**Parent Epic:** gt-zfyl8

## Overview

This document specifies semantic "slugs" as human-friendly aliases for beads.
Rather than replacing random IDs, slugs provide an alternative lookup mechanism
while preserving the canonical random ID for stability.

## Design Philosophy

### Slug as Alias, Not Replacement

```
Canonical ID:  gt-zfyl8        ← Immutable, always works
Slug alias:    semantic_ids    ← Human-friendly, can be renamed
```

This mirrors common patterns:
- **Git**: SHA `a1b2c3d` vs branch `main`
- **Web**: `/posts/12345` vs `/posts/my-great-article`
- **DNS**: IP `192.168.1.1` vs hostname `server.local`

### Benefits

1. **No breaking changes**: Random IDs remain canonical
2. **Rename-safe**: Slugs can be updated without breaking references
3. **Optional**: Beads can exist without slugs (backward compat)
4. **Multiple lookups**: Same bead accessible by ID or slug
5. **Simpler implementation**: Just add a field + lookup index

## Format Specification

### Slug Structure

```
<type>-<descriptive_slug>
```

**Components:**
- `<type>`: 3-character type code (e.g., `epc`, `bug`, `tsk`)
- `-`: Separator
- `<descriptive_slug>`: Human-readable name using underscores

### Full Reference (with rig prefix)

When referencing across rigs, include the prefix:
```
gt:epc-semantic_ids     # Full reference
epc-semantic_ids        # Within same rig (prefix optional)
```

### Type Codes

| Bead Type | Code | Example Slug |
|-----------|------|--------------|
| epic | `epc` | `epc-semantic_ids` |
| bug | `bug` | `bug-login_timeout` |
| task | `tsk` | `tsk-add_validation` |
| feature | `ftr` | `ftr-dark_mode` |
| decision | `dec` | `dec-cache_strategy` |
| convoy | `cnv` | `cnv-fix_auth_flow` |
| molecule | `mol` | `mol-deacon_patrol` |
| wisp | `wsp` | `wsp-check_inbox` |
| agent | `agt` | `agt-gastown_witness` |
| role | `rol` | `rol-polecat` |
| mr | `mrq` | `mrq-feature_branch` |

### Examples

| Random ID | Slug | Title |
|-----------|------|-------|
| `gt-zfyl8` | `epc-semantic_ids` | Semantic Issue IDs |
| `gt-zfyl8.1` | `epc-semantic_ids.1` | Design: ID format specification |
| `gt-3q6a9` | `bug-login_timeout` | Fix login timeout |
| `hq-abc123` | `dec-cache_strategy` | Which cache strategy? |

## Hierarchical Slugs

### Canonical IDs vs Slugs

Canonical IDs use numeric dot suffixes:
- `gt-zfyl8` - Parent (epic)
- `gt-zfyl8.1` - First child (task)
- `gt-zfyl8.2` - Second child

Slugs use **named** dot suffixes derived from titles:
- `epc-semantic_ids` → `gt-zfyl8`
- `epc-semantic_ids.format_spec` → `gt-zfyl8.1`
- `epc-semantic_ids.validation` → `gt-zfyl8.8`

### Why Named Children?

Titles are **already mandatory** for all beads. Since every bead has a title,
we can always generate a meaningful slug. No need for numeric suffixes in slugs.

### Examples

| Canonical ID | Title | Slug |
|--------------|-------|------|
| `gt-zfyl8` | Semantic Issue IDs | `epc-semantic_ids` |
| `gt-zfyl8.1` | ID format specification | `epc-semantic_ids.format_spec` |
| `gt-zfyl8.8` | Validation preview | `epc-semantic_ids.validation` |
| `gt-zfyl8.6` | Migration tool | `epc-semantic_ids.migration_tool` |

### Child Slug Generation

Children inherit parent slug as prefix:
```
child_slug = parent_slug + "." + slugify(child_title)
```

No type code on children (parent provides context).

## Slug Rules

### Character Set

| Character | Allowed | Notes |
|-----------|---------|-------|
| `a-z` | Yes | Lowercase letters |
| `0-9` | Yes | Digits (not at start of descriptive part) |
| `_` | Yes | Word separator within descriptive part |
| `-` | Yes | Type-slug separator (exactly one per segment) |
| `.` | Yes | Hierarchy separator (parent.child) |

### Length Constraints

| Component | Min | Max |
|-----------|-----|-----|
| Type code | 3 | 3 |
| Descriptive slug | 3 | 40 |
| **Total** | **7** | **44** |

### Validation Regex

```regex
^(epc|bug|tsk|ftr|dec|cnv|mol|wsp|agt|rol|mrq)-[a-z][a-z0-9_]{2,39}(\.[a-z][a-z0-9_]{2,39})*$
```

**Breakdown:**
- `^(epc|bug|...)` - Type code
- `-` - Type separator
- `[a-z][a-z0-9_]{2,39}` - Root slug (3-40 chars, starts with letter)
- `(\.[a-z][a-z0-9_]{2,39})*` - Optional named children (dot + name)

### Normalization

| Input | Normalized | Rule |
|-------|-----------|------|
| `Fix LOGIN Bug` | `fix_login_bug` | Lowercase, spaces→underscore |
| `Add user auth` | `add_user_auth` | Spaces→underscore |
| `123-bug` | `n123_bug` | Prefix numbers with 'n' |
| `fix--bug` | `fix_bug` | Collapse multiple underscores |

## Collision Handling

### Within Rig: Numeric Suffix

When a slug already exists, append numeric suffix:
```
bug-login_timeout      # First
bug-login_timeout_2    # Second with same title
bug-login_timeout_3    # Third
```

### Cross-Rig: Prefix Required

Slugs are unique within a rig. Cross-rig references need prefix:
```
gt:bug-login_timeout   # gastown rig
bd:bug-login_timeout   # beads rig (different bead)
```

## Data Model

### Bead Schema Addition

```sql
ALTER TABLE issues ADD COLUMN slug TEXT;
CREATE UNIQUE INDEX idx_issues_slug ON issues(slug) WHERE slug IS NOT NULL;
```

### Lookup Priority

1. Try exact ID match: `gt-zfyl8`
2. Try slug match: `epc-semantic_ids` → resolves to `gt-zfyl8`
3. Error if not found

### CLI Usage

```bash
# Both work identically
bd show gt-zfyl8
bd show epc-semantic_ids

# Set/update slug
bd slug gt-zfyl8 epc-semantic_ids

# Remove slug
bd slug gt-zfyl8 --clear

# List beads with slugs
bd list --with-slugs
```

## Auto-Generation

### When Creating Beads

```bash
# Auto-generate slug from title
bd create -t bug "Fix login timeout"
# → ID: gt-abc123
# → Slug: bug-fix_login_timeout (auto-generated)

# Explicit slug
bd create -t bug "Fix login timeout" --slug bug-auth_fix
# → ID: gt-abc123
# → Slug: bug-auth_fix (user-specified)

# No slug (opt-out)
bd create -t bug "Fix login timeout" --no-slug
# → ID: gt-abc123
# → Slug: (none)
```

### Algorithm

```
function generateSlug(type, title):
    1. typeCode = getTypeCode(type)
    2. slug = lowercase(title)
    3. slug = replaceAll(slug, /[^a-z0-9]+/, "_")
    4. slug = collapseConsecutive(slug, "_")
    5. slug = trim(slug, "_")
    6. if startsWithDigit(slug): slug = "n" + slug
    7. if len(slug) > 40: slug = truncateAtWordBoundary(slug, 40)
    8. if len(slug) < 3: slug = padRight(slug, "x", 3)
    9. base = typeCode + "-" + slug
    10. if exists(base): base = appendNumericSuffix(base)
    11. return base
```

## Backward Compatibility

### Random IDs Remain Canonical

- All existing IDs continue to work unchanged
- Slugs are purely additive
- Internal references use IDs, not slugs
- Slugs are for human convenience only

### Migration

No migration required. Slugs are:
1. Optional for existing beads
2. Auto-generated for new beads (can be disabled)
3. Can be added to old beads retroactively

## Implementation Phases

### Phase 1: Core (MVP)
- [ ] Add `slug` column to issues table
- [ ] Implement slug validation regex
- [ ] Add `bd slug` command (set/clear/show)
- [ ] Lookup by slug in `bd show`, `bd update`, etc.

### Phase 2: Auto-Generation
- [ ] Auto-generate slug on `bd create` from type + title
- [ ] `--slug` and `--no-slug` flags
- [ ] Collision handling with `_2`, `_3` suffix

### Phase 3: Hierarchy
- [ ] Child slugs inherit parent prefix: `parent.child`
- [ ] Auto-generate child slug from parent slug + child title
- [ ] Validate parent exists when creating hierarchical slug

### Phase 4: Enhancements (Future)
- [ ] Slug aliases (multiple slugs → same bead)
- [ ] Slug history/redirects (old slug → new slug)
- [ ] Bulk slug generation for existing beads

## Test Cases

### Valid Slugs

```
epc-semantic_ids
bug-fix_login_timeout
tsk-add_user_auth
dec-cache_strategy
epc-semantic_ids.format_spec
epc-semantic_ids.validation
epc-semantic_ids.format_spec.regex_pattern
bug-login_timeout_2
```

### Invalid Slugs

```
semantic_ids           # Missing type code
epc-ab                 # Too short (min 3 chars)
epc-Fix_Login          # Uppercase
bug-fix-login          # Hyphen in descriptive part
EPC-test               # Uppercase type
epc-semantic_ids.1     # Numeric child (use names, not numbers)
epc-semantic_ids/child # Wrong separator (use dot, not slash)
```

## Open Questions

### Resolved

1. **Q: Replace IDs or alias?**
   A: **Alias**. Slugs are lookup helpers, IDs remain canonical.

2. **Q: Hierarchy separator?**
   A: **Dot** (`.`). Same as IDs but with names: `epc-semantic_ids.format_spec`

3. **Q: Numeric or named children?**
   A: **Named**. Titles are mandatory, so always derive from title.

4. **Q: Auto-generate or manual?**
   A: Auto-generate by default, with opt-out.

### Deferred

1. Slug aliases (multiple slugs → one bead)
2. Slug redirects (old slug → new slug)
3. Deep nesting limits (how many `.` levels allowed?)

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 0.1 | 2026-01-29 | Initial draft: single-hyphen replacement format |
| 0.2 | 2026-01-29 | Added type code and random suffix |
| 0.3 | 2026-01-29 | **Pivot**: Slug as alias, not replacement. Random IDs remain canonical. |
| 0.4 | 2026-01-29 | Named children: `epc-semantic_ids.format_spec` (dot separator, title-derived) |
