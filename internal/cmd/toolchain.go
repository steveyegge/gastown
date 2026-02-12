package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
)

var toolchainCmd = &cobra.Command{
	Use:     "toolchain",
	GroupID: GroupServices,
	Short:   "Manage toolchain sidecar in K8s agent pods",
	Long: `Interact with the toolchain sidecar container in a K8s agent pod.

The toolchain sidecar provides additional development tools (compilers,
linters, build systems) that run alongside the agent container. These
commands let you inspect and use the toolchain from within the agent.

Requires: Running inside a K8s agent pod with a toolchain sidecar.
Environment: GT_TOOLCHAIN_CONTAINER, GT_TOOLCHAIN_IMAGE, GT_TOOLCHAIN_PROFILE`,
}

var toolchainStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show toolchain sidecar status",
	Long: `Show the current toolchain sidecar configuration and status.

Displays the profile, image, container name, and whether the sidecar
is detected as running.`,
	RunE: runToolchainStatus,
}

var toolchainExecCmd = &cobra.Command{
	Use:   "exec -- <command> [args...]",
	Short: "Run a command in the toolchain sidecar",
	Long: `Execute a command in the toolchain sidecar container via kubectl exec.

The command runs in the toolchain container which shares the workspace
volume with the agent. This lets you use tools (compilers, linters, etc.)
that are installed in the sidecar but not in the agent image.

Examples:
  gt toolchain exec -- node --version
  gt toolchain exec -- npm install
  gt toolchain exec -- python3 -c "print('hello')"`,
	RunE:               runToolchainExec,
	DisableFlagParsing: true,
}

var toolchainListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tools available in the sidecar",
	Long: `Query the toolchain sidecar for available development tools.

Runs 'which' for common tool names in the sidecar container and
reports which are available. Useful for discovering what tools the
current sidecar profile provides.`,
	RunE: runToolchainList,
}

func init() {
	toolchainCmd.AddCommand(toolchainStatusCmd)
	toolchainCmd.AddCommand(toolchainExecCmd)
	toolchainCmd.AddCommand(toolchainListCmd)
	rootCmd.AddCommand(toolchainCmd)
}

func runToolchainStatus(cmd *cobra.Command, args []string) error {
	container := os.Getenv("GT_TOOLCHAIN_CONTAINER")
	image := os.Getenv("GT_TOOLCHAIN_IMAGE")
	profile := os.Getenv("GT_TOOLCHAIN_PROFILE")

	if container == "" && image == "" {
		fmt.Printf("%s No toolchain sidecar configured.\n", style.Dim.Render("⚠"))
		fmt.Println("  This pod was created without a toolchain sidecar.")
		fmt.Println("  Set sidecar_profile or sidecar_image metadata on the agent bead to enable.")
		return nil
	}

	fmt.Printf("%s Toolchain Sidecar\n\n", style.Bold.Render("🔧"))

	if profile != "" {
		fmt.Printf("  %s  %s\n", style.Bold.Render("Profile:"), profile)
	}
	if image != "" {
		fmt.Printf("  %s    %s\n", style.Bold.Render("Image:"), image)
	}
	if container != "" {
		fmt.Printf("  %s %s\n", style.Bold.Render("Container:"), container)
	}

	// Check if running in K8s by looking for HOSTNAME (pod name).
	podName := os.Getenv("HOSTNAME")
	namespace := os.Getenv("GT_POD_NAMESPACE")
	if namespace == "" {
		namespace = detectNamespace()
	}

	if podName != "" {
		fmt.Printf("  %s      %s\n", style.Bold.Render("Pod:"), podName)
	}
	if namespace != "" {
		fmt.Printf("  %s %s\n", style.Bold.Render("Namespace:"), namespace)
	}

	// Try to check if sidecar is actually running via kubectl.
	if podName != "" && container != "" {
		running := checkSidecarRunning(podName, namespace, container)
		if running {
			fmt.Printf("\n  %s Sidecar is running\n", style.Bold.Render("✓"))
		} else {
			fmt.Printf("\n  %s Sidecar not detected (may still be starting)\n",
				style.Dim.Render("⚠"))
		}
	}

	return nil
}

func runToolchainExec(cmd *cobra.Command, args []string) error {
	container := os.Getenv("GT_TOOLCHAIN_CONTAINER")
	if container == "" {
		return fmt.Errorf("no toolchain sidecar configured (GT_TOOLCHAIN_CONTAINER not set)")
	}

	// Strip leading "--" if present (cobra passes it through with DisableFlagParsing).
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		return fmt.Errorf("usage: gt toolchain exec -- <command> [args...]")
	}

	podName := os.Getenv("HOSTNAME")
	if podName == "" {
		return fmt.Errorf("cannot determine pod name (HOSTNAME not set)")
	}

	namespace := os.Getenv("GT_POD_NAMESPACE")
	if namespace == "" {
		namespace = detectNamespace()
	}

	// Build kubectl exec command.
	kubectlArgs := []string{"exec", podName, "-c", container}
	if namespace != "" {
		kubectlArgs = append(kubectlArgs, "-n", namespace)
	}
	kubectlArgs = append(kubectlArgs, "--")
	kubectlArgs = append(kubectlArgs, args...)

	execCmd := exec.Command("kubectl", kubectlArgs...)
	execCmd.Stdin = os.Stdin
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr

	return execCmd.Run()
}

// detectNamespace reads the K8s namespace from the service account mount.
func detectNamespace() string {
	data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// commonTools is the list of tool names to probe in the sidecar.
var commonTools = []string{
	"node", "npm", "npx", "yarn", "pnpm",
	"python3", "pip3", "python", "pip",
	"go", "rustc", "cargo",
	"gcc", "g++", "make", "cmake",
	"java", "javac", "mvn", "gradle",
	"ruby", "gem", "perl",
	"git", "curl", "wget", "jq",
	"docker", "kubectl",
}

func runToolchainList(cmd *cobra.Command, args []string) error {
	container := os.Getenv("GT_TOOLCHAIN_CONTAINER")
	if container == "" {
		return fmt.Errorf("no toolchain sidecar configured (GT_TOOLCHAIN_CONTAINER not set)")
	}

	podName := os.Getenv("HOSTNAME")
	if podName == "" {
		return fmt.Errorf("cannot determine pod name (HOSTNAME not set)")
	}

	namespace := os.Getenv("GT_POD_NAMESPACE")
	if namespace == "" {
		namespace = detectNamespace()
	}

	fmt.Printf("%s Available tools in sidecar %s\n\n",
		style.Bold.Render("🔧"), style.Dim.Render("("+container+")"))

	// Build a single shell command that probes all tools.
	var checks []string
	for _, tool := range commonTools {
		checks = append(checks, fmt.Sprintf("which %s 2>/dev/null && echo '%s: found' || echo '%s: not found'", tool, tool, tool))
	}
	shellCmd := strings.Join(checks, "; ")

	kubectlArgs := []string{"exec", podName, "-c", container}
	if namespace != "" {
		kubectlArgs = append(kubectlArgs, "-n", namespace)
	}
	kubectlArgs = append(kubectlArgs, "--", "sh", "-c", shellCmd)

	out, err := exec.Command("kubectl", kubectlArgs...).Output()
	if err != nil {
		return fmt.Errorf("failed to query sidecar: %w", err)
	}

	// Parse output — lines are either paths (from which) or "tool: found/not found".
	var found []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasSuffix(line, ": found") {
			name := strings.TrimSuffix(line, ": found")
			found = append(found, name)
		}
	}

	if len(found) == 0 {
		fmt.Println("  No common tools detected.")
	} else {
		for _, name := range found {
			fmt.Printf("  %s %s\n", style.Bold.Render("✓"), name)
		}
	}
	fmt.Printf("\n  %s Use 'gt toolchain exec -- <cmd>' to run any command.\n",
		style.Dim.Render("tip:"))

	return nil
}

// checkSidecarRunning uses kubectl to check if the sidecar container is running.
func checkSidecarRunning(podName, namespace, container string) bool {
	args := []string{"get", "pod", podName, "-o",
		fmt.Sprintf("jsonpath={.status.initContainerStatuses[?(@.name=='%s')].state.running}", container)}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	out, err := exec.Command("kubectl", args...).Output()
	if err != nil {
		return false
	}
	return len(out) > 0 && string(out) != "{}"
}
