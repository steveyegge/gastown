# Gas Town Glossary

Gas Town is an agentic development environment for managing multiple Claude Code instances simultaneously using the `gt` and `bd` (Beads) binaries, coordinated with tmux in git-managed directories.

## Core Principles

### MEOW (Molecular Expression of Work)
Breaking large goals into detailed instructions for agents. Supported by Beads, Epics, Formulas, and Molecules. MEOW ensures work is decomposed into trackable, atomic units that agents can execute autonomously.

### GUPP (Gas Town Universal Propulsion Principle)
"If there is work on your Hook, YOU MUST RUN IT." This principle ensures agents autonomously proceed with available work without waiting for external input. GUPP is the heartbeat of autonomous operation.

### NDI (Nondeterministic Idempotence)
The overarching goal ensuring useful outcomes through orchestration of potentially unreliable processes. Persistent Beads and oversight agents (Witness, Deacon) guarantee eventual workflow completion even when individual operations may fail or produce varying results.

## Environments

### Town
The management headquarters (e.g., `~/gt/`). The Town coordinates all workers across multiple Rigs and houses town-level agents like Mayor and Deacon.

### Rig
A project-specific Git repository under Gas Town management. Each Rig has its own Polecats, Refinery, Witness, and Crew members. Rigs are where actual development work happens.

## Town-Level Roles

### Mayor
Chief-of-staff agent responsible for initiating Convoys, coordinating work distribution, and notifying users of important events. The Mayor operates from the town level and has visibility across all Rigs.

### Deacon
Daemon beacon running continuous Patrol cycles. The Deacon ensures worker activity, monitors system health, and triggers recovery when agents become unresponsive. Think of the Deacon as the system's watchdog.

### Dogs
The Deacon's crew of maintenance agents handling background tasks like cleanup, health checks, and system maintenance.

### Boot (the Dog)
A special Dog that checks the Deacon every 5 minutes, ensuring the watchdog itself is still watching. This creates a chain of accountability.

## Rig-Level Roles

### Polecat
Worker agents with persistent identity but ephemeral sessions. Each polecat has a permanent agent bead, CV chain, and work history that accumulates across assignments. Sessions and sandboxes are ephemeral — spawned for specific tasks, cleaned up on completion — but the identity persists. They work in isolated git worktrees to avoid conflicts.

### Refinery
Manages the Merge Queue for a Rig. The Refinery intelligently merges changes from Polecats, handling conflicts and ensuring code quality before changes reach the main branch.

### Witness
Patrol agent that oversees Polecats and the Refinery within a Rig. The Witness monitors progress, detects stuck agents, and can trigger recovery actions.

### Crew
Long-lived, named agents for persistent collaboration. Unlike ephemeral Polecats, Crew members maintain context across sessions and are ideal for ongoing work relationships.

## Work Units

### Bead
Git-backed atomic work unit stored in Dolt. Beads are the fundamental unit of work tracking in Gas Town. They can represent issues, tasks, epics, or any trackable work item.

### Formula
TOML-based workflow source template. Formulas define reusable patterns for common operations like patrol cycles, code review, or deployment.

### Protomolecule
A template class for instantiating Molecules. Protomolecules define the structure and steps of a workflow without being tied to specific work items.

### Molecule
Durable chained Bead workflows. Molecules represent multi-step processes where each step is tracked as a Bead. They survive agent restarts and ensure complex workflows complete.

### Wisp
Ephemeral Beads destroyed after runs. Wisps are lightweight work items used for transient operations that don't need permanent tracking.

### Hook
A special pinned Bead for each agent. The Hook is an agent's primary work queue - when work appears on your Hook, GUPP dictates you must run it.

## Postings

### Posting
A behavioral specialization that augments an agent's base role with additional responsibilities and context. Postings inject a role-specific prompt template into the agent's session at prime time. There are two layers: session-level (assumed) postings and persistent postings.

### Assumed Posting
A transient, session-level posting set via `gt posting assume <name>`. Stored in `.runtime/posting` and automatically cleared on handoff, polecat completion, or explicit drop. An assumed posting cannot coexist with a persistent posting — if a crew member has a persistent posting, `gt posting assume` is blocked until the persistent posting is cleared.

### Persistent Posting
A posting assigned to a worker through rig configuration via `gt crew post <name> <posting>`. Stored in `RigSettings.WorkerPostings` and applied on every session start. A persistent posting blocks `gt posting assume` until cleared with `gt crew post <name> --clear`.

### Dispatcher (built-in posting)
A posting that augments a worker with coordination responsibilities: routing incoming beads to the right polecat, load-balancing work across workers, tracking dependencies, triaging priority, and aggressively unblocking stalled work.

### Inspector (built-in posting)
A posting that augments a worker with quality assurance responsibilities: reviewing diffs for correctness and security, enforcing codebase conventions, detecting regressions, and verifying quality gates pass before work enters the merge queue.

### Scout (built-in posting)
A posting that augments a worker with exploration and research responsibilities: mapping unfamiliar areas of the codebase, running research spikes, analyzing the impact of proposed changes, and persisting findings to beads so they survive session death.

## Workflow Commands

### Convoy
Primary work-order wrapping related Beads. Convoys group related tasks together and can be assigned to multiple workers. Created with `gt convoy create`.

### Slinging
Assigning work to agents via `gt sling`. When you sling work to a Polecat or Crew member, you're putting it on their Hook for execution.

### Nudging
Real-time messaging between agents with `gt nudge`. Nudges allow immediate communication without going through the mail system.

### Handoff
Agent session refresh via `/handoff`. When context gets full or an agent needs a fresh start, handoff transfers work state to a new session.

### Seance
Communicating with previous sessions via `gt seance`. Allows agents to query their predecessors for context and decisions from earlier work.

### Patrol
Ephemeral loop maintaining system heartbeat. Patrol agents (Deacon, Witness) continuously cycle through health checks and trigger actions as needed.

---

*This glossary was contributed by [Clay Shirky](https://github.com/cshirky) in [Issue #80](https://github.com/steveyegge/gastown/issues/80).*
