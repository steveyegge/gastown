// Package epic provides upstream contribution workflow support.
package epic

import (
	"fmt"
	"strings"
)

// UpstreamPR tracks a PR submitted to upstream.
type UpstreamPR struct {
	Number     int      // GitHub PR number
	URL        string   // Full PR URL
	SubtaskIDs []string // Which subtasks are included
	BaseBranch string   // What it targets (main or another PR's branch)
	HeadBranch string   // Our branch name
	DependsOn  int      // PR number this depends on (0 if none)
	Status     string   // open, merged, closed, changes_requested
}

// PRGroup represents a logical grouping for submission.
type PRGroup struct {
	Title      string   // Group title
	SubtaskIDs []string // Subtasks in this group
	DependsOn  []string // SubtaskIDs this group depends on
}

// PRStack represents a stack of dependent PRs.
type PRStack struct {
	PRs      []UpstreamPR // PRs in dependency order (roots first)
	EpicID   string       // Parent epic ID
	EpicName string       // Epic title
}

// DependencyGraph holds subtask dependency relationships.
type DependencyGraph struct {
	Nodes    map[string]bool     // All subtask IDs
	Edges    map[string][]string // subtask -> [dependencies]
	Reverse  map[string][]string // dependency -> [dependents]
}

// NewDependencyGraph creates an empty dependency graph.
func NewDependencyGraph() *DependencyGraph {
	return &DependencyGraph{
		Nodes:   make(map[string]bool),
		Edges:   make(map[string][]string),
		Reverse: make(map[string][]string),
	}
}

// AddNode adds a subtask to the graph.
func (g *DependencyGraph) AddNode(id string) {
	g.Nodes[id] = true
}

// AddEdge adds a dependency: "from" depends on "to".
func (g *DependencyGraph) AddEdge(from, to string) {
	g.Edges[from] = append(g.Edges[from], to)
	g.Reverse[to] = append(g.Reverse[to], from)
}

// GetRoots returns subtasks with no dependencies (roots of the DAG).
func (g *DependencyGraph) GetRoots() []string {
	var roots []string
	for id := range g.Nodes {
		if len(g.Edges[id]) == 0 {
			roots = append(roots, id)
		}
	}
	return roots
}

// GetDependents returns subtasks that depend on the given ID.
func (g *DependencyGraph) GetDependents(id string) []string {
	return g.Reverse[id]
}

// GetDependencies returns subtasks that the given ID depends on.
func (g *DependencyGraph) GetDependencies(id string) []string {
	return g.Edges[id]
}

// TopologicalSort returns subtasks in dependency order (dependencies first).
func (g *DependencyGraph) TopologicalSort() ([]string, error) {
	var result []string
	visited := make(map[string]bool)
	inStack := make(map[string]bool)

	var visit func(id string) error
	visit = func(id string) error {
		if inStack[id] {
			return fmt.Errorf("cycle detected involving %s", id)
		}
		if visited[id] {
			return nil
		}

		inStack[id] = true
		for _, dep := range g.Edges[id] {
			if err := visit(dep); err != nil {
				return err
			}
		}
		inStack[id] = false
		visited[id] = true
		result = append(result, id)
		return nil
	}

	for id := range g.Nodes {
		if err := visit(id); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// PRStatusOpen represents an open PR.
const PRStatusOpen = "open"

// PRStatusMerged represents a merged PR.
const PRStatusMerged = "merged"

// PRStatusClosed represents a closed (not merged) PR.
const PRStatusClosed = "closed"

// PRStatusChangesRequested represents a PR with requested changes.
const PRStatusChangesRequested = "changes_requested"

// PRStatusApproved represents an approved PR.
const PRStatusApproved = "approved"

// PRStatusDraft represents a draft PR.
const PRStatusDraft = "draft"

// FormatPRURL creates a GitHub PR URL.
func FormatPRURL(owner, repo string, number int) string {
	return fmt.Sprintf("https://github.com/%s/%s/pull/%d", owner, repo, number)
}

// ParsePRURL extracts owner, repo, and PR number from a GitHub PR URL.
func ParsePRURL(url string) (owner, repo string, number int, err error) {
	// Expected format: https://github.com/owner/repo/pull/123
	url = strings.TrimSuffix(url, "/")

	if !strings.HasPrefix(url, "https://github.com/") {
		return "", "", 0, fmt.Errorf("not a GitHub URL: %s", url)
	}

	path := strings.TrimPrefix(url, "https://github.com/")
	parts := strings.Split(path, "/")

	if len(parts) < 4 || parts[2] != "pull" {
		return "", "", 0, fmt.Errorf("not a GitHub PR URL: %s", url)
	}

	owner = parts[0]
	repo = parts[1]

	_, err = fmt.Sscanf(parts[3], "%d", &number)
	if err != nil {
		return "", "", 0, fmt.Errorf("invalid PR number in URL: %s", url)
	}

	return owner, repo, number, nil
}

// FormatPRBranchName creates a branch name for a PR.
func FormatPRBranchName(epicID, subtaskRef string) string {
	// Format: epic-<id>/<subtask-ref>
	return fmt.Sprintf("epic-%s/%s", epicID, subtaskRef)
}

// ParseUpstreamPRs parses the upstream_prs field from epic description.
// Format: "url1,url2,..." or "url1\nurl2\n..."
func ParseUpstreamPRs(field string) []string {
	if field == "" {
		return nil
	}

	// Try comma-separated first
	if strings.Contains(field, ",") {
		parts := strings.Split(field, ",")
		var result []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				result = append(result, p)
			}
		}
		return result
	}

	// Try newline-separated
	parts := strings.Split(field, "\n")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// FormatUpstreamPRs formats PR URLs for storage in epic description.
func FormatUpstreamPRs(urls []string) string {
	return strings.Join(urls, ",")
}
