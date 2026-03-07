package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/daytona"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
)

var (
	polecatDiscoverReconcile bool
	polecatDiscoverDryRun    bool
	polecatDiscoverJSON      bool
)

var polecatDiscoverCmd = &cobra.Command{
	Use:   "discover <rig>",
	Short: "Discover daytona workspaces and reconcile with beads",
	Long: `Discover daytona workspaces owned by this installation for a rig.

Lists all daytona workspaces matching the rig's install prefix and cross-references
them with polecat agent beads to identify:

  - Matched: workspace has a corresponding agent bead (healthy)
  - Orphaned workspace: daytona workspace exists but no agent bead
  - Orphaned bead: agent bead references a workspace that doesn't exist

Use --reconcile to automatically fix orphans:
  - Orphaned workspaces are stopped (preserving state for investigation)
  - Orphaned beads have their daytona_workspace label cleared

Use --dry-run with --reconcile to preview what would happen without acting.

Only works for rigs with remote_backend configured.

Examples:
  gt polecat discover MyRig
  gt polecat discover MyRig --reconcile --dry-run
  gt polecat discover MyRig --reconcile
  gt polecat discover MyRig --json`,
	Args: cobra.ExactArgs(1),
	RunE: runPolecatDiscover,
}

// DiscoverResult holds the full discovery output for display and JSON.
type DiscoverResult struct {
	Rig              string            `json:"rig"`
	InstallPrefix    string            `json:"install_prefix"`
	Matched          []DiscoverMatch   `json:"matched,omitempty"`
	OrphanWorkspaces []DiscoverOrphan  `json:"orphan_workspaces,omitempty"`
	OrphanBeads      []DiscoverOrphan  `json:"orphan_beads,omitempty"`
	Reconciled       bool              `json:"reconciled"`
	DryRun           bool              `json:"dry_run,omitempty"`
	ReconcileActions []ReconcileActionEntry `json:"reconcile_actions,omitempty"`
}

// DiscoverMatch represents a workspace with a matching agent bead.
type DiscoverMatch struct {
	Polecat        string `json:"polecat"`
	WorkspaceName  string `json:"workspace_name"`
	WorkspaceState string `json:"workspace_state"`
	BeadID         string `json:"bead_id"`
}

// DiscoverOrphan represents an orphaned workspace or bead.
type DiscoverOrphan struct {
	Polecat        string `json:"polecat,omitempty"`
	WorkspaceName  string `json:"workspace_name,omitempty"`
	WorkspaceState string `json:"workspace_state,omitempty"`
	BeadID         string `json:"bead_id,omitempty"`
}

// ReconcileActionEntry records what reconciliation did (for display/JSON output).
type ReconcileActionEntry struct {
	Type    string `json:"type"`    // "stop_workspace" or "clear_bead"
	Target  string `json:"target"`  // workspace name or bead ID
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

func init() {
	polecatDiscoverCmd.Flags().BoolVar(&polecatDiscoverReconcile, "reconcile", false, "Automatically fix orphaned workspaces and beads")
	polecatDiscoverCmd.Flags().BoolVar(&polecatDiscoverDryRun, "dry-run", false, "Preview reconcile actions without performing them (requires --reconcile)")
	polecatDiscoverCmd.Flags().BoolVar(&polecatDiscoverJSON, "json", false, "Output as JSON")

	polecatCmd.AddCommand(polecatDiscoverCmd)
}

func runPolecatDiscover(cmd *cobra.Command, args []string) error {
	rigName := args[0]

	// Resolve rig
	townRoot, r, err := getRig(rigName)
	if err != nil {
		return err
	}

	// Load rig settings to check for RemoteBackend
	settingsPath := config.RigSettingsPath(r.Path)
	settings, err := config.LoadRigSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("loading rig settings: %w (is %s configured with remote_backend?)", err, rigName)
	}

	if settings.RemoteBackend == nil {
		return fmt.Errorf("rig %s does not have remote_backend configured — discovery only applies to daytona-backed rigs", rigName)
	}

	if err := settings.RemoteBackend.Validate(); err != nil {
		return err
	}

	// Load town config for installation prefix
	townConfigPath := constants.MayorTownPath(townRoot)
	townCfg, err := config.LoadTownConfig(townConfigPath)
	if err != nil {
		return fmt.Errorf("loading town config: %w", err)
	}

	shortID := townCfg.ShortInstallationID()
	if shortID == "" {
		return fmt.Errorf("installation ID not set in town config — run 'gt install' to initialize")
	}

	installPrefix := constants.InstallPrefix(shortID)

	// Create daytona client and discover workspaces
	client := daytona.NewClient(installPrefix)
	listCtx, listCancel := context.WithTimeout(context.Background(), constants.DaytonaListTimeout)
	defer listCancel()

	// List all owned workspaces from daytona
	workspaces, err := client.ListOwned(listCtx)
	if err != nil {
		return fmt.Errorf("listing daytona workspaces: %w", err)
	}

	// Gather agent beads for cross-referencing
	agentBeads := gatherPolecatAgentBeads(r, rigName)

	// Use daytona.DiscoverWorkspaces for the cross-referencing algorithm
	report := daytona.DiscoverWorkspaces(rigName, workspaces, agentBeads)

	// Convert report to display result
	result := reportToDiscoverResult(report, installPrefix)

	// Validate --dry-run requires --reconcile
	if polecatDiscoverDryRun && !polecatDiscoverReconcile {
		return fmt.Errorf("--dry-run requires --reconcile")
	}

	// Reconcile if requested — delegate to daytona.Reconcile
	if polecatDiscoverReconcile {
		result.Reconciled = true
		result.DryRun = polecatDiscoverDryRun

		bd := beads.New(r.Path)
		beadResetter := func(beadID string) error {
			empty := ""
			return bd.UpdateAgentDescriptionFields(beadID, beads.AgentFieldUpdates{
				DaytonaWorkspace: &empty,
			})
		}

		opts := daytona.ReconcileOptions{DryRun: polecatDiscoverDryRun}
		logger := log.New(io.Discard, "", 0)

		reconcileCtx, reconcileCancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer reconcileCancel()
		reconcileResult := daytona.Reconcile(reconcileCtx, client, report, opts, beadResetter, nil, logger)
		result.ReconcileActions = buildReconcileActions(report, reconcileResult, polecatDiscoverDryRun)
	}

	// Output
	if polecatDiscoverJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	printDiscoverResult(result)
	return nil
}

// gatherPolecatAgentBeads builds []daytona.AgentBead from the rig's polecat beads.
// This mirrors how the daemon gathers beads but uses the beads API directly
// instead of JSON parsing via the bd CLI.
func gatherPolecatAgentBeads(r *rig.Rig, rigName string) []daytona.AgentBead {
	bd := beads.New(r.Path)
	prefix := rigPrefix(r)
	polecatNames := listPolecatNames(r)

	var result []daytona.AgentBead
	for _, name := range polecatNames {
		beadID := beads.PolecatBeadIDWithPrefix(prefix, rigName, name)
		_, fields, err := bd.GetAgentBead(beadID)
		if err != nil || fields == nil {
			continue
		}
		if fields.DaytonaWorkspace == "" {
			continue
		}
		result = append(result, daytona.AgentBead{
			ID:                 beadID,
			Polecat:            name,
			DaytonaWorkspaceName: fields.DaytonaWorkspace,
			AgentState:         fields.AgentState,
			CertSerial:         fields.CertSerial,
		})
	}
	return result
}

// reportToDiscoverResult converts a daytona.ReconcileReport to the display-oriented DiscoverResult.
func reportToDiscoverResult(report *daytona.ReconcileReport, installPrefix string) *DiscoverResult {
	result := &DiscoverResult{
		Rig:           report.Rig,
		InstallPrefix: installPrefix,
	}

	for _, item := range report.Results {
		switch item.Action {
		case daytona.ActionHealthy:
			result.Matched = append(result.Matched, DiscoverMatch{
				Polecat:        item.Polecat,
				WorkspaceName:  item.Workspace.Name,
				WorkspaceState: item.Workspace.State,
				BeadID:         item.BeadID,
			})
		case daytona.ActionOrphanedWorkspace:
			result.OrphanWorkspaces = append(result.OrphanWorkspaces, DiscoverOrphan{
				Polecat:        item.Polecat,
				WorkspaceName:  item.Workspace.Name,
				WorkspaceState: item.Workspace.State,
			})
		case daytona.ActionOrphanedBead:
			result.OrphanBeads = append(result.OrphanBeads, DiscoverOrphan{
				Polecat: item.Polecat,
				BeadID:  item.BeadID,
			})
		}
	}

	return result
}

// buildReconcileActions builds display actions from the reconcile report and result.
// For dry-run, all actions are marked as success. For actual runs, errors from
// daytona.Reconcile are matched to actions by target name.
func buildReconcileActions(report *daytona.ReconcileReport, reconcileResult *daytona.ReconcileResult, dryRun bool) []ReconcileActionEntry {
	var actions []ReconcileActionEntry

	// Build a set of skipped workspace names from the report: workspaces in
	// transitional states are skipped by Reconcile without error, so they
	// should not be reported as successful stops.
	skippedWorkspaces := make(map[string]string) // name → state
	for _, item := range report.Results {
		if item.Action == daytona.ActionOrphanedWorkspace && item.Workspace != nil {
			switch item.Workspace.State {
			case "creating", "starting", "stopping":
				skippedWorkspaces[item.Workspace.Name] = item.Workspace.State
			}
		}
	}

	for _, item := range report.Results {
		switch item.Action {
		case daytona.ActionOrphanedWorkspace:
			if state, skipped := skippedWorkspaces[item.Workspace.Name]; skipped {
				actions = append(actions, ReconcileActionEntry{
					Type:    "skipped_workspace",
					Target:  item.Workspace.Name,
					Success: true,
					Error:   fmt.Sprintf("transitional state: %s", state),
				})
			} else {
				actions = append(actions, ReconcileActionEntry{
					Type:    "stop_workspace",
					Target:  item.Workspace.Name,
					Success: true,
				})
			}
		case daytona.ActionOrphanedBead:
			actions = append(actions, ReconcileActionEntry{
				Type:    "clear_bead",
				Target:  item.BeadID,
				Success: true,
			})
		}
	}

	if !dryRun {
		// Match errors from reconcileResult to actions by target name.
		for _, err := range reconcileResult.Errors {
			errStr := err.Error()
			for i := range actions {
				if strings.Contains(errStr, actions[i].Target) {
					actions[i].Success = false
					actions[i].Error = errStr
					break
				}
			}
		}
	}

	return actions
}

// listPolecatNames returns all polecat names for a rig by scanning the polecats directory.
func listPolecatNames(r *rig.Rig) []string {
	polecatsDir := filepath.Join(r.Path, "polecats")
	entries, err := os.ReadDir(polecatsDir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && !isHiddenDir(e.Name()) {
			names = append(names, e.Name())
		}
	}
	return names
}

func isHiddenDir(name string) bool {
	return len(name) > 0 && name[0] == '.'
}

func printDiscoverResult(result *DiscoverResult) {
	fmt.Printf("%s Daytona workspace discovery for rig %s\n", style.Bold.Render("🔍"), style.Bold.Render(result.Rig))
	fmt.Printf("   Install prefix: %s\n\n", style.Dim.Render(result.InstallPrefix))

	// Matched
	if len(result.Matched) > 0 {
		fmt.Printf("%s Matched (%d):\n", style.Success.Render("✓"), len(result.Matched))
		for _, m := range result.Matched {
			stateStr := formatWorkspaceState(m.WorkspaceState)
			fmt.Printf("  %s  %s  %s\n", style.Bold.Render(m.Polecat), stateStr, style.Dim.Render(m.WorkspaceName))
		}
		fmt.Println()
	}

	// Orphaned workspaces
	if len(result.OrphanWorkspaces) > 0 {
		fmt.Printf("%s Orphaned workspaces (%d) — workspace exists, no agent bead:\n", style.Warning.Render("⚠"), len(result.OrphanWorkspaces))
		for _, o := range result.OrphanWorkspaces {
			stateStr := formatWorkspaceState(o.WorkspaceState)
			fmt.Printf("  %s  %s  %s\n", style.Bold.Render(o.Polecat), stateStr, style.Dim.Render(o.WorkspaceName))
		}
		fmt.Println()
	}

	// Orphaned beads
	if len(result.OrphanBeads) > 0 {
		fmt.Printf("%s Orphaned beads (%d) — bead references workspace that doesn't exist:\n", style.Warning.Render("⚠"), len(result.OrphanBeads))
		for _, o := range result.OrphanBeads {
			fmt.Printf("  %s  %s\n", style.Bold.Render(o.Polecat), style.Dim.Render(o.BeadID))
		}
		fmt.Println()
	}

	// Summary
	total := len(result.Matched) + len(result.OrphanWorkspaces) + len(result.OrphanBeads)
	if total == 0 {
		fmt.Println("No daytona workspaces found for this rig.")
		return
	}

	// Reconciliation results
	if result.Reconciled && len(result.ReconcileActions) > 0 {
		header := "🔧"
		if result.DryRun {
			header = "🔍"
			fmt.Printf("%s Reconciliation preview (dry-run — no changes made):\n", style.Bold.Render(header))
		} else {
			fmt.Printf("%s Reconciliation actions:\n", style.Bold.Render(header))
		}
		for _, a := range result.ReconcileActions {
			prefix := ""
			if result.DryRun {
				prefix = "would "
			}
			if a.Success {
				fmt.Printf("  %s %s%s: %s\n", style.Success.Render("✓"), prefix, a.Type, a.Target)
			} else {
				fmt.Printf("  %s %s%s: %s — %s\n", style.Error.Render("✗"), prefix, a.Type, a.Target, a.Error)
			}
		}
		fmt.Println()
	} else if !result.Reconciled && (len(result.OrphanWorkspaces) > 0 || len(result.OrphanBeads) > 0) {
		fmt.Printf("Run with %s to fix orphans (use %s to preview first).\n",
			style.Bold.Render("--reconcile"), style.Bold.Render("--reconcile --dry-run"))
	}
}

func formatWorkspaceState(state string) string {
	switch state {
	case "running", "Running":
		return style.Success.Render("running")
	case "stopped", "Stopped":
		return style.Dim.Render("stopped")
	default:
		return style.Warning.Render(state)
	}
}
