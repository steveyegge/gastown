package cmd

import (
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/web"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	dashboardPort int
	dashboardOpen bool
)

var dashboardCmd = &cobra.Command{
	Use:     "dashboard",
	GroupID: GroupDiag,
	Short:   "Start the convoy tracking web dashboard",
	Long: `Start a web server that displays the convoy tracking dashboard.

The dashboard shows real-time convoy status with:
- Convoy list with status indicators
- Progress tracking for each convoy
- Last activity indicator (green/yellow/red)
- Auto-refresh every 30 seconds via htmx

Endpoints:
  /        Convoy dashboard (HTML)
  /feed    Server-Sent Events (SSE) stream of real-time activity
           Streams events from .events.jsonl and bd activity

Example:
  gt dashboard              # Start on default port 8080
  gt dashboard --port 3000  # Start on port 3000
  gt dashboard --open       # Start and open browser

SSE Feed usage:
  curl -N http://localhost:8080/feed
  # Or in JavaScript:
  const es = new EventSource('http://localhost:8080/feed');
  es.onmessage = e => console.log(JSON.parse(e.data));`,
	RunE: runDashboard,
}

func init() {
	dashboardCmd.Flags().IntVar(&dashboardPort, "port", 8080, "HTTP port to listen on")
	dashboardCmd.Flags().BoolVar(&dashboardOpen, "open", false, "Open browser automatically")
	rootCmd.AddCommand(dashboardCmd)
}

func runDashboard(cmd *cobra.Command, args []string) error {
	// Verify we're in a workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Create the live convoy fetcher
	fetcher, err := web.NewLiveConvoyFetcher()
	if err != nil {
		return fmt.Errorf("creating convoy fetcher: %w", err)
	}

	// Create the handler
	handler, err := web.NewConvoyHandler(fetcher)
	if err != nil {
		return fmt.Errorf("creating convoy handler: %w", err)
	}

	// Set town root for SSE feed endpoint
	handler.SetTownRoot(townRoot)

	// Set up routing
	mux := http.NewServeMux()
	mux.Handle("/", handler)
	mux.HandleFunc("/polecat/", func(w http.ResponseWriter, r *http.Request) {
		// Parse /polecat/{rig}/{name} from URL path
		path := r.URL.Path[len("/polecat/"):]
		parts := splitPath(path)
		if len(parts) != 2 {
			http.Error(w, "Invalid polecat path, expected /polecat/{rig}/{name}", http.StatusBadRequest)
			return
		}
		handler.ServePolecatDetail(w, r, parts[0], parts[1])
	})

	// Build the URL
	url := fmt.Sprintf("http://localhost:%d", dashboardPort)

	// Open browser if requested
	if dashboardOpen {
		go openBrowser(url)
	}

	// Start the server with timeouts
	fmt.Printf("🚚 Gas Town Dashboard starting at %s\n", url)
	fmt.Printf("   Press Ctrl+C to stop\n")

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", dashboardPort),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return server.ListenAndServe()
}

// splitPath splits a URL path into its components, filtering empty strings.
func splitPath(path string) []string {
	var parts []string
	for _, p := range splitBySlash(path) {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

// splitBySlash splits a string by "/" without using strings package.
func splitBySlash(s string) []string {
	var result []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '/' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	return result
}

// openBrowser opens the specified URL in the default browser.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return
	}
	_ = cmd.Start()
}
