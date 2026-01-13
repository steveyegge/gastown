package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

var cleanupCmd = &cobra.Command{
	Use:     "cleanup",
	GroupID: GroupServices,
	Short:   "Manage orphaned processes and sessions",
	Long: `Manage orphaned processes and sessions in Gas Town.

Subcommands:
  orphans  - Find and report orphaned work (work assigned to dead agents)
  sessions - Clean up dead/zombie tmux sessions
  stale    - Clean up stale polecats (way behind main branch)`,
	RunE: requireSubcommand,
}

var cleanupOrphansCmd = &cobra.Command{
	Use:   "orphans",
	Short: "Find and report orphaned work",
	Long: `Find work assigned to dead agents.

Orphaned work is work that has been assigned to a polecat agent (hook_bead set)
but the agent's tmux session is no longer running. This can happen when:
- Agent crashed without cleanup
- Session was killed manually
- System reboot

The daemon normally detects this and notifies the Witness automatically,
but this command allows manual inspection and triggering of recovery.`,
	RunE: runCleanupOrphans,
}

var cleanupSessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "Clean up dead/zombie tmux sessions",
	Long: `Clean up tmux sessions that are dead or zombie.

A zombie session is one where the tmux session exists but the agent
process (Claude) is no longer running. These sessions can accumulate
and should be cleaned up.`,
	RunE: runCleanupSessions,
}

var cleanupStaleCmd = &cobra.Command{
	Use:   "stale",
	Short: "Clean up stale polecats",
	Long: `Clean up stale polecats (way behind main branch).

Wraps 'gt polecat stale --cleanup' for all rigs.
A stale polecat is one that:
- Has no active tmux session
- Is way behind main branch (>threshold commits)
- Has no uncommitted work`,
	RunE: runCleanupStale,
}

var (
	cleanupDryRun bool
	cleanupForce  bool
	cleanupAll    bool
)

func init() {
	cleanupOrphansCmd.Flags().BoolVar(&cleanupDryRun, "dry-run", false, "Show what would be done without taking action")
	cleanupSessionsCmd.Flags().BoolVar(&cleanupDryRun, "dry-run", false, "Show what would be done without taking action")
	cleanupSessionsCmd.Flags().BoolVar(&cleanupForce, "force", false, "Force cleanup even for running sessions")
	cleanupStaleCmd.Flags().BoolVar(&cleanupDryRun, "dry-run", false, "Show what would be done without taking action")
	cleanupStaleCmd.Flags().BoolVar(&cleanupAll, "all", false, "Clean up stale polecats from all rigs")

	cleanupCmd.AddCommand(cleanupOrphansCmd)
	cleanupCmd.AddCommand(cleanupSessionsCmd)
	cleanupCmd.AddCommand(cleanupStaleCmd)

	rootCmd.AddCommand(cleanupCmd)
}

// OrphanedWork represents work assigned to a dead agent
type OrphanedWork struct {
	AgentID     string `json:"agent_id"`
	HookBead    string `json:"hook_bead"`
	SessionName string `json:"session_name"`
	RigName     string `json:"rig_name"`
}

func runCleanupOrphans(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	tmuxClient := tmux.NewTmux()

	// Get all agent beads with hooked work
	agentCmd := exec.Command("bd", "list", "--type=agent", "--json")
	agentCmd.Dir = townRoot

	output, err := agentCmd.Output()
	if err != nil {
		return fmt.Errorf("loading agent data: %w", err)
	}

	var agents []struct {
		ID       string `json:"id"`
		HookBead string `json:"hook_bead"`
	}

	if err := json.Unmarshal(output, &agents); err != nil {
		return fmt.Errorf("parsing agent data: %w", err)
	}

	// Find orphaned work
	var orphans []OrphanedWork

	for _, agent := range agents {
		// Skip agents without hooked work
		if agent.HookBead == "" {
			continue
		}

		// Check if this is a polecat agent
		if !strings.Contains(agent.ID, "-polecat-") {
			continue
		}

		// Extract rig and polecat name from agent ID
		rigName := extractRigFromAgentID(agent.ID)
		polecatName := extractPolecatNameFromAgentID(agent.ID)
		sessionName := fmt.Sprintf("gt-%s-%s", rigName, polecatName)

		// Check if session is alive
		if tmuxClient.IsClaudeRunning(sessionName) {
			continue // Session alive, not orphaned
		}

		// Session dead but has hooked work = orphaned!
		orphans = append(orphans, OrphanedWork{
			AgentID:     agent.ID,
			HookBead:    agent.HookBead,
			SessionName: sessionName,
			RigName:     rigName,
		})
	}

	// Report findings
	if len(orphans) == 0 {
		fmt.Printf("%s No orphaned work found\n", style.Bold.Render("✓"))
		return nil
	}

	fmt.Printf("%s Found %d orphaned work items:\n\n", style.Bold.Render("⚠"), len(orphans))

	for _, orphan := range orphans {
		fmt.Printf("  %s Agent: %s\n", style.Bold.Render("●"), orphan.AgentID)
		fmt.Printf("    Session: %s %s\n", orphan.SessionName, style.Dim.Render("(dead)"))
		fmt.Printf("    Hook: %s\n", orphan.HookBead)
		fmt.Printf("    Rig: %s\n\n", orphan.RigName)
	}

	if cleanupDryRun {
		fmt.Println(style.Dim.Render("(dry-run mode: no actions taken)"))
		return nil
	}

	// Notify witnesses about orphaned work
	fmt.Println("Notifying witnesses...")
	for _, orphan := range orphans {
		if err := notifyWitnessOfOrphan(townRoot, orphan); err != nil {
			fmt.Printf("%s Failed to notify witness for %s: %v\n",
				style.Bold.Render("✗"), orphan.AgentID, err)
		} else {
			fmt.Printf("%s Notified %s/witness about %s\n",
				style.Bold.Render("✓"), orphan.RigName, orphan.AgentID)
		}
	}

	return nil
}

func runCleanupSessions(cmd *cobra.Command, args []string) error {
	_, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	tmuxClient := tmux.NewTmux()

	// Get all tmux sessions
	sessions, err := tmuxClient.ListSessions()
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}

	var deadSessions []string
	var zombieSessions []string

	for _, session := range sessions {
		if session == "" {
			continue
		}

		// Only check Gas Town sessions
		if !isGasTownSession(session) {
			continue
		}

		// Check if session exists
		exists, _ := tmuxClient.HasSession(session)
		if !exists {
			deadSessions = append(deadSessions, session)
			continue
		}

		// Check if agent is running in the session
		if !tmuxClient.IsAgentRunning(session) {
			zombieSessions = append(zombieSessions, session)
		}
	}

	if len(deadSessions) == 0 && len(zombieSessions) == 0 {
		fmt.Printf("%s No dead or zombie sessions found\n", style.Bold.Render("✓"))
		return nil
	}

	// Report findings
	if len(deadSessions) > 0 {
		fmt.Printf("%s Found %d dead sessions:\n", style.Bold.Render("⚠"), len(deadSessions))
		for _, session := range deadSessions {
			fmt.Printf("  %s %s\n", style.Dim.Render("○"), session)
		}
		fmt.Println()
	}

	if len(zombieSessions) > 0 {
		fmt.Printf("%s Found %d zombie sessions:\n", style.Bold.Render("⚠"), len(zombieSessions))
		for _, session := range zombieSessions {
			fmt.Printf("  %s %s\n", style.Dim.Render("○"), session)
		}
		fmt.Println()
	}

	if cleanupDryRun {
		fmt.Println(style.Dim.Render("(dry-run mode: no sessions killed)"))
		return nil
	}

	// Kill zombie sessions
	if len(zombieSessions) > 0 {
		fmt.Println("Cleaning up zombie sessions...")
		for _, session := range zombieSessions {
			if err := tmuxClient.KillSession(session); err != nil {
				fmt.Printf("%s Failed to kill %s: %v\n",
					style.Bold.Render("✗"), session, err)
			} else {
				fmt.Printf("%s Killed zombie session: %s\n",
					style.Bold.Render("✓"), session)
			}
		}
	}

	// Note about dead sessions
	if len(deadSessions) > 0 {
		fmt.Printf("\n%s Dead sessions don't need cleanup (already gone)\n",
			style.Dim.Render("ℹ"))
	}

	return nil
}

func runCleanupStale(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Get list of rigs
	rigs, err := getKnownRigs(townRoot)
	if err != nil {
		return fmt.Errorf("getting rigs: %w", err)
	}

	if len(rigs) == 0 {
		fmt.Println("No rigs found")
		return nil
	}

	fmt.Printf("Checking for stale polecats in %d rigs...\n\n", len(rigs))

	for _, rigName := range rigs {
		fmt.Printf("%s Rig: %s\n", style.Bold.Render("●"), rigName)

		// Run gt polecat stale for this rig
		staleArgs := []string{"polecat", "stale", rigName}
		if !cleanupDryRun {
			staleArgs = append(staleArgs, "--cleanup")
		}

		staleCmd := exec.Command("gt", staleArgs...)
		staleCmd.Dir = townRoot
		staleCmd.Stdout = os.Stdout
		staleCmd.Stderr = os.Stderr

		if err := staleCmd.Run(); err != nil {
			fmt.Printf("  %s Error checking stale polecats: %v\n",
				style.Bold.Render("✗"), err)
		}
		fmt.Println()
	}

	if cleanupDryRun {
		fmt.Println(style.Dim.Render("(dry-run mode: use without --dry-run to clean up)"))
	}

	return nil
}

func notifyWitnessOfOrphan(townRoot string, orphan OrphanedWork) error {
	witnessAddr := orphan.RigName + "/witness"
	subject := fmt.Sprintf("ORPHANED_WORK: %s", orphan.AgentID)
	body := fmt.Sprintf(`Agent %s has orphaned work.

The agent's tmux session (%s) is dead but work is still assigned.

hook_bead: %s

Action needed: Restart the agent or reassign the work.`,
		orphan.AgentID, orphan.SessionName, orphan.HookBead)

	cmd := exec.Command("gt", "mail", "send", witnessAddr, "-s", subject, "-m", body)
	cmd.Dir = townRoot

	return cmd.Run()
}

func extractRigFromAgentID(agentID string) string {
	// Format: gt-<rig>-polecat-<name> → <rig>
	parts := strings.Split(agentID, "-")
	if len(parts) >= 3 {
		return parts[1]
	}
	return ""
}

func extractPolecatNameFromAgentID(agentID string) string {
	// Format: gt-<rig>-polecat-<name> → <name>
	parts := strings.Split(agentID, "-")
	if len(parts) >= 4 && parts[2] == "polecat" {
		return parts[3]
	}
	return ""
}

func getKnownRigs(townRoot string) ([]string, error) {
	// Get rigs from the rigs directory
	rigsDir := townRoot + "/rigs"
	entries, err := os.ReadDir(rigsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var rigs []string
	for _, entry := range entries {
		if entry.IsDir() {
			// Check if it's a valid rig (has rig.yaml)
			rigYAML := rigsDir + "/" + entry.Name() + "/rig.yaml"
			if _, err := os.Stat(rigYAML); err == nil {
				rigs = append(rigs, entry.Name())
			}
		}
	}

	return rigs, nil
}
