---
title: "GT SLING"
---

## gt sling

Assign work to an agent (THE unified work dispatch command)

### Synopsis

Sling work onto an agent's hook and start working immediately.

This is THE command for assigning work in Gas Town. It handles:
  - Existing agents (mayor, crew, witness, refinery)
  - Auto-spawning polecats when target is a rig
  - Dispatching to dogs (Deacon's helper workers)
  - Formula instantiation and wisp creation
  - Auto-convoy creation for dashboard visibility

Auto-Convoy:
  When slinging a single issue (not a formula), sling automatically creates
  a convoy to track the work unless --no-convoy is specified. This ensures
  all work appears in 'gt convoy list', even "swarm of one" assignments.

  gt sling gt-abc gastown              # Creates "Work: <issue-title>" convoy
  gt sling gt-abc gastown --no-convoy  # Skip auto-convoy creation

Merge Strategy (--merge):
  Controls how completed work lands. Stored on the auto-convoy.
  gt sling gt-abc gastown --merge=direct  # Push branch directly to main
  gt sling gt-abc gastown --merge=mr      # Merge queue (default)
  gt sling gt-abc gastown --merge=local   # Keep on feature branch

Target Resolution:
  gt sling gt-abc                       # Self (current agent)
  gt sling gt-abc crew                  # Crew worker in current rig
  gt sling gp-abc greenplace               # Auto-spawn polecat in rig
  gt sling gt-abc greenplace/Toast         # Specific polecat
  gt sling gt-abc mayor                 # Mayor
  gt sling gt-abc deacon/dogs           # Auto-dispatch to idle dog
  gt sling gt-abc deacon/dogs/alpha     # Specific dog

Spawning Options (when target is a rig):
  gt sling gp-abc greenplace --create               # Create polecat if missing
  gt sling gp-abc greenplace --force                # Ignore unread mail
  gt sling gp-abc greenplace --account work         # Use specific Claude account

Natural Language Args:
  gt sling gt-abc --args "patch release"
  gt sling code-review --args "focus on security"

The --args string is stored in the bead and shown via gt prime. Since the
executor is an LLM, it interprets these instructions naturally.

Stdin Mode (for shell-quoting-safe multi-line content):
  echo "review for security issues" | gt sling gt-abc gastown --stdin
  gt sling gt-abc gastown --stdin <<'EOF'
  Focus on:
  1. SQL injection in query builders
  2. XSS in template rendering
  EOF

  # With --args on CLI, stdin goes to --message:
  echo "Extra context here" | gt sling gt-abc gastown --args "patch release" --stdin

Formula Slinging:
  gt sling mol-release mayor/           # Cook + wisp + attach + nudge
  gt sling towers-of-hanoi --var disks=3

Formula-on-Bead (--on flag):
  gt sling mol-review --on gt-abc       # Apply formula to existing work
  gt sling shiny --on gt-abc crew       # Apply formula, sling to crew

Compare:
  gt hook <bead>      # Just attach (no action)
  gt sling <bead>     # Attach + start now (keep context)
  gt handoff <bead>   # Attach + restart (fresh context)

The propulsion principle: if it's on your hook, YOU RUN IT.

Batch Slinging:
  gt sling gt-abc gt-def gt-ghi gastown   # Sling multiple beads to a rig
  gt sling gt-abc gt-def gastown --max-concurrent 3  # Limit concurrent spawns

  When multiple beads are provided with a rig target, each bead gets its own
  polecat. This parallelizes work dispatch without running gt sling N times.
  Use --max-concurrent to throttle spawn rate and prevent Dolt server overload.

Examples:
  gt sling gt-abc gastown                    # Sling to gastown rig
  gt sling gt-abc crew                       # Sling to crew (current rig)
  gt sling gt-abc mayor                      # Sling to mayor
  gt sling gt-abc greenplace --create        # Auto-spawn polecat
  gt sling mol-release mayor/                # Sling formula
  gt sling gt-abc gt-def gastown             # Batch sling
  gt sling gt-abc --args "security review"   # With instructions

```
gt sling <bead-or-formula> [target] [flags]
```

### Options

```
      --account string       Claude Code account handle to use
      --agent string         Override agent/runtime for this sling (e.g., claude, gemini, codex, or custom alias)
  -a, --args string          Natural language instructions for the executor (e.g., 'patch release')
      --base-branch string   Override base branch for polecat worktree (e.g., 'develop', 'release/v2')
      --create               Create polecat if it doesn't exist
  -n, --dry-run              Show what would be done
      --force                Force spawn even if polecat has unread mail
      --formula string       Formula to apply (default: mol-polecat-work for polecat targets)
  -h, --help                 help for sling
      --hook-raw-bead        Hook raw bead without default formula (expert mode)
      --max-concurrent int   Limit concurrent polecat spawns in batch mode (0 = no limit)
      --merge string         Merge strategy: direct (push to main), mr (merge queue, default), local (keep on branch)
  -m, --message string       Context message for the work
      --no-boot              Skip rig boot after polecat spawn (avoids witness/refinery lock contention)
      --no-convoy            Skip auto-convoy creation for single-issue sling
      --no-merge             Skip merge queue on completion (keep work on feature branch for review)
      --on string            Apply formula to existing bead (implies wisp scaffolding)
      --owned                Mark auto-convoy as caller-managed lifecycle (no automatic witness/refinery registration)
      --ralph                Enable Ralph Wiggum loop mode (fresh context per step, for multi-step workflows)
      --stdin                Read --message and/or --args from stdin (avoids shell quoting issues)
  -s, --subject string       Context subject for the work
      --var stringArray      Formula variable (key=value), can be repeated
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt sling respawn-reset](../cli/gt_sling_respawn-reset/)	 - Reset the respawn counter for a bead

