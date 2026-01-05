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
)

var dogCmd = &cobra.Command{
	Use:     "dog",
	Aliases: []string{"dogs"},
	GroupID: GroupAgents,
	Short:   "Manage dogs (Deacon's helper workers)",
	Long: `Manage dogs in the kennel.

Dogs are reusable helper workers managed by the Deacon for infrastructure
and cleanup tasks. Unlike polecats (single-rig, ephemeral), dogs handle
cross-rig infrastructure work with worktrees into each rig.

The kennel is located at ~/gt/deacon/dogs/.`,
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

	// Add subcommands
	dogCmd.AddCommand(dogAddCmd)
	dogCmd.AddCommand(dogRemoveCmd)
	dogCmd.AddCommand(dogListCmd)
	dogCmd.AddCommand(dogCallCmd)
	dogCmd.AddCommand(dogStatusCmd)

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
			fmt.Printf("Dog %s is already working\n", name)
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
