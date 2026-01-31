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

**Filing command:**
```bash
bd create -t bug "Brief description" --parent <epic-id> -d "Detailed explanation..."
```

**Preferentially assign bugs to:**
1. Your epic
2. Another existing epic
3. The "Untracked Work" epic (create if needed)

**But DO NOT:**
1. Create a new epic

As you work on your epic you will add many tasks to it; the best way to complete these tasks is by creating polecats. You should peek at your polecats while they are running. Many valuable <FAIL>s can be <FILE>d using this information.

### Common Failure Patterns to Watch For

**Command Errors:**
- Wrong command syntax or flag names
- Missing required flags
- Commands that should exist but don't (desire paths)
- Silent failures (command exits 0 but didn't do what expected)

**Warnings & Partial Failures:**
- Warning messages during normal operations (e.g., "mtime update failed")
- Operations that succeed but emit concerning messages
- Database warnings (constraint violations, missing tables)

**Documentation Gaps:**
- Had to guess how a command works
- Missing examples for common use cases
- Outdated or incorrect docs
- Behavior differs from documentation

**Process Friction:**
- Steps that feel unnecessary or could be automated
- Repetitive manual operations
- Workflows that require multiple retries
- Confusing or inconsistent behavior

**Tooling Gaps:**
- Features that "should work" but don't exist
- Intuitive commands that fail (e.g., `gt hook clear` vs `gt unhook`)
- Missing integrations between tools
- Verbosity where there should be defaults

**Race Conditions & Timing:**
- Operations that sometimes fail, sometimes succeed
- Order-dependent behavior that isn't documented
- Stale state after restarts or handoffs

**Examples of Filing:**
```bash
# Command syntax issue
bd create -t bug "gt sling rejects valid bead ID format" --parent hq-8af330 \
  -d "When running 'gt sling hq-abc' it complains about invalid prefix"

# Warning during operation
bd create -t bug "bd sync emits mtime warning in Dolt mode" --parent hq-8af330 \
  -d "Warning: failed to update database mtime. This appears when beads.db doesn't exist."

# Documentation gap
bd create -t bug "No docs for multi-rig bead routing" --parent hq-8af330 \
  -d "Unclear how prefix-based routing (gt-/bd-/hq-) works across rigs"
```

**Remember:** Failures are information. Untracked failures are lost knowledge. File first, fix later.

---

## Agent Advice System

**Advice appears automatically in `gt prime` output.** Advice is persistent guidance that helps
agents work effectively. It's surfaced at session start, not buried in docs.

### Advice Scopes (Hierarchy)

| Scope | Audience | Example Use |
|-------|----------|-------------|
| **Global** | All agents | "Always verify git status before pushing" |
| **Rig** | Agents in a rig | "Use fimbaz account for spawning in gastown" |
| **Role** | Role type in a rig | "Polecats in beads should use go test ./..." |
| **Agent** | Specific agent | "quartz owns the CLI implementation" |

### When to Create Advice

Create advice when you discover something others should know:
- **Workarounds** for known issues
- **Conventions** for a codebase
- **Gotchas** that caused you pain
- **Ownership** assignments

```bash
# Global advice (everyone sees this)
bd advice add "Always check hook before mail" -d "Hook takes priority per GUPP"

# Rig advice (only this rig)
bd advice add "Use go test for testing" --rig beads -d "Not pytest"

# Role advice (requires rig)
bd advice add "Complete work before gt done" --rig gastown --role polecat \
  -d "Polecats should never have uncommitted changes at session end"
```

### Advice vs Beads

| Use | Advice | Bead |
|-----|--------|------|
| Persistent guidance | ✓ | |
| Known workaround | ✓ | |
| Track a bug | | ✓ |
| Track work to do | | ✓ |
| Document a convention | ✓ | |

**Rule of thumb:** Advice is "how to work here". Beads are "what work to do".

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
# For gastown:
cd /path/to/gastown && git pull && make install

# For beads:
cd /path/to/beads && git pull && make install
```

`make install` handles:
- Setting proper ldflags (version, commit hash, build time)
- Installing to ~/.local/bin/
- Proper build configuration

---

**Your Mission:** Bootstrap the "File After Fail" process by:
1. Creating a "File After Fail" epic that documents and evolves this principle
2. Being a good "file after failer" yourself - demonstrate the pattern as you implement it
