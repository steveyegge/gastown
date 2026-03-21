package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
)

var memoriesTypeFilter string
var memoriesScope string

func init() {
	memoriesCmd.Flags().StringVar(&memoriesTypeFilter, "type", "", "Filter by memory type: feedback, project, user, reference, general")
	memoriesCmd.Flags().StringVar(&memoriesScope, "scope", "all", "Which memories to show: local, city, or all (default: all)")
	memoriesCmd.GroupID = GroupWork
	rootCmd.AddCommand(memoriesCmd)
}

var memoriesCmd = &cobra.Command{
	Use:   "memories [search-term]",
	Short: "List or search stored memories",
	Long: `List or search memories stored in the beads key-value store.

Without arguments, lists all memories. With a search term, filters
memories whose key or value contains the term (case-insensitive).

Use --type to filter by memory category:
  feedback   Guidance or corrections from users
  project    Ongoing work context, goals, deadlines
  user       Info about the user's role and preferences
  reference  Pointers to external resources
  general    Uncategorized memories

Use --scope to filter by memory scope:
  local  Only memories stored in this rig's beads store
  city   Only town-wide memories (visible to all agents in the town)
  all    Both local and city memories (default)

Examples:
  gt memories                    # List all memories (local + city)
  gt memories --scope city       # Show only town-wide memories
  gt memories --type feedback    # Show only behavioral corrections
  gt memories refinery           # Search for memories about refinery`,
	Args: cobra.MaximumNArgs(1),
	RunE: runMemories,
}

func runMemories(cmd *cobra.Command, args []string) error {
	scope := strings.ToLower(strings.TrimSpace(memoriesScope))
	if scope == "" {
		scope = "all"
	}
	if scope != "local" && scope != "city" && scope != "all" {
		return fmt.Errorf("invalid scope %q — valid scopes: local, city, all", scope)
	}

	var search string
	if len(args) > 0 {
		search = strings.ToLower(args[0])
	}

	typeFilter := strings.ToLower(strings.TrimSpace(memoriesTypeFilter))
	if typeFilter != "" {
		if _, ok := validMemoryTypes[typeFilter]; !ok {
			return fmt.Errorf("invalid memory type %q — valid types: feedback, project, user, reference, general", typeFilter)
		}
	}

	type memory struct {
		memType  string
		shortKey string
		value    string
		isCity   bool
	}
	var memories []memory

	collectFrom := func(kvs map[string]string, isCity bool) {
		for k, v := range kvs {
			if !strings.HasPrefix(k, memoryKeyPrefix) {
				continue
			}
			memType, shortKey := parseMemoryKey(k)
			if typeFilter != "" && memType != typeFilter {
				continue
			}
			if search != "" {
				if !strings.Contains(strings.ToLower(shortKey), search) &&
					!strings.Contains(strings.ToLower(v), search) &&
					!strings.Contains(strings.ToLower(memType), search) {
					continue
				}
			}
			memories = append(memories, memory{memType: memType, shortKey: shortKey, value: v, isCity: isCity})
		}
	}

	if scope == memoryScopeLocal || scope == "all" {
		kvs, err := bdKvListJSON()
		if err != nil {
			return fmt.Errorf("listing local memories: %w", err)
		}
		collectFrom(kvs, false)
	}

	if scope == memoryScopeCity || scope == "all" {
		cityDB := cityBeadsPath()
		if cityDB != "" {
			kvs, err := bdKvListJSONDB(cityDB)
			if err == nil {
				collectFrom(kvs, true)
			}
			// Silently skip city memories if city beads are unreachable
		}
	}

	sort.Slice(memories, func(i, j int) bool {
		// City memories sort after local memories of the same type
		if memories[i].isCity != memories[j].isCity {
			return !memories[i].isCity
		}
		if memories[i].memType != memories[j].memType {
			return memTypeRank(memories[i].memType) < memTypeRank(memories[j].memType)
		}
		return memories[i].shortKey < memories[j].shortKey
	})

	if len(memories) == 0 {
		if search != "" {
			fmt.Printf("No memories matching %q\n", search)
		} else if typeFilter != "" {
			fmt.Printf("No %s memories stored.\n", typeFilter)
		} else {
			fmt.Println("No memories stored. Use 'gt remember \"insight\"' to add one.")
		}
		return nil
	}

	header := "Memories"
	if scope != "all" {
		header = fmt.Sprintf("Memories [%s]", scope)
	}
	if typeFilter != "" {
		header = fmt.Sprintf("%s [%s]", header, typeFilter)
	}
	if search != "" {
		header = fmt.Sprintf("%s matching %q", header, search)
	}
	fmt.Printf("%s (%d):\n\n", style.Bold.Render(header), len(memories))

	lastType := ""
	lastCity := false
	for i, m := range memories {
		typeChanged := m.memType != lastType
		scopeChanged := i > 0 && m.isCity != lastCity

		if typeChanged || scopeChanged {
			if lastType != "" {
				fmt.Println()
			}
			label := m.memType
			if m.isCity {
				label += " · city"
			}
			fmt.Printf("  %s\n", style.Dim.Render("["+label+"]"))
			lastType = m.memType
			lastCity = m.isCity
		}
		fmt.Printf("  %s\n", style.Bold.Render(m.shortKey))
		fmt.Printf("    %s\n\n", m.value)
	}

	return nil
}

// memTypeRank returns the sort order for a memory type (lower = first).
func memTypeRank(memType string) int {
	for i, t := range memoryTypeOrder {
		if t == memType {
			return i
		}
	}
	return len(memoryTypeOrder)
}
