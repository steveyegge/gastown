package cmd

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/quota"
	"github.com/steveyegge/gastown/internal/style"
	ttmux "github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

// quotaLogger adapts style.PrintWarning to the quota.Logger interface.
type quotaLogger struct{}

func (quotaLogger) Warn(format string, args ...interface{}) {
	style.PrintWarning(format, args...)
}

// Quota command flags
var (
	quotaJSON bool
)

var quotaCmd = &cobra.Command{
	Use:     "quota",
	GroupID: GroupServices,
	Short:   "Manage account quota rotation",
	RunE:    requireSubcommand,
	Long: `Manage Claude Code account quota rotation for Gas Town.

When sessions hit rate limits, quota commands help detect blocked sessions
and rotate them to available accounts from the pool.

Commands:
  gt quota status            Show account quota status
  gt quota scan              Detect rate-limited sessions
  gt quota rotate            Swap blocked sessions to available accounts
  gt quota clear             Mark account(s) as available again`,
}

var quotaStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show account quota status",
	Long: `Show the quota status of all registered accounts.

Displays which accounts are available, rate-limited, or in cooldown,
along with timestamps for limit detection and estimated reset times.

Examples:
  gt quota status           # Text output
  gt quota status --json    # JSON output`,
	RunE: runQuotaStatus,
}

// QuotaStatusItem represents an account in status output.
type QuotaStatusItem struct {
	Handle    string `json:"handle"`
	Email     string `json:"email"`
	Status    string `json:"status"`
	LimitedAt string `json:"limited_at,omitempty"`
	ResetsAt  string `json:"resets_at,omitempty"`
	LastUsed  string `json:"last_used,omitempty"`
	IsDefault bool   `json:"is_default"`
}

func runQuotaStatus(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	// Load accounts
	accountsPath := constants.MayorAccountsPath(townRoot)
	acctCfg, err := config.LoadAccountsConfig(accountsPath)
	if err != nil {
		fmt.Println("No accounts configured.")
		fmt.Println("\nTo add an account:")
		fmt.Println("  gt account add <handle>")
		return nil
	}

	if len(acctCfg.Accounts) == 0 {
		fmt.Println("No accounts configured.")
		return nil
	}

	// Load quota state
	mgr := quota.NewManager(townRoot)
	state, err := mgr.Load()
	if err != nil {
		return fmt.Errorf("loading quota state: %w", err)
	}

	// Ensure all accounts are tracked
	mgr.EnsureAccountsTracked(state, acctCfg.Accounts)

	if quotaJSON {
		return printQuotaStatusJSON(acctCfg, state)
	}
	return printQuotaStatusText(acctCfg, state)
}

func printQuotaStatusJSON(acctCfg *config.AccountsConfig, state *config.QuotaState) error {
	var items []QuotaStatusItem
	for _, handle := range slices.Sorted(maps.Keys(acctCfg.Accounts)) {
		acct := acctCfg.Accounts[handle]
		qs := state.Accounts[handle]
		status := string(qs.Status)
		if status == "" {
			status = string(config.QuotaStatusAvailable)
		}
		items = append(items, QuotaStatusItem{
			Handle:    handle,
			Email:     acct.Email,
			Status:    status,
			LimitedAt: qs.LimitedAt,
			ResetsAt:  qs.ResetsAt,
			LastUsed:  qs.LastUsed,
			IsDefault: handle == acctCfg.Default,
		})
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(items)
}

func printQuotaStatusText(acctCfg *config.AccountsConfig, state *config.QuotaState) error {
	available := 0
	limited := 0

	fmt.Println(style.Bold.Render("Account Quota Status"))
	fmt.Println()

	for _, handle := range slices.Sorted(maps.Keys(acctCfg.Accounts)) {
		acct := acctCfg.Accounts[handle]
		qs := state.Accounts[handle]
		status := qs.Status
		if status == "" {
			status = config.QuotaStatusAvailable
		}

		// Handle marker and default indicator
		marker := " "
		if handle == acctCfg.Default {
			marker = "*"
		}

		// Status badge
		var badge string
		switch status {
		case config.QuotaStatusAvailable:
			badge = style.Success.Render("available")
			available++
		case config.QuotaStatusLimited:
			badge = style.Error.Render("limited")
			limited++
			if qs.ResetsAt != "" {
				badge += style.Dim.Render(" (resets " + qs.ResetsAt + ")")
			}
		case config.QuotaStatusCooldown:
			badge = style.Warning.Render("cooldown")
			limited++
		default:
			badge = style.Dim.Render("unknown")
		}

		email := ""
		if acct.Email != "" {
			email = style.Dim.Render(" <" + acct.Email + ">")
		}

		fmt.Printf(" %s %-12s %s%s\n", marker, handle, badge, email)
	}

	fmt.Println()
	fmt.Printf(" %s %d available, %d limited\n",
		style.Info.Render("Summary:"), available, limited)

	return nil
}

// Scan command flags
var (
	scanUpdate bool
)

var quotaScanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Detect rate-limited sessions",
	Long: `Scan all Gas Town tmux sessions for rate-limit indicators.

Captures recent pane output from each session and checks for rate-limit
messages. Reports which sessions are blocked and which account they use.

Use --update to automatically update quota state with detected limits.

Examples:
  gt quota scan              # Report rate-limited sessions
  gt quota scan --update     # Report and update quota state
  gt quota scan --json       # JSON output`,
	RunE: runQuotaScan,
}

func runQuotaScan(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	// Load accounts config
	accountsPath := constants.MayorAccountsPath(townRoot)
	acctCfg, loadErr := config.LoadAccountsConfig(accountsPath)
	// acctCfg can be nil if no accounts configured — scan still works

	// Create scanner
	t := ttmux.NewTmux()
	scanner, err := quota.NewScanner(t, nil, acctCfg)
	if err != nil {
		return fmt.Errorf("creating scanner: %w", err)
	}

	results, err := scanner.ScanAll()
	if err != nil {
		return fmt.Errorf("scanning sessions: %w", err)
	}

	// Optionally update quota state
	if scanUpdate && loadErr == nil && acctCfg != nil {
		if err := updateQuotaState(townRoot, results, acctCfg); err != nil {
			return fmt.Errorf("updating quota state: %w", err)
		}
	}

	if quotaJSON {
		return printScanJSON(results)
	}
	return printScanText(results)
}

func updateQuotaState(townRoot string, results []quota.ScanResult, acctCfg *config.AccountsConfig) error {
	mgr := quota.NewManager(townRoot)
	return mgr.WithLock(func() error {
		state, err := mgr.Load()
		if err != nil {
			return err
		}
		mgr.EnsureAccountsTracked(state, acctCfg.Accounts)

		now := time.Now().UTC().Format(time.RFC3339)
		for _, r := range results {
			if r.RateLimited && r.AccountHandle != "" {
				existing := state.Accounts[r.AccountHandle]
				state.Accounts[r.AccountHandle] = config.AccountQuotaState{
					Status:    config.QuotaStatusLimited,
					LimitedAt: now,
					ResetsAt:  r.ResetsAt,
					LastUsed:  existing.LastUsed,
				}
			}
		}

		return mgr.SaveUnlocked(state)
	})
}

func printScanJSON(results []quota.ScanResult) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(results)
}

func printScanText(results []quota.ScanResult) error {
	limited := 0

	for _, r := range results {
		if r.RateLimited {
			limited++
			account := r.AccountHandle
			if account == "" {
				account = "(unknown)"
			}
			resets := ""
			if r.ResetsAt != "" {
				resets = style.Dim.Render(" resets " + r.ResetsAt)
			}
			fmt.Printf(" %s %-25s %s %s%s\n",
				style.Error.Render("!"),
				r.Session,
				style.Dim.Render("account:"),
				account,
				resets,
			)
		}
	}

	if limited == 0 {
		fmt.Printf(" %s No rate-limited sessions detected (%d scanned)\n",
			style.SuccessPrefix, len(results))
	} else {
		fmt.Println()
		fmt.Printf(" %s %d of %d sessions rate-limited\n",
			style.Warning.Render("Summary:"), limited, len(results))
	}

	return nil
}

// Rotate command flags
var (
	rotateDryRun bool
)

var quotaRotateCmd = &cobra.Command{
	Use:   "rotate",
	Short: "Swap blocked sessions to available accounts",
	Long: `Rotate rate-limited sessions to available accounts.

Scans all sessions for rate limits, plans account assignments using
least-recently-used ordering, and restarts blocked sessions with fresh accounts.

The rotation process:
  1. Scans all Gas Town sessions for rate-limit indicators
  2. Selects available accounts (LRU order)
  3. Updates tmux session environment with new CLAUDE_CONFIG_DIR
  4. Restarts blocked sessions via respawn-pane

Examples:
  gt quota rotate              # Rotate all blocked sessions
  gt quota rotate --dry-run    # Show plan without executing
  gt quota rotate --json       # JSON output`,
	RunE: runQuotaRotate,
}

func runQuotaRotate(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	// Load accounts config (required for rotation)
	accountsPath := constants.MayorAccountsPath(townRoot)
	acctCfg, err := config.LoadAccountsConfig(accountsPath)
	if err != nil {
		return fmt.Errorf("no accounts configured (run 'gt account add' first): %w", err)
	}
	if len(acctCfg.Accounts) < 2 {
		return fmt.Errorf("need at least 2 accounts for rotation (have %d)", len(acctCfg.Accounts))
	}

	// Create scanner and plan rotation
	t := ttmux.NewTmux()
	scanner, err := quota.NewScanner(t, nil, acctCfg)
	if err != nil {
		return fmt.Errorf("creating scanner: %w", err)
	}

	mgr := quota.NewManager(townRoot)
	plan, err := quota.PlanRotation(scanner, mgr, acctCfg)
	if err != nil {
		return fmt.Errorf("planning rotation: %w", err)
	}

	if len(plan.LimitedSessions) == 0 {
		fmt.Printf(" %s No rate-limited sessions detected\n", style.SuccessPrefix)
		return nil
	}

	if len(plan.Assignments) == 0 {
		fmt.Printf(" %s %d sessions rate-limited but no available accounts to rotate to\n",
			style.WarningPrefix, len(plan.LimitedSessions))
		return nil
	}

	// Sort sessions for deterministic output
	sortedSessions := slices.Sorted(maps.Keys(plan.Assignments))

	// Show plan (text only — skip for JSON consumers)
	if !quotaJSON {
		fmt.Println(style.Bold.Render("Rotation Plan"))
		fmt.Println()
		for _, session := range sortedSessions {
			newAccount := plan.Assignments[session]
			var oldAccount string
			for _, r := range plan.LimitedSessions {
				if r.Session == session {
					oldAccount = r.AccountHandle
					break
				}
			}
			if oldAccount == "" {
				oldAccount = "(unknown)"
			}
			fmt.Printf(" %s %-25s %s → %s\n",
				style.ArrowPrefix, session,
				style.Dim.Render(oldAccount),
				style.Success.Render(newAccount),
			)
		}
		unassigned := len(plan.LimitedSessions) - len(plan.Assignments)
		if unassigned > 0 {
			fmt.Printf("\n %s %d sessions cannot be rotated (not enough available accounts)\n",
				style.WarningPrefix, unassigned)
		}
	}

	if rotateDryRun {
		if !quotaJSON {
			fmt.Println()
			fmt.Println(style.Dim.Render(" (dry run — no changes made)"))
		}
		return nil
	}

	// Execute rotation (Rotator holds the lock for the entire lifecycle:
	// load → rotate all sessions → single save).
	if !quotaJSON {
		fmt.Println()
	}
	rotator := quota.NewRotator(t, t, mgr, acctCfg, buildRestartCommand, quotaLogger{},
		townRoot, "" /* agentName: default "claude" */, symlinkSessionToConfigDir)
	results := rotator.Execute(plan, sortedSessions)

	if !quotaJSON {
		for _, result := range results {
			if result.Session == "" && result.Error != "" {
				// Lifecycle error (lock acquisition or final save failure).
				fmt.Printf(" %s %s\n", style.ErrorPrefix, result.Error)
			} else if result.Rotated {
				resumeInfo := ""
				if result.ResumedSession != "" {
					resumeInfo = style.Dim.Render(" (resumed)")
				}
				fmt.Printf(" %s %s → %s%s\n", style.SuccessPrefix, result.Session, result.NewAccount, resumeInfo)
			} else if result.Error != "" {
				fmt.Printf(" %s %s: %s\n", style.ErrorPrefix, result.Session, result.Error)
			}
		}
	}

	if quotaJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}

	return nil
}

var quotaClearCmd = &cobra.Command{
	Use:   "clear [handle...]",
	Short: "Mark account(s) as available again",
	Long: `Clear the rate-limited status for one or more accounts, marking them available.

When no handles are specified, all limited accounts are cleared.

Examples:
  gt quota clear              # Clear all limited accounts
  gt quota clear work         # Clear a specific account
  gt quota clear work personal`,
	RunE: runQuotaClear,
}

func runQuotaClear(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	mgr := quota.NewManager(townRoot)

	if len(args) == 0 {
		// Clear all limited accounts
		state, err := mgr.Load()
		if err != nil {
			return fmt.Errorf("loading quota state: %w", err)
		}
		cleared := 0
		for handle, acctState := range state.Accounts {
			if acctState.Status == config.QuotaStatusLimited || acctState.Status == config.QuotaStatusCooldown {
				if err := mgr.MarkAvailable(handle); err != nil {
					return fmt.Errorf("clearing %s: %w", handle, err)
				}
				fmt.Printf(" %s %s → available\n", style.SuccessPrefix, handle)
				cleared++
			}
		}
		if cleared == 0 {
			fmt.Printf(" %s No limited accounts to clear\n", style.SuccessPrefix)
		}
		return nil
	}

	for _, handle := range args {
		if err := mgr.MarkAvailable(handle); err != nil {
			return fmt.Errorf("clearing %s: %w", handle, err)
		}
		fmt.Printf(" %s %s → available\n", style.SuccessPrefix, handle)
	}
	return nil
}

func init() {
	quotaStatusCmd.Flags().BoolVar(&quotaJSON, "json", false, "Output as JSON")

	quotaScanCmd.Flags().BoolVar(&quotaJSON, "json", false, "Output as JSON")
	quotaScanCmd.Flags().BoolVar(&scanUpdate, "update", false, "Update quota state with detected limits")

	quotaRotateCmd.Flags().BoolVar(&rotateDryRun, "dry-run", false, "Show plan without executing")
	quotaRotateCmd.Flags().BoolVar(&quotaJSON, "json", false, "Output as JSON")

	quotaCmd.AddCommand(quotaStatusCmd)
	quotaCmd.AddCommand(quotaScanCmd)
	quotaCmd.AddCommand(quotaRotateCmd)
	quotaCmd.AddCommand(quotaClearCmd)

	rootCmd.AddCommand(quotaCmd)
}
