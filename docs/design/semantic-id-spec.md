# Semantic Issue ID Format Specification

**Status:** Draft v0.5
**Author:** gastown/crew/semantic_id_design
**Date:** 2026-01-29
**Parent Epic:** gt-zfyl8

## Overview

This document specifies semantic "slugs" as human-friendly aliases for beads.
Slugs embed the canonical ID's random component, providing both readability
and a direct link to the underlying bead.

## Format

```
<prefix>-<type>-<title><random>.<child>.<grandchild>...
```

**Components:**
- `<prefix>`: 2-3 char rig identifier (`gt`, `bd`, `hq`)
- `<type>`: 3-char type code (`epc`, `bug`, `tsk`, etc.)
- `<title>`: Slugified title (underscores for spaces)
- `<random>`: Same random ID from canonical bead ID
- `.<child>`: Optional child segments (title-derived, not numbered)

## Examples

| Canonical ID | Title | Slug |
|--------------|-------|------|
| `gt-zfyl8` | Semantic Issue IDs | `gt-epc-semantic_idszfyl8` |
| `gt-zfyl8.1` | Format specification | `gt-epc-semantic_idszfyl8.format_spec` |
| `gt-zfyl8.8` | Validation preview | `gt-epc-semantic_idszfyl8.validation` |
| `gt-3q6a9` | Fix login timeout | `gt-bug-fix_login_timeout3q6a9` |

### Hierarchy

Children append `.name` after the parent's random:

```
gt-epc-semantic_idszfyl8                         # Epic (parent)
gt-epc-semantic_idszfyl8.format_spec             # Task (child)
gt-epc-semantic_idszfyl8.validation              # Task (child)
gt-epc-semantic_idszfyl8.format_spec.regex       # Subtask (grandchild)
```

**Key insight:** The random component (`zfyl8`) anchors the entire tree.
Children are just `.name` appended—no numbers in slugs.

## Type Codes

| Bead Type | Code | Example |
|-----------|------|---------|
| epic | `epc` | `gt-epc-semantic_idszfyl8` |
| bug | `bug` | `gt-bug-login_timeout3q6a9` |
| task | `tsk` | `gt-tsk-add_validationx7m2` |
| feature | `ftr` | `gt-ftr-dark_mode9k4p` |
| decision | `dec` | `gt-dec-cache_strategy2n5q` |
| convoy | `cnv` | `gt-cnv-fix_auth8r3w` |
| molecule | `mol` | `gt-mol-deacon_patrol4j6v` |
| wisp | `wsp` | `gt-wsp-check_inbox1t8y` |
| agent | `agt` | `hq-agt-gastown_witness` |
| role | `rol` | `hq-rol-polecat` |
| mr | `mrq` | `gt-mrq-feature_branch5s2m` |

## Character Set

| Character | Allowed | Notes |
|-----------|---------|-------|
| `a-z` | Yes | Lowercase letters |
| `0-9` | Yes | Digits |
| `_` | Yes | Word separator in title |
| `-` | Yes | Prefix-type-title separator |
| `.` | Yes | Child hierarchy separator |

## Slug Rules

### Title Slugification

1. Lowercase the title
2. Replace non-alphanumeric with `_`
3. Collapse consecutive `_` to single `_`
4. Trim leading/trailing `_`
5. If starts with digit, prefix with `n`
6. Truncate to 40 chars at word boundary
7. Minimum 3 chars (pad with `x` if needed)

### Random Component

- Extracted from canonical ID: `gt-zfyl8` → `zfyl8`
- Appended directly after title slug (no separator)
- Typically 4-6 alphanumeric characters
- Guarantees uniqueness—no collision handling needed

### Child Segments

- Derived from child bead's title
- Appended with `.` separator
- No type code on children (parent provides context)
- No numbers—always use title-derived names

## Validation Regex

```regex
^[a-z]{2,3}-(epc|bug|tsk|ftr|dec|cnv|mol|wsp|agt|rol|mrq)-[a-z][a-z0-9_]{2,39}[a-z0-9]{4,6}(\.[a-z][a-z0-9_]{2,39})*$
```

**Breakdown:**
- `^[a-z]{2,3}` - Prefix (2-3 chars)
- `-` - Separator
- `(epc|bug|...)` - Type code (3 chars)
- `-` - Separator
- `[a-z][a-z0-9_]{2,39}` - Title slug (3-40 chars)
- `[a-z0-9]{4,6}` - Random from canonical ID
- `(\.[a-z][a-z0-9_]{2,39})*` - Optional child segments

## Lookup & Cross-Reference

### From Slug to Canonical ID

Extract random component, reconstruct:
```
gt-epc-semantic_idszfyl8.format_spec
                   └────┘
                   zfyl8 → gt-zfyl8 (parent)
                         → gt-zfyl8.1 (lookup by title match)
```

### From Canonical ID to Slug

Look up bead, construct from type + title + random:
```
gt-zfyl8.1 → type=task, title="Format specification"
           → gt-tsk-... wait, parent is epic
           → gt-epc-semantic_idszfyl8.format_spec
```

## CLI Usage

```bash
# Both work identically
bd show gt-zfyl8
bd show gt-epc-semantic_idszfyl8

# Show with slug
bd show gt-zfyl8 --with-slug

# List beads with slugs
bd list --slugs
```

## Data Model

### Schema

```sql
-- Slug is computed/cached, not a separate column
-- Or stored for performance:
ALTER TABLE issues ADD COLUMN slug TEXT;
CREATE UNIQUE INDEX idx_issues_slug ON issues(slug) WHERE slug IS NOT NULL;
```

### Generation

Slugs can be:
1. **Computed on-the-fly** from type + title + random
2. **Cached in column** for faster lookup
3. **Generated on create** and updated if title changes

## Backward Compatibility

- Canonical random IDs remain the source of truth
- Slugs are derived/computed aliases
- All existing IDs continue to work unchanged
- Slugs add human-friendly lookup, don't replace anything

## Test Cases

### Valid Slugs

```
gt-epc-semantic_idszfyl8
gt-bug-fix_login_timeout3q6a9
gt-tsk-add_user_authx7m2
gt-epc-semantic_idszfyl8.format_spec
gt-epc-semantic_idszfyl8.validation
gt-epc-semantic_idszfyl8.format_spec.regex
```

### Invalid Slugs

```
semantic_idszfyl8                    # Missing prefix and type
gt-semantic_idszfyl8                 # Missing type
epc-semantic_idszfyl8                # Missing prefix
gt-epc-semantic_ids                  # Missing random
gt-epc-semantic_idszfyl8.1           # Numeric child (use names)
gt-epc-Fix_Loginzfyl8                # Uppercase
gt-epc-fix-loginzfyl8                # Hyphen in title
```

## Implementation Phases

### Phase 1: Core
- [ ] Slug generation function (type + title + random)
- [ ] Slug validation regex
- [ ] Lookup by slug in `bd show`

### Phase 2: Children
- [ ] Parent slug extraction from child
- [ ] Child slug generation (parent.child_title)
- [ ] Hierarchical lookup

### Phase 3: CLI Integration
- [ ] `--with-slug` flag on show/list
- [ ] Slug display in standard output
- [ ] Tab completion for slugs

### Phase 4: Caching (Optional)
- [ ] Store slug in column for performance
- [ ] Update slug when title changes
- [ ] Migration for existing beads

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 0.1 | 2026-01-29 | Initial draft: single-hyphen replacement format |
| 0.2 | 2026-01-29 | Added type code and random suffix |
| 0.3 | 2026-01-29 | Pivot: Slug as alias, not replacement |
| 0.4 | 2026-01-29 | Named children with dot separator |
| 0.5 | 2026-01-29 | **Final**: Embed canonical random in slug, children append after |
