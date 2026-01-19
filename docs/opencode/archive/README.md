# Archive: Point-in-Time Documentation

This directory contains analysis, research, and test results that are **snapshots from specific points in time**.

> ⚠️ **Warning**: Documents here may contain outdated information about features or capabilities.
> 
> **For current feature status**, see:
> - [Agent Feature Comparison](../../agent-features.md) - Authoritative feature matrix
> - [reference/](../reference/) - Current stable documentation
>
> Archive docs record **verified findings at a point in time**, not ongoing truth.

## Required Frontmatter

All documents in this directory **must** include YAML frontmatter:

```yaml
---
# Required fields
title: Brief descriptive title
date: YYYY-MM-DD
status: complete | in-progress | superseded
type: analysis | research | test-results | review

# Context (at least one required)
commit: abc1234  # Related commit hash
branch: branch-name  # Branch when created
version: 1.2.3  # OpenCode version tested (if applicable)

# Phase association (optional)
phase: 1 | 2 | null  # Which phase this relates to (null = foundational/general)

# Optional
superseded_by: filename.md  # If this doc is outdated
related: [file1.md, file2.md]  # Related documents
---
```

## Document Types

| Type | Description | Example |
|------|-------------|---------|
| `analysis` | Studying a topic in depth | concept-analysis.md |
| `research` | Investigating external sources | technical-research.md |
| `test-results` | Recording test/experiment outcomes | e2e-test-results.md |
| `review` | Reviewing code, PRs, or designs | upstream-review.md |

## When to Create

Create a document here when:
- Conducting research that may become stale
- Recording test results from a specific version
- Analyzing something that will evolve
- Any work tied to a specific commit/branch/version

## When to Update vs Create New

- **Update**: Fix typos, add clarifications, update status
- **Create New**: Re-run analysis, new version tested, significant changes

If creating a new version, mark the old one as `superseded_by: new-file.md`.

## Current Documents

### Foundational (No Phase)

General research applicable across all phases:

| File | Type | Date |
|------|------|------|
| concept-analysis.md | analysis | 2026-01-15 |
| technical-research.md | research | 2026-01-15 |

### Phase 1: Claude Code Parity

| File | Type | Date |
|------|------|------|
| experiments.md | research | 2026-01-15 |
| e2e-test-results.md | test-results | 2026-01-17 |
| integration-test-results.md | test-results | 2026-01-17 |
| session-fork-test-results.md | test-results | 2026-01-17 |
| impact-analysis.md | analysis | 2026-01-17 |
| upstream-review.md | review | 2026-01-17 |
