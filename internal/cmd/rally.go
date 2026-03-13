package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/rally"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	rallySearchTags         []string
	rallySearchCodebaseType string
	rallySearchProfile      bool
	rallySearchJSON         bool
	rallySearchLimit        int
)

func init() {
	rootCmd.AddCommand(rallyCmd)
	rallyCmd.AddCommand(rallySearchCmd)
	rallyCmd.AddCommand(rallyLookupCmd)

	rallySearchCmd.Flags().StringSliceVar(&rallySearchTags, "tags", nil, "Filter by tags (comma-separated)")
	rallySearchCmd.Flags().StringVar(&rallySearchCodebaseType, "codebase-type", "", "Filter by codebase type (e.g. go-cobra, python-flask)")
	rallySearchCmd.Flags().BoolVar(&rallySearchProfile, "profile", false, "Auto-query from tavern-profile.yaml in current repo")
	rallySearchCmd.Flags().BoolVar(&rallySearchJSON, "json", false, "Output as JSON")
	rallySearchCmd.Flags().IntVar(&rallySearchLimit, "limit", 10, "Maximum results to show")
}

var rallyCmd = &cobra.Command{
	Use:     "rally",
	Short:   "Search and contribute to rally_tavern knowledge base",
	GroupID: GroupWork,
	RunE:    requireSubcommand,
	Long: `Search and contribute to the rally_tavern knowledge base.

Rally Tavern is a shared knowledge repository at $GT_ROOT/rally_tavern/.
If rally_tavern is not present, commands degrade gracefully.

Commands:
  search    Search knowledge by query, tags, or codebase type
  lookup    Look up knowledge by exact tag (agent self-serve)
  nominate  Nominate a knowledge contribution for review by the Barkeep
  report    Report an entry as stale, wrong, or improvable
  verify    Confirm an entry is still accurate (updates last_verified)`,
}

var rallySearchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search knowledge base by query, tags, or codebase type",
	Long: `Search the rally_tavern knowledge base.

Examples:
  gt rally search "dolt session management"
  gt rally search --tags=security,auth
  gt rally search --codebase-type=go-cobra
  gt rally search --profile`,
	RunE: func(cmd *cobra.Command, args []string) error {
		townRoot, err := workspace.FindFromCwdOrError()
		if err != nil {
			return err
		}

		idx, err := rally.LoadKnowledgeIndex(townRoot)
		if err != nil {
			return fmt.Errorf("loading knowledge index: %w", err)
		}
		if idx == nil {
			fmt.Fprintln(cmd.OutOrStdout(), "rally_tavern not found at "+townRoot+"/rally_tavern/ — no knowledge available")
			return nil
		}

		q := rally.SearchQuery{
			Tags:         rallySearchTags,
			CodebaseType: rallySearchCodebaseType,
		}

		if len(args) > 0 {
			q.Text = strings.Join(args, " ")
		}

		if rallySearchProfile {
			p, err := rally.LoadProfile(".")
			if err != nil {
				return fmt.Errorf("loading tavern-profile.yaml: %w", err)
			}
			if p != nil {
				pq := p.ToSearchQuery()
				// Merge profile query with explicit flags (flags take precedence).
				if q.Text == "" {
					q.Text = pq.Text
				}
				if len(q.Tags) == 0 {
					q.Tags = pq.Tags
				}
				if q.CodebaseType == "" {
					q.CodebaseType = pq.CodebaseType
				}
			}
		}

		results := idx.Search(q)

		if len(results) > rallySearchLimit {
			results = results[:rallySearchLimit]
		}

		if len(results) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No results found.")
			return nil
		}

		if rallySearchJSON {
			return printRallyJSON(cmd, results)
		}

		printRallyResults(cmd, results)
		return nil
	},
}

var rallyLookupCmd = &cobra.Command{
	Use:   "lookup <tag>",
	Short: "Look up knowledge entries by exact tag",
	Long: `Look up knowledge entries by exact tag match.

Designed for agent self-serve during implementation — concise output
suitable for inclusion in polecat context.

Example:
  gt rally lookup security
  gt rally lookup dolt`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		townRoot, err := workspace.FindFromCwdOrError()
		if err != nil {
			return err
		}

		idx, err := rally.LoadKnowledgeIndex(townRoot)
		if err != nil {
			return fmt.Errorf("loading knowledge index: %w", err)
		}
		if idx == nil {
			return nil // graceful degradation — no output
		}

		results := idx.Search(rally.SearchQuery{Tags: []string{args[0]}})
		if len(results) == 0 {
			return nil // silent — agents call this opportunistically
		}

		for _, r := range results {
			printRallyEntry(cmd, r, true /* compact */)
		}
		return nil
	},
}

func printRallyResults(cmd *cobra.Command, results []rally.KnowledgeEntry) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Found %d result(s):\n\n", len(results))
	for i, r := range results {
		fmt.Fprintf(out, "%d. ", i+1)
		printRallyEntry(cmd, r, false)
		if i < len(results)-1 {
			fmt.Fprintln(out)
		}
	}
}

func printRallyEntry(cmd *cobra.Command, r rally.KnowledgeEntry, compact bool) {
	out := cmd.OutOrStdout()

	title := r.Title
	if title == "" {
		title = r.ID
	}
	fmt.Fprintf(out, "[%s] %s\n", r.Kind, title)

	if r.Tags != nil {
		fmt.Fprintf(out, "  Tags: %s\n", strings.Join(r.Tags, ", "))
	}

	if compact {
		// For lookup: show summary or first sentence of details.
		text := r.Summary
		if text == "" {
			text = r.Lesson
		}
		if text == "" {
			text = r.Solution
		}
		if text != "" {
			// Trim to first paragraph.
			if idx := strings.Index(text, "\n\n"); idx > 0 {
				text = text[:idx]
			}
			fmt.Fprintf(out, "  %s\n", strings.TrimSpace(text))
		}
		return
	}

	// Full output.
	if r.Summary != "" {
		fmt.Fprintf(out, "  %s\n", strings.TrimSpace(r.Summary))
	}
	if r.Lesson != "" {
		fmt.Fprintf(out, "  Lesson: %s\n", strings.TrimSpace(r.Lesson))
	}
	if r.Solution != "" {
		text := r.Solution
		if idx := strings.Index(text, "\n\n"); idx > 0 {
			text = text[:idx]
		}
		fmt.Fprintf(out, "  Solution: %s\n", strings.TrimSpace(text))
	}
	if len(r.Gotchas) > 0 {
		fmt.Fprintf(out, "  Gotchas:\n")
		for _, g := range r.Gotchas {
			fmt.Fprintf(out, "    • %s\n", g)
		}
	}
}

func printRallyJSON(cmd *cobra.Command, results []rally.KnowledgeEntry) error {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "[")
	for i, r := range results {
		fmt.Fprintf(out, `  {"id":%q,"kind":%q,"title":%q,"tags":%s,"summary":%q}`,
			r.ID, r.Kind, r.Title,
			jsonStringSlice(r.Tags),
			strings.TrimSpace(r.Summary+r.Lesson+r.Solution),
		)
		if i < len(results)-1 {
			_, _ = fmt.Fprint(out, ",")
		}
		fmt.Fprintln(out)
	}
	fmt.Fprintln(out, "]")
	return nil
}

func jsonStringSlice(ss []string) string {
	if len(ss) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteString("[")
	for i, s := range ss {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, "%q", s)
	}
	b.WriteString("]")
	return b.String()
}
