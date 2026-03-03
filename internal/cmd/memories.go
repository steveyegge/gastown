package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
)

func init() {
	memoriesCmd.GroupID = GroupWork
	rootCmd.AddCommand(memoriesCmd)
}

var memoriesCmd = &cobra.Command{
	Use:   "memories [search-term]",
	Short: "List or search stored memories",
	Long: `List or search memories stored in the beads key-value store.

Without arguments, lists all memories. With a search term, filters
memories whose key or value contains the term (case-insensitive).

Examples:
  gt memories                    # List all memories
  gt memories refinery           # Search for memories about refinery
  gt memories "worktree"         # Search for worktree-related memories`,
	Args: cobra.MaximumNArgs(1),
	RunE: runMemories,
}

func runMemories(cmd *cobra.Command, args []string) error {
	kvs, err := bdKvListJSON()
	if err != nil {
		return fmt.Errorf("listing memories: %w", err)
	}

	var search string
	if len(args) > 0 {
		search = strings.ToLower(args[0])
	}

	// Filter for memory.* keys and optional search
	type memory struct {
		key   string
		value string
	}
	var memories []memory

	for k, v := range kvs {
		if !strings.HasPrefix(k, memoryKeyPrefix) {
			continue
		}
		shortKey := strings.TrimPrefix(k, memoryKeyPrefix)

		if search != "" {
			if !strings.Contains(strings.ToLower(shortKey), search) &&
				!strings.Contains(strings.ToLower(v), search) {
				continue
			}
		}

		memories = append(memories, memory{key: shortKey, value: v})
	}

	sort.Slice(memories, func(i, j int) bool {
		return memories[i].key < memories[j].key
	})

	if len(memories) == 0 {
		if search != "" {
			fmt.Printf("No memories matching %q\n", search)
		} else {
			fmt.Println("No memories stored. Use 'gt remember \"insight\"' to add one.")
		}
		return nil
	}

	header := "Memories"
	if search != "" {
		header = fmt.Sprintf("Memories matching %q", search)
	}
	fmt.Printf("%s (%d):\n\n", style.Bold.Render(header), len(memories))

	for _, m := range memories {
		fmt.Printf("  %s\n", style.Bold.Render(m.key))
		// Wrap long values for readability
		fmt.Printf("    %s\n\n", m.value)
	}

	return nil
}
