package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/posting"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/templates"
	"github.com/steveyegge/gastown/internal/workspace"
)

var postingRig string
var postingAssumeReason string

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
  gt posting list       List available posting definitions
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
	Short: "List all available posting definitions",
	Long: `List all available postings (built-in + town + rig).

Shows posting definitions that can be assumed, not worker assignments.
Each posting is shown with its resolution level (embedded, town, rig).

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
  gt posting assume dispatcher --reason "bead X needs triage"
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
	Use:   "cycle [posting]",
	Short: "Drop current posting and restart session with clean context",
	Long: `Drop the current session-level posting and restart the session.

Unlike "gt posting drop" (which injects a system-reminder into the current
context), cycle triggers a full session restart via "gt handoff --cycle".
The new session starts with fresh priming — no stale posting instructions
polluting the context window.

With an argument: drops current posting, assumes the new one, then restarts.
Without an argument: drops current posting and restarts with no posting.

A persistent posting still blocks cycle. Clear it with
"gt crew post <name> --clear" first.

Examples:
  gt posting cycle              # drop posting, restart clean
  gt posting cycle scout        # drop current, assume scout, restart
  gt posting cycle dispatcher   # drop current, assume dispatcher, restart`,
	Args: cobra.MaximumNArgs(1),
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
	postingAssumeCmd.Flags().StringVar(&postingAssumeReason, "reason", "", "Reason for assuming this posting (logged to stderr)")

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

	postings := templates.ListAvailablePostings(townRoot, r.Path)
	if len(postings) == 0 {
		fmt.Println("No postings available")
		return nil
	}

	fmt.Printf("%s Available postings for %s:\n\n", style.Bold.Render("📋"), rigName)

	for _, p := range postings {
		fmt.Printf("  %-20s (%s)\n", p.Name, p.Level)
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

	// Validate that a template exists for this posting name.
	// Resolve town root and rig path for template lookup.
	townRoot, _ := workspace.FindFromCwd()
	rigPath := ""
	if info, err := GetRole(); err == nil && info.Rig != "" && townRoot != "" {
		rigPath = filepath.Join(townRoot, info.Rig)
	}
	if _, err := templates.LoadPosting(townRoot, rigPath, postingName); err != nil {
		return fmt.Errorf("posting %q not found: %w\n  Define it at:\n    Rig:  %s/postings/%s.md.tmpl\n    Town: %s/postings/%s.md.tmpl\n  Or use a built-in: %v",
			postingName, err, rigPath, postingName, townRoot, postingName, templates.BuiltinPostingNames())
	}

	if err := posting.Write(workDir, postingName); err != nil {
		return fmt.Errorf("writing posting: %w", err)
	}

	if postingAssumeReason != "" {
		fmt.Fprintf(os.Stderr, "posting assume: %s (reason: %s)\n", postingName, postingAssumeReason)
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
		persistent, workerName := getPersistentPosting()
		if persistent != "" {
			fmt.Printf("No session posting to drop (persistent posting %q is set for %s; use: gt crew post %s --clear)\n",
				persistent, workerName, workerName)
		} else {
			fmt.Println("No session posting to drop")
		}
		return nil
	}

	if err := posting.Clear(workDir); err != nil {
		return fmt.Errorf("clearing posting: %w", err)
	}

	fmt.Printf("%s Dropped posting: %s\n", style.Success.Render("✓"), current)
	outputPostingDropReminder(current)

	return nil
}

// outputPostingDropReminder emits a system-reminder so the agent knows it's no longer posted.
func outputPostingDropReminder(name string) {
	fmt.Printf("<system-reminder>\n")
	fmt.Printf("Your posting %q has been dropped. You are no longer posted.\n", name)
	fmt.Printf("You have returned to your base role. Any posting-specific context\n")
	fmt.Printf("or instructions no longer apply.\n")
	fmt.Printf("</system-reminder>\n")
}

func runPostingCycle(cmd *cobra.Command, args []string) error {
	var newPosting string
	if len(args) > 0 {
		newPosting = args[0]
	}

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

	// Assume new posting if requested
	if newPosting != "" {
		if err := posting.Write(workDir, newPosting); err != nil {
			return fmt.Errorf("writing posting: %w", err)
		}
	}

	// Report what happened
	switch {
	case old != "" && newPosting != "":
		fmt.Printf("%s Cycling posting: %s → %s (restarting session)\n", style.Success.Render("✓"), old, newPosting)
	case old != "" && newPosting == "":
		fmt.Printf("%s Cycling out of posting: %s (restarting session)\n", style.Success.Render("✓"), old)
	case old == "" && newPosting != "":
		fmt.Printf("%s Cycling into posting: %s (restarting session)\n", style.Success.Render("✓"), newPosting)
	default:
		fmt.Println("No posting to cycle — no session restart needed")
		return nil
	}

	// Trigger session restart for clean priming context.
	// This execs gt handoff --cycle, which replaces the current process.
	gtBin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding gt executable: %w", err)
	}
	handoffArgs := []string{gtBin, "handoff", "--cycle"}
	return syscall.Exec(gtBin, handoffArgs, os.Environ())
}
