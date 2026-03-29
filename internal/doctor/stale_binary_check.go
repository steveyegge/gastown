package doctor

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/deps"
	"github.com/steveyegge/gastown/internal/version"
)

var discoverGTBinaries = version.DiscoverGTBinaries

// StaleBinaryCheck inspects discovered gt binaries, marks the active one, and
// warns about PATH shadowing or stale source builds.
type StaleBinaryCheck struct {
	BaseCheck
}

// NewStaleBinaryCheck creates a new gt binary inventory check.
func NewStaleBinaryCheck() *StaleBinaryCheck {
	return &StaleBinaryCheck{
		BaseCheck: BaseCheck{
			CheckName:        "stale-binary",
			CheckDescription: "List discovered gt binaries and detect shadowing or stale builds",
			CheckCategory:    CategoryInfrastructure,
		},
	}
}

// Run checks the active gt binary, PATH ordering, and any stale dev build signals.
func (c *StaleBinaryCheck) Run(ctx *CheckContext) *CheckResult {
	inventory := discoverGTBinaries()
	active := inventory.Active()
	if active == nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "Could not determine the active gt binary",
			FixHint: "Reinstall gt and verify the intended binary is executable",
		}
	}

	details := formatGTBinaryDetails(inventory)
	issues := summarizeGTBinaryIssues(inventory)
	if len(issues) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: fmt.Sprintf("Active gt: %s", shortGTBinaryLabel(*active)),
			Details: details,
		}
	}

	msg := issues[0]
	if len(issues) > 1 {
		msg = fmt.Sprintf("%s (%d issue(s))", msg, len(issues))
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: msg,
		Details: details,
		FixHint: "Verify 'command -v gt' resolves to the intended install, then remove, rename, or reorder older gt binaries earlier on PATH so the right binary wins",
	}
}

func shortGTBinaryLabel(bin version.GTBinaryCandidate) string {
	path := bin.Path
	if bin.ResolvedPath != "" && bin.ResolvedPath != bin.Path {
		path = fmt.Sprintf("%s -> %s", path, bin.ResolvedPath)
	}

	meta := []string{}
	if bin.VersionInfo.Version != "" {
		meta = append(meta, "gt "+bin.VersionInfo.Version)
	}
	if bin.VersionInfo.Build != "" {
		meta = append(meta, bin.VersionInfo.Build)
	}
	if bin.VersionInfo.Detail != "" {
		meta = append(meta, bin.VersionInfo.Detail)
	}
	if len(meta) == 0 {
		return path
	}
	return fmt.Sprintf("%s [%s]", path, strings.Join(meta, ", "))
}

func formatGTBinaryDetails(inventory *version.GTBinaryInventory) []string {
	if inventory == nil || len(inventory.Binaries) == 0 {
		return nil
	}

	details := make([]string, 0, len(inventory.Binaries)+1)
	for _, bin := range inventory.Binaries {
		roles := []string{}
		if bin.Active {
			roles = append(roles, "active")
		}
		if bin.PathPrimary {
			roles = append(roles, fmt.Sprintf("PATH[%d]", bin.PATHIndex))
		} else if bin.OnPATH {
			roles = append(roles, fmt.Sprintf("shadowed PATH[%d]", bin.PATHIndex))
		} else {
			roles = append(roles, "not on PATH")
		}

		line := fmt.Sprintf("%s: %s", strings.Join(roles, " "), shortGTBinaryLabel(bin))

		meta := []string{}
		if !bin.VersionInfo.Recognized() {
			if bin.VersionInfo.MainPackage != "" {
				meta = append(meta, "not recognized as Gas Town gt ("+bin.VersionInfo.MainPackage+")")
			} else if bin.VersionInfo.Error != nil {
				meta = append(meta, "not recognized as Gas Town gt")
			}
		}
		if bin.StaleInfo != nil && bin.StaleInfo.IsStale {
			stale := fmt.Sprintf("stale vs repo %s", version.ShortCommit(bin.StaleInfo.RepoCommit))
			if bin.StaleInfo.CommitsBehind > 0 {
				stale = fmt.Sprintf("%s (%d commits behind)", stale, bin.StaleInfo.CommitsBehind)
			}
			meta = append(meta, stale)
		}
		if bin.VersionInfo.Error != nil && bin.VersionInfo.Recognized() {
			meta = append(meta, "version probe failed: "+bin.VersionInfo.Error.Error())
		}
		if len(meta) > 0 {
			line = fmt.Sprintf("%s; %s", line, strings.Join(meta, "; "))
		}

		details = append(details, line)
	}

	if inventory.RepoRoot != "" {
		details = append(details, "repo reference: "+filepath.Clean(inventory.RepoRoot))
	}

	return details
}

func summarizeGTBinaryIssues(inventory *version.GTBinaryInventory) []string {
	if inventory == nil {
		return []string{"Could not inspect gt binaries"}
	}

	active := inventory.Active()
	if active == nil {
		return []string{"Could not determine the active gt binary"}
	}

	issues := []string{}
	if !active.VersionInfo.Recognized() {
		issues = append(issues, fmt.Sprintf("Active gt (%s) could not be identified as a Gas Town binary", active.Path))
	}

	if pathPrimary := inventory.PathPrimary(); pathPrimary != nil {
		if inventory.ActiveIndex != inventory.PathPrimaryIndex && !sameBinaryTarget(*active, *pathPrimary) {
			issues = append(issues,
				fmt.Sprintf("Current gt (%s) differs from PATH-selected gt (%s)", active.Path, pathPrimary.Path))
		}
	}

	pathTargets := map[string]bool{}
	for _, bin := range inventory.Binaries {
		if !bin.OnPATH {
			continue
		}
		pathTargets[binaryTargetKey(bin)] = true
	}
	pathCount := len(pathTargets)
	if pathCount > 1 {
		issues = append(issues,
			fmt.Sprintf("Discovered %d gt binaries on PATH; active binary shadows %d later entries", pathCount, pathCount-1))
	}

	if active.StaleInfo != nil && active.StaleInfo.IsStale {
		if active.StaleInfo.CommitsBehind > 0 {
			issues = append(issues,
				fmt.Sprintf("Active gt dev build is %d commit(s) behind the source repo", active.StaleInfo.CommitsBehind))
		} else {
			issues = append(issues, "Active gt dev build is stale relative to the source repo")
		}
	}

	for _, bin := range inventory.Binaries {
		if bin.PathPrimary || !bin.OnPATH {
			continue
		}
		if sameBinaryTarget(*active, bin) {
			continue
		}
		if newerShadowedBinary(*active, bin) {
			issues = append(issues,
				fmt.Sprintf("Active gt %s is older than shadowed gt %s at %s", active.VersionInfo.Version, bin.VersionInfo.Version, bin.Path))
			break
		}
	}

	return dedupeStrings(issues)
}

func newerShadowedBinary(active, shadowed version.GTBinaryCandidate) bool {
	if active.VersionInfo.Version == "" || shadowed.VersionInfo.Version == "" {
		return false
	}
	if deps.CompareVersions(shadowed.VersionInfo.Version, active.VersionInfo.Version) <= 0 {
		return false
	}
	if !shadowed.VersionInfo.Recognized() {
		return false
	}
	return true
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func sameBinaryTarget(a, b version.GTBinaryCandidate) bool {
	return binaryTargetKey(a) == binaryTargetKey(b)
}

func binaryTargetKey(bin version.GTBinaryCandidate) string {
	if bin.ResolvedPath != "" {
		return filepath.Clean(bin.ResolvedPath)
	}
	return filepath.Clean(bin.Path)
}
