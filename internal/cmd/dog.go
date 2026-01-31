package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/dog"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/plugin"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

// Dog command flags
var (
	dogListJSON   bool
	dogStatusJSON bool
	dogForce      bool
	dogRemoveAll  bool
	dogCallAll    bool
	dogClearAll   bool
	dogClearForce bool

	// Dispatch flags
	dogDispatchPlugin string
	dogDispatchRig    string
	dogDispatchCreate bool
	dogDispatchDog    string
	dogDispatchJSON   bool
	dogDispatchDryRun bool
)

var dogCmd = &cobra.Command{
	Use:     "dog",
	Aliases: []string{"dogs"},
	GroupID: GroupAgents,
	Short:   "Manage dogs (cross-rig infrastructure workers)",
	Long: `Manage dogs - reusable workers for infrastructure and cleanup.

CATS VS DOGS:
  Polecats (cats) build features. One rig. Ephemeral (one task, then nuked).
  Dogs clean up messes. Cross-rig. Reusable (multiple tasks, eventually recycled).

Dogs are managed by the Deacon for town-level work:
  - Infrastructure tasks (rebuilding, syncing, migrations)
  - Cleanup operations (orphan branches, stale files)
  - Cross-rig work that spans multiple projects

Each dog has worktrees into every configured rig, enabling cross-project
operations. Dogs return to idle state after completing work (unlike cats).

The kennel is at ~/gt/deacon/dogs/. The Deacon dispatches work to dogs.`,
}

var dogAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Create a new dog in the kennel",
	Long: `Create a new dog in the kennel with multi-rig worktrees.

Each dog gets a worktree per configured rig (e.g., gastown, beads).
The dog starts in idle state, ready to receive work from the Deacon.

Example:
  gt dog add alpha
  gt dog add bravo`,
	Args: cobra.ExactArgs(1),
	RunE: runDogAdd,
}

var dogRemoveCmd = &cobra.Command{
	Use:   "remove <name>... | --all",
	Short: "Remove dogs from the kennel",
	Long: `Remove one or more dogs from the kennel.

Removes all worktrees and the dog directory.
Use --force to remove even if dog is in working state.

Examples:
  gt dog remove alpha
  gt dog remove alpha bravo
  gt dog remove --all
  gt dog remove alpha --force`,
	Args: func(cmd *cobra.Command, args []string) error {
		if dogRemoveAll {
			return nil
		}
		if len(args) < 1 {
			return fmt.Errorf("requires at least 1 dog name (or use --all)")
		}
		return nil
	},
	RunE: runDogRemove,
}

var dogListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all dogs in the kennel",
	Long: `List all dogs in the kennel with their status.

Shows each dog's state (idle/working), current work assignment,
and last active timestamp.

Examples:
  gt dog list
  gt dog list --json`,
	RunE: runDogList,
}

var dogCallCmd = &cobra.Command{
	Use:   "call [name]",
	Short: "Wake idle dog(s) for work",
	Long: `Wake an idle dog to prepare for work.

With a name, wakes the specific dog.
With --all, wakes all idle dogs.
Without arguments, wakes one idle dog (if available).

This updates the dog's last-active timestamp and can trigger
session creation for the dog's worktrees.

Examples:
  gt dog call alpha
  gt dog call --all
  gt dog call`,
	RunE: runDogCall,
}

var dogDoneCmd = &cobra.Command{
	Use:   "done [name]",
	Short: "Mark dog as done and return to idle",
	Long: `Mark a dog as done with its current work and return to idle state.

Dogs should call this when they complete their work assignment.
This clears the work field and sets state to idle, making the dog
available for new work.

Without a name argument, auto-detects the current dog from the working
directory (must be run from within a dog's worktree).

Examples:
  gt dog done         # Auto-detect from cwd
  gt dog done alpha   # Explicit name`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDogDone,
}

var dogStatusCmd = &cobra.Command{
	Use:   "status [name]",
	Short: "Show detailed dog status",
	Long: `Show detailed status for a specific dog or summary for all dogs.

With a name, shows detailed info including:
  - State (idle/working)
  - Current work assignment
  - Worktree paths per rig
  - Last active timestamp

Without a name, shows pack summary:
  - Total dogs
  - Idle/working counts
  - Pack health

Examples:
  gt dog status alpha
  gt dog status
  gt dog status --json`,
	RunE: runDogStatus,
}

var dogClearCmd = &cobra.Command{
	Use:   "clear <name>... | --all",
	Short: "Clear stuck dog(s) back to idle state",
	Long: `Clear one or more dogs back to idle state.

Resets the dog's state from "working" to "idle" and clears any work assignment.
Use this to recover dogs that are stuck in working state without actual work.

Use --force to clear even if the dog appears to be actively working
(has a recent last-active timestamp).

Examples:
  gt dog clear alpha
  gt dog clear alpha bravo charlie
  gt dog clear --all
  gt dog clear alpha --force`,
	Args: func(cmd *cobra.Command, args []string) error {
		if dogClearAll {
			return nil
		}
		if len(args) < 1 {
			return fmt.Errorf("requires at least 1 dog name (or use --all)")
		}
		return nil
	},
	RunE: runDogClear,
}

var dogDispatchCmd = &cobra.Command{
	Use:   "dispatch --plugin <name>",
	Short: "Dispatch plugin execution to a dog",
	Long: `Dispatch a plugin for execution by a dog worker.

This is the formalized command for sending plugin work to dogs. The Deacon
uses this during patrol cycles to dispatch plugins with open gates.

The command:
1. Finds the plugin definition (plugin.md)
2. Assigns work to an idle dog (marks as working)
3. Sends mail with plugin instructions to the dog
4. Returns immediately (non-blocking)

The dog discovers the work via its mail inbox and executes the plugin
instructions. On completion, the dog sends DOG_DONE mail to deacon/.

Examples:
  gt dog dispatch --plugin rebuild-gt
  gt dog dispatch --plugin rebuild-gt --rig gastown
  gt dog dispatch --plugin rebuild-gt --dog alpha
  gt dog dispatch --plugin rebuild-gt --create
  gt dog dispatch --plugin rebuild-gt --dry-run
  gt dog dispatch --plugin rebuild-gt --json`,
	RunE: runDogDispatch,
}

func init() {
	// List flags
	dogListCmd.Flags().BoolVar(&dogListJSON, "json", false, "Output as JSON")

	// Remove flags
	dogRemoveCmd.Flags().BoolVarP(&dogForce, "force", "f", false, "Force removal even if working")
	dogRemoveCmd.Flags().BoolVar(&dogRemoveAll, "all", false, "Remove all dogs")

	// Call flags
	dogCallCmd.Flags().BoolVar(&dogCallAll, "all", false, "Wake all idle dogs")

	// Status flags
	dogStatusCmd.Flags().BoolVar(&dogStatusJSON, "json", false, "Output as JSON")

	// Clear flags
	dogClearCmd.Flags().BoolVar(&dogClearAll, "all", false, "Clear all stuck dogs")
	dogClearCmd.Flags().BoolVarP(&dogClearForce, "force", "f", false, "Force clear even if recently active")

	// Dispatch flags
	dogDispatchCmd.Flags().StringVar(&dogDispatchPlugin, "plugin", "", "Plugin name to dispatch (required)")
	dogDispatchCmd.Flags().StringVar(&dogDispatchRig, "rig", "", "Limit plugin search to specific rig")
	dogDispatchCmd.Flags().StringVar(&dogDispatchDog, "dog", "", "Dispatch to specific dog (default: any idle)")
	dogDispatchCmd.Flags().BoolVar(&dogDispatchCreate, "create", false, "Create a dog if none idle")
	dogDispatchCmd.Flags().BoolVar(&dogDispatchJSON, "json", false, "Output as JSON")
	dogDispatchCmd.Flags().BoolVarP(&dogDispatchDryRun, "dry-run", "n", false, "Show what would be done without doing it")
	_ = dogDispatchCmd.MarkFlagRequired("plugin")

	// Add subcommands
	dogCmd.AddCommand(dogAddCmd)
	dogCmd.AddCommand(dogRemoveCmd)
	dogCmd.AddCommand(dogListCmd)
	dogCmd.AddCommand(dogCallCmd)
	dogCmd.AddCommand(dogDoneCmd)
	dogCmd.AddCommand(dogStatusCmd)
	dogCmd.AddCommand(dogClearCmd)
	dogCmd.AddCommand(dogDispatchCmd)

	rootCmd.AddCommand(dogCmd)
}

// getDogManager creates a dog.Manager with the current town root.
func getDogManager() (*dog.Manager, error) {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return nil, fmt.Errorf("finding town root: %w", err)
	}

	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		return nil, fmt.Errorf("loading rigs config: %w", err)
	}

	return dog.NewManager(townRoot, rigsConfig), nil
}

func runDogAdd(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Validate name
	if strings.ContainsAny(name, "/\\. ") {
		return fmt.Errorf("dog name cannot contain /, \\, ., or spaces")
	}

	mgr, err := getDogManager()
	if err != nil {
		return err
	}

	d, err := mgr.Add(name)
	if err != nil {
		return fmt.Errorf("adding dog %s: %w", name, err)
	}

	fmt.Printf("✓ Created dog %s in kennel\n", style.Bold.Render(name))
	fmt.Printf("  Path: %s\n", d.Path)
	fmt.Printf("  Worktrees:\n")
	for rigName, path := range d.Worktrees {
		fmt.Printf("    %s: %s\n", rigName, path)
	}

	// Create agent bead for the dog
	townRoot, _ := workspace.FindFromCwd()
	if townRoot != "" {
		b := beads.New(townRoot)
		location := filepath.Join("deacon", "dogs", name)

		issue, err := b.CreateDogAgentBead(name, location)
		if err != nil {
			// Non-fatal: warn but don't fail dog creation
			fmt.Printf("  Warning: could not create agent bead: %v\n", err)
		} else {
			fmt.Printf("  Agent bead: %s\n", issue.ID)
		}
	}

	return nil
}

func runDogRemove(cmd *cobra.Command, args []string) error {
	mgr, err := getDogManager()
	if err != nil {
		return err
	}

	var names []string
	if dogRemoveAll {
		dogs, err := mgr.List()
		if err != nil {
			return fmt.Errorf("listing dogs: %w", err)
		}
		for _, d := range dogs {
			names = append(names, d.Name)
		}
		if len(names) == 0 {
			fmt.Println("No dogs in kennel")
			return nil
		}
	} else {
		names = args
	}

	// Get beads client for cleanup
	townRoot, _ := workspace.FindFromCwd()
	var b *beads.Beads
	if townRoot != "" {
		b = beads.New(townRoot)
	}

	for _, name := range names {
		d, err := mgr.Get(name)
		if err != nil {
			fmt.Printf("Warning: dog %s not found, skipping\n", name)
			continue
		}

		// Check if working
		if d.State == dog.StateWorking && !dogForce {
			return fmt.Errorf("dog %s is working (use --force to remove anyway)", name)
		}

		if err := mgr.Remove(name); err != nil {
			return fmt.Errorf("removing dog %s: %w", name, err)
		}

		fmt.Printf("✓ Removed dog %s\n", name)

		// Delete agent bead for the dog
		if b != nil {
			if err := b.DeleteDogAgentBead(name); err != nil {
				// Non-fatal: warn but don't fail dog removal
				fmt.Printf("  Warning: could not delete agent bead: %v\n", err)
			}
		}
	}

	return nil
}

func runDogList(cmd *cobra.Command, args []string) error {
	mgr, err := getDogManager()
	if err != nil {
		return err
	}

	dogs, err := mgr.List()
	if err != nil {
		return fmt.Errorf("listing dogs: %w", err)
	}

	if len(dogs) == 0 {
		if dogListJSON {
			fmt.Println("[]")
		} else {
			fmt.Println("No dogs in kennel")
		}
		return nil
	}

	if dogListJSON {
		type DogListItem struct {
			Name       string            `json:"name"`
			State      dog.State         `json:"state"`
			Work       string            `json:"work,omitempty"`
			LastActive time.Time         `json:"last_active"`
			Worktrees  map[string]string `json:"worktrees,omitempty"`
		}

		var items []DogListItem
		for _, d := range dogs {
			items = append(items, DogListItem{
				Name:       d.Name,
				State:      d.State,
				Work:       d.Work,
				LastActive: d.LastActive,
				Worktrees:  d.Worktrees,
			})
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(items)
	}

	// Pretty print
	fmt.Println(style.Bold.Render("The Pack"))
	fmt.Println()

	idleCount := 0
	workingCount := 0

	for _, d := range dogs {
		stateIcon := "○"
		stateStyle := style.Dim
		if d.State == dog.StateWorking {
			stateIcon = "●"
			stateStyle = style.Bold
			workingCount++
		} else {
			idleCount++
		}

		line := fmt.Sprintf("  %s %s", stateIcon, stateStyle.Render(d.Name))
		if d.Work != "" {
			line += fmt.Sprintf(" → %s", style.Dim.Render(d.Work))
		}
		fmt.Println(line)
	}

	fmt.Println()
	fmt.Printf("  %d idle, %d working\n", idleCount, workingCount)

	return nil
}

func runDogCall(cmd *cobra.Command, args []string) error {
	mgr, err := getDogManager()
	if err != nil {
		return err
	}

	if dogCallAll {
		// Wake all idle dogs
		dogs, err := mgr.List()
		if err != nil {
			return fmt.Errorf("listing dogs: %w", err)
		}

		woken := 0
		for _, d := range dogs {
			if d.State == dog.StateIdle {
				if err := mgr.SetState(d.Name, dog.StateIdle); err != nil {
					fmt.Printf("Warning: failed to wake %s: %v\n", d.Name, err)
					continue
				}
				woken++
				fmt.Printf("✓ Called %s\n", d.Name)
			}
		}

		if woken == 0 {
			fmt.Println("No idle dogs to call")
		} else {
			fmt.Printf("\n%d dog(s) ready\n", woken)
		}
		return nil
	}

	if len(args) > 0 {
		// Wake specific dog
		name := args[0]
		d, err := mgr.Get(name)
		if err != nil {
			return fmt.Errorf("getting dog %s: %w", name, err)
		}

		if d.State == dog.StateWorking {
			fmt.Printf("Dog %s is already working (use 'gt dog done %s' when complete)\n", name, name)
			return nil
		}

		if err := mgr.SetState(name, dog.StateIdle); err != nil {
			return fmt.Errorf("waking dog %s: %w", name, err)
		}

		fmt.Printf("✓ Called %s - ready for work\n", name)
		return nil
	}

	// Wake one idle dog
	d, err := mgr.GetIdleDog()
	if err != nil {
		return fmt.Errorf("getting idle dog: %w", err)
	}

	if d == nil {
		fmt.Println("No idle dogs available")
		return nil
	}

	if err := mgr.SetState(d.Name, dog.StateIdle); err != nil {
		return fmt.Errorf("waking dog %s: %w", d.Name, err)
	}

	fmt.Printf("✓ Called %s - ready for work\n", d.Name)
	return nil
}

func runDogDone(cmd *cobra.Command, args []string) error {
	mgr, err := getDogManager()
	if err != nil {
		return err
	}

	var name string
	if len(args) > 0 {
		name = args[0]
	} else {
		// Auto-detect dog from cwd
		// Dog worktrees are at ~/gt/deacon/dogs/<name>/<rig>/
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting cwd: %w", err)
		}

		// Look for /deacon/dogs/<name>/ in path
		parts := strings.Split(cwd, string(filepath.Separator))
		for i := 0; i < len(parts)-1; i++ {
			if parts[i] == "dogs" && i > 0 && parts[i-1] == "deacon" {
				name = parts[i+1]
				break
			}
		}

		if name == "" {
			return fmt.Errorf("could not detect dog name from cwd: %s\nRun from a dog worktree or specify name: gt dog done <name>", cwd)
		}
	}

	d, err := mgr.Get(name)
	if err != nil {
		return fmt.Errorf("getting dog %s: %w", name, err)
	}

	if d.State == dog.StateIdle && d.Work == "" {
		fmt.Printf("Dog %s is already idle with no work\n", name)
		return nil
	}

	if err := mgr.ClearWork(name); err != nil {
		return fmt.Errorf("clearing work for dog %s: %w", name, err)
	}

	fmt.Printf("✓ Dog %s returned to kennel (idle)\n", name)
	return nil
}

func runDogStatus(cmd *cobra.Command, args []string) error {
	mgr, err := getDogManager()
	if err != nil {
		return err
	}

	if len(args) > 0 {
		// Show specific dog status
		name := args[0]
		return showDogStatus(mgr, name)
	}

	// Show pack summary
	return showPackStatus(mgr)
}

func showDogStatus(mgr *dog.Manager, name string) error {
	d, err := mgr.Get(name)
	if err != nil {
		return fmt.Errorf("getting dog %s: %w", name, err)
	}

	if dogStatusJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(d)
	}

	fmt.Printf("Dog: %s\n\n", style.Bold.Render(d.Name))
	fmt.Printf("  State:       %s\n", d.State)
	if d.Work != "" {
		fmt.Printf("  Work:        %s\n", d.Work)
	} else {
		fmt.Printf("  Work:        %s\n", style.Dim.Render("(none)"))
	}
	fmt.Printf("  Path:        %s\n", d.Path)
	fmt.Printf("  Last Active: %s\n", dogFormatTimeAgo(d.LastActive))
	fmt.Printf("  Created:     %s\n", d.CreatedAt.Format("2006-01-02 15:04"))

	if len(d.Worktrees) > 0 {
		fmt.Println("\nWorktrees:")
		for rigName, path := range d.Worktrees {
			// Check if worktree exists
			exists := "✓"
			if _, err := os.Stat(path); os.IsNotExist(err) {
				exists = "✗"
			}
			fmt.Printf("  %s %s: %s\n", exists, rigName, path)
		}
	}

	// Check for tmux session
	townRoot, _ := workspace.FindFromCwd()
	if townRoot != "" {
		townName, err := workspace.GetTownName(townRoot)
		if err == nil {
			sessionName := fmt.Sprintf("gt-%s-deacon-%s", townName, name)
			tm := tmux.NewTmux()
			if has, _ := tm.HasSession(sessionName); has {
				fmt.Printf("\nSession: %s (running)\n", sessionName)
			}
		}
	}

	return nil
}

func showPackStatus(mgr *dog.Manager) error {
	dogs, err := mgr.List()
	if err != nil {
		return fmt.Errorf("listing dogs: %w", err)
	}

	if dogStatusJSON {
		type PackStatus struct {
			Total     int    `json:"total"`
			Idle      int    `json:"idle"`
			Working   int    `json:"working"`
			KennelDir string `json:"kennel_dir"`
		}

		townRoot, _ := workspace.FindFromCwd()
		status := PackStatus{
			Total:     len(dogs),
			KennelDir: filepath.Join(townRoot, "deacon", "dogs"),
		}
		for _, d := range dogs {
			if d.State == dog.StateIdle {
				status.Idle++
			} else {
				status.Working++
			}
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}

	fmt.Println(style.Bold.Render("Pack Status"))
	fmt.Println()

	if len(dogs) == 0 {
		fmt.Println("  No dogs in kennel")
		fmt.Println()
		fmt.Println("  Use 'gt dog add <name>' to add a dog")
		return nil
	}

	idleCount := 0
	workingCount := 0
	for _, d := range dogs {
		if d.State == dog.StateIdle {
			idleCount++
		} else {
			workingCount++
		}
	}

	fmt.Printf("  Total:   %d\n", len(dogs))
	fmt.Printf("  Idle:    %d\n", idleCount)
	fmt.Printf("  Working: %d\n", workingCount)

	if idleCount > 0 {
		fmt.Println()
		fmt.Println(style.Dim.Render("  Ready for work. Use 'gt dog call' to wake."))
	}

	return nil
}

// dogFormatTimeAgo formats a time as a relative string like "2 hours ago".
func dogFormatTimeAgo(t time.Time) string {
	if t.IsZero() {
		return "(unknown)"
	}

	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}

// runDogDispatch dispatches plugin execution to a dog worker.
func runDogDispatch(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	// Get rig names for plugin scanner
	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		return fmt.Errorf("loading rigs config: %w", err)
	}

	var rigNames []string
	for rigName := range rigsConfig.Rigs {
		rigNames = append(rigNames, rigName)
	}

	// If --rig specified, search only that rig
	if dogDispatchRig != "" {
		rigNames = []string{dogDispatchRig}
	}

	// Find the plugin using scanner
	scanner := plugin.NewScanner(townRoot, rigNames)
	p, err := scanner.GetPlugin(dogDispatchPlugin)
	if err != nil {
		return fmt.Errorf("finding plugin: %w", err)
	}

	// Get dog manager (reuse rigsConfig from above)
	mgr := dog.NewManager(townRoot, rigsConfig)

	// Find target dog
	var targetDog *dog.Dog
	var dogCreated bool
	if dogDispatchDog != "" {
		// Specific dog requested
		targetDog, err = mgr.Get(dogDispatchDog)
		if err != nil {
			return fmt.Errorf("getting dog %s: %w", dogDispatchDog, err)
		}
		if targetDog.State == dog.StateWorking {
			return fmt.Errorf("dog %s is already working", dogDispatchDog)
		}
	} else {
		// Find idle dog from pool
		targetDog, err = mgr.GetIdleDog()
		if err != nil {
			return fmt.Errorf("finding idle dog: %w", err)
		}

		if targetDog == nil {
			if dogDispatchCreate {
				// Create a new dog (reuse generateDogName from sling_dog.go)
				newName := generateDogName(mgr)
				if dogDispatchDryRun {
					targetDog = &dog.Dog{Name: newName, State: dog.StateIdle}
					dogCreated = true
				} else {
					targetDog, err = mgr.Add(newName)
					if err != nil {
						return fmt.Errorf("creating dog %s: %w", newName, err)
					}
					dogCreated = true

					// Create agent bead for the dog
					b := beads.New(townRoot)
					location := filepath.Join("deacon", "dogs", newName)
					if _, beadErr := b.CreateDogAgentBead(newName, location); beadErr != nil {
						// Non-fatal warning
						if !dogDispatchJSON {
							fmt.Printf("  Warning: could not create agent bead: %v\n", beadErr)
						}
					}
				}
			} else {
				return fmt.Errorf("no idle dogs available (use --create to add one)")
			}
		}
	}

	// Prepare dispatch result for JSON output
	workDesc := fmt.Sprintf("plugin:%s", p.Name)
	result := dogDispatchResult{
		Plugin:     p.Name,
		PluginPath: p.Path,
		Dog:        targetDog.Name,
		DogCreated: dogCreated,
		Work:       workDesc,
		DryRun:     dogDispatchDryRun,
	}
	if p.RigName != "" {
		result.PluginRig = p.RigName
	}

	// Dry-run mode: show what would happen and exit
	if dogDispatchDryRun {
		if dogDispatchJSON {
			return json.NewEncoder(os.Stdout).Encode(result)
		}
		fmt.Printf("Dry run - would dispatch:\n")
		fmt.Printf("  Plugin: %s\n", p.Name)
		if p.RigName != "" {
			fmt.Printf("  Location: %s/plugins/%s\n", p.RigName, p.Name)
		} else {
			fmt.Printf("  Location: plugins/%s (town-level)\n", p.Name)
		}
		fmt.Printf("  Dog: %s%s\n", targetDog.Name, ifStr(dogCreated, " (would create)", ""))
		fmt.Printf("  Work: %s\n", workDesc)
		return nil
	}

	// Assign work FIRST (before sending mail) to prevent race condition
	// If this fails, we haven't sent any mail yet
	if err := mgr.AssignWork(targetDog.Name, workDesc); err != nil {
		return fmt.Errorf("assigning work to dog: %w", err)
	}

	// Create and send mail message with plugin instructions
	dogAddress := fmt.Sprintf("deacon/dogs/%s", targetDog.Name)
	subject := fmt.Sprintf("Plugin: %s", p.Name)
	body := formatPluginMailBody(p)

	router := mail.NewRouterWithTownRoot(townRoot, townRoot)
	msg := &mail.Message{
		From:      "deacon/",
		To:        dogAddress,
		Subject:   subject,
		Body:      body,
		Timestamp: time.Now(),
	}

	if err := router.Send(msg); err != nil {
		// Rollback: clear work assignment since mail failed
		if clearErr := mgr.ClearWork(targetDog.Name); clearErr != nil {
			// Log rollback failure but return original error
			if !dogDispatchJSON {
				fmt.Printf("  Warning: rollback failed: %v\n", clearErr)
			}
		}
		return fmt.Errorf("sending plugin mail to dog: %w", err)
	}

	// Success - output result
	if dogDispatchJSON {
		return json.NewEncoder(os.Stdout).Encode(result)
	}

	fmt.Printf("%s Found plugin: %s\n", style.Bold.Render("✓"), p.Name)
	if p.RigName != "" {
		fmt.Printf("  Location: %s/plugins/%s\n", p.RigName, p.Name)
	} else {
		fmt.Printf("  Location: plugins/%s (town-level)\n", p.Name)
	}
	if dogCreated {
		fmt.Printf("%s Created dog %s (pool was empty)\n", style.Bold.Render("✓"), targetDog.Name)
	}
	fmt.Printf("%s Dispatching to dog: %s\n", style.Bold.Render("🐕"), targetDog.Name)
	fmt.Printf("%s Plugin dispatched (non-blocking)\n", style.Bold.Render("✓"))
	fmt.Printf("  Dog: %s\n", targetDog.Name)
	fmt.Printf("  Work: %s\n", workDesc)

	return nil
}

// dogDispatchResult is the JSON output for gt dog dispatch.
type dogDispatchResult struct {
	Plugin     string `json:"plugin"`
	PluginRig  string `json:"plugin_rig,omitempty"`
	PluginPath string `json:"plugin_path"`
	Dog        string `json:"dog"`
	DogCreated bool   `json:"dog_created,omitempty"`
	Work       string `json:"work"`
	DryRun     bool   `json:"dry_run,omitempty"`
}

// ifStr returns ifTrue if cond is true, otherwise ifFalse.
func ifStr(cond bool, ifTrue, ifFalse string) string {
	if cond {
		return ifTrue
	}
	return ifFalse
}

// runDogClear clears stuck dogs back to idle state.
func runDogClear(cmd *cobra.Command, args []string) error {
	mgr, err := getDogManager()
	if err != nil {
		return err
	}

	var names []string
	if dogClearAll {
		dogs, err := mgr.List()
		if err != nil {
			return fmt.Errorf("listing dogs: %w", err)
		}
		// Only clear dogs that are in working state
		for _, d := range dogs {
			if d.State == dog.StateWorking {
				names = append(names, d.Name)
			}
		}
		if len(names) == 0 {
			fmt.Println("No stuck dogs to clear (all dogs are idle)")
			return nil
		}
	} else {
		names = args
	}

	cleared := 0
	for _, name := range names {
		d, err := mgr.Get(name)
		if err != nil {
			fmt.Printf("Warning: dog %s not found, skipping\n", name)
			continue
		}

		// Check if already idle
		if d.State == dog.StateIdle {
			fmt.Printf("Dog %s is already idle\n", name)
			continue
		}

		// Check if recently active (last 5 minutes) - may still be working
		if !dogClearForce && time.Since(d.LastActive) < 5*time.Minute {
			fmt.Printf("Warning: dog %s was active %s - may still be working (use --force to clear anyway)\n",
				name, dogFormatTimeAgo(d.LastActive))
			continue
		}

		// Clear the dog's work and set to idle
		if err := mgr.ClearWork(name); err != nil {
			return fmt.Errorf("clearing dog %s: %w", name, err)
		}

		fmt.Printf("✓ Cleared dog %s (was working on: %s)\n", name, d.Work)
		cleared++
	}

	if cleared > 0 {
		fmt.Printf("\n%d dog(s) cleared and ready\n", cleared)
	}

	return nil
}

// formatPluginMailBody formats the plugin as instructions for the dog.
func formatPluginMailBody(p *plugin.Plugin) string {
	var sb strings.Builder

	sb.WriteString("Execute the following plugin:\n\n")
	sb.WriteString(fmt.Sprintf("**Plugin**: %s\n", p.Name))
	sb.WriteString(fmt.Sprintf("**Description**: %s\n", p.Description))
	if p.RigName != "" {
		sb.WriteString(fmt.Sprintf("**Rig**: %s\n", p.RigName))
	}
	if p.Execution != nil && p.Execution.Timeout != "" {
		sb.WriteString(fmt.Sprintf("**Timeout**: %s\n", p.Execution.Timeout))
	}
	sb.WriteString("\n---\n\n")
	sb.WriteString("## Instructions\n\n")
	sb.WriteString(p.Instructions)
	sb.WriteString("\n\n---\n\n")
	sb.WriteString("After completion:\n")
	sb.WriteString("1. Create a wisp to record the result (success/failure)\n")
	sb.WriteString("2. Send DOG_DONE mail to deacon/\n")
	sb.WriteString("3. Return to idle state\n")

	return sb.String()
}
