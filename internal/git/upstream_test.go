package git

import (
	"testing"
)

func TestGit_UpstreamRemote(t *testing.T) {
	tmp := t.TempDir()
	g := NewGit(tmp)
	runGit(t, tmp, "init", "--initial-branch", "main")

	t.Run("initially absent", func(t *testing.T) {
		has, err := g.HasUpstreamRemote()
		if err != nil {
			t.Fatalf("HasUpstreamRemote: %v", err)
		}
		if has {
			t.Fatal("expected no upstream remote initially")
		}

		url, err := g.GetUpstreamURL()
		if err != nil {
			t.Fatalf("GetUpstreamURL: %v", err)
		}
		if url != "" {
			t.Errorf("expected empty URL, got %q", url)
		}
	})

	upstream1 := "https://example.com/upstream1.git"

	t.Run("add", func(t *testing.T) {
		if err := g.AddUpstreamRemote(upstream1); err != nil {
			t.Fatalf("AddUpstreamRemote: %v", err)
		}

		has, err := g.HasUpstreamRemote()
		if err != nil {
			t.Fatalf("HasUpstreamRemote: %v", err)
		}
		if !has {
			t.Fatal("expected upstream remote to exist")
		}

		url, err := g.GetUpstreamURL()
		if err != nil {
			t.Fatalf("GetUpstreamURL: %v", err)
		}
		if url != upstream1 {
			t.Errorf("URL = %q, want %q", url, upstream1)
		}
	})

	t.Run("idempotent same URL is true no-op", func(t *testing.T) {
		if err := g.AddUpstreamRemote(upstream1); err != nil {
			t.Fatalf("AddUpstreamRemote: %v", err)
		}

		url, err := g.GetUpstreamURL()
		if err != nil {
			t.Fatalf("GetUpstreamURL: %v", err)
		}
		if url != upstream1 {
			t.Errorf("URL = %q, want %q", url, upstream1)
		}
	})

	upstream2 := "https://example.com/upstream2.git"

	t.Run("update different URL", func(t *testing.T) {
		if err := g.AddUpstreamRemote(upstream2); err != nil {
			t.Fatalf("AddUpstreamRemote: %v", err)
		}

		url, err := g.GetUpstreamURL()
		if err != nil {
			t.Fatalf("GetUpstreamURL: %v", err)
		}
		if url != upstream2 {
			t.Errorf("URL = %q, want %q", url, upstream2)
		}
	})
}
