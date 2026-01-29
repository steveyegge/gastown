# Semantic Issue ID Format Specification

**Status:** Draft
**Author:** gastown/crew/semantic_id_design
**Date:** 2026-01-29
**Parent Epic:** gt-zfyl8

## Overview

This document specifies the format for semantic issue IDs in the beads system.
Semantic IDs replace random identifiers (e.g., `gt-3q6.9`) with human-readable
identifiers derived from issue titles (e.g., `gt-fix_login_timeout`).

## Design Goals

1. **Readability**: IDs should convey meaning at a glance
2. **Memorability**: Easy to remember and discuss verbally
3. **Typability**: Fast to type without errors
4. **Shell-friendly**: No quoting or escaping required
5. **Collision-resistant**: Unique at scale (5000+ issues)
6. **Backward-compatible**: Coexist with existing random IDs

## Format Specification

### Full ID Structure

```
<prefix>-<semantic_slug>
```

**Components:**
- `<prefix>`: 2-3 character rig identifier (e.g., `gt`, `bd`, `hq`)
- `-`: Separator between prefix and slug
- `<semantic_slug>`: Human-readable identifier derived from title

### Examples

| Title | Semantic ID |
|-------|-------------|
| Fix login timeout | `gt-fix_login_timeout` |
| Add user authentication | `gt-add_user_auth` |
| CRITICAL: Database migration failure | `gt-critical_db_migration_fail` |
| Bug in polecat spawn | `bd-bug_polecat_spawn` |

### Hierarchical IDs (Subtasks)

Subtasks use dot notation appended to parent ID:

```
<parent_id>.<ordinal>
```

**Examples:**
- `gt-semantic_ids.1` - First subtask of `gt-semantic_ids`
- `gt-semantic_ids.2` - Second subtask
- `gt-fix_auth.1.1` - Sub-subtask (if needed)

## Character Set

### Allowed Characters

| Character | Allowed | Notes |
|-----------|---------|-------|
| `a-z` | Yes | Lowercase letters only |
| `0-9` | Yes | Digits |
| `_` | Yes | Primary word separator |
| `-` | Reserved | Prefix separator only |
| `.` | Reserved | Hierarchy separator only |

### Why Underscore as Word Separator

**Recommended: Underscore (`_`)**

| Criterion | Underscore (`_`) | Hyphen (`-`) |
|-----------|-----------------|--------------|
| Shell safety | No escaping needed | Can be interpreted as flag |
| Double-click selection | Selects full slug | Breaks at hyphens |
| Tab completion | Works smoothly | Works, but conflicts exist |
| Prefix separation | Unambiguous | Ambiguous with word separator |
| URL-friendliness | Valid | Valid |
| Typing ergonomics | Shift required | No shift |

**Rationale:** While hyphens are easier to type, the underscore provides critical
advantages for shell usage and prefix parsing. The prefix already uses hyphen
as its separator (`gt-`), so using underscore for word separation creates
unambiguous parsing: split on `-` for prefix, split on `_` for words.

**Shell examples:**
```bash
# Underscore - no issues
bd show gt-fix_login_timeout
gt sling gt-add_user_auth

# Hyphen - potential issues
bd show gt-fix-login-timeout  # Ambiguous: where does prefix end?
bd update --status=done gt-add-auth  # Could confuse with flags
```

## Length Constraints

### Slug Length (after prefix)

| Constraint | Value | Rationale |
|------------|-------|-----------|
| Minimum | 3 chars | Meaningful minimum (`fix`, `add`, `bug`) |
| Maximum | 50 chars | Readable in logs, fits terminal width |
| Recommended | 15-30 chars | Balance of meaning and brevity |

### Full ID Length (with prefix)

| Component | Min | Max |
|-----------|-----|-----|
| Prefix | 2 | 3 |
| Separator | 1 | 1 |
| Slug | 3 | 50 |
| **Total** | **6** | **54** |

### Truncation Rules

When auto-generating from long titles:
1. Truncate at word boundary when possible
2. Maximum 50 characters for slug portion
3. Never truncate to less than 3 characters
4. Remove trailing underscores after truncation

**Example:**
```
Title: "Implement comprehensive user authentication with OAuth2 and JWT support"
Full slug: "impl_comprehensive_user_auth_oauth2_jwt"  # Truncated at word boundary
```

## Reserved Words

The following patterns are **prohibited** as complete slugs:

### CLI Command Names
```
new, list, show, create, delete, update, close, open
sync, add, remove, edit, help, version, status
```

### Logic Keywords
```
all, none, true, false, null, undefined
```

### Special Prefixes
```
mr_*     # Reserved for merge requests
role_*   # Reserved for role beads
agent_*  # Reserved for agent beads
```

**Note:** Reserved words are fine as part of larger slugs:
- `gt-delete` - **PROHIBITED**
- `gt-delete_orphan_files` - **ALLOWED**

## Validation Rules

### Regex Pattern

```regex
^[a-z][a-z0-9_]{2,49}$
```

**Breakdown:**
- `^[a-z]` - Must start with lowercase letter
- `[a-z0-9_]{2,49}` - 2-49 more chars (letters, digits, underscore)
- Total: 3-50 characters

### Full ID Pattern (with prefix)

```regex
^[a-z]{2,3}-[a-z][a-z0-9_]{2,49}(\.\d+)*$
```

**Breakdown:**
- `^[a-z]{2,3}` - 2-3 letter prefix
- `-` - Literal hyphen
- `[a-z][a-z0-9_]{2,49}` - Semantic slug (3-50 chars)
- `(\.\d+)*` - Optional hierarchy suffixes (`.1`, `.2`, etc.)

### Validation Examples

| ID | Valid | Reason |
|----|-------|--------|
| `gt-fix_login` | ✓ | Standard semantic ID |
| `gt-a` | ✗ | Too short (min 3 chars) |
| `gt-123_bug` | ✗ | Must start with letter |
| `gt-FIX_LOGIN` | ✗ | Must be lowercase |
| `gt-fix-login` | ✗ | No hyphens in slug |
| `gt-fix__login` | ✗ | No consecutive underscores |
| `gt-fix_login.1` | ✓ | Valid hierarchical ID |
| `gt-delete` | ✗ | Reserved word |
| `gt-delete_files` | ✓ | Reserved word as prefix is OK |

## Backward Compatibility

### Coexistence with Random IDs

The system must support both ID types indefinitely:

| Pattern | Type | Example |
|---------|------|---------|
| `prefix-[a-z0-9]{4,7}` | Random (legacy) | `gt-3q6a9`, `bd-xyz123` |
| `prefix-[a-z][a-z0-9_]+` | Semantic (new) | `gt-fix_login` |

### Detection Heuristic

```go
func IsSemanticID(id string) bool {
    // After stripping prefix, check if it:
    // 1. Contains underscore (random IDs don't)
    // 2. Starts with letter and has only [a-z0-9_]
    slug := stripPrefix(id)
    return strings.Contains(slug, "_") && semanticPattern.MatchString(slug)
}
```

### Migration Path

1. **Phase 1**: Accept both ID types, generate random by default
2. **Phase 2**: Generate semantic IDs for new issues (with fallback)
3. **Phase 3**: Optional migration tool to rename legacy IDs

## Edge Cases

### Duplicate Slugs

When generated slug already exists:
1. Append incrementing suffix: `fix_login`, `fix_login_2`, `fix_login_3`
2. Check suffix is still within length limit
3. If length exceeded, truncate base slug before adding suffix

### Slug Normalization

| Input | Output | Rule Applied |
|-------|--------|--------------|
| `Fix LOGIN` | `fix_login` | Lowercase |
| `fix  login` | `fix_login` | Collapse spaces |
| `fix--login` | `fix_login` | Collapse separators |
| `__fix_login__` | `fix_login` | Trim underscores |
| `fix@login#timeout` | `fix_login_timeout` | Replace special chars |
| `123_fix` | `n123_fix` | Prefix number with 'n' |

### Internationalization

Non-ASCII characters are transliterated or removed:
- `Ääkkönen` → `aakkonen`
- `日本語` → removed (would need 'n' prefix or fail validation)

**Current scope:** ASCII-only. Unicode support deferred to future iteration.

## Implementation Notes

### Slug Generation Algorithm

```
function generateSlug(title):
    1. slug = lowercase(title)
    2. slug = transliterate(slug)           # ä→a, ö→o
    3. slug = replaceAll(slug, /[^a-z0-9]+/, "_")
    4. slug = collapseConsecutive(slug, "_")
    5. slug = trim(slug, "_")
    6. if startsWithDigit(slug): slug = "n" + slug
    7. if len(slug) > 50: slug = truncateAtWordBoundary(slug, 50)
    8. if len(slug) < 3: slug = padRight(slug, "x", 3)
    9. if isReserved(slug): slug = slug + "_issue"
    10. return slug
```

### Performance Considerations

- Collision checking should use indexed lookup
- At 5000 issues, expect ~1-2% collision rate on common titles
- Suffix generation is O(n) worst case, O(1) typical

## Test Cases

### Valid IDs

```
gt-fix_login_timeout
gt-add_user_auth
gt-critical_db_migration_fail
bd-bug_polecat_spawn
hq-refactor_mail_routing
gt-update_cli_version_2
gt-fix.1
gt-fix_auth.1.2
```

### Invalid IDs

```
gt-a                    # Too short
gt-123fix              # Starts with digit
gt-FIX_LOGIN           # Uppercase
gt-fix-login           # Hyphen in slug
gt-fix__login          # Consecutive underscores
gt-_fix_login          # Starts with underscore
gt-delete              # Reserved word
gt-thisisaverylongslugthatshouldberejectedbecauseitexceedsfiftychars  # Too long
```

## Open Questions

### Resolved

1. **Q: Underscore vs hyphen?**
   A: Underscore. Better shell ergonomics and unambiguous prefix parsing.

2. **Q: Length limits?**
   A: 3-50 characters for slug, 6-54 total.

3. **Q: Case sensitivity?**
   A: Lowercase only. Case-insensitive matching.

### Deferred

1. **Unicode support**: How to handle non-ASCII titles? (Deferred to future iteration)
2. **Rename API**: Should existing random IDs be renameable? (Covered in gt-zfyl8.6)
3. **Collision thresholds**: At what collision rate should we alert? (Covered in gt-zfyl8.8)

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 0.1 | 2026-01-29 | Initial draft |
