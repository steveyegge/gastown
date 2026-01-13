package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var readyJSON bool
var readyRig string

var readyCmd = &cobra.Command{
	Use:     "ready",
	GroupID: GroupWork,
	Short:   "Show work ready across town",
	Long: `Display all ready work items across the town and all rigs.

Aggregates ready issues from:
- Town beads (hq-* items: convoys, cross-rig coordination)
- Each rig's beads (project-level issues, MRs)

Ready items have no blockers and can be worked immediately.
Results are sorted by priority (highest first) then by source.

Examples:
  gt ready              # Show all ready work
  gt ready --json       # Output as JSON
  gt ready --rig=gastown  # Show only one rig`,
	RunE: runReady,
}

func init() {
	readyCmd.Flags().BoolVar(&readyJSON, "json", false, "Output as JSON")
	readyCmd.Flags().StringVar(&readyRig, "rig", "", "Filter to a specific rig")
	rootCmd.AddCommand(readyCmd)
}

// ReadySource represents ready items from a single source (town or rig).
type ReadySource struct {
	Name   string         `json:"name"`   // "town" or rig name
	Issues []*beads.Issue `json:"issues"` // Ready issues from this source
	Error  string         `json:"error,omitempty"`
}

// ReadyResult is the aggregated result of gt ready.
type ReadyResult struct {
	Sources  []ReadySource `json:"sources"`
	Summary  ReadySummary  `json:"summary"`
	TownRoot string        `json:"town_root,omitempty"`
}

// ReadySummary provides counts for the ready report.
type ReadySummary struct {
	Total    int            `json:"total"`
	BySource map[string]int `json:"by_source"`
	P0Count  int            `json:"p0_count"`
	P1Count  int            `json:"p1_count"`
	P2Count  int            `json:"p2_count"`
	P3Count  int            `json:"p3_count"`
	P4Count  int            `json:"p4_count"`
}

func runReady(cmd *cobra.Command, args []string) error {
	// Find town root
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Load rigs config
	rigsConfigPath := constants.MayorRigsPath(townRoot)
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	// Create rig manager and discover rigs
	g := git.NewGit(townRoot)
	mgr := rig.NewManager(townRoot, rigsConfig, g)
	rigs, err := mgr.DiscoverRigs()
	if err != nil {
		return fmt.Errorf("discovering rigs: %w", err)
	}

	// Filter rigs if --rig flag provided
	if readyRig != "" {
		var filtered []*rig.Rig
		for _, r := range rigs {
			if r.Name == readyRig {
				filtered = append(filtered, r)
				break
			}
		}
		if len(filtered) == 0 {
			return fmt.Errorf("rig not found: %s", readyRig)
		}
		rigs = filtered
	}

	// Collect results from all sources in parallel
	var wg sync.WaitGroup
	var mu sync.Mutex
	sources := make([]ReadySource, 0, len(rigs)+1)

	// Fetch town beads (only if not filtering to a specific rig)
	if readyRig == "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			townBeadsPath := beads.GetTownBeadsPath(townRoot)
			townBeads := beads.New(townBeadsPath)
			issues, err := townBeads.Ready()

			mu.Lock()
			defer mu.Unlock()
			src := ReadySource{Name: "town"}
			if err != nil {
				src.Error = err.Error()
			} else {
				src.Issues = issues
			}
			sources = append(sources, src)
		}()
	}

	// Fetch from each rig in parallel
	for _, r := range rigs {
		wg.Add(1)
		go func(r *rig.Rig) {
			defer wg.Done()
			// Use mayor/rig path where rig-level beads are stored
			rigBeadsPath := constants.RigMayorPath(r.Path)
			rigBeads := beads.New(rigBeadsPath)
			issues, err := rigBeads.Ready()

			mu.Lock()
			defer mu.Unlock()
			src := ReadySource{Name: r.Name}
			if err != nil {
				src.Error = err.Error()
			} else {
				src.Issues = issues
			}
			sources = append(sources, src)
		}(r)
	}

	wg.Wait()

	// Sort sources: town first, then rigs alphabetically
	sort.Slice(sources, func(i, j int) bool {
		if sources[i].Name == "town" {
			return true
		}
		if sources[j].Name == "town" {
			return false
		}
		return sources[i].Name < sources[j].Name
	})

	// Sort issues within each source by priority (lower number = higher priority)
	for i := range sources {
		sort.Slice(sources[i].Issues, func(a, b int) bool {
			return sources[i].Issues[a].Priority < sources[i].Issues[b].Priority
		})
	}

	// Build summary
	summary := ReadySummary{
		BySource: make(map[string]int),
	}
	for _, src := range sources {
		count := len(src.Issues)
		summary.Total += count
		summary.BySource[src.Name] = count
		for _, issue := range src.Issues {
			switch issue.Priority {
			case 0:
				summary.P0Count++
			case 1:
				summary.P1Count++
			case 2:
				summary.P2Count++
			case 3:
				summary.P3Count++
			case 4:
				summary.P4Count++
			}
		}
	}

	result := ReadyResult{
		Sources:  sources,
		Summary:  summary,
		TownRoot: townRoot,
	}

	// Output
	if readyJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	return printReadyHuman(result)
}

func printReadyHuman(result ReadyResult) error {
	if result.Summary.Total == 0 {
		fmt.Println("No ready work across town.")
		return nil
	}

	fmt.Printf("%s Ready work across town:\n\n", style.Bold.Render("📋"))

	for _, src := range result.Sources {
		if src.Error != "" {
			fmt.Printf("%s %s\n", style.Dim.Render(src.Name+"/"), style.Warning.Render("(error: "+src.Error+")"))
			continue
		}

		count := len(src.Issues)
		if count == 0 {
			fmt.Printf("%s %s\n", style.Dim.Render(src.Name+"/"), style.Dim.Render("(none)"))
			continue
		}

		fmt.Printf("%s (%d items)\n", style.Bold.Render(src.Name+"/"), count)
		for _, issue := range src.Issues {
			priorityStr := fmt.Sprintf("P%d", issue.Priority)
			var priorityStyled string
			switch issue.Priority {
			case 0:
				priorityStyled = style.Error.Render(priorityStr) // P0 is critical
			case 1:
				priorityStyled = style.Error.Render(priorityStr)
			case 2:
				priorityStyled = style.Warning.Render(priorityStr)
			default:
				priorityStyled = style.Dim.Render(priorityStr)
			}

			// Truncate title if too long
			title := issue.Title
			if len(title) > 60 {
				title = title[:57] + "..."
			}

			fmt.Printf("  [%s] %s %s\n", priorityStyled, style.Dim.Render(issue.ID), title)
		}
		fmt.Println()
	}

	// Summary line
	parts := []string{}
	if result.Summary.P0Count > 0 {
		parts = append(parts, fmt.Sprintf("%d P0", result.Summary.P0Count))
	}
	if result.Summary.P1Count > 0 {
		parts = append(parts, fmt.Sprintf("%d P1", result.Summary.P1Count))
	}
	if result.Summary.P2Count > 0 {
		parts = append(parts, fmt.Sprintf("%d P2", result.Summary.P2Count))
	}
	if result.Summary.P3Count > 0 {
		parts = append(parts, fmt.Sprintf("%d P3", result.Summary.P3Count))
	}
	if result.Summary.P4Count > 0 {
		parts = append(parts, fmt.Sprintf("%d P4", result.Summary.P4Count))
	}

	if len(parts) > 0 {
		fmt.Printf("Total: %d items ready (%s)\n", result.Summary.Total, strings.Join(parts, ", "))
	} else {
		fmt.Printf("Total: %d items ready\n", result.Summary.Total)
	}

	return nil
}
