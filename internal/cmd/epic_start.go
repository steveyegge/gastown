package cmd

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/epic"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	epicStartCrew   string // --crew flag: specific crew member to use
	epicStartNoCrew bool   // --no-crew flag: don't create/use crew
)

var epicStartCmd = &cobra.Command{
	Use:   "start <rig> [title]",
	Short: "Start a new epic for upstream contribution",
	Long: `Start a new epic for contributing a feature upstream.

This command:
1. Creates an epic bead in drafting state
2. Discovers CONTRIBUTING.md from the rig's repo
3. Hooks the epic to a crew member for planning
4. Prime will inject CONTRIBUTING.md into the context

When run by MAYOR:
- Prompts to create a crew member if none exist
- Slings epic to crew member for planning
- Outputs: "Go discuss this with <crew-member>"

When run by CREW/POLECAT:
- Hooks epic to current agent
- Begins planning conversation

EXAMPLES:

  gt epic start gastown "Add user authentication"
  gt epic start beads "Implement molecule validation" --crew dave
  gt epic start myrig "New feature"                  # Auto-selects crew`,
	Args: cobra.MinimumNArgs(1),
	RunE: runEpicStart,
}

func init() {
	epicStartCmd.Flags().StringVar(&epicStartCrew, "crew", "", "Specific crew member to use for planning")
	epicStartCmd.Flags().BoolVar(&epicStartNoCrew, "no-crew", false, "Don't create or use crew member")
}

func runEpicStart(cmd *cobra.Command, args []string) error {
	rigName := args[0]
	title := "New Epic"
	if len(args) > 1 {
		title = strings.Join(args[1:], " ")
	}

	// Find town root
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	// Validate rig exists
	rigPath := filepath.Join(townRoot, rigName)
	if _, err := os.Stat(rigPath); os.IsNotExist(err) {
		return fmt.Errorf("rig '%s' not found", rigName)
	}

	// Discover CONTRIBUTING.md
	contributingPath, contributingContent, err := epic.DiscoverContributing(filepath.Join(rigPath, "mayor", "rig"))
	if err != nil {
		return fmt.Errorf("discovering CONTRIBUTING.md: %w", err)
	}

	if contributingPath != "" {
		fmt.Printf("%s Found CONTRIBUTING.md at %s\n", style.Bold.Render("📋"), contributingPath)
	} else {
		fmt.Printf("%s No CONTRIBUTING.md found\n", style.Dim.Render("○"))
	}

	// Generate epic ID
	epicID := generateEpicID(rigName)

	// Create epic fields
	fields := &beads.EpicFields{
		EpicState:      beads.EpicStateDrafting,
		ContributingMD: contributingPath,
	}

	// Create epic bead
	bd := beads.New(filepath.Join(rigPath, "mayor", "rig"))
	epicIssue, err := bd.CreateEpicBead(epicID, title, fields)
	if err != nil {
		return fmt.Errorf("creating epic bead: %w", err)
	}

	fmt.Printf("%s Created epic %s: %s\n", style.Bold.Render("✓"), epicID, title)
	fmt.Printf("  State: %s\n", fields.EpicState)

	// Determine current role
	role := epicDetectRole()

	// Handle based on role
	if role == RoleMayor && !epicStartNoCrew {
		// Mayor flow: sling to crew member
		crewMember, err := ensureCrewExists(rigName, townRoot)
		if err != nil {
			return fmt.Errorf("ensuring crew exists: %w", err)
		}

		if crewMember == "" {
			// User declined to create crew
			fmt.Printf("\n%s Epic created but not assigned.\n", style.Dim.Render("○"))
			fmt.Printf("  Assign later with: gt sling %s <crew>\n", epicID)
			return nil
		}

		// Sling to crew member
		fmt.Printf("\n%s Slinging epic to %s for planning...\n", style.Bold.Render("→"), crewMember)

		target := fmt.Sprintf("%s/crew/%s", rigName, crewMember)
		slingArgs := []string{"sling", epicID, target}
		slingCmd := exec.Command("gt", slingArgs...)
		slingCmd.Stdout = os.Stdout
		slingCmd.Stderr = os.Stderr

		if err := slingCmd.Run(); err != nil {
			return fmt.Errorf("slinging epic: %w", err)
		}

		fmt.Printf("\n%s Go discuss this with %s to build the plan.\n",
			style.Bold.Render("💬"), crewMember)

		// Show CONTRIBUTING.md summary if found
		if contributingContent != "" {
			fmt.Printf("\n%s CONTRIBUTING.md will be injected into their context.\n",
				style.Dim.Render("○"))
		}
	} else {
		// Crew/Polecat flow: hook to self
		fmt.Printf("\n%s Epic hooked. Begin planning.\n", style.Bold.Render("📝"))

		// Hook the epic to current agent
		hookCmd := exec.Command("bd", "update", epicID, "--status=hooked")
		hookCmd.Dir = filepath.Join(rigPath, "mayor", "rig")
		if err := hookCmd.Run(); err != nil {
			fmt.Printf("%s Could not hook epic: %v\n", style.Dim.Render("Warning:"), err)
		}

		// Show CONTRIBUTING.md summary if found
		if contributingContent != "" {
			showContributingSummary(contributingContent)
		}

		fmt.Println()
		fmt.Println("Write your plan in the epic description using step format:")
		fmt.Println()
		fmt.Println("  ## Step: implement-api")
		fmt.Println("  Implement the core API changes")
		fmt.Println("  Tier: opus")
		fmt.Println()
		fmt.Println("  ## Step: add-tests")
		fmt.Println("  Write tests")
		fmt.Println("  Needs: implement-api")
		fmt.Println("  Tier: sonnet")
		fmt.Println()
		fmt.Printf("When plan is ready, run: gt epic ready %s\n", epicID)
	}

	// Store the epic issue for reference
	_ = epicIssue

	return nil
}

// ensureCrewExists checks if the rig has crew members and optionally creates one.
func ensureCrewExists(rigName, townRoot string) (string, error) {
	rigPath := filepath.Join(townRoot, rigName)
	crewPath := filepath.Join(rigPath, "crew")

	// Check if crew directory exists
	if _, err := os.Stat(crewPath); os.IsNotExist(err) {
		return promptCreateCrew(rigName)
	}

	// List crew members
	entries, err := os.ReadDir(crewPath)
	if err != nil {
		return "", err
	}

	var crewMembers []string
	for _, entry := range entries {
		if entry.IsDir() {
			crewMembers = append(crewMembers, entry.Name())
		}
	}

	if len(crewMembers) == 0 {
		return promptCreateCrew(rigName)
	}

	// If --crew specified, use it
	if epicStartCrew != "" {
		// Validate it exists
		for _, member := range crewMembers {
			if member == epicStartCrew {
				return epicStartCrew, nil
			}
		}
		return "", fmt.Errorf("crew member '%s' not found in rig '%s'", epicStartCrew, rigName)
	}

	// If single crew member, use it
	if len(crewMembers) == 1 {
		return crewMembers[0], nil
	}

	// Multiple crew members - prompt
	return promptSelectCrew(crewMembers)
}

// promptCreateCrew prompts the user to create a crew member.
func promptCreateCrew(rigName string) (string, error) {
	fmt.Printf("\n%s This rig doesn't have any crew members yet.\n", style.Warning.Render("⚠"))
	fmt.Println("Epics work best with a dedicated crew member for planning.")
	fmt.Println()
	fmt.Printf("Would you like to create one? [Y/n] ")

	var response string
	fmt.Scanln(&response)

	response = strings.ToLower(strings.TrimSpace(response))
	if response == "n" || response == "no" {
		return "", nil
	}

	// Suggest a name
	suggestedName := "contributor"
	fmt.Printf("Crew member name [%s]: ", suggestedName)

	var name string
	fmt.Scanln(&name)
	name = strings.TrimSpace(name)
	if name == "" {
		name = suggestedName
	}

	// Create crew member
	fmt.Printf("\nCreating crew member '%s'...\n", name)
	crewAddCmd := exec.Command("gt", "crew", "add", name, "--rig", rigName)
	crewAddCmd.Stdout = os.Stdout
	crewAddCmd.Stderr = os.Stderr

	if err := crewAddCmd.Run(); err != nil {
		return "", fmt.Errorf("creating crew member: %w", err)
	}

	return name, nil
}

// promptSelectCrew prompts the user to select a crew member.
func promptSelectCrew(members []string) (string, error) {
	fmt.Printf("\n%s Multiple crew members available:\n", style.Bold.Render("?"))
	for i, member := range members {
		fmt.Printf("  %d. %s\n", i+1, member)
	}
	fmt.Print("\nSelect crew member [1]: ")

	var response string
	fmt.Scanln(&response)
	response = strings.TrimSpace(response)

	if response == "" {
		return members[0], nil
	}

	var index int
	if _, err := fmt.Sscanf(response, "%d", &index); err != nil {
		// Try as name
		for _, member := range members {
			if member == response {
				return member, nil
			}
		}
		return members[0], nil
	}

	if index < 1 || index > len(members) {
		return members[0], nil
	}

	return members[index-1], nil
}

// showContributingSummary shows a brief summary of CONTRIBUTING.md.
func showContributingSummary(content string) {
	fmt.Printf("\n%s\n", style.Bold.Render("📋 CONTRIBUTING.md Summary:"))

	// Show first few meaningful lines
	lines := strings.Split(content, "\n")
	shown := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if shown >= 3 {
			fmt.Println("  ...")
			break
		}
		if len(line) > 80 {
			line = line[:77] + "..."
		}
		fmt.Printf("  %s\n", line)
		shown++
	}
	fmt.Println()
	fmt.Println(style.Dim.Render("Full CONTRIBUTING.md will be in your context."))
}

// generateEpicID generates a unique epic ID.
func generateEpicID(rigName string) string {
	// Get rig prefix from routes or use first 2-3 chars
	prefix := getBeadsPrefix(rigName)
	if prefix == "" {
		if len(rigName) >= 2 {
			prefix = strings.ToLower(rigName[:2])
		} else {
			prefix = "ep"
		}
	}

	// Generate short random ID
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	shortID := strings.ToLower(base32.StdEncoding.EncodeToString(b)[:5])

	return fmt.Sprintf("%s-epic-%s", prefix, shortID)
}

// getBeadsPrefix gets the beads prefix for a rig.
func getBeadsPrefix(rigName string) string {
	// Try to get from routes.jsonl
	// For now, use simple mapping
	prefixes := map[string]string{
		"gastown": "gt",
		"beads":   "bd",
	}
	if prefix, ok := prefixes[rigName]; ok {
		return prefix
	}
	return ""
}

// epicDetectRole detects the current role from environment.
func epicDetectRole() Role {
	// Check for polecat
	if os.Getenv("GT_POLECAT") != "" {
		return RolePolecat
	}

	// Check for crew
	cwd, _ := os.Getwd()
	if strings.Contains(cwd, "/crew/") {
		return RoleCrew
	}

	// Check for mayor
	if strings.Contains(cwd, "/mayor") || os.Getenv("GT_ROLE") == "mayor" {
		return RoleMayor
	}

	return RoleUnknown
}
