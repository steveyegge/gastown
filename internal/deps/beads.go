// Package deps manages external dependencies for Gas Town.
package deps

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// MinBeadsVersion is the minimum compatible beads version for this Gas Town release.
// Update this when Gas Town requires new beads features.
const MinBeadsVersion = "0.52.0"

// BeadsInstallPath is the go install path for beads.
const BeadsInstallPath = "github.com/steveyegge/beads/cmd/bd@latest"

// BeadsStatus represents the state of the beads installation.
type BeadsStatus int

const (
	BeadsOK          BeadsStatus = iota // bd found, version compatible
	BeadsNotFound                       // bd not in PATH
	BeadsTooOld                         // bd found but version too old
	BeadsUnknown                        // bd found but couldn't parse version
)

// CheckBeads checks if bd is installed and compatible.
// Returns status and the installed version (if found).
func CheckBeads() (BeadsStatus, string) {
	// Check if bd exists in PATH
	path, err := exec.LookPath("bd")
	if err != nil {
		return BeadsNotFound, ""
	}
	_ = path // bd found

	// Get version (with timeout to prevent hanging on broken bd installs)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bd", "version")
	output, err := cmd.Output()
	if err != nil {
		return BeadsUnknown, ""
	}

	version := parseBeadsVersion(string(output))
	if version == "" {
		return BeadsUnknown, ""
	}

	// Compare versions
	if compareVersions(version, MinBeadsVersion) < 0 {
		return BeadsTooOld, version
	}

	return BeadsOK, version
}

// EnsureBeads checks for bd and installs it if missing or outdated.
// Returns nil if bd is available and compatible.
// If autoInstall is true, will attempt to install bd when missing.
func EnsureBeads(autoInstall bool) error {
	status, version := CheckBeads()

	switch status {
	case BeadsOK:
		return nil

	case BeadsNotFound:
		if !autoInstall {
			return fmt.Errorf("beads (bd) not found in PATH\n\nInstall with: go install %s", BeadsInstallPath)
		}
		return installBeads()

	case BeadsTooOld:
		return fmt.Errorf("beads version %s is too old (minimum: %s)\n\nUpgrade with: go install %s",
			version, MinBeadsVersion, BeadsInstallPath)

	case BeadsUnknown:
		// Found bd but couldn't determine version - proceed with warning
		return nil
	}

	return nil
}

// installBeads runs go install to install the latest beads.
func installBeads() error {
	fmt.Printf("   beads (bd) not found. Installing...\n")

	cmd := exec.Command("go", "install", BeadsInstallPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install beads: %s\n%s", err, string(output))
	}

	// Verify installation
	status, version := CheckBeads()
	if status == BeadsNotFound {
		return fmt.Errorf("beads installed but not in PATH - ensure $GOPATH/bin is in your PATH")
	}
	if status == BeadsTooOld {
		return fmt.Errorf("installed beads %s but minimum required is %s", version, MinBeadsVersion)
	}

	fmt.Printf("   ✓ Installed beads %s\n", version)
	return nil
}

// parseBeadsVersion extracts version from "bd version X.Y.Z ..." output.
func parseBeadsVersion(output string) string {
	// Match patterns like "bd version 0.52.0" or "bd version 0.52.0 (dev: ...)"
	re := regexp.MustCompile(`bd version (\d+\.\d+\.\d+)`)
	matches := re.FindStringSubmatch(output)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// compareVersions compares two semver strings.
// Returns -1 if a < b, 0 if a == b, 1 if a > b.
func compareVersions(a, b string) int {
	aParts := parseVersion(a)
	bParts := parseVersion(b)

	for i := 0; i < 3; i++ {
		if aParts[i] < bParts[i] {
			return -1
		}
		if aParts[i] > bParts[i] {
			return 1
		}
	}
	return 0
}

// parseVersion parses "X.Y.Z" into [3]int.
func parseVersion(v string) [3]int {
	var parts [3]int
	split := strings.Split(v, ".")
	for i := 0; i < 3 && i < len(split); i++ {
		parts[i], _ = strconv.Atoi(split[i])
	}
	return parts
}
