// Package upgrade provides self-update functionality for the gt binary.
package upgrade

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Version represents a semantic version.
type Version struct {
	Major      int
	Minor      int
	Patch      int
	Prerelease string // e.g., "alpha", "beta.1", "rc.2"
	Raw        string // Original string representation
}

// semverRegex matches versions like v0.2.6, 0.2.6, v1.0.0-alpha, v1.0.0-beta.1
var semverRegex = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:-([a-zA-Z0-9.]+))?$`)

// ParseVersion parses a version string into a Version struct.
// Accepts formats: "v0.2.6", "0.2.6", "v1.0.0-alpha", "v1.0.0-beta.1"
func ParseVersion(s string) (*Version, error) {
	s = strings.TrimSpace(s)
	matches := semverRegex.FindStringSubmatch(s)
	if matches == nil {
		return nil, fmt.Errorf("invalid version format: %q", s)
	}

	major, _ := strconv.Atoi(matches[1])
	minor, _ := strconv.Atoi(matches[2])
	patch, _ := strconv.Atoi(matches[3])

	return &Version{
		Major:      major,
		Minor:      minor,
		Patch:      patch,
		Prerelease: matches[4],
		Raw:        s,
	}, nil
}

// String returns the version as a string without the "v" prefix.
func (v *Version) String() string {
	if v.Prerelease != "" {
		return fmt.Sprintf("%d.%d.%d-%s", v.Major, v.Minor, v.Patch, v.Prerelease)
	}
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// StringWithV returns the version as a string with the "v" prefix.
func (v *Version) StringWithV() string {
	return "v" + v.String()
}

// Compare compares two versions.
// Returns:
//
//	-1 if v < other
//	 0 if v == other
//	 1 if v > other
func (v *Version) Compare(other *Version) int {
	if v.Major != other.Major {
		if v.Major < other.Major {
			return -1
		}
		return 1
	}
	if v.Minor != other.Minor {
		if v.Minor < other.Minor {
			return -1
		}
		return 1
	}
	if v.Patch != other.Patch {
		if v.Patch < other.Patch {
			return -1
		}
		return 1
	}

	// Handle prerelease: a version without prerelease > a version with prerelease
	// e.g., 1.0.0 > 1.0.0-alpha
	if v.Prerelease == "" && other.Prerelease != "" {
		return 1
	}
	if v.Prerelease != "" && other.Prerelease == "" {
		return -1
	}
	if v.Prerelease != other.Prerelease {
		// Simple lexicographic comparison for prerelease tags
		// This handles most cases correctly: alpha < beta < rc
		if v.Prerelease < other.Prerelease {
			return -1
		}
		return 1
	}

	return 0
}

// LessThan returns true if v < other.
func (v *Version) LessThan(other *Version) bool {
	return v.Compare(other) < 0
}

// GreaterThan returns true if v > other.
func (v *Version) GreaterThan(other *Version) bool {
	return v.Compare(other) > 0
}

// Equal returns true if v == other.
func (v *Version) Equal(other *Version) bool {
	return v.Compare(other) == 0
}

// IsMajorUpgrade returns true if upgrading from v to other is a major version change.
func (v *Version) IsMajorUpgrade(other *Version) bool {
	return other.Major > v.Major
}

// IsMinorUpgrade returns true if upgrading from v to other is a minor version change.
func (v *Version) IsMinorUpgrade(other *Version) bool {
	return other.Major == v.Major && other.Minor > v.Minor
}

// MatchesPattern returns true if the version matches a pattern like "0.2.x" or "0.x".
// Useful for checking migration requirements.
func (v *Version) MatchesPattern(pattern string) bool {
	pattern = strings.TrimPrefix(pattern, "v")

	parts := strings.Split(pattern, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return false
	}

	// Check major
	if parts[0] != "x" && parts[0] != "*" {
		major, err := strconv.Atoi(parts[0])
		if err != nil || major != v.Major {
			return false
		}
	}

	// Check minor (if present)
	if len(parts) >= 2 && parts[1] != "x" && parts[1] != "*" {
		minor, err := strconv.Atoi(parts[1])
		if err != nil || minor != v.Minor {
			return false
		}
	}

	// Check patch (if present)
	if len(parts) >= 3 && parts[2] != "x" && parts[2] != "*" {
		patch, err := strconv.Atoi(parts[2])
		if err != nil || patch != v.Patch {
			return false
		}
	}

	return true
}
