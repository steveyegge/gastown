# Integration Branches

> Group epic work on a shared branch, land to main as a unit.

Integration branches provide end-to-end support for epic-scoped work across
the Gas Town pipeline. When you create an integration branch for an epic, it
becomes the automatic target for every stage: polecats spawn their worktrees
from the integration branch (so they start with sibling work already present),
the Refinery merges completed MRs into the integration branch instead of main,
and when all epic children are closed, the Refinery can land the integration
branch back to its base branch (main by default, or whatever was specified
with `--base-branch` at creation) as a single merge commit.

Landing can happen on command or automatically via patrol. The result is that
an entire epic flows through the system as a coherent unit, from first sling
to final land, without any manual branch targeting.

## Workflow

1. **Create the epic and its children.** Structure your work as an epic with
   child tasks (or sub-epics) underneath. Set up dependencies between children
   to define which can run in parallel and which must wait.

2. **Create the integration branch.** This is the shared branch where all child
   work accumulates.
   ```bash
   gt mq integration create gt-auth-epic
   ```

3. **Create a convoy to track the work.** The convoy gives you a single dashboard
   for the entire epic's progress.
   ```bash
   gt convoy create "Auth overhaul" gt-auth-tokens gt-auth-sessions gt-auth-middleware
   ```

4. **Sling the first wave.** Identify children with no blockers and sling them
   to the rig. Use `--no-convoy` since the tracking convoy already exists.
   ```bash
   gt sling gt-auth-tokens gastown --no-convoy
   gt sling gt-auth-sessions gastown --no-convoy
   ```

5. **Polecats process the work.** Each polecat spawns its worktree from the
   integration branch, so it starts with any sibling work that has already
   landed there. When a polecat finishes, it submits a merge request.

6. **Refinery merges to the integration branch.** Instead of merging to main,
   the Refinery merges each MR into the integration branch and marks the child
   task as complete.

7. **Track progress via the convoy.** The convoy status updates each time the
   Refinery completes a task.
   ```bash
   gt convoy status hq-cv-abc
   ```

8. **Sling the next wave.** When a wave completes and its dependent children
   unblock, sling the next batch. Those polecats will start from the integration
   branch — which now contains all the work from the preceding wave.
   ```bash
   gt sling gt-auth-middleware gastown --no-convoy
   ```

9. **Land when complete.** When all children under the epic are closed, the
   integration branch is ready to land. If `integration_branch_auto_land` is
   enabled, the Refinery does this automatically during patrol. Otherwise,
   land manually:
   ```bash
   gt mq integration land gt-auth-epic
   ```
   This merges the integration branch back to its base branch (main by
   default) as a single merge commit, deletes the branch, and closes the
   epic.

## Concept

### The Problem

Without integration branches, epic work lands piecemeal:

```
Child A ──► MR ──► main     (lands Tuesday)
Child B ──► MR ──► main     (lands Wednesday, breaks A's work)
Child C ──► MR ──► main     (lands Thursday, depends on A+B together)
```

Each child merges independently. If Child C depends on A and B being coherent
together, you're relying on merge order and hoping nothing breaks between lands.

### The Solution

Integration branches batch epic work on a shared branch, then land atomically:

```
                           Epic: gt-auth-epic
                                  │
                    ┌─────────────┼─────────────┐
                    │             │             │
               Child A       Child B       Child C
                    │             │             │
                    ▼             ▼             ▼
               ┌────────┐  ┌────────┐  ┌────────┐
               │  MR A  │  │  MR B  │  │  MR C  │
               └───┬────┘  └───┬────┘  └───┬────┘
                   │           │           │
                   └───────────┼───────────┘
                               ▼
                 integration/gt-auth-epic
                    (shared branch)
                               │
                               ▼ gt mq integration land
                          base branch
                    (main or --base-branch)
                     (single merge commit)
```

All child MRs merge into the integration branch first. Children can build on
each other's work. When everything is ready, one command lands it all.

### With vs Without

| Aspect | Without | With Integration Branch |
|--------|---------|------------------------|
| MR target | main | integration/{epic} |
| Land timing | Each MR lands independently | All MRs land together |
| Cross-child deps | Risky—depends on merge order | Safe—children share a branch |
| Rollback | Revert individual commits | Revert one merge commit |
| CI on main | Runs per-MR | Runs once on combined work |

## Lifecycle

### 1. Create the Epic

```bash
bd create --type=epic --title="Auth overhaul"
# → gt-auth-epic
```

Create child issues under the epic as normal.

### 2. Create the Integration Branch

```bash
gt mq integration create gt-auth-epic
# → Created integration/gt-auth-epic from origin/main
# → Stored branch name in epic metadata
```

This pushes a new branch to origin and records its name on the epic.

### 3. Sling Work

Assign children to polecats as normal:

```bash
gt sling gt-auth-tokens gastown
gt sling gt-auth-sessions gastown
```

Polecats auto-detect the integration branch when their issue is a child of an
epic that has one. No manual targeting needed.

### 4. MRs Merge to Integration Branch

When polecats run `gt done` or `gt mq submit`, auto-detection kicks in:

```
gt done
  → Detects parent epic gt-auth-epic
  → Finds integration/gt-auth-epic branch
  → Submits MR targeting integration/gt-auth-epic (not main)
```

The Refinery processes these MRs and merges them to the integration branch.

### 5. Land When Complete

Once all children are closed and all MRs merged:

```bash
gt mq integration land gt-auth-epic
# → Verified all MRs merged
# → Merged integration/gt-auth-epic → base branch (--no-ff)
# → Tests passed
# → Pushed to origin
# → Deleted integration/gt-auth-epic
# → Closed epic gt-auth-epic
```

## Auto-Detection

Integration branches work without manual targeting. Three systems auto-detect them:

| System | What It Does | Config Gate |
|--------|-------------|-------------|
| `gt done` / `gt mq submit` | Targets MR at integration branch instead of main | `integration_branch_refinery_enabled` |
| Polecat spawn | Sources worktree from integration branch | `integration_branch_polecat_enabled` |
| Refinery patrol | Checks if integration branches are ready to land | `integration_branch_auto_land` |

### Detection Algorithm

When `gt done` or `gt mq submit` runs:

| Step | Action | Result |
|------|--------|--------|
| 1 | Load config, check `integration_branch_refinery_enabled` | If false, skip detection |
| 2 | Get current issue ID from branch name | e.g., `gt-auth-tokens` |
| 3 | Walk parent chain (max 10 levels) | Find ancestor epics |
| 4 | For each epic: read `integration_branch:` from metadata | Get stored branch name |
| 5 | Fallback: generate name from template | e.g., `integration/{epic}` |
| 6 | Check if branch exists (local, then remote) | Verify it's real |
| 7 | If found, target MR at that branch | Instead of main |

The `--epic` flag on `gt mq submit` bypasses auto-detection and resolves
the target branch using the configured template (defaulting to
`integration/{epic}`).

## Branch Naming

### Template Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `{epic}` | Full epic ID | `gt-auth-epic` |
| `{prefix}` | Epic prefix (before first hyphen) | `gt` |
| `{user}` | From `git config user.name` | `klauern` |

### Precedence

| Priority | Source | Example |
|----------|--------|---------|
| 1 (highest) | `--branch` flag on create | `--branch "feat/{epic}"` |
| 2 | `integration_branch_template` in config | `"{user}/{epic}"` |
| 3 (lowest) | Default | `"integration/{epic}"` |

### Examples

```bash
# Default template
gt mq integration create gt-auth-epic
# → integration/gt-auth-epic

# Custom template in config: "{user}/{prefix}/{epic}"
gt mq integration create RA-123
# → klauern/RA/RA-123

# Override with --branch flag
gt mq integration create RA-123 --branch "feature/{epic}"
# → feature/RA-123
```

The actual branch name created is stored in the epic's metadata, so auto-detection
always finds the right branch regardless of which template was used.

## Commands

### `gt mq integration create <epic-id>`

Create an integration branch for an epic.

```bash
gt mq integration create <epic-id> [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--branch` | Override branch name template | Config template or `integration/{epic}` |
| `--base-branch` | Create from this branch instead of main (also sets where `land` merges back to) | `origin/main` |

**What it does:**

1. Verifies the epic exists
2. Generates branch name from template (expanding variables)
3. Validates branch name (git-safe characters)
4. Creates local branch from base
5. Pushes to origin
6. Stores branch name and base branch in epic metadata

**Error cases:**

- Epic not found
- Branch already exists
- Invalid characters in generated branch name

### `gt mq integration status <epic-id>`

Display integration branch status for an epic.

```bash
gt mq integration status <epic-id> [flags]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--json` | Output as JSON |

**Output includes:**

- Branch name and creation date
- Commits ahead of main
- Merged MRs (closed, targeting integration branch)
- Pending MRs (open, targeting integration branch)
- Child issue progress (closed / total)
- Ready-to-land status
- Auto-land configuration

**Ready-to-land criteria** (all must be true):

1. Integration branch has commits ahead of main
2. Epic has children
3. All children are closed
4. No pending MRs (all submitted work is merged)

### `gt mq integration land <epic-id>`

Merge an epic's integration branch back to its base branch.

```bash
gt mq integration land <epic-id> [flags]
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--force` | Land even if some MRs still open | `false` |
| `--skip-tests` | Skip test run after merge | `false` |
| `--dry-run` | Preview only, make no changes | `false` |

**What it does:**

1. Verifies epic exists and has an integration branch
2. Reads base branch from epic metadata (defaults to `main` if not stored)
3. Checks all MRs targeting integration branch are merged
4. Creates a temporary worktree (avoids disrupting running agents)
5. Merges integration branch to base branch using `--no-ff`
6. Runs tests (unless `--skip-tests`)
7. Verifies merge brought changes (guards against empty merges)
8. Pushes to origin
9. Deletes integration branch (local and remote)
10. Closes the epic

**Error cases:**

- Epic has no integration branch
- Pending MRs exist (use `--force` to override)
- Tests fail
- Empty merge (no changes to land)

## Configuration

All fields live under `merge_queue` in rig settings (`settings/config.json`):

```json
{
  "merge_queue": {
    "enabled": true,
    "integration_branch_polecat_enabled": true,
    "integration_branch_refinery_enabled": true,
    "integration_branch_template": "integration/{epic}",
    "integration_branch_auto_land": false
  }
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `integration_branch_polecat_enabled` | `*bool` | `true` | Polecats auto-source worktrees from integration branches |
| `integration_branch_refinery_enabled` | `*bool` | `true` | `gt mq submit` and `gt done` auto-detect integration branches as MR targets |
| `integration_branch_template` | `string` | `"integration/{epic}"` | Branch name template (supports `{epic}`, `{prefix}`, `{user}`) |
| `integration_branch_auto_land` | `*bool` | `false` | Refinery patrol auto-lands when all children closed |

**Note:** `*bool` fields use pointer semantics — `null`/omitted means "use default"
(true for polecat/refinery enabled, false for auto-land). Set explicitly to `false`
to disable.

## Auto-Landing

When `integration_branch_auto_land` is `true`, the Refinery patrol automatically
lands integration branches that are ready.

### How It Works

During each patrol cycle, the Refinery:

1. Lists all open epics: `bd list --type=epic --status=open`
2. Checks each epic's integration branch: `gt mq integration status <epic-id>`
3. If `ready_to_land: true`: runs `gt mq integration land <epic-id>`
4. If not ready: skips (epic work is incomplete)

### Conditions for Auto-Land

Both config gates must be true:

- `integration_branch_refinery_enabled: true` (integration feature is on)
- `integration_branch_auto_land: true` (auto-landing is on)

If either is false, the patrol step exits early.

### When to Enable

| Scenario | Recommendation |
|----------|---------------|
| Trusted CI, no human review needed | Enable auto-land |
| Need human sign-off before landing | Keep disabled (default), land manually |
| Mix of both | Keep disabled, use `gt mq integration land` for manual control |

## Anti-Patterns

### Creating Integration Branch After Work Starts

**Wrong:** Sling children, then create the integration branch later.

Children slung before the integration branch exists will target main. Their MRs
won't flow to the integration branch. Create the integration branch *first*,
before slinging any child work.

### Manually Targeting the Integration Branch

**Wrong:** Using `--branch integration/gt-epic` on `gt mq submit`.

Auto-detection handles this. If you find yourself manually targeting, check that:
- The integration branch actually exists
- `integration_branch_refinery_enabled` is not `false`
- The issue is a child (or descendant) of the epic

### Landing Partial Epics

**Wrong:** Using `--force` to land when children are still open.

This defeats the purpose. The integration branch exists so work lands together.
If you need to land early, close or remove the incomplete children first.

## See Also

- [Polecat Lifecycle](polecat-lifecycle.md) — How polecats submit to the merge queue
- [Reference](../reference.md) — Full CLI reference including MQ commands
