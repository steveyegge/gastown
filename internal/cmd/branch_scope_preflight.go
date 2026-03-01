package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/steveyegge/gastown/internal/git"
)

const branchScopeEnvVar = "GT_BRANCH_SCOPE_PATHS"

// BranchScopeDiagnostics provides machine-readable contamination diagnostics.
type BranchScopeDiagnostics struct {
	Classification  string   `json:"classification"`
	BaseRef         string   `json:"base_ref"`
	HeadRef         string   `json:"head_ref"`
	AllowedPrefixes []string `json:"allowed_prefixes"`
	ChangedFiles    []string `json:"changed_files"`
	OutOfScopeFiles []string `json:"out_of_scope_files"`
}

func parseScopePrefixes(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	normalized := make([]string, 0)
	seen := make(map[string]bool)
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == '\t'
	}) {
		prefix := normalizeScopePath(part)
		if prefix == "" || seen[prefix] {
			continue
		}
		seen[prefix] = true
		normalized = append(normalized, prefix)
	}
	return normalized
}

func normalizeScopePath(path string) string {
	normalized := strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	normalized = strings.TrimPrefix(normalized, "./")
	normalized = strings.TrimPrefix(normalized, "/")
	normalized = strings.TrimSuffix(normalized, "/")
	return normalized
}

func outOfScopeFiles(changedFiles, allowedPrefixes []string) []string {
	if len(changedFiles) == 0 || len(allowedPrefixes) == 0 {
		return nil
	}
	out := make([]string, 0)
	for _, file := range changedFiles {
		normalizedFile := normalizeScopePath(file)
		inScope := false
		for _, prefix := range allowedPrefixes {
			if normalizedFile == prefix || strings.HasPrefix(normalizedFile, prefix+"/") {
				inScope = true
				break
			}
		}
		if !inScope {
			out = append(out, normalizedFile)
		}
	}
	return out
}

func resolveDefaultBaseRef(g *git.Git) string {
	if exists, err := g.RefExists("origin/main"); err == nil && exists {
		return "origin/main"
	}
	if exists, err := g.RefExists("origin/master"); err == nil && exists {
		return "origin/master"
	}
	return "origin/main"
}

func runBranchScopePreflight(g *git.Git, baseRef string) error {
	allowedPrefixes := parseScopePrefixes(os.Getenv(branchScopeEnvVar))
	if len(allowedPrefixes) == 0 {
		return nil
	}
	if strings.TrimSpace(baseRef) == "" {
		baseRef = resolveDefaultBaseRef(g)
	}

	changedFiles, err := g.FilesChangedSince(baseRef, "HEAD")
	if err != nil {
		return fmt.Errorf("branch scope preflight: computing changed files: %w", err)
	}
	outOfScope := outOfScopeFiles(changedFiles, allowedPrefixes)
	if len(outOfScope) == 0 {
		return nil
	}

	diag := BranchScopeDiagnostics{
		Classification:  "branch_contamination",
		BaseRef:         baseRef,
		HeadRef:         "HEAD",
		AllowedPrefixes: allowedPrefixes,
		ChangedFiles:    changedFiles,
		OutOfScopeFiles: outOfScope,
	}
	payload, _ := json.Marshal(diag)
	return fmt.Errorf("branch scope preflight failed: %s", string(payload))
}

func runBranchScopePreflightFromCWD(cwd string) error {
	if len(parseScopePrefixes(os.Getenv(branchScopeEnvVar))) == 0 {
		return nil
	}
	g := git.NewGit(cwd)
	if !g.IsRepo() {
		return nil
	}
	return runBranchScopePreflight(g, "")
}
