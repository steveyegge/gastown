# Towers of Hanoi Demo

A durability proof demonstrating Gas Town's ability to execute arbitrarily long
sequential workflows with crash recovery and session cycling.

## What This Proves

1. **Large Molecule Creation**: Creating 1000+ issues in a single workflow
2. **Sequential Execution**: Dependencies chain properly across many steps
3. **Crash Recovery**: Work resumes correctly after session restart
4. **Nondeterministic Idempotence**: Different sessions, same outcome

## The Math

Towers of Hanoi requires `2^n - 1` moves for `n` disks:

| Disks | Moves   | Formula Size | Est. Runtime |
|-------|---------|--------------|--------------|
| 7     | 127     | ~19 KB       | ~14 sec      |
| 9     | 511     | ~74 KB       | ~1 min       |
| 10    | 1,023   | ~149 KB      | ~2 min       |
| 15    | 32,767  | ~4.7 MB      | ~1 hour      |
| 20    | 1M+     | ~163 MB      | ~30 hours    |

## Pre-Generated Formulas

Located in `.beads/formulas/`:

- `towers-of-hanoi-7.formula.toml` - 127 moves (quick test)
- `towers-of-hanoi-9.formula.toml` - 511 moves (medium test)
- `towers-of-hanoi-10.formula.toml` - 1023 moves (standard demo)

## Running the Demo

### Quick Test (7 disks, ~14 seconds)

```bash
# Create wisp
bd mol wisp towers-of-hanoi-7 --json | jq -r '.new_epic_id'
# Returns: gt-eph-xxx

# Get all child IDs
bd list --parent=gt-eph-xxx --limit=200 --json | jq -r '.[].id' > /tmp/ids.txt

# Close all issues (serial)
while read id; do bd --no-daemon close "$id" >/dev/null; done < /tmp/ids.txt

# Burn the wisp (cleanup)
bd mol burn gt-eph-xxx --force
```

### Standard Demo (10 disks, ~2 minutes)

```bash
# Create wisp
WISP=$(bd mol wisp towers-of-hanoi-10 --json | jq -r '.new_epic_id')
echo "Created wisp: $WISP"

# Get all 1025 child IDs (1023 moves + setup + verify)
bd list --parent=$WISP --limit=2000 --json | jq -r '.[].id' > /tmp/ids.txt
wc -l /tmp/ids.txt  # Should show 1025

# Time the execution
START=$(date +%s)
while read id; do bd --no-daemon close "$id" >/dev/null 2>&1; done < /tmp/ids.txt
END=$(date +%s)
echo "Completed in $((END - START)) seconds"

# Verify completion
bd list --parent=$WISP --status=open  # Should be empty

# Cleanup
bd mol burn $WISP --force
```

## Why Wisps?

The demo uses wisps (ephemeral molecules) because:

1. **No Git Pollution**: Wisps don't sync to JSONL, keeping git history clean
2. **Auto-Cleanup**: Wisps can be burned without leaving tombstones
3. **Speed**: No export overhead during rapid closes
4. **Appropriate Semantics**: This is operational testing, not auditable work

## Key Insights

### `bd ready` Excludes Wisps

By design, `bd ready` filters out ephemeral issues:
```go
"(i.ephemeral = 0 OR i.ephemeral IS NULL)", // Exclude wisps
```

For wisp execution, query children directly:
```bash
bd list --parent=$WISP --status=open
```

### Dependencies Work Correctly

Each move depends on the previous one via `needs`:
```toml
[[steps]]
id = "move-42"
needs = ["move-41"]
```

This creates proper `blocks` dependencies. Parent-child relationships
provide hierarchy only - they don't block execution.

### Close Speed

With `bd --no-daemon close`:
- ~109ms per close (serial)
- ~9 closes/second

Parallelization would improve throughput but requires careful
dependency ordering.

## Generating Larger Formulas

Use the generator script:

```bash
# Generate 15-disk formula (32K moves)
python3 scripts/gen_hanoi.py 15 > .beads/formulas/towers-of-hanoi-15.formula.toml
```

**Warning**: 20-disk formula is ~163MB and creates 1M+ issues. Only for
stress testing post-launch.

## Monitoring Progress

For long-running executions:

```bash
# Count closed issues
bd list --parent=$WISP --status=closed --json | jq 'length'

# Count remaining
bd list --parent=$WISP --status=open --json | jq 'length'

# Progress percentage
TOTAL=1025
CLOSED=$(bd list --parent=$WISP --status=closed --limit=2000 --json | jq 'length')
echo "$CLOSED / $TOTAL = $((CLOSED * 100 / TOTAL))%"
```

## Session Cycling

The beauty of this demo: you can stop at any time and resume later.

```bash
# Session 1: Start the wisp, close some issues
WISP=$(bd mol wisp towers-of-hanoi-10 --json | jq -r '.new_epic_id')
# ... close some issues ...
# Context fills, need to cycle

gt handoff -s "Hanoi demo" -m "Wisp: $WISP, progress: 400/1025"
```

```bash
# Session 2: Resume where you left off
# (Read handoff mail for wisp ID)
bd list --parent=$WISP --status=open --limit=2000 --json | jq -r '.[].id' > /tmp/ids.txt
# ... continue closing ...
```

The molecule IS the state. No memory of previous session needed.
