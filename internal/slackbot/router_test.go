package slackbot

import (
	"testing"

	"github.com/steveyegge/gastown/internal/config"
)

func TestRouter_ResolveChannel_ExactMatch(t *testing.T) {
	cfg := &config.SlackConfig{
		Enabled:        true,
		DefaultChannel: "C0000000000",
		Channels: map[string]string{
			"gastown/polecats/furiosa": "C1111111111",
			"gastown/polecats/*":       "C2222222222",
		},
	}

	r := NewRouter(cfg)

	// Exact match should win
	got := r.ResolveChannel("gastown/polecats/furiosa")
	if got != "C1111111111" {
		t.Errorf("ResolveChannel(gastown/polecats/furiosa) = %q, want %q", got, "C1111111111")
	}
}

func TestRouter_ResolveChannel_WildcardMatch(t *testing.T) {
	cfg := &config.SlackConfig{
		Enabled:        true,
		DefaultChannel: "C0000000000",
		Channels: map[string]string{
			"gastown/polecats/*": "C2222222222",
		},
	}

	r := NewRouter(cfg)

	got := r.ResolveChannel("gastown/polecats/max")
	if got != "C2222222222" {
		t.Errorf("ResolveChannel(gastown/polecats/max) = %q, want %q", got, "C2222222222")
	}
}

func TestRouter_ResolveChannel_MultiWildcard(t *testing.T) {
	cfg := &config.SlackConfig{
		Enabled:        true,
		DefaultChannel: "C0000000000",
		Channels: map[string]string{
			"*/polecats/*": "C3333333333",
		},
	}

	r := NewRouter(cfg)

	tests := []struct {
		agent string
		want  string
	}{
		{"gastown/polecats/furiosa", "C3333333333"},
		{"beads/polecats/max", "C3333333333"},
	}

	for _, tt := range tests {
		got := r.ResolveChannel(tt.agent)
		if got != tt.want {
			t.Errorf("ResolveChannel(%q) = %q, want %q", tt.agent, got, tt.want)
		}
	}
}

func TestRouter_ResolveChannel_DefaultFallback(t *testing.T) {
	cfg := &config.SlackConfig{
		Enabled:        true,
		DefaultChannel: "C0000000000",
		Channels: map[string]string{
			"gastown/*": "C1111111111",
		},
	}

	r := NewRouter(cfg)

	// Different rig should fall back to default
	got := r.ResolveChannel("beads/polecats/max")
	if got != "C0000000000" {
		t.Errorf("ResolveChannel(beads/polecats/max) = %q, want %q (default)", got, "C0000000000")
	}
}

func TestRouter_ResolveChannel_SegmentCountMustMatch(t *testing.T) {
	cfg := &config.SlackConfig{
		Enabled:        true,
		DefaultChannel: "C0000000000",
		Channels: map[string]string{
			"gastown/*": "C1111111111", // 2 segments
		},
	}

	r := NewRouter(cfg)

	// 3 segments should NOT match 2-segment pattern
	got := r.ResolveChannel("gastown/polecats/furiosa")
	if got != "C0000000000" {
		t.Errorf("ResolveChannel(gastown/polecats/furiosa) = %q, want %q (default - segment mismatch)", got, "C0000000000")
	}

	// 2 segments SHOULD match
	got = r.ResolveChannel("gastown/witness")
	if got != "C1111111111" {
		t.Errorf("ResolveChannel(gastown/witness) = %q, want %q", got, "C1111111111")
	}
}

func TestRouter_ResolveChannel_Precedence(t *testing.T) {
	// Test that more specific patterns win
	cfg := &config.SlackConfig{
		Enabled:        true,
		DefaultChannel: "C0000000000",
		Channels: map[string]string{
			"gastown/crew/decision_point": "C9999999999", // exact
			"gastown/polecats/*":          "C1111111111", // 1 wildcard
			"gastown/crew/*":              "C2222222222", // 1 wildcard
			"*/polecats/*":                "C3333333333", // 2 wildcards
			"*/*/*":                       "C4444444444", // 3 wildcards
		},
	}

	r := NewRouter(cfg)

	tests := []struct {
		agent string
		want  string
		desc  string
	}{
		{"gastown/crew/decision_point", "C9999999999", "exact match wins"},
		{"gastown/polecats/furiosa", "C1111111111", "single wildcard match"},
		{"gastown/crew/max", "C2222222222", "single wildcard match for crew"},
		{"beads/polecats/furiosa", "C3333333333", "double wildcard match"},
		{"longeye/crew/bob", "C4444444444", "triple wildcard match"},
	}

	for _, tt := range tests {
		got := r.ResolveChannel(tt.agent)
		if got != tt.want {
			t.Errorf("ResolveChannel(%q) = %q, want %q (%s)", tt.agent, got, tt.want, tt.desc)
		}
	}
}

func TestRouter_ResolveChannel_EmptyAgent(t *testing.T) {
	cfg := &config.SlackConfig{
		Enabled:        true,
		DefaultChannel: "C0000000000",
		Channels: map[string]string{
			"*": "C1111111111",
		},
	}

	r := NewRouter(cfg)

	got := r.ResolveChannel("")
	if got != "C0000000000" {
		t.Errorf("ResolveChannel('') = %q, want %q (default)", got, "C0000000000")
	}
}

func TestRouter_ResolveChannel_SingleSegment(t *testing.T) {
	cfg := &config.SlackConfig{
		Enabled:        true,
		DefaultChannel: "C0000000000",
		Channels: map[string]string{
			"mayor": "C1111111111",
			"*":     "C2222222222",
		},
	}

	r := NewRouter(cfg)

	// Exact match
	got := r.ResolveChannel("mayor")
	if got != "C1111111111" {
		t.Errorf("ResolveChannel(mayor) = %q, want %q", got, "C1111111111")
	}

	// Single wildcard match
	got = r.ResolveChannel("deacon")
	if got != "C2222222222" {
		t.Errorf("ResolveChannel(deacon) = %q, want %q", got, "C2222222222")
	}
}

func TestRouter_ResolveChannel_NilConfig(t *testing.T) {
	r := NewRouter(nil)

	// Should not panic and return empty (default from NewSlackConfig)
	got := r.ResolveChannel("gastown/polecats/furiosa")
	if got != "" {
		t.Errorf("ResolveChannel with nil config = %q, want empty", got)
	}
}

func TestRouter_ResolveChannel_EmptyChannels(t *testing.T) {
	cfg := &config.SlackConfig{
		Enabled:        true,
		DefaultChannel: "C0000000000",
		Channels:       map[string]string{},
	}

	r := NewRouter(cfg)

	got := r.ResolveChannel("gastown/polecats/furiosa")
	if got != "C0000000000" {
		t.Errorf("ResolveChannel with empty channels = %q, want %q (default)", got, "C0000000000")
	}
}

func TestRouter_ChannelName(t *testing.T) {
	cfg := &config.SlackConfig{
		Enabled:        true,
		DefaultChannel: "C0000000000",
		ChannelNames: map[string]string{
			"C0000000000": "#decisions-general",
			"C1111111111": "#decisions-gastown",
		},
	}

	r := NewRouter(cfg)

	tests := []struct {
		channelID string
		want      string
	}{
		{"C0000000000", "#decisions-general"},
		{"C1111111111", "#decisions-gastown"},
		{"C9999999999", "C9999999999"}, // no name configured
	}

	for _, tt := range tests {
		got := r.ChannelName(tt.channelID)
		if got != tt.want {
			t.Errorf("ChannelName(%q) = %q, want %q", tt.channelID, got, tt.want)
		}
	}
}

func TestRouter_IsEnabled(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.SlackConfig
		want bool
	}{
		{
			name: "enabled",
			cfg:  &config.SlackConfig{Enabled: true},
			want: true,
		},
		{
			name: "disabled",
			cfg:  &config.SlackConfig{Enabled: false},
			want: false,
		},
		{
			name: "nil config",
			cfg:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRouter(tt.cfg)
			got := r.IsEnabled()
			if got != tt.want {
				t.Errorf("IsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRouter_PatternSpecificity(t *testing.T) {
	// Verify that patterns are sorted correctly by checking order of matches
	cfg := &config.SlackConfig{
		Enabled:        true,
		DefaultChannel: "default",
		Channels: map[string]string{
			"*/*/*":      "wildcard3", // least specific
			"gastown/*/*": "wildcard2", // more specific
			"gastown/*/x": "wildcard1", // even more specific (same wildcards, but more specific pattern alphabetically comes before)
		},
	}

	r := NewRouter(cfg)

	// Verify the internal pattern order
	if len(r.patterns) != 3 {
		t.Fatalf("Expected 3 patterns, got %d", len(r.patterns))
	}

	// First should have fewest wildcards
	if r.patterns[0].wildcards > r.patterns[1].wildcards {
		t.Errorf("Patterns not sorted by wildcard count: first has %d, second has %d",
			r.patterns[0].wildcards, r.patterns[1].wildcards)
	}
}

// Benchmark to ensure pattern matching is efficient
func BenchmarkRouter_ResolveChannel(b *testing.B) {
	cfg := &config.SlackConfig{
		Enabled:        true,
		DefaultChannel: "default",
		Channels: map[string]string{
			"gastown/polecats/furiosa":  "exact",
			"gastown/polecats/*":        "polecat",
			"gastown/crew/*":            "crew",
			"gastown/*":                 "rig-2seg",
			"gastown/*/*":               "rig",
			"*/polecats/*":              "all-polecats",
			"*/crew/*":                  "all-crew",
			"*/*/*":                     "any-3seg",
		},
	}

	r := NewRouter(cfg)

	agents := []string{
		"gastown/polecats/furiosa",
		"gastown/polecats/max",
		"gastown/crew/jack",
		"beads/polecats/nux",
		"longeye/crew/bob",
		"unknown/unknown/unknown",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, agent := range agents {
			r.ResolveChannel(agent)
		}
	}
}
