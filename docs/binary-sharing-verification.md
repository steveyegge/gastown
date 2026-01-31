# Binary Sharing Verification: gt12 Uses gt11's Binaries

**Task:** hq-arcu0q.10
**Epic:** hq-arcu0q (Colonization)
**Date:** 2026-01-27
**Status:** Verified - SAFE TO SHARE

## Summary

**CONFIRMED: gt and bd binaries are town-agnostic and can be shared between gt11 and gt12.**

Both binaries are purely driven by environment variables and runtime filesystem discovery.
No hardcoded paths or town-specific values are compiled into the binaries.

## Binary Locations

```
/home/ubuntu/.local/bin/gt   # Shared by all towns
/home/ubuntu/.local/bin/bd   # Shared by all towns
```

Both are standard ELF 64-bit executables with no embedded path information.

## Why Binaries Are Town-Agnostic

### 1. Runtime Town Discovery

Town root is discovered at runtime by walking the filesystem looking for marker files:

```go
// internal/workspace/find.go
const PrimaryMarker = "mayor/town.json"

func Find(startDir string) (string, error) {
    // Walks up directory tree looking for marker
    // Returns first match - completely generic
}
```

### 2. Environment Variable Driven

All town-specific behavior comes from runtime environment variables:

| Variable | Set By | Purpose |
|----------|--------|---------|
| `GT_ROOT` | Shell hook (`gt detect`) | Town root path |
| `GT_TOWN_ROOT` | Shell hook | Fallback when CWD deleted |
| `GT_RIG` | Shell hook | Current rig name |
| `GT_ROLE` | Agent launcher | Agent role |
| `BD_ACTOR` | Agent launcher | Beads actor identity |

### 3. No Build-Time Town Info

Makefile ldflags embed only version metadata:

```makefile
LDFLAGS := -X ...Version=$(VERSION) \
           -X ...Commit=$(COMMIT) \
           -X ...BuildTime=$(BUILD_TIME)
```

No paths, no town names, no environment-specific values.

### 4. PATH-Based Discovery

The `bd` binary is discovered via `$PATH`, not hardcoded:

```go
cmd := exec.Command("bd", args...) // Uses $PATH
```

## Verification Checklist

| Check | Result |
|-------|--------|
| Hardcoded paths in source | None found |
| Town-specific strings in binary | None |
| Build-time path embedding | None |
| Runtime town discovery | Working (marker-based) |
| Environment variable support | Fully implemented |

## gt12 Setup Requirements

For gt12 to use the shared binaries:

1. **No binary installation needed** - Use existing `~/.local/bin/gt` and `bd`

2. **Shell integration** - Add to gt12 shell profile:
   ```bash
   eval "$(gt rig detect)"
   ```

3. **Environment variables** - Set automatically by `gt detect` when in gt12 directory:
   ```bash
   cd /home/ubuntu/gt12
   eval "$(gt rig detect)"
   # Now GT_ROOT=/home/ubuntu/gt12
   ```

## Testing

To verify binary sharing works:

```bash
# In gt11
cd /home/ubuntu/gt11
eval "$(gt rig detect)"
echo $GT_ROOT  # Should show /home/ubuntu/gt11

# In gt12 (after setup)
cd /home/ubuntu/gt12
eval "$(gt rig detect)"
echo $GT_ROOT  # Should show /home/ubuntu/gt12

# Same binary, different behavior based on directory
which gt  # ~/.local/bin/gt in both cases
```

## Conclusion

Binary sharing is **safe and recommended**. Both towns use the same binaries with
behavior determined at runtime by:

1. Current working directory (for town detection)
2. Environment variables (set by shell hook)
3. Configuration files (in each town's directory structure)

No rebuild or separate installation needed for gt12.
