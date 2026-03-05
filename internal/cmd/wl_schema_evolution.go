package cmd

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// SchemaChangeKind describes the type of schema change between two semver strings.
type SchemaChangeKind int

const (
	// SchemaUnchanged means local and upstream versions are identical or local is ahead.
	SchemaUnchanged SchemaChangeKind = iota
	// SchemaMinorChange means upstream added columns or tables (backwards-compatible).
	SchemaMinorChange
	// SchemaMajorChange means upstream made breaking changes that require explicit upgrade.
	SchemaMajorChange
)

// ParseSchemaVersion parses a "MAJOR.MINOR" version string into its components.
func ParseSchemaVersion(s string) (major, minor int, err error) {
	parts := strings.SplitN(strings.TrimSpace(s), ".", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid schema version %q: expected MAJOR.MINOR", s)
	}
	major, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid major version in %q: %w", s, err)
	}
	minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid minor version in %q: %w", s, err)
	}
	return major, minor, nil
}

// ClassifySchemaChange compares local and upstream version strings and returns
// the kind of change. Downgrades (upstream older than local) return SchemaUnchanged.
func ClassifySchemaChange(local, upstream string) (SchemaChangeKind, error) {
	localMajor, localMinor, err := ParseSchemaVersion(local)
	if err != nil {
		return SchemaUnchanged, fmt.Errorf("local version: %w", err)
	}
	upMajor, upMinor, err := ParseSchemaVersion(upstream)
	if err != nil {
		return SchemaUnchanged, fmt.Errorf("upstream version: %w", err)
	}

	switch {
	case upMajor > localMajor:
		return SchemaMajorChange, nil
	case upMajor == localMajor && upMinor > localMinor:
		return SchemaMinorChange, nil
	default:
		return SchemaUnchanged, nil
	}
}

// readDoltSchemaVersion reads schema_version from the _meta table of a local
// dolt fork. asOf specifies the branch/ref (e.g. "HEAD" or "upstream/main").
// Returns ("", nil) when the _meta table or schema_version row does not exist.
func readDoltSchemaVersion(doltPath, forkDir, asOf string) (string, error) {
	var query string
	if asOf == "" || asOf == "HEAD" {
		query = "SELECT value FROM _meta WHERE `key` = 'schema_version';"
	} else {
		query = fmt.Sprintf(
			"SELECT value FROM _meta AS OF '%s' WHERE `key` = 'schema_version';",
			asOf,
		)
	}

	cmd := exec.Command(doltPath, "sql", "-r", "csv", "-q", query)
	cmd.Dir = forkDir
	out, err := cmd.Output()
	if err != nil {
		// _meta may not exist on older forks — treat as unknown, not fatal.
		return "", nil
	}

	// Output format: "value\n<version>\n"
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return "", nil
	}
	version := strings.TrimSpace(lines[1])
	return version, nil
}

// checkSchemaEvolution fetches upstream version metadata and classifies any
// schema change. It prints an informational line for MINOR bumps. For MAJOR
// bumps it returns a descriptive error unless upgrade is true.
//
// Precondition: caller has already run `dolt fetch upstream` so that
// upstream/main is available for AS OF queries.
//
// Returns (false, nil) when the fork lacks a _meta table or schema_version row
// (pre-versioned fork) — the pull proceeds without interruption.
func checkSchemaEvolution(doltPath, forkDir string, upgrade bool) error {
	localVer, err := readDoltSchemaVersion(doltPath, forkDir, "HEAD")
	if err != nil || localVer == "" {
		return nil // pre-versioned fork — skip check
	}

	upstreamVer, err := readDoltSchemaVersion(doltPath, forkDir, "upstream/main")
	if err != nil || upstreamVer == "" {
		return nil // upstream has no version info — skip check
	}

	kind, err := ClassifySchemaChange(localVer, upstreamVer)
	if err != nil {
		return fmt.Errorf("schema version check: %w", err)
	}

	switch kind {
	case SchemaUnchanged:
		// nothing to report
	case SchemaMinorChange:
		fmt.Printf("  Schema: %s → %s (MINOR — auto-applying)\n", localVer, upstreamVer)
	case SchemaMajorChange:
		if !upgrade {
			return fmt.Errorf(
				"upstream schema version %s is a MAJOR upgrade from your local %s\n\n"+
					"This may require manual data migration. To proceed:\n\n"+
					"  gt wl sync --upgrade\n\n"+
					"Review the upstream CHANGELOG before upgrading.",
				upstreamVer, localVer,
			)
		}
		fmt.Printf("  Schema: %s → %s (MAJOR — upgrading as requested)\n", localVer, upstreamVer)
	}

	return nil
}
