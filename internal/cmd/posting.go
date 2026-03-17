package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"text/template"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/posting"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/templates"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

var postingRig string
var postingAssumeReason string
var postingListAll bool

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
	Use:   "show [name]",
	Short: "Show current posting or display a posting's content",
	Long: `Show the current posting assignment, or display a named posting's content.

Without arguments: displays both session-level (transient) and persistent
postings, and indicates which one is active.

With a posting name: loads and displays the posting template content,
showing which resolution level it came from (embedded, town, or rig).

Examples:
  gt posting show              # show current posting state
  gt posting show dispatcher   # display dispatcher posting content`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPostingShow,
}

var postingListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available posting definitions",
	Long: `List all available postings (built-in + town + rig).

Shows posting definitions that can be assumed, not worker assignments.
Each posting is shown with its resolution level (embedded, town, rig).

When run from inside a rig directory, shows that rig's postings.
When run from outside a rig (e.g. town root), shows all postings
across all rigs. Use --all to explicitly show all rigs' postings
regardless of current directory.

Examples:
  gt posting list
  gt posting list --rig gastown
  gt posting list --all`,
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
	postingListCmd.Flags().BoolVar(&postingListAll, "all", false, "Show postings from all rigs")
	postingCreateCmd.Flags().StringVar(&postingRig, "rig", "", "Rig to create posting in")
	postingAssumeCmd.Flags().StringVar(&postingAssumeReason, "reason", "", "Reason for assuming this posting (logged to stderr)")

	rootCmd.AddCommand(postingCmd)
}

// validatePostingName rejects names that contain path separators or traversal sequences.
func validatePostingName(name string) error {
	if name == "" {
		return fmt.Errorf("posting name cannot be empty")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("posting name %q is not allowed", name)
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("posting name %q contains path separators", name)
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("posting name %q contains path traversal sequence", name)
	}
	return nil
}

func runPostingCreate(cmd *cobra.Command, args []string) error {
	name := args[0]

	if err := validatePostingName(name); err != nil {
		return err
	}

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

	content := fmt.Sprintf(`---
description: "TODO: Brief one-line description of this posting"
---
# Posting: %s

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
// Uses GT_ROLE_HOME if set, otherwise falls back to cwd after validating
// it is within a Gas Town workspace. Without validation, running from
// arbitrary directories (e.g. /tmp) can read phantom .runtime/posting
// state left by tests or prior sessions (gt-2hz).
func getWorkDir() (string, error) {
	if home := os.Getenv(EnvGTRoleHome); home != "" {
		return home, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	// Verify cwd is within a Gas Town workspace to prevent reading
	// phantom .runtime/posting state from arbitrary directories.
	townRoot, findErr := workspace.Find(cwd)
	if findErr != nil || townRoot == "" {
		return "", fmt.Errorf("not in a Gas Town workspace (cwd: %s) — set GT_ROLE_HOME or run from a workspace directory", cwd)
	}
	return cwd, nil
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
	// If a posting name is provided, display its content
	if len(args) == 1 {
		return showPostingContent(args[0])
	}

	// No args: show current session/persistent posting state
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

// showPostingContent loads and displays a posting template's content.
func showPostingContent(name string) error {
	if err := validatePostingName(name); err != nil {
		return err
	}

	townRoot, _ := workspace.FindFromCwd()
	rigPath := ""
	if info, err := GetRole(); err == nil && info.Rig != "" && townRoot != "" {
		rigPath = filepath.Join(townRoot, info.Rig)
	}

	result, err := templates.LoadPosting(townRoot, rigPath, name)
	if err != nil {
		return fmt.Errorf("posting %q not found: %w\n  Available built-in postings: %v",
			name, err, templates.BuiltinPostingNames())
	}

	// Render the template to resolve {{ cmd }} and other template variables.
	tmpl, err := template.New("posting").Funcs(templates.TemplateFuncs()).Parse(result.Content)
	if err != nil {
		return fmt.Errorf("posting %q: template parse error: %w", name, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, templates.RoleData{Posting: name}); err != nil {
		return fmt.Errorf("posting %q: template render error: %w", name, err)
	}

	fmt.Printf("%s %s (level: %s)\n\n", style.Bold.Render("Posting:"), result.Name, result.Level)
	fmt.Print(buf.String())
	return nil
}

func runPostingList(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Determine mode: --all, --rig, inferred rig, or fallback to all rigs
	if postingListAll {
		return listPostingsAllRigs(townRoot)
	}

	rigName := postingRig
	if rigName == "" {
		rigName, err = inferRigFromCwd(townRoot)
		if err != nil {
			// Outside a rig — show all rigs' postings
			return listPostingsAllRigs(townRoot)
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
		if p.Description != "" {
			fmt.Printf("  %-20s (%-8s)  %s\n", p.Name, p.Level, p.Description)
		} else {
			fmt.Printf("  %-20s (%s)\n", p.Name, p.Level)
		}
	}

	return nil
}

// listPostingsAllRigs shows postings from all rigs, plus embedded and town-level.
func listPostingsAllRigs(townRoot string) error {
	rigs, err := getAllRigs()
	if err != nil {
		return fmt.Errorf("discovering rigs: %w", err)
	}

	// First show embedded + town-level (not rig-specific)
	globalPostings := templates.ListAvailablePostings(townRoot, "")
	if len(globalPostings) > 0 {
		fmt.Printf("%s Embedded & town-level postings:\n\n", style.Bold.Render("📋"))
		for _, p := range globalPostings {
			if p.Description != "" {
				fmt.Printf("  %-20s (%-8s)  %s\n", p.Name, p.Level, p.Description)
			} else {
				fmt.Printf("  %-20s (%s)\n", p.Name, p.Level)
			}
		}
	}

	// Then show each rig's postings (rig-level only, skip if no rig-specific ones)
	for _, r := range rigs {
		rigPostings := templates.ListAvailablePostings(townRoot, r.Path)
		// Filter to only rig-level postings for this section
		var rigOnly []templates.PostingInfo
		for _, p := range rigPostings {
			if p.Level == "rig" {
				rigOnly = append(rigOnly, p)
			}
		}
		if len(rigOnly) == 0 {
			continue
		}
		fmt.Printf("\n%s Rig-level postings for %s:\n\n", style.Bold.Render("📋"), r.Name)
		for _, p := range rigOnly {
			if p.Description != "" {
				fmt.Printf("  %-20s (%-8s)  %s\n", p.Name, p.Level, p.Description)
			} else {
				fmt.Printf("  %-20s (%s)\n", p.Name, p.Level)
			}
		}
	}

	if len(globalPostings) == 0 && len(rigs) == 0 {
		fmt.Println("No postings available")
	}

	return nil
}

func runPostingAssume(cmd *cobra.Command, args []string) error {
	postingName := args[0]

	if err := validatePostingName(postingName); err != nil {
		return err
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

	// Update BD_ACTOR and GIT_AUTHOR_NAME in the running process so that
	// subsequent bd invocations within this session include bracket notation.
	// Without this, BD_ACTOR retains the base form set at session start.
	updatePostingEnvVars(postingName)

	// Update tmux window name to reflect posting (gt-drc, gt-iew).
	if info, err := GetRole(); err == nil {
		tmux.RenameWindow(posting.FormatWindowName(info.Polecat, postingName))
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

	// Restore BD_ACTOR and GIT_AUTHOR_NAME to base form (without brackets).
	clearPostingEnvVars()

	// Revert tmux window name to worker name without posting (gt-drc, gt-iew).
	if info, err := GetRole(); err == nil {
		tmux.RenameWindow(posting.FormatWindowName(info.Polecat, ""))
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

// updatePostingEnvVars appends bracket notation to BD_ACTOR and GIT_AUTHOR_NAME
// in the running process, and sets GT_POSTING. This ensures that bd invocations
// within the current session reflect the active posting.
func updatePostingEnvVars(postingName string) {
	if actor := os.Getenv("BD_ACTOR"); actor != "" {
		os.Setenv("BD_ACTOR", config.AppendPostingBracket(actor, postingName))
	}
	if author := os.Getenv("GIT_AUTHOR_NAME"); author != "" {
		os.Setenv("GIT_AUTHOR_NAME", config.AppendPostingBracket(author, postingName))
	}
	os.Setenv("GT_POSTING", postingName)
}

// clearPostingEnvVars strips bracket notation from BD_ACTOR and GIT_AUTHOR_NAME
// in the running process, and unsets GT_POSTING.
func clearPostingEnvVars() {
	if actor := os.Getenv("BD_ACTOR"); actor != "" {
		os.Setenv("BD_ACTOR", config.StripPostingBracket(actor))
	}
	if author := os.Getenv("GIT_AUTHOR_NAME"); author != "" {
		os.Setenv("GIT_AUTHOR_NAME", config.StripPostingBracket(author))
	}
	os.Unsetenv("GT_POSTING")
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
