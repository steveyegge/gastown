# Gastown Concepts: Claude Code Coupling Analysis

> Comprehensive analysis of all major Gastown concepts, their Claude Code dependencies, 
> and required abstractions for Opencode support
> 
> **Purpose**: Identify minimal abstractions needed to support both Claude Code and Opencode
> **Status**: Planning
> **Created**: 2026-01-16
> **Related**: [opencode-orchestration.md](opencode-orchestration.md)

## Overview

This document analyzes each major Gastown concept to identify:
1. **What it does** - Core functionality and purpose
2. **Claude Code integration** - How it currently depends on Claude Code
3. **Coupling level** - How tightly bound to Claude Code (None/Low/Medium/High)
4. **Abstraction needed** - What needs to change for Opencode support
5. **Implementation approach** - Minimal changes required

**Key Principle**: We're not adding new concepts, but making minimal abstractions to support both backends.

---

## Town-Level Concepts

### Mayor 🎩

**What it does**:
- Primary AI coordinator for the entire workspace
- Creates convoys and coordinates work distribution
- Notifies users of important events
- Operates from town level with visibility across all rigs

**Claude Code Integration**:
```go
// internal/mayor/manager.go
func (m *Manager) Start() error {
    // Uses tmux to spawn Claude Code session
    t := tmux.NewTmux()
    
    // Installs Claude-specific settings.json
    if err := claude.EnsureSettingsForRole(mayorDir, "mayor"); err != nil {
        return err
    }
    
    // Starts Claude via tmux
    sessionID := fmt.Sprintf("gt-mayor")
    return t.NewSession(sessionID, "claude --dangerously-skip-permissions", mayorDir)
}
```

**Current Dependencies**:
- `internal/claude` package for settings installation
- Claude-specific hooks (SessionStart, Compaction)
- Claude command invocation via tmux
- `.claude/settings.json` configuration

**Coupling Level**: 🔴 **HIGH** - Direct Claude Code dependencies throughout

**Abstraction Needed**:

1. **Runtime abstraction** (already exists partially in `internal/runtime`):
```go
// Use runtime config instead of hardcoded Claude
func (m *Manager) Start() error {
    rc := config.LoadRuntimeConfig(mayorDir)
    
    // Install runtime-specific hooks/plugins
    if err := runtime.EnsureSettingsForRole(mayorDir, "mayor", rc); err != nil {
        return err
    }
    
    // Start via runtime-agnostic command
    cmd := rc.BuildCommand()
    return t.NewSession(sessionID, cmd, mayorDir)
}
```

2. **Hook/Plugin abstraction** (partially exists):
```go
// internal/runtime/runtime.go (already has this pattern)
switch rc.Hooks.Provider {
case "claude":
    return claude.EnsureSettingsForRoleAt(...)
case "opencode":
    return opencode.EnsurePluginAt(...)
}
```

**Implementation Approach**:
- ✅ **Already abstracted**: Runtime config system exists
- ✅ **Already abstracted**: Hook/plugin installation logic exists
- ⚠️ **Needs update**: Mayor manager should use `runtime.EnsureSettingsForRole` instead of calling `claude.` directly
- ⚠️ **Needs update**: Session startup should use `RuntimeConfig.BuildCommand()`

**Changes Required**: **MINIMAL** - Update 2-3 call sites in `internal/mayor/manager.go`

---

### Deacon 🔔

**What it does**:
- Daemon beacon running continuous patrol cycles
- Monitors system health and worker activity
- Triggers recovery when agents become unresponsive
- Runs Dogs for maintenance tasks

**Claude Code Integration**:
```go
// Similar to Mayor - uses Claude for the agent runtime
// But patrol logic itself is runtime-agnostic
```

**Current Dependencies**:
- Session spawning via Claude
- Hook installation for autonomous operation
- Nudge mechanism via tmux send-keys

**Coupling Level**: 🟡 **MEDIUM** - Patrol logic is independent, but spawning is Claude-specific

**Abstraction Needed**:
Same as Mayor - use runtime config for spawning. Patrol logic already runtime-agnostic.

**Implementation Approach**:
- ✅ **Already abstracted**: Patrol cycle logic in `internal/deacon/` is runtime-agnostic
- ⚠️ **Needs update**: Session spawning should use runtime config
- ✅ **Already works**: Nudging via tmux works for any runtime

**Changes Required**: **MINIMAL** - Similar to Mayor, update spawn logic

---

### Dogs 🐕

**What it does**:
- Maintenance agents handling background tasks
- Cleanup, health checks, system maintenance
- Dispatched by Deacon for infrastructure work

**Claude Code Integration**:
Dogs are spawned similar to other agents via Claude.

**Current Dependencies**:
- Session spawning (Claude-specific currently)
- Work assignment via hooks/mail

**Coupling Level**: 🟡 **MEDIUM** - Dog logic is independent, spawning is not

**Abstraction Needed**:
Same runtime abstraction as other agents.

**Implementation Approach**:
- ✅ **Already abstracted**: Dog tasks are runtime-agnostic
- ⚠️ **Needs update**: Dog spawning via runtime config

**Changes Required**: **MINIMAL**

---

## Rig-Level Concepts

### Polecat 🦨

**What it does**:
- Ephemeral worker agents for specific tasks
- Spawned on-demand, complete work, then cleaned up
- Work in isolated git worktrees
- Produce merge requests

**Claude Code Integration**:
```go
// internal/polecat/session_manager.go
func (m *SessionManager) Start(polecat string, opts *SessionStartOptions) error {
    // Create worktree
    // Install Claude settings
    // Start Claude session via tmux
    cmd := opts.Command
    if cmd == "" {
        cmd = "claude --dangerously-skip-permissions"
    }
    return m.tmux.NewSession(sessionName, cmd, workDir)
}
```

**Current Dependencies**:
- Claude hook installation (`claude.EnsureSettings`)
- Claude command invocation
- SessionStart hook for context injection
- Tmux for session management

**Coupling Level**: 🔴 **HIGH** - Multiple Claude-specific integration points

**Abstraction Needed**:

1. **Use RuntimeConfig for spawning**:
```go
func (m *SessionManager) Start(polecat string, opts *SessionStartOptions) error {
    rc := config.LoadRuntimeConfig(m.rig.Path)
    
    // Runtime-agnostic hook/plugin installation
    if err := runtime.EnsureSettingsForRole(workDir, "polecat", rc); err != nil {
        return err
    }
    
    // Runtime-agnostic command
    cmd := rc.BuildCommand()
    return m.tmux.NewSession(sessionName, cmd, workDir)
}
```

2. **SessionManager should be runtime-agnostic**:
Already mostly is - just uses tmux. Main coupling is in `Start()` method.

**Implementation Approach**:
- ✅ **Already abstracted**: Git worktree logic is runtime-agnostic
- ✅ **Already abstracted**: RuntimeConfig exists for command building
- ⚠️ **Needs update**: `Start()` method should use runtime config
- ⚠️ **Needs update**: Hook/plugin installation should go through `runtime` package

**Changes Required**: **LOW** - Update `Start()` method in `internal/polecat/session_manager.go`

---

### Witness 👁️

**What it does**:
- Patrol agent overseeing Polecats and Refinery
- Monitors progress and detects stuck agents
- Triggers recovery actions
- One per rig

**Claude Code Integration**:
```go
// internal/witness/manager.go
func (m *Manager) Start() error {
    if err := claude.EnsureSettingsForRole(witnessParentDir, "witness"); err != nil {
        return err
    }
    // Start Claude session
}
```

**Current Dependencies**:
- Claude settings installation
- Claude session spawning

**Coupling Level**: 🟡 **MEDIUM** - Monitoring logic is independent

**Abstraction Needed**:
Same runtime abstraction pattern.

**Implementation Approach**:
- ✅ **Already abstracted**: Monitoring logic is runtime-agnostic
- ⚠️ **Needs update**: Session spawning

**Changes Required**: **MINIMAL**

---

### Refinery 🏭

**What it does**:
- Manages merge queue for a rig
- Intelligently merges Polecat changes
- Handles conflicts and ensures code quality
- One per rig

**Claude Code Integration**:
Similar to Witness - spawns via Claude.

**Current Dependencies**:
- Claude session spawning
- Git operations (runtime-agnostic)

**Coupling Level**: 🟡 **MEDIUM** - Merge logic is independent

**Abstraction Needed**:
Runtime abstraction for spawning.

**Implementation Approach**:
- ✅ **Already abstracted**: Merge logic is runtime-agnostic
- ⚠️ **Needs update**: Session spawning

**Changes Required**: **MINIMAL**

---

### Crew 👤

**What it does**:
- Long-lived, named agents for humans
- Personal workspace within a rig
- Maintains context across sessions
- Full git clones (not worktrees)

**Claude Code Integration**:
Less structured than Polecats - crew members can start sessions manually.

**Current Dependencies**:
- Optional: Claude hooks if using Claude
- Otherwise: manual invocation

**Coupling Level**: 🟢 **LOW** - Mostly user-controlled

**Abstraction Needed**:
Minimal - crew is already flexible.

**Implementation Approach**:
- ✅ **Already flexible**: Crew can use any runtime
- ✅ **Already works**: Hook/plugin installation is optional
- ℹ️ **Enhancement**: Could offer "setup crew for opencode" helper

**Changes Required**: **NONE** (already flexible) or **MINIMAL** (optional helper)

---

## Work Tracking Concepts

### Beads 📿

**What it does**:
- Git-backed atomic work units
- Fundamental tracking system (issues, tasks, epics)
- JSONL storage format
- Completely runtime-agnostic

**Claude Code Integration**:
**NONE** - Beads are pure data storage.

**Current Dependencies**:
None - operates via `bd` CLI (separate binary).

**Coupling Level**: 🟢 **NONE** - Already runtime-agnostic

**Abstraction Needed**: 
**NONE** - Beads work with any runtime.

**Implementation Approach**:
No changes needed.

**Changes Required**: **NONE**

---

### Hooks 🪝

**What it does**:
- Special pinned Bead for each agent
- Agent's primary work queue
- GUPP: "If work is on your Hook, you must run it"

**Claude Code Integration**:
The *Hook* (Bead) is runtime-agnostic. The confusion is with Claude *hooks* (settings.json events).

**Current Dependencies**:
- Hook Bead: None (it's just a Bead)
- Claude hooks (SessionStart, etc.): Claude-specific

**Coupling Level**: 🟢 **NONE** (Hook Bead) / 🔴 **HIGH** (Claude hooks)

**Abstraction Needed**:

**Clarification needed**: "Hook" means two things:
1. **Hook Bead** - Work queue (runtime-agnostic ✅)
2. **Claude hooks** - Settings.json event handlers (Claude-specific ❌)

For Opencode:
- Hook Bead works as-is ✅
- Need Opencode plugin events instead of Claude hooks ⚠️

**Implementation Approach**:
- ✅ **Already abstracted**: Hook Bead is just data
- ✅ **Already abstracted**: `runtime.EnsureSettingsForRole` handles both Claude hooks and Opencode plugins
- ℹ️ **Documentation**: Clarify "Hook" terminology (work queue vs event hooks)

**Changes Required**: **NONE** (technical) / **DOCUMENTATION** (terminology)

---

### Convoy 🚚

**What it does**:
- Tracks batched work across rigs
- Groups related issues/tasks
- Provides dashboard visibility
- Notifies on completion

**Claude Code Integration**:
**NONE** - Convoy is pure tracking logic.

**Current Dependencies**:
- Beads for storage (runtime-agnostic)
- Mail for notifications (runtime-agnostic)

**Coupling Level**: 🟢 **NONE** - Already runtime-agnostic

**Abstraction Needed**:
**NONE** - Convoys work with any runtime.

**Implementation Approach**:
No changes needed.

**Changes Required**: **NONE**

---

### Molecules 🧪

**What it does**:
- Durable chained Bead workflows
- Multi-step processes with state
- Survive agent restarts
- Based on Formula templates

**Claude Code Integration**:
**NONE** - Molecules are workflow state machines.

**Current Dependencies**:
- Beads for storage
- Formulas for templates
- Both runtime-agnostic

**Coupling Level**: 🟢 **NONE** - Already runtime-agnostic

**Abstraction Needed**:
**NONE**

**Implementation Approach**:
No changes needed.

**Changes Required**: **NONE**

---

### Formulas 📋

**What it does**:
- TOML-based workflow templates
- Define reusable patterns
- Protomolecules for instantiation

**Claude Code Integration**:
**NONE** - Formulas are declarative templates.

**Current Dependencies**:
None - pure TOML files.

**Coupling Level**: 🟢 **NONE** - Already runtime-agnostic

**Abstraction Needed**:
**NONE**

**Implementation Approach**:
No changes needed.

**Changes Required**: **NONE**

---

### Wisps 💨

**What it does**:
- Ephemeral Beads destroyed after runs
- Lightweight transient operations
- High-volume digestible events

**Claude Code Integration**:
**NONE** - Just temporary Beads.

**Current Dependencies**:
Beads system (runtime-agnostic).

**Coupling Level**: 🟢 **NONE** - Already runtime-agnostic

**Abstraction Needed**:
**NONE**

**Implementation Approach**:
No changes needed.

**Changes Required**: **NONE**

---

## Communication Concepts

### Mail 📧

**What it does**:
- Durable inter-agent messaging
- Stored in Beads ledger
- Delivers work assignments and notifications
- Supports mailing lists, queues, announces

**Claude Code Integration**:
Mail *delivery* happens via Claude hooks OR plugin injection.

**Current Dependencies**:
- Mail *storage*: Beads (runtime-agnostic ✅)
- Mail *injection*: Claude SessionStart hook OR Opencode plugin ⚠️

**Coupling Level**: 🟡 **MEDIUM** - Storage is agnostic, injection is runtime-specific

**Abstraction Needed**:

**Two parts**:
1. **Mail creation/storage** - Already runtime-agnostic ✅
2. **Mail injection into agent** - Runtime-specific, needs abstraction ⚠️

**Current approach**:
```go
// Claude: SessionStart hook runs "gt mail check --inject"
// Opencode: Plugin event runs "gt mail check --inject"
```

This already works! The abstraction is:
- Claude: hooks call `gt mail check --inject`
- Opencode: plugin calls `gt mail check --inject`
- Both use the same CLI command

**Implementation Approach**:
- ✅ **Already abstracted**: `gt mail check --inject` is runtime-agnostic
- ✅ **Already works**: Both Claude hooks and Opencode plugins can call it
- ℹ️ **Verify**: Ensure Opencode plugin reliably calls mail check

**Changes Required**: **NONE** (already works) or **VERIFY** (test Opencode plugin)

---

### Nudge ⚡

**What it does**:
- Real-time messaging between agents
- Immediate communication via tmux send-keys
- No persistent storage (unlike mail)

**Claude Code Integration**:
Uses tmux to send keys to Claude sessions.

**Current Dependencies**:
- Tmux for message delivery
- Session must be running in tmux

**Coupling Level**: 🟢 **LOW** - Tmux-based, works with any runtime in tmux

**Abstraction Needed**:
**MINIMAL** - Nudge works with any tmux-based runtime.

For non-tmux runtimes (future):
- Need alternative real-time messaging mechanism
- Could use process signals, files, or API calls

**Implementation Approach**:
- ✅ **Already works**: Nudge works with Claude, Codex, Opencode (all via tmux)
- ℹ️ **Future**: For non-tmux runtimes, need alternative mechanism

**Changes Required**: **NONE** (for tmux-based runtimes)

---

### Handoff 🤝

**What it does**:
- Agent session refresh mechanism
- Transfers work state to new session
- Used when context gets full
- Claude-specific feature (`/handoff` command)

**Claude Code Integration**:
```go
// Uses Claude's /handoff command
// Forks current session into new session
```

**Current Dependencies**:
- Claude's `--fork-session` flag
- Claude-specific handoff command
- Tmux for session management

**Coupling Level**: 🔴 **HIGH** - Relies on Claude-specific feature

**Abstraction Needed**:

**Problem**: Opencode may not have equivalent handoff feature.

**Options**:
1. **Check Opencode capabilities** - Does it support session forking?
2. **Manual handoff** - Save state to Bead, spawn new session, load state
3. **Feature parity gap** - Document as Claude-only feature

**Implementation Approach**:
- 🔍 **Research needed**: Does Opencode support handoff/session-fork? (EXP-007)
- ⚠️ **If no**: Implement manual handoff via state serialization
- ℹ️ **Document**: Feature parity differences

**Changes Required**: **MEDIUM** (if Opencode lacks feature) or **NONE** (if Opencode has it)

---

### Seance 🔮

**What it does**:
- Query previous agent sessions
- Access historical context and decisions
- Uses Claude's session history

**Claude Code Integration**:
```go
// Uses Claude's session resume + fork
// Queries specific past session
```

**Current Dependencies**:
- Claude session IDs
- Claude's resume capability

**Coupling Level**: 🔴 **HIGH** - Relies on Claude session history

**Abstraction Needed**:

Similar to Handoff - depends on runtime session history.

**Options**:
1. **Check Opencode** - Does it support session history/resume?
2. **Alternative**: Store session logs in Beads, query from there
3. **Feature gap**: Document as Claude-only or limited feature

**Implementation Approach**:
- 🔍 **Research needed**: Opencode session resume (EXP-007)
- ⚠️ **Alternative**: Session logging to Beads for history
- ℹ️ **Document**: Feature differences

**Changes Required**: **MEDIUM** (if alternative needed) or **NONE** (if Opencode supports)

---

## Infrastructure Concepts

### Tmux Integration 🖥️

**What it does**:
- Session management for agents
- Multiplexing multiple agents
- Send-keys for nudging
- Session monitoring

**Claude Code Integration**:
Tmux is runtime-agnostic - works with Claude, Codex, Opencode, etc.

**Current Dependencies**:
- Tmux binary
- Sessions run agents via command line

**Coupling Level**: 🟢 **NONE** - Tmux works with any CLI

**Abstraction Needed**:
**NONE** - Already runtime-agnostic.

**Implementation Approach**:
No changes needed.

**Changes Required**: **NONE**

---

### Git Worktrees 🌳

**What it does**:
- Isolated workspaces for Polecats
- Share git object storage
- Fast spawning, no full clones
- Cleanup on completion

**Claude Code Integration**:
**NONE** - Git worktrees are runtime-agnostic.

**Current Dependencies**:
- Git binary
- Rig must be a git repository

**Coupling Level**: 🟢 **NONE** - Already runtime-agnostic

**Abstraction Needed**:
**NONE**

**Implementation Approach**:
No changes needed.

**Changes Required**: **NONE**

---

### Runtime Config System ⚙️

**What it does**:
- Selects which AI runtime to use (Claude, Codex, Opencode, etc.)
- Provides command, args, hooks config per runtime
- Supports custom agent presets
- Already designed for multi-runtime support

**Claude Code Integration**:
**Designed to be runtime-agnostic** - this is the abstraction layer!

**Current Dependencies**:
None - this IS the abstraction.

**Coupling Level**: 🟢 **NONE** - Already runtime-agnostic by design

**Abstraction Needed**:
**NONE** - Already provides the abstraction.

**Problem**: Not all code uses it yet!

**Implementation Approach**:
- ✅ **Already exists**: `internal/runtime/runtime.go` and `internal/config/types.go`
- ⚠️ **Needs adoption**: Update agent managers to use runtime config instead of calling `claude.` directly

**Changes Required**: **ADOPTION** - Update existing code to use runtime config

---

## Summary: Coupling Analysis

### No Coupling (🟢 Already Runtime-Agnostic)

These concepts work with any runtime **without changes**:
- ✅ Beads
- ✅ Convoys
- ✅ Molecules
- ✅ Formulas
- ✅ Wisps
- ✅ Tmux integration
- ✅ Git worktrees
- ✅ Runtime config system
- ✅ Nudge (for tmux-based runtimes)
- ✅ Mail storage

**Action**: None needed

---

### Low Coupling (🟡 Minor Runtime-Specific Aspects)

These concepts are mostly agnostic but have minor runtime-specific behavior:
- ⚠️ Mail injection - Works via CLI command, both runtimes support
- ⚠️ Crew - Already flexible, could add setup helper

**Action**: Verify/test, minimal changes

---

### Medium Coupling (🟠 Runtime-Specific Spawning)

These concepts have runtime-agnostic logic but use Claude for spawning:
- ⚠️ Deacon
- ⚠️ Dogs
- ⚠️ Witness
- ⚠️ Refinery

**Action**: Update to use runtime config for spawning

---

### High Coupling (🔴 Claude-Specific Features)

These concepts directly depend on Claude-specific features:
- ❌ Mayor - Direct Claude package calls
- ❌ Polecat - Claude settings installation
- ❌ Handoff - Uses Claude's fork-session
- ❌ Seance - Uses Claude's session history

**Action**: 
- Mayor/Polecat: Update to use runtime config (straightforward)
- Handoff/Seance: Research Opencode capabilities, implement alternatives if needed

---

## Required Abstractions: Priority List

### Priority 1: Use Runtime Config (STRAIGHTFORWARD)

**Files to update**:
1. `internal/mayor/manager.go` - Use `runtime.EnsureSettingsForRole` instead of `claude.`
2. `internal/polecat/session_manager.go` - Use RuntimeConfig for spawning
3. `internal/witness/manager.go` - Use runtime config
4. `internal/refinery/manager.go` - Use runtime config
5. `internal/deacon/manager.go` - Use runtime config

**Pattern for all**:
```go
// BEFORE
if err := claude.EnsureSettingsForRole(workDir, role); err != nil {
    return err
}
cmd := "claude --dangerously-skip-permissions"

// AFTER
rc := config.LoadRuntimeConfig(workDir)
if err := runtime.EnsureSettingsForRole(workDir, role, rc); err != nil {
    return err
}
cmd := rc.BuildCommand()
```

**Estimated effort**: 2-3 hours (update 5 files, run tests)

---

### Priority 2: Verify Opencode Integration (TESTING)

**What to verify**:
1. Opencode plugin reliably calls `gt prime`
2. Opencode plugin reliably calls `gt mail check --inject`
3. Opencode sessions work with tmux nudging
4. Opencode handles work assignments via mail

**Experiments**:
- EXP-002: Plugin Installation
- EXP-003: Work Assignment via Mailbox
- EXP-008: Cross-Session Messaging

**Estimated effort**: 1-2 days of experimentation

---

### Priority 3: Handle Feature Gaps (CONDITIONAL)

**Research needed**:
1. Does Opencode support handoff/session-fork? (EXP-007)
2. Does Opencode support session resume? (EXP-007)
3. Does Opencode maintain session history?

**If gaps exist**:
- Implement alternative mechanisms (state serialization)
- Document feature parity differences
- Provide workarounds where possible

**Estimated effort**: 3-5 days (if alternatives needed)

---

### Priority 4: Documentation (CRITICAL)

**Clarify terminology**:
- "Hook" means two things (work queue Bead vs Claude event hooks)
- Rename or namespace to avoid confusion

**Document**:
- Which concepts are runtime-agnostic (most are!)
- Which require runtime-specific behavior
- Feature parity between Claude and Opencode
- Migration guide for switching runtimes

**Estimated effort**: 2-3 days

---

## Conclusion

**Key Findings**:

1. **Most concepts are already runtime-agnostic** 🎉
   - Beads, Convoys, Molecules, Formulas, Wisps
   - Tmux, Git worktrees
   - Mail storage, Nudge

2. **The abstraction already exists** ✅
   - `internal/runtime` and `internal/config` provide the framework
   - **Problem**: Not all code uses it yet

3. **Minimal changes needed** 🎯
   - Update 5 agent manager files to use runtime config
   - ~200-300 lines of code changes total
   - No new concepts or major refactoring

4. **Main unknowns are Opencode capabilities** 🔍
   - Handoff/session-fork support?
   - Session resume support?
   - Session history?
   - Need experimentation (EXP-007)

5. **Documentation is critical** 📚
   - Clarify "Hook" terminology
   - Document feature parity
   - Update architecture docs

**Bottom line**: The architecture is already well-designed for multi-runtime support. The main work is:
1. **Adoption** - Use the existing runtime config everywhere (Priority 1)
2. **Verification** - Test Opencode integration points (Priority 2)
3. **Gaps** - Handle any Opencode feature gaps (Priority 3)
4. **Docs** - Clarify and document (Priority 4)

**Estimated total effort**: 1-2 weeks for core implementation + testing

---

**Next Steps**:
1. Update Priority 1 files to use runtime config
2. Run experiments EXP-002, EXP-003, EXP-007, EXP-008
3. Document findings and feature parity
4. Update architecture docs with this analysis

---

**Last Updated**: 2026-01-16
**Owner**: Gastown Team
**Status**: Analysis Complete - See [Integration Architecture](opencode-integration-architecture.md) for Opencode-specific implementation details
