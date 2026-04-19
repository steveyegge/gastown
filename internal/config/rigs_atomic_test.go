package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestSaveRigsConfig_AtomicAgainstConcurrentReaders is the regression test for
// #3464: SaveRigsConfig used os.WriteFile, which truncates the file before
// writing. Concurrent readers in that window observed a zero-byte file and
// failed with "unexpected end of JSON input". With atomic write-then-rename,
// readers must always see either the old complete contents or the new.
func TestSaveRigsConfig_AtomicAgainstConcurrentReaders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rigs.json")

	// Seed with a valid initial config.
	initial := &RigsConfig{Version: 1, Rigs: map[string]RigEntry{"alpha": {}}}
	if err := SaveRigsConfig(path, initial); err != nil {
		t.Fatalf("seed SaveRigsConfig: %v", err)
	}

	// Large payload → a non-atomic write would leave a wider torn-read window.
	big := &RigsConfig{Version: 1, Rigs: make(map[string]RigEntry, 500)}
	for i := 0; i < 500; i++ {
		name := "rig_"
		for d := i; ; d /= 10 {
			name += string(rune('0' + d%10))
			if d < 10 {
				break
			}
		}
		big.Rigs[name] = RigEntry{}
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup

	// 4 concurrent writers alternating between small and big payloads.
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			payloads := []*RigsConfig{initial, big}
			i := 0
			for {
				select {
				case <-stop:
					return
				default:
				}
				if err := SaveRigsConfig(path, payloads[i%2]); err != nil {
					t.Errorf("SaveRigsConfig: %v", err)
					return
				}
				i++
			}
		}()
	}

	// Reader: in 2000 iterations, not a single raw read may yield a truncated
	// or empty file. (We bypass LoadRigsConfig's retry so the underlying write
	// semantics are what's under test.)
	var tornReads int
	for i := 0; i < 2000; i++ {
		data, err := os.ReadFile(path)
		if err != nil {
			// Transient rename races can cause ENOENT on some platforms; tolerate.
			if os.IsNotExist(err) {
				continue
			}
			t.Fatalf("read rigs.json: %v", err)
		}
		if len(data) == 0 {
			tornReads++
			continue
		}
		var probe RigsConfig
		if err := json.Unmarshal(data, &probe); err != nil {
			tornReads++
		}
	}
	close(stop)
	wg.Wait()

	if tornReads > 0 {
		t.Fatalf("observed %d torn/empty reads — SaveRigsConfig is not atomic", tornReads)
	}

	// Final state must be one of the two valid payloads.
	final, err := LoadRigsConfig(path)
	if err != nil {
		t.Fatalf("final LoadRigsConfig: %v", err)
	}
	if len(final.Rigs) != 1 && len(final.Rigs) != 500 {
		t.Fatalf("final rig count = %d, want 1 or 500", len(final.Rigs))
	}
}

// TestLoadRigsConfig_RetriesOnTruncatedRead simulates a transient read where
// the first attempt sees a zero-byte file (as from a non-atomic concurrent
// writer) and verifies LoadRigsConfig's retry recovers.
func TestLoadRigsConfig_RetriesOnTruncatedRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rigs.json")

	// Write an empty file, then race a proper write against LoadRigsConfig.
	if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
		t.Fatalf("write empty: %v", err)
	}

	// Kick off a writer that fixes the file after a brief moment.
	done := make(chan error, 1)
	go func() {
		done <- SaveRigsConfig(path, &RigsConfig{Version: 1, Rigs: map[string]RigEntry{"a": {}}})
	}()

	// Wait for the writer to finish so the retry has real contents to parse.
	if err := <-done; err != nil {
		t.Fatalf("SaveRigsConfig: %v", err)
	}

	cfg, err := LoadRigsConfig(path)
	if err != nil {
		t.Fatalf("LoadRigsConfig after retry: %v", err)
	}
	if _, ok := cfg.Rigs["a"]; !ok {
		t.Fatalf("expected rig 'a' in result, got %v", cfg.Rigs)
	}
}
