// Package beads provides routing helpers for prefix-based beads resolution.
package beads

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Route represents a prefix-to-path routing rule.
// This mirrors the structure in bd's internal/routing package.
type Route struct {
	Prefix string `json:"prefix"` // Issue ID prefix (e.g., "gt-")
	Path   string `json:"path"`   // Relative path to .beads directory from town root
}

// RoutesFileName is the name of the routes configuration file.
const RoutesFileName = "routes.jsonl"

// LoadRoutes loads routes from routes.jsonl in the given beads directory.
// Returns an empty slice if the file doesn't exist.
func LoadRoutes(beadsDir string) ([]Route, error) {
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

// GetPrefixForRig returns the beads prefix for a given rig name.
// The prefix is returned without the trailing hyphen (e.g., "bd" not "bd-").
// If the rig is not found in routes, returns "gt" as the default.
// The townRoot should be the Gas Town root directory (e.g., ~/gt).
func GetPrefixForRig(townRoot, rigName string) string {
	beadsDir := filepath.Join(townRoot, ".beads")
	routes, err := LoadRoutes(beadsDir)
	if err != nil || routes == nil {
		return "gt" // Default prefix
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

	return "gt" // Default prefix
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
