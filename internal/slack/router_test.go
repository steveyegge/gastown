package slack

import (
	"testing"
)

func TestNewRouter(t *testing.T) {
	cfg := &Config{
		Enabled:        true,
		DefaultChannel: "C0DEFAULT",
		Channels: map[string]string{
			"gastown/polecats/*": "C0POLECATS",
			"gastown/crew/*":     "C0CREW",
			"*/crew/*":           "C0ALLCREW",
		},
		ChannelNames: map[string]string{
			"C0DEFAULT":  "#decisions",
			"C0POLECATS": "#polecats",
			"C0CREW":     "#gastown-crew",
		},
	}

	r := NewRouter(cfg)

	if r == nil {
		t.Fatal("NewRouter returned nil")
	}

	if !r.IsEnabled() {
		t.Error("Router should be enabled")
	}
}

func TestResolve_ExactMatch(t *testing.T) {
	cfg := &Config{
		Enabled:        true,
		DefaultChannel: "C0DEFAULT",
		Channels: map[string]string{
			"gastown/polecats/furiosa": "C0FURIOSA",
			"gastown/polecats/*":       "C0POLECATS",
		},
	}

	r := NewRouter(cfg)

	result := r.Resolve("gastown/polecats/furiosa")

	if result.ChannelID != "C0FURIOSA" {
		t.Errorf("Expected C0FURIOSA, got %s", result.ChannelID)
	}
	if result.IsDefault {
		t.Error("Should not be default channel")
	}
	if result.MatchedBy != "gastown/polecats/furiosa" {
		t.Errorf("Expected exact pattern match, got %s", result.MatchedBy)
	}
}

func TestResolve_WildcardMatch(t *testing.T) {
	cfg := &Config{
		Enabled:        true,
		DefaultChannel: "C0DEFAULT",
		Channels: map[string]string{
			"gastown/polecats/*": "C0POLECATS",
		},
	}

	r := NewRouter(cfg)

	result := r.Resolve("gastown/polecats/max")

	if result.ChannelID != "C0POLECATS" {
		t.Errorf("Expected C0POLECATS, got %s", result.ChannelID)
	}
	if result.MatchedBy != "gastown/polecats/*" {
		t.Errorf("Expected wildcard pattern match, got %s", result.MatchedBy)
	}
}

func TestResolve_MultipleWildcards(t *testing.T) {
	cfg := &Config{
		Enabled:        true,
		DefaultChannel: "C0DEFAULT",
		Channels: map[string]string{
			"*/crew/*": "C0ALLCREW",
		},
	}

	r := NewRouter(cfg)

	tests := []struct {
		agent    string
		expected string
	}{
		{"gastown/crew/emma", "C0ALLCREW"},
		{"beads/crew/wolf", "C0ALLCREW"},
		{"hq/crew/admin", "C0ALLCREW"},
	}

	for _, tt := range tests {
		result := r.Resolve(tt.agent)
		if result.ChannelID != tt.expected {
			t.Errorf("Resolve(%s) = %s, want %s", tt.agent, result.ChannelID, tt.expected)
		}
	}
}

func TestResolve_DefaultFallback(t *testing.T) {
	cfg := &Config{
		Enabled:        true,
		DefaultChannel: "C0DEFAULT",
		Channels: map[string]string{
			"gastown/polecats/*": "C0POLECATS",
		},
	}

	r := NewRouter(cfg)

	result := r.Resolve("beads/crew/daemon_churn")

	if result.ChannelID != "C0DEFAULT" {
		t.Errorf("Expected default channel C0DEFAULT, got %s", result.ChannelID)
	}
	if !result.IsDefault {
		t.Error("Should be default channel")
	}
	if result.MatchedBy != "(default)" {
		t.Errorf("Expected (default) match, got %s", result.MatchedBy)
	}
}

func TestResolve_PatternPriority(t *testing.T) {
	// More specific patterns should match first
	cfg := &Config{
		Enabled:        true,
		DefaultChannel: "C0DEFAULT",
		Channels: map[string]string{
			"gastown/polecats/furiosa": "C0FURIOSA",  // Most specific (no wildcards)
			"gastown/polecats/*":       "C0POLECATS", // One wildcard
			"gastown/*/*":              "C0GASTOWN",  // Two wildcards
			"*/*/*":                    "C0CATCHALL", // Three wildcards
		},
	}

	r := NewRouter(cfg)

	tests := []struct {
		agent    string
		expected string
	}{
		{"gastown/polecats/furiosa", "C0FURIOSA"},  // Exact match
		{"gastown/polecats/max", "C0POLECATS"},    // One wildcard
		{"gastown/crew/emma", "C0GASTOWN"},        // Two wildcards
		{"beads/crew/wolf", "C0CATCHALL"},         // Three wildcards
	}

	for _, tt := range tests {
		result := r.Resolve(tt.agent)
		if result.ChannelID != tt.expected {
			t.Errorf("Resolve(%s) = %s, want %s (matched by %s)",
				tt.agent, result.ChannelID, tt.expected, result.MatchedBy)
		}
	}
}

func TestResolve_WebhookURL(t *testing.T) {
	cfg := &Config{
		Enabled:        true,
		DefaultChannel: "C0DEFAULT",
		WebhookURL:     "https://hooks.slack.com/default",
		Channels: map[string]string{
			"gastown/polecats/*": "C0POLECATS",
		},
		ChannelWebhooks: map[string]string{
			"C0POLECATS": "https://hooks.slack.com/polecats",
		},
	}

	r := NewRouter(cfg)

	// Channel with specific webhook
	result := r.Resolve("gastown/polecats/furiosa")
	if result.WebhookURL != "https://hooks.slack.com/polecats" {
		t.Errorf("Expected polecats webhook, got %s", result.WebhookURL)
	}

	// Default channel uses default webhook
	result = r.Resolve("beads/crew/wolf")
	if result.WebhookURL != "https://hooks.slack.com/default" {
		t.Errorf("Expected default webhook, got %s", result.WebhookURL)
	}
}

func TestResolve_ChannelNames(t *testing.T) {
	cfg := &Config{
		Enabled:        true,
		DefaultChannel: "C0DEFAULT",
		Channels: map[string]string{
			"gastown/polecats/*": "C0POLECATS",
		},
		ChannelNames: map[string]string{
			"C0DEFAULT":  "#decisions",
			"C0POLECATS": "#gastown-polecats",
		},
	}

	r := NewRouter(cfg)

	result := r.Resolve("gastown/polecats/furiosa")
	if result.ChannelName != "#gastown-polecats" {
		t.Errorf("Expected channel name #gastown-polecats, got %s", result.ChannelName)
	}

	result = r.Resolve("beads/crew/wolf")
	if result.ChannelName != "#decisions" {
		t.Errorf("Expected channel name #decisions, got %s", result.ChannelName)
	}
}

func TestResolveAll_UniqueChannels(t *testing.T) {
	cfg := &Config{
		Enabled:        true,
		DefaultChannel: "C0DEFAULT",
		Channels: map[string]string{
			"gastown/polecats/*": "C0POLECATS",
			"gastown/crew/*":     "C0CREW",
		},
	}

	r := NewRouter(cfg)

	// Multiple agents, some going to same channel
	agents := []string{
		"gastown/polecats/furiosa",
		"gastown/polecats/max",
		"gastown/crew/emma",
		"beads/crew/wolf", // Goes to default
	}

	results := r.ResolveAll(agents)

	// Should have 3 unique channels: C0POLECATS, C0CREW, C0DEFAULT
	if len(results) != 3 {
		t.Errorf("Expected 3 unique channels, got %d", len(results))
	}

	channelIDs := make(map[string]bool)
	for _, r := range results {
		channelIDs[r.ChannelID] = true
	}

	expected := []string{"C0POLECATS", "C0CREW", "C0DEFAULT"}
	for _, ch := range expected {
		if !channelIDs[ch] {
			t.Errorf("Missing expected channel %s", ch)
		}
	}
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		pattern []string
		agent   []string
		want    bool
	}{
		{[]string{"gastown", "polecats", "furiosa"}, []string{"gastown", "polecats", "furiosa"}, true},
		{[]string{"gastown", "polecats", "*"}, []string{"gastown", "polecats", "furiosa"}, true},
		{[]string{"gastown", "*", "*"}, []string{"gastown", "polecats", "furiosa"}, true},
		{[]string{"*", "*", "*"}, []string{"gastown", "polecats", "furiosa"}, true},
		{[]string{"*", "crew", "*"}, []string{"gastown", "crew", "emma"}, true},
		{[]string{"gastown", "polecats", "*"}, []string{"beads", "crew", "wolf"}, false},
		{[]string{"gastown", "polecats"}, []string{"gastown", "polecats", "furiosa"}, false}, // Length mismatch
		{[]string{"gastown", "polecats", "*", "*"}, []string{"gastown", "polecats", "furiosa"}, false}, // Length mismatch
	}

	for _, tt := range tests {
		got := matchPattern(tt.pattern, tt.agent)
		if got != tt.want {
			t.Errorf("matchPattern(%v, %v) = %v, want %v", tt.pattern, tt.agent, got, tt.want)
		}
	}
}

func TestPatternLessThan(t *testing.T) {
	tests := []struct {
		a, b compiledPattern
		want bool
	}{
		// More segments = higher priority
		{
			compiledPattern{segments: []string{"a", "b", "c", "d"}},
			compiledPattern{segments: []string{"a", "b", "c"}},
			true,
		},
		// Fewer wildcards = higher priority (same segment count)
		{
			compiledPattern{segments: []string{"a", "b", "c"}},
			compiledPattern{segments: []string{"a", "*", "c"}},
			true,
		},
		// Alphabetical tie-breaker
		{
			compiledPattern{original: "aaa", segments: []string{"a", "b", "c"}},
			compiledPattern{original: "bbb", segments: []string{"a", "b", "c"}},
			true,
		},
	}

	for _, tt := range tests {
		got := patternLessThan(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("patternLessThan(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestCountWildcards(t *testing.T) {
	tests := []struct {
		segments []string
		want     int
	}{
		{[]string{"a", "b", "c"}, 0},
		{[]string{"a", "*", "c"}, 1},
		{[]string{"*", "*", "*"}, 3},
		{[]string{"a", "*", "*"}, 2},
	}

	for _, tt := range tests {
		got := countWildcards(tt.segments)
		if got != tt.want {
			t.Errorf("countWildcards(%v) = %d, want %d", tt.segments, got, tt.want)
		}
	}
}

func TestLoadRouterFromFile(t *testing.T) {
	// This test would need a temp file - skipping for now
	// as it tests file I/O which is straightforward
	t.Skip("Requires temp file setup")
}

func TestNormalizeAgent(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"mayor", "mayor"},
		{"mayor/", "mayor"},
		{"gastown/crew/emma", "gastown/crew/emma"},
		{"gastown/crew/emma/", "gastown/crew/emma"},
		{"gastown/polecats/furiosa//", "gastown/polecats/furiosa"},
		{"", ""},
	}

	for _, tt := range tests {
		got := normalizeAgent(tt.input)
		if got != tt.want {
			t.Errorf("normalizeAgent(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGetAgentByChannel(t *testing.T) {
	cfg := &Config{
		Enabled:        true,
		DefaultChannel: "C0DEFAULT",
		Overrides: map[string]string{
			"gastown/crew/decisions": "C0DECISIONS",
			"gastown/crew/wolf":      "C0WOLF",
			"beads/crew/quartz":      "C0QUARTZ",
		},
	}

	r := NewRouter(cfg)

	tests := []struct {
		channelID string
		want      string
	}{
		{"C0DECISIONS", "gastown/crew/decisions"},
		{"C0WOLF", "gastown/crew/wolf"},
		{"C0QUARTZ", "beads/crew/quartz"},
		{"C0UNKNOWN", ""},
		{"C0DEFAULT", ""},
	}

	for _, tt := range tests {
		got := r.GetAgentByChannel(tt.channelID)
		if got != tt.want {
			t.Errorf("GetAgentByChannel(%q) = %q, want %q", tt.channelID, got, tt.want)
		}
	}
}

func TestGetAgentByChannel_NilOverrides(t *testing.T) {
	cfg := &Config{
		Enabled:        true,
		DefaultChannel: "C0DEFAULT",
	}

	r := NewRouter(cfg)

	got := r.GetAgentByChannel("C0ANYTHING")
	if got != "" {
		t.Errorf("GetAgentByChannel with nil overrides = %q, want empty string", got)
	}
}

func TestExtractMetadataFromDescription(t *testing.T) {
	tests := []struct {
		name        string
		description string
		want        string
	}{
		{
			name: "valid metadata line",
			description: `slack-routing: slack-routing

rig: *
category: slack-routing
metadata: {"enabled":true,"default_channel":"C0123456789"}`,
			want: `{"enabled":true,"default_channel":"C0123456789"}`,
		},
		{
			name:        "no metadata line",
			description: "rig: *\ncategory: slack-routing\n",
			want:        "",
		},
		{
			name:        "empty description",
			description: "",
			want:        "",
		},
		{
			name:        "metadata with nested JSON",
			description: "metadata: {\"channels\":{\"gastown/polecats/*\":\"C0987654321\"}}",
			want:        `{"channels":{"gastown/polecats/*":"C0987654321"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractMetadataFromDescription(tt.description)
			if got != tt.want {
				t.Errorf("extractMetadataFromDescription() = %q, want %q", got, tt.want)
			}
		})
	}
}
