package telegram

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_ValidConfigPassesValidation(t *testing.T) {
	cfg := Config{
		Token:  "123456789:ABCDEFabcdef_-1234567890",
		ChatID: -100123456789,
	}
	assert.NoError(t, cfg.Validate())
}

func TestConfig_MissingTokenFails(t *testing.T) {
	cfg := Config{
		ChatID: 12345,
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token")
}

func TestConfig_InvalidTokenFormatFails(t *testing.T) {
	cases := []string{
		"notavalidtoken",
		"123456:abc def",   // space not allowed
		":ABCDEFabcdef",    // missing numeric id
		"123456789:",       // missing secret part
		"abc:ABCDEFabcdef", // non-numeric id
	}
	for _, tok := range cases {
		cfg := Config{Token: tok, ChatID: 12345}
		err := cfg.Validate()
		require.Errorf(t, err, "expected error for token %q", tok)
		assert.Containsf(t, err.Error(), "token", "error should mention token for %q", tok)
	}
}

func TestConfig_MissingChatIDFails(t *testing.T) {
	cfg := Config{
		Token: "123456789:ABCDEFabcdef",
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat_id")
}

func TestConfig_IsEnabled_WithTokenAndEnabled(t *testing.T) {
	cfg := Config{
		Token:   "123456789:ABCDEFabcdef",
		ChatID:  12345,
		Enabled: true,
	}
	assert.True(t, cfg.IsEnabled())
}

func TestConfig_IsEnabled_WithoutToken(t *testing.T) {
	cfg := Config{
		ChatID:  12345,
		Enabled: true,
	}
	assert.False(t, cfg.IsEnabled())
}

func TestConfig_IsEnabled_ExplicitlyDisabled(t *testing.T) {
	cfg := Config{
		Token:   "123456789:ABCDEFabcdef",
		ChatID:  12345,
		Enabled: false,
	}
	assert.False(t, cfg.IsEnabled())
}

func TestConfig_ApplyDefaults(t *testing.T) {
	cfg := Config{}
	cfg.ApplyDefaults()
	assert.Equal(t, "mayor/", cfg.Target)
	assert.Equal(t, 30, cfg.RateLimit)
	assert.Equal(t, []string{"escalations"}, cfg.Notify)
}

func TestConfig_ApplyDefaults_DoesNotOverrideExistingValues(t *testing.T) {
	cfg := Config{
		Target:    "custom/",
		RateLimit: 60,
		Notify:    []string{"all"},
	}
	cfg.ApplyDefaults()
	assert.Equal(t, "custom/", cfg.Target)
	assert.Equal(t, 60, cfg.RateLimit)
	assert.Equal(t, []string{"all"}, cfg.Notify)
}

func TestConfig_MaskedToken(t *testing.T) {
	cfg := Config{Token: "123456789:ABCDEFabcdef"}
	masked := cfg.MaskedToken()
	assert.Equal(t, "****cdef", masked)
}

func TestConfig_MaskedToken_ShortToken(t *testing.T) {
	cfg := Config{Token: "ab"}
	masked := cfg.MaskedToken()
	// For very short tokens, show what we can
	assert.Equal(t, "****ab", masked)
}

func TestConfig_MaskedToken_EmptyToken(t *testing.T) {
	cfg := Config{}
	masked := cfg.MaskedToken()
	assert.Equal(t, "****", masked)
}

func TestConfig_ConfigPath(t *testing.T) {
	path := ConfigPath("/home/user/gt")
	assert.Equal(t, "/home/user/gt/mayor/telegram.json", path)
}

func TestConfig_LoadConfig_RejectsBadPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "telegram.json")

	cfg := Config{
		Token:  "123456789:ABCDEFabcdef",
		ChatID: 12345,
	}
	data, err := json.Marshal(cfg)
	require.NoError(t, err)

	// Write with world-readable permissions (0644) — should be rejected
	require.NoError(t, os.WriteFile(path, data, 0644))

	_, err = LoadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission")
}

func TestConfig_LoadConfig_AcceptsCorrectPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "telegram.json")

	cfg := Config{
		Token:   "123456789:ABCDEFabcdef",
		ChatID:  12345,
		Enabled: true,
	}
	data, err := json.Marshal(cfg)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(path, data, 0600))

	loaded, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, cfg.Token, loaded.Token)
	assert.Equal(t, cfg.ChatID, loaded.ChatID)
	assert.Equal(t, cfg.Enabled, loaded.Enabled)
}

func TestConfig_SaveConfig_WritesWithCorrectPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "telegram.json")

	cfg := Config{
		Token:  "123456789:ABCDEFabcdef",
		ChatID: 12345,
	}
	require.NoError(t, SaveConfig(path, cfg))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestConfig_IsAllowed_EmptyAllowFromBlocksAll(t *testing.T) {
	cfg := Config{AllowFrom: []int64{}}
	assert.False(t, cfg.IsAllowed(12345))
}

func TestConfig_IsAllowed_NilAllowFromBlocksAll(t *testing.T) {
	cfg := Config{AllowFrom: nil}
	assert.False(t, cfg.IsAllowed(12345))
}

func TestConfig_IsAllowed_MatchingUserPasses(t *testing.T) {
	cfg := Config{AllowFrom: []int64{111, 222, 333}}
	assert.True(t, cfg.IsAllowed(222))
}

func TestConfig_IsAllowed_NonMatchingUserBlocked(t *testing.T) {
	cfg := Config{AllowFrom: []int64{111, 222, 333}}
	assert.False(t, cfg.IsAllowed(999))
}
