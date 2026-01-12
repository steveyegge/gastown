package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/dog"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

// IsDogTarget checks if target is a dog target pattern.
// Returns the dog name (or empty for pool dispatch) and true if it's a dog target.
// Patterns:
//   - "deacon/dogs" -> ("", true) - dispatch to any idle dog
//   - "deacon/dogs/alpha" -> ("alpha", true) - dispatch to specific dog
func IsDogTarget(target string) (dogName string, isDog bool) {
	target = strings.ToLower(target)

	// Check for exact "deacon/dogs" (pool dispatch)
	if target == "deacon/dogs" {
		return "", true
	}

	// Check for "deacon/dogs/<name>" (specific dog)
	if strings.HasPrefix(target, "deacon/dogs/") {
		name := strings.TrimPrefix(target, "deacon/dogs/")
		if name != "" && !strings.Contains(name, "/") {
			return name, true
		}
	}

	return "", false
}

// DogDispatchInfo contains information about a dog dispatch.
type DogDispatchInfo struct {
	DogName string // Name of the dog
	AgentID string // Agent ID format (deacon/dogs/<name>)
	Pane    string // Tmux pane (empty if no session)
	Spawned bool   // True if dog was spawned (new)
}

// DispatchToDog finds or spawns a dog for work dispatch.
// If dogName is empty, finds an idle dog from the pool.
// If create is true and no dogs exist, creates one.
func DispatchToDog(dogName string, create bool) (*DogDispatchInfo, error) {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return nil, fmt.Errorf("finding town root: %w", err)
	}

	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		return nil, fmt.Errorf("loading rigs config: %w", err)
	}

	mgr := dog.NewManager(townRoot, rigsConfig)

	var targetDog *dog.Dog
	var spawned bool

	if dogName != "" {
		// Specific dog requested
		targetDog, err = mgr.Get(dogName)
		if err != nil {
			if create {
				// Create the dog if it doesn't exist
				targetDog, err = mgr.Add(dogName)
				if err != nil {
					return nil, fmt.Errorf("creating dog %s: %w", dogName, err)
				}
				fmt.Printf("✓ Created dog %s\n", dogName)
				spawned = true
			} else {
				return nil, fmt.Errorf("dog %s not found (use --create to add)", dogName)
			}
		}
	} else {
		// Pool dispatch - find an idle dog
		targetDog, err = mgr.GetIdleDog()
		if err != nil {
			return nil, fmt.Errorf("finding idle dog: %w", err)
		}

		if targetDog == nil {
			if create {
				// No idle dogs - create one
				newName := generateDogName(mgr)
				targetDog, err = mgr.Add(newName)
				if err != nil {
					return nil, fmt.Errorf("creating dog %s: %w", newName, err)
				}
				fmt.Printf("✓ Created dog %s (pool was empty)\n", newName)
				spawned = true
			} else {
				return nil, fmt.Errorf("no idle dogs available (use --create to add)")
			}
		}
	}

	// Mark dog as working
	if err := mgr.SetState(targetDog.Name, dog.StateWorking); err != nil {
		return nil, fmt.Errorf("setting dog state: %w", err)
	}

	// Build agent ID
	agentID := fmt.Sprintf("deacon/dogs/%s", targetDog.Name)

	// Try to find tmux session for the dog (dogs may run in tmux like polecats)
	// Dogs use the pattern gt-{town}-deacon-{name}
	townName, _ := workspace.GetTownName(townRoot)
	sessionName := fmt.Sprintf("gt-%s-deacon-%s", townName, targetDog.Name)
	t := tmux.NewTmux()
	var pane string
	if has, _ := t.HasSession(sessionName); has {
		// Get the pane from the session
		pane, _ = getSessionPane(sessionName)
	}

	return &DogDispatchInfo{
		DogName: targetDog.Name,
		AgentID: agentID,
		Pane:    pane,
		Spawned: spawned,
	}, nil
}

// generateDogName creates a unique dog name for pool expansion.
func generateDogName(mgr *dog.Manager) string {
	// Use Greek alphabet for dog names
	names := []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel"}

	dogs, _ := mgr.List()
	existing := make(map[string]bool)
	for _, d := range dogs {
		existing[d.Name] = true
	}

	for _, name := range names {
		if !existing[name] {
			return name
		}
	}

	// Fallback: numbered dogs
	for i := 1; i <= 100; i++ {
		name := fmt.Sprintf("dog%d", i)
		if !existing[name] {
			return name
		}
	}

	return fmt.Sprintf("dog%d", len(dogs)+1)
}
