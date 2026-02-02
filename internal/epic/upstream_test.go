package epic

import (
	"testing"
)

func TestDependencyGraph_AddAndGet(t *testing.T) {
	g := NewDependencyGraph()

	g.AddNode("task1")
	g.AddNode("task2")
	g.AddNode("task3")

	g.AddEdge("task2", "task1") // task2 depends on task1
	g.AddEdge("task3", "task1") // task3 depends on task1
	g.AddEdge("task3", "task2") // task3 also depends on task2

	// Test GetDependencies
	deps := g.GetDependencies("task3")
	if len(deps) != 2 {
		t.Errorf("expected 2 dependencies for task3, got %d", len(deps))
	}

	// Test GetDependents
	dependents := g.GetDependents("task1")
	if len(dependents) != 2 {
		t.Errorf("expected 2 dependents for task1, got %d", len(dependents))
	}
}

func TestDependencyGraph_GetRoots(t *testing.T) {
	g := NewDependencyGraph()

	g.AddNode("root1")
	g.AddNode("root2")
	g.AddNode("child1")
	g.AddNode("child2")

	g.AddEdge("child1", "root1")
	g.AddEdge("child2", "root1")
	g.AddEdge("child2", "root2")

	roots := g.GetRoots()

	// Should have 2 roots (root1 and root2)
	if len(roots) != 2 {
		t.Errorf("expected 2 roots, got %d", len(roots))
	}

	// Verify roots are correct
	rootMap := make(map[string]bool)
	for _, r := range roots {
		rootMap[r] = true
	}

	if !rootMap["root1"] || !rootMap["root2"] {
		t.Error("expected root1 and root2 to be roots")
	}
}

func TestDependencyGraph_TopologicalSort(t *testing.T) {
	g := NewDependencyGraph()

	g.AddNode("a")
	g.AddNode("b")
	g.AddNode("c")
	g.AddNode("d")

	// a <- b <- d
	//      ^
	//      c
	g.AddEdge("b", "a") // b depends on a
	g.AddEdge("c", "a") // c depends on a
	g.AddEdge("d", "b") // d depends on b

	order, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort failed: %v", err)
	}

	// Create position map
	pos := make(map[string]int)
	for i, id := range order {
		pos[id] = i
	}

	// Verify ordering constraints
	if pos["a"] > pos["b"] {
		t.Error("a should come before b")
	}
	if pos["a"] > pos["c"] {
		t.Error("a should come before c")
	}
	if pos["b"] > pos["d"] {
		t.Error("b should come before d")
	}
}

func TestDependencyGraph_CycleDetection(t *testing.T) {
	g := NewDependencyGraph()

	g.AddNode("a")
	g.AddNode("b")
	g.AddNode("c")

	// Create cycle: a -> b -> c -> a
	g.AddEdge("a", "c")
	g.AddEdge("b", "a")
	g.AddEdge("c", "b")

	_, err := g.TopologicalSort()
	if err == nil {
		t.Error("expected cycle detection error")
	}
}

func TestFormatPRURL(t *testing.T) {
	url := FormatPRURL("owner", "repo", 123)
	expected := "https://github.com/owner/repo/pull/123"

	if url != expected {
		t.Errorf("expected '%s', got '%s'", expected, url)
	}
}

func TestParsePRURL(t *testing.T) {
	tests := []struct {
		url       string
		owner     string
		repo      string
		number    int
		expectErr bool
	}{
		{
			url:    "https://github.com/owner/repo/pull/123",
			owner:  "owner",
			repo:   "repo",
			number: 123,
		},
		{
			url:    "https://github.com/org/project/pull/456/",
			owner:  "org",
			repo:   "project",
			number: 456,
		},
		{
			url:       "https://gitlab.com/owner/repo/merge_requests/123",
			expectErr: true,
		},
		{
			url:       "not-a-url",
			expectErr: true,
		},
		{
			url:       "https://github.com/owner/repo/issues/123",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			owner, repo, number, err := ParsePRURL(tt.url)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if owner != tt.owner {
				t.Errorf("owner: expected '%s', got '%s'", tt.owner, owner)
			}
			if repo != tt.repo {
				t.Errorf("repo: expected '%s', got '%s'", tt.repo, repo)
			}
			if number != tt.number {
				t.Errorf("number: expected %d, got %d", tt.number, number)
			}
		})
	}
}

func TestFormatPRBranchName(t *testing.T) {
	tests := []struct {
		epicID    string
		subtask   string
		expected  string
	}{
		{"abc12", "implement-api", "epic-abc12/implement-api"},
		{"xyz", "add-tests", "epic-xyz/add-tests"},
	}

	for _, tt := range tests {
		result := FormatPRBranchName(tt.epicID, tt.subtask)
		if result != tt.expected {
			t.Errorf("FormatPRBranchName(%s, %s) = '%s', expected '%s'",
				tt.epicID, tt.subtask, result, tt.expected)
		}
	}
}

func TestParseUpstreamPRs(t *testing.T) {
	tests := []struct {
		field    string
		expected []string
	}{
		{
			field:    "",
			expected: nil,
		},
		{
			field:    "https://github.com/test/repo/pull/1",
			expected: []string{"https://github.com/test/repo/pull/1"},
		},
		{
			field:    "https://github.com/test/repo/pull/1,https://github.com/test/repo/pull/2",
			expected: []string{"https://github.com/test/repo/pull/1", "https://github.com/test/repo/pull/2"},
		},
		{
			field:    "https://github.com/test/repo/pull/1\nhttps://github.com/test/repo/pull/2",
			expected: []string{"https://github.com/test/repo/pull/1", "https://github.com/test/repo/pull/2"},
		},
		{
			field:    "  url1  ,  url2  ",
			expected: []string{"url1", "url2"},
		},
	}

	for _, tt := range tests {
		result := ParseUpstreamPRs(tt.field)

		if len(result) != len(tt.expected) {
			t.Errorf("ParseUpstreamPRs(%q) returned %d items, expected %d",
				tt.field, len(result), len(tt.expected))
			continue
		}

		for i := range result {
			if result[i] != tt.expected[i] {
				t.Errorf("ParseUpstreamPRs(%q)[%d] = %q, expected %q",
					tt.field, i, result[i], tt.expected[i])
			}
		}
	}
}

func TestFormatUpstreamPRs(t *testing.T) {
	urls := []string{"url1", "url2", "url3"}
	result := FormatUpstreamPRs(urls)
	expected := "url1,url2,url3"

	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}
