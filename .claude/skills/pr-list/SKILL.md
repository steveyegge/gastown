---
name: pr-list
description: >
  List GitHub PRs in a formatted ASCII table. Supports filters like --state,
  --author, --label. Use for PR review workflows and sheriff duties.
allowed-tools: "Bash(gh pr list:*)"
version: "1.0.0"
author: "Gas Town"
---

# PR List - Formatted Pull Request Table

Display GitHub pull requests in a clean ASCII box-drawing table format.

## Usage

```
/pr-list [options]
```

## Options (passed to gh pr list)

- `--state open|closed|merged|all` - Filter by state (default: open)
- `--author <user>` - Filter by author
- `--label <label>` - Filter by label (can repeat)
- `--limit <n>` - Max items to fetch (default: 30)
- `--assignee <user>` - Filter by assignee
- `--draft` - Show only drafts
- `--all-reviews` - Include PRs with CHANGES_REQUESTED (excluded by default)

## Execution Steps

1. Run `gh pr list` with any provided options plus `--json number,author,title,state,isDraft,reviewDecision`
2. **Filter out PRs with reviewDecision="CHANGES_REQUESTED"** (unless `--all-reviews` specified)
3. Format output as an ASCII table using box-drawing characters
4. Truncate titles to fit reasonable terminal width (~50 chars)
5. Truncate author names if needed (~18 chars)
6. Show state as OPEN/CLOSED/MERGED/DRAFT, append "(changes requested)" if filtered with --all-reviews

## Output Format

Use this exact table style with box-drawing characters:

```
┌─────┬────────────────────┬───────────────────────────────────────────────────┬────────┐
│  PR │ Author             │ Title                                             │ State  │
├─────┼────────────────────┼───────────────────────────────────────────────────┼────────┤
│ 123 │ username           │ feat: add new feature for something               │ OPEN   │
│ 122 │ another-user       │ fix: resolve bug in component                     │ DRAFT  │
└─────┴────────────────────┴───────────────────────────────────────────────────┴────────┘
```

## Table Requirements

- Use Unicode box-drawing: `┌ ┐ └ ┘ ├ ┤ ┬ ┴ ┼ │ ─`
- Right-align PR numbers
- Left-align text columns
- Pad columns consistently
- Show count summary below table: "**N open PRs** (M drafts)"

## Example Commands

```bash
# Default - open PRs (excludes CHANGES_REQUESTED)
gh pr list --json number,author,title,state,isDraft,reviewDecision

# With filters
gh pr list --state all --author boshu2 --json number,author,title,state,isDraft,reviewDecision
```
