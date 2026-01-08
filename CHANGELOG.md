# Changelog

All notable changes to the Gas Town project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.2] - 2026-01-07

Rig operational state management, unified agent startup, and extensive stability fixes.

### Added

#### Rig Operational State Management
- **`gt rig park/unpark` commands** - Level 1 rig control: pause daemon auto-start while preserving sessions
- **`gt rig dock/undock` commands** - Level 2 rig control: stop all sessions and prevent auto-start (gt-9gm9n)
- **`gt rig config` commands** - Per-rig configuration management (gt-hhmkq)
- **Rig identity beads** - Schema and creation for rig identity tracking (gt-zmznh)
- **Property layer lookup** - Hierarchical configuration resolution (gt-emh1c)
- **Operational state in status** - `gt rig status` shows park/dock state

#### Agent Configuration & Startup
- **`--agent` overrides** - Override agent for start/attach/sling commands
- **Unified agent startup** - Manager pattern for consistent agent initialization
- **Claude settings installation** - Auto-install during rig and HQ creation
- **Runtime-aware tmux checks** - Detect actual agent state from tmux sessions

#### Status & Monitoring
- **`gt status --watch`** - Watch mode with auto-refresh (#231)
- **Compact status output** - One-line-per-worker format as new default
- **LED status indicators** - Visual indicators for rigs in Mayor tmux status line
- **Parked/docked indicators** - Pause emoji (⏸) for inactive rigs in statusline

#### Beads & Workflow
- **Minimum beads version check** - Validates beads CLI compatibility (gt-im3fl)
- **ZFC convoy auto-close** - `bd close` triggers convoy completion (gt-3qw5s)
- **Stale hooked bead cleanup** - Deacon clears orphaned hooks (gt-2yls3)
- **Doctor prefix mismatch detection** - Detect misconfigured rig prefixes (gt-17wdl)
- **Unified beads redirect** - Single redirect system for tracked and local beads (#222)
- **Route from rig to town beads** - Cross-level bead routing

#### Infrastructure
- **Windows-compatible file locking** - Daemon lock works on Windows
- **`--purge` flag for crews** - Full crew obliteration option
- **Debug logging for suppressed errors** - Better visibility into startup issues (gt-6d7eh)
- **hq- prefix in tmux cycle bindings** - Navigate to Mayor/Deacon sessions
- **Wisp config storage layer** - Transient/local settings for ephemeral workflows
- **Sparse checkout** - Exclude Claude context files from source repos

### Changed

- **Daemon respects rig operational state** - Parked/docked rigs not auto-started
- **Agent startup unified** - Manager pattern replaces ad-hoc initialization
- **Mayor files moved** - Reorganized into `mayor/` subdirectory
- **Refinery merges local branches** - No longer fetches from origin (gt-cio03)
- **Polecats start from origin/default-branch** - Consistent recycled state
- **Observable states removed** - Discover agent state from tmux, don't track (gt-zecmc)
- **mol-town-shutdown v3** - Complete cleanup formula (gt-ux23f)
- **Witness delays polecat cleanup** - Wait until MR merges (gt-12hwb)
- **Nudge on divergence** - Daemon nudges agents instead of silent accept
- **README rewritten** - Comprehensive guides and architecture docs (#226)
- **`gt rigs` → `gt rig list`** - Command renamed in templates/docs (#217)

### Fixed

#### Doctor & Lifecycle
- **`--restart-sessions` flag required** - Doctor won't cycle sessions without explicit flag (gt-j44ri)
- **Only cycle patrol roles** - Doctor --fix doesn't restart crew/polecats (hq-qthgye)
- **Session-ended events auto-closed** - Prevent accumulation (gt-8tc1v)
- **GUPP propulsion nudge** - Added to daemon restartSession

#### Sling & Beads
- **Sling uses bd native routing** - No BEADS_DIR override needed
- **Sling parses wisp JSON correctly** - Handle `new_epic_id` field
- **Sling resolves rig path** - Cross-rig bead hooking works
- **Sling waits for Claude ready** - Don't nudge until session responsive (#146)
- **Correct beads database for sling** - Rig-level beads used (gt-n5gga)
- **Close hooked beads before clearing** - Proper cleanup order (gt-vwjz6)
- **Removed dead sling flags** - `--molecule` and `--quality` cleaned up

#### Agent Sessions
- **Witness kills tmux on Stop()** - Clean session termination
- **Deacon uses session package** - Correct hq- session names (gt-r38pj)
- **Honor rig agent for witness/refinery** - Respect per-rig settings
- **Canonical hq role bead IDs** - Consistent naming
- **hq- prefix in status display** - Global agents shown correctly (gt-vcvyd)
- **Restart Claude when dead** - Recover sessions where tmux exists but Claude died
- **Town session cycling** - Works from any directory

#### Polecat & Crew
- **Nuke not blocked by stale hooks** - Closed beads don't prevent cleanup (gt-jc7bq)
- **Crew stop dry-run support** - Preview cleanup before executing (gt-kjcx4)
- **Crew defaults to --all** - `gt crew start <rig>` starts all crew (gt-s8mpt)
- **Polecat cleanup handlers** - `gt witness process` invokes handlers (gt-h3gzj)

#### Daemon & Configuration
- **Create mayor/daemon.json** - `gt start` and `gt doctor --fix` initialize daemon state (#225)
- **Initialize git before beads** - Enable repo fingerprint (#180)
- **Handoff preserves env vars** - Claude Code environment not lost (#216)
- **Agent settings passed correctly** - Witness and daemon respawn use rigPath
- **Log rig discovery errors** - Don't silently swallow (gt-rsnj9)

#### Refinery & Merge Queue
- **Use rig's default_branch** - Not hardcoded 'main'
- **MERGE_FAILED sent to Witness** - Proper failure notification
- **Removed BranchPushedToRemote checks** - Local-only workflow support (gt-dymy5)

#### Misc Fixes
- **BeadsSetupRedirect preserves tracked files** - Don't clobber existing files (gt-fj0ol)
- **PATH export in hooks** - Ensure commands find binaries
- **Replace panic with fallback** - ID generation gracefully degrades (#213)
- **Removed duplicate WorktreeAddFromRef** - Code cleanup
- **Town root beads for Deacon** - Use correct beads location (gt-sstg)

### Refactored

- **AgentStateManager pattern** - Shared state management extracted (gt-gaw8e)
- **CleanupStatus type** - Replace raw strings (gt-77gq7)
- **ExecWithOutput utility** - Common command execution (gt-vurfr)
- **runBdCommand helper** - DRY mail package (gt-8i6bg)
- **Config expansion helper** - Generic DRY config (gt-i85sg)

### Documentation

- **Property layers guide** - Implementation documentation
- **Worktree architecture** - Clarified beads routing
- **Agent config** - Onboarding docs mention --agent overrides
- **Polecat Operations section** - Added to Mayor docs (#140)

### Contributors

Thanks to all contributors for this release:
- @julianknutsen - Claude settings inheritance (#239)
- @joshuavial - Sling wisp JSON parse (#238)
- @michaellady - Unified beads redirect (#222), daemon.json fix (#225)
- @greghughespdx - PATH in hooks fix (#139)

## [0.2.1] - 2026-01-05

Bug fixes, security hardening, and new `gt config` command.

### Added

- **`gt config` command** - Manage agent settings (model, provider) per-rig or globally
- **`hq-` prefix for patrol sessions** - Mayor and Deacon sessions use town-prefixed names
- **Doctor hooks-path check** - Verify Git hooks path is configured correctly
- **Block internal PRs** - Pre-push hook and GitHub Action prevent accidental internal PRs (#117)
- **Dispatcher notifications** - Notify dispatcher when polecat work completes
- **Unit tests** - Added tests for `formatTrackBeadID` helper, done redirect, hook slot E2E

### Fixed

#### Security
- **Command injection prevention** - Validate beads prefix to prevent injection (gt-l1xsa)
- **Path traversal prevention** - Validate crew names to prevent traversal (gt-wzxwm)
- **ReDoS prevention** - Escape user input in mail search (gt-qysj9)
- **Error handling** - Handle crypto/rand.Read errors in ID generation

#### Convoy & Sling
- **Hook slot initialization** - Set hook slot when creating agent beads during sling (#124)
- **Cross-rig bead formatting** - Format cross-rig beads as external refs in convoy tracking (#123)
- **Reliable bd calls** - Add `--no-daemon` and `BEADS_DIR` for reliable beads operations

#### Rig Inference
- **`gt rig status`** - Infer rig name from current working directory
- **`gt crew start --all`** - Infer rig from cwd for batch crew starts
- **`gt prime` in crew start** - Pass as initial prompt in crew start commands
- **Town default_agent** - Honor default agent setting for Mayor and Deacon

#### Session & Lifecycle
- **Hook persistence** - Hook persists across session interruption via `in_progress` lookup (gt-ttn3h)
- **Polecat cleanup** - Clean up stale worktrees and git tracking
- **`gt done` redirect** - Use ResolveBeadsDir for redirect file support

#### Build & CI
- **Embedded formulas** - Sync and commit formulas for `go install @latest`
- **CI lint fixes** - Resolve lint and build errors
- **Flaky test fix** - Sync database before beads integration tests

## [0.2.0] - 2026-01-04

Major release featuring the Convoy Dashboard, two-level beads architecture, and significant multi-agent improvements.

### Added

#### Convoy Dashboard (Web UI)
- **`gt dashboard` command** - Launch web-based monitoring UI for Gas Town (#71)
- **Polecat Workers section** - Real-time activity monitoring with tmux session timestamps
- **Refinery Merge Queue display** - Always-visible MR queue status
- **Dynamic work status** - Convoy status columns with live updates
- **HTMX auto-refresh** - 10-second refresh interval for real-time monitoring

#### Two-Level Beads Architecture
- **Town-level beads** (`~/gt/.beads/`) - `hq-*` prefix for Mayor mail and cross-rig coordination
- **Rig-level beads** - Project-specific issues with rig prefixes (e.g., `gt-*`)
- **`gt migrate-agents` command** - Migration tool for two-level architecture (#nnub1)
- **TownBeadsPrefix constant** - Centralized `hq-` prefix handling
- **Prefix-based routing** - Commands auto-route to correct rig via `routes.jsonl`

#### Multi-Agent Support
- **Pluggable agent registry** - Multi-agent support with configurable providers (#107)
- **Multi-rig management** - `gt rig start/stop/restart/status` for batch operations (#11z8l)
- **`gt crew stop` command** - Stop crew sessions cleanly
- **`spawn` alias** - Alternative to `start` for all role subcommands
- **Batch slinging** - `gt sling` supports multiple beads to a rig in one command (#l9toz)

#### Ephemeral Polecat Model
- **Immediate recycling** - Polecats recycled after each work unit (#81)
- **Updated patrol formula** - Witness formula adapted for ephemeral model
- **`mol-polecat-work` formula** - Updated for ephemeral polecat lifecycle (#si8rq.4)

#### Cost Tracking
- **`gt costs` command** - Session cost tracking and reporting
- **Beads-based storage** - Costs stored in beads instead of JSONL (#f7jxr)
- **Stop hook integration** - Auto-record costs on session end
- **Tmux session auto-detection** - Costs hook finds correct session

#### Conflict Resolution
- **Conflict resolution workflow** - Formula-based conflict handling for polecats (#si8rq.5)
- **Merge-slot gate** - Refinery integration for ordered conflict resolution
- **`gt done --phase-complete`** - Gate-based phase handoffs (#si8rq.7)

#### Communication & Coordination
- **`gt mail archive` multi-ID** - Archive multiple messages at once (#82)
- **`gt mail --all` flag** - Clear all mail for agent ergonomics (#105q3)
- **Convoy stranded detection** - Detect and feed stranded convoys (#8otmd)
- **`gt convoy --tree`** - Show convoy + child status tree
- **`gt convoy check`** - Cross-rig auto-close for completed convoys (#00qjk)

#### Developer Experience
- **Shell completion** - Installation instructions for bash/zsh/fish (#pdrh0)
- **`gt prime --hook`** - LLM runtime session handling flag
- **`gt doctor` enhancements** - Session-hooks check, repo-fingerprint validation (#nrgm5)
- **Binary age detection** - `gt status` shows stale binary warnings (#42whv)
- **Circuit breaker** - Automatic handling for stuck agents (#72cqu)

#### Infrastructure
- **SessionStart hooks** - Deployed during `gt install` for Mayor role
- **`hq-dog-role` beads** - Town-level dog role initialization (#2jjry)
- **Watchdog chain docs** - Boot/Deacon lifecycle documentation (#1847v)
- **Integration tests** - CI workflow for `gt install` and `gt rig add` (#htlmp)
- **Local repo reference clones** - Save disk space with `--reference` cloning

### Changed

- **Handoff migrated to skills** - `gt handoff` now uses skills format (#nqtqp)
- **Crew workers push to main** - Documentation clarifies no PR workflow for crew
- **Session names include town** - Mayor/Deacon sessions use town-prefixed names
- **Formula semantics clarified** - Formulas are templates, not instructions
- **Witness reports stopped** - No more routine Mayor reports (saves tokens)

### Fixed

#### Daemon & Session Stability
- **Thread-safety** - Added locks for agent session resume support
- **Orphan daemon prevention** - File locking prevents duplicate daemons (#108)
- **Zombie tmux cleanup** - Kill zombie sessions before recreating (#vve6k)
- **Tmux exact matching** - `HasSession` uses exact match to prevent prefix collisions
- **Health check fallback** - Prevents killing healthy sessions on tmux errors

#### Beads Integration
- **Mayor/rig path** - Use correct path for beads to prevent prefix mismatch (#38)
- **Agent bead creation** - Fixed during `gt rig add` (#32)
- **bd daemon startup** - Circuit breaker and restart logic (#2f0p3)
- **BEADS_DIR environment** - Correctly set for polecat hooks and cross-rig work

#### Agent Workflows
- **Default branch detection** - `gt done` no longer hardcodes 'main' (#42)
- **Enter key retry** - Reliable Enter key delivery with retry logic (#53)
- **SendKeys debounce** - Increased to 500ms for reliability
- **MR bead closure** - Close beads after successful merge from queue (#52)

#### Installation & Setup
- **Embedded formulas** - Copy formulas to new installations (#86)
- **Vestigial cleanup** - Remove `rigs/` directory and `state.json` files
- **Symlink preservation** - Workspace detection preserves symlink paths (#3, #75)
- **Golangci-lint errors** - Resolved errcheck and gosec issues (#76)

### Contributors

Thanks to all contributors for this release:
- @kiwiupover - README updates (#109)
- @michaellady - Convoy dashboard (#71), ResolveBeadsDir fix (#54)
- @jsamuel1 - Dependency updates (#83)
- @dannomayernotabot - Witness fixes (#87), daemon race condition (#64)
- @markov-kernel - Mayor session hooks (#93), daemon init recommendation (#95)
- @rawwerks - Multi-agent support (#107)
- @jakehemmerle - Daemon orphan race condition (#108)
- @danshapiro - Install role slots (#106), rig beads dir (#61)
- @vessenes - Town session helpers (#91), install copy formulas (#86)
- @kustrun - Init bugs (#34)
- @austeane - README quickstart fix (#44)
- @Avyukth - Patrol roles per-rig check (#26)

## [0.1.1] - 2026-01-02

### Fixed

- **Tmux keybindings scoped to Gas Town sessions** - C-b n/p no longer override default tmux behavior in non-GT sessions (#13)

### Added

- **OSS project files** - CHANGELOG.md, .golangci.yml, RELEASING.md
- **Version bump script** - `scripts/bump-version.sh` for releases
- **Documentation fixes** - Corrected `gt rig add` and `gt crew add` CLI syntax (#6)
- **Rig prefix routing** - Agent beads now use correct rig-specific prefixes (#11)
- **Beads init fix** - Rig beads initialization targets correct database (#9)

## [0.1.0] - 2026-01-02

### Added

Initial public release of Gas Town - a multi-agent workspace manager for Claude Code.

#### Core Architecture
- **Town structure** - Hierarchical workspace with rigs, crews, and polecats
- **Rig management** - `gt rig add/list/remove` for project containers
- **Crew workspaces** - `gt crew add` for persistent developer workspaces
- **Polecat workers** - Transient agent workers managed by Witness

#### Agent Roles
- **Mayor** - Global coordinator for cross-rig work
- **Deacon** - Town-level lifecycle patrol and heartbeat
- **Witness** - Per-rig polecat lifecycle manager
- **Refinery** - Merge queue processor with code review
- **Crew** - Persistent developer workspaces
- **Polecat** - Transient worker agents

#### Work Management
- **Convoy system** - `gt convoy create/list/status` for tracking related work
- **Sling workflow** - `gt sling <bead> <rig>` to assign work to agents
- **Hook mechanism** - Work attached to agent hooks for pickup
- **Molecule workflows** - Formula-based multi-step task execution

#### Communication
- **Mail system** - `gt mail inbox/send/read` for agent messaging
- **Escalation protocol** - `gt escalate` with severity levels
- **Handoff mechanism** - `gt handoff` for context-preserving session cycling

#### Integration
- **Beads integration** - Issue tracking via beads (`bd` commands)
- **Tmux sessions** - Agent sessions in tmux with theming
- **GitHub CLI** - PR creation and merge queue via `gh`

#### Developer Experience
- **Status dashboard** - `gt status` for town overview
- **Session cycling** - `C-b n/p` to navigate between agents
- **Activity feed** - `gt feed` for real-time event stream
- **Nudge system** - `gt nudge` for reliable message delivery to sessions

### Infrastructure
- **Daemon mode** - Background lifecycle management
- **npm package** - Cross-platform binary distribution
- **GitHub Actions** - CI/CD workflows for releases
- **GoReleaser** - Multi-platform binary builds
