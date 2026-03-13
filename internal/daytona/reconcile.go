package daytona

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"
)

// ReconcileAction describes what should be done for a discovered workspace or bead.
type ReconcileAction string

const (
	// ActionHealthy means the workspace and bead are matched and consistent.
	ActionHealthy ReconcileAction = "healthy"

	// ActionOrphanedWorkspace means a daytona workspace exists with no matching agent bead.
	ActionOrphanedWorkspace ReconcileAction = "orphaned_workspace"

	// ActionOrphanedBead means an agent bead references a daytona workspace that doesn't exist.
	ActionOrphanedBead ReconcileAction = "orphaned_bead"
)

// DiscoveryResult represents the outcome of workspace discovery for one item.
type DiscoveryResult struct {
	Action  ReconcileAction
	Rig     string // rig name
	Polecat string // polecat name

	// Workspace is non-nil for healthy matches and orphaned workspaces.
	Workspace *Workspace

	// Info holds richer workspace state from `daytona info -f json`, if available.
	// Nil when info fetch failed or was not attempted.
	Info *WorkspaceInfo

	// BeadID is set for healthy matches and orphaned beads.
	BeadID string

	// CertSerial is the proxy mTLS cert serial for revocation (set for orphaned beads).
	CertSerial string
}

// ReconcileReport summarizes the full reconciliation for a rig.
type ReconcileReport struct {
	Rig     string
	Results []DiscoveryResult

	// Counts for quick summary.
	Healthy            int
	OrphanedWorkspaces int
	OrphanedBeads      int
	SpawningSkipped    int // beads skipped because agent_state is "spawning"
}

// AgentBead represents the minimal polecat bead info needed for reconciliation.
// The caller provides these by querying beads (bd list --label=gt:agent).
type AgentBead struct {
	ID                 string // agent bead ID (e.g., "gtd-GasTownDaytona-polecat-garnet")
	Polecat            string // polecat name
	DaytonaWorkspaceName string // workspace name from bead description field
	AgentState         string // agent_state field (spawning, working, idle, etc.)
	CertSerial         string // proxy mTLS cert serial (lowercase hex) for revocation on cleanup
}

// DiscoverWorkspaces cross-references daytona workspaces with agent beads for a single rig.
// It returns a report categorizing each item as healthy, orphaned workspace, or orphaned bead.
//
// Parameters:
//   - rigName: the rig to reconcile
//   - workspaces: all workspaces from ListOwned, pre-filtered to this rig
//   - beads: all polecat agent beads for this rig that have a DaytonaWorkspaceName
func DiscoverWorkspaces(rigName string, workspaces []Workspace, beads []AgentBead) *ReconcileReport {
	report := &ReconcileReport{Rig: rigName}

	// Index workspaces by polecat name (workspace names are deterministic).
	wsByPolecat := make(map[string]*Workspace, len(workspaces))
	for i := range workspaces {
		ws := &workspaces[i]
		if ws.Rig == rigName {
			wsByPolecat[ws.Polecat] = ws
		}
	}

	// Index beads by polecat name.
	beadsByPolecat := make(map[string]*AgentBead, len(beads))
	for i := range beads {
		b := &beads[i]
		beadsByPolecat[b.Polecat] = b
	}

	// Check workspaces against beads.
	for polecatName, ws := range wsByPolecat {
		if bead, ok := beadsByPolecat[polecatName]; ok {
			// Healthy match: workspace exists and bead references it.
			report.Results = append(report.Results, DiscoveryResult{
				Action:    ActionHealthy,
				Rig:       rigName,
				Polecat:   polecatName,
				Workspace: ws,
				BeadID:    bead.ID,
			})
			report.Healthy++
		} else {
			// Orphaned workspace: workspace exists but no bead references it.
			report.Results = append(report.Results, DiscoveryResult{
				Action:    ActionOrphanedWorkspace,
				Rig:       rigName,
				Polecat:   polecatName,
				Workspace: ws,
			})
			report.OrphanedWorkspaces++
		}
	}

	// Check beads for workspaces that don't exist.
	for polecatName, bead := range beadsByPolecat {
		if _, ok := wsByPolecat[polecatName]; !ok {
			// Skip beads in "spawning" state — the workspace is being created
			// concurrently and may not have appeared in ListOwned yet.
			// This prevents a race where reconcile runs between agent bead
			// creation (step 3) and workspace provisioning (step 4) in
			// addDaytona, which would misclassify the bead as orphaned
			// and clear its daytona_workspace reference.
			if bead.AgentState == "spawning" {
				report.SpawningSkipped++
				continue
			}

			// Orphaned bead: bead references a workspace that doesn't exist.
			report.Results = append(report.Results, DiscoveryResult{
				Action:     ActionOrphanedBead,
				Rig:        rigName,
				Polecat:    polecatName,
				BeadID:     bead.ID,
				CertSerial: bead.CertSerial,
			})
			report.OrphanedBeads++
		}
	}

	return report
}

// DiscoverWorkspacesWithInfo works like DiscoverWorkspaces but also calls
// client.Info for each discovered workspace, enriching the report with
// last_activity, resource usage, and network config. Info failures are
// non-fatal — the result's Info field is left nil and a warning is logged.
func DiscoverWorkspacesWithInfo(ctx context.Context, client *Client, rigName string, workspaces []Workspace, beads []AgentBead, logger *log.Logger) *ReconcileReport {
	report := DiscoverWorkspaces(rigName, workspaces, beads)

	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}

	for i := range report.Results {
		r := &report.Results[i]
		if r.Workspace == nil {
			continue
		}
		info, err := client.Info(ctx, r.Workspace.Name)
		if err != nil {
			logger.Printf("Warning: daytona info for %s failed (non-fatal): %v", r.Workspace.Name, err)
			continue
		}
		r.Info = info
	}
	return report
}

// ReconcileOptions controls reconciliation behavior.
type ReconcileOptions struct {
	// DryRun logs actions without performing them.
	DryRun bool

	// AutoDelete removes orphaned workspaces. If false, they're only stopped.
	AutoDelete bool

	// PerOperationTimeout is the timeout for each individual stop/delete/revoke
	// operation. Each orphan gets its own timeout budget so late operations are
	// not starved by earlier ones. Zero means 30s default.
	PerOperationTimeout time.Duration

	// ZombieThreshold is the duration of inactivity after which a "running"
	// workspace is considered a zombie (agent crashed but workspace lingered).
	// When non-zero and Info data is available, Reconcile logs zombie warnings
	// for healthy workspaces that exceed this threshold.
	// Zero disables zombie detection.
	ZombieThreshold time.Duration
}

// ReconcileResult holds outcomes of reconciliation actions taken.
type ReconcileResult struct {
	WorkspacesStopped  int
	WorkspacesArchived int
	WorkspacesDeleted  int
	WorkspacesSkipped  int // transitional states skipped
	BeadsReset         int
	CertsRevoked       int
	ZombiesDetected    int
	Errors             []error
}

// Reconcile acts on a discovery report: stops/deletes orphaned workspaces and
// resets orphaned beads. The beadResetter callback handles bead state reset
// since the daytona package doesn't import beads directly. The certRevoker
// callback (optional) revokes mTLS certs before bead reset to prevent
// orphaned certs from authenticating to the proxy.
func Reconcile(ctx context.Context, client *Client, report *ReconcileReport, opts ReconcileOptions, beadResetter func(beadID string) error, certRevoker func(ctx context.Context, serial string) error, logger *log.Logger) *ReconcileResult {
	result := &ReconcileResult{}

	// Guard nil logger: all orphan paths log unconditionally, so replace nil
	// with a discard logger to avoid nil pointer dereference panics.
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}

	opTimeout := opts.PerOperationTimeout
	if opTimeout == 0 {
		opTimeout = 30 * time.Second
	}

	for _, item := range report.Results {
		switch item.Action {
		case ActionOrphanedWorkspace:
			if opts.DryRun {
				logger.Printf("[dry-run] would stop orphaned workspace %s (rig=%s, polecat=%s, state=%s)",
					item.Workspace.Name, item.Rig, item.Polecat, item.Workspace.State)
				if opts.AutoDelete {
					logger.Printf("[dry-run] would delete orphaned workspace %s", item.Workspace.Name)
				}
				continue
			}

			// Handle workspace based on its current state.
			switch item.Workspace.State {
			case "creating", "starting", "stopping":
				// Transitional states — skip, will resolve on their own or
				// be caught in the next reconciliation cycle.
				logger.Printf("Skipping orphaned workspace %s in transitional state %q (rig=%s, polecat=%s)",
					item.Workspace.Name, item.Workspace.State, item.Rig, item.Polecat)
				result.WorkspacesSkipped++
				continue

			case "error":
				// Error state — attempt to stop but log distinctly so operators
				// can investigate. Proceed to delete if configured.
				logger.Printf("Orphaned workspace %s is in error state (rig=%s, polecat=%s), attempting stop",
					item.Workspace.Name, item.Rig, item.Polecat)
				opCtx, opCancel := context.WithTimeout(ctx, opTimeout)
				if err := client.Stop(opCtx, item.Workspace.Name); err != nil {
					logger.Printf("Warning: failed to stop errored orphaned workspace %s: %v", item.Workspace.Name, err)
					result.Errors = append(result.Errors, fmt.Errorf("stop errored %s: %w", item.Workspace.Name, err))
				} else {
					result.WorkspacesStopped++
				}
				opCancel()

			case "stopped":
				// Already stopped — archive to reduce storage cost.
				opCtx, opCancel := context.WithTimeout(ctx, opTimeout)
				if err := client.Archive(opCtx, item.Workspace.Name); err != nil {
					logger.Printf("Warning: failed to archive stopped orphaned workspace %s: %v", item.Workspace.Name, err)
					result.Errors = append(result.Errors, fmt.Errorf("archive %s: %w", item.Workspace.Name, err))
				} else {
					logger.Printf("Archived stopped orphaned workspace %s (rig=%s, polecat=%s)",
						item.Workspace.Name, item.Rig, item.Polecat)
					result.WorkspacesArchived++
				}
				opCancel()

			default:
				// "running" or any other active state — stop it, then archive.
				opCtx, opCancel := context.WithTimeout(ctx, opTimeout)
				if err := client.Stop(opCtx, item.Workspace.Name); err != nil {
					logger.Printf("Warning: failed to stop orphaned workspace %s: %v", item.Workspace.Name, err)
					result.Errors = append(result.Errors, fmt.Errorf("stop %s: %w", item.Workspace.Name, err))
				} else {
					logger.Printf("Stopped orphaned workspace %s (rig=%s, polecat=%s)",
						item.Workspace.Name, item.Rig, item.Polecat)
					result.WorkspacesStopped++
					// Archive after successful stop to move to cheaper storage.
					if err := client.Archive(opCtx, item.Workspace.Name); err != nil {
						logger.Printf("Warning: failed to archive orphaned workspace %s: %v", item.Workspace.Name, err)
						result.Errors = append(result.Errors, fmt.Errorf("archive %s: %w", item.Workspace.Name, err))
					} else {
						result.WorkspacesArchived++
					}
				}
				opCancel()
			}

			// Delete if configured.
			if opts.AutoDelete {
				opCtx, opCancel := context.WithTimeout(ctx, opTimeout)
				if err := client.Delete(opCtx, item.Workspace.Name); err != nil {
					logger.Printf("Warning: failed to delete orphaned workspace %s: %v", item.Workspace.Name, err)
					result.Errors = append(result.Errors, fmt.Errorf("delete %s: %w", item.Workspace.Name, err))
				} else {
					logger.Printf("Deleted orphaned workspace %s", item.Workspace.Name)
					result.WorkspacesDeleted++
				}
				opCancel()
			}

		case ActionOrphanedBead:
			if beadResetter == nil {
				continue
			}
			if opts.DryRun {
				logger.Printf("[dry-run] would reset orphaned bead %s (rig=%s, polecat=%s)",
					item.BeadID, item.Rig, item.Polecat)
				continue
			}

			// Revoke cert BEFORE resetting the bead (reset clears the serial).
			if certRevoker != nil && item.CertSerial != "" {
				opCtx, opCancel := context.WithTimeout(ctx, opTimeout)
				if err := certRevoker(opCtx, item.CertSerial); err != nil {
					logger.Printf("Warning: failed to revoke cert for orphaned bead %s (serial %s): %v",
						item.BeadID, item.CertSerial, err)
					result.Errors = append(result.Errors, fmt.Errorf("revoke cert %s: %w", item.CertSerial, err))
				} else {
					result.CertsRevoked++
				}
				opCancel()
			}

			if err := beadResetter(item.BeadID); err != nil {
				logger.Printf("Warning: failed to reset orphaned bead %s: %v", item.BeadID, err)
				result.Errors = append(result.Errors, fmt.Errorf("reset bead %s: %w", item.BeadID, err))
			} else {
				logger.Printf("Reset orphaned bead %s (rig=%s, polecat=%s)",
					item.BeadID, item.Rig, item.Polecat)
				result.BeadsReset++
			}

		case ActionHealthy:
			logger.Printf("Healthy match: workspace %s ↔ bead %s (rig=%s, polecat=%s)",
				item.Workspace.Name, item.BeadID, item.Rig, item.Polecat)

			// Zombie detection: flag healthy workspaces that have been inactive
			// beyond the configured threshold. This catches cases where the agent
			// crashed but the workspace remains running.
			if opts.ZombieThreshold > 0 && item.Info != nil && item.Workspace.State == "running" {
				if lastActivity, err := time.Parse(time.RFC3339, item.Info.LastActivity); err == nil {
					idle := time.Since(lastActivity)
					if idle > opts.ZombieThreshold {
						logger.Printf("ZOMBIE: workspace %s idle for %v (threshold %v) — agent may have crashed (rig=%s, polecat=%s)",
							item.Workspace.Name, idle.Truncate(time.Second), opts.ZombieThreshold, item.Rig, item.Polecat)
						result.ZombiesDetected++
					}
				}
			}
		}
	}

	return result
}
