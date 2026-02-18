//go:build darwin

package quota

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	// keychainServiceBase is the base service name Claude Code uses for keychain credentials.
	keychainServiceBase = "Claude Code-credentials"

	// defaultClaudeConfigDir is Claude Code's default config directory (no suffix in keychain).
	defaultClaudeConfigDir = ".claude"
)

// KeychainCredential holds a backup of a keychain credential for rollback.
type KeychainCredential struct {
	ServiceName string // keychain service name
	Token       string // backed-up token value
}

// KeychainServiceName computes the macOS Keychain service name for a given config dir path.
// Claude Code stores OAuth tokens under: "Claude Code-credentials-<sha256(configDir)[:8]>"
// The default config dir (~/.claude) uses the bare name "Claude Code-credentials" (no suffix).
func KeychainServiceName(configDirPath string) string {
	// Expand ~ to home dir for consistent hashing
	expanded := expandTilde(configDirPath)

	// Check if this is the default config dir (~/.claude or /Users/xxx/.claude)
	home, err := os.UserHomeDir()
	if err == nil {
		defaultPath := home + "/" + defaultClaudeConfigDir
		if expanded == defaultPath {
			return keychainServiceBase
		}
	}

	// Non-default dir: append first 8 chars of SHA-256 hex
	h := sha256.Sum256([]byte(expanded))
	return fmt.Sprintf("%s-%x", keychainServiceBase, h[:4])
}

// ReadKeychainToken reads the password/token for a keychain service name.
func ReadKeychainToken(serviceName string) (string, error) {
	cmd := exec.Command("security", "find-generic-password", "-s", serviceName, "-w")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("reading keychain token for %q: %w", serviceName, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// WriteKeychainToken writes (or updates) a token in the macOS Keychain.
// The -U flag updates the existing entry if it exists.
func WriteKeychainToken(serviceName, accountLabel, token string) error {
	cmd := exec.Command("security", "add-generic-password",
		"-U",
		"-s", serviceName,
		"-a", accountLabel,
		"-w", token,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("writing keychain token for %q: %s: %w", serviceName, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// SwapKeychainCredential backs up the target's keychain token, then overwrites it
// with the source's token. Returns the backup for rollback via RestoreKeychainToken.
//
// This is the core of context-preserving rotation: by swapping the token in the
// target config dir's keychain entry (rather than changing CLAUDE_CONFIG_DIR),
// the respawned session reads a fresh auth token while /resume still finds the
// previous session transcript.
func SwapKeychainCredential(targetConfigDir, sourceConfigDir string) (*KeychainCredential, error) {
	targetSvc := KeychainServiceName(targetConfigDir)
	sourceSvc := KeychainServiceName(sourceConfigDir)

	// Step 1: Back up the target's current token
	backupToken, err := ReadKeychainToken(targetSvc)
	if err != nil {
		return nil, fmt.Errorf("backing up target token: %w", err)
	}

	// Step 2: Read the source's token (the fresh, non-rate-limited one)
	sourceToken, err := ReadKeychainToken(sourceSvc)
	if err != nil {
		return nil, fmt.Errorf("reading source token: %w", err)
	}

	// Step 3: Write the source's token into the target's keychain entry
	if err := WriteKeychainToken(targetSvc, "claude-code", sourceToken); err != nil {
		return nil, fmt.Errorf("writing source token to target keychain: %w", err)
	}

	return &KeychainCredential{
		ServiceName: targetSvc,
		Token:       backupToken,
	}, nil
}

// RestoreKeychainToken writes the backup token back to the keychain,
// undoing a previous SwapKeychainCredential.
func RestoreKeychainToken(backup *KeychainCredential) error {
	if backup == nil {
		return nil
	}
	return WriteKeychainToken(backup.ServiceName, "claude-code", backup.Token)
}

// expandTilde expands a leading ~/ to the user's home directory.
func expandTilde(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return home + path[1:]
		}
	}
	return path
}
