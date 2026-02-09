package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	connectURL         string
	connectToken       string
	connectContext     string
	connectStatus      bool
	connectDisconnect  bool
	connectPortForward bool
)

var connectCmd = &cobra.Command{
	Use:     "connect [namespace]",
	GroupID: GroupWorkspace,
	Short:   "Connect to a remote Gas Town daemon",
	Long: `Connect to a remote Gas Town daemon running in a Kubernetes cluster.

By default, discovers the daemon in the given K8s namespace using kubectl.
Use --url to connect directly to a known daemon endpoint.

Examples:
  gt connect gastown-uat                              # Auto-discover daemon in K8s namespace
  gt connect --url https://gastown-uat.app.e2e.dev    # Direct URL to daemon
  gt connect --status                                 # Show current connection info
  gt connect --disconnect                             # Remove remote daemon config`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if connectStatus {
			return runConnectStatus(cmd, args)
		}
		if connectDisconnect {
			return runDisconnect(cmd, args)
		}
		return runConnect(cmd, args)
	},
}

func init() {
	connectCmd.Flags().StringVar(&connectURL, "url", "", "Direct daemon URL (skips K8s discovery)")
	connectCmd.Flags().StringVar(&connectToken, "token", "", "Daemon auth token (skips K8s secret lookup)")
	connectCmd.Flags().StringVar(&connectContext, "context", "", "Kubectl context to use")
	connectCmd.Flags().BoolVar(&connectStatus, "status", false, "Show current connection status")
	connectCmd.Flags().BoolVar(&connectDisconnect, "disconnect", false, "Remove remote daemon config")
	connectCmd.Flags().BoolVar(&connectPortForward, "port-forward", false, "Start kubectl port-forward (fallback when no ingress)")
	rootCmd.AddCommand(connectCmd)
}

func runConnect(cmd *cobra.Command, args []string) error {
	// Determine daemon URL
	var daemonURL string
	if connectURL != "" {
		daemonURL = connectURL
	} else {
		// Need a namespace for K8s discovery
		if len(args) == 0 {
			return fmt.Errorf("namespace is required (or use --url to connect directly)")
		}
		namespace := args[0]
		var err error
		daemonURL, err = discoverK8sDaemon(namespace, connectContext)
		if err != nil {
			return fmt.Errorf("discovering daemon in namespace %q: %w", namespace, err)
		}
	}

	// Determine auth token
	var token string
	if connectToken != "" {
		token = connectToken
	} else {
		namespace := ""
		if len(args) > 0 {
			namespace = args[0]
		}
		var err error
		token, err = extractK8sToken(namespace, connectContext)
		if err != nil {
			return fmt.Errorf("extracting auth token: %w", err)
		}
	}

	// Persist connection config
	if err := writeBeadsConfig(daemonURL, token); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	// Verify connectivity
	if err := verifyConnection(daemonURL, token); err != nil {
		return fmt.Errorf("verifying connection: %w", err)
	}

	fmt.Printf("Connected to remote daemon at %s\n", daemonURL)
	return nil
}
