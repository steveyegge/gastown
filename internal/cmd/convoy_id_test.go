package cmd

import (
	"testing"
)

type uniqueBase36Reader struct {
	n uint64
}

func (r *uniqueBase36Reader) Read(p []byte) (int, error) {
	v := r.n
	r.n++
	for i := len(p) - 1; i >= 0; i-- {
		p[i] = byte(v % 36)
		v /= 36
	}
	return len(p), nil
}

func TestGenerateShortID_Length(t *testing.T) {
	id := generateShortID()
	if len(id) != 5 {
		t.Errorf("generateShortID() = %q (len %d), want length 5", id, len(id))
	}
}

func TestGenerateShortID_ValidChars(t *testing.T) {
	const validChars = "0123456789abcdefghijklmnopqrstuvwxyz"
	valid := make(map[byte]bool)
	for i := range validChars {
		valid[validChars[i]] = true
	}

	for i := 0; i < 100; i++ {
		id := generateShortID()
		for j, c := range []byte(id) {
			if !valid[c] {
				t.Errorf("generateShortID()[%d] = %c, not in base36 alphabet", j, c)
			}
		}
	}
}

func TestGenerateShortID_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	const n = 1000
	reader := &uniqueBase36Reader{}
	for i := 0; i < n; i++ {
		id := generateShortIDFromReader(reader)
		if seen[id] {
			t.Errorf("collision after %d IDs: %q", i, id)
		}
		seen[id] = true
	}
}
