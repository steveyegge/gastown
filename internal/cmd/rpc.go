package cmd

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/rpcserver"
	"github.com/steveyegge/gastown/internal/workspace"
)

var rpcCmd = &cobra.Command{
	Use:     "rpc",
	GroupID: GroupServices,
	Short:   "RPC server for mobile clients",
	Long: `Manage the Gas Town RPC server for mobile client access.

The RPC server exposes StatusService, MailService, and DecisionService
via Connect-RPC protocol for mobile apps and other clients.

Examples:
  gt rpc serve                    # Start RPC server on default port
  gt rpc serve --port 9443        # Start on custom port
  gt rpc serve --api-key secret   # Enable API key authentication`,
	RunE: requireSubcommand,
}

var rpcServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the RPC server",
	Long: `Start the Gas Town RPC server.

The server exposes:
  - StatusService: Town, rig, and agent status
  - MailService: Mailbox access for agents
  - DecisionService: Create, list, and resolve decisions

Endpoints:
  /gastown.v1.StatusService/*     Status API
  /gastown.v1.MailService/*       Mail API
  /gastown.v1.DecisionService/*   Decision API
  /events/decisions               SSE stream for decisions
  /health                         Health check
  /metrics                        Server metrics`,
	RunE: runRPCServe,
}

var (
	rpcPort     int
	rpcTownRoot string
	rpcAPIKey   string
	rpcCertFile string
	rpcKeyFile  string
)

func init() {
	rootCmd.AddCommand(rpcCmd)
	rpcCmd.AddCommand(rpcServeCmd)

	rpcServeCmd.Flags().IntVar(&rpcPort, "port", 8443, "Server port")
	rpcServeCmd.Flags().StringVar(&rpcTownRoot, "town", "", "Town root directory (auto-detected if not set)")
	rpcServeCmd.Flags().StringVar(&rpcAPIKey, "api-key", "", "API key for authentication (optional)")
	rpcServeCmd.Flags().StringVar(&rpcCertFile, "cert", "", "TLS certificate file (optional)")
	rpcServeCmd.Flags().StringVar(&rpcKeyFile, "key", "", "TLS key file (optional)")
}

func runRPCServe(cmd *cobra.Command, args []string) error {
	// Find town root
	root := rpcTownRoot
	if root == "" {
		var err error
		root, err = workspace.FindFromCwdOrError()
		if err != nil {
			return fmt.Errorf("not in a Gas Town workspace: %w", err)
		}
	}

	cfg := rpcserver.ServerConfig{
		Port:     rpcPort,
		TownRoot: root,
		APIKey:   rpcAPIKey,
		CertFile: rpcCertFile,
		KeyFile:  rpcKeyFile,
	}

	if err := rpcserver.RunServer(cfg); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
	return nil
}
