package feed

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"time"

	"github.com/steveyegge/gastown/internal/constants"
)

// PolecatInfo represents a polecat's status from gt polecat list --json
type PolecatInfo struct {
	Rig            string `json:"rig"`
	Name           string `json:"name"`
	State          string `json:"state"`
	Issue          string `json:"issue,omitempty"`
	SessionRunning bool   `json:"session_running"`
	Zombie         bool   `json:"zombie,omitempty"`
	SessionName    string `json:"session_name,omitempty"`
}

// PolecatState holds all polecat data for the tree panel
type PolecatState struct {
	Polecats   []PolecatInfo
	LastUpdate time.Time
}

// FetchPolecats retrieves polecat status from gt polecat list --all --json
func FetchPolecats(townRoot string) (*PolecatState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), constants.BdSubprocessTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gt", "polecat", "list", "--all", "--json") //nolint:gosec // G204: args are constructed internally
	cmd.Dir = townRoot
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	var polecats []PolecatInfo
	if err := json.Unmarshal(stdout.Bytes(), &polecats); err != nil {
		return nil, err
	}

	return &PolecatState{
		Polecats:   polecats,
		LastUpdate: time.Now(),
	}, nil
}
