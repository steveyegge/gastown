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
	servePort int
	serveTown string
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the Gas Town HTTP API server",
	Long: `Start an HTTP API server that exposes Gas Town functionality via REST API.

This allows web applications and other services to interact with Gas Town
programmatically without shelling out to CLI commands.

Endpoints:
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

Examples:
  gt serve                        # Start on default port 8080
  gt serve --port 3000            # Start on port 3000
  gt serve --town ~/my-town       # Specify town root`,
	RunE: runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)

	serveCmd.Flags().IntVar(&servePort, "port", 8080, "Port to listen on")
	serveCmd.Flags().StringVar(&serveTown, "town", "", "Town root directory (default: auto-detect)")
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

	// Create and start server
	server := api.NewServer(townRoot, servePort)

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
