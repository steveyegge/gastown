---
name: ghi-list
description: >
  List GitHub issues in a formatted ASCII table. Supports filters like --state,
  --assignee, --label. Use for issue triage and tracking workflows.
allowed-tools: "Bash(gh issue list:*)"
version: "1.0.0"
author: "Gas Town"
---

# GHI List - Formatted GitHub Issues Table

Display GitHub issues in a clean ASCII box-drawing table format.

## Usage

```
/ghi-list [options]
```

## Options (passed to gh issue list)

- `--state open|closed|all` - Filter by state (default: open)
- `--assignee <user>` - Filter by assignee
- `--label <label>` - Filter by label (can repeat)
- `--limit <n>` - Max items to fetch (default: 30)
- `--author <user>` - Filter by author
- `--milestone <name>` - Filter by milestone

## Execution Steps

1. Run `gh issue list` with any provided options plus `--json number,assignees,labels,title,state`
2. Format output as an ASCII table using box-drawing characters
3. Truncate titles to fit reasonable terminal width (~45 chars)
4. Show first assignee only, or "-" if none
5. Show first 2-3 labels abbreviated, or "-" if none

## Output Format

Use this exact table style with box-drawing characters:

```
┌───────┬────────────┬─────────────────┬───────────────────────────────────────────────┬────────┐
│ Issue │ Assignee   │ Labels          │ Title                                         │ State  │
├───────┼────────────┼─────────────────┼───────────────────────────────────────────────┼────────┤
│   372 │ max        │ enhancement     │ Create /pr-list and /ghi-list skills          │ OPEN   │
│   371 │ -          │ bug, priority   │ Fix null pointer in auth module               │ OPEN   │
│   370 │ joe        │ -               │ Update documentation for new API              │ CLOSED │
└───────┴────────────┴─────────────────┴───────────────────────────────────────────────┴────────┘
```

## Table Requirements

- Use Unicode box-drawing: `┌ ┐ └ ┘ ├ ┤ ┬ ┴ ┼ │ ─`
- Right-align issue numbers
- Left-align text columns
- Pad columns consistently
- For assignees: show login of first assignee, "-" if none
- For labels: show comma-separated names (truncate if >17 chars)
- Show count summary below table: "**N issues** (M open, K closed)"

## Example Commands

```bash
# Default - open issues
gh issue list --json number,assignees,labels,title,state

# With filters
gh issue list --state all --label bug --json number,assignees,labels,title,state
```
