package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// CrashReportCheck looks for recent macOS crash reports related to tmux or Claude.
// This helps diagnose mass session death events.
type CrashReportCheck struct {
	BaseCheck
	crashReports []crashReport // Cached during Run for display
}

// crashReport represents a found crash report file.
type crashReport struct {
	path     string
	name     string
	modTime  time.Time
	process  string // "tmux", "claude", "node", etc.
}

// NewCrashReportCheck creates a new crash report check.
func NewCrashReportCheck() *CrashReportCheck {
	return &CrashReportCheck{
		BaseCheck: BaseCheck{
			CheckName:        "crash-reports",
			CheckDescription: "Check for recent macOS crash reports (tmux, Claude)",
			CheckCategory:    CategoryCleanup,
		},
	}
}

// Run checks for recent crash reports in macOS diagnostic directories.
func (c *CrashReportCheck) Run(ctx *CheckContext) *CheckResult {
	// Only run on macOS
	if runtime.GOOS != "darwin" {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "Crash report check not applicable (non-macOS)",
		}
	}

	// Look for crash reports in the last 24 hours
	lookbackWindow := 24 * time.Hour
	cutoff := time.Now().Add(-lookbackWindow)

	// macOS crash report locations
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "Could not determine home directory",
			Details: []string{err.Error()},
		}
	}

	crashDirs := []string{
		filepath.Join(homeDir, "Library", "Logs", "DiagnosticReports"),
		"/Library/Logs/DiagnosticReports",
	}

	// Processes we care about
	relevantProcesses := []string{
		"tmux",
		"claude",
		"claude-code",
		"node",
	}

	var reports []crashReport

	for _, dir := range crashDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // Directory may not exist
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			name := entry.Name()

			// Check if this is a crash report for a relevant process
			var matchedProcess string
			nameLower := strings.ToLower(name)
			for _, proc := range relevantProcesses {
				if strings.Contains(nameLower, proc) {
					matchedProcess = proc
					break
				}
			}

			if matchedProcess == "" {
				continue
			}

			// Check modification time
			info, err := entry.Info()
			if err != nil {
				continue
			}

			if info.ModTime().Before(cutoff) {
				continue // Too old
			}

			reports = append(reports, crashReport{
				path:    filepath.Join(dir, name),
				name:    name,
				modTime: info.ModTime(),
				process: matchedProcess,
			})
		}
	}

	// Sort by time (most recent first)
	sort.Slice(reports, func(i, j int) bool {
		return reports[i].modTime.After(reports[j].modTime)
	})

	// Cache for display
	c.crashReports = reports

	if len(reports) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No recent crash reports found",
		}
	}

	// Group by process
	processCounts := make(map[string]int)
	for _, r := range reports {
		processCounts[r.process]++
	}

	// Build details
	var details []string
	for _, r := range reports {
		age := time.Since(r.modTime).Round(time.Minute)
		details = append(details, fmt.Sprintf("%s (%s ago): %s", r.process, age, r.name))
	}

	// Build summary
	var summary []string
	for proc, count := range processCounts {
		summary = append(summary, fmt.Sprintf("%d %s", count, proc))
	}

	message := fmt.Sprintf("Found %d crash report(s): %s", len(reports), strings.Join(summary, ", "))

	// tmux crashes are particularly concerning
	status := StatusWarning
	if processCounts["tmux"] > 0 {
		message += " - TMUX CRASHED (may explain session deaths)"
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  status,
		Message: message,
		Details: details,
		FixHint: "Review crash reports in Console.app → User Reports or check ~/Library/Logs/DiagnosticReports/",
	}
}

// Fix does nothing - crash reports are informational.
func (c *CrashReportCheck) Fix(ctx *CheckContext) error {
	return nil
}

// CanFix returns false - crash reports cannot be auto-fixed.
func (c *CrashReportCheck) CanFix() bool {
	return false
}
