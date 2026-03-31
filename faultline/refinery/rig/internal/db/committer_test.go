package db

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestCommitterSkipsWhenClean(t *testing.T) {
	d := &DB{} // nil sql.DB is fine — flush never reaches ExecContext when clean
	c := NewCommitter(d, slog.New(slog.NewTextHandler(os.Stderr, nil)), 10*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	c.Run(ctx) // should complete without panic (no SQL calls on zero dirty)
}

func TestMarkDirtyIncrementsCounter(t *testing.T) {
	d := &DB{}
	d.MarkDirty()
	d.MarkDirty()
	d.MarkDirty()
	if got := d.dirty.Load(); got != 3 {
		t.Fatalf("dirty = %d, want 3", got)
	}
}

func TestCommitterSwapResetsDirty(t *testing.T) {
	d := &DB{}
	d.MarkDirty()
	d.MarkDirty()

	// Simulate what flush does: swap to 0.
	n := d.dirty.Swap(0)
	if n != 2 {
		t.Fatalf("swapped = %d, want 2", n)
	}
	if got := d.dirty.Load(); got != 0 {
		t.Fatalf("dirty after swap = %d, want 0", got)
	}
}
