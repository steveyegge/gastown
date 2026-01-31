<!-- Polecat lifecycle test: 20260129-183509 -->
<!-- Last reviewed: 2026-01-23 -->
# Changelog

All notable changes to the Gas Town project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.5.0] - 2026-01-22

### Added

#### Mail Improvements
- **Numeric index support for `gt mail read`** - Read messages by inbox position (e.g., `gt mail read 1`)
- **`gt mail hook` alias** - Shortcut for `gt hook attach` from mail context
- **`--body` alias for `--message`** - More intuitive flag in `gt mail send` and `gt mail reply`
- **Multiple message IDs in delete** - `gt mail delete msg1 msg2 msg3`
- **Positional message arg in reply** - `gt mail reply <id> "message"` without --message flag
- **`--all` flag for inbox** - Show all messages including read
- **Parallel inbox queries** - ~6x speedup for mail inbox

#### Command Aliases
- **`gt bd`** - Alias for `gt bead`
- **`gt work`** - Alias for `gt hook`
- **`--comment` alias for `--reason`** - In `gt close`
- **`read` alias for `show`** - In `gt bead`

#### Configuration & Agents
- **OpenCode as built-in agent preset** - Configure with `gt config set agent opencode`
- **Config-based role definition system** - Roles defined in config, not beads
- **Env field in RuntimeConfig** - Custom environment variables for agent presets
- **ShellQuote helper** - Safe env var escaping for shell commands

#### Infrastructure
- **Deacon status line display** - Shows deacon icon in mayor status line
- **Configurable polecat branch naming** - Template-based branch naming
- **Hook registry and install command** - Manage Claude Code hooks via `gt hooks`
- **Doctor auto-fix capability** - SessionHookCheck can auto-repair
- **`gt orphans kill` command** - Clean up orphaned Claude processes
- **Zombie-scan command for deacon** - tmux-verified process cleanup
- **Initial prompt for autonomous patrol startup** - Better agent priming

#### Refinery & Merging
- **Squash merge for cleaner history** - Eliminates redundant merge commits
- **Redundant observers** - Witness and Refinery both watch convoys

### Fixed

#### Crew & Session Stability
- **Don't kill pane processes on new sessions** - Prevents destroying fresh shells
- **Auto-recover from stale tmux pane references** - Recreates sessions automatically
- **Preserve GT_AGENT across session restarts** - Handoff maintains identity

#### Process Management
- **KillPaneProcesses kills pane process itself** - Not just descendants
- **Kill pane processes before all RespawnPane calls** - Prevents orphan leaks
- **Shutdown reliability improvements** - Multiple fixes for clean shutdown
- **Deacon spawns immediately after killing stuck session**

#### Convoy & Routing
- **Pass convoy ID to convoy check command** - Correct ID propagation
- **Multi-repo routing for custom types** - Correct beads routing across repos
- **Normalize agent ID trailing slash** - Consistent ID handling

#### Miscellaneous
- **Sling auto-apply mol-polecat-work** - Auto-attach on open polecat beads
- **Wisp orphan lifecycle bug** - Proper cleanup of abandoned wisps
- **Misclassified wisp detection** - Defense-in-depth filtering
- **Cross-account session access in seance** - Talk to predecessors across accounts
- **Many more bug fixes** - See git log for full details

## [0.4.0] - 2026-01-19

_Changelog not documented at release time. See git log v0.3.1..v0.4.0 for changes._

## [0.3.1] - 2026-01-18

_Changelog not documented at release time. See git log v0.3.0..v0.3.1 for changes._

## [0.3.0] - 2026-01-17

### Added

#### Release Automation
- **`gastown-release` molecule formula** - Workflow for releases with preflight checks, CHANGELOG/info.go updates, local install, and daemon restart

#### New Commands
- **`gt show`** - Inspect bead contents and metadata
- **`gt cat`** - Display bead content directly
- **`gt orphans list/kill`** - Detect and clean up orphaned Claude processes
- **`gt convoy close`** - Manual convoy closure command
- **`gt commit`** - Wrapper for git commit with bead awareness
- **`gt trail`** - View commit trail for current work
- **`gt mail ack`** - Alias for mark-read command

#### Plugin System
- **Plugin discovery and management** - `gt plugin run`, `gt plugin history`
- **`gt dispatch --plugin`** - Execute plugins via dispatch command

#### Messaging Infrastructure (Beads-Native)
- **Queue beads** - New bead type for message queues
- **Channel beads** - Pub/sub messaging with retention
- **Group beads** - Group management for messaging
- **Address resolution** - Resolve agent addresses for mail routing
- **`gt mail claim`** - Claim messages from queues

#### Agent Identity
- **`gt polecat identity show`** - Display CV summary for agents
- **Worktree setup hooks** - Inject local configurations into worktrees

#### Performance & Reliability
- **Parallel agent startup** - Faster boot with concurrency limit
- **Event-driven convoy completion** - Deacon checks convoy status on events
- **Automatic orphan cleanup** - Detect and kill orphaned Claude processes
- **Namepool auto-theming** - Themes selected per rig based on name hash

### Changed

- **MR tracking via beads** - Removed mrqueue package, MRs now stored as beads
- **Desire-path commands** - Added agent ergonomics shortcuts
- **Explicit escalation in templates** - Polecat templates include escalation instructions
- **NamePool state is transient** - InUse state no longer persisted to config

### Fixed

#### Process Management
- **Kill process tree on shutdown** - Prevents orphaned Claude processes
- **Explicit pane process kill** - Prevents setsid orphans in tmux
- **Session survival verification** - Verify session survives startup before returning
- **Batch session queries** - Improved performance in `gt down`
- **Prevent tmux server exit** - `gt down` no longer kills tmux server

#### Beads & Routing
- **Agent bead prefix alignment** - Force multi-hyphen IDs for consistency
- **hq- prefix for town-level beads** - Groups, channels use correct prefix
- **CreatedAt for group/channel beads** - Proper timestamps on creation
- **Routes.jsonl protection** - Doctor check for rig-level routing issues
- **Clear BEADS_DIR in auto-convoys** - Prevent prefix inheritance issues

#### Mail & Communication
- **Channel routing in router.Send()** - Mail correctly routes to channels
- **Filter unread in beads mode** - Correct unread message filtering
- **Town root detection** - Use workspace.Find for consistent detection

#### Session & Lifecycle
- **Idle Polecat Heresy warnings** - Templates warn against idle waiting
- **Direct push prohibition for polecats** - Explicit in templates
- **Handoff working directory** - Use correct witness directory
- **Dead polecat handling in sling** - Detect and handle dead polecats
- **gt done self-cleaning** - Kill tmux session on completion

#### Doctor & Diagnostics
- **Zombie session detection** - Detect dead Claude processes in tmux
- **sqlite3 availability check** - Verify sqlite3 is installed
- **Clone divergence check** - Remove blocking git fetch

#### Build & Platform
- **Windows build support** - Platform-specific process/signal handling
- **macOS codesigning** - Sign binary after install

### Documentation

- **Idle Polecat Heresy** - Document the anti-pattern of waiting for work
- **Bead ID vs Issue ID** - Clarify terminology in README
- **Explicit escalation** - Add escalation guidance to polecat templates
- **Getting Started placement** - Fix README section ordering

## [0.2.6] - 2026-01-12

### Added

#### Escalation System
- **Unified escalation system** - Complete escalation implementation with severity levels, routing, and tracking (gt-i9r20)
- **Escalation config schema alignment** - Configuration now matches design doc specifications

#### Agent Identity & Management
- **`gt polecat identity` subcommand group** - Agent bead management commands for polecat lifecycle
- **AGENTS.md fallback copy** - Polecats automatically copy AGENTS.md from mayor/rig for context bootstrapping
- **`--debug` flag for `gt crew at`** - Debug mode for crew attachment troubleshooting
- **Boot role detection in priming** - Proper context injection for boot role agents (#370)

#### Statusline Improvements
- **Per-agent-type health tracking** - Statusline now shows health status per agent type (#344)
- **Visual rig grouping** - Rigs sorted by activity with visual grouping in tmux statusline (#337)

#### Mail & Communication
- **`gt mail show` alias** - Alternative command for reading mail (#340)

#### Developer Experience
- **`gt stale` command** - Check for stale binaries and version mismatches

### Changed

- **Refactored statusline** - Merged session loops and removed dead code for cleaner implementation
- **Refactored sling.go** - Split 1560-line file into 7 focused modules for maintainability
- **Magic numbers extracted** - Suggest package now uses named constants (#353)

### Fixed

#### Configuration & Environment
- **Empty GT_ROOT/BEADS_DIR not exported** - AgentEnv no longer exports empty environment variables (#385)
- **Inherited BEADS_DIR prefix mismatch** - Prevent inherited BEADS_DIR from causing prefix mismatches (#321)

#### Beads & Routing
- **routes.jsonl corruption prevention** - Added protection against routes.jsonl corruption with doctor check for rig-level issues (#377)
- **Tracked beads init after clone** - Initialize beads database for tracked beads after git clone (#376)
- **Rig root from BeadsPath()** - Correctly return rig root to respect redirect system

#### Sling & Formula
- **Feature and issue vars in formula-on-bead mode** - Pass both variables correctly (#382)
- **Crew member shorthand resolution** - Resolve crew members correctly with shorthand paths
- **Removed obsolete --naked flag** - Cleanup of deprecated sling option

#### Doctor & Diagnostics
- **Role beads check with shared definitions** - Doctor now validates role beads using shared role definitions (#378)
- **Filter bd "Note:" messages** - Custom types check no longer confused by bd informational output (#381)

#### Installation & Setup
- **gt:role label on role beads** - Role beads now properly labeled during creation (#383)
- **Fetch origin after refspec config** - Bare clones now fetch after configuring refspec (#384)
- **Allow --wrappers in existing town** - No longer recreates HQ unnecessarily (#366)

#### Session & Lifecycle
- **Fallback instructions in start/restart beacons** - Session beacons now include fallback instructions
- **Handoff recognizes polecat session pattern** - Correctly handles gt-<rig>-<name> session names (#373)
- **gt done resilient to missing agent beads** - No longer fails when agent beads don't exist
- **MR beads as ephemeral wisps** - Create MR beads as ephemeral wisps for proper cleanup
- **Auto-detect cleanup status** - Prevents premature polecat nuke (#361)
- **Delete remote polecat branches after merge** - Refinery cleans up remote branches (#369)

#### Costs & Events
- **Query all beads locations for session events** - Cost tracking finds events across locations (#374)

#### Linting & Quality
- **errcheck and unparam violations resolved** - Fixed linting errors
- **NudgeSession for all agent notifications** - Mail now uses consistent notification method

### Documentation

- **Polecat three-state model** - Clarified working/stalled/zombie states
- **Name pool vs polecat pool** - Clarified misconception about pools
- **Plugin and escalation system designs** - Added design documentation
- **Documentation reorganization** - Concepts, design, and examples structure
- **gt prime clarification** - Clarified that gt prime is context recovery, not session start (GH #308)
- **Formula package documentation** - Comprehensive package docs
- **Various godoc additions** - GenerateMRIDWithTime, isAutonomousRole, formatInt, nil sentinel pattern
- **Beads issue ID format** - Clarified format in README (gt-uzx2c)
- **Stale polecat identity description** - Fixed outdated documentation

### Tests

- **AGENTS.md worktree tests** - Test coverage for AGENTS.md in worktrees
- **Comprehensive test coverage** - Added tests for 5 packages (#351)
- **Sling test for bd empty output** - Fixed test for empty output handling

### Deprecated

- **`gt polecat add`** - Added migration warning for deprecated command

### Contributors

Thanks to all contributors for this release:
- @JeremyKalmus - Various contributions (#364)
- @boshu2 - Formula package documentation (#343), PR documentation (#352)
- @sauerdaniel - Polecat mail notification fix (#347)
- @abhijit360 - Assign model to role (#368)
- @julianknutsen - Beads path fix (#334)

## [0.2.5] - 2026-01-11

### Added
- **`gt mail mark-read`** - Mark messages as read without opening them (desire path)
- **`gt down --polecats`** - Shut down polecats without affecting other components
- **Self-cleaning polecat model** - Polecats self-nuke on completion, witness tracks leases
- **`gt prime --state` validation** - Flag exclusivity checks for cleaner CLI

### Changed
- **Removed `gt stop`** - Use `gt down --polecats` instead (cleaner semantics)
- **Policy-neutral templates** - crew.md.tmpl checks remote origin for PR policy
- **Refactored prime.go** - Split 1833-line file into logical modules

### Fixed
- **Polecat re-spawn** - CreateOrReopenAgentBead handles polecat lifecycle correctly (#333)
- **Vim mode compatibility** - tmux sends Escape before Enter for vim users
- **Worktree default branch** - Uses rig's configured default branch (#325)
- **Agent bead type** - Sets --type=agent when creating agent beads
- **Bootstrap priming** - Reduced AGENTS.md to bootstrap pointer, fixed CLAUDE.md templates

### Documentation
- Updated witness help text for self-cleaning model
- Updated daemon comments for self-cleaning model
- Policy-aware PR guidance in crew template

## [0.2.4] - 2026-01-10

Priming subsystem overhaul and Zero Framework Cognition (ZFC) improvements.

### Added

#### Priming Subsystem
- **PRIME.md provisioning** - Auto-provision PRIME.md at rig level so all workers inherit Gas Town context (GUPP, hooks, propulsion) (#hq-5z76w)
- **Post-handoff detection** - `gt prime` detects handoff marker and outputs "HANDOFF COMPLETE" warning to prevent handoff loop bug (#hq-ukjrr)
- **Priming health checks** - `gt doctor` validates priming subsystem: SessionStart hook, gt prime command, PRIME.md presence, CLAUDE.md size (#hq-5scnt)
- **`gt prime --dry-run`** - Preview priming without side effects
- **`gt prime --state`** - Output session state (normal, post-handoff, crash-recovery, autonomous)
- **`gt prime --explain`** - Add [EXPLAIN] tags for debugging priming decisions

#### Formula & Configuration
- **Rig-level default formulas** - Configure default formula at rig level (#297)
- **Witness --agent/--env overrides** - Override agent and environment variables for witness (#293, #294)

#### Developer Experience
- **UX system import** - Comprehensive UX system from beads (#311)
- **Explicit handoff instructions** - Clearer nudge message for handoff recipients

### Fixed

#### Zero Framework Cognition (ZFC)
- **Query tmux directly** - Remove marker TTL, query tmux for agent state
- **Remove PID-based detection** - Agent liveness from tmux, not PIDs
- **Agent-controlled thresholds** - Stuck detection moved to agent config
- **Remove pending.json tracking** - Eliminated anti-pattern
- **Derive state from files** - ZFC state from filesystem, not memory cache
- **Remove Go-side computation** - No stderr parsing violations

#### Hooks & Beads
- **Cross-level hook visibility** - Hooked beads visible to mayor/deacon (#aeb4c0d)
- **Warn on closed hooked bead** - Alert when hooked bead already closed (#2f50a59)
- **Correct agent bead ID format** - Fix bd create flags for agent beads (#c4fcdd8)

#### Formula
- **rigPath fallback** - Set rigPath when falling back to gastown default (#afb944f)

#### Doctor
- **Full AgentEnv for env-vars check** - Use complete environment for validation (#ce231a3)

### Changed

- **Refactored beads/mail modules** - Split large files into focused modules for maintainability

## [0.2.3] - 2026-01-09

Worker safety release - prevents accidental termination of active agents.

> **Note**: The Deacon safety improvements are believed to be correct but have not
> yet been extensively tested in production. We recommend running with
> `gt deacon pause` initially and monitoring behavior before enabling full patrol.
> Please report any issues. A 0.3.0 release will follow once these changes are
> battle-tested.

### Critical Safety Improvements

- **Kill authority removed from Deacon** - Deacon patrol now only detects zombies via `--dry-run`, never kills directly. Death warrants are filed for Boot to handle interrogation/execution. This prevents destruction of worker context, mid-task progress, and unsaved state (#gt-vhaej)
- **Bulletproof pause mechanism** - Multi-layer pause for Deacon with file-based state, `gt deacon pause/resume` commands, and guards in `gt prime` and heartbeat (#265)
- **Doctor warns instead of killing** - `gt doctor` now warns about stale town-root settings rather than killing sessions (#243)
- **Orphan process check informational** - Doctor's orphan process detection is now informational only, not actionable (#272)

### Added

- **`gt account switch` command** - Switch between Claude Code accounts with `gt account switch <handle>`. Manages `~/.claude` symlinks and updates default account
- **`gt crew list --all`** - Show all crew members across all rigs (#276)
- **Rig-level custom agent support** - Configure different agents per-rig (#12)
- **Rig identity beads check** - Doctor validates rig identity beads exist
- **GT_ROOT env var** - Set for all agent sessions for consistent environment
- **New agent presets** - Added Cursor, Auggie (Augment Code), and Sourcegraph AMP as built-in agent presets (#247)
- **Context Management docs** - Added to Witness template for better context handling (gt-jjama)

### Fixed

- **`gt prime --hook` recognized** - Doctor now recognizes `gt prime --hook` as valid session hook config (#14)
- **Integration test reliability** - Improved test stability (#13)
- **IsClaudeRunning detection** - Now detects 'claude' and version patterns correctly (#273)
- **Deacon heartbeat restored** - `ensureDeaconRunning` restored to heartbeat using Manager pattern (#271)
- **Deacon session names** - Correct session name references in formulas (#270)
- **Hidden directory scanning** - Ignore `.claude` and other dot directories when enumerating polecats (#258, #279)
- **SetupRedirect tracked beads** - Works correctly with tracked beads architecture where canonical location is `mayor/rig/.beads`
- **Tmux shell ready** - Wait for shell ready before sending keys (#264)
- **Gastown prefix derivation** - Correctly derive `gt-` prefix for gastown compound words (gt-m46bb)
- **Custom beads types** - Register custom beads types during install (#250)

### Changed

- **Refinery Manager pattern** - Replaced `ensureRefinerySession` with `refinery.Manager.Start()` for consistency

### Removed

- **Unused formula JSON** - Removed unused JSON formula file (cleanup)

### Contributors

Thanks to all contributors for this release:
- @julianknutsen - Doctor fixes (#14, #271, #272, #273), formula fixes (#270), GT_ROOT env (#268)
- @joshuavial - Hidden directory scanning (#258, #279), crew list --all (#276)

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
