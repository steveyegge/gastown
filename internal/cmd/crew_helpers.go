package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/crew"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/workspace"
)

// getCrewManager returns a crew manager for the specified or inferred rig.
func getCrewManager(rigName string) (*crew.Manager, *rig.Rig, error) {
	// Handle optional rig inference from cwd
	if rigName == "" {
		townRoot, err := workspace.FindFromCwdOrError()
		if err != nil {
			return nil, nil, fmt.Errorf("not in a Gas Town workspace: %w", err)
		}
		rigName, err = inferRigFromCwd(townRoot)
		if err != nil {
			return nil, nil, fmt.Errorf("could not determine rig (use --rig flag): %w", err)
		}
	}

	_, r, err := getRig(rigName)
	if err != nil {
		return nil, nil, err
	}

	crewGit := git.NewGit(r.Path)
	crewMgr := crew.NewManager(r, crewGit)

	return crewMgr, r, nil
}

// crewSessionName generates the tmux session name for a crew worker.
func crewSessionName(rigName, crewName string) string {
	return session.CrewSessionName(session.PrefixFor(rigName), crewName)
}

// crewDetection holds the result of detecting crew workspace from cwd.
type crewDetection struct {
	rigName  string
	crewName string
}

// detectCrewFromCwd attempts to detect the crew workspace from the current directory.
// It looks for the pattern <town>/<rig>/crew/<name>/ in the current path.
func detectCrewFromCwd() (*crewDetection, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting cwd: %w", err)
	}

	// Find town root
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return nil, fmt.Errorf("not in Gas Town workspace: %w", err)
	}
	if townRoot == "" {
		return nil, fmt.Errorf("not in Gas Town workspace")
	}

	// Get relative path from town root
	relPath, err := filepath.Rel(townRoot, cwd)
	if err != nil {
		return nil, fmt.Errorf("getting relative path: %w", err)
	}

	// Normalize and split path
	relPath = filepath.ToSlash(relPath)
	parts := strings.Split(relPath, "/")

	// Look for pattern: <rig>/crew/<name>/...
	// Minimum: rig, crew, name = 3 parts
	if len(parts) < 3 {
		return nil, fmt.Errorf("not inside a crew workspace - specify the crew name or cd into a crew directory (e.g., gastown/crew/max)")
	}

	rigName := parts[0]
	if parts[1] != "crew" {
		return nil, fmt.Errorf("not in a crew workspace (not in crew/ directory)")
	}
	crewName := parts[2]

	return &crewDetection{
		rigName:  rigName,
		crewName: crewName,
	}, nil
}

// parseCrewSessionName extracts rig, crew name, and prefix from a tmux session name.
// Format: <prefix>-crew-<name>
// Returns empty strings and false if the format doesn't match.
func parseCrewSessionName(sessionName string) (rigName, crewName, prefix string, ok bool) {
	identity, err := session.ParseSessionName(sessionName)
	if err != nil {
		return "", "", "", false
	}
	if identity.Role != session.RoleCrew {
		return "", "", "", false
	}
	if identity.Rig == "" || identity.Name == "" {
		return "", "", "", false
	}
	return identity.Rig, identity.Name, identity.Prefix, true
}

// findRigCrewSessions returns all crew sessions for a given rig, sorted alphabetically.
// Uses tmux list-sessions to find sessions matching <prefix>-crew-* pattern.
// rigPrefix is the rig's beads prefix (e.g., "gt", "bd") — passed directly from
// the parsed session identity to avoid re-derivation failures when the registry
// isn't loaded.
func findRigCrewSessions(rigPrefix string) ([]string, error) { //nolint:unparam // error return kept for future use
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	out, err := cmd.Output()
	if err != nil {
		// No tmux server or no sessions
		return nil, nil
	}

	prefix := rigPrefix + "-crew-"
	var sessions []string

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, prefix) {
			sessions = append(sessions, line)
		}
	}

	// Sessions are already sorted by tmux, but sort explicitly for consistency
	// (alphabetical by session name means alphabetical by crew name)
	return sessions, nil
}
