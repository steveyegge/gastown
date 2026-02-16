package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/version"
)

var infoCmd = &cobra.Command{
	Use:     "info",
	GroupID: GroupDiag,
	Short:   "Show Gas Town information and what's new",
	Long: `Display information about the current Gas Town installation.

This command shows:
  - Version information
  - What's new in recent versions (with --whats-new flag)

Examples:
  gt info
  gt info --whats-new
  gt info --whats-new --json`,
	Run: func(cmd *cobra.Command, args []string) {
		whatsNewFlag, _ := cmd.Flags().GetBool("whats-new")
		jsonFlag, _ := cmd.Flags().GetBool("json")

		if whatsNewFlag {
			showWhatsNew(jsonFlag)
			return
		}

		// Default: show basic info
		info := map[string]interface{}{
			"version": Version,
			"build":   Build,
		}

		if commit := resolveCommitHash(); commit != "" {
			info["commit"] = version.ShortCommit(commit)
		}
		if branch := resolveBranch(); branch != "" {
			info["branch"] = branch
		}

		if jsonFlag {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(info)
			return
		}

		fmt.Printf("Gas Town v%s (%s)\n", Version, Build)
		if commit, ok := info["commit"].(string); ok {
			if branch, ok := info["branch"].(string); ok {
				fmt.Printf("  %s@%s\n", branch, commit)
			} else {
				fmt.Printf("  %s\n", commit)
			}
		}
		fmt.Println("\nUse 'gt info --whats-new' to see recent changes")
	},
}

// VersionChange represents agent-relevant changes for a specific version
type VersionChange struct {
	Version string   `json:"version"`
	Date    string   `json:"date"`
	Changes []string `json:"changes"`
}

// versionChanges contains agent-actionable changes for recent versions
var versionChanges = []VersionChange{
	{
		Version: "0.7.0",
		Date:    "2026-02-15",
		Changes: []string{
			"NEW: Convoy ownership — --owned flag, --merge strategy (direct/mr/local), gt convoy land",
			"NEW: gt done checkpoint-based resilience — recovery from session death mid-completion",
			"NEW: Agent factory — data-driven preset registry (no more provider switch statements)",
			"NEW: Gemini CLI and GitHub Copilot CLI integrations as runtime adapters",
			"NEW: Non-destructive nudge delivery — queue and wait-idle modes",
			"NEW: Submodule support for worktrees and refinery merge queue",
			"NEW: Dashboard rich activity timeline, mobile layout, toast notifications",
			"NEW: Auto-dismiss stalled polecat permission prompts",
			"NEW: JSON patrol receipts for stale/orphan verdicts",
			"NEW: Orphaned molecule detection and auto-close (mol-polecat-work)",
			"NEW: Merge queue --verify flag to detect orphaned entries",
			"NEW: Mayor GT_ROLE Task tool guard",
			"NEW: Remote hook attach (gt hook attach with remote target)",
			"CHANGED: Beads Classic dead code removed (-924 lines SQLite/JSONL/sync)",
			"CHANGED: Session prefixes now registry-based (replaces hardcoded gt-* patterns)",
			"CHANGED: Molecule step readiness delegated to bd ready --mol",
			"FIX: Race conditions in web dashboard, feed curator, TUI convoy/feed",
			"FIX: Convoy lifecycle guards extended to batch auto-close and synthesis",
			"FIX: Rig remove now kills tmux sessions, aborts on kill failures",
			"FIX: Don't infer RoleMayor from town root cwd",
			"FIX: Polecat zero-commit completion blocked",
			"FIX: Signal stop hook infinite loop prevention",
			"FIX: 50+ additional bug fixes from community contributions",
		},
	},
	{
		Version: "0.6.0",
		Date:    "2026-02-15",
		Changes: []string{
			"NEW: Dolt-native architecture — SQLite fully removed, Dolt is the only backend",
			"NEW: gt dolt command — server management (start, stop, migrate, rollback, sync)",
			"NEW: gt install now folds Dolt identity, HQ database, and server start into one step",
			"NEW: Branch-per-polecat write isolation prevents Dolt conflicts",
			"NEW: Proactive Dolt health alerting in daemon (30s health ticker)",
			"NEW: Dashboard UX overhaul — 13 panels, SSE real-time updates, command palette",
			"NEW: launchd/systemd daemon supervision support",
			"NEW: Boot watchdog — ephemeral dog triages Deacon state each tick",
			"NEW: gt mol dag — DAG visualization for molecules",
			"NEW: Fan-out/gather parallel steps in patrol workflows",
			"NEW: gt compact — TTL-based wisp compaction",
			"NEW: Centralized hook management (gt hooks sync/diff/list)",
			"NEW: Persistent polecat identity model — agent beads survive nuke",
			"NEW: Convoy completion notifications pushed to active Mayor session",
			"NEW: --stdin flag for mail, nudge, handoff, escalate, sling (shell-safe)",
			"NEW: gt signal stop — turn-boundary messaging for clean stops",
			"NEW: gt rig settings — interactive rig settings management",
			"NEW: Dark mode CLI theme support",
			"NEW: C-b g keybinding for tmux agent switcher",
			"NEW: Auto-create DoltHub repos and configure remotes (gt dolt sync)",
			"CHANGED: gt status --fast optimized from ~5s to ~2s",
			"CHANGED: Centralized configuration — hardcoded timeouts moved to TownSettings",
			"CHANGED: Priority-based mail notifications prevent agent derailment",
			"FIX: flock-based locking across molecules, events, crew files, locks",
			"FIX: TOCTOU guards on Dolt startup, worktrees, cleanup, FindRigBeadsDir",
			"FIX: Security hardening — input validation, path traversal, shell injection prevention",
			"FIX: Dolt read-only state auto-recovery and split-brain prevention",
			"FIX: Session name parsing for hyphenated rig names",
			"FIX: 200+ bug fixes from community contributions and internal development",
		},
	},
	{
		Version: "0.5.0",
		Date:    "2026-01-22",
		Changes: []string{
			"NEW: gt mail read <index> - Read messages by inbox position",
			"NEW: gt mail hook - Shortcut for gt hook attach from mail",
			"NEW: --body alias for --message in gt mail send/reply",
			"NEW: gt bd alias for gt bead, gt work alias for gt hook",
			"NEW: OpenCode as built-in agent preset (gt config set agent opencode)",
			"NEW: Config-based role definition system",
			"NEW: Deacon icon in mayor status line",
			"NEW: gt hooks - Hook registry and install command",
			"NEW: Squash merge in refinery for cleaner history",
			"CHANGED: Parallel mail inbox queries (~6x speedup)",
			"FIX: Crew session stability - Don't kill pane processes on new sessions",
			"FIX: Auto-recover from stale tmux pane references",
			"FIX: KillPaneProcesses now kills pane process itself, not just descendants",
			"FIX: Convoy ID propagation in refinery and convoy watcher",
			"FIX: Multi-repo routing for custom types and role slots",
		},
	},
	{
		Version: "0.4.0",
		Date:    "2026-01-19",
		Changes: []string{
			"FIX: Orphan cleanup skips valid tmux sessions - Prevents false kills of witnesses/refineries/deacon during startup by checking gt-*/hq-* session membership",
		},
	},
	{
		Version: "0.3.1",
		Date:    "2026-01-17",
		Changes: []string{
			"FIX: Orphan cleanup on macOS - TTY comparison now handles macOS '??' format",
			"FIX: Session kill orphan prevention - gt done and gt crew stop use KillSessionWithProcesses",
		},
	},
	{
		Version: "0.3.0",
		Date:    "2026-01-17",
		Changes: []string{
			"NEW: gt show/cat - Inspect bead contents and metadata",
			"NEW: gt orphans list/kill - Detect and clean up orphaned Claude processes",
			"NEW: gt convoy close - Manual convoy closure command",
			"NEW: gt commit/trail - Git wrappers with bead awareness",
			"NEW: Plugin system - gt plugin run/history, gt dispatch --plugin",
			"NEW: Beads-native messaging - Queue, channel, and group beads",
			"NEW: gt mail claim - Claim messages from queues",
			"NEW: gt polecat identity show - Display CV summary",
			"NEW: gastown-release molecule formula - Automated release workflow",
			"NEW: Parallel agent startup - Faster boot with concurrency limit",
			"NEW: Automatic orphan cleanup - Detect and kill orphaned processes",
			"NEW: Worktree setup hooks - Inject local configurations",
			"CHANGED: MR tracking via beads - Removed mrqueue package",
			"CHANGED: Desire-path commands - Agent ergonomics shortcuts",
			"CHANGED: Explicit escalation in polecat templates",
			"FIX: Kill process tree on shutdown - Prevents orphaned Claude processes",
			"FIX: Agent bead prefix alignment - Multi-hyphen IDs for consistency",
			"FIX: Idle Polecat Heresy warnings in templates",
			"FIX: Zombie session detection in doctor",
			"FIX: Windows build support with platform-specific handling",
		},
	},
	{
		Version: "0.2.0",
		Date:    "2026-01-04",
		Changes: []string{
			"NEW: Convoy Dashboard - Web UI for monitoring Gas Town (gt dashboard)",
			"NEW: Two-level beads architecture - hq-* prefix for town, rig prefixes for projects",
			"NEW: Multi-agent support with pluggable registry",
			"NEW: gt rig start/stop/restart/status - Multi-rig management commands",
			"NEW: Ephemeral polecat model - Immediate recycling after each work unit",
			"NEW: gt costs command - Session cost tracking and reporting",
			"NEW: Conflict resolution workflow for polecats with merge-slot gates",
			"NEW: gt convoy --tree and gt convoy check for cross-rig coordination",
			"NEW: Batch slinging - gt sling supports multiple beads at once",
			"NEW: spawn alias for start across all role subcommands",
			"NEW: gt mail archive supports multiple message IDs",
			"NEW: gt mail --all flag for clearing all mail",
			"NEW: Circuit breaker for stuck agents",
			"NEW: Binary age detection in gt status",
			"NEW: Shell completion installation instructions",
			"CHANGED: Handoff migrated to skills format",
			"CHANGED: Crew workers push directly to main (no PRs)",
			"CHANGED: Session names include town name",
			"FIX: Thread-safety for agent session resume",
			"FIX: Orphan daemon prevention via file locking",
			"FIX: Zombie tmux session cleanup",
			"FIX: Default branch detection (no longer hardcodes 'main')",
			"FIX: Enter key retry logic for reliable delivery",
			"FIX: Beads prefix routing for cross-rig operations",
		},
	},
	{
		Version: "0.1.1",
		Date:    "2026-01-02",
		Changes: []string{
			"FIX: Tmux keybindings scoped to Gas Town sessions only",
			"NEW: OSS project files - CHANGELOG.md, .golangci.yml, RELEASING.md",
			"NEW: Version bump script - scripts/bump-version.sh",
			"FIX: gt rig add and gt crew add CLI syntax documentation",
			"FIX: Rig prefix routing for agent beads",
			"FIX: Beads init targets correct database",
		},
	},
	{
		Version: "0.1.0",
		Date:    "2026-01-02",
		Changes: []string{
			"Initial public release of Gas Town",
			"NEW: Town structure - Hierarchical workspace with rigs, crews, and polecats",
			"NEW: Rig management - gt rig add/list/remove",
			"NEW: Crew workspaces - gt crew add for persistent developer workspaces",
			"NEW: Polecat workers - Transient agent workers managed by Witness",
			"NEW: Mayor - Global coordinator for cross-rig work",
			"NEW: Deacon - Town-level lifecycle patrol and heartbeat",
			"NEW: Witness - Per-rig polecat lifecycle manager",
			"NEW: Refinery - Merge queue processor with code review",
			"NEW: Convoy system - gt convoy create/list/status",
			"NEW: Sling workflow - gt sling <bead> <rig>",
			"NEW: Molecule workflows - Formula-based multi-step task execution",
			"NEW: Mail system - gt mail inbox/send/read",
			"NEW: Escalation protocol - gt escalate with severity levels",
			"NEW: Handoff mechanism - gt handoff for context-preserving session cycling",
			"NEW: Beads integration - Issue tracking via beads (bd commands)",
			"NEW: Tmux sessions with theming",
			"NEW: Status dashboard - gt status",
			"NEW: Activity feed - gt feed",
			"NEW: Nudge system - gt nudge for reliable message delivery",
		},
	},
}

// showWhatsNew displays agent-relevant changes from recent versions
func showWhatsNew(jsonOutput bool) {
	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]interface{}{
			"current_version": Version,
			"recent_changes":  versionChanges,
		})
		return
	}

	// Human-readable output
	fmt.Printf("\nWhat's New in Gas Town (Current: v%s)\n", Version)
	fmt.Println(strings.Repeat("=", 50))
	fmt.Println()

	for _, vc := range versionChanges {
		// Highlight if this is the current version
		versionMarker := ""
		if vc.Version == Version {
			versionMarker = " <- current"
		}

		fmt.Printf("## v%s (%s)%s\n\n", vc.Version, vc.Date, versionMarker)

		for _, change := range vc.Changes {
			fmt.Printf("  * %s\n", change)
		}
		fmt.Println()
	}

	fmt.Println("Tip: Use 'gt info --whats-new --json' for machine-readable output")
	fmt.Println()
}

func init() {
	infoCmd.Flags().Bool("whats-new", false, "Show agent-relevant changes from recent versions")
	infoCmd.Flags().Bool("json", false, "Output in JSON format")
	rootCmd.AddCommand(infoCmd)
}
