# Gastown Workflows

## Basic Single-Agent Workflow

```bash
# 1. Create issue
bd create "Add dark mode toggle"
# Returns: <prefix>-XXX

# 2. Dispatch to agent
gt sling <prefix>-XXX --rig=<rig>

# 3. Monitor progress
gt hook                              # Show hook status
tmux capture-pane -t gt-<rig>-<polecat> -p | tail -50   # Live output

# 4. When complete
gt done
```

## Multi-Agent Parallel Work

```bash
# Create multiple issues
bd create "Frontend: dark mode toggle"    # <prefix>-1
bd create "Backend: theme API endpoint"   # <prefix>-2
bd create "Tests: theme switching"        # <prefix>-3

# Create convoy to group them
gt convoy create dark-mode --rig=<rig>
gt convoy add <prefix>-1 <prefix>-2 <prefix>-3 --convoy=dark-mode

# Dispatch (each gets own polecat)
gt sling <prefix>-1 --rig=<rig>
gt sling <prefix>-2 --rig=<rig>
gt sling <prefix>-3 --rig=<rig>

# Monitor convoy progress
gt convoy status dark-mode
```

## Review and Feedback to Polecats

When you review a polecat's work and find issues:

```bash
# 1. Check what polecat produced
cd ~/gt/<rig>/polecats/<polecat>
git diff --stat HEAD

# 2. Add feedback via bead comment
bd comments add <prefix>-XXX "Issues to fix:
1. Move TIER_LIMITS to shared module (server import in client)
2. Add unit tests for stripe.ts
3. Add integration tests for webhook endpoint"

# 3. Wake polecat and point to feedback
tmux send-keys -t gt-<rig>-<polecat> "Check bd comments for <prefix>-XXX - fixes needed" Enter

# 4. Monitor progress (repeat as needed)
sleep 30 && tmux capture-pane -t gt-<rig>-<polecat> -p | tail -80

# 5. If vitest is in watch mode, unstick it
tmux send-keys -t gt-<rig>-<polecat> C-c
tmux send-keys -t gt-<rig>-<polecat> "Fix the failing tests, then run pnpm test:unit" Enter
```

## Crew Development Workflow

Human developers work in crew spaces, separate from agent polecats:

```bash
# Work in your crew space
cd ~/gt/<rig>/crew/<name>

# Normal git workflow
git checkout -b feature/my-work
# ... make changes ...
git add . && git commit -m "feat: my changes"
git push origin feature/my-work
```

## Agent Handoff (Context Preservation)

When an agent needs to continue work in a fresh session:

```bash
gt handoff                # Saves state, signals handoff
gt resume                 # New session picks up work
```

## Merge Queue Workflow

```bash
# Agent completes work
gt done

# Add branch to merge queue
gt mq add feature/<prefix>-XXX

# Check queue status
gt mq status
```

## Debugging Agent Work

```bash
# Check what's on agent's hook
gt hook

# Check bead status
bd show <prefix>-XXX

# See polecat's live output
tmux capture-pane -t gt-<rig>-<polecat> -p -S -150 | tail -100

# Check if polecat process is alive
tmux list-panes -t gt-<rig>-<polecat> -F "#{pane_pid}: #{pane_current_command}"

# Find orphaned work
gt orphans

# Release stuck issues
gt release <prefix>-XXX
```

## Formula-Based Workflows

Formulas are reusable workflow templates stored in Beads:

```bash
# List available formulas
gt formula list

# Apply formula to issue
gt formula apply component-creation --issue=<prefix>-XXX
```
