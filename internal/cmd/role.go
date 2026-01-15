package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

// Environment variables for role detection
const (
	EnvGTRole     = "GT_ROLE"
	EnvGTRoleHome = "GT_ROLE_HOME"
)

// RoleInfo contains information about a role and its detection source.
// This is the canonical struct for role detection - used by both GetRole()
// and detectRole() functions.
type RoleInfo struct {
	Role          Role   `json:"role"`
	Source        string `json:"source"` // "env", "cwd", or "explicit"
	Home          string `json:"home"`
	Rig           string `json:"rig,omitempty"`
	Polecat       string `json:"polecat,omitempty"`
	EnvRole       string `json:"env_role,omitempty"`    // Value of GT_ROLE if set
	CwdRole       Role   `json:"cwd_role,omitempty"`    // Role detected from cwd
	Mismatch      bool   `json:"mismatch,omitempty"`    // True if env != cwd detection
	EnvIncomplete bool   `json:"env_incomplete,omitempty"` // True if env was set but missing rig/polecat, filled from cwd
	TownRoot      string `json:"town_root,omitempty"`
	WorkDir       string `json:"work_dir,omitempty"`    // Current working directory
}

var roleCmd = &cobra.Command{
	Use:     "role",
	GroupID: GroupAgents,
	Short:   "Show or manage agent role",
	Long: `Display the current agent role and its detection source.

Role is determined by:
1. GT_ROLE environment variable (authoritative if set)
2. Current working directory (fallback)

If both are available and disagree, a warning is shown.`,
	RunE: runRoleShow,
}

var roleShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current role",
	RunE:  runRoleShow,
}

var roleHomeCmd = &cobra.Command{
	Use:   "home [ROLE]",
	Short: "Show home directory for a role",
	Long: `Show the canonical home directory for a role.

If no role is specified, shows the home for the current role.

Examples:
  gt role home           # Home for current role
  gt role home mayor     # Home for mayor
  gt role home witness   # Home for witness (requires --rig)`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRoleHome,
}

var roleDetectCmd = &cobra.Command{
	Use:   "detect",
	Short: "Force cwd-based role detection (debugging)",
	Long: `Detect role from current working directory, ignoring GT_ROLE env var.

This is useful for debugging role detection issues.`,
	RunE: runRoleDetect,
}

var roleListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all known roles",
	RunE:  runRoleList,
}

var roleEnvCmd = &cobra.Command{
	Use:   "env",
	Short: "Print export statements for current role",
	Long: `Print shell export statements for the current role.

Role is determined from GT_ROLE environment variable or current working directory.
This is a read-only command that displays the current role's env vars.

Examples:
  eval $(gt role env)    # Export current role's env vars
  gt role env            # View what would be exported`,
	RunE: runRoleEnv,
}

// Flags for role home command
var (
	roleRig     string
	rolePolecat string
)

func init() {
	rootCmd.AddCommand(roleCmd)
	roleCmd.AddCommand(roleShowCmd)
	roleCmd.AddCommand(roleHomeCmd)
	roleCmd.AddCommand(roleDetectCmd)
	roleCmd.AddCommand(roleListCmd)
	roleCmd.AddCommand(roleEnvCmd)

	// Add --rig and --polecat flags to home command for overrides
	roleHomeCmd.Flags().StringVar(&roleRig, "rig", "", "Rig name (required for rig-specific roles)")
	roleHomeCmd.Flags().StringVar(&rolePolecat, "polecat", "", "Polecat/crew member name")
}

// GetRole returns the current role, checking GT_ROLE first then falling back to cwd.
// This is the canonical function for role detection.
func GetRole() (RoleInfo, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return RoleInfo{}, fmt.Errorf("getting current directory: %w", err)
	}

	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return RoleInfo{}, fmt.Errorf("finding workspace: %w", err)
	}
	if townRoot == "" {
		return RoleInfo{}, fmt.Errorf("not in a Gas Town workspace")
	}

	return GetRoleWithContext(cwd, townRoot)
}

// GetRoleWithContext returns role info given explicit cwd and town root.
func GetRoleWithContext(cwd, townRoot string) (RoleInfo, error) {
	info := RoleInfo{
		TownRoot: townRoot,
		WorkDir:  cwd,
	}

	// Check environment variable first
	envRole := os.Getenv(EnvGTRole)
	info.EnvRole = envRole

	// Always detect from cwd for comparison/fallback
	cwdCtx := detectRole(cwd, townRoot)
	info.CwdRole = cwdCtx.Role

	// Determine authoritative role
	if envRole != "" {
		// Parse env role - it might be simple ("mayor") or compound ("gastown/witness")
		parsedRole, rig, polecat := parseRoleString(envRole)
		info.Role = parsedRole
		info.Rig = rig
		info.Polecat = polecat
		info.Source = "env"

		// For simple role strings like "crew" or "polecat", also check
		// GT_RIG and GT_CREW/GT_POLECAT env vars for the full identity
		if info.Rig == "" {
			if envRig := os.Getenv("GT_RIG"); envRig != "" {
				info.Rig = envRig
			}
		}
		if info.Polecat == "" {
			if envCrew := os.Getenv("GT_CREW"); envCrew != "" {
				info.Polecat = envCrew
			} else if envPolecat := os.Getenv("GT_POLECAT"); envPolecat != "" {
				info.Polecat = envPolecat
			}
		}

		// If env is incomplete (missing rig/polecat for roles that need them),
		// fill gaps from cwd detection and mark as incomplete
		needsRig := parsedRole == RoleWitness || parsedRole == RoleRefinery || parsedRole == RolePolecat || parsedRole == RoleCrew
		needsPolecat := parsedRole == RolePolecat || parsedRole == RoleCrew

		if needsRig && info.Rig == "" && cwdCtx.Rig != "" {
			info.Rig = cwdCtx.Rig
			info.EnvIncomplete = true
		}
		if needsPolecat && info.Polecat == "" && cwdCtx.Polecat != "" {
			info.Polecat = cwdCtx.Polecat
			info.EnvIncomplete = true
		}

		// Check for mismatch with cwd detection
		if cwdCtx.Role != RoleUnknown && cwdCtx.Role != parsedRole {
			info.Mismatch = true
		}
	} else {
		// Fall back to cwd detection - copy all fields from cwdCtx
		info.Role = cwdCtx.Role
		info.Rig = cwdCtx.Rig
		info.Polecat = cwdCtx.Polecat
		info.Source = "cwd"
	}

	// Determine home directory
	info.Home = getRoleHome(info.Role, info.Rig, info.Polecat, townRoot)

	return info, nil
}

// parseRoleString parses a role string like "mayor", "gastown/witness", or "gastown/polecats/alpha".
func parseRoleString(s string) (Role, string, string) {
	s = strings.TrimSpace(s)

	// Simple roles
	switch s {
	case "mayor":
		return RoleMayor, "", ""
	case "deacon":
		return RoleDeacon, "", ""
	}

	// Compound roles: rig/role or rig/polecats/name or rig/crew/name
	parts := strings.Split(s, "/")
	if len(parts) < 2 {
		// Unknown format, try to match as simple role
		return Role(s), "", ""
	}

	rig := parts[0]

	switch parts[1] {
	case "witness":
		return RoleWitness, rig, ""
	case "refinery":
		return RoleRefinery, rig, ""
	case "polecats":
		if len(parts) >= 3 {
			return RolePolecat, rig, parts[2]
		}
		return RolePolecat, rig, ""
	case "crew":
		if len(parts) >= 3 {
			return RoleCrew, rig, parts[2]
		}
		return RoleCrew, rig, ""
	default:
		// Might be rig/polecatName format
		return RolePolecat, rig, parts[1]
	}
}

// ActorString returns the actor identity string for beads attribution.
// Format matches beads created_by convention:
//   - Simple roles: "mayor", "deacon"
//   - Rig-specific: "gastown/witness", "gastown/refinery"
//   - Workers: "gastown/crew/max", "gastown/polecats/Toast"
func (info RoleInfo) ActorString() string {
	switch info.Role {
	case RoleMayor:
		return "mayor"
	case RoleDeacon:
		return "deacon"
	case RoleWitness:
		if info.Rig != "" {
			return fmt.Sprintf("%s/witness", info.Rig)
		}
		return "witness"
	case RoleRefinery:
		if info.Rig != "" {
			return fmt.Sprintf("%s/refinery", info.Rig)
		}
		return "refinery"
	case RolePolecat:
		if info.Rig != "" && info.Polecat != "" {
			return fmt.Sprintf("%s/polecats/%s", info.Rig, info.Polecat)
		}
		return "polecat"
	case RoleCrew:
		if info.Rig != "" && info.Polecat != "" {
			return fmt.Sprintf("%s/crew/%s", info.Rig, info.Polecat)
		}
		return "crew"
	default:
		return string(info.Role)
	}
}

// getRoleHome returns the canonical home directory for a role.
func getRoleHome(role Role, rig, polecat, townRoot string) string {
	switch role {
	case RoleMayor:
		return filepath.Join(townRoot, "mayor")
	case RoleDeacon:
		return filepath.Join(townRoot, "deacon")
	case RoleWitness:
		if rig == "" {
			return ""
		}
		return filepath.Join(townRoot, rig, "witness")
	case RoleRefinery:
		if rig == "" {
			return ""
		}
		return filepath.Join(townRoot, rig, "refinery", "rig")
	case RolePolecat:
		if rig == "" || polecat == "" {
			return ""
		}
		return filepath.Join(townRoot, rig, "polecats", polecat, "rig")
	case RoleCrew:
		if rig == "" || polecat == "" {
			return ""
		}
		return filepath.Join(townRoot, rig, "crew", polecat, "rig")
	default:
		return ""
	}
}

func runRoleShow(cmd *cobra.Command, args []string) error {
	info, err := GetRole()
	if err != nil {
		return err
	}

	// Header
	fmt.Printf("%s\n", style.Bold.Render(string(info.Role)))
	fmt.Printf("Source: %s\n", info.Source)

	if info.Home != "" {
		fmt.Printf("Home: %s\n", info.Home)
	}

	if info.Rig != "" {
		fmt.Printf("Rig: %s\n", info.Rig)
	}

	if info.Polecat != "" {
		fmt.Printf("Worker: %s\n", info.Polecat)
	}

	// Show mismatch warning
	if info.Mismatch {
		fmt.Println()
		fmt.Printf("%s\n", style.Bold.Render("⚠️  ROLE MISMATCH"))
		fmt.Printf("  GT_ROLE=%s (authoritative)\n", info.EnvRole)
		fmt.Printf("  cwd suggests: %s\n", info.CwdRole)
		fmt.Println()
		fmt.Println("The GT_ROLE env var takes precedence, but you may be in the wrong directory.")
		fmt.Printf("Expected home: %s\n", info.Home)
	}

	return nil
}

func runRoleHome(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}

	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding workspace: %w", err)
	}
	if townRoot == "" {
		return fmt.Errorf("not in a Gas Town workspace")
	}

	// Validate flag combinations: --polecat requires --rig to prevent strange merges
	if rolePolecat != "" && roleRig == "" {
		return fmt.Errorf("--polecat requires --rig to be specified")
	}

	// Start with current role detection (from env vars or cwd)
	info, err := GetRole()
	if err != nil {
		return err
	}
	role := info.Role
	rig := info.Rig
	polecat := info.Polecat

	// Apply overrides from arguments/flags
	if len(args) > 0 {
		role, _, _ = parseRoleString(args[0])
	}
	if roleRig != "" {
		rig = roleRig
	}
	if rolePolecat != "" {
		polecat = rolePolecat
	}

	home := getRoleHome(role, rig, polecat, townRoot)
	if home == "" {
		return fmt.Errorf("cannot determine home for role %s (rig=%q, polecat=%q)", role, rig, polecat)
	}

	// Warn if computed home doesn't match cwd
	if home != cwd && !strings.HasPrefix(cwd, home) {
		fmt.Fprintf(os.Stderr, "⚠️  Warning: cwd (%s) is not within role home (%s)\n", cwd, home)
	}

	fmt.Println(home)
	return nil
}

func runRoleDetect(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}

	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding workspace: %w", err)
	}
	if townRoot == "" {
		return fmt.Errorf("not in a Gas Town workspace")
	}

	ctx := detectRole(cwd, townRoot)

	fmt.Printf("%s (from cwd)\n", style.Bold.Render(string(ctx.Role)))
	fmt.Printf("Directory: %s\n", cwd)

	if ctx.Rig != "" {
		fmt.Printf("Rig: %s\n", ctx.Rig)
	}
	if ctx.Polecat != "" {
		fmt.Printf("Worker: %s\n", ctx.Polecat)
	}

	// Check if env var disagrees
	envRole := os.Getenv(EnvGTRole)
	if envRole != "" {
		parsedRole, _, _ := parseRoleString(envRole)
		if parsedRole != ctx.Role {
			fmt.Println()
			fmt.Printf("%s\n", style.Bold.Render("⚠️  Mismatch with $GT_ROLE"))
			fmt.Printf("  $GT_ROLE=%s\n", envRole)
			fmt.Println("  The env var takes precedence in normal operation.")
		}
	}

	return nil
}

func runRoleList(cmd *cobra.Command, args []string) error {
	roles := []struct {
		name Role
		desc string
	}{
		{RoleMayor, "Global coordinator at mayor/"},
		{RoleDeacon, "Background supervisor daemon"},
		{RoleWitness, "Per-rig polecat lifecycle manager"},
		{RoleRefinery, "Per-rig merge queue processor"},
		{RolePolecat, "Ephemeral worker with own worktree"},
		{RoleCrew, "Persistent worker with own worktree"},
	}

	fmt.Println("Available roles:")
	fmt.Println()
	for _, r := range roles {
		fmt.Printf("  %-10s  %s\n", style.Bold.Render(string(r.name)), r.desc)
	}
	return nil
}

func runRoleEnv(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}

	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding workspace: %w", err)
	}
	if townRoot == "" {
		return fmt.Errorf("not in a Gas Town workspace")
	}

	// Get current role (read-only - from env vars or cwd)
	info, err := GetRole()
	if err != nil {
		return err
	}

	home := getRoleHome(info.Role, info.Rig, info.Polecat, townRoot)
	if home == "" {
		return fmt.Errorf("cannot determine home for role %s (rig=%q, polecat=%q)", info.Role, info.Rig, info.Polecat)
	}

	// Warn if env was incomplete and we filled from cwd
	if info.EnvIncomplete {
		fmt.Fprintf(os.Stderr, "⚠️  Warning: env vars incomplete, filled from cwd\n")
	}

	// Warn if computed home doesn't match cwd
	if home != cwd && !strings.HasPrefix(cwd, home) {
		fmt.Fprintf(os.Stderr, "⚠️  Warning: cwd (%s) is not within role home (%s)\n", cwd, home)
	}

	// Get canonical env vars from shared source of truth
	envVars := config.AgentEnv(config.AgentEnvConfig{
		Role:      string(info.Role),
		Rig:       info.Rig,
		AgentName: info.Polecat,
		TownRoot:  townRoot,
	})
	envVars[EnvGTRoleHome] = home

	// Output in sorted order for consistent output
	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("export %s=%s\n", k, envVars[k])
	}

	return nil
}
