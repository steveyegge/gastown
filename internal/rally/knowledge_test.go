package rally

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadKnowledgeIndex_Absent(t *testing.T) {
	idx, err := LoadKnowledgeIndex(t.TempDir())
	if err != nil {
		t.Fatalf("expected nil error for absent rally_tavern, got: %v", err)
	}
	if idx != nil {
		t.Fatal("expected nil index for absent rally_tavern")
	}
}

func TestLoadKnowledgeIndex_Empty(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "rally_tavern", "mayor", "rig", "knowledge", "practices"), 0755); err != nil {
		t.Fatal(err)
	}

	idx, err := LoadKnowledgeIndex(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if idx == nil {
		t.Fatal("expected non-nil index")
	}
	if idx.Len() != 0 {
		t.Fatalf("expected 0 entries, got %d", idx.Len())
	}
}

func TestLoadKnowledgeIndex_ParsesPractice(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "rally_tavern", "mayor", "rig", "knowledge", "practices")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	yaml := `id: test-practice
title: Test Practice
summary: A test practice for unit tests
codebase_type: go-cobra
tags: [testing, go]
gotchas:
  - Don't forget to clean up
`
	if err := os.WriteFile(filepath.Join(dir, "test-practice.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	idx, err := LoadKnowledgeIndex(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if idx.Len() != 1 {
		t.Fatalf("expected 1 entry, got %d", idx.Len())
	}

	results := idx.Search(SearchQuery{Tags: []string{"go"}})
	if len(results) != 1 {
		t.Fatalf("expected 1 result for tag 'go', got %d", len(results))
	}
	if results[0].Kind != "practice" {
		t.Errorf("expected kind=practice, got %q", results[0].Kind)
	}
	if results[0].CodebaseType != "go-cobra" {
		t.Errorf("expected codebase_type=go-cobra, got %q", results[0].CodebaseType)
	}
}

func TestSearch_Ranking(t *testing.T) {
	idx := &KnowledgeIndex{entries: []KnowledgeEntry{
		{ID: "a", Title: "Auth workflow", Summary: "oauth guide", Tags: []string{"auth"}},
		{ID: "b", Title: "Generic guide", Summary: "auth is mentioned", Tags: []string{"security"}},
		{ID: "c", Title: "Unrelated", Summary: "nothing relevant", Tags: []string{"other"}},
	}}

	results := idx.Search(SearchQuery{Tags: []string{"auth"}, Text: "auth"})

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// "a" should rank first: tag match (+3) + title match (+2) = 5
	if results[0].ID != "a" {
		t.Errorf("expected 'a' first, got %q", results[0].ID)
	}
	// "b" should rank second: text match in summary (+1) = 1
	if results[1].ID != "b" {
		t.Errorf("expected 'b' second, got %q", results[1].ID)
	}
}

func TestSearch_NoResults(t *testing.T) {
	idx := &KnowledgeIndex{entries: []KnowledgeEntry{
		{ID: "a", Title: "Go patterns", Tags: []string{"go"}},
	}}

	results := idx.Search(SearchQuery{Text: "python flask"})
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	idx := &KnowledgeIndex{entries: []KnowledgeEntry{
		{ID: "a", Title: "Go patterns", Tags: []string{"go"}},
	}}

	results := idx.Search(SearchQuery{})
	if len(results) != 0 {
		t.Fatalf("expected 0 results for empty query, got %d", len(results))
	}
}
