// Package deps manages external dependencies for Gas Town.
package deps

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// MinBeadsVersion is the minimum compatible beads version for this Gas Town release.
// Update this when Gas Town requires new beads features.
const MinBeadsVersion = "0.43.0"

// BeadsInstallPath is the go install path for beads.
const BeadsInstallPath = "github.com/steveyegge/beads/cmd/bd@latest"

var (
	// resolvedBeadsPath caches the resolved beads binary path
	resolvedBeadsPath string
	beadsPathOnce     sync.Once
	beadsPathErr      error
)

// BeadsPath returns the absolute path to the beads (bd) binary.
// This ensures consistent use of the correct version across all calls.
// Priority order:
// 1. $BEADS_BIN environment variable (explicit override)
// 2. ~/go/bin/bd (canonical Go install location)
// 3. First bd in PATH that meets version requirements
func BeadsPath() (string, error) {
	beadsPathOnce.Do(func() {
		resolvedBeadsPath, beadsPathErr = resolveBeadsPath()
	})
	return resolvedBeadsPath, beadsPathErr
}

// resolveBeadsPath finds the correct beads binary.
func resolveBeadsPath() (string, error) {
	// 1. Check explicit override
	if envPath := os.Getenv("BEADS_BIN"); envPath != "" {
		if _, err := os.Stat(envPath); err == nil {
			version := getVersionAt(envPath)
			if version != "" && compareVersions(version, MinBeadsVersion) >= 0 {
				return envPath, nil
			}
			if version != "" {
				return "", fmt.Errorf("BEADS_BIN points to bd %s (minimum: %s)", version, MinBeadsVersion)
			}
		}
		return "", fmt.Errorf("BEADS_BIN points to non-existent file: %s", envPath)
	}

	// 2. Check canonical Go install location
	home, _ := os.UserHomeDir()
	goPath := filepath.Join(home, "go", "bin", "bd")
	if _, err := os.Stat(goPath); err == nil {
		version := getVersionAt(goPath)
		if version != "" && compareVersions(version, MinBeadsVersion) >= 0 {
			return goPath, nil
		}
		// Fall through if ~/go/bin/bd is too old
	}

	// 3. Search PATH for compatible version
	pathEnv := os.Getenv("PATH")
	for _, dir := range filepath.SplitList(pathEnv) {
		candidate := filepath.Join(dir, "bd")
		if _, err := os.Stat(candidate); err == nil {
			version := getVersionAt(candidate)
			if version != "" && compareVersions(version, MinBeadsVersion) >= 0 {
				return candidate, nil
			}
		}
	}

	return "", fmt.Errorf("no compatible beads (bd) found (minimum: %s)\n\nInstall with: go install %s",
		MinBeadsVersion, BeadsInstallPath)
}

// getVersionAt runs the specified bd binary and returns its version.
func getVersionAt(path string) string {
	cmd := exec.Command(path, "version")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return parseBeadsVersion(string(output))
}

// ResetBeadsPath clears the cached beads path (for testing).
func ResetBeadsPath() {
	beadsPathOnce = sync.Once{}
	resolvedBeadsPath = ""
	beadsPathErr = nil
}

// BeadsStatus represents the state of the beads installation.
type BeadsStatus int

const (
	BeadsOK       BeadsStatus = iota // bd found, version compatible
	BeadsNotFound                    // bd not in PATH
	BeadsTooOld                      // bd found but version too old
	BeadsUnknown                     // bd found but couldn't parse version
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

	// Get version
	cmd := exec.Command("bd", "version")
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
	// Match patterns like "bd version 0.43.0" or "bd version 0.43.0 (dev: ...)"
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
