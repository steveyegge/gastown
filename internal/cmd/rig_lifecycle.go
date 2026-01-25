package cmd

import (
	"fmt"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/refinery"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/witness"
)

// StopRigOptions defines which agents to stop for a rig.
type StopRigOptions struct {
	StopPolecats bool
	StopWitness  bool
	StopRefinery bool
	StopBdDaemon bool
	Force        bool
}

// StopRigAgents stops the specified rig agents and returns any errors encountered.
func StopRigAgents(r *rig.Rig, opts StopRigOptions) []error {
	var errs []error

	if opts.StopPolecats {
		t := tmux.NewTmux()
		polecatMgr := polecat.NewSessionManager(t, r)
		infos, err := polecatMgr.List()
		if err == nil && len(infos) > 0 {
			if err := polecatMgr.StopAll(opts.Force); err != nil {
				errs = append(errs, fmt.Errorf("polecat sessions: %w", err))
			}
		}
	}

	if opts.StopRefinery {
		refMgr := refinery.NewManager(r)
		if running, _ := refMgr.IsRunning(); running {
			if err := refMgr.Stop(); err != nil {
				errs = append(errs, fmt.Errorf("refinery: %w", err))
			}
		}
	}

	if opts.StopWitness {
		witMgr := witness.NewManager(r)
		if running, _ := witMgr.IsRunning(); running {
			if err := witMgr.Stop(); err != nil {
				errs = append(errs, fmt.Errorf("witness: %w", err))
			}
		}
	}

	if opts.StopBdDaemon {
		if err := StopRigBdDaemon(r); err != nil {
			errs = append(errs, err)
		}
	}

	return errs
}

// StopRigBdDaemon stops the bd daemon for a rig workspace.
func StopRigBdDaemon(r *rig.Rig) error {
	if r == nil {
		return nil
	}

	if err := beads.StopBdDaemonForWorkspace(r.BeadsPath()); err != nil {
		return fmt.Errorf("bd daemon: %w", err)
	}

	return nil
}
