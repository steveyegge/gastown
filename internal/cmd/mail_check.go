package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/style"
)

const mailCheckCacheTTL = 30 * time.Second

// mailCheckCacheDir is the directory for mail check cache files.
// Override in tests with t.TempDir().
var mailCheckCacheDir = ""

// mailCheckCacheEntry is a cached result from gt mail check.
type mailCheckCacheEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Address   string    `json:"address"`
	Unread    int       `json:"unread"`
	Subjects  []string  `json:"subjects,omitempty"`
}

// mailCheckCachePath returns the cache file path for a given address.
// Slashes in the address are replaced with underscores for safe filenames.
func mailCheckCachePath(address string) string {
	dir := mailCheckCacheDir
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "gt-mail-check-cache")
	}
	safe := strings.ReplaceAll(address, "/", "_")
	return filepath.Join(dir, safe+".json")
}

// loadMailCheckCache loads a cached mail check entry if it exists and is not expired.
// Returns nil if the cache is missing, expired, or malformed.
func loadMailCheckCache(address string) *mailCheckCacheEntry {
	data, err := os.ReadFile(mailCheckCachePath(address))
	if err != nil {
		return nil
	}
	var entry mailCheckCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil
	}
	if entry.Address != address {
		return nil
	}
	if time.Since(entry.Timestamp) > mailCheckCacheTTL {
		return nil
	}
	return &entry
}

// saveMailCheckCache writes a mail check entry to the cache.
// Errors are silently ignored since caching is a best-effort optimization.
func saveMailCheckCache(entry *mailCheckCacheEntry) {
	path := mailCheckCachePath(entry.Address)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0600)
}

func runMailCheck(cmd *cobra.Command, args []string) error {
	// Determine which inbox (priority: --identity flag, auto-detect)
	address := ""
	if mailCheckIdentity != "" {
		address = mailCheckIdentity
	} else {
		address = detectSender()
	}

	// All mail uses town beads (two-level architecture)
	workDir, err := findMailWorkDir()
	if err != nil {
		if mailCheckInject {
			// Inject mode: always exit 0, silent on error
			return nil
		}
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Get mailbox
	router := mail.NewRouter(workDir)
	mailbox, err := router.GetMailbox(address)
	if err != nil {
		if mailCheckInject {
			return nil
		}
		return fmt.Errorf("getting mailbox: %w", err)
	}

	// Count unread
	_, unread, err := mailbox.Count()
	if err != nil {
		if mailCheckInject {
			return nil
		}
		return fmt.Errorf("counting messages: %w", err)
	}

	// JSON output
	if mailCheckJSON {
		result := map[string]interface{}{
			"address": address,
			"unread":  unread,
			"has_new": unread > 0,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	// Inject mode: output system-reminder if mail exists
	if mailCheckInject {
		if unread > 0 {
			// Get subjects for context
			messages, _ := mailbox.ListUnread()
			var subjects []string
			for _, msg := range messages {
				subjects = append(subjects, fmt.Sprintf("- %s from %s: %s", msg.ID, msg.From, msg.Subject))
			}

			fmt.Println("<system-reminder>")
			fmt.Printf("You have %d unread message(s) in your inbox.\n\n", unread)
			for _, s := range subjects {
				fmt.Println(s)
			}
			fmt.Println()
			fmt.Println("Run 'gt mail inbox' to see your messages, or 'gt mail read <id>' for a specific message.")
			fmt.Println("</system-reminder>")
		}
		return nil
	}

	// Normal mode
	if unread > 0 {
		fmt.Printf("%s %d unread message(s)\n", style.Bold.Render("📬"), unread)
		return NewSilentExit(0)
	}
	fmt.Println("No new mail")
	return NewSilentExit(1)
}
