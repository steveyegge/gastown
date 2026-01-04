# Gastown Commands Reference

## gt (Gas Town) Commands

Gas Town manages multi-agent workspaces called rigs. It coordinates agent spawning, work distribution, and communication across distributed teams of AI agents.

### Work Management

```bash
gt convoy list                           # Dashboard of active work
gt convoy create <name> <issues>         # Create convoy for batch work
gt convoy status <id>                    # Detailed convoy progress
gt convoy add <issues> --convoy=<name>   # Add issues to convoy

gt sling <issue> --rig=<rig>             # Dispatch to agent (THE dispatch command)
gt unsling <issue>                       # Remove from agent's hook
gt hook                                  # Show current work on hook
gt hook attach <mail-id>                 # Hook mail as assignment
gt done                                  # Signal work ready for merge queue
gt handoff -m "context"                  # Hand off to fresh session

gt orphans                               # Find lost polecat work
gt release <issue>                       # Release stuck issues back to pending
gt park                                  # Park work on a gate for async resumption
gt resume                                # Resume from parked work or handoff

gt mol list                              # List molecules
gt mol status                            # Current molecule state
gt formula list                          # List workflow templates
gt formula apply <name> --issue=<id>     # Apply formula to issue

gt synthesis                             # Manage convoy synthesis steps
gt gate                                  # Gate coordination commands
```

### Agent Management

```bash
gt agents                                # Switch between Gas Town agent sessions
gt polecat list [rig]                    # List polecats in a rig
gt session                               # Manage polecat sessions
gt role                                  # Show or manage agent role

gt mayor                                 # Manage the Mayor session
gt witness                               # Manage the polecat monitoring agent
gt refinery                              # Manage the merge queue processor
gt deacon                                # Manage the Deacon session
gt boot                                  # Manage Boot (Deacon watchdog)
gt dog                                   # Manage dogs (Deacon's helper workers)

gt callbacks                             # Handle agent callbacks
```

### Communication

```bash
gt mail inbox                            # Check your messages
gt mail read <id>                        # Read a specific message
gt mail send <addr> -s "Subj" -m "Msg"   # Send mail

gt broadcast "message"                   # Send nudge to all workers
gt nudge <target> "message"              # Send message to polecat/deacon
gt peek <target>                         # View recent output from polecat
gt escalate <issue>                      # Escalate to human overseer

gt notify                                # Set notification level
gt dnd                                   # Toggle Do Not Disturb mode
```

### Rig Management

```bash
gt rig list                              # List all rigs
gt rig add <name> <git-url>              # Add new rig
gt rigs                                  # Alias for rig list
gt status                                # Overall town status
```

### Services

```bash
gt daemon                                # Manage the Gas Town daemon
gt down                                  # Stop all Gas Town services
gt prime                                 # Recovery after compaction/clear/new session
```

### Crew (Human Workspaces)

```bash
gt crew add <name> --rig=<rig>           # Create workspace
gt crew list --rig=<rig>                 # List crew members
gt worktree <rig>                        # Create worktree for cross-rig fixes
```

### Merge Queue

```bash
gt mq status                             # Merge queue status
gt mq add <branch>                       # Add to queue
```

---

## bd (Beads) Commands

Issues chained together like beads. A lightweight issue tracker with first-class dependency support.

### Working With Issues

```bash
bd create "Description"                  # Create issue (uses rig prefix)
bd create-form                           # Create via interactive form
bd q "Quick capture"                     # Create and output only ID

bd list                                  # List issues
bd list --status=open                    # Filter by status
bd show <id>                             # Show issue details
bd search "query"                        # Search by text

bd update <id> --field=value             # Update issue fields
bd edit <id>                             # Edit in $EDITOR
bd set-state <id> <state>                # Set operational state

bd close <id>                            # Close issue
bd reopen <id>                           # Reopen closed issue
bd delete <id>                           # Delete issue and clean refs

bd label <id> <label>                    # Manage labels
bd comments list <id>                    # View comments
bd comments add <id> "message"           # Add comment (for polecat feedback)
```

### Dependencies & Structure

```bash
bd dep add <issue> <depends-on>          # Add dependency (X needs Y)
bd dep remove <issue> <dep>              # Remove dependency
bd blocked                               # Show blocked issues
bd ready                                 # Issues ready to work (no blockers)

bd graph                                 # Display dependency graph
bd epic                                  # Epic management
bd swarm                                 # Swarm management for structured epics

bd duplicate <id> <original>             # Mark as duplicate
bd duplicates                            # Find duplicate issues
bd supersede <old> <new>                 # Mark as superseded
```

### Views & Reports

```bash
bd status                                # Database overview and statistics
bd count                                 # Count matching issues
bd stale                                 # Show stale issues
bd activity                              # Real-time molecule state feed
```

### Sync & Data

```bash
bd sync                                  # Synchronize with git remote
bd daemon                                # Manage background sync daemon
bd export                                # Export to JSONL
bd import                                # Import from JSONL
bd merge                                 # Git merge driver for beads files
bd restore <id>                          # Restore from git history
```

### Maintenance

```bash
bd rename-prefix <new>                   # Rename prefix for all issues
bd repair                                # Repair corrupted database
```

---

## tmux Polecat Interaction

Running polecats operate in tmux sessions named `gt-<rig>-<polecat>`:

```bash
# List active polecat sessions
tmux list-sessions                       # Shows gt-<rig>-<polecat>, etc.

# Monitor polecat output
tmux capture-pane -t gt-<rig>-<polecat> -p | tail -50

# Send instruction to polecat
tmux send-keys -t gt-<rig>-<polecat> "Your instruction here" Enter

# Cancel stuck operation (sends Ctrl+C)
tmux send-keys -t gt-<rig>-<polecat> C-c

# Attach to polecat session (interactive)
tmux attach -t gt-<rig>-<polecat>

# Check if polecat process is alive
tmux list-panes -t gt-<rig>-<polecat> -F "#{pane_pid}: #{pane_current_command}"
```

---

## Environment Variables

```bash
BEADS_DIR="<path>"                       # Point to rig's beads directory
BD_DEBUG_ROUTING=1                       # Debug prefix routing
```

---

## Dependency Gotcha

**Temporal language inverts dependencies.** "Phase 1 blocks Phase 2" is backwards.

```bash
# WRONG: bd dep add phase1 phase2  (temporal: "1 before 2")
# RIGHT: bd dep add phase2 phase1  (requirement: "2 needs 1")
```

**Rule**: Think "X needs Y", not "X comes before Y". Verify with `bd blocked`.
