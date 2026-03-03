package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
)

func init() {
	forgetCmd.GroupID = GroupWork
	rootCmd.AddCommand(forgetCmd)
}

var forgetCmd = &cobra.Command{
	Use:   "forget <key>",
	Short: "Remove a stored memory",
	Long: `Remove a memory from the beads key-value store.

The key should match the short name shown by 'gt memories'
(without the memory. prefix).

Examples:
  gt forget refinery-worktree
  gt forget hooks-package-structure`,
	Args: cobra.ExactArgs(1),
	RunE: runForget,
}

func runForget(cmd *cobra.Command, args []string) error {
	key := args[0]

	// Strip memory. prefix if the user included it
	key = strings.TrimPrefix(key, memoryKeyPrefix)

	fullKey := memoryKeyPrefix + key

	// Check it exists
	existing, err := bdKvGet(fullKey)
	if err != nil || existing == "" {
		return fmt.Errorf("memory %q not found", key)
	}

	if err := bdKvClear(fullKey); err != nil {
		return fmt.Errorf("removing memory: %w", err)
	}

	fmt.Printf("%s Forgot memory: %s\n", style.Success.Render("✓"), style.Bold.Render(key))
	return nil
}
