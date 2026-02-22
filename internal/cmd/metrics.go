package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	metricsByActor bool
	metricsSince   int
	metricsDead    bool
)

func init() {
	metricsCmd.Flags().BoolVar(&metricsByActor, "by-actor", false, "Show breakdown by actor")
	metricsCmd.Flags().IntVar(&metricsSince, "since", 0, "Only show data from last N days")
	metricsCmd.Flags().BoolVar(&metricsDead, "dead", false, "Show commands defined but never invoked")

	rootCmd.AddCommand(metricsCmd)
}

var metricsCmd = &cobra.Command{
	Use:     "metrics",
	GroupID: GroupDiag,
	Short:   "Show command usage statistics",
	Long: `Reads ~/.gt/cmd-usage.jsonl and reports which gt commands are used,
how often, and by whom. Helps identify dead commands before pruning.`,
	RunE: runMetrics,
}

type usageEntry struct {
	Ts    string `json:"ts"`
	Cmd   string `json:"cmd"`
	Actor string `json:"actor"`
	Argc  int    `json:"argc"`
}

func runMetrics(cmd *cobra.Command, args []string) error {
	entries, err := readUsageLog()
	if err != nil {
		return err
	}

	// Filter by --since
	if metricsSince > 0 {
		cutoff := time.Now().AddDate(0, 0, -metricsSince)
		var filtered []usageEntry
		for _, e := range entries {
			if t, err := time.Parse(time.RFC3339, e.Ts); err == nil && t.After(cutoff) {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	if metricsDead {
		return showDeadCommands(entries)
	}
	if metricsByActor {
		return showByActor(entries)
	}
	return showFrequency(entries)
}

func readUsageLog() ([]usageEntry, error) {
	f, err := os.Open(logUsagePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no usage data yet (run some gt commands first)")
		}
		return nil, err
	}
	defer f.Close()

	var entries []usageEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e usageEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue // skip malformed lines
		}
		entries = append(entries, e)
	}
	return entries, scanner.Err()
}

// showFrequency prints commands sorted by invocation count.
func showFrequency(entries []usageEntry) error {
	if len(entries) == 0 {
		fmt.Println("No usage data.")
		return nil
	}

	counts := map[string]int{}
	lastSeen := map[string]string{}
	for _, e := range entries {
		counts[e.Cmd]++
		if e.Ts > lastSeen[e.Cmd] {
			lastSeen[e.Cmd] = e.Ts
		}
	}

	type row struct {
		cmd   string
		count int
		last  string
	}
	var rows []row
	for cmd, count := range counts {
		last := lastSeen[cmd]
		if t, err := time.Parse(time.RFC3339, last); err == nil {
			last = t.Format("2006-01-02")
		}
		rows = append(rows, row{cmd, count, last})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].count > rows[j].count })

	fmt.Printf("%-35s %6s  %s\n", "COMMAND", "COUNT", "LAST USED")
	fmt.Printf("%-35s %6s  %s\n", strings.Repeat("─", 35), strings.Repeat("─", 6), strings.Repeat("─", 10))
	for _, r := range rows {
		fmt.Printf("%-35s %6d  %s\n", r.cmd, r.count, r.last)
	}
	fmt.Printf("\nTotal: %d invocations across %d commands\n", len(entries), len(counts))
	return nil
}

// showByActor prints usage broken down by actor.
func showByActor(entries []usageEntry) error {
	if len(entries) == 0 {
		fmt.Println("No usage data.")
		return nil
	}

	// actor -> cmd -> count
	actors := map[string]map[string]int{}
	for _, e := range entries {
		if actors[e.Actor] == nil {
			actors[e.Actor] = map[string]int{}
		}
		actors[e.Actor][e.Cmd]++
	}

	// Sort actors alphabetically
	var actorNames []string
	for a := range actors {
		actorNames = append(actorNames, a)
	}
	sort.Strings(actorNames)

	for _, actor := range actorNames {
		cmds := actors[actor]
		type row struct {
			cmd   string
			count int
		}
		var rows []row
		total := 0
		for cmd, count := range cmds {
			rows = append(rows, row{cmd, count})
			total += count
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].count > rows[j].count })

		fmt.Printf("\n%s (%d invocations)\n", actor, total)
		fmt.Printf("  %-33s %6s\n", "COMMAND", "COUNT")
		for _, r := range rows {
			fmt.Printf("  %-33s %6d\n", r.cmd, r.count)
		}
	}
	return nil
}

// showDeadCommands finds commands registered in the binary but absent from the log.
func showDeadCommands(entries []usageEntry) error {
	// Collect all invoked commands
	invoked := map[string]bool{}
	for _, e := range entries {
		invoked[e.Cmd] = true
	}

	// Walk the command tree to find all registered commands
	var registered []string
	walkCommands(rootCmd, &registered)

	// Find dead commands
	var dead []string
	for _, cmd := range registered {
		if !invoked[cmd] {
			dead = append(dead, cmd)
		}
	}

	sort.Strings(dead)

	if len(dead) == 0 {
		fmt.Println("No dead commands — every registered command has been invoked.")
		return nil
	}

	fmt.Printf("Dead commands (%d registered, never invoked):\n\n", len(dead))
	for _, cmd := range dead {
		fmt.Printf("  %s\n", cmd)
	}
	fmt.Printf("\nTotal: %d dead out of %d registered commands\n", len(dead), len(registered))
	return nil
}

// walkCommands recursively collects all runnable command paths.
func walkCommands(cmd *cobra.Command, paths *[]string) {
	// Include if it has a Run/RunE handler (leaf command)
	if cmd.Run != nil || cmd.RunE != nil {
		path := buildCommandPath(cmd)
		*paths = append(*paths, path)
	}
	for _, sub := range cmd.Commands() {
		if sub.IsAvailableCommand() {
			walkCommands(sub, paths)
		}
	}
}
