package ratelimit

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

// SessionAdapter implements SessionOps using real gt infrastructure.
type SessionAdapter struct {
	townRoot string
}

// NewSessionAdapter creates a session adapter for the given town root.
func NewSessionAdapter(townRoot string) *SessionAdapter {
	return &SessionAdapter{townRoot: townRoot}
}

// NewSessionAdapterFromCwd creates a session adapter using the current workspace.
func NewSessionAdapterFromCwd() (*SessionAdapter, error) {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return nil, fmt.Errorf("not in a Gas Town workspace: %w", err)
	}
	return &SessionAdapter{townRoot: townRoot}, nil
}

func (a *SessionAdapter) getSessionManager(rigName string) (*polecat.SessionManager, *rig.Rig, error) {
	rigsConfigPath := filepath.Join(a.townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	g := git.NewGit(a.townRoot)
	rigMgr := rig.NewManager(a.townRoot, rigsConfig, g)
	r, err := rigMgr.GetRig(rigName)
	if err != nil {
		return nil, nil, fmt.Errorf("rig '%s' not found: %w", rigName, err)
	}

	t := tmux.NewTmux()
	sessMgr := polecat.NewSessionManager(t, r)
	return sessMgr, r, nil
}

// IsRunning checks if a polecat session is currently active.
func (a *SessionAdapter) IsRunning(rigName, polecatName string) (bool, error) {
	sessMgr, _, err := a.getSessionManager(rigName)
	if err != nil {
		return false, err
	}
	return sessMgr.IsRunning(polecatName)
}

// Stop terminates a polecat session.
func (a *SessionAdapter) Stop(rigName, polecatName string, force bool) error {
	sessMgr, _, err := a.getSessionManager(rigName)
	if err != nil {
		return err
	}
	return sessMgr.Stop(polecatName, force)
}

// Start creates and starts a new session for a polecat with the given profile.
func (a *SessionAdapter) Start(rigName, polecatName, profile string) (string, error) {
	sessMgr, r, err := a.getSessionManager(rigName)
	if err != nil {
		return "", err
	}

	// Build startup command with profile
	// Profile determines which API provider/account to use
	opts := polecat.SessionStartOptions{}

	// If profile specified, we need to configure the session to use it
	// The profile maps to runtime configuration that includes API keys
	if profile != "" {
		// Build command that uses the specified profile
		// This could be extended to pass profile to the runtime
		cmd := config.BuildPolecatStartupCommand(rigName, polecatName, r.Path, "")
		opts.Command = cmd
		// Note: Full profile support would require extending SessionStartOptions
		// to accept profile name and configure the runtime accordingly
	}

	if err := sessMgr.Start(polecatName, opts); err != nil {
		return "", err
	}

	sessionID := sessMgr.SessionName(polecatName)
	return sessionID, nil
}

// GetHookedWork returns the bead ID of work currently hooked to the polecat.
func (a *SessionAdapter) GetHookedWork(rigName, polecatName string) (string, error) {
	_, r, err := a.getSessionManager(rigName)
	if err != nil {
		return "", err
	}

	// Run bd hook show to get hooked work
	agentID := fmt.Sprintf("%s/polecats/%s", rigName, polecatName)
	cmd := exec.Command("bd", "list", "--json", "--status=hooked", "--assignee="+agentID) //nolint:gosec
	cmd.Dir = r.Path
	output, err := cmd.Output()
	if err != nil {
		return "", nil // No hooked work is not an error
	}

	// Parse first hooked bead ID from output
	// Simple approach: look for "id" field in JSON
	outputStr := string(output)
	if strings.Contains(outputStr, `"id"`) {
		// Extract ID - basic parsing
		start := strings.Index(outputStr, `"id":"`)
		if start >= 0 {
			start += 6
			end := strings.Index(outputStr[start:], `"`)
			if end >= 0 {
				return outputStr[start : start+end], nil
			}
		}
	}

	return "", nil
}

// HookWork hooks a bead to the polecat.
func (a *SessionAdapter) HookWork(rigName, polecatName, beadID string) error {
	_, r, err := a.getSessionManager(rigName)
	if err != nil {
		return err
	}

	agentID := fmt.Sprintf("%s/polecats/%s", rigName, polecatName)
	cmd := exec.Command("bd", "update", beadID, "--status=hooked", "--assignee="+agentID) //nolint:gosec
	cmd.Dir = r.Path
	return cmd.Run()
}

// Nudge sends a message to the polecat session.
func (a *SessionAdapter) Nudge(rigName, polecatName, message string) error {
	sessMgr, _, err := a.getSessionManager(rigName)
	if err != nil {
		return err
	}

	return sessMgr.Inject(polecatName, message)
}

// Ensure SessionAdapter implements SessionOps.
var _ SessionOps = (*SessionAdapter)(nil)
