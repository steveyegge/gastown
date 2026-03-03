package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	t.Run("parses all fields correctly", func(t *testing.T) {
		cfg := ProxyConfig{
			ListenAddr:      "0.0.0.0:9876",
			CADir:           "/tmp/ca",
			TownRoot:        "/tmp/gt",
			AllowedCommands: []string{"gt", "bd"},
			AllowedSubcommands: map[string][]string{
				"gt": {"prime", "hook"},
				"bd": {"create", "show"},
			},
			ExtraSANIPs:   []string{"170.170.170.170", "10.8.0.1"},
			ExtraSANHosts: []string{"proxy.mycompany.com", "gt-proxy.local"},
		}
		data, err := json.Marshal(cfg)
		require.NoError(t, err)

		path := filepath.Join(t.TempDir(), "config.json")
		require.NoError(t, os.WriteFile(path, data, 0644))

		got, err := loadConfig(path)
		require.NoError(t, err)

		assert.Equal(t, "0.0.0.0:9876", got.ListenAddr)
		assert.Equal(t, "/tmp/ca", got.CADir)
		assert.Equal(t, "/tmp/gt", got.TownRoot)
		assert.Equal(t, []string{"gt", "bd"}, got.AllowedCommands)
		assert.Equal(t, []string{"prime", "hook"}, got.AllowedSubcommands["gt"])
		assert.Equal(t, []string{"create", "show"}, got.AllowedSubcommands["bd"])
		assert.Equal(t, []string{"170.170.170.170", "10.8.0.1"}, got.ExtraSANIPs)
		assert.Equal(t, []string{"proxy.mycompany.com", "gt-proxy.local"}, got.ExtraSANHosts)
	})
}

func TestLoadConfigMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.json")
	cfg, err := loadConfig(path)
	require.NoError(t, err, "missing config file should not return error")
	assert.Equal(t, ProxyConfig{}, cfg, "missing config file should return empty ProxyConfig")
}

func TestLoadConfigInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte("{not valid json"), 0644))

	_, err := loadConfig(path)
	assert.Error(t, err, "malformed JSON should return error")
}

func TestLoadConfigInvalidIP(t *testing.T) {
	// Invalid IPs in extra_san_ips are validated in main(), not loadConfig().
	// loadConfig only deserialises the JSON — the IP strings are returned as-is.
	// This test verifies that loadConfig itself does not error on invalid IP strings.
	data := []byte(`{"extra_san_ips": ["not-an-ip", "256.256.256.256", "192.0.2.1"]}`)
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, data, 0644))

	cfg, err := loadConfig(path)
	require.NoError(t, err, "loadConfig should not error on invalid IP strings")
	assert.Equal(t, []string{"not-an-ip", "256.256.256.256", "192.0.2.1"}, cfg.ExtraSANIPs)
}

func TestParseAllowedSubcmds(t *testing.T) {
	t.Run("empty string returns nil", func(t *testing.T) {
		assert.Nil(t, parseAllowedSubcmds(""))
	})

	t.Run("empty subcommand list skipped", func(t *testing.T) {
		// "gt:" has a colon but no subcommands after it — entry is skipped.
		result := parseAllowedSubcmds("gt:")
		assert.Empty(t, result)
		assert.NotContains(t, result, "gt")
	})

	t.Run("missing colon separator skips entry", func(t *testing.T) {
		// Entry with no colon at all is silently ignored.
		result := parseAllowedSubcmds("gtprime")
		assert.Empty(t, result)
	})

	t.Run("missing semicolon between entries treats second group as subcommand", func(t *testing.T) {
		// Without a semicolon, "hookbd:create" is parsed as a single subcommand of gt.
		result := parseAllowedSubcmds("gt:prime,hookbd:create")
		assert.Equal(t, []string{"prime", "hookbd:create"}, result["gt"])
		assert.NotContains(t, result, "bd")
	})

	t.Run("duplicate subcommands are preserved", func(t *testing.T) {
		// The parser does not deduplicate — duplicates survive into the result.
		result := parseAllowedSubcmds("gt:prime,prime,hook")
		assert.Equal(t, []string{"prime", "prime", "hook"}, result["gt"])
	})

	t.Run("single entry without semicolons parses correctly", func(t *testing.T) {
		result := parseAllowedSubcmds("gt:prime,hook,done")
		assert.Equal(t, []string{"prime", "hook", "done"}, result["gt"])
	})

	t.Run("multiple entries parse correctly", func(t *testing.T) {
		result := parseAllowedSubcmds("gt:prime,hook;bd:create,show")
		assert.Equal(t, []string{"prime", "hook"}, result["gt"])
		assert.Equal(t, []string{"create", "show"}, result["bd"])
	})

	t.Run("extra semicolons and empty parts are skipped", func(t *testing.T) {
		result := parseAllowedSubcmds(";gt:prime;;bd:create;")
		assert.Equal(t, []string{"prime"}, result["gt"])
		assert.Equal(t, []string{"create"}, result["bd"])
	})

	t.Run("whitespace is trimmed from cmd and subcommands", func(t *testing.T) {
		result := parseAllowedSubcmds(" gt : prime , hook ; bd : create ")
		assert.Equal(t, []string{"prime", "hook"}, result["gt"])
		assert.Equal(t, []string{"create"}, result["bd"])
	})
}

func TestBuildAllowedSubcmds(t *testing.T) {
	t.Run("nil map returns empty string", func(t *testing.T) {
		assert.Equal(t, "", buildAllowedSubcmds(nil))
	})

	t.Run("empty map returns empty string", func(t *testing.T) {
		assert.Equal(t, "", buildAllowedSubcmds(map[string][]string{}))
	})

	t.Run("single entry serialises correctly", func(t *testing.T) {
		result := buildAllowedSubcmds(map[string][]string{
			"gt": {"prime", "hook"},
		})
		assert.Equal(t, "gt:prime,hook", result)
	})

	t.Run("empty subcommands list produces cmd-colon entry", func(t *testing.T) {
		// buildAllowedSubcmds does not filter out empty sub-lists — it emits "cmd:".
		result := buildAllowedSubcmds(map[string][]string{
			"gt": {},
		})
		assert.Equal(t, "gt:", result)
	})
}

func TestParseAllowedSubcmdsRoundTrip(t *testing.T) {
	t.Run("empty string survives round-trip", func(t *testing.T) {
		// parse("") → nil → build(nil) → "" → parse("") → nil
		parsed := parseAllowedSubcmds("")
		assert.Nil(t, parsed)
		built := buildAllowedSubcmds(parsed)
		assert.Equal(t, "", built)
		assert.Nil(t, parseAllowedSubcmds(built))
	})

	t.Run("single entry: string equality after round-trip", func(t *testing.T) {
		// Single-entry maps have no ordering ambiguity, so the rebuilt string
		// must equal the original.
		original := "gt:prime,hook,done"
		built := buildAllowedSubcmds(parseAllowedSubcmds(original))
		assert.Equal(t, original, built)
	})

	t.Run("multi-entry: map content preserved after round-trip", func(t *testing.T) {
		// Map iteration order is non-deterministic, so we can't compare strings
		// directly. Instead verify that parse → build → parse yields the same map.
		original := "gt:prime,hook,done,mail,nudge;bd:create,update,close,show,list"
		parsed := parseAllowedSubcmds(original)
		reparsed := parseAllowedSubcmds(buildAllowedSubcmds(parsed))

		require.Equal(t, len(parsed), len(reparsed))
		for cmd, subs := range parsed {
			sortedOrig := append([]string(nil), subs...)
			sortedGot := append([]string(nil), reparsed[cmd]...)
			sort.Strings(sortedOrig)
			sort.Strings(sortedGot)
			assert.Equalf(t, sortedOrig, sortedGot, "subcommands for %q differ after round-trip", cmd)
		}
	})

	t.Run("empty sub-list is not preserved across round-trip", func(t *testing.T) {
		// buildAllowedSubcmds emits "gt:" for an empty sub-list, but
		// parseAllowedSubcmds skips entries with no subcommands.
		// So the round-trip does NOT preserve empty sub-lists.
		original := map[string][]string{"gt": {}}
		built := buildAllowedSubcmds(original)
		assert.Equal(t, "gt:", built)
		reparsed := parseAllowedSubcmds(built)
		assert.Empty(t, reparsed, "empty sub-list is lost on round-trip")
	})
}
