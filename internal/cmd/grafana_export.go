package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

const (
	defaultGrafanaURL  = "http://localhost:9429"
	defaultGrafanaUser = "admin"
	defaultGrafanaPass = "admin"
	grafanaHTTPTimeout = 10 * time.Second
)

var (
	grafanaExportDryRun    bool
	grafanaExportDashboard string
	grafanaExportURL       string
)

var grafanaExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export dashboards from Grafana to provisioning JSON files",
	Long: `Export all dashboards from Grafana's database to provisioned JSON files on disk.

Grafana stores UI edits in its SQLite database, but provisions dashboards
from JSON files on startup. This command syncs the database state back to
the JSON files so they can be committed to git.

Without this, container restarts overwrite all UI work with stale files.

Examples:
  gt grafana export                    # Export all dashboards
  gt grafana export --dry-run          # Preview what would change
  gt grafana export --dashboard gastown-overview  # Export one dashboard`,
	RunE: runGrafanaExport,
}

func init() {
	grafanaExportCmd.Flags().BoolVar(&grafanaExportDryRun, "dry-run", false, "Show what would change without writing files")
	grafanaExportCmd.Flags().StringVar(&grafanaExportDashboard, "dashboard", "", "Export a single dashboard by UID")
	grafanaExportCmd.Flags().StringVar(&grafanaExportURL, "url", defaultGrafanaURL, "Grafana base URL")
	grafanaCmd.AddCommand(grafanaExportCmd)
}

// grafanaSearchResult represents a dashboard from the Grafana search API.
type grafanaSearchResult struct {
	UID   string `json:"uid"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

// grafanaDashboardResponse is the envelope from GET /api/dashboards/uid/{uid}.
type grafanaDashboardResponse struct {
	Dashboard json.RawMessage        `json:"dashboard"`
	Meta      map[string]interface{} `json:"meta"`
}

func runGrafanaExport(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	dashDir := filepath.Join(townRoot, "sfgastown", "mayor", "rig", "opentelemetry",
		"grafana", "provisioning", "dashboards")

	if _, err := os.Stat(dashDir); os.IsNotExist(err) {
		return fmt.Errorf("dashboard directory not found: %s", dashDir)
	}

	client := &http.Client{Timeout: grafanaHTTPTimeout}

	// List dashboards
	dashboards, err := grafanaListDashboards(client, grafanaExportURL)
	if err != nil {
		return fmt.Errorf("failed to list dashboards: %w", err)
	}

	// Filter to single dashboard if requested
	if grafanaExportDashboard != "" {
		var filtered []grafanaSearchResult
		for _, d := range dashboards {
			if d.UID == grafanaExportDashboard {
				filtered = append(filtered, d)
			}
		}
		if len(filtered) == 0 {
			return fmt.Errorf("dashboard %q not found", grafanaExportDashboard)
		}
		dashboards = filtered
	}

	if grafanaExportDryRun {
		fmt.Printf("%s Dry run — listing %d dashboards:\n", style.Bold.Render("●"), len(dashboards))
	} else {
		fmt.Printf("%s Exporting %d dashboards...\n", style.Bold.Render("●"), len(dashboards))
	}

	var updated, unchanged, created int

	for _, d := range dashboards {
		raw, err := grafanaGetDashboard(client, grafanaExportURL, d.UID)
		if err != nil {
			fmt.Printf("  %s %s: fetch failed: %v\n", style.Warning.Render("!"), d.Title, err)
			continue
		}

		normalized, err := normalizeDashboardJSON(raw)
		if err != nil {
			fmt.Printf("  %s %s: normalize failed: %v\n", style.Warning.Render("!"), d.Title, err)
			continue
		}

		filename := d.UID + ".json"
		outPath := filepath.Join(dashDir, filename)

		status := classifyChange(outPath, normalized)

		switch status {
		case "unchanged":
			unchanged++
			if grafanaExportDryRun {
				fmt.Printf("  %s %s (unchanged)\n", style.Bold.Render("·"), filename)
			}
		case "updated":
			updated++
			fmt.Printf("  %s %s (updated)\n", style.Bold.Render("✓"), filename)
		case "new":
			created++
			fmt.Printf("  %s %s (new)\n", style.Bold.Render("+"), filename)
		}

		if !grafanaExportDryRun && status != "unchanged" {
			if err := os.WriteFile(outPath, normalized, 0644); err != nil {
				fmt.Printf("  %s %s: write failed: %v\n", style.Warning.Render("!"), filename, err)
			}
		}
	}

	fmt.Println()
	if grafanaExportDryRun {
		fmt.Printf("Dry run complete: %d would update, %d new, %d unchanged\n",
			updated, created, unchanged)
	} else {
		fmt.Printf("Exported %d dashboards to %s\n", updated+created, shortenPath(dashDir, townRoot))
		if updated+created > 0 {
			relDir := shortenPath(dashDir, townRoot)
			fmt.Printf("To commit: git add %s/*.json && git commit -m \"grafana: export dashboards\"\n", relDir)
		}
	}

	return nil
}

// grafanaListDashboards calls the Grafana search API.
func grafanaListDashboards(client *http.Client, baseURL string) ([]grafanaSearchResult, error) {
	req, err := http.NewRequest("GET", baseURL+"/api/search?type=dash-db", nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(defaultGrafanaUser, defaultGrafanaPass)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var results []grafanaSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, err
	}

	// Sort by UID for deterministic output
	sort.Slice(results, func(i, j int) bool {
		return results[i].UID < results[j].UID
	})

	return results, nil
}

// grafanaGetDashboard fetches a single dashboard by UID and returns the raw dashboard JSON.
func grafanaGetDashboard(client *http.Client, baseURL, uid string) (json.RawMessage, error) {
	req, err := http.NewRequest("GET", baseURL+"/api/dashboards/uid/"+uid, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(defaultGrafanaUser, defaultGrafanaPass)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var envelope grafanaDashboardResponse
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, err
	}

	return envelope.Dashboard, nil
}

// normalizeDashboardJSON removes volatile fields and pretty-prints with sorted keys.
func normalizeDashboardJSON(raw json.RawMessage) ([]byte, error) {
	var dash map[string]interface{}
	if err := json.Unmarshal(raw, &dash); err != nil {
		return nil, err
	}

	// Remove fields that change on every save
	delete(dash, "id")
	delete(dash, "version")

	return marshalSorted(dash)
}

// marshalSorted produces deterministic JSON with sorted keys and 2-space indent.
func marshalSorted(v interface{}) ([]byte, error) {
	buf, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	// Ensure trailing newline
	if len(buf) > 0 && buf[len(buf)-1] != '\n' {
		buf = append(buf, '\n')
	}
	return buf, nil
}

// classifyChange compares new content against existing file.
func classifyChange(path string, newContent []byte) string {
	existing, err := os.ReadFile(path)
	if err != nil {
		return "new"
	}

	// Normalize existing file for comparison (remove volatile fields too)
	var existingDash map[string]interface{}
	if err := json.Unmarshal(existing, &existingDash); err != nil {
		return "updated" // can't parse existing, treat as updated
	}
	delete(existingDash, "id")
	delete(existingDash, "version")

	existingNorm, err := marshalSorted(existingDash)
	if err != nil {
		return "updated"
	}

	if string(existingNorm) == string(newContent) {
		return "unchanged"
	}
	return "updated"
}

// shortenPath returns a relative path from townRoot if possible.
func shortenPath(path, townRoot string) string {
	rel, err := filepath.Rel(townRoot, path)
	if err != nil {
		return path
	}
	if strings.HasPrefix(rel, "..") {
		return path
	}
	return rel
}
