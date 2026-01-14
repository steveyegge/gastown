package ratelimit

import (
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"

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
	sessMgr, _, err := a.getSessionManager(rigName)
	if err != nil {
		return "", err
	}

	opts := polecat.SessionStartOptions{}

	// If profile specified, resolve it to a runtime config directory
	// Profile maps to an account handle in accounts.json
	if profile != "" {
		accountsPath := filepath.Join(a.townRoot, "mayor", "accounts.json")
		configDir, _, resolveErr := config.ResolveAccountConfigDir(accountsPath, profile)
		if resolveErr != nil {
			// Log warning but continue - fallback to default account
			log.Printf("[WARN] failed to resolve profile '%s' to config dir: %v", profile, resolveErr)
		} else if configDir != "" {
			opts.Account = profile
			opts.RuntimeConfigDir = configDir
		}
	}

	if err := sessMgr.Start(polecatName, opts); err != nil {
		return "", err
	}

	sessionID := sessMgr.SessionName(polecatName)
	return sessionID, nil
}

// beadEntry represents a bead in JSON output from bd list.
type beadEntry struct {
	ID string `json:"id"`
}

// formatAgentID constructs the agent ID for bd commands.
func formatAgentID(rigName, polecatName string) string {
	return fmt.Sprintf("%s/polecats/%s", rigName, polecatName)
}

// GetHookedWork returns the bead ID of work currently hooked to the polecat.
func (a *SessionAdapter) GetHookedWork(rigName, polecatName string) (string, error) {
	_, r, err := a.getSessionManager(rigName)
	if err != nil {
		return "", err
	}

	// Run bd list to get hooked work
	agentID := formatAgentID(rigName, polecatName)
	cmd := exec.Command("bd", "list", "--json", "--status=hooked", "--assignee="+agentID) //nolint:gosec
	cmd.Dir = r.Path
	output, err := cmd.Output()
	if err != nil {
		return "", nil // No hooked work is not an error
	}

	// Parse JSON output to extract bead ID
	var beads []beadEntry
	if err := json.Unmarshal(output, &beads); err != nil {
		// JSON parse error - log and return empty (non-fatal)
		return "", fmt.Errorf("parsing hooked work JSON: %w", err)
	}

	if len(beads) > 0 {
		return beads[0].ID, nil
	}

	return "", nil
}

// HookWork hooks a bead to the polecat.
func (a *SessionAdapter) HookWork(rigName, polecatName, beadID string) error {
	_, r, err := a.getSessionManager(rigName)
	if err != nil {
		return err
	}

	agentID := formatAgentID(rigName, polecatName)
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
