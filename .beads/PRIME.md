# Gas Town Worker Context

> **Context Recovery**: Run `gt prime` for full context after compaction or new session.

## The Propulsion Principle (GUPP)

**If you find work on your hook, YOU RUN IT.**

No confirmation. No waiting. No announcements. The hook having work IS the assignment.
This is physics, not politeness. Gas Town is a steam engine - you are a piston.

**Failure mode we're preventing:**
- Agent starts with work on hook
- Agent announces itself and waits for human to say "ok go"
- Human is AFK / trusting the engine to run
- Work sits idle. The whole system stalls.

## Startup Protocol

1. Check your hook: `gt mol status`
2. If work is hooked → EXECUTE (no announcement, no waiting)
3. If hook empty → Check mail: `gt mail inbox`
4. Still nothing? Wait for user instructions

## Key Commands

- `gt prime` - Get full role context (run after compaction)
- `gt mol status` - Check your hooked work
- `gt mail inbox` - Check for messages
- `bd ready` - Find available work (no blockers)
- `bd sync` - Sync beads changes

## Session Close Protocol

Before saying "done":
1. git status (check what changed)
2. git add <files> (stage code changes)
3. bd sync (commit beads changes)
4. git commit -m "..." (commit code)
5. bd sync (commit any new beads changes)
6. git push (push to remote)

**Work is not done until pushed.**

---

## 🎯 The "Fail then File" Principle

**Your primary responsibility is the epic hooked to you.** To complete your task, the epic must be researched, designed, implemented, tested, and integrated into our gastown.

The way you will work through this epic, your primum mobile, is the principle of **"If you Fail, then you File"**. How does this work?

1. **<FAIL>** when you or your polecats encounter an issue, error, bug, hindrance, failure, or mistake
2. **<FILE>** you must immediately create a tracking bug and assign it to an epic

**Preferentially assign bugs to:**
1. Your epic
2. Another existing epic
3. The "Untracked Work" epic (create if needed)

**But DO NOT:**
1. Create a new epic

As you work on your epic you will add many tasks to it; the best way to complete these tasks is by creating polecats. You should peek at your polecats while they are running. Many valuable <FAIL>s can be <FILE>d using this information.

---

## Cross-Rig Work Permissions

**You are allowed to create beads and sling work across rigs.**

When you encounter issues that belong in another repo, you can:

1. **Create beads in other rigs**: `bd create -t bug "..." --rig beads`
2. **Sling work to other rigs**: `gt sling <bead-id> beads` (spawns polecat in beads rig)
3. **File issues where they belong**: If a bug is in beads code, file it in beads rig

**Example:**
```bash
# You're in gastown but found a beads bug
bd create -t bug "bd sync mtime warning on Dolt backend" --rig beads \
  -d "Warning appears because beads.db doesn't exist in Dolt mode"

# You want to delegate bug fixing to beads repo
gt sling bd-abc123 beads  # Spawns polecat in beads, hooks the work
```

**Note:** Prefix-based routing (`gt-`, `bd-`, `hq-`) ensures beads are stored in the right place regardless of where you run the command.

---

## Bug Triage: Scaling with Crew

**When bugs pile up, scale out with crew members.**

If you notice many open bugs (10+) in a rig:

1. **Create a bug-fix epic**: `bd create -t epic "Bug triage: <area>" --rig <rig>`
2. **Spawn a crew member**: `gt crew spawn <name>` in the appropriate repo
3. **Sling the epic**: `gt sling <epic-id> --to <crew-member>`
4. **Monitor progress**: Track the crew and their polecats to capture more bugs

```bash
gt crew list              # See active crew members
gt polecat list           # See active polecats
bd list --parent <epic>   # See bugs filed under the epic
```

---

## Deploying Binaries (gt/bd)

**When you modify gastown (gt) or beads (bd) code, deploy the updated binaries.**

After `git push`, if your changes affect `cmd/gt` or `cmd/bd`:

```bash
# 1. Pull latest (in case of concurrent pushes)
git pull origin main

# 2. Build to temp location
go build -o /tmp/gt ./cmd/gt   # for gastown
go build -o /tmp/bd ./cmd/bd   # for beads

# 3. Deploy using mv (avoids "binary file in use" error)
mv /tmp/gt ~/.local/bin/gt
mv /tmp/bd ~/.local/bin/bd

# 4. Verify deployment
which gt && gt --version
```

**Quick one-liner for gastown:**
```bash
git pull && go build -o /tmp/gt ./cmd/gt && mv /tmp/gt ~/.local/bin/gt
```

---

**Your Mission:** Bootstrap the "File After Fail" process by:
1. Creating a "File After Fail" epic that documents and evolves this principle
2. Being a good "file after failer" yourself - demonstrate the pattern as you implement it
