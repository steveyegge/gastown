# Gastown Architecture

## Directory Structure

```
Town (~/gt)
├── mayor/                         # Global coordinator state
│   ├── town.json                  # Town identity
│   ├── rigs.json                  # Registered rigs
│   └── overseer.json              # Human owner
├── plugins/                       # Deacon plugins
└── <rig>/                         # Project container
    ├── crew/<name>/               # Human workspaces
    ├── polecats/<name>/           # Agent worktrees
    ├── refinery/                  # Merge queue processor
    ├── witness/                   # Agent lifecycle manager
    └── .beads/                    # Issue tracking (gitignored)
```

## Local Configuration (gitignored)

These files contain your town's runtime state:

| File | Purpose |
|------|---------|
| `~/gt/mayor/town.json` | Town metadata (owner, name) |
| `~/gt/mayor/rigs.json` | Registered rigs and their prefixes |
| `~/gt/mayor/overseer.json` | Human overseer details |
| `~/gt/mayor/state.json` | Current mayor state |
| `<rig>/.beads/issues.jsonl` | Rig-level issue tracking |
| `<rig>/.beads/routes.jsonl` | Prefix routing rules |

**Check current config:**
```bash
cat ~/gt/mayor/rigs.json        # See registered rigs
cat ~/gt/mayor/town.json        # Town identity
```

## Agent Roles

| Role | Location | Purpose |
|------|----------|---------|
| **Mayor** | `~/gt/mayor/` | Global coordinator, dispatches work, handles escalations |
| **Polecat** | `<rig>/polecats/<name>/` | Worker agent executing tasks in isolated worktree |
| **Crew** | `<rig>/crew/<name>/` | Human developer workspace |
| **Witness** | `<rig>/witness/` | Per-rig monitor for polecats |
| **Refinery** | `<rig>/refinery/` | Merge queue processor |
| **Deacon** | daemon | Background coordinator, runs plugins |

**Important:** Mayor coordinates, does NOT edit code. For code changes, dispatch to crew/polecats.

## Environment Setup

Required environment variables (add to shell profile):

```bash
export BEADS_DIR="$HOME/gt/<rig>/.beads"  # Point to rig's beads
export PATH="$HOME/go/bin:$PATH"           # gt and bd binaries
```

Or prefix commands inline:
```bash
BEADS_DIR="$HOME/gt/<rig>/.beads" ~/go/bin/bd show <prefix>-XXX
```

## tmux Session Naming

Running polecats use session naming `gt-<rig>-<polecat>`:
```bash
tmux list-sessions               # Shows: gt-<rig>-<polecat>, etc.
```

## The Capability Ledger

Every completion is recorded. Every handoff is logged.

- **Work is visible:** Beads track what you actually did
- **Redemption is real:** Trajectory matters more than snapshots
- **Every completion is evidence:** Successes prove autonomous execution works

## Session End Checklist

```
[ ] git status              (check what changed)
[ ] git add <files>         (stage code changes)
[ ] bd sync                 (commit beads changes)
[ ] git commit -m "..."     (commit code)
[ ] git push                (push to remote)
[ ] HANDOFF (if incomplete):
    gt mail send mayor/ -s "🤝 HANDOFF: <brief>" -m "<context>"
```
