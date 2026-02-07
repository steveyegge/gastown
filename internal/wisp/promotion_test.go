package wisp

import (
	"testing"
	"time"
)

func TestHasComments(t *testing.T) {
	tests := []struct {
		name         string
		commentCount int
		want         bool
	}{
		{"no comments", 0, false},
		{"one comment", 1, true},
		{"many comments", 5, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &WispCandidate{CommentCount: tt.commentCount}
			if got := HasComments(c); got != tt.want {
				t.Errorf("HasComments() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsReferenced(t *testing.T) {
	tests := []struct {
		name            string
		nonWispRefCount int
		want            bool
	}{
		{"no references", 0, false},
		{"one reference", 1, true},
		{"many references", 3, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &WispCandidate{NonWispRefCount: tt.nonWispRefCount}
			if got := IsReferenced(c); got != tt.want {
				t.Errorf("IsReferenced() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasKeepLabel(t *testing.T) {
	tests := []struct {
		name   string
		labels []string
		want   bool
	}{
		{"no labels", nil, false},
		{"empty labels", []string{}, false},
		{"unrelated labels", []string{"gt:task", "gt:agent"}, false},
		{"has keep label", []string{"gt:task", KeepLabel}, true},
		{"only keep label", []string{KeepLabel}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &WispCandidate{Labels: tt.labels}
			if got := HasKeepLabel(c); got != tt.want {
				t.Errorf("HasKeepLabel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsPastTTL(t *testing.T) {
	tests := []struct {
		name string
		age  time.Duration
		ttl  time.Duration
		want bool
	}{
		{"zero TTL means never expires", 24 * time.Hour, 0, false},
		{"within TTL", 1 * time.Hour, 6 * time.Hour, false},
		{"exactly at TTL", 6 * time.Hour, 6 * time.Hour, false},
		{"past TTL", 7 * time.Hour, 6 * time.Hour, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &WispCandidate{Age: tt.age, TTL: tt.ttl}
			if got := isPastTTL(c); got != tt.want {
				t.Errorf("isPastTTL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldPromote(t *testing.T) {
	tests := []struct {
		name      string
		candidate *WispCandidate
		want      bool
	}{
		{
			name:      "no criteria met",
			candidate: &WispCandidate{},
			want:      false,
		},
		{
			name:      "has comments only",
			candidate: &WispCandidate{CommentCount: 1},
			want:      true,
		},
		{
			name:      "is referenced only",
			candidate: &WispCandidate{NonWispRefCount: 1},
			want:      true,
		},
		{
			name:      "has keep label only",
			candidate: &WispCandidate{Labels: []string{KeepLabel}},
			want:      true,
		},
		{
			name:      "past TTL only",
			candidate: &WispCandidate{Age: 25 * time.Hour, TTL: 24 * time.Hour},
			want:      true,
		},
		{
			name: "all criteria met",
			candidate: &WispCandidate{
				CommentCount:    2,
				NonWispRefCount: 1,
				Labels:          []string{KeepLabel},
				Age:             48 * time.Hour,
				TTL:             24 * time.Hour,
			},
			want: true,
		},
		{
			name: "within TTL with no other criteria",
			candidate: &WispCandidate{
				Age: 1 * time.Hour,
				TTL: 6 * time.Hour,
			},
			want: false,
		},
		{
			name: "zero TTL not enough on its own",
			candidate: &WispCandidate{
				Age: 100 * time.Hour,
				TTL: 0,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldPromote(tt.candidate); got != tt.want {
				t.Errorf("ShouldPromote() = %v, want %v", got, tt.want)
			}
		})
	}
}
