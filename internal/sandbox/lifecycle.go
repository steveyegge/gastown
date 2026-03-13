// Package sandbox defines the lifecycle interface for external sandbox backends
// (e.g., Daytona workspaces). Sandbox lifecycle hooks run at the SessionManager
// level — before tmux session creation and after session destruction — to manage
// external resources that the exec-wrapper command prefix depends on.
//
// The exec-wrapper (PR #2689) injects a command prefix into the tmux pane command
// (e.g., "daytona exec <workspace> --tty --"). The sandbox lifecycle hooks ensure
// the workspace exists and is running before the wrapper command is invoked, and
// clean up resources after the session ends.
//
// Implementations should be safe for concurrent use by SessionManager.
package sandbox

import (
	"context"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/proxy"
)

// Lifecycle is implemented by exec-wrapper plugins that manage external sandbox
// state (workspace creation, cert management, cleanup). SessionManager calls
// these hooks around session create/destroy.
type Lifecycle interface {
	// PreStart is called before the tmux session is created.
	// For Daytona: creates/starts workspace, issues cert, injects cert volume.
	// Returns inner env vars to inject after the wrapper's -- delimiter.
	PreStart(ctx context.Context, opts SandboxOpts) (innerEnv map[string]string, err error)

	// PostStop is called after the tmux session is killed.
	// For Daytona: revokes cert, optionally stops/deletes workspace.
	// PostStop is non-fatal — errors are logged but do not fail session teardown.
	// Reconciliation handles any cleanup that PostStop misses.
	PostStop(ctx context.Context, opts SandboxOpts) error

	// Reconcile is called periodically by patrol (not per-session) to discover
	// orphaned workspaces and beads, and clean them up. Each orphan gets an
	// independent deadline to prevent one slow operation from starving others.
	Reconcile(ctx context.Context, opts ReconcileOpts) error

	// WorkspaceName returns the deterministic workspace name for a rig/polecat pair.
	// The naming convention is: <installPrefix>-<rig>--<polecat>
	WorkspaceName(rig, polecat string) string
}

// SandboxOpts contains the parameters for PreStart and PostStop lifecycle hooks.
type SandboxOpts struct {
	// Rig is the name of the rig that owns the polecat session.
	Rig string

	// Polecat is the name of the polecat whose session is starting/stopping.
	Polecat string

	// InstallPrefix is the shortened installation identifier (gt-<installID>).
	InstallPrefix string

	// WorkspaceName is the pre-computed workspace identifier:
	// <installPrefix>-<rig>--<polecat>
	WorkspaceName string

	// RigSettings holds the rig's configuration, including exec-wrapper and
	// inner env settings.
	RigSettings *config.RigSettings

	// ProxyCA is the CA used for issuing mTLS client certificates.
	// May be nil if the sandbox backend does not use mTLS.
	ProxyCA *proxy.CA

	// Branch is the git branch to check out in the workspace.
	// Used during workspace creation to set the initial branch.
	Branch string

	// CertSerial is the certificate serial number (lowercase hex) issued during
	// PreStart. PostStop uses this to revoke the correct certificate.
	// Set by SessionManager after reading from tmux environment.
	CertSerial string
}

// ReconcileOpts contains the parameters for periodic reconciliation.
type ReconcileOpts struct {
	// Rig is the name of the rig to reconcile workspaces for.
	Rig string

	// InstallPrefix is the shortened installation identifier (gt-<installID>).
	InstallPrefix string

	// RigSettings holds the rig's configuration.
	RigSettings *config.RigSettings

	// BeadsClient provides access to bead state for cross-referencing
	// workspaces against known polecat assignments.
	BeadsClient *beads.Beads
}
