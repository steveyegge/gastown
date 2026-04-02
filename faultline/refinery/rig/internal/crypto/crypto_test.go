package crypto

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func testKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, KeySize)
	for i := range key {
		key[i] = byte(i)
	}
	return key
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := testKey(t)
	plain := "root:s3cret@tcp(127.0.0.1:3306)/faultline"

	ct, err := Encrypt(plain, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	got, err := Decrypt(ct, key)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if got != plain {
		t.Errorf("round-trip mismatch: got %q, want %q", got, plain)
	}
}

func TestEncryptProducesDifferentCiphertexts(t *testing.T) {
	key := testKey(t)
	plain := "test"

	ct1, _ := Encrypt(plain, key)
	ct2, _ := Encrypt(plain, key)

	if ct1 == ct2 {
		t.Error("two encryptions of the same plaintext should differ (unique nonces)")
	}
}

func TestEncryptBadKeySize(t *testing.T) {
	_, err := Encrypt("test", []byte("short"))
	if err == nil {
		t.Error("expected error for short key")
	}
}

func TestDecryptBadKeySize(t *testing.T) {
	_, err := Decrypt("aabb", []byte("short"))
	if err == nil {
		t.Error("expected error for short key")
	}
}

func TestDecryptBadHex(t *testing.T) {
	key := testKey(t)
	_, err := Decrypt("not-hex!", key)
	if err == nil {
		t.Error("expected error for invalid hex")
	}
}

func TestDecryptTamperedCiphertext(t *testing.T) {
	key := testKey(t)
	ct, _ := Encrypt("secret", key)

	// Flip a byte in the ciphertext.
	raw, _ := hex.DecodeString(ct)
	raw[len(raw)-1] ^= 0xff
	tampered := hex.EncodeToString(raw)

	_, err := Decrypt(tampered, key)
	if err == nil {
		t.Error("expected error for tampered ciphertext")
	}
}

func TestMaskConnectionString(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"root:s3cret@tcp(127.0.0.1:3306)/db", "root:***@tcp(127.0.0.1:3306)/db"},
		{"user:pass@localhost/db", "user:***@localhost/db"},
		{"nopassword@host/db", "nopassword@host/db"},
		{"nocolon", "nocolon"},
		{"", ""},
	}

	for _, tt := range tests {
		got := MaskConnectionString(tt.input)
		if got != tt.want {
			t.Errorf("MaskConnectionString(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestLoadKeyFromEnv(t *testing.T) {
	keyHex := hex.EncodeToString(testKey(t))
	t.Setenv(EnvKey, keyHex)

	key, err := LoadKey()
	if err != nil {
		t.Fatalf("LoadKey: %v", err)
	}
	if len(key) != KeySize {
		t.Errorf("key length = %d, want %d", len(key), KeySize)
	}
}

func TestLoadKeyFromEnvBadHex(t *testing.T) {
	t.Setenv(EnvKey, "not-hex-at-all")
	_, err := LoadKey()
	if err == nil {
		t.Error("expected error for bad hex in env")
	}
}

func TestLoadKeyFromEnvWrongLength(t *testing.T) {
	t.Setenv(EnvKey, "aabb") // too short
	_, err := LoadKey()
	if err == nil {
		t.Error("expected error for wrong length key")
	}
}

func TestLoadKeyAutoGenerate(t *testing.T) {
	// Use a temp dir as home so we don't touch real files.
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, ".faultline", "encryption.key")

	// Clear env so LoadKey falls through to file.
	t.Setenv(EnvKey, "")
	// Override HOME so defaultKeyPath uses our temp dir.
	t.Setenv("HOME", tmpDir)

	key, err := LoadKey()
	if err != nil {
		t.Fatalf("LoadKey (auto-gen): %v", err)
	}
	if len(key) != KeySize {
		t.Errorf("generated key length = %d, want %d", len(key), KeySize)
	}

	// Verify file was created with correct permissions.
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("key file not created: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("key file permissions = %o, want 0600", perm)
	}

	// Loading again should return the same key.
	key2, err := LoadKey()
	if err != nil {
		t.Fatalf("LoadKey (second): %v", err)
	}
	if hex.EncodeToString(key) != hex.EncodeToString(key2) {
		t.Error("second LoadKey returned different key")
	}
}
