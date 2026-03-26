// Package config — AgentCopilot Preset Specification
//
// This file specifies the Gas Town agent preset for GitHub Copilot CLI.
// It is intended to be reviewed and adapted for the gastown fork.
//
// ═══════════════════════════════════════════════════════════════════
// GHCP CLI AGENT PRESET — SPECIFICATION
// ═══════════════════════════════════════════════════════════════════
//
// ┌─────────────────────────────────────────────────────────────────┐
// │ STATUS: SPEC — not yet integrated into agents.go               │
// │ TARGET: gastown/internal/config/agents.go                      │
// │ AUTHOR: Generated from research, March 2026                    │
// │ SOURCES:                                                       │
// │   - https://docs.github.com/en/copilot/concepts/agents/        │
// │     copilot-cli/about-copilot-cli                              │
// │   - https://github.com/github/copilot-cli (v1.0.11)           │
// │   - https://github.com/jongio/dispatch (v0.6.0)               │
// │   - https://blog.jongallant.com/2026/03/                       │
// │     dispatch-github-copilot-cli-session-explorer               │
// └─────────────────────────────────────────────────────────────────┘
//
// ═══════════════════════════════════════════════════════════════════
// 1. CONFIRMED COPILOT CLI CAPABILITIES (verified from docs + OSS)
// ═══════════════════════════════════════════════════════════════════
//
// Binary:           copilot (standalone, not gh extension)
// Install:          winget install GitHub.Copilot
//
//	brew install copilot-cli
//	npm i -g @github/copilot
//	curl -fsSL https://gh.io/copilot-install | bash
//
// Version:          v1.0.11 (199 releases, 9.6k stars, very active)
// Platforms:        Windows (PowerShell 6+), macOS, Linux
// Default model:    Claude Sonnet 4.5 (selectable: GPT-5, others)
// Auth:             GitHub OAuth (device flow) or PAT via GH_TOKEN
//
// MODES:
//
//	Interactive:    copilot                        (REPL session)
//	Programmatic:   copilot -p "prompt" --flags     (one-shot, exits)
//	Plan:           Shift+Tab cycles to plan mode   (structured planning)
//	Autopilot:      --experimental enables autopilot (keeps working)
//
// PERMISSIONS (equivalent of --dangerously-skip-permissions):
//
//	--allow-all-tools           Full YOLO mode
//	--allow-tool='shell'        Allow shell commands
//	--allow-tool='write'        Allow file writes
//	--allow-tool='MCP(tool)'    Allow specific MCP tool
//	--deny-tool='shell(rm)'     Deny specific command
//	/yolo                       Slash command in interactive mode
//
// SESSION STORAGE (confirmed from Dispatch OSS):
//
//	Database:       ~/.copilot/session-store.db    (SQLite)
//	Plan files:     ~/.copilot/session-state/{session-id}/plan.md
//	Contains:       session ID, working dir, git branch, repo,
//	                full conversation history, checkpoints,
//	                files touched, git refs, summaries, timestamps,
//	                turn count
//
// CONFIG DIRECTORY:    ~/.copilot/
// CUSTOM INSTRUCTIONS: ~/.copilot/ + .github/ (all files combine)
// MCP SERVERS:         Supported, ships with GitHub MCP server
// HOOKS:               Same hooks system as Copilot coding agent
// LSP:                 ~/.copilot/lsp-config.json or .github/lsp.json
// ACP:                 Built-in ACP server for third-party integration
// AUTO-COMPACTION:     /compact or auto at 95% context (infinite sessions)
// SKILLS:              Supported (npx skills ecosystem)
// MEMORY:              Copilot Memory (persistent repo knowledge)
//
// ═══════════════════════════════════════════════════════════════════
// 2. PRESET DEFINITION
// ═══════════════════════════════════════════════════════════════════
//
// Add to builtinPresets map in internal/config/agents.go:
//
//	AgentCopilot: {
//	    Name:                AgentCopilot,
//	    Command:             "copilot",
//	    Args:                []string{"--allow-all-tools"},
//	    ProcessNames:        []string{"copilot", "node"},
//	    SessionIDEnv:        "",  // See §3 — discover from SQLite or env
//	    ResumeFlag:          "--resume",
//	    ContinueFlag:        "--continue",
//	    ResumeStyle:         "flag",
//	    SupportsHooks:       true,
//	    SupportsForkSession: false,  // Not yet confirmed
//	    NonInteractive: &NonInteractiveConfig{
//	        PromptFlag: "-p",
//	        // OutputFlag: "",  // TBD — watch for --json support
//	    },
//	    // Runtime defaults
//	    PromptMode:             "arg",
//	    ConfigDirEnv:           "",  // TBD — discover if COPILOT_CONFIG_DIR exists
//	    ConfigDir:              ".copilot",
//	    HooksProvider:          "copilot",
//	    HooksDir:               ".copilot",
//	    HooksSettingsFile:      "",  // TBD — discover hooks file format
//	    ReadyDelayMs:           5000,
//	    InstructionsFile:       "AGENTS.md",
//	    EmitsPermissionWarning: false,  // --allow-all-tools skips prompts
//	    HasTurnBoundaryDrain:   false,  // Needs nudge-poller fallback
//	    ACP: &ACPConfig{
//	        Mode: ACPModeNative,  // Built-in ACP server
//	    },
//	},
//
// ═══════════════════════════════════════════════════════════════════
// 3. SESSION MANAGEMENT STRATEGY
// ═══════════════════════════════════════════════════════════════════
//
// Copilot CLI persists ALL session state to SQLite, unlike Claude which
// relies on tmux capture-pane for context recovery. This enables:
//
// A. SESSION DISCOVERY (replaces tmux list-sessions for copilot)
//
//	Read ~/.copilot/session-store.db to enumerate sessions.
//	Dispatch (github.com/jongio/dispatch) does this with modernc SQLite.
//	Gas Town could:
//	- Import Dispatch's data package as a library
//	- Or query SQLite directly (pure Go, no CGO)
//	- Session schema includes: id, dir, branch, repo, updated, turns
//
// B. CONTEXT RECOVERY (replaces tmux capture-pane for seance)
//
//	Full conversation history is in the database — richer than
//	terminal buffer capture. Seance for copilot should query SQLite
//	instead of tmux.
//
// C. STALL DETECTION (replaces tmux pane monitoring for witness)
//
//	Check session's last-updated timestamp from SQLite.
//	If stale > threshold, flag as stuck.
//
// D. SESSION RESUME
//
//	Dispatch confirms resume works. It launches copilot with session ID.
//	Gas Town should use the same mechanism.
//	The custom_command pattern in Dispatch: "copilot --resume {sessionId}"
//
// ═══════════════════════════════════════════════════════════════════
// 4. THREE EXECUTION PATHS
// ═══════════════════════════════════════════════════════════════════
//
// PATH A: PROGRAMMATIC (no tmux required)
//
//	Use case:  One-shot tasks, CI/CD, batch dispatch
//	Command:   copilot -p "Fix bd-42: <description>" --allow-all-tools
//	Lifecycle: Start → work → exit (no persistent session)
//	Nudge:     N/A (non-interactive)
//	Recovery:  Session stored in SQLite automatically
//
// PATH B: INTERACTIVE + TMUX (full polecat lifecycle)
//
//	Use case:  Long iterative sessions, multi-polecat parallel work
//	Command:   tmux new-session -s gt-<rig>-p-<name> copilot --allow-all-tools
//	Lifecycle: Spawn → work → nudge → done → idle (standard polecat)
//	Nudge:     tmux send-keys (same as Claude)
//	Recovery:  SQLite store + tmux capture-pane (belt and suspenders)
//
// PATH C: INTERACTIVE + ACP (tmux optional, structured comms)
//
//	Use case:  Structured orchestration without keystroke injection
//	Command:   copilot (with ACP server enabled)
//	Lifecycle: Spawn → work → done (ACP handles message delivery)
//	Nudge:     ACP protocol call (replaces tmux send-keys)
//	Recovery:  SQLite store
//	Status:    EXPERIMENTAL — ACP is new, investigate feasibility
//
// RECOMMENDED DEFAULT: Path B (same as Claude, proven pattern)
// INVESTIGATE NEXT:    Path A for non-interactive dispatch
//
//	Path C for tmux-free future
//
// ═══════════════════════════════════════════════════════════════════
// 5. DISPATCH AS A COMPONENT
// ═══════════════════════════════════════════════════════════════════
//
// Dispatch (github.com/jongio/dispatch) is:
//   - MIT licensed, Go, Bubble Tea (same stack as Gas Town)
//   - 92% Go, well-tested (79% coverage, 39 test files)
//   - Pure-Go SQLite (modernc.org/sqlite — no CGO dependency)
//   - Reads ~/.copilot/session-store.db directly
//   - v0.6.0, 7 releases in 2 weeks, actively maintained
//
// Integration options:
//
//	OPTION 1: Import as Go module
//	  import "github.com/jongio/dispatch/internal/data"
//	  NOTE: internal/ packages can't be imported externally in Go.
//	  Would need a PR to Dispatch exposing a public API, or fork.
//
//	OPTION 2: Vendor the SQLite reader
//	  Copy the session-store reading logic (~200 lines) into Gas Town.
//	  Depends on: modernc.org/sqlite (pure Go, no system deps)
//	  Schema is simple — sessions table with standard columns.
//
//	OPTION 3: Shell out to dispatch CLI
//	  dispatch --json (if it adds JSON output) or parse TUI output.
//	  Least clean, most fragile.
//
//	OPTION 4: Direct SQLite queries in Gas Town
//	  Add modernc.org/sqlite to go.mod (Gas Town already uses Go).
//	  Query ~/.copilot/session-store.db directly.
//	  Minimal dependency, full control.
//
// RECOMMENDATION: Option 4 (direct SQLite queries).
//   - Gas Town already manages its own data layer
//   - modernc SQLite is pure Go, zero system dependencies
//   - Session schema is likely simple (id, dir, branch, updated, etc.)
//   - Avoids external tool dependencies
//
// ═══════════════════════════════════════════════════════════════════
// 6. INSTALLATION & PLATFORM SUPPORT
// ═══════════════════════════════════════════════════════════════════
//
// Gas Town preset requires copilot CLI to be installed.
// Detection: check if "copilot" is on PATH.
//
// POWERSHELL:
//
//	winget install GitHub.Copilot
//	# Or: npm install -g @github/copilot
//	# Binary: copilot.exe in PATH
//	# Session DB: $env:USERPROFILE\.copilot\session-store.db
//
// BASH / ZSH (macOS/Linux):
//
//	brew install copilot-cli
//	# Or: curl -fsSL https://gh.io/copilot-install | bash
//	# Binary: copilot in $HOME/.local/bin or /usr/local/bin
//	# Session DB: ~/.copilot/session-store.db
//
// VS CODE INTEGRATION:
//
//	Copilot CLI is separate from the VS Code Copilot extension.
//	Both can coexist. For VS Code-hosted agents, use:
//	  - GasTownBridgeExt (terminal event hooks, heartbeat, session save)
//	  - GasTownHooksMCPapp (MCP tools for hooks/mail/events)
//	For CLI agents, use the preset defined in §2.
//
// AUTH:
//
//	copilot uses GitHub OAuth (device flow) on first launch.
//	Alternatively: set GH_TOKEN or GITHUB_TOKEN env var with a PAT
//	that has "Copilot Requests" permission enabled.
//	For Gas Town polecats: inject GH_TOKEN into the tmux session env.
//
// ═══════════════════════════════════════════════════════════════════
// 7. FIELDS TO DISCOVER (requires hands-on testing)
// ═══════════════════════════════════════════════════════════════════
//
// These fields need to be confirmed by running copilot CLI:
//
//	SessionIDEnv        What env var (if any) holds the session ID?
//	                    Try: COPILOT_SESSION_ID, check copilot --help
//
//	ResumeFlag          Dispatch uses session ID to resume. Verify:
//	                    copilot --resume <session-id>
//
//	ContinueFlag        Does copilot --continue resume the last session?
//
//	ReadyPromptPrefix   What does the idle prompt look like?
//	                    Launch copilot, observe the prompt character.
//
//	HooksSettingsFile   What's the hooks config file name?
//	                    Check ~/.copilot/ after enabling hooks.
//
//	ConfigDirEnv        Is there a COPILOT_CONFIG_DIR env var?
//	                    Check copilot --help or source code.
//
//	SQLite schema       What columns does session-store.db have?
//	                    Run: sqlite3 ~/.copilot/session-store.db .schema
//
//	--json output       Does copilot -p "x" --json work?
//	                    Useful for programmatic result parsing.
//
// ═══════════════════════════════════════════════════════════════════
// 8. HOOKS INTEGRATION
// ═══════════════════════════════════════════════════════════════════
//
// Copilot CLI supports the same hooks system as the coding agent.
// Reference: https://docs.github.com/en/copilot/concepts/agents/
//
//	coding-agent/about-hooks
//
// Gas Town hooks installer (internal/cmd/hooks.go) needs a new
// provider type: "copilot"
//
// Hook lifecycle slots to map:
//
//	PrePrompt          → Gas Town startup (gt prime, context load)
//	UserPromptSubmit   → Turn boundary (drain nudge queue)
//	ToolUseStart       → Telemetry / guard rails
//
// Config merger (3-tier):
//
//	~/.gt/hooks-base.json            → shared base
//	~/.gt/hooks-overrides/crew.json  → role-specific
//	.copilot/hooks.json              → agent-specific (TBD file name)
//
// If copilot hooks match Claude's settings.json pattern:
//
//	HooksProvider:       "copilot"
//	HooksDir:            ".copilot"
//	HooksSettingsFile:   "hooks.json"  // or "settings.json" — TBD
//	HooksUseSettingsDir: false         // TBD
//
// If copilot hooks are informational-only (no executable lifecycle):
//
//	HooksInformational:    true
//	HasTurnBoundaryDrain:  false
//	→ Gas Town falls back to nudge-poller for queue draining
//
// ═══════════════════════════════════════════════════════════════════
// 9. DIFFERENCES FROM CLAUDE PRESET
// ═══════════════════════════════════════════════════════════════════
//
// | Feature              | Claude                          | Copilot CLI                      |
// |----------------------|---------------------------------|----------------------------------|
// | Binary               | claude                          | copilot                          |
// | YOLO flag            | --dangerously-skip-permissions  | --allow-all-tools                |
// | Session storage      | In-memory (tmux-dependent)      | SQLite (~/.copilot/session-store) |
// | Non-interactive      | No clean equivalent             | copilot -p "prompt" --flags      |
// | ACP                  | No                              | Built-in ACP server              |
// | Plan mode            | No                              | Shift+Tab or --plan              |
// | Autopilot            | No                              | --experimental (keeps working)   |
// | Context management   | Manual (/compact in some builds)| Auto at 95% + /compact           |
// | Model selection      | Fixed                           | --model or /model                |
// | Permission warning   | Yes (must acknowledge)          | No (--allow-all-tools skips)     |
// | Hooks                | .claude/settings.json           | TBD (.copilot/hooks.json?)       |
// | Fork session         | Yes (--fork-session)            | Not confirmed                    |
// | Skills               | No                              | npx skills ecosystem             |
// | Memory               | No                              | Copilot Memory (persistent)      |
// | LSP                  | No                              | Built-in LSP support             |
// | Config dir           | .claude/                        | .copilot/                        |
// | Instructions         | CLAUDE.md                       | AGENTS.md                        |
// | Install (Windows)    | npm                             | winget install GitHub.Copilot    |
// | Auth                 | Anthropic API key               | GitHub OAuth / GH_TOKEN          |
//
// KEY ADVANTAGE: SQLite session store means Gas Town gets richer
// session intelligence (full conversation, checkpoints, files touched)
// without depending on tmux capture-pane. This is strictly better
// for seance, witness stall detection, and session enumeration.
//
// KEY ADVANTAGE: Non-interactive mode (copilot -p) enables tmux-free
// dispatch for one-shot tasks. Claude has no equivalent.
//
// ═══════════════════════════════════════════════════════════════════
// 10. IMPLEMENTATION ORDER
// ═══════════════════════════════════════════════════════════════════
//
// Phase 1: Stub preset (this spec → agents.go entry)
//   - Add AgentCopilot to builtinPresets with known fields
//   - Leave TBD fields empty
//   - No behavior changes, just registration
//
// Phase 2: Hands-on discovery
//   - Install copilot CLI
//   - Run it, observe prompt, test --resume, check env vars
//   - Inspect ~/.copilot/session-store.db schema
//   - Fill in SessionIDEnv, ReadyPromptPrefix, hooks file
//
// Phase 3: Session reader
//   - Add modernc.org/sqlite to go.mod
//   - Implement CopilotSessionReader in internal/copilot/
//   - Wire into seance (session enumeration + context recovery)
//   - Wire into witness (stall detection via last-updated)
//
// Phase 4: Non-interactive dispatch
//   - Add Path A support: copilot -p "prompt" --allow-all-tools
//   - No tmux needed for one-shot tasks
//   - Wire into gt sling for programmatic work assignment
//
// Phase 5: Hooks integration
//   - Add "copilot" hooks provider
//   - Implement hooks installer for .copilot/ directory
//   - Test turn-boundary drain or confirm nudge-poller needed
//
// Phase 6: ACP investigation (future)
//   - Test ACP server capabilities
//   - Evaluate as replacement for tmux send-keys nudge
//   - If viable, add Path C support
//
// ═══════════════════════════════════════════════════════════════════
// 11. SKILL PACKAGE (distribution for GHCP CLI users)
// ═══════════════════════════════════════════════════════════════════
//
// The optimal distribution channel is a SKILL + MCP SERVER + CUSTOM AGENT
// bundled together. Copilot CLI supports all three natively.
//
// Package location: gastown/plugins/copilot-cli/
//
// Structure:
//
//	plugins/copilot-cli/
//	├── install.sh                          # Bash installer
//	├── install.ps1                         # PowerShell installer
//	├── mcp-config-fragment.json            # MCP server entry to merge
//	├── skills/gastown/
//	│   ├── SKILL.md                        # Skill manifest + instructions
//	│   └── references/
//	│       ├── polecat-lifecycle.md        # Polecat state machine
//	│       ├── mail-protocol.md            # Agent mail routing
//	│       ├── issue-workflow.md           # bd issue lifecycle
//	│       └── landing-the-plane.md        # Session completion checklist
//	└── agents/
//	    └── gastown-crew.md                 # Custom agent profile
//
// Installation targets:
//
//	~/.copilot/skills/gastown/              # Skill (persists across projects)
//	~/.copilot/agents/gastown-crew.md       # Custom agent profile
//	~/.copilot/mcp-config.json              # MCP server entry (merged)
//
// User install:
//
//	# PowerShell (Windows)
//	.\plugins\copilot-cli\install.ps1
//	# Bash (macOS/Linux)
//	bash plugins/copilot-cli/install.sh
//
// User verify:
//
//	.\install.ps1 -Check   # or: bash install.sh --check
//
// User launch:
//
//	copilot                           # Interactive with Gas Town skill loaded
//	copilot --agent=gastown-crew      # Launch as Gas Town polecat
//	copilot -p "gt bd ready" --yolo   # Quick check for ready work
//
// FUTURE: Publish to npx skills ecosystem
//
//	npx skills add steveyegge/gastown@gastown-copilot -g -y
//	This requires adding a skills.json manifest to the repo root.
//
// ═══════════════════════════════════════════════════════════════════
// 12. COPILOT CLI EXTENSION SURFACE SUMMARY
// ═══════════════════════════════════════════════════════════════════
//
// | Mechanism        | File Location                    | What Gas Town Uses It For    |
// |------------------|----------------------------------|-----------------------------|
// | Skill            | ~/.copilot/skills/gastown/       | Workflow knowledge + refs    |
// | Custom Agent     | ~/.copilot/agents/gastown-crew.md| Polecat persona + rules     |
// | MCP Server       | ~/.copilot/mcp-config.json       | Tools (mail, hooks, events) |
// | Custom Instr.    | .github/copilot-instructions.md  | Per-repo conventions        |
// | Hooks            | .copilot/hooks/                  | Turn-boundary queue drain   |
// | AGENTS.md        | repo root                        | Already exists in gastown   |
//
// All five mechanisms are additive — they compose into a single
// cohesive experience when a Copilot CLI user works on a Gas Town project.
package config
