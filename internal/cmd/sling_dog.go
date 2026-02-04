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
//   - "dog:" -> ("", true) - dispatch to any idle dog (shorthand)
//   - "dog:alpha" -> ("alpha", true) - dispatch to specific dog (shorthand)
func IsDogTarget(target string) (dogName string, isDog bool) {
	target = strings.ToLower(target)

	// Check for exact "deacon/dogs" (pool dispatch)
	if target == "deacon/dogs" || target == "dog:" {
		return "", true
	}

	// Check for "dog:<name>" shorthand (like rig:polecat syntax)
	if strings.HasPrefix(target, "dog:") {
		name := strings.TrimPrefix(target, "dog:")
		if name != "" && !strings.Contains(name, "/") {
			return name, true
		}
		return "", true // "dog:" without name = pool dispatch
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

// DogDispatchOptions contains options for dispatching work to a dog.
type DogDispatchOptions struct {
	Create            bool   // Create dog if it doesn't exist
	WorkDesc          string // Work description (formula or bead ID)
	DelaySessionStart bool   // If true, don't start session (caller will start later)
}

// DogDispatchInfo contains information about a dog dispatch.
type DogDispatchInfo struct {
	DogName string // Name of the dog
	AgentID string // Agent ID format (deacon/dogs/<name>)
	Pane    string // Tmux pane (empty if session start was delayed)
	Spawned bool   // True if dog was spawned (new)

	// Internal fields for delayed session start
	sessionDelayed bool
	townRoot       string
	workDesc       string
}

// DispatchToDog finds or spawns a dog for work dispatch.
// If dogName is empty, finds an idle dog from the pool.
// If opts.Create is true and no dogs exist, creates one.
// opts.WorkDesc is recorded in the dog's state so we know what it's working on.
// If opts.DelaySessionStart is true, the session is not started (caller must call StartDelayedSession).
func DispatchToDog(dogName string, opts DogDispatchOptions) (*DogDispatchInfo, error) {
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
			if opts.Create {
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
			if opts.Create {
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

	// Mark dog as working with the assigned work
	if err := mgr.AssignWork(targetDog.Name, opts.WorkDesc); err != nil {
		return nil, fmt.Errorf("assigning work to dog: %w", err)
	}

	// Build agent ID
	agentID := fmt.Sprintf("deacon/dogs/%s", targetDog.Name)

	// If delayed start, return info for later session start
	if opts.DelaySessionStart {
		fmt.Printf("Dog %s assigned (session start delayed)\n", targetDog.Name)
		return &DogDispatchInfo{
			DogName:        targetDog.Name,
			AgentID:        agentID,
			Pane:           "", // No pane yet
			Spawned:        spawned,
			sessionDelayed: true,
			townRoot:       townRoot,
			workDesc:       opts.WorkDesc,
		}, nil
	}

	// Ensure dog session is running (start if needed)
	t := tmux.NewTmux()
	sessMgr := dog.NewSessionManager(t, townRoot)

	sessOpts := dog.SessionStartOptions{
		WorkDesc: opts.WorkDesc,
	}
	pane, err := sessMgr.EnsureRunning(targetDog.Name, sessOpts)
	if err != nil {
		// Log but don't fail - dog state is set, session may start later
		fmt.Printf("Warning: could not start dog session: %v\n", err)
		pane = ""
	}

	return &DogDispatchInfo{
		DogName: targetDog.Name,
		AgentID: agentID,
		Pane:    pane,
		Spawned: spawned,
	}, nil
}

// StartDelayedSession starts the dog session after bead setup is complete.
// This should only be called when DelaySessionStart was true during dispatch.
func (d *DogDispatchInfo) StartDelayedSession() (string, error) {
	if !d.sessionDelayed {
		return d.Pane, nil // Session was already started
	}

	t := tmux.NewTmux()
	sessMgr := dog.NewSessionManager(t, d.townRoot)

	opts := dog.SessionStartOptions{
		WorkDesc: d.workDesc,
	}
	pane, err := sessMgr.EnsureRunning(d.DogName, opts)
	if err != nil {
		return "", fmt.Errorf("starting dog session: %w", err)
	}

	d.Pane = pane
	d.sessionDelayed = false
	return pane, nil
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
