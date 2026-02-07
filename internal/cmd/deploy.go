// Package cmd provides CLI commands for the gt tool.
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
)

var deployCmd = &cobra.Command{
	Use:   "deploy <file> <ct>:<path>",
	Short: "Deploy a file to a Proxmox container",
	Long: `Deploy a file to a Proxmox container via SSH and pct push.

This command simplifies the common pattern of copying files to containers:
  1. scp file to PVE host
  2. pct push into container
  3. optionally restart a service

Examples:
  gt deploy portal.html 103:/var/www/html/index.html
  gt deploy app.py 109:/opt/app/app.py --restart myapp
  gt deploy config.yml 112:/etc/caddy/Caddyfile --restart caddy
  gt deploy --dry-run script.sh 107:/opt/scripts/

The command assumes 'pve' is configured as an SSH host in your ~/.ssh/config.`,
	Args: cobra.ExactArgs(2),
	RunE: runDeploy,
}

var (
	deployRestart string
	deployDryRun  bool
	deployPVEHost string
)

func init() {
	deployCmd.Flags().StringVar(&deployRestart, "restart", "", "Systemd service to restart after deploy")
	deployCmd.Flags().BoolVar(&deployDryRun, "dry-run", false, "Show commands without executing")
	deployCmd.Flags().StringVar(&deployPVEHost, "pve", "pve", "SSH host for Proxmox (default: pve)")

	rootCmd.AddCommand(deployCmd)
}

func runDeploy(cmd *cobra.Command, args []string) error {
	localFile := args[0]
	target := args[1]

	// Parse target: CT:path
	parts := strings.SplitN(target, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid target format, expected CT:path (e.g., 103:/var/www/html/index.html)")
	}
	ct := parts[0]
	remotePath := parts[1]

	// Verify local file exists
	if _, err := os.Stat(localFile); os.IsNotExist(err) {
		return fmt.Errorf("local file not found: %s", localFile)
	}

	// Get absolute path for local file
	absLocalFile, err := filepath.Abs(localFile)
	if err != nil {
		return fmt.Errorf("getting absolute path: %w", err)
	}

	// Generate temp path on PVE
	tmpFile := fmt.Sprintf("/tmp/%s", filepath.Base(localFile))

	fmt.Printf("%s Deploying %s → CT %s:%s\n",
		style.Bold.Render("→"),
		style.Cyan.Render(filepath.Base(localFile)),
		style.Yellow.Render(ct),
		style.Dim.Render(remotePath))

	// Step 1: scp to PVE
	scpCmd := fmt.Sprintf("scp %s %s:%s", absLocalFile, deployPVEHost, tmpFile)
	if deployDryRun {
		fmt.Printf("  [dry-run] %s\n", scpCmd)
	} else {
		fmt.Printf("  %s Copying to PVE...\n", style.Dim.Render("•"))
		if err := runShellCmd(scpCmd); err != nil {
			return fmt.Errorf("scp failed: %w", err)
		}
	}

	// Step 2: pct push into container
	pushCmd := fmt.Sprintf("ssh %s \"pct push %s %s %s\"", deployPVEHost, ct, tmpFile, remotePath)
	if deployDryRun {
		fmt.Printf("  [dry-run] %s\n", pushCmd)
	} else {
		fmt.Printf("  %s Pushing to container...\n", style.Dim.Render("•"))
		if err := runShellCmd(pushCmd); err != nil {
			return fmt.Errorf("pct push failed: %w", err)
		}
	}

	// Step 3: Cleanup temp file on PVE
	cleanupCmd := fmt.Sprintf("ssh %s \"rm -f %s\"", deployPVEHost, tmpFile)
	if deployDryRun {
		fmt.Printf("  [dry-run] %s\n", cleanupCmd)
	} else {
		_ = runShellCmd(cleanupCmd) // Best effort cleanup
	}

	// Step 4: Optional service restart
	if deployRestart != "" {
		restartCmd := fmt.Sprintf("ssh %s \"pct exec %s -- systemctl restart %s\"", deployPVEHost, ct, deployRestart)
		if deployDryRun {
			fmt.Printf("  [dry-run] %s\n", restartCmd)
		} else {
			fmt.Printf("  %s Restarting %s...\n", style.Dim.Render("•"), deployRestart)
			if err := runShellCmd(restartCmd); err != nil {
				return fmt.Errorf("service restart failed: %w", err)
			}
		}
	}

	if deployDryRun {
		fmt.Printf("\n%s Dry run complete\n", style.Yellow.Render("⚠"))
	} else {
		fmt.Printf("%s Deployed successfully\n", style.Green.Render("✓"))
	}

	return nil
}

func runShellCmd(command string) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
