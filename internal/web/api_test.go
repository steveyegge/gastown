package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateCommand(t *testing.T) {
	tests := []struct {
		name      string
		command   string
		wantErr   bool
		wantSafe  bool
		errSubstr string
	}{
		// Allowed safe commands
		{
			name:     "status command",
			command:  "status",
			wantErr:  false,
			wantSafe: true,
		},
		{
			name:     "status with flags",
			command:  "status --json",
			wantErr:  false,
			wantSafe: true,
		},
		{
			name:     "convoy list",
			command:  "convoy list",
			wantErr:  false,
			wantSafe: true,
		},
		{
			name:     "mail inbox",
			command:  "mail inbox",
			wantErr:  false,
			wantSafe: true,
		},

		// Allowed but requires confirmation
		{
			name:     "mail send",
			command:  "mail send foo bar",
			wantErr:  false,
			wantSafe: false,
		},
		{
			name:     "convoy create",
			command:  "convoy create myconvoy",
			wantErr:  false,
			wantSafe: false,
		},

		// Blocked patterns
		{
			name:      "force flag blocked",
			command:   "reset --force",
			wantErr:   true,
			errSubstr: "blocked pattern",
		},
		{
			name:      "delete blocked",
			command:   "delete something",
			wantErr:   true,
			errSubstr: "blocked pattern",
		},
		{
			name:      "kill blocked",
			command:   "kill session",
			wantErr:   true,
			errSubstr: "blocked pattern",
		},
		{
			name:      "rm blocked",
			command:   "rm -rf /",
			wantErr:   true,
			errSubstr: "blocked pattern",
		},

		// Not in whitelist
		{
			name:      "unknown command",
			command:   "randomcmd foo",
			wantErr:   true,
			errSubstr: "not in whitelist",
		},
		{
			name:      "empty command",
			command:   "",
			wantErr:   true,
			errSubstr: "empty command",
		},
		{
			name:      "whitespace only",
			command:   "   ",
			wantErr:   true,
			errSubstr: "empty command",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta, err := ValidateCommand(tt.command)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateCommand(%q) expected error containing %q, got nil", tt.command, tt.errSubstr)
					return
				}
				if tt.errSubstr != "" && !bytes.Contains([]byte(err.Error()), []byte(tt.errSubstr)) {
					t.Errorf("ValidateCommand(%q) error = %q, want error containing %q", tt.command, err.Error(), tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Errorf("ValidateCommand(%q) unexpected error: %v", tt.command, err)
				return
			}
			if meta.Safe != tt.wantSafe {
				t.Errorf("ValidateCommand(%q) Safe = %v, want %v", tt.command, meta.Safe, tt.wantSafe)
			}
		})
	}
}

func TestSanitizeArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "clean args unchanged",
			args: []string{"status", "--json", "--fast"},
			want: []string{"status", "--json", "--fast"},
		},
		{
			name: "removes semicolon",
			args: []string{"status; rm -rf /"},
			want: []string{"status rm -rf /"},
		},
		{
			name: "removes pipe",
			args: []string{"status | cat"},
			want: []string{"status  cat"},
		},
		{
			name: "removes shell metacharacters",
			args: []string{"$(whoami)", "`id`", "${HOME}"},
			want: []string{"whoami", "id", "HOME"},
		},
		{
			name: "removes newlines",
			args: []string{"foo\nbar", "baz\rbat"},
			want: []string{"foobar", "bazbat"},
		},
		{
			name: "empty after sanitize removed",
			args: []string{"$()"},
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeArgs(tt.args)
			if len(got) != len(tt.want) {
				t.Errorf("SanitizeArgs(%v) = %v, want %v", tt.args, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("SanitizeArgs(%v)[%d] = %q, want %q", tt.args, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseCommandArgs(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    []string
	}{
		{
			name:    "simple command",
			command: "status",
			want:    []string{"status"},
		},
		{
			name:    "command with args",
			command: "mail send foo bar",
			want:    []string{"mail", "send", "foo", "bar"},
		},
		{
			name:    "command with flags",
			command: "status --json --fast",
			want:    []string{"status", "--json", "--fast"},
		},
		{
			name:    "quoted string",
			command: `mail send "hello world"`,
			want:    []string{"mail", "send", "hello world"},
		},
		{
			name:    "single quoted string",
			command: `mail send 'hello world'`,
			want:    []string{"mail", "send", "hello world"},
		},
		{
			name:    "mixed quotes",
			command: `mail send "hello 'nested'" world`,
			want:    []string{"mail", "send", "hello 'nested'", "world"},
		},
		{
			name:    "extra whitespace",
			command: "  status   --json  ",
			want:    []string{"status", "--json"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCommandArgs(tt.command)
			if len(got) != len(tt.want) {
				t.Errorf("parseCommandArgs(%q) = %v, want %v", tt.command, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseCommandArgs(%q)[%d] = %q, want %q", tt.command, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestAPIHandler_Commands(t *testing.T) {
	handler := NewAPIHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/commands", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /api/commands status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp CommandListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(resp.Commands) == 0 {
		t.Error("Expected non-empty command list")
	}

	// Verify some expected commands are present
	foundStatus := false
	foundMailSend := false
	for _, cmd := range resp.Commands {
		if cmd.Name == "status" {
			foundStatus = true
			if !cmd.Safe {
				t.Error("status command should be safe")
			}
		}
		if cmd.Name == "mail send" {
			foundMailSend = true
			if !cmd.Confirm {
				t.Error("mail send should require confirmation")
			}
		}
	}
	if !foundStatus {
		t.Error("Expected 'status' in command list")
	}
	if !foundMailSend {
		t.Error("Expected 'mail send' in command list")
	}
}

func TestAPIHandler_Run_BlockedCommand(t *testing.T) {
	handler := NewAPIHandler()

	body := `{"command": "delete everything"}`
	req := httptest.NewRequest(http.MethodPost, "/api/run", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("POST /api/run blocked command status = %d, want %d", w.Code, http.StatusForbidden)
	}

	var resp CommandResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Success {
		t.Error("Expected success=false for blocked command")
	}
	if resp.Error == "" {
		t.Error("Expected error message for blocked command")
	}
}

func TestAPIHandler_Run_InvalidJSON(t *testing.T) {
	handler := NewAPIHandler()

	body := `{invalid json}`
	req := httptest.NewRequest(http.MethodPost, "/api/run", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("POST /api/run invalid JSON status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAPIHandler_Run_EmptyCommand(t *testing.T) {
	handler := NewAPIHandler()

	body := `{"command": ""}`
	req := httptest.NewRequest(http.MethodPost, "/api/run", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("POST /api/run empty command status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestAPIHandler_NotFound(t *testing.T) {
	handler := NewAPIHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/unknown", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("GET /api/unknown status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestGetCommandList(t *testing.T) {
	commands := GetCommandList()

	if len(commands) == 0 {
		t.Error("GetCommandList returned empty list")
	}

	// Check that all commands have required fields
	for _, cmd := range commands {
		if cmd.Name == "" {
			t.Error("Command has empty name")
		}
		if cmd.Desc == "" {
			t.Errorf("Command %q has empty description", cmd.Name)
		}
		if cmd.Category == "" {
			t.Errorf("Command %q has empty category", cmd.Name)
		}
	}
}
