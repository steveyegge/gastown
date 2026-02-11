// Package beads provides routing helpers for prefix-based beads resolution.
package beads

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/config"
)

// Route represents a prefix-to-path routing rule.
// This mirrors the structure in bd's internal/routing package.
type Route struct {
	Prefix string `json:"prefix"` // Issue ID prefix (e.g., "gt-")
	Path   string `json:"path"`   // Relative path to .beads directory from town root
}

// RoutesFileName is the name of the routes configuration file.
const RoutesFileName = "routes.jsonl"

// LoadRoutes loads routes, trying beads daemon first (via bd route list), falling back to routes.jsonl.
// Returns an empty slice if no routes are found.
func LoadRoutes(beadsDir string) ([]Route, error) {
	// Try loading from beads daemon first
	routes, err := loadRoutesFromDaemon(beadsDir)
	if err == nil && len(routes) > 0 {
		if os.Getenv("GT_DEBUG_ROUTING") != "" {
			fmt.Fprintf(os.Stderr, "[routing] Loaded %d routes from beads daemon\n", len(routes))
		}
		return routes, nil
	}
	if os.Getenv("GT_DEBUG_ROUTING") != "" && err != nil {
		fmt.Fprintf(os.Stderr, "[routing] Daemon route query failed: %v, falling back to file\n", err)
	}

	// Fall back to routes.jsonl
	return LoadRoutesFromFile(beadsDir)
}

// loadRoutesFromDaemon queries route beads via the bd daemon.
// Returns routes parsed from beads, or error if unavailable.
func loadRoutesFromDaemon(beadsDir string) ([]Route, error) {
	// Run bd list --type=route --json from the beads directory
	cmd := exec.Command("bd", "list", "--type=route", "--json")
	cmd.Dir = filepath.Dir(beadsDir) // Run from parent of .beads (the rig dir)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bd list failed: %w", err)
	}

	// Parse JSON output - bd list returns array of issues
	var rawRoutes []struct {
		ID     string   `json:"id"`
		Title  string   `json:"title"`
		Status string   `json:"status"`
		Labels []string `json:"labels"`
	}
	if err := json.Unmarshal(output, &rawRoutes); err != nil {
		return nil, fmt.Errorf("parsing route data: %w", err)
	}

	// Convert to Route structs, filtering to open routes only
	var routes []Route
	for _, raw := range rawRoutes {
		if raw.Status == "closed" {
			continue
		}

		// Try to extract from labels first (our bead_route.go format)
		var prefix, path string
		for _, label := range raw.Labels {
			switch {
			case strings.HasPrefix(label, "prefix:"):
				prefix = strings.TrimPrefix(label, "prefix:") + "-"
			case strings.HasPrefix(label, "path:"):
				path = strings.TrimPrefix(label, "path:")
			}
		}

		// Fall back to parsing title format "prefix → path"
		if prefix == "" || path == "" {
			r := parseRouteFromTitle(raw.Title)
			if r.Prefix != "" {
				prefix = r.Prefix
			}
			if r.Path != "" {
				path = r.Path
			}
		}

		if prefix != "" && path != "" {
			routes = append(routes, Route{Prefix: prefix, Path: path})
		}
	}

	return routes, nil
}

// parseRouteFromTitle extracts a Route from a route bead title.
// Route beads use title format "prefix → path" (e.g., "gt- → gastown").
func parseRouteFromTitle(title string) Route {
	var parts []string
	if strings.Contains(title, " → ") {
		parts = strings.SplitN(title, " → ", 2)
	} else if strings.Contains(title, " -> ") {
		parts = strings.SplitN(title, " -> ", 2)
	}

	if len(parts) != 2 {
		return Route{}
	}

	prefix := strings.TrimSpace(parts[0])
	path := strings.TrimSpace(parts[1])

	// Normalize prefix to include hyphen
	if prefix != "" && !strings.HasSuffix(prefix, "-") {
		prefix = prefix + "-"
	}

	// Normalize special path values
	if path == "town root" || path == ".beads" {
		path = "."
	}

	return Route{Prefix: prefix, Path: path}
}

// LoadRoutesFromFile loads routes from routes.jsonl in the given beads directory.
// Returns an empty slice if the file doesn't exist.
func LoadRoutesFromFile(beadsDir string) ([]Route, error) {
	routesPath := filepath.Join(beadsDir, RoutesFileName)
	file, err := os.Open(routesPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No routes file is not an error
		}
		return nil, err
	}
	defer file.Close()

	var routes []Route
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue // Skip empty lines and comments
		}

		var route Route
		if err := json.Unmarshal([]byte(line), &route); err != nil {
			continue // Skip malformed lines
		}
		if route.Prefix != "" && route.Path != "" {
			routes = append(routes, route)
		}
	}

	return routes, scanner.Err()
}

// AppendRoute appends a route to routes.jsonl in the town's beads directory.
// If the prefix already exists, it updates the path.
func AppendRoute(townRoot string, route Route) error {
	beadsDir := filepath.Join(townRoot, ".beads")
	return AppendRouteToDir(beadsDir, route)
}

// AppendRouteToDir appends a route to routes.jsonl in the given beads directory.
// If the prefix already exists, it updates the path.
func AppendRouteToDir(beadsDir string, route Route) error {
	// Load existing routes
	routes, err := LoadRoutes(beadsDir)
	if err != nil {
		return fmt.Errorf("loading routes: %w", err)
	}

	// Check if prefix already exists
	found := false
	for i, r := range routes {
		if r.Prefix == route.Prefix {
			routes[i].Path = route.Path
			found = true
			break
		}
	}

	if !found {
		routes = append(routes, route)
	}

	// Write back
	return WriteRoutes(beadsDir, routes)
}

// RemoveRoute removes a route by prefix from routes.jsonl.
func RemoveRoute(townRoot string, prefix string) error {
	beadsDir := filepath.Join(townRoot, ".beads")

	// Load existing routes
	routes, err := LoadRoutes(beadsDir)
	if err != nil {
		return fmt.Errorf("loading routes: %w", err)
	}

	// Filter out the prefix
	var filtered []Route
	for _, r := range routes {
		if r.Prefix != prefix {
			filtered = append(filtered, r)
		}
	}

	// Write back
	return WriteRoutes(beadsDir, filtered)
}

// WriteRoutes writes routes to routes.jsonl, overwriting existing content.
func WriteRoutes(beadsDir string, routes []Route) error {
	if IsDaemonMode() {
		return fmt.Errorf("WriteRoutes not supported in daemon mode; use bd route commands instead")
	}
	// Ensure beads directory exists
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		return fmt.Errorf("creating beads directory: %w", err)
	}

	routesPath := filepath.Join(beadsDir, RoutesFileName)

	file, err := os.Create(routesPath)
	if err != nil {
		return fmt.Errorf("creating routes file: %w", err)
	}
	defer file.Close()

	for _, r := range routes {
		data, err := json.Marshal(r)
		if err != nil {
			return fmt.Errorf("marshaling route: %w", err)
		}
		if _, err := file.Write(data); err != nil {
			return fmt.Errorf("writing route: %w", err)
		}
		if _, err := file.WriteString("\n"); err != nil {
			return fmt.Errorf("writing newline: %w", err)
		}
	}

	return nil
}

// GetTownBeadsPath returns the path to town-level beads directory.
// Town beads store hq-* prefixed issues including Mayor, Deacon, and role beads.
// The townRoot should be the Gas Town root directory (e.g., ~/gt).
func GetTownBeadsPath(townRoot string) string {
	return filepath.Join(townRoot, ".beads")
}

// GetRoutePathForRigName returns the route path for a given rig name.
// The rig name is matched against the first component of each route's path.
// Returns the full route path (e.g., "gastown/mayor/rig") or empty string if not found.
// This is the inverse of extracting the rig name from a route path.
func GetRoutePathForRigName(townRoot, rigName string) string {
	beadsDir := filepath.Join(townRoot, ".beads")
	routes, err := LoadRoutes(beadsDir)
	if err != nil || routes == nil {
		return ""
	}

	for _, r := range routes {
		// Skip town-level routes
		if r.Path == "." || r.Path == "" {
			continue
		}
		// Extract rig name from first path component
		parts := strings.SplitN(r.Path, "/", 2)
		if len(parts) > 0 && parts[0] == rigName {
			return r.Path
		}
	}

	return ""
}

// GetPrefixForRig returns the beads prefix for a given rig name.
// The prefix is returned without the trailing hyphen (e.g., "bd" not "bd-").
// If the rig is not found in routes, returns "gt" as the default.
// The townRoot should be the Gas Town root directory (e.g., ~/gt).
func GetPrefixForRig(townRoot, rigName string) string {
	beadsDir := filepath.Join(townRoot, ".beads")
	routes, err := LoadRoutes(beadsDir)
	if err != nil || routes == nil {
		return config.GetRigPrefix(townRoot, rigName)
	}

	// Look for a route where the path starts with the rig name
	// Routes paths are like "gastown/mayor/rig" or "beads/mayor/rig"
	for _, r := range routes {
		parts := strings.SplitN(r.Path, "/", 2)
		if len(parts) > 0 && parts[0] == rigName {
			// Return prefix without trailing hyphen
			return strings.TrimSuffix(r.Prefix, "-")
		}
	}

	return config.GetRigPrefix(townRoot, rigName)
}

// FindConflictingPrefixes checks for duplicate prefixes in routes.
// Returns a map of prefix -> list of paths that use it.
func FindConflictingPrefixes(beadsDir string) (map[string][]string, error) {
	routes, err := LoadRoutes(beadsDir)
	if err != nil {
		return nil, err
	}

	// Group by prefix
	prefixPaths := make(map[string][]string)
	for _, r := range routes {
		prefixPaths[r.Prefix] = append(prefixPaths[r.Prefix], r.Path)
	}

	// Filter to only conflicts (more than one path per prefix)
	conflicts := make(map[string][]string)
	for prefix, paths := range prefixPaths {
		if len(paths) > 1 {
			conflicts[prefix] = paths
		}
	}

	return conflicts, nil
}

// ExtractPrefix extracts the prefix from a bead ID.
// For example, "ap-qtsup.16" returns "ap-", "hq-cv-abc" returns "hq-".
// Returns empty string if no valid prefix found (empty input, no hyphen,
// or hyphen at position 0 which would indicate an invalid prefix).
func ExtractPrefix(beadID string) string {
	if beadID == "" {
		return ""
	}

	idx := strings.Index(beadID, "-")
	if idx <= 0 {
		return ""
	}

	return beadID[:idx+1]
}

// GetRigPathForPrefix returns the rig path for a given bead ID prefix.
// The townRoot should be the Gas Town root directory (e.g., ~/gt).
// Returns the full absolute path to the rig directory, or empty string if not found.
// For town-level beads (path="."), returns townRoot.
func GetRigPathForPrefix(townRoot, prefix string) string {
	beadsDir := filepath.Join(townRoot, ".beads")
	routes, err := LoadRoutes(beadsDir)
	if err != nil || routes == nil {
		return ""
	}

	for _, r := range routes {
		if r.Prefix == prefix {
			if r.Path == "." {
				return townRoot // Town-level beads
			}
			return filepath.Join(townRoot, r.Path)
		}
	}

	return ""
}

// ResolveHookDir determines the directory for running bd update on a bead.
// Since bd update doesn't support routing or redirects, we must resolve the
// actual rig directory from the bead's prefix. hookWorkDir is only used as
// a fallback if prefix resolution fails.
func ResolveHookDir(townRoot, beadID, hookWorkDir string) string {
	// Always try prefix resolution first - bd update needs the actual rig dir
	prefix := ExtractPrefix(beadID)
	if rigPath := GetRigPathForPrefix(townRoot, prefix); rigPath != "" {
		return rigPath
	}
	// Fallback to hookWorkDir if provided
	if hookWorkDir != "" {
		return hookWorkDir
	}
	return townRoot
}

// ResolveToExternalRef converts a bead ID to an external reference format
// compatible with bd's routing expectations.
//
// External refs use the format "external:<project>:<bead-id>" where project
// is derived from the route path (e.g., "gastown", "beads"), not from the
// bead ID prefix.
//
// Examples (with routes {"prefix":"gt-","path":"gastown/mayor/rig"}):
//   - "gt-mol-abc123" -> "external:gastown:gt-mol-abc123"
//   - "bd-xyz" -> "external:beads:bd-xyz" (if route exists)
//   - "hq-abc" -> "" (hq- beads are local, no external ref needed)
//   - "unknown-id" -> "" (no matching route)
//
// Returns empty string if:
//   - The bead ID has an "hq-" prefix (local to town beads)
//   - No route matches the bead ID prefix
//   - The route path doesn't contain a project name
func ResolveToExternalRef(townRoot, beadID string) string {
	// HQ beads are local - no external ref needed
	if strings.HasPrefix(beadID, "hq-") {
		return ""
	}

	prefix := ExtractPrefix(beadID)
	if prefix == "" {
		return ""
	}

	beadsDir := filepath.Join(townRoot, ".beads")
	routes, err := LoadRoutes(beadsDir)
	if err != nil || routes == nil {
		return ""
	}

	for _, route := range routes {
		if route.Prefix == prefix {
			// Extract project name from route path (first path component)
			// e.g., "gastown/mayor/rig" -> "gastown"
			parts := strings.SplitN(route.Path, "/", 2)
			if len(parts) > 0 && parts[0] != "" && parts[0] != "." {
				return fmt.Sprintf("external:%s:%s", parts[0], beadID)
			}
		}
	}

	return ""
}
