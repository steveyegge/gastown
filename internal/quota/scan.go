package quota

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/util"
)

// ScanResult holds the result of scanning a single tmux session.
type ScanResult struct {
	Session       string `json:"session"`                  // tmux session name
	AccountHandle string `json:"account_handle,omitempty"` // resolved account handle
	RateLimited   bool   `json:"rate_limited"`             // whether rate-limit was detected
	MatchedLine   string `json:"matched_line,omitempty"`   // the line that matched
	ResetsAt      string `json:"resets_at,omitempty"`      // parsed reset time if available
}

// TmuxClient is the interface for tmux operations needed by the scanner.
// This allows testing without a real tmux server.
type TmuxClient interface {
	ListSessions() ([]string, error)
	CapturePane(session string, lines int) (string, error)
	GetEnvironment(session, key string) (string, error)
}

// Scanner detects rate-limited sessions by examining tmux pane content.
type Scanner struct {
	tmux     TmuxClient
	patterns []*regexp.Regexp
	accounts *config.AccountsConfig
}

// NewScanner creates a scanner with the given tmux client and rate-limit patterns.
// If patterns is nil, DefaultRateLimitPatterns are used.
func NewScanner(tmux TmuxClient, patterns []string, accounts *config.AccountsConfig) (*Scanner, error) {
	if len(patterns) == 0 {
		patterns = constants.DefaultRateLimitPatterns
	}

	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := regexp.Compile("(?i)" + p)
		if err != nil {
			return nil, fmt.Errorf("compiling pattern %q: %w", p, err)
		}
		compiled = append(compiled, re)
	}

	return &Scanner{
		tmux:     tmux,
		patterns: compiled,
		accounts: accounts,
	}, nil
}

// scanLines is the number of pane lines to capture for rate-limit detection.
// The rate-limit prompt appears at the bottom of the pane, so 30 lines
// gives plenty of margin.
const scanLines = 30

// ScanAll scans all Gas Town tmux sessions for rate-limit indicators.
// Returns results only for sessions where a rate-limit was detected or
// where an account handle could be resolved.
func (s *Scanner) ScanAll() ([]ScanResult, error) {
	sessions, err := s.tmux.ListSessions()
	if err != nil {
		return nil, fmt.Errorf("listing sessions: %w", err)
	}

	var results []ScanResult
	for _, sess := range sessions {
		if !isGasTownSession(sess) {
			continue
		}

		result := s.scanSession(sess)
		results = append(results, result)
	}

	return results, nil
}

// scanSession examines a single tmux session for rate-limit indicators.
func (s *Scanner) scanSession(session string) ScanResult {
	result := ScanResult{Session: session}

	// Derive account from CLAUDE_CONFIG_DIR
	result.AccountHandle = s.resolveAccountHandle(session)

	// Capture pane content
	content, err := s.tmux.CapturePane(session, scanLines)
	if err != nil {
		// Can't capture — session might be dead. Not rate-limited.
		return result
	}

	// Check each line against patterns
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		for _, re := range s.patterns {
			if re.MatchString(line) {
				result.RateLimited = true
				result.MatchedLine = line
				result.ResetsAt = parseResetTime(line)
				return result
			}
		}
	}

	return result
}

// resolveAccountHandle maps a session's active account back to a handle.
// Checks GT_QUOTA_ACCOUNT first (set by keychain swap rotation), then
// falls back to matching CLAUDE_CONFIG_DIR against registered accounts.
func (s *Scanner) resolveAccountHandle(session string) string {
	if s.accounts == nil {
		return ""
	}

	// After keychain swap, the config dir still maps to the old account.
	// GT_QUOTA_ACCOUNT records which account's token is actually active.
	if override, err := s.tmux.GetEnvironment(session, "GT_QUOTA_ACCOUNT"); err == nil {
		override = strings.TrimSpace(override)
		if override != "" {
			if _, ok := s.accounts.Accounts[override]; ok {
				return override
			}
		}
	}

	configDir, err := s.tmux.GetEnvironment(session, "CLAUDE_CONFIG_DIR")
	if err != nil {
		return "" // No CLAUDE_CONFIG_DIR = using default config
	}

	configDir = strings.TrimSpace(configDir)
	for handle, acct := range s.accounts.Accounts {
		// Compare normalized paths (accounts may use ~/... while tmux has expanded)
		if acct.ConfigDir == configDir || util.ExpandHome(acct.ConfigDir) == configDir {
			return handle
		}
	}

	return "" // CLAUDE_CONFIG_DIR doesn't match any registered account
}

// isGasTownSession returns true if the session name belongs to Gas Town.
// Uses the prefix registry to check for known rig prefixes (gt-, bd-, etc.)
// and the hq- prefix for town-level services.
func isGasTownSession(sess string) bool {
	return session.IsKnownSession(sess)
}

// parseResetTime attempts to extract the reset time from a rate-limit message.
// Examples:
//
//	"You've hit your limit · resets 7pm (America/Los_Angeles)" → "7pm (America/Los_Angeles)"
//	"resets 3:00 AM PST" → "3:00 AM PST"
var resetTimePattern = regexp.MustCompile(`(?i)\bresets\s+(.+)`)

func parseResetTime(line string) string {
	m := resetTimePattern.FindStringSubmatch(line)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}
