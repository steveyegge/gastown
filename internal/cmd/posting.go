package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/posting"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var postingRig string

var postingCmd = &cobra.Command{
	Use:     "posting",
	GroupID: GroupAgents,
	Short:   "Manage posting assignments (specialized roles)",
	Long: `Manage posting assignments for agents.

A posting is a specialized role that augments the base agent context.
There are two layers:

  Persistent: Set via "gt crew post <name> <posting>". Stored in rig
              settings and applied on every session start.

  Session:    Set via "gt posting assume <posting>". Stored in
              .runtime/posting and cleared on handoff/completion/drop.

Conflict rules:
  - A persistent posting blocks "gt posting assume" (must clear first)
  - "gt posting drop" only clears the session-level posting
  - Use "gt crew post <name> --clear" to remove persistent postings

Commands:
  gt posting show       Show current posting (session + persistent)
  gt posting list       List available postings in rig settings
  gt posting assume     Assume a session-level posting
  gt posting drop       Drop the current session-level posting
  gt posting cycle      Drop current and assume a new posting
  gt posting create     Scaffold a new rig-level posting template`,
	RunE: requireSubcommand,
}

var postingShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current posting for this agent",
	Long: `Show the current posting assignment for this agent.

Displays both session-level (transient) and persistent postings,
and indicates which one is active.

Examples:
  gt posting show`,
	RunE: runPostingShow,
}

var postingListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all posting assignments in the rig",
	Long: `List all persistent posting assignments from rig settings.

Shows which workers have postings assigned via "gt crew post".

Examples:
  gt posting list
  gt posting list --rig gastown`,
	RunE: runPostingList,
}

var postingAssumeCmd = &cobra.Command{
	Use:   "assume <posting>",
	Short: "Assume a session-level posting",
	Long: `Assume a session-level posting for the current agent.

This writes the posting to .runtime/posting in the agent's work directory.
The posting takes effect immediately and is cleared on handoff, completion,
or explicit drop.

A persistent posting (set via "gt crew post") blocks assume. You must
clear the persistent posting first or use "gt posting drop" to drop
the session posting only.

Examples:
  gt posting assume dispatcher
  gt posting assume security-reviewer`,
	Args: cobra.ExactArgs(1),
	RunE: runPostingAssume,
}

var postingDropCmd = &cobra.Command{
	Use:   "drop",
	Short: "Drop the current session-level posting",
	Long: `Drop the current session-level posting.

Removes the .runtime/posting file, returning to the base role.
This does NOT affect persistent postings set via "gt crew post".

Examples:
  gt posting drop`,
	RunE: runPostingDrop,
}

var postingCycleCmd = &cobra.Command{
	Use:   "cycle <posting>",
	Short: "Drop current posting and assume a new one",
	Long: `Drop the current session-level posting and assume a new one.

Equivalent to "gt posting drop && gt posting assume <posting>".
This bypasses the "must drop before re-assume" check since the
drop happens first.

A persistent posting still blocks cycle. Clear it with
"gt crew post <name> --clear" first.

Examples:
  gt posting cycle scout
  gt posting cycle dispatcher`,
	Args: cobra.ExactArgs(1),
	RunE: runPostingCycle,
}

var postingCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Scaffold a new rig-level posting template",
	Long: `Create a new posting template in the current rig's postings directory.

This scaffolds a starter .md.tmpl file at <rig>/postings/<name>.md.tmpl
with placeholder content and RoleData template variables. Edit the file
to define the posting's behavioral constraints.

Example:
  gt posting create reviewer
  gt posting create reviewer --rig gastown`,
	Args: cobra.ExactArgs(1),
	RunE: runPostingCreate,
}

func init() {
	postingCmd.AddCommand(postingShowCmd)
	postingCmd.AddCommand(postingListCmd)
	postingCmd.AddCommand(postingAssumeCmd)
	postingCmd.AddCommand(postingDropCmd)
	postingCmd.AddCommand(postingCycleCmd)
	postingCmd.AddCommand(postingCreateCmd)

	postingListCmd.Flags().StringVar(&postingRig, "rig", "", "Rig to list postings from")
	postingCreateCmd.Flags().StringVar(&postingRig, "rig", "", "Rig to create posting in")

	rootCmd.AddCommand(postingCmd)
}

func runPostingCreate(cmd *cobra.Command, args []string) error {
	name := args[0]

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	rigName := postingRig
	if rigName == "" {
		rigName, err = inferRigFromCwd(townRoot)
		if err != nil {
			return fmt.Errorf("could not determine rig (use --rig flag): %w", err)
		}
	}

	_, r, err := getRig(rigName)
	if err != nil {
		return err
	}

	postingsDir := filepath.Join(r.Path, "postings")
	if err := os.MkdirAll(postingsDir, 0755); err != nil {
		return fmt.Errorf("creating postings directory: %w", err)
	}

	templatePath := filepath.Join(postingsDir, name+".md.tmpl")
	if _, err := os.Stat(templatePath); err == nil {
		return fmt.Errorf("posting %q already exists at %s", name, templatePath)
	}

	content := fmt.Sprintf(`# Posting: %s

You are operating under the **%s** posting. This augments your base role
with additional responsibilities.

## Responsibilities

- TODO: Define what this posting specializes in
- TODO: Define behavioral constraints (what you do / don't do)

## Principles

1. TODO: Add guiding principles for this posting

## Key Commands

`+"```bash"+`
bd ready                        # Find unblocked work
{{ cmd }} nudge <target> "msg"         # Coordinate with other workers
`+"```"+`
`, name, name)

	if err := os.WriteFile(templatePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing template: %w", err)
	}

	fmt.Printf("%s Created posting template: %s\n", style.Success.Render("✓"), templatePath)
	fmt.Printf("  Edit the file to define %s's behavioral constraints.\n", name)
	return nil
}

// getWorkDir returns the agent's work directory for posting state.
// Uses GT_ROLE_HOME if set, otherwise falls back to cwd.
func getWorkDir() (string, error) {
	if home := os.Getenv(EnvGTRoleHome); home != "" {
		return home, nil
	}
	return os.Getwd()
}

// getPersistentPosting returns the persistent posting for the current agent
// from rig settings, if any. Returns ("", "") if none.
func getPersistentPosting() (postingName string, workerName string) {
	info, err := GetRole()
	if err != nil {
		return "", ""
	}

	if info.Rig == "" || info.Polecat == "" {
		return "", ""
	}

	townRoot, err := workspace.FindFromCwd()
	if err != nil || townRoot == "" {
		return "", ""
	}

	settingsPath := config.RigSettingsPath(filepath.Join(townRoot, info.Rig))
	settings, err := config.LoadRigSettings(settingsPath)
	if err != nil {
		return "", ""
	}

	if settings.WorkerPostings == nil {
		return "", ""
	}

	p, ok := settings.WorkerPostings[info.Polecat]
	if !ok {
		return "", ""
	}

	return p, info.Polecat
}

func runPostingShow(cmd *cobra.Command, args []string) error {
	workDir, err := getWorkDir()
	if err != nil {
		return fmt.Errorf("getting work directory: %w", err)
	}

	sessionPosting := posting.Read(workDir)
	persistentPosting, workerName := getPersistentPosting()

	if sessionPosting == "" && persistentPosting == "" {
		fmt.Println("No posting assigned")
		return nil
	}

	if persistentPosting != "" {
		fmt.Printf("%s %s (persistent, worker: %s)\n",
			style.Bold.Render("Posting:"), persistentPosting, workerName)
		if sessionPosting != "" && sessionPosting != persistentPosting {
			fmt.Printf("%s %s (session override)\n",
				style.Bold.Render("Session:"), sessionPosting)
		}
	} else if sessionPosting != "" {
		fmt.Printf("%s %s (session)\n",
			style.Bold.Render("Posting:"), sessionPosting)
	}

	return nil
}

func runPostingList(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	rigName := postingRig
	if rigName == "" {
		rigName, err = inferRigFromCwd(townRoot)
		if err != nil {
			return fmt.Errorf("could not determine rig (use --rig flag): %w", err)
		}
	}

	_, r, err := getRig(rigName)
	if err != nil {
		return err
	}

	settingsPath := filepath.Join(r.Path, "settings", "config.json")
	settings, err := config.LoadRigSettings(settingsPath)
	if err != nil {
		if errors.Is(err, config.ErrNotFound) {
			fmt.Println("No postings configured (no settings file)")
			return nil
		}
		return fmt.Errorf("loading settings: %w", err)
	}

	if len(settings.WorkerPostings) == 0 {
		fmt.Println("No postings configured")
		return nil
	}

	fmt.Printf("%s Worker postings for %s:\n\n", style.Bold.Render("📋"), rigName)

	// Sort by worker name for consistent output
	names := make([]string, 0, len(settings.WorkerPostings))
	for name := range settings.WorkerPostings {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		fmt.Printf("  %-16s %s\n", name, settings.WorkerPostings[name])
	}

	return nil
}

func runPostingAssume(cmd *cobra.Command, args []string) error {
	postingName := args[0]

	workDir, err := getWorkDir()
	if err != nil {
		return fmt.Errorf("getting work directory: %w", err)
	}

	// Check for persistent posting conflict
	persistent, workerName := getPersistentPosting()
	if persistent != "" {
		return fmt.Errorf("persistent posting %q is set for %s — clear it first with: gt crew post %s --clear",
			persistent, workerName, workerName)
	}

	// Check if already assumed (must drop before re-assume)
	current := posting.Read(workDir)
	if current != "" {
		return fmt.Errorf("already assumed posting %q — drop it first with: gt posting drop", current)
	}

	if err := posting.Write(workDir, postingName); err != nil {
		return fmt.Errorf("writing posting: %w", err)
	}

	fmt.Printf("%s Assumed posting: %s\n", style.Success.Render("✓"), postingName)
	return nil
}

func runPostingDrop(cmd *cobra.Command, args []string) error {
	workDir, err := getWorkDir()
	if err != nil {
		return fmt.Errorf("getting work directory: %w", err)
	}

	current := posting.Read(workDir)
	if current == "" {
		fmt.Println("No session posting to drop")
		return nil
	}

	if err := posting.Clear(workDir); err != nil {
		return fmt.Errorf("clearing posting: %w", err)
	}

	fmt.Printf("%s Dropped posting: %s\n", style.Success.Render("✓"), current)

	// Emit system-reminder so the agent knows it's no longer posted.
	fmt.Printf("<system-reminder>\n")
	fmt.Printf("Your posting %q has been dropped. You are no longer posted.\n", current)
	fmt.Printf("You have returned to your base role. Any posting-specific context\n")
	fmt.Printf("or instructions no longer apply.\n")
	fmt.Printf("</system-reminder>\n")

	return nil
}

func runPostingCycle(cmd *cobra.Command, args []string) error {
	newPosting := args[0]

	workDir, err := getWorkDir()
	if err != nil {
		return fmt.Errorf("getting work directory: %w", err)
	}

	// Check for persistent posting conflict
	persistent, workerName := getPersistentPosting()
	if persistent != "" {
		return fmt.Errorf("persistent posting %q is set for %s — clear it first with: gt crew post %s --clear",
			persistent, workerName, workerName)
	}

	// Drop current (if any)
	old := posting.Read(workDir)
	if old != "" {
		if err := posting.Clear(workDir); err != nil {
			return fmt.Errorf("clearing posting: %w", err)
		}
	}

	// Assume new
	if err := posting.Write(workDir, newPosting); err != nil {
		return fmt.Errorf("writing posting: %w", err)
	}

	if old != "" {
		fmt.Printf("%s Cycled posting: %s → %s\n", style.Success.Render("✓"), old, newPosting)
	} else {
		fmt.Printf("%s Assumed posting: %s\n", style.Success.Render("✓"), newPosting)
	}
	return nil
}
