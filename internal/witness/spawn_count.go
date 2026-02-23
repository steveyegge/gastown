package witness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/steveyegge/gastown/internal/workspace"
)

// defaultMaxBeadRespawns is the threshold above which a SPAWN_STORM warning is
// included in the RECOVERED_BEAD mail sent to deacon. It does not block respawns
// — the intent is audit visibility so deacon/mayor can investigate.
const defaultMaxBeadRespawns = 2

// beadRespawnRecord tracks how many times a single bead has been reset for re-dispatch.
type beadRespawnRecord struct {
	BeadID      string    `json:"bead_id"`
	Count       int       `json:"count"`
	LastRespawn time.Time `json:"last_respawn"`
}

// beadRespawnState holds respawn counts for all tracked beads.
type beadRespawnState struct {
	Beads       map[string]*beadRespawnRecord `json:"beads"`
	LastUpdated time.Time                     `json:"last_updated"`
}

func beadRespawnStateFile(townRoot string) string {
	return filepath.Join(townRoot, "witness", "bead-respawn-counts.json")
}

func loadBeadRespawnState(townRoot string) *beadRespawnState {
	data, err := os.ReadFile(beadRespawnStateFile(townRoot)) //nolint:gosec // G304: path from trusted townRoot
	if err != nil {
		return &beadRespawnState{Beads: make(map[string]*beadRespawnRecord)}
	}
	var state beadRespawnState
	if err := json.Unmarshal(data, &state); err != nil {
		return &beadRespawnState{Beads: make(map[string]*beadRespawnRecord)}
	}
	if state.Beads == nil {
		state.Beads = make(map[string]*beadRespawnRecord)
	}
	return &state
}

func saveBeadRespawnState(townRoot string, state *beadRespawnState) error {
	stateFile := beadRespawnStateFile(townRoot)
	if err := os.MkdirAll(filepath.Dir(stateFile), 0755); err != nil {
		return fmt.Errorf("creating witness dir: %w", err)
	}
	state.LastUpdated = time.Now().UTC()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling respawn state: %w", err)
	}
	return os.WriteFile(stateFile, data, 0600)
}

// recordBeadRespawn increments the respawn count for beadID and returns the new count.
// workDir is the rig path; townRoot is resolved internally via workspace.Find.
// On state file errors the count is still incremented in memory and returned, so the
// caller can log/warn without blocking the respawn itself.
func recordBeadRespawn(workDir, beadID string) int {
	townRoot, err := workspace.Find(workDir)
	if err != nil || townRoot == "" {
		townRoot = workDir
	}
	state := loadBeadRespawnState(townRoot)
	rec, ok := state.Beads[beadID]
	if !ok {
		rec = &beadRespawnRecord{BeadID: beadID}
		state.Beads[beadID] = rec
	}
	rec.Count++
	rec.LastRespawn = time.Now().UTC()
	_ = saveBeadRespawnState(townRoot, state) // Non-fatal: tracking failure must not block respawn
	return rec.Count
}
