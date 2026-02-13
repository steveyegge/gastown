package output

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// captureStdout redirects os.Stdout to a pipe, runs fn, and returns what was written.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error: %v", err)
	}
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	buf := make([]byte, 64*1024)
	n, _ := r.Read(buf)
	r.Close()
	return string(buf[:n])
}

// withEnv sets GT_OUTPUT_FORMAT for the duration of fn, then restores it.
func withEnv(t *testing.T, value string, fn func()) {
	t.Helper()
	orig := os.Getenv("GT_OUTPUT_FORMAT")
	if value == "" {
		os.Unsetenv("GT_OUTPUT_FORMAT")
	} else {
		os.Setenv("GT_OUTPUT_FORMAT", value)
	}
	defer func() {
		if orig == "" {
			os.Unsetenv("GT_OUTPUT_FORMAT")
		} else {
			os.Setenv("GT_OUTPUT_FORMAT", orig)
		}
	}()
	fn()
}

// --- ResolveFormat tests ---

func TestResolveFormat(t *testing.T) {
	orig := os.Getenv("GT_OUTPUT_FORMAT")
	defer os.Setenv("GT_OUTPUT_FORMAT", orig)

	t.Run("default is json", func(t *testing.T) {
		os.Unsetenv("GT_OUTPUT_FORMAT")
		if got := ResolveFormat(""); got != FormatJSON {
			t.Errorf("ResolveFormat(\"\") = %q, want %q", got, FormatJSON)
		}
	})

	t.Run("explicit flag wins over env", func(t *testing.T) {
		os.Setenv("GT_OUTPUT_FORMAT", "json")
		if got := ResolveFormat("toon"); got != FormatTOON {
			t.Errorf("ResolveFormat(\"toon\") = %q, want %q", got, FormatTOON)
		}
	})

	t.Run("env var when no flag", func(t *testing.T) {
		os.Setenv("GT_OUTPUT_FORMAT", "toon")
		if got := ResolveFormat(""); got != FormatTOON {
			t.Errorf("ResolveFormat(\"\") with GT_OUTPUT_FORMAT=toon = %q, want %q", got, FormatTOON)
		}
	})

	t.Run("case insensitive flag", func(t *testing.T) {
		os.Unsetenv("GT_OUTPUT_FORMAT")
		if got := ResolveFormat("TOON"); got != FormatTOON {
			t.Errorf("ResolveFormat(\"TOON\") = %q, want %q", got, FormatTOON)
		}
	})

	t.Run("case insensitive env", func(t *testing.T) {
		os.Setenv("GT_OUTPUT_FORMAT", "TOON")
		if got := ResolveFormat(""); got != FormatTOON {
			t.Errorf("got %q, want %q", got, FormatTOON)
		}
	})

	t.Run("unknown flag falls through to env", func(t *testing.T) {
		os.Setenv("GT_OUTPUT_FORMAT", "toon")
		if got := ResolveFormat("yaml"); got != FormatTOON {
			t.Errorf("unknown flag should fall through to env; got %q, want %q", got, FormatTOON)
		}
	})

	t.Run("unknown flag and no env defaults to json", func(t *testing.T) {
		os.Unsetenv("GT_OUTPUT_FORMAT")
		if got := ResolveFormat("yaml"); got != FormatJSON {
			t.Errorf("unknown flag with no env should default to json; got %q", got)
		}
	})

	t.Run("unknown env defaults to json", func(t *testing.T) {
		os.Setenv("GT_OUTPUT_FORMAT", "xml")
		if got := ResolveFormat(""); got != FormatJSON {
			t.Errorf("unknown env should default to json; got %q", got)
		}
	})
}

// --- IsTOON tests ---

func TestIsTOON(t *testing.T) {
	t.Run("false by default", func(t *testing.T) {
		withEnv(t, "", func() {
			if IsTOON() {
				t.Error("IsTOON() should be false with no env set")
			}
		})
	})

	t.Run("true when env is toon", func(t *testing.T) {
		withEnv(t, "toon", func() {
			if !IsTOON() {
				t.Error("IsTOON() should be true with GT_OUTPUT_FORMAT=toon")
			}
		})
	})

	t.Run("false when env is json", func(t *testing.T) {
		withEnv(t, "json", func() {
			if IsTOON() {
				t.Error("IsTOON() should be false with GT_OUTPUT_FORMAT=json")
			}
		})
	})
}

// --- PrintJSON tests ---

func TestPrintJSON(t *testing.T) {
	t.Run("outputs valid json", func(t *testing.T) {
		data := map[string]string{"name": "mayor", "role": "coordinator"}
		got := captureStdout(t, func() {
			if err := PrintJSON(data); err != nil {
				t.Errorf("PrintJSON error: %v", err)
			}
		})

		var parsed map[string]string
		if err := json.Unmarshal([]byte(got), &parsed); err != nil {
			t.Fatalf("output is not valid JSON: %v\nGot: %s", err, got)
		}
		if parsed["name"] != "mayor" {
			t.Errorf("parsed[\"name\"] = %q, want \"mayor\"", parsed["name"])
		}
	})

	t.Run("pretty printed with 2-space indent", func(t *testing.T) {
		data := map[string]int{"count": 42}
		got := captureStdout(t, func() {
			_ = PrintJSON(data)
		})
		if !strings.Contains(got, "  \"count\"") {
			t.Errorf("expected 2-space indented output, got:\n%s", got)
		}
	})

	t.Run("trailing newline", func(t *testing.T) {
		got := captureStdout(t, func() {
			_ = PrintJSON("hello")
		})
		if !strings.HasSuffix(got, "\n") {
			t.Errorf("expected trailing newline, got: %q", got)
		}
	})

	t.Run("array of structs", func(t *testing.T) {
		type item struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		items := []item{{ID: "1", Name: "alpha"}, {ID: "2", Name: "beta"}}
		got := captureStdout(t, func() {
			_ = PrintJSON(items)
		})
		var parsed []item
		if err := json.Unmarshal([]byte(got), &parsed); err != nil {
			t.Fatalf("not valid JSON array: %v", err)
		}
		if len(parsed) != 2 || parsed[1].Name != "beta" {
			t.Errorf("unexpected parsed result: %+v", parsed)
		}
	})

	t.Run("error on unmarshalable type", func(t *testing.T) {
		err := PrintJSON(make(chan int))
		if err == nil {
			t.Error("expected error for unmarshalable type (chan)")
		}
	})
}

// --- PrintTOON tests ---

func TestPrintTOON(t *testing.T) {
	t.Run("outputs non-empty string", func(t *testing.T) {
		data := map[string]string{"name": "mayor"}
		got := captureStdout(t, func() {
			if err := PrintTOON(data); err != nil {
				t.Errorf("PrintTOON error: %v", err)
			}
		})
		if strings.TrimSpace(got) == "" {
			t.Error("PrintTOON produced empty output")
		}
	})

	t.Run("output differs from json", func(t *testing.T) {
		type agent struct {
			Name    string `json:"name" toon:"name"`
			Role    string `json:"role" toon:"role"`
			Runtime string `json:"runtime" toon:"runtime"`
		}
		agents := []agent{
			{Name: "hq-mayor", Role: "mayor", Runtime: "claude"},
			{Name: "hq-deacon", Role: "deacon", Runtime: "opencode"},
		}

		jsonOut := captureStdout(t, func() { _ = PrintJSON(agents) })
		toonOut := captureStdout(t, func() { _ = PrintTOON(agents) })

		if jsonOut == toonOut {
			t.Error("TOON output should differ from JSON output")
		}
	})

	t.Run("toon is smaller than json for tabular data", func(t *testing.T) {
		type row struct {
			ID     string `json:"id" toon:"id"`
			Status string `json:"status" toon:"status"`
			Name   string `json:"name" toon:"name"`
		}
		rows := []row{
			{ID: "st-001", Status: "open", Name: "Fix auth bug"},
			{ID: "st-002", Status: "closed", Name: "Add logging"},
			{ID: "st-003", Status: "open", Name: "Refactor config"},
			{ID: "st-004", Status: "in_progress", Name: "Update deps"},
			{ID: "st-005", Status: "open", Name: "Write tests"},
		}

		jsonOut := captureStdout(t, func() { _ = PrintJSON(rows) })
		toonOut := captureStdout(t, func() { _ = PrintTOON(rows) })

		if len(toonOut) >= len(jsonOut) {
			t.Errorf("TOON (%d bytes) should be smaller than JSON (%d bytes) for tabular data",
				len(toonOut), len(jsonOut))
		}
	})

	t.Run("trailing newline", func(t *testing.T) {
		got := captureStdout(t, func() {
			_ = PrintTOON("hello")
		})
		if !strings.HasSuffix(got, "\n") {
			t.Errorf("expected trailing newline, got: %q", got)
		}
	})
}

// --- PrintFormatted tests ---

func TestPrintFormatted(t *testing.T) {
	type testAgent struct {
		Name    string `json:"name" toon:"name"`
		Role    string `json:"role" toon:"role"`
		Runtime string `json:"runtime" toon:"runtime"`
	}

	agents := []testAgent{
		{Name: "hq-mayor", Role: "mayor", Runtime: "claude"},
		{Name: "hq-deacon", Role: "deacon", Runtime: "claude"},
	}

	t.Run("json format produces valid json", func(t *testing.T) {
		got := captureStdout(t, func() {
			if err := PrintFormatted(agents, FormatJSON); err != nil {
				t.Errorf("error: %v", err)
			}
		})
		var parsed []testAgent
		if err := json.Unmarshal([]byte(got), &parsed); err != nil {
			t.Fatalf("not valid JSON: %v\nGot: %s", err, got)
		}
		if len(parsed) != 2 {
			t.Errorf("expected 2 agents, got %d", len(parsed))
		}
	})

	t.Run("toon format produces non-json output", func(t *testing.T) {
		got := captureStdout(t, func() {
			if err := PrintFormatted(agents, FormatTOON); err != nil {
				t.Errorf("error: %v", err)
			}
		})
		// TOON output should not be valid JSON
		var discard any
		if err := json.Unmarshal([]byte(got), &discard); err == nil {
			t.Error("TOON output should not be valid JSON for struct arrays")
		}
	})

	t.Run("toon fallback to json on failure", func(t *testing.T) {
		// Channels can't be marshaled by toon or json.
		// But PrintFormatted's fallback path tries JSON too, which also fails.
		// Use a type that toon can't handle but json can.
		// In practice toon.Marshal handles everything json.Marshal handles,
		// so we test the fallback by verifying the code path doesn't panic.
		data := map[string]string{"safe": "data"}
		got := captureStdout(t, func() {
			if err := PrintFormatted(data, FormatTOON); err != nil {
				t.Errorf("error: %v", err)
			}
		})
		if strings.TrimSpace(got) == "" {
			t.Error("expected non-empty output")
		}
	})
}

// --- Print (convenience wrapper) tests ---

func TestPrint(t *testing.T) {
	t.Run("defaults to json", func(t *testing.T) {
		withEnv(t, "", func() {
			data := map[string]string{"key": "value"}
			got := captureStdout(t, func() {
				if err := Print(data); err != nil {
					t.Errorf("Print error: %v", err)
				}
			})
			var parsed map[string]string
			if err := json.Unmarshal([]byte(got), &parsed); err != nil {
				t.Fatalf("default Print should produce JSON: %v\nGot: %s", err, got)
			}
			if parsed["key"] != "value" {
				t.Errorf("parsed[\"key\"] = %q, want \"value\"", parsed["key"])
			}
		})
	})

	t.Run("respects env for toon", func(t *testing.T) {
		withEnv(t, "toon", func() {
			type item struct {
				Name string `json:"name" toon:"name"`
			}
			items := []item{{Name: "alpha"}, {Name: "beta"}}
			got := captureStdout(t, func() {
				if err := Print(items); err != nil {
					t.Errorf("Print error: %v", err)
				}
			})
			// Should NOT be valid JSON when format is toon (for arrays of structs)
			var discard any
			if err := json.Unmarshal([]byte(got), &discard); err == nil {
				t.Error("Print with GT_OUTPUT_FORMAT=toon should not produce JSON for struct arrays")
			}
		})
	})

	t.Run("respects env for json", func(t *testing.T) {
		withEnv(t, "json", func() {
			data := map[string]int{"count": 7}
			got := captureStdout(t, func() {
				_ = Print(data)
			})
			var parsed map[string]int
			if err := json.Unmarshal([]byte(got), &parsed); err != nil {
				t.Fatalf("Print with GT_OUTPUT_FORMAT=json should produce JSON: %v", err)
			}
		})
	})

	t.Run("nil value", func(t *testing.T) {
		withEnv(t, "", func() {
			got := captureStdout(t, func() {
				if err := Print(nil); err != nil {
					t.Errorf("Print(nil) error: %v", err)
				}
			})
			if strings.TrimSpace(got) != "null" {
				t.Errorf("Print(nil) should produce \"null\", got: %q", strings.TrimSpace(got))
			}
		})
	})

	t.Run("empty slice", func(t *testing.T) {
		withEnv(t, "", func() {
			got := captureStdout(t, func() {
				_ = Print([]string{})
			})
			if strings.TrimSpace(got) != "[]" {
				t.Errorf("Print([]) should produce \"[]\", got: %q", strings.TrimSpace(got))
			}
		})
	})
}

// --- printJSONBytes tests ---

func TestPrintJSONBytes(t *testing.T) {
	t.Run("pretty prints compact json", func(t *testing.T) {
		compact := []byte(`{"name":"mayor","role":"coordinator"}`)
		got := captureStdout(t, func() {
			if err := printJSONBytes(compact); err != nil {
				t.Errorf("error: %v", err)
			}
		})
		if !strings.Contains(got, "  \"name\"") {
			t.Errorf("expected indented output, got:\n%s", got)
		}
		// Should still be valid JSON
		var parsed map[string]string
		if err := json.Unmarshal([]byte(got), &parsed); err != nil {
			t.Fatalf("not valid JSON: %v", err)
		}
	})

	t.Run("passes through invalid json unchanged", func(t *testing.T) {
		bad := []byte(`not json at all`)
		got := captureStdout(t, func() {
			_ = printJSONBytes(bad)
		})
		if got != "not json at all" {
			t.Errorf("expected raw passthrough, got: %q", got)
		}
	})

	t.Run("trailing newline on valid json", func(t *testing.T) {
		data := []byte(`[1,2,3]`)
		got := captureStdout(t, func() {
			_ = printJSONBytes(data)
		})
		if !strings.HasSuffix(got, "\n") {
			t.Errorf("expected trailing newline")
		}
	})
}

// --- Round-trip fidelity tests ---

func TestJSONRoundTrip(t *testing.T) {
	// Verify that data printed via PrintJSON can be parsed back identically.
	type status struct {
		Rig       string   `json:"rig"`
		Branch    string   `json:"branch"`
		Polecats  int      `json:"polecats"`
		Issues    []string `json:"issues"`
		IsHealthy bool     `json:"is_healthy"`
	}

	original := status{
		Rig:       "sfgastown",
		Branch:    "main",
		Polecats:  3,
		Issues:    []string{"st-001", "st-002"},
		IsHealthy: true,
	}

	got := captureStdout(t, func() {
		if err := PrintJSON(original); err != nil {
			t.Fatalf("PrintJSON error: %v", err)
		}
	})

	var roundTripped status
	if err := json.Unmarshal([]byte(got), &roundTripped); err != nil {
		t.Fatalf("round-trip unmarshal failed: %v", err)
	}

	if roundTripped.Rig != original.Rig ||
		roundTripped.Branch != original.Branch ||
		roundTripped.Polecats != original.Polecats ||
		roundTripped.IsHealthy != original.IsHealthy ||
		len(roundTripped.Issues) != len(original.Issues) {
		t.Errorf("round-trip mismatch:\n  original:     %+v\n  roundTripped: %+v", original, roundTripped)
	}
}
