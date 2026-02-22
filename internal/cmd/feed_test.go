package cmd

import (
	"strings"
	"testing"
)

func resetFeedFlagsForTest() {
	feedFollow = false
	feedLimit = 100
	feedSince = ""
	feedMol = ""
	feedType = ""
	feedRig = ""
	feedActor = ""
	feedContains = ""
	feedNoFollow = false
	feedWindow = false
	feedPlain = false
	feedProblems = false
}

func TestHasFeedQueryFilters(t *testing.T) {
	resetFeedFlagsForTest()
	if err := feedCmd.Flags().Set("limit", "100"); err != nil {
		t.Fatalf("setting limit: %v", err)
	}
	feedCmd.Flags().Lookup("limit").Changed = false

	if hasFeedQueryFilters(feedCmd) {
		t.Fatalf("expected no query filters by default")
	}

	feedActor = "crew/joe"
	if !hasFeedQueryFilters(feedCmd) {
		t.Fatalf("expected actor filter to trigger query mode")
	}
	feedActor = ""

	feedContains = "conflict"
	if !hasFeedQueryFilters(feedCmd) {
		t.Fatalf("expected contains filter to trigger query mode")
	}
	feedContains = ""

	feedSince = "1h"
	if !hasFeedQueryFilters(feedCmd) {
		t.Fatalf("expected since filter to trigger query mode")
	}
	feedSince = ""

	feedNoFollow = true
	if !hasFeedQueryFilters(feedCmd) {
		t.Fatalf("expected no-follow to trigger query mode")
	}
	feedNoFollow = false

	if err := feedCmd.Flags().Set("limit", "101"); err != nil {
		t.Fatalf("setting limit: %v", err)
	}
	if !hasFeedQueryFilters(feedCmd) {
		t.Fatalf("expected changed limit to trigger query mode")
	}
	if err := feedCmd.Flags().Set("limit", "100"); err != nil {
		t.Fatalf("resetting limit: %v", err)
	}
	feedCmd.Flags().Lookup("limit").Changed = false
}

func TestBuildFeedArgs_IncludesActorAndContains(t *testing.T) {
	resetFeedFlagsForTest()
	feedActor = "greenplace/crew"
	feedContains = "merge_failed"
	feedFollow = true

	args := buildFeedArgs()
	joined := " " + joinArgs(args) + " "

	if !containsArg(joined, "--follow") {
		t.Fatalf("expected --follow in args, got: %v", args)
	}
	if !containsArg(joined, "--actor greenplace/crew") {
		t.Fatalf("expected --actor in args, got: %v", args)
	}
	if !containsArg(joined, "--contains merge_failed") {
		t.Fatalf("expected --contains in args, got: %v", args)
	}
}

func containsArg(joined, target string) bool {
	return strings.Contains(strings.ToLower(joined), strings.ToLower(" "+target+" "))
}

func joinArgs(args []string) string {
	out := ""
	for i, a := range args {
		if i > 0 {
			out += " "
		}
		out += a
	}
	return out
}
