package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
)

const memoryKeyPrefix = "memory."

var rememberKey string

func init() {
	rememberCmd.Flags().StringVar(&rememberKey, "key", "", "Explicit key slug (default: auto-generated from content)")
	rememberCmd.GroupID = GroupWork
	rootCmd.AddCommand(rememberCmd)
}

var rememberCmd = &cobra.Command{
	Use:   `remember "insight"`,
	Short: "Store a persistent memory",
	Long: `Store a persistent memory in the beads key-value store.

Memories persist across sessions and are injected during gt prime.
This replaces filesystem-based MEMORY.md with bead-backed storage.

The key is auto-generated from the content if not specified.
Use --key to provide an explicit slug for easy retrieval.

Examples:
  gt remember "Refinery uses worktree, cannot checkout main"
  gt remember --key refinery-worktree "Refinery uses worktree, cannot checkout main"
  gt remember "Always use --stdin for multi-line mail"`,
	Args: cobra.ExactArgs(1),
	RunE: runRemember,
}

func runRemember(cmd *cobra.Command, args []string) error {
	content := args[0]
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("memory content cannot be empty")
	}

	key := rememberKey
	if key == "" {
		key = autoKey(content)
	}

	// Sanitize key: lowercase, hyphens instead of spaces, strip dots
	key = sanitizeKey(key)

	fullKey := memoryKeyPrefix + key

	// Check if key already exists
	existing, _ := bdKvGet(fullKey)
	verb := "Stored"
	if existing != "" {
		verb = "Updated"
	}

	if err := bdKvSet(fullKey, content); err != nil {
		return fmt.Errorf("storing memory: %w", err)
	}

	fmt.Printf("%s %s memory: %s\n", style.Success.Render("✓"), verb, style.Bold.Render(key))
	return nil
}

// autoKey generates a short key from content using first few meaningful words.
func autoKey(content string) string {
	// Take first ~5 words, lowercase, hyphenate
	words := strings.Fields(strings.ToLower(content))
	if len(words) > 5 {
		words = words[:5]
	}

	// Strip non-alphanumeric chars from each word
	var clean []string
	for _, w := range words {
		w = strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				return r
			}
			return -1
		}, w)
		if w != "" {
			clean = append(clean, w)
		}
	}

	if len(clean) == 0 {
		// Fallback to hash
		h := sha256.Sum256([]byte(content))
		return hex.EncodeToString(h[:4])
	}

	slug := strings.Join(clean, "-")
	// Cap length
	if len(slug) > 40 {
		slug = slug[:40]
	}
	return slug
}

// sanitizeKey normalizes a key slug.
func sanitizeKey(key string) string {
	key = strings.ToLower(key)
	key = strings.ReplaceAll(key, " ", "-")
	key = strings.ReplaceAll(key, ".", "-")

	// Strip anything that isn't alphanumeric or hyphen
	key = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return -1
	}, key)

	// Collapse multiple hyphens
	for strings.Contains(key, "--") {
		key = strings.ReplaceAll(key, "--", "-")
	}
	key = strings.Trim(key, "-")

	return key
}

// bdKvSet calls bd kv set <key> <value>.
func bdKvSet(key, value string) error {
	cmd := exec.Command("bd", "kv", "set", key, value)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// bdKvGet calls bd kv get <key> and returns the value.
func bdKvGet(key string) (string, error) {
	cmd := exec.Command("bd", "kv", "get", key)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// bdKvClear calls bd kv clear <key>.
func bdKvClear(key string) error {
	cmd := exec.Command("bd", "kv", "clear", key)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// bdKvListJSON calls bd kv list --json and returns the parsed map.
func bdKvListJSON() (map[string]string, error) {
	cmd := exec.Command("bd", "kv", "list", "--json")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var kvs map[string]string
	if err := json.Unmarshal(out, &kvs); err != nil {
		return nil, fmt.Errorf("parsing kv list: %w", err)
	}
	return kvs, nil
}
