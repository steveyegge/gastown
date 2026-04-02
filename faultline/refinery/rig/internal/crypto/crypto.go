package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	// KeySize is the required key size in bytes (AES-256).
	KeySize = 32
	// EnvKey is the environment variable for the hex-encoded encryption key.
	EnvKey = "FAULTLINE_DB_ENCRYPTION_KEY"
)

// Encrypt encrypts plaintext using AES-256-GCM with the given 32-byte key.
// Returns hex-encoded ciphertext (nonce + sealed data).
func Encrypt(plaintext string, key []byte) (string, error) {
	if len(key) != KeySize {
		return "", fmt.Errorf("crypto: key must be %d bytes, got %d", KeySize, len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("crypto: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("crypto: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("crypto: generate nonce: %w", err)
	}

	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(sealed), nil
}

// Decrypt decrypts hex-encoded ciphertext (nonce + sealed data) using AES-256-GCM.
func Decrypt(ciphertext string, key []byte) (string, error) {
	if len(key) != KeySize {
		return "", fmt.Errorf("crypto: key must be %d bytes, got %d", KeySize, len(key))
	}

	data, err := hex.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("crypto: invalid hex: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("crypto: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("crypto: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("crypto: ciphertext too short")
	}

	nonce, sealed := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", fmt.Errorf("crypto: decrypt: %w", err)
	}

	return string(plaintext), nil
}

// MaskConnectionString masks the password in a DSN connection string.
// Supports user:pass@host and user:pass@tcp(host) formats.
// Returns the DSN with the password replaced by "***".
func MaskConnectionString(connStr string) string {
	// Find user:password@... pattern
	atIdx := strings.Index(connStr, "@")
	if atIdx < 0 {
		return connStr
	}

	userPass := connStr[:atIdx]
	colonIdx := strings.Index(userPass, ":")
	if colonIdx < 0 {
		return connStr
	}

	return userPass[:colonIdx+1] + "***" + connStr[atIdx:]
}

// LoadKey loads the encryption key from the environment variable or the
// default key file at ~/.faultline/encryption.key. If neither exists,
// a new key is generated and saved to the default path with mode 0600.
func LoadKey() ([]byte, error) {
	if envVal := os.Getenv(EnvKey); envVal != "" {
		key, err := hex.DecodeString(envVal)
		if err != nil {
			return nil, fmt.Errorf("crypto: %s invalid hex: %w", EnvKey, err)
		}
		if len(key) != KeySize {
			return nil, fmt.Errorf("crypto: %s must be %d hex chars (got %d)", EnvKey, KeySize*2, len(envVal))
		}
		return key, nil
	}

	keyPath, err := defaultKeyPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(keyPath)
	if err == nil {
		key, err := hex.DecodeString(strings.TrimSpace(string(data)))
		if err != nil {
			return nil, fmt.Errorf("crypto: %s invalid hex: %w", keyPath, err)
		}
		if len(key) != KeySize {
			return nil, fmt.Errorf("crypto: key in %s must be %d hex chars", keyPath, KeySize*2)
		}
		return key, nil
	}

	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("crypto: read key file: %w", err)
	}

	// Auto-generate a new key.
	key := make([]byte, KeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("crypto: generate key: %w", err)
	}

	dir := filepath.Dir(keyPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("crypto: create dir %s: %w", dir, err)
	}

	if err := os.WriteFile(keyPath, []byte(hex.EncodeToString(key)+"\n"), 0600); err != nil {
		return nil, fmt.Errorf("crypto: write key file: %w", err)
	}

	return key, nil
}

func defaultKeyPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("crypto: user home dir: %w", err)
	}
	return filepath.Join(home, ".faultline", "encryption.key"), nil
}
