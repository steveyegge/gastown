package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/terminal"
)

var (
	coopBrowser bool
	coopURL     bool
)

var coopCmd = &cobra.Command{
	Use:     "coop <target>",
	GroupID: GroupAgents,
	Short:   "Connect to a K8s agent's terminal via coop",
	Long: `Attach to (or open) a K8s agent's coop web terminal.

Target formats:
  mayor                        Town-level mayor
  gastown/witness               Rig witness
  gastown/polecats/nux          Polecat by name
  gastown/crew/max              Crew member
  gt-gastown-polecat-nux        Direct pod name

By default, attaches an interactive terminal (coop attach).
Use --browser to open the web terminal in a browser instead.
Use --url to just print the local URL (for scripting).`,
	Args: cobra.ExactArgs(1),
	RunE: runCoop,
}

func init() {
	rootCmd.AddCommand(coopCmd)
	coopCmd.Flags().BoolVarP(&coopBrowser, "browser", "b", false, "Open web terminal in browser instead of attaching")
	coopCmd.Flags().BoolVar(&coopURL, "url", false, "Print local URL and keep port-forward running (for scripting)")
}

func runCoop(cmd *cobra.Command, args []string) error {
	target := args[0]

	// Resolve target to pod name.
	podName, ns := resolveCoopTarget(target)
	if podName == "" {
		return fmt.Errorf("could not find running K8s pod for %q", target)
	}

	fmt.Printf("%s Connecting to %s in %s...\n",
		style.Bold.Render("☸"), style.Bold.Render(podName), ns)

	// Set up port-forward.
	conn := terminal.NewCoopPodConnection(terminal.CoopPodConnectionConfig{
		PodName:   podName,
		Namespace: ns,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := conn.Open(ctx); err != nil {
		return fmt.Errorf("connecting to pod: %w", err)
	}
	defer conn.Close()

	localURL := conn.LocalURL()
	fmt.Printf("  Port-forward: localhost:%d → %s:8080\n", conn.LocalPort(), podName)

	if coopURL {
		// Just print URL and block until interrupted.
		fmt.Println(localURL)
		fmt.Fprintf(os.Stderr, "  Press Ctrl+C to stop port-forward\n")
		// Block until signal.
		sigCh := make(chan os.Signal, 1)
		// Can't import signal here easily, just select forever.
		// The deferred Close() will clean up on return.
		<-sigCh
		return nil
	}

	if coopBrowser {
		// Open browser.
		opener := "xdg-open"
		if _, err := exec.LookPath("open"); err == nil {
			opener = "open"
		}
		fmt.Printf("  Opening %s\n", localURL)
		openCmd := exec.Command(opener, localURL)
		if err := openCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "  Failed to open browser: %v\n", err)
			fmt.Fprintf(os.Stderr, "  Open manually: %s\n", localURL)
		}
		fmt.Fprintf(os.Stderr, "  Press Ctrl+C to stop port-forward\n")
		// Keep port-forward alive until interrupted.
		sigCh := make(chan os.Signal, 1)
		<-sigCh
		return nil
	}

	// Default: coop attach (interactive terminal).
	coopPath, err := findCoopBinary()
	if err != nil {
		return err
	}

	fmt.Printf("  Detach: Ctrl+]\n\n")

	// Exec into coop attach — replaces this process.
	return syscall.Exec(coopPath, []string{"coop", "attach", localURL}, os.Environ())
}

// resolveCoopTarget converts a target string to (podName, namespace).
// Supports:
//   - Direct pod names: "gt-gastown-polecat-nux"
//   - Role paths: "gastown/polecats/nux", "gastown/witness", "mayor"
//   - Short forms: "mayor", "deacon"
func resolveCoopTarget(target string) (string, string) {
	ns := os.Getenv("GT_K8S_NAMESPACE")
	if ns == "" {
		ns = "gastown-uat"
	}

	var podName string

	if strings.HasPrefix(target, "gt-") {
		// Direct pod name.
		podName = target
	} else {
		podName = targetToPodName(target)
	}

	if podName == "" {
		return "", ""
	}

	// Verify pod is running.
	out, err := exec.Command("kubectl", "get", "pod", podName, "-n", ns,
		"-o", "jsonpath={.status.phase}").Output()
	if err != nil {
		// Pod not found — try listing pods with label match.
		return resolveByLabel(target, ns)
	}
	if strings.TrimSpace(string(out)) != "Running" {
		return "", ""
	}

	return podName, ns
}

// targetToPodName converts a role path to a K8s pod name.
func targetToPodName(target string) string {
	parts := strings.Split(target, "/")

	switch len(parts) {
	case 1:
		// Simple role: "mayor", "deacon"
		switch parts[0] {
		case "mayor":
			return "gt-town-mayor-hq"
		case "deacon":
			return "gt-town-deacon-hq"
		default:
			// Could be a rig name — can't resolve without more info.
			return ""
		}
	case 2:
		// rig/role: "gastown/witness", "gastown/refinery"
		rig, role := parts[0], parts[1]
		switch role {
		case "witness":
			return fmt.Sprintf("gt-%s-witness-hq", rig)
		case "refinery":
			return fmt.Sprintf("gt-%s-refinery-hq", rig)
		default:
			return ""
		}
	case 3:
		// rig/type/name: "gastown/polecats/nux", "gastown/crew/max"
		rig, roleType, name := parts[0], parts[1], parts[2]
		switch roleType {
		case "polecats", "polecat":
			return fmt.Sprintf("gt-%s-polecat-%s", rig, name)
		case "crew":
			return fmt.Sprintf("gt-%s-crew-%s", rig, name)
		default:
			return ""
		}
	}

	return ""
}

// resolveByLabel tries to find a pod by gt.* labels.
func resolveByLabel(target string, ns string) (string, string) {
	parts := strings.Split(target, "/")

	var labelSelector string
	switch len(parts) {
	case 1:
		labelSelector = fmt.Sprintf("gt.role=%s", parts[0])
	case 2:
		labelSelector = fmt.Sprintf("gt.rig=%s,gt.role=%s", parts[0], parts[1])
	case 3:
		role := parts[1]
		if role == "polecats" {
			role = "polecat"
		}
		labelSelector = fmt.Sprintf("gt.rig=%s,gt.role=%s,gt.agent=%s", parts[0], role, parts[2])
	default:
		return "", ""
	}

	out, err := exec.Command("kubectl", "get", "pods", "-n", ns,
		"-l", labelSelector,
		"--field-selector=status.phase=Running",
		"-o", "jsonpath={.items[0].metadata.name}").Output()
	if err != nil {
		return "", ""
	}

	name := strings.TrimSpace(string(out))
	if name == "" {
		return "", ""
	}
	return name, ns
}
