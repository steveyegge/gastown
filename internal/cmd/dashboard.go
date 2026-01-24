package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/web"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	dashboardPort int
	dashboardOpen bool
	beadsUIPort   = 3131 // Internal port for beads-ui
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

Example:
  gt dashboard              # Start on default port 8080
  gt dashboard --port 3000  # Start on port 3000
  gt dashboard --open       # Start and open browser`,
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

	// Start beads-ui subprocess
	beadsUIProcess, beadsUIAvailable := startBeadsUI(townRoot)

	// Set beads-ui port on handler for direct iframe embedding
	if beadsUIAvailable {
		handler.SetBeadsUIPort(beadsUIPort)
	}

	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n   Shutting down...")
		if beadsUIProcess != nil {
			stopBeadsUI(beadsUIProcess)
		}
		cancel()
		os.Exit(0)
	}()

	// Set up routes
	mux := http.NewServeMux()
	mux.HandleFunc("/", handler.ServeHTTP)
	mux.HandleFunc("/polecat/", handler.ServePolecatDetail)
	mux.HandleFunc("/convoy/", handler.ServeConvoyDetail)
	mux.HandleFunc("/hq/", handler.ServeHQDetail)

	// No reverse proxy needed - iframe points directly to bdui port

	// Build the URL
	dashURL := fmt.Sprintf("http://localhost:%d", dashboardPort)

	// Open browser if requested
	if dashboardOpen {
		go openBrowser(dashURL)
	}

	// Start the server with timeouts
	fmt.Printf("🚚 Gas Town Dashboard starting at %s\n", dashURL)
	if beadsUIAvailable {
		fmt.Printf("   Beads UI available at %s/beads-ui/\n", dashURL)
	} else {
		fmt.Printf("   Beads UI not available (install with: npm install -g beads-ui)\n")
	}
	fmt.Printf("   Press Ctrl+C to stop\n")

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", dashboardPort),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Use context for graceful shutdown
	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	return server.ListenAndServe()
}

// startBeadsUI attempts to start the beads-ui server as a subprocess.
// Returns the process (for cleanup) and whether it was successfully started.
func startBeadsUI(townRoot string) (*exec.Cmd, bool) {
	// Check if bdui is available
	if _, err := exec.LookPath("bdui"); err != nil {
		return nil, false
	}

	// Start bdui on the internal port
	// #nosec G204 -- bdui is a trusted CLI tool
	cmd := exec.Command("bdui", "start", "--port", fmt.Sprintf("%d", beadsUIPort))
	cmd.Dir = townRoot
	cmd.Stdout = nil // Suppress output
	cmd.Stderr = nil

	// Start in a new process group so we can kill it cleanly
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		fmt.Printf("   Warning: Failed to start beads-ui: %v\n", err)
		return nil, false
	}

	// Give it a moment to start
	time.Sleep(500 * time.Millisecond)

	// Check if it's still running
	if cmd.Process == nil {
		return nil, false
	}

	return cmd, true
}

// stopBeadsUI gracefully stops the beads-ui subprocess.
func stopBeadsUI(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}

	// Kill the process group to ensure all child processes are terminated
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err == nil {
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
	} else {
		_ = cmd.Process.Kill()
	}

	// Wait for process to exit (with timeout)
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-done:
		// Process exited
	case <-time.After(2 * time.Second):
		// Force kill if still running
		if pgid, err := syscall.Getpgid(cmd.Process.Pid); err == nil {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		}
	}
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
