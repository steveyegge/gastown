# Done — Submit Work to Merge Queue

Signal that your work is complete and ready for the merge queue.

Arguments: $ARGUMENTS

## Pre-flight Checks

Before running `gt done`, verify your work is ready:

```bash
git status                          # Must be clean (no uncommitted changes)
git log --oneline origin/main..HEAD # Must have at least 1 commit
```

If there are uncommitted changes, commit them first:
```bash
git add <files>
git commit -m "<type>: <description>"
```

## Execute

Run `gt done` with any provided arguments:

```bash
gt done $ARGUMENTS
```

**Common usage:**
- `gt done` — Submit completed work (default: --status COMPLETED)
- `gt done --pre-verified` — Submit with pre-verification in advisory mode only
- `gt done --status ESCALATED` — Signal blocker, skip MR
- `gt done --status DEFERRED` — Pause work, skip MR

If the rig uses `merge_queue.verification_mode=strict`, `gt done` reruns the
configured pre-merge gates before push/MR creation and blocks submission on any
failure. `--pre-verified` is rejected in strict mode.

**If the bead has nothing to implement** (already fixed, can't reproduce):
```bash
bd close <issue-id> --reason="no-changes: <brief explanation>"
gt done
```

This command pushes your branch, submits an MR to the merge queue, and transitions
you to IDLE. The repo-local verifier configured in merge-queue settings, and
declared by any repo contract in `.gastown/settings.json`, is the CI source of
truth. The Refinery handles the actual merge. You are done after this.
