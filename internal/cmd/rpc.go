package cmd

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/rpcserver"
	"github.com/steveyegge/gastown/internal/style"
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
	// Find town root first (needed for systemd unit name)
	root := rpcTownRoot
	if root == "" {
		var err error
		root, err = workspace.FindFromCwdOrError()
		if err != nil {
			return fmt.Errorf("not in a Gas Town workspace: %w", err)
		}
	}

	// Check if this should be managed by systemd instead
	if os.Getenv("GT_RPC_SYSTEMD") == "" {
		// Build the systemd unit name (gt-rpc@<escaped-path>.service)
		absRoot, _ := filepath.Abs(root)
		unitName := fmt.Sprintf("gt-rpc@%s.service", systemdEscapePath(absRoot))
		if isRPCSystemdUnitEnabled(unitName) {
			fmt.Fprintln(os.Stderr, style.Warning.Render("⚠ gt rpc serve is managed by systemd"))
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "Use systemctl to manage the RPC server:")
			fmt.Fprintf(os.Stderr, "  systemctl --user status %s    # Check status\n", unitName)
			fmt.Fprintf(os.Stderr, "  systemctl --user restart %s   # Restart after rebuild\n", unitName)
			fmt.Fprintf(os.Stderr, "  journalctl --user -u %s -f    # View logs\n", unitName)
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "To disable systemd management:")
			fmt.Fprintf(os.Stderr, "  systemctl --user disable %s\n", unitName)
			return fmt.Errorf("use 'systemctl --user restart %s' instead", unitName)
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

// systemdEscapePath escapes a path for use in systemd unit names.
func systemdEscapePath(path string) string {
	cmd := exec.Command("systemd-escape", "--path", path)
	out, err := cmd.Output()
	if err != nil {
		// Fallback: manual escape (replace / with -)
		escaped := path
		if len(escaped) > 0 && escaped[0] == '/' {
			escaped = escaped[1:] // Remove leading /
		}
		// Replace path separators
		escaped = filepath.ToSlash(escaped)
		result := ""
		for _, c := range escaped {
			if c == '/' {
				result += "-"
			} else {
				result += string(c)
			}
		}
		return result
	}
	return string(out[:len(out)-1]) // Remove trailing newline
}

// isRPCSystemdUnitEnabled checks if a systemd user unit is enabled.
func isRPCSystemdUnitEnabled(unit string) bool {
	cmd := exec.Command("systemctl", "--user", "is-enabled", unit)
	err := cmd.Run()
	return err == nil
}
