package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/crew"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/posting"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
)

// CrewListItem represents a crew worker in list output.
type CrewListItem struct {
	Name           string `json:"name"`
	Rig            string `json:"rig"`
	Branch         string `json:"branch"`
	Path           string `json:"path"`
	HasSession     bool   `json:"has_session"`
	GitClean       bool   `json:"git_clean"`
	Posting        string `json:"posting,omitempty"`
	PostingSource  string `json:"posting_source,omitempty"` // "session", "config", or ""
}

func runCrewList(cmd *cobra.Command, args []string) error {
	// Accept positional rig argument: gt crew list <rig>
	if len(args) > 0 {
		if crewRig != "" {
			return fmt.Errorf("cannot specify both positional rig argument and --rig flag")
		}
		crewRig = args[0]
	}

	if crewListAll && crewRig != "" {
		return fmt.Errorf("cannot use --all with a rig filter (--rig flag or positional argument)")
	}

	var rigs []*rig.Rig
	if crewListAll {
		allRigs, err := getAllRigs()
		if err != nil {
			return err
		}
		rigs = allRigs
	} else {
		_, r, err := getCrewManager(crewRig)
		if err != nil {
			// If no rig was explicitly specified and inference failed,
			// fall back to --all behavior instead of erroring.
			if crewRig == "" {
				allRigs, err2 := getAllRigs()
				if err2 != nil {
					return err2
				}
				rigs = allRigs
			} else {
				return err
			}
		} else {
			rigs = []*rig.Rig{r}
		}
	}

	// Check session and git status for each worker
	t := tmux.NewTmux()
	var items []CrewListItem

	for _, r := range rigs {
		crewGit := git.NewGit(r.Path)
		crewMgr := crew.NewManager(r, crewGit)

		workers, err := crewMgr.List()
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to list crew workers in %s: %v\n", r.Name, err)
			continue
		}

		// Load persistent postings from rig settings (once per rig)
		settingsPath := config.RigSettingsPath(r.Path)
		rigSettings, _ := config.LoadRigSettings(settingsPath)
		var workerPostings map[string]string
		if rigSettings != nil {
			workerPostings = rigSettings.WorkerPostings
		}

		for _, w := range workers {
			sessionID := crewSessionName(r.Name, w.Name)
			hasSession, _ := t.HasSession(sessionID)

			workerGit := git.NewGit(w.ClonePath)
			gitClean := true
			if status, err := workerGit.Status(); err == nil {
				gitClean = status.Clean
			}

			// Resolve posting: session (transient) takes priority over config (persistent)
			var postingName, postingSource string
			if sp := posting.Read(w.ClonePath); sp != "" {
				postingName = sp
				postingSource = "session"
			} else if workerPostings != nil {
				if pp, ok := workerPostings[w.Name]; ok && pp != "" {
					postingName = pp
					postingSource = "config"
				}
			}

			items = append(items, CrewListItem{
				Name:          w.Name,
				Rig:           r.Name,
				Branch:        w.Branch,
				Path:          w.ClonePath,
				HasSession:    hasSession,
				GitClean:      gitClean,
				Posting:       postingName,
				PostingSource: postingSource,
			})
		}
	}

	if len(items) == 0 {
		fmt.Println("No crew workspaces found.")
		return nil
	}

	if crewJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(items)
	}

	// Text output
	fmt.Printf("%s\n\n", style.Bold.Render("Crew Workspaces"))
	for _, item := range items {
		status := style.Dim.Render("○")
		if item.HasSession {
			status = style.Bold.Render("●")
		}

		gitStatus := style.Dim.Render("clean")
		if !item.GitClean {
			gitStatus = style.Bold.Render("dirty")
		}

		postingStr := ""
		if item.Posting != "" {
			postingStr = fmt.Sprintf("  [%s (%s)]", item.Posting, item.PostingSource)
		}

		fmt.Printf("  %s %s/%s%s\n", status, item.Rig, item.Name, postingStr)
		fmt.Printf("    Branch: %s  Git: %s\n", item.Branch, gitStatus)
		fmt.Printf("    %s\n", style.Dim.Render(item.Path))
	}

	return nil
}
