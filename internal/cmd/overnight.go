package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var overnightRig string

var overnightCmd = &cobra.Command{
	Use:     "overnight",
	GroupID: GroupDiag,
	Short:   "Inspect overnight reliability incidents",
}

var overnightSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Show recovered and blocked overnight incidents",
	RunE:  runOvernightSummary,
}

func init() {
	overnightSummaryCmd.Flags().StringVar(&overnightRig, "rig", "", "Limit summary to a single rig")
	overnightCmd.AddCommand(overnightSummaryCmd)
	rootCmd.AddCommand(overnightCmd)
}

type overnightIncident struct {
	Rig         string
	Title       string
	Status      string
	CommitSHA   string
	Check       string
	WorkflowRun string
	StatusClass string
	Recovery    string
}

func runOvernightSummary(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	rigs := resolveOvernightRigs(townRoot, overnightRig)
	var recovered []overnightIncident
	var blocked []overnightIncident
	var human []overnightIncident

	for _, rigName := range rigs {
		bd := beads.New(filepath.Join(townRoot, rigName))
		issues, err := bd.List(beads.ListOptions{
			Label:  "gt:ci-failure",
			Status: "all",
			Limit:  0,
		})
		if err != nil {
			return fmt.Errorf("%s: list incidents: %w", rigName, err)
		}
		for _, issue := range issues {
			fields := parseIncidentFields(issue.Description)
			item := overnightIncident{
				Rig:         rigName,
				Title:       issue.Title,
				Status:      issue.Status,
				CommitSHA:   fields["commit_sha"],
				Check:       fields["check"],
				WorkflowRun: fields["workflow_run"],
				StatusClass: fields["status_class"],
				Recovery:    fields["last_recovery_result"],
			}
			switch {
			case issue.Status == "closed" && item.Recovery == "recovered":
				recovered = append(recovered, item)
			case item.StatusClass == "human-action-required":
				human = append(human, item)
			default:
				blocked = append(blocked, item)
			}
		}
	}

	sort.Slice(recovered, func(i, j int) bool { return recovered[i].Title < recovered[j].Title })
	sort.Slice(blocked, func(i, j int) bool { return blocked[i].Title < blocked[j].Title })
	sort.Slice(human, func(i, j int) bool { return human[i].Title < human[j].Title })

	fmt.Printf("%s Overnight Summary\n\n", style.Bold.Render("●"))
	printOvernightSection("Recovered Automatically", recovered)
	printOvernightSection("Still Blocked", blocked)
	printOvernightSection("Human Action Required", human)

	if len(recovered) == 0 && len(blocked) == 0 && len(human) == 0 {
		fmt.Println(style.Dim.Render("No overnight incidents found."))
	}
	return nil
}

func resolveOvernightRigs(townRoot, onlyRig string) []string {
	if onlyRig != "" {
		return []string{onlyRig}
	}
	data, err := os.ReadFile(filepath.Join(townRoot, "mayor", "rigs.json"))
	if err != nil {
		return nil
	}
	var raw struct {
		Rigs map[string]json.RawMessage `json:"rigs"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	names := make([]string, 0, len(raw.Rigs))
	for name := range raw.Rigs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func parseIncidentFields(description string) map[string]string {
	fields := make(map[string]string)
	for _, line := range strings.Split(description, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		fields[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return fields
}

func printOvernightSection(title string, incidents []overnightIncident) {
	fmt.Printf("%s\n", style.Bold.Render(title))
	if len(incidents) == 0 {
		fmt.Printf("  %s\n\n", style.Dim.Render("none"))
		return
	}
	for _, incident := range incidents {
		line := fmt.Sprintf("  - [%s] %s", incident.Rig, incident.Title)
		if incident.CommitSHA != "" {
			line += fmt.Sprintf(" (%s)", shortText(incident.CommitSHA, 8))
		}
		fmt.Println(line)
		if incident.Check != "" {
			fmt.Printf("    check: %s\n", incident.Check)
		}
		if incident.WorkflowRun != "" {
			fmt.Printf("    workflow: %s\n", incident.WorkflowRun)
		}
	}
	fmt.Println()
}

func shortText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
