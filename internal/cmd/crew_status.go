package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/crew"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
)

// CrewStatusItem represents detailed status for a crew worker.
type CrewStatusItem struct {
	Name         string   `json:"name"`
	Rig          string   `json:"rig"`
	Path         string   `json:"path"`
	Branch       string   `json:"branch"`
	HasSession   bool     `json:"has_session"`
	SessionID    string   `json:"session_id,omitempty"`
	GitClean     bool     `json:"git_clean"`
	GitModified  []string `json:"git_modified,omitempty"`
	GitUntracked []string `json:"git_untracked,omitempty"`
	MailTotal    int      `json:"mail_total"`
	MailUnread   int      `json:"mail_unread"`
}

func runCrewStatus(cmd *cobra.Command, args []string) error {
	// Parse rig/name format before getting manager (e.g., "beads/emma" -> rig=beads, name=emma)
	var targetName string
	if len(args) > 0 {
		targetName = args[0]
		if rig, crewName, ok := parseRigSlashName(targetName); ok {
			if crewRig == "" {
				crewRig = rig
			}
			targetName = crewName
		} else if crewRig == "" {
			// Check if single arg (without "/") is a valid rig name
			// If so, show status for all crew in that rig
			if _, _, err := getRig(targetName); err == nil {
				crewRig = targetName
				targetName = "" // Show all crew in the rig
			}
		}
	}

	crewMgr, r, err := getCrewManager(crewRig)
	if err != nil {
		return err
	}

	var workers []*crew.CrewWorker

	if targetName != "" {
		// Specific worker
		worker, err := crewMgr.Get(targetName)
		if err != nil {
			if err == crew.ErrCrewNotFound {
				return fmt.Errorf("crew workspace '%s' not found", targetName)
			}
			return fmt.Errorf("getting crew worker: %w", err)
		}
		workers = []*crew.CrewWorker{worker}
	} else {
		// All workers
		workers, err = crewMgr.List()
		if err != nil {
			return fmt.Errorf("listing crew workers: %w", err)
		}
	}

	if len(workers) == 0 {
		fmt.Println("No crew workspaces found.")
		return nil
	}

	t := tmux.NewTmux()
	var items []CrewStatusItem

	for _, w := range workers {
		sessionID := crewSessionName(r.Name, w.Name)
		hasSession, _ := t.HasSession(sessionID)

		// Git status
		crewGit := git.NewGit(w.ClonePath)
		gitStatus, _ := crewGit.Status()
		branch, _ := crewGit.CurrentBranch()

		gitClean := true
		var modified, untracked []string
		if gitStatus != nil {
			gitClean = gitStatus.Clean
			modified = append(gitStatus.Modified, gitStatus.Added...)
			modified = append(modified, gitStatus.Deleted...)
			untracked = gitStatus.Untracked
		}

		// Mail status (non-fatal: display defaults to 0 if count fails)
		mailDir := filepath.Join(w.ClonePath, "mail")
		mailTotal, mailUnread := 0, 0
		if _, err := os.Stat(mailDir); err == nil {
			mailbox := mail.NewMailbox(mailDir)
			mailTotal, mailUnread, _ = mailbox.Count()
		}

		item := CrewStatusItem{
			Name:         w.Name,
			Rig:          r.Name,
			Path:         w.ClonePath,
			Branch:       branch,
			HasSession:   hasSession,
			GitClean:     gitClean,
			GitModified:  modified,
			GitUntracked: untracked,
			MailTotal:    mailTotal,
			MailUnread:   mailUnread,
		}
		if hasSession {
			item.SessionID = sessionID
		}

		items = append(items, item)
	}

	if crewJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(items)
	}

	// Text output
	for i, item := range items {
		if i > 0 {
			fmt.Println()
		}

		sessionStatus := style.Dim.Render("○ stopped")
		if item.HasSession {
			sessionStatus = style.Bold.Render("● running")
		}

		fmt.Printf("%s %s/%s\n", sessionStatus, item.Rig, item.Name)
		fmt.Printf("  Path:   %s\n", item.Path)
		fmt.Printf("  Branch: %s\n", item.Branch)

		if item.GitClean {
			fmt.Printf("  Git:    %s\n", style.Dim.Render("clean"))
		} else {
			fmt.Printf("  Git:    %s\n", style.Bold.Render("dirty"))
			if len(item.GitModified) > 0 {
				fmt.Printf("          Modified: %s\n", strings.Join(item.GitModified, ", "))
			}
			if len(item.GitUntracked) > 0 {
				fmt.Printf("          Untracked: %s\n", strings.Join(item.GitUntracked, ", "))
			}
		}

		if item.MailUnread > 0 {
			fmt.Printf("  Mail:   %d unread / %d total\n", item.MailUnread, item.MailTotal)
		} else {
			fmt.Printf("  Mail:   %s\n", style.Dim.Render(fmt.Sprintf("%d messages", item.MailTotal)))
		}
	}

	return nil
}
