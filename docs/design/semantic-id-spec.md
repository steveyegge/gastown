# Semantic Issue ID Format Specification

**Status:** Draft v0.2
**Author:** gastown/crew/semantic_id_design
**Date:** 2026-01-29
**Parent Epic:** gt-zfyl8

## Overview

This document specifies the format for semantic issue IDs in the beads system.
Semantic IDs replace random identifiers (e.g., `gt-3q6.9`) with human-readable
identifiers that include type information and a semantic slug derived from the
title (e.g., `gt-epc-semantic_ids7x9k`).

## Design Goals

1. **Readability**: IDs should convey meaning at a glance
2. **Type-aware**: Bead type visible in the ID itself
3. **Memorability**: Easy to remember and discuss verbally
4. **Shell-friendly**: No quoting or escaping required
5. **Collision-proof**: Random suffix guarantees uniqueness
6. **Backward-compatible**: Coexist with existing random IDs

## Format Specification

### Full ID Structure

```
<prefix>-<type>-<slug><suffix>
```

**Components:**
- `<prefix>`: 2-3 character rig identifier (e.g., `gt`, `bd`, `hq`)
- `-`: Separator
- `<type>`: 3-character type code (e.g., `epc`, `bug`, `tsk`)
- `-`: Separator
- `<slug>`: Human-readable identifier derived from title (underscores for spaces)
- `<suffix>`: 4-character random alphanumeric for uniqueness

### Type Codes

| Bead Type | Code | Example |
|-----------|------|---------|
| epic | `epc` | `gt-epc-semantic_ids7x9k` |
| bug | `bug` | `gt-bug-login_timeout3f2a` |
| task | `tsk` | `gt-tsk-add_validation9b1c` |
| feature | `ftr` | `gt-ftr-dark_mode4d7e` |
| decision | `dec` | `gt-dec-cache_strategy2m4n` |
| convoy | `cnv` | `gt-cnv-fix_auth_flow8p3q` |
| molecule | `mol` | `gt-mol-deacon_patrol5r6s` |
| wisp | `wsp` | `gt-wsp-check_inbox7t8u` |
| agent | `agt` | `hq-agt-gastown_witness` |
| role | `rol` | `hq-rol-polecat_config` |
| mr | `mrq` | `gt-mrq-feature_branch2v3w` |

### Examples

| Title | Type | Semantic ID |
|-------|------|-------------|
| Semantic Issue IDs | epic | `gt-epc-semantic_ids7x9k` |
| Fix login timeout | bug | `gt-bug-fix_login_timeout3f2a` |
| Add user authentication | task | `gt-tsk-add_user_auth9b1c` |
| Database migration failure | bug | `bd-bug-db_migration_fail4d7e` |
| Which cache strategy? | decision | `gt-dec-cache_strategy2m4n` |

### Hierarchical IDs (Subtasks)

Subtasks use dot notation appended to parent ID:

```
<parent_id>.<ordinal>
```

**Examples:**
- `gt-epc-semantic_ids7x9k.1` - First subtask of the epic
- `gt-epc-semantic_ids7x9k.2` - Second subtask
- `gt-tsk-add_auth9b1c.1` - Subtask of a task

## Character Set

### Allowed Characters by Segment

| Segment | Characters | Notes |
|---------|------------|-------|
| Prefix | `a-z` | 2-3 lowercase letters |
| Type | `a-z` | Exactly 3 lowercase letters |
| Slug | `a-z`, `0-9`, `_` | Underscore as word separator |
| Suffix | `a-z`, `0-9` | 4-char random alphanumeric |
| Separators | `-`, `.` | Hyphen between segments, dot for hierarchy |

### Why Two Hyphens?

The format uses hyphens to separate prefix, type, and slug+suffix:

```
gt-epc-semantic_ids7x9k
│  │   └─ slug + suffix (underscore-separated words)
│  └─ type code
└─ prefix
```

**Parsing is unambiguous:**
- Split on `-` gives exactly 3 parts: `[prefix, type, slug+suffix]`
- Underscores only appear within the slug
- Suffix is always last 4 chars of the third segment

**Shell examples:**
```bash
# Clear structure, easy to parse
bd show gt-epc-semantic_ids7x9k
gt sling gt-bug-fix_login3f2a

# Tab completion works on type
bd list --type=bug  # or filter by gt-bug-*
```

## Length Constraints

### Segment Lengths

| Segment | Min | Max | Notes |
|---------|-----|-----|-------|
| Prefix | 2 | 3 | Rig identifier |
| Type | 3 | 3 | Fixed-length type code |
| Slug | 3 | 40 | Human-readable portion |
| Suffix | 4 | 4 | Random uniqueness |

### Full ID Length

| Component | Min | Max |
|-----------|-----|-----|
| Prefix | 2 | 3 |
| Hyphen | 1 | 1 |
| Type | 3 | 3 |
| Hyphen | 1 | 1 |
| Slug | 3 | 40 |
| Suffix | 4 | 4 |
| **Total** | **14** | **52** |

### Truncation Rules

When auto-generating from long titles:
1. Truncate slug at word boundary when possible
2. Maximum 40 characters for slug portion
3. Never truncate to less than 3 characters
4. Remove trailing underscores after truncation
5. Append 4-char random suffix after truncation

**Example:**
```
Title: "Implement comprehensive user authentication with OAuth2 and JWT support"
Slug:  "impl_user_auth_oauth2_jwt"  (truncated at word boundary)
Full:  "gt-tsk-impl_user_auth_oauth2_jwt8k3m"
```

## Random Suffix

### Purpose

The 4-character random suffix guarantees uniqueness even when titles collide:

| Title | Generated ID |
|-------|--------------|
| Fix login bug | `gt-bug-fix_login_bug3f2a` |
| Fix login bug | `gt-bug-fix_login_bug7x9k` |
| Fix login bug | `gt-bug-fix_login_bug2m4n` |

### Generation

- Characters: `[a-z0-9]` (36 possible per position)
- Length: 4 characters
- Combinations: 36^4 = 1,679,616 unique suffixes
- Collision probability at 10,000 issues: < 0.003%

### Suffix Placement

Suffix is appended directly to slug without separator:

```
gt-bug-fix_login3f2a
           └────┴─── suffix (4 chars)
```

This keeps IDs compact while maintaining readability.

## Validation Rules

### Regex Pattern (Full ID)

```regex
^[a-z]{2,3}-(epc|bug|tsk|ftr|dec|cnv|mol|wsp|agt|rol|mrq)-[a-z][a-z0-9_]{2,39}[a-z0-9]{4}(\.\d+)*$
```

**Breakdown:**
- `^[a-z]{2,3}` - 2-3 letter prefix
- `-` - Literal hyphen
- `(epc|bug|tsk|ftr|dec|cnv|mol|wsp|agt|rol|mrq)` - Type code enum
- `-` - Literal hyphen
- `[a-z][a-z0-9_]{2,39}` - Slug: starts with letter, 3-40 chars
- `[a-z0-9]{4}` - Random suffix: exactly 4 alphanumeric
- `(\.\d+)*` - Optional hierarchy suffixes (`.1`, `.2`, etc.)

### Validation Examples

| ID | Valid | Reason |
|----|-------|--------|
| `gt-epc-semantic_ids7x9k` | ✓ | Standard semantic ID |
| `gt-bug-fix_login3f2a` | ✓ | Bug with short slug |
| `gt-tsk-a7x9` | ✗ | Slug too short (min 3 before suffix) |
| `gt-xxx-test1234` | ✗ | Invalid type code |
| `gt-epc-Fix_Login7x9k` | ✗ | Must be lowercase |
| `gt-bug-fix-login3f2a` | ✗ | No hyphens in slug |
| `gt-epc-semantic_ids7x9k.1` | ✓ | Valid hierarchical ID |
| `GT-EPC-TEST1234` | ✗ | Must be lowercase |

## Reserved Patterns

### Special Agent IDs

Agent beads may omit the suffix for well-known singletons:

```
hq-agt-mayor          # Town-level agent (no suffix needed)
hq-agt-deacon         # Town-level agent
hq-agt-gastown_witness    # Rig-level agent
```

### Role Beads

Role configuration beads use consistent naming:

```
hq-rol-polecat        # Polecat role config
hq-rol-crew           # Crew role config
```

## Backward Compatibility

### Coexistence with Random IDs

The system must support both ID types indefinitely:

| Pattern | Type | Example |
|---------|------|---------|
| `prefix-[a-z0-9]{4,7}` | Random (legacy) | `gt-3q6a9`, `bd-xyz123` |
| `prefix-type-slug+suffix` | Semantic (new) | `gt-epc-semantic_ids7x9k` |

### Detection Heuristic

```go
func IsSemanticID(id string) bool {
    parts := strings.Split(id, "-")
    if len(parts) < 3 {
        return false  // Legacy format: prefix-random
    }
    // Semantic format has 3 parts: prefix-type-slug
    return isValidTypeCode(parts[1])
}
```

### Migration Path

1. **Phase 1**: Accept both ID types, generate random by default
2. **Phase 2**: Generate semantic IDs for new issues
3. **Phase 3**: Optional migration tool to rename legacy IDs

## Implementation Notes

### ID Generation Algorithm

```
function generateSemanticID(prefix, type, title):
    1. typeCode = getTypeCode(type)  # epic→epc, bug→bug, etc.
    2. slug = lowercase(title)
    3. slug = transliterate(slug)     # ä→a, ö→o
    4. slug = replaceAll(slug, /[^a-z0-9]+/, "_")
    5. slug = collapseConsecutive(slug, "_")
    6. slug = trim(slug, "_")
    7. if startsWithDigit(slug): slug = "n" + slug
    8. if len(slug) > 40: slug = truncateAtWordBoundary(slug, 40)
    9. if len(slug) < 3: slug = padRight(slug, "x", 3)
    10. suffix = randomAlphanumeric(4)
    11. return prefix + "-" + typeCode + "-" + slug + suffix
```

### Parsing Algorithm

```
function parseSemanticID(id):
    parts = split(id, "-")
    if len(parts) != 3: return error("invalid format")

    prefix = parts[0]
    typeCode = parts[1]
    slugWithSuffix = parts[2]

    # Handle hierarchy suffix
    if contains(slugWithSuffix, "."):
        mainPart, hierarchy = splitFirst(slugWithSuffix, ".")
        slugWithSuffix = mainPart

    # Extract random suffix (last 4 chars)
    if len(slugWithSuffix) < 7:  # min: 3 slug + 4 suffix
        return error("slug too short")

    slug = slugWithSuffix[:-4]
    suffix = slugWithSuffix[-4:]

    return {prefix, typeCode, slug, suffix, hierarchy}
```

### Storage Considerations

- Index on full ID for lookups
- Index on `(prefix, typeCode)` for type-filtered queries
- Suffix uniqueness enforced at creation time

## Test Cases

### Valid IDs

```
gt-epc-semantic_ids7x9k
gt-bug-fix_login_timeout3f2a
gt-tsk-add_user_auth9b1c
bd-bug-db_migration4d7e
hq-dec-cache_strategy2m4n
gt-cnv-fix_auth_flow8p3q
gt-mol-deacon_patrol5r6s
gt-epc-semantic_ids7x9k.1
gt-epc-semantic_ids7x9k.2
hq-agt-mayor
hq-rol-polecat
```

### Invalid IDs

```
gt-semantic_ids7x9k       # Missing type code
gt-epc-ab12               # Slug too short (need 3+4)
gt-xxx-test1234           # Invalid type code
gt-epc-Fix_Login7x9k      # Uppercase not allowed
gt-bug-fix-login3f2a      # Hyphen in slug
gt-epc-test               # Missing random suffix
GT-EPC-TEST1234           # Uppercase prefix/type
```

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 0.1 | 2026-01-29 | Initial draft with single-hyphen format |
| 0.2 | 2026-01-29 | **Breaking**: New format `prefix-type-slug+suffix` with embedded type code and random suffix for collision resistance |
