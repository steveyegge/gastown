package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

// forwardPattern matches "forward to <target>:" and "foward to <target>:" (common typo).
// The target can be a name like "melania" or a group like "PMs".
// Everything after the colon becomes the forwarded subject.
var forwardPattern = regexp.MustCompile(`(?i)^fo[r]?ward\s+to\s+(.+?):\s*(.*)$`)

var mailAutoforwardCmd = &cobra.Command{
	Use:   "autoforward",
	Short: "Auto-forward 'forward to <name>:' messages to recipients",
	Long: `Scan unread messages for overseer "forward to <name>:" subjects
and automatically forward the message body to the resolved recipient.

The target name is resolved by searching crew/ and polecats/ directories
across all rigs. Special targets:
  "PMs" or "all PMs" → forwards to all PM agents (melania, dallas, zhora)
  "melania"           → resolves to the rig containing crew/melania
  "cfutons/melania"   → direct address (passed through)

After forwarding, the original message is marked as read.

Also integrates with 'gt mail check --inject' for automatic forwarding
on every prompt hook.

Examples:
  gt mail autoforward    # Scan and forward all matching unread messages`,
	RunE: runMailAutoforward,
}

func runMailAutoforward(cmd *cobra.Command, args []string) error {
	address := detectSender()

	workDir, err := findMailWorkDir()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	forwarded, errs := autoForwardMessages(address, workDir, false)
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "⚠ %s\n", e)
		}
	}
	if forwarded == 0 {
		fmt.Println("No messages to forward")
	} else {
		fmt.Printf("%s Auto-forwarded %d message(s)\n", style.Bold.Render("✓"), forwarded)
	}
	return nil
}

// autoForwardMessages scans unread messages for "forward to <name>:" subjects
// and forwards them to the resolved recipient. Returns count of forwarded
// messages and any errors encountered.
// If silent is true, no output is printed (for inject mode integration).
func autoForwardMessages(address, workDir string, silent bool) (int, []string) {
	router := mail.NewRouter(workDir)
	mailbox, err := router.GetMailbox(address)
	if err != nil {
		return 0, []string{fmt.Sprintf("mailbox error: %v", err)}
	}

	messages, err := mailbox.ListUnread()
	if err != nil {
		return 0, []string{fmt.Sprintf("listing unread: %v", err)}
	}

	townRoot, _ := workspace.FindFromCwd()
	forwarded := 0
	var errs []string

	for _, msg := range messages {
		matches := forwardPattern.FindStringSubmatch(msg.Subject)
		if matches == nil {
			continue
		}

		target := strings.TrimSpace(matches[1])
		newSubject := strings.TrimSpace(matches[2])
		if newSubject == "" {
			newSubject = fmt.Sprintf("[Forwarded from %s]", msg.From)
		}

		// Resolve target to address(es)
		recipients, resolveErr := resolveForwardTarget(target, townRoot)
		if resolveErr != nil {
			errs = append(errs, fmt.Sprintf("cannot resolve %q: %v", target, resolveErr))
			continue
		}

		// Build forwarded body with attribution
		body := fmt.Sprintf("[Forwarded by %s from %s]\n\n%s", address, msg.From, msg.Body)

		// Send to each recipient
		sendOK := true
		for _, recipAddr := range recipients {
			fwdMsg := mail.NewMessage(address, recipAddr, newSubject, body)
			fwdMsg.Priority = msg.Priority
			fwdMsg.Wisp = true
			if sendErr := router.Send(fwdMsg); sendErr != nil {
				errs = append(errs, fmt.Sprintf("send to %s: %v", recipAddr, sendErr))
				sendOK = false
			} else if !silent {
				fmt.Printf("  → Forwarded to %s: %s\n", recipAddr, newSubject)
			}
		}

		// Mark original as read if all sends succeeded
		if sendOK {
			if markErr := mailbox.MarkReadOnly(msg.ID); markErr != nil {
				errs = append(errs, fmt.Sprintf("marking %s as read: %v", msg.ID, markErr))
			}
			forwarded++
		}
	}

	router.WaitPendingNotifications()
	return forwarded, errs
}

// resolveForwardTarget resolves a bare name or group alias to mail addresses.
func resolveForwardTarget(target, townRoot string) ([]string, error) {
	lower := strings.ToLower(strings.TrimSpace(target))

	// Handle "PMs" / "all PMs" / "all pms" group alias
	if lower == "pms" || lower == "all pms" || lower == "all pm" {
		return findAllPMs(townRoot)
	}

	// If target contains "/" it's already a full address
	if strings.Contains(target, "/") {
		return []string{target}, nil
	}

	// Bare name — search workspace directories for matching agent
	return findAgentByName(target, townRoot)
}

// findAllPMs returns addresses for all designated PM agents.
func findAllPMs(townRoot string) ([]string, error) {
	if townRoot == "" {
		return nil, fmt.Errorf("cannot discover PMs: town root not found")
	}

	knownPMs := []string{"melania", "dallas", "zhora"}
	var addresses []string

	for _, name := range knownPMs {
		addrs, err := findAgentByName(name, townRoot)
		if err == nil && len(addrs) > 0 {
			addresses = append(addresses, addrs...)
		}
	}

	if len(addresses) == 0 {
		return nil, fmt.Errorf("no PMs found in workspace")
	}
	return addresses, nil
}

// findAgentByName searches crew/ and polecats/ across all rigs for an agent.
func findAgentByName(name, townRoot string) ([]string, error) {
	if townRoot == "" {
		return nil, fmt.Errorf("cannot resolve %q: town root not found", name)
	}

	name = strings.TrimSpace(name)

	entries, err := os.ReadDir(townRoot)
	if err != nil {
		return nil, fmt.Errorf("reading town root: %w", err)
	}

	var found []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		rigName := entry.Name()
		// Skip non-rig directories
		if strings.HasPrefix(rigName, ".") || rigName == "mayor" || rigName == "config" || rigName == "scripts" {
			continue
		}

		// Check crew/<name>
		crewPath := filepath.Join(townRoot, rigName, "crew", name)
		if info, statErr := os.Stat(crewPath); statErr == nil && info.IsDir() {
			found = append(found, rigName+"/"+name)
		}

		// Check polecats/<name>
		polecatPath := filepath.Join(townRoot, rigName, "polecats", name)
		if info, statErr := os.Stat(polecatPath); statErr == nil && info.IsDir() {
			found = append(found, rigName+"/"+name)
		}
	}

	if len(found) == 0 {
		return nil, fmt.Errorf("no agent named %q found in any rig", name)
	}

	return found, nil
}
