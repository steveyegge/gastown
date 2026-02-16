# Agent Provider Interface

> Design document for the Gas Town agent provider abstraction.
> Written 2026-02-15, crew/dennis session.

## Overview

Gas Town orchestrates coding agents (Claude, Gemini, Codex, Cursor, Auggie, AMP,
OpenCode, and potentially any future CLI agent) through a unified provider
abstraction. This document describes the current integration surface, where
agent-specific logic lives, what each agent must provide (natively or via
shims), and where the abstraction leaks.

The long-term goal is a published **Agent Factory Worker Interface** that any
coding agent can implement to participate in multi-agent orchestration systems
like Gas Town.

## Current Architecture

### The Preset Registry

Every agent starts as an entry in `builtinPresets` (`internal/config/agents.go`):

```go
type AgentPresetInfo struct {
    Name                AgentPreset
    Command             string              // CLI binary
    Args                []string            // Autonomous mode flags
    Env                 map[string]string   // Agent-specific env vars
    ProcessNames        []string            // Tmux process detection
    SessionIDEnv        string              // Env var for session tracking
    ResumeFlag          string              // How to resume sessions
    ResumeStyle         string              // "flag" or "subcommand"
    SupportsHooks       bool                // Lifecycle extension system
    SupportsForkSession bool                // Session forking (Claude-only today)
    NonInteractive      *NonInteractiveConfig // Headless execution
}
```

This is the **primary registration point**. Adding a new agent is ~80% "add an
entry here." The registry is thread-safe, supports user overrides via
`settings/agents.json`, and resolves hierarchically:
town settings -> rig settings -> role_agents mapping -> fallback "claude".

### RuntimeConfig (The Runtime Layer)

`AgentPresetInfo` is the static preset. `RuntimeConfig` (`internal/config/types.go`)
is the fully resolved runtime configuration, which adds:

- **Hooks config**: Provider, directory, settings file for lifecycle extensions
- **Tmux config**: Process names, ready prompt prefix, ready delay
- **Instructions config**: Which instruction file to read (CLAUDE.md vs AGENTS.md)
- **Prompt mode**: How the initial prompt is delivered ("arg" vs "none")
- **Session config**: Session ID env, config dir env

`RuntimeConfigFromPreset()` converts a preset into a RuntimeConfig.
`normalizeRuntimeConfig()` fills defaults via ~11 `default*()` functions.

## The Integration Surface

### What Gas Town Needs From an Agent

Gas Town interacts with agents through these capabilities:

| Capability | Required | How Gas Town Uses It | Native API? |
|------------|----------|----------------------|-------------|
| **Launch** | Yes | `command + args` to start in tmux | Yes (CLI) |
| **Process detection** | Yes | Check if agent is alive in tmux pane | No - tmux `pane_current_command` |
| **Readiness detection** | Yes | Know when agent is ready for input | No - prompt prefix or delay heuristic |
| **Text input** | Yes | Send work instructions, nudges | No - tmux `send-keys` |
| **Context injection** | Yes | Prime agent with Gas Town identity/role | Varies (hooks, prompt arg, or nudge) |
| **Lifecycle hooks** | Preferred | Session start, tool guards, shutdown | Claude, OpenCode, Pi have native hooks |
| **Session resume** | Preferred | Resume after context cycling | Most agents support `--resume` |
| **Non-interactive mode** | Preferred | Headless execution for dogs/formulas | Most agents support `-p` or subcommand |
| **Session fork** | Optional | Seance (talk to past sessions) | Claude only |
| **Output peek** | Desired | Read agent's current output for status | No - tmux `capture-pane` |
| **Autonomous mode** | Yes | Skip confirmation prompts | Varies (`--yolo`, `--dangerously-skip-permissions`, env var) |

### What Gas Town Fills In (Shims)

For capabilities agents don't expose natively, Gas Town provides shims:

**Tmux-based messaging**: All agents receive work instructions via tmux
`send-keys`. This is the universal fallback for agents without prompt injection
APIs. It works but is brittle (timing-sensitive, no delivery confirmation).

**Tmux-based process detection**: Gas Town checks `pane_current_command` against
known process names to determine if an agent is alive. This is why each preset
declares `ProcessNames` — there's no standard "am I running?" API.

**Tmux-based readiness**: Two strategies:
1. **Prompt prefix matching** — scan pane content for a known prompt character
   (Claude uses `❯`). Reliable but requires knowing the prompt format.
2. **Delay-based** — wait N milliseconds and hope. Used for agents with TUI
   interfaces that don't have scannable prompts (OpenCode, Codex).

**Startup fallback commands**: For agents without hooks, Gas Town sends
`gt prime && gt mail check --inject` via tmux as a substitute for the
session_start hook. This works but adds latency and fragility.

**Output peeking**: `tmux capture-pane` lets Gas Town read what an agent is
displaying, used by the Witness for health checks. No agent provides an API
for "what are you working on?" so tmux screen-scraping is the universal answer.

## Where Agent-Specific Logic Lives

### Centralized (Good)

| Location | What | Pattern |
|----------|------|---------|
| `config/agents.go` | Preset registry | Map entry per agent |
| `config/types.go` | Default values | ~11 `default*()` switch functions |
| `config/loader.go` | `fillRuntimeDefaults` | Auto-fill hooks/tmux for custom configs |
| `runtime/runtime.go` | Startup fallback matrix | Hooks x Prompt capability grid |

### Scattered (Needs Work)

| Location | What | Problem |
|----------|------|---------|
| `runtime/runtime.go:39-50` | Hook installation switch | Only knows "claude", "opencode" — code change per new agent |
| `templates/commands/provision.go` | Slash command provisioning | Manual `Agents` map — only "claude", "opencode" registered |
| `cmd/seance.go:217` | Session forking | Hardcodes `exec.Command("claude", "--fork-session", ...)` with no capability check |
| `cmd/sling_helpers.go:273` | Permission bypass | `if agentName == "claude"` for skip-permissions warning |
| `runtime/runtime.go:71` | Session ID fallback | Hardcodes `CLAUDE_SESSION_ID` as final fallback |

## Capability Matrix (Current Agents)

| Agent | Hooks | Resume | Non-Interactive | Fork | Prompt Mode | Process Names |
|-------|-------|--------|-----------------|------|-------------|---------------|
| Claude | Yes (settings.json) | `--resume` (flag) | Native | Yes | arg | node, claude |
| Gemini | Yes | `--resume` (flag) | `-p` | No | arg | gemini |
| Codex | No | `resume` (subcmd) | `exec` subcmd | No | none | codex |
| Cursor | No | `--resume` (flag) | `-p` | No | arg | cursor-agent |
| Auggie | No | `--resume` (flag) | No | No | arg | auggie |
| AMP | No | `threads continue` (subcmd) | No | No | arg | amp |
| OpenCode | Yes (plugin JS) | No | `run` subcmd | No | none | opencode, node, bun |

## What an Agent Factory Worker Interface Would Look Like

If we were designing an industry-standard interface for coding agents to
participate in orchestration systems, it would formalize the capabilities
above. The key insight is that Gas Town already knows what the interface
*should* be — it's the set of things we currently shim via tmux.

### Tier 1: Required (must implement to participate)

```
interface AgentWorker {
    // Lifecycle
    start(workDir string, env map[string]string) -> Process
    isReady() -> bool                    // Currently: prompt prefix or delay
    isAlive() -> bool                    // Currently: pane_current_command

    // Communication
    sendMessage(text string) -> error    // Currently: tmux send-keys
    getStatus() -> AgentStatus           // Currently: tmux capture-pane

    // Identity
    name() -> string
    version() -> string
}
```

### Tier 2: Preferred (enables full orchestration)

```
interface AgentWorkerExtended {
    // Context injection
    injectContext(context string) -> error   // Currently: hooks or prompt arg
    onSessionStart(callback) -> void        // Currently: hooks framework

    // Session management
    resume(sessionID string) -> Process
    sessionID() -> string                   // Currently: env var capture

    // Tool guards
    onToolCall(callback) -> void            // Currently: Claude/OpenCode hooks

    // Autonomous mode
    enableAutoApprove() -> void             // Currently: --yolo / --dangerously-skip-permissions
}
```

### Tier 3: Advanced (nice to have)

```
interface AgentWorkerAdvanced {
    // Session forking (seance)
    forkSession(sessionID string) -> Process

    // Non-interactive execution
    exec(prompt string) -> Result

    // Cost tracking
    getUsage() -> UsageReport

    // Structured output
    getOutput(format string) -> string
}
```

### What We Provide Regardless (The Tmux Shim Layer)

Even without any native API, Gas Town can orchestrate any agent that runs
in a terminal. The tmux shim provides:

- **Messaging**: `send-keys` for input delivery
- **Peeking**: `capture-pane` for output observation
- **Liveness**: `pane_current_command` for process detection
- **Readiness**: Prompt scanning or delay-based heuristics
- **Session management**: Tmux session create/attach/kill

This is the "zero API" floor — any CLI tool gets basic orchestration for
free. The interface tiers above describe what agents can *optionally*
implement to get tighter integration (reliable delivery, context injection,
tool guards, session continuity).

## Design Principles

### Discover, Don't Track
Agent liveness is derived from tmux state, not tracked in a database.
Process names and ready prompts are observed, not self-reported.

### ZFC: Zero Framework Cognition
The agent decides what to do with instructions. Gas Town provides transport
(tmux, hooks, nudges) but doesn't make decisions for agents. The interface
is about communication channels, not control flow.

### Graceful Degradation
Every capability has a fallback:
- No hooks? -> Startup fallback commands via tmux
- No prompt mode? -> Nudge delivery
- No resume? -> Fresh session with handoff mail
- No process API? -> Tmux pane_current_command

The system works (less reliably) with zero native API support.

## Known Issues

1. **Hook installation requires code changes** — `runtime.go` switch statement
   only handles "claude" and "opencode". New hook providers need a code change.
   Should be data-driven: hook provider supplies an installer function or
   the hook file is a generic template.

2. **Seance hardcodes Claude** — `seance.go:217` calls `exec.Command("claude")`
   directly. Should check `SupportsForkSession` and use the resolved agent command.

3. **Slash commands only provision for 2 agents** — `commands/provision.go` only
   knows about "claude" and "opencode". Should derive from the preset registry
   or use a generic provisioning path.

4. **11 scattered default functions** — `types.go` has 11 separate `default*()`
   functions that switch on provider strings. These should be consolidated into
   the preset struct itself (each preset declares its own defaults).

5. **Session ID fallback hardcodes Claude** — `runtime.go:71` falls back to
   `CLAUDE_SESSION_ID`. Should use the resolved agent's `SessionIDEnv`.

6. **No build tags for Linux-only code** — `/proc/` reading in status.go (from
   PR #1450) has no `//go:build linux` guard. Silently does nothing on macOS.
