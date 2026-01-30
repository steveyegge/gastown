// ABOUTME: HTTP API server command for Gas Town.
// ABOUTME: Starts an HTTP server exposing Gas Town functionality via REST API.

package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/api"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	servePort        int
	serveTown        string
	serveNoDashboard bool
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the Gas Town HTTP server (API + Dashboard)",
	Long: `Start an HTTP server that serves both the REST API and HTML dashboard.

By default, this serves:
  /                    HTML Dashboard (convoy tracking UI)
  /swagger/            Swagger API documentation
  /api/...             REST API endpoints

This allows web applications and other services to interact with Gas Town
programmatically without shelling out to CLI commands.

API Endpoints:
  GET  /health                    Health check
  GET  /api/rigs                  List all rigs
  GET  /api/rigs/{rig}            Get rig details
  POST /api/rigs/{rig}/jobs       Create a job (bead)
  GET  /api/rigs/{rig}/jobs       List jobs
  GET  /api/rigs/{rig}/jobs/{id}  Get job details
  POST /api/rigs/{rig}/sling      Dispatch work to polecat
  GET  /api/rigs/{rig}/mq         List merge queue
  GET  /api/rigs/{rig}/polecats   List polecats
  GET  /api/rigs/{rig}/refinery   Refinery status
  GET  /api/mayor                 Mayor status
  POST /api/mayor/start           Start mayor
  POST /api/mayor/stop            Stop mayor

Examples:
  gt serve                        # Start on default port 8080
  gt serve --port 3000            # Start on port 3000
  gt serve --town ~/my-town       # Specify town root
  gt serve --no-dashboard         # API only (no HTML dashboard)`,
	RunE: runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)

	serveCmd.Flags().IntVar(&servePort, "port", 8080, "Port to listen on")
	serveCmd.Flags().StringVar(&serveTown, "town", "", "Town root directory (default: auto-detect)")
	serveCmd.Flags().BoolVar(&serveNoDashboard, "no-dashboard", false, "Disable HTML dashboard (API only)")
}

func runServe(cmd *cobra.Command, args []string) error {
	// Determine town root
	townRoot := serveTown
	if townRoot == "" {
		// Try to auto-detect from current directory
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("could not get current directory: %w", err)
		}

		detected, err := workspace.Find(cwd)
		if err != nil || detected == "" {
			// Try home directory ~/gt
			homeDir, _ := os.UserHomeDir()
			defaultTown := homeDir + "/gt"
			if _, err := os.Stat(defaultTown); err == nil {
				townRoot = defaultTown
			} else {
				return fmt.Errorf("could not detect town root. Use --town flag or cd to a Gas Town workspace")
			}
		} else {
			townRoot = detected
		}
	}

	// Validate town root exists
	if _, err := os.Stat(townRoot); os.IsNotExist(err) {
		return fmt.Errorf("town root does not exist: %s", townRoot)
	}

	// Create and start server with options
	var opts []api.ServerOption
	if !serveNoDashboard {
		opts = append(opts, api.WithDashboard())
	}
	server := api.NewServer(townRoot, servePort, opts...)

	// Handle graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		fmt.Println("\n🛑 Shutting down server...")
		if err := server.Shutdown(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "Error during shutdown: %v\n", err)
		}
	}()

	// Start server (blocks until shutdown)
	if err := server.Start(); err != nil && err.Error() != "http: Server closed" {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}
