package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/workspace"
)

var envCmd = &cobra.Command{
	Use:     "env",
	GroupID: GroupDiag,
	Short:   "Output agent environment variables",
	Long: `Output the canonical agent environment variables as JSON or shell exports.

By default outputs JSON. Use --export for shell export statements.

Flags --role, --rig, and --agent-name specify the agent identity.
If omitted, values are read from the current environment (GT_ROLE, GT_RIG, etc).

Examples:
  gt env --json --role polecat --rig gastown --agent-name toast
  gt env --export --role crew --rig beads --agent-name quartz
  eval $(gt env --export)`,
	RunE: runEnv,
}

var (
	envRole      string
	envRig       string
	envAgentName string
	envExport    bool
)

func init() {
	envCmd.Flags().StringVar(&envRole, "role", "", "Agent role (mayor, deacon, witness, refinery, crew, polecat)")
	envCmd.Flags().StringVar(&envRig, "rig", "", "Rig name")
	envCmd.Flags().StringVar(&envAgentName, "agent-name", "", "Agent name (polecat or crew member name)")
	envCmd.Flags().BoolVar(&envExport, "export", false, "Output as shell export statements instead of JSON")
	rootCmd.AddCommand(envCmd)
}

func runEnv(cmd *cobra.Command, args []string) error {
	// If flags not provided, fall back to environment variables
	role := envRole
	if role == "" {
		role = inferRoleFromEnv()
	}
	if role == "" {
		return fmt.Errorf("--role is required (or set GT_ROLE)")
	}

	rigName := envRig
	if rigName == "" {
		rigName = os.Getenv("GT_RIG")
	}

	agentName := envAgentName
	if agentName == "" {
		// Try GT_POLECAT or GT_CREW depending on role
		if role == "polecat" {
			agentName = os.Getenv("GT_POLECAT")
		} else if role == "crew" {
			agentName = os.Getenv("GT_CREW")
		}
	}

	// Validate role requires rig
	switch role {
	case "witness", "refinery", "polecat", "crew":
		if rigName == "" {
			return fmt.Errorf("--rig is required for role %q", role)
		}
	}

	// Validate role requires agent name
	switch role {
	case "polecat", "crew":
		if agentName == "" {
			return fmt.Errorf("--agent-name is required for role %q", role)
		}
	}

	// Discover town root
	townRoot, _ := workspace.FindFromCwd()
	if townRoot == "" {
		townRoot = os.Getenv("GT_ROOT")
	}

	envVars := config.AgentEnv(config.AgentEnvConfig{
		Role:         role,
		Rig:          rigName,
		AgentName:    agentName,
		TownRoot:     townRoot,
		BDDaemonHost: os.Getenv("BD_DAEMON_HOST"),
	})

	if envExport {
		return envOutputExport(envVars)
	}
	return envOutputJSON(envVars)
}

func envOutputJSON(envVars map[string]string) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(envVars)
}

func envOutputExport(envVars map[string]string) error {
	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("export %s=%s\n", k, config.ShellQuote(envVars[k]))
	}
	return nil
}

// inferRoleFromEnv extracts the base role from GT_ROLE env var.
// GT_ROLE is compound: "mayor", "deacon", "gastown/witness", "gastown/polecats/toast", etc.
func inferRoleFromEnv() string {
	gtRole := os.Getenv("GT_ROLE")
	if gtRole == "" {
		return ""
	}

	// Simple roles
	switch gtRole {
	case "mayor", "deacon":
		return gtRole
	}

	// Compound roles: "rig/witness", "rig/refinery", "rig/polecats/name", "rig/crew/name"
	parts := splitRoleParts(gtRole)
	if len(parts) >= 2 {
		switch parts[1] {
		case "witness":
			return "witness"
		case "refinery":
			return "refinery"
		case "polecats":
			return "polecat"
		case "crew":
			return "crew"
		}
	}

	// Boot role
	if gtRole == "deacon/boot" {
		return "boot"
	}

	return ""
}

// splitRoleParts splits a GT_ROLE value by "/".
func splitRoleParts(role string) []string {
	var parts []string
	current := ""
	for _, c := range role {
		if c == '/' {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}
