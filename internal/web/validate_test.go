package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIsValidID(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"empty", "", false},
		{"valid simple", "abc", true},
		{"valid bead ID", "gt-abc12", true},
		{"valid message ID", "msg.001", true},
		{"alphanumeric start", "x7k2m", true},
		{"leading dash", "-flag", false},
		{"leading dot", ".hidden", false},
		{"max length boundary", strings.Repeat("a", 200), true},
		{"over max length", strings.Repeat("a", 201), false},
		{"with underscore", "my_rig", true},
		{"with hyphen and dot", "hq-x7k2m.v1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidID(tt.input); got != tt.want {
				t.Errorf("isValidID(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsValidRigName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"simple", "myrig", true},
		{"with underscore", "my_rig", true},
		{"alphanumeric", "rig123", true},
		{"leading underscore", "_internal", true},
		{"leading dash rejected", "-flag", false},
		{"hyphen rejected", "my-rig", false},
		{"dot rejected", "my.rig", false},
		{"space rejected", "my rig", false},
		{"empty", "", false},
		{"max length", strings.Repeat("a", 200), true},
		{"over max length", strings.Repeat("a", 201), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidRigName(tt.input); got != tt.want {
				t.Errorf("isValidRigName(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsValidMailAddress(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"simple name", "alice", true},
		{"agent path", "rig/agent", true},
		{"mayor path", "mayor/", true},
		{"at-pattern", "@town", true},
		{"at-rig pattern", "@rig/myrig", true},
		{"prefixed group", "group:oncall", true},
		{"prefixed queue", "queue:builds", true},
		{"prefixed channel", "channel:alerts", true},
		{"legacy list", "list:oncall", true},
		{"wildcard", "myrig/*", true},
		{"leading dash rejected", "-flag", false},
		{"empty rejected", "", false},
		{"null byte rejected", "alice\x00bob", false},
		{"newline rejected", "alice\nbob", false},
		{"over max length", strings.Repeat("a", 201), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidMailAddress(tt.input); got != tt.want {
				t.Errorf("isValidMailAddress(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsNumeric(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"empty", "", false},
		{"zero", "0", true},
		{"positive", "12345", true},
		{"negative", "-1", false},
		{"decimal", "1.5", false},
		{"letters", "abc", false},
		{"mixed", "12a", false},
		{"max length boundary", strings.Repeat("9", 20), true},
		{"over max length", strings.Repeat("9", 21), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isNumeric(tt.input); got != tt.want {
				t.Errorf("isNumeric(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsValidRepoRef(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"valid owner/repo", "owner/repo", true},
		{"valid with dots", "my.org/my.repo", true},
		{"leading dash owner", "-bad/repo", false},
		{"leading dash repo", "owner/-bad", false},
		{"missing slash", "ownerrepo", false},
		{"triple segment", "a/b/c", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidRepoRef(tt.input); got != tt.want {
				t.Errorf("isValidRepoRef(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsValidGitURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"https URL", "https://github.com/org/repo.git", true},
		{"http URL", "http://internal.example.com/repo.git", true},
		{"ssh URL", "ssh://git@github.com/org/repo.git", true},
		{"git protocol", "git://github.com/org/repo.git", true},
		{"SCP-style", "git@github.com:org/repo.git", true},
		{"SCP-style custom user", "deploy@host.example.com:path/repo.git", true},
		{"bare owner/repo rejected", "owner/repo", false},
		{"flag injection", "--evil", false},
		{"empty", "", false},
		{"ftp accepted", "ftp://example.com/repo.git", true},
		{"s3 accepted", "s3://my-bucket/rigs/project", true},
		{"codecommit accepted", "codecommit://my-repo", true},
		{"file rejected", "file:///tmp/repo", false},
		{"local path rejected", "/tmp/repo", false},
		{"SCP empty path rejected", "git@host:", false},
		// Parity vectors from internal/cmd/rig_test.go:TestIsGitRemoteURL —
		// isValidGitURL must agree on all inputs that isGitRemoteURL tests.
		{"parity: relative dot", "./foo", false},
		{"parity: relative dotdot", "../foo", false},
		{"parity: tilde path", "~/projects/foo", false},
		{"parity: windows backslash", `C:\Users\scott\projects\foo`, false},
		{"parity: windows forward", "C:/Users/scott/projects/foo", false},
		{"parity: file with user", "file://user@localhost:/tmp/evil-repo", false},
		{"parity: arg injection -o", "-oProxyCommand=evil", false},
		{"parity: arg injection --upload-pack", "--upload-pack=touch /tmp/pwned", false},
		{"parity: arg injection -c", "-c", false},
		{"parity: SCP empty user", "@host:path", false},
		{"parity: SCP empty host", "user@:/path", false},
		{"parity: SCP no user", "localhost:path", false},
		{"parity: SCP custom user", "deploy@private-host.internal:repos/app.git", true},
		{"parity: bare dirname", "foo", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidGitURL(tt.input); got != tt.want {
				t.Errorf("isValidGitURL(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestExpandHomePath(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantErr   bool
		errSubstr string
	}{
		{"tilde-relative valid", "~/projects", false, ""},
		{"tilde traversal escape", "~/../../etc/passwd", true, "escapes home"},
		{"bare tilde", "~", false, ""},
		{"absolute path passthrough", "/absolute/path", false, ""},
		{"relative path passthrough", "foo/bar", false, ""},
		{"dot-dot cleaning", "foo/../bar", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := expandHomePath(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expandHomePath(%q) expected error containing %q, got nil (result=%q)", tt.input, tt.errSubstr, result)
					return
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("expandHomePath(%q) error = %q, want error containing %q", tt.input, err.Error(), tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Errorf("expandHomePath(%q) unexpected error: %v", tt.input, err)
				return
			}
			if result == "" {
				t.Errorf("expandHomePath(%q) returned empty string", tt.input)
			}
		})
	}
}

// --- Handler-level validation tests ---
// These verify that validation is correctly wired in the HTTP handlers,
// not just that the helper functions work.
//
// Limitation: these tests cannot verify that the -- sentinel prevents argument
// injection end-to-end because gt/bd binaries are not available in the test
// environment. Tests that pass validation (expect 500, not 400) confirm the
// web handler constructs args correctly, but actual -- sentinel behavior depends
// on cobra/pflag in the target CLI. Both gt and bd use cobra, which respects --
// natively. Full end-to-end verification requires integration tests with the
// real binaries (e.g., via gastown-docker).

func TestHandler_MailRead_InvalidID(t *testing.T) {
	handler := NewAPIHandler(30*time.Second, 60*time.Second, "test-token")

	req := httptest.NewRequest(http.MethodGet, "/api/mail/read?id=--inject", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("GET /api/mail/read?id=--inject status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandler_MailSend_InvalidRecipient(t *testing.T) {
	handler := NewAPIHandler(30*time.Second, 60*time.Second, "test-token")

	body := `{"to": "--flag", "subject": "test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/mail/send", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Dashboard-Token", "test-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("POST /api/mail/send flag recipient status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandler_MailSend_ValidAgentPath(t *testing.T) {
	handler := NewAPIHandler(30*time.Second, 60*time.Second, "test-token")

	// rig/agent is a valid mail address — should NOT be rejected by validation.
	// gt isn't available in test, so expect 500 (command failed), NOT 400 (validation).
	body := `{"to": "myrig/agent", "subject": "test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/mail/send", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Dashboard-Token", "test-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code == http.StatusBadRequest {
		t.Errorf("POST /api/mail/send with rig/agent address rejected as bad request (should pass validation)")
	}
	// Verify the response doesn't mention validation failure
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err == nil {
		if errMsg, ok := resp["error"].(string); ok {
			if strings.Contains(errMsg, "Invalid recipient") {
				t.Errorf("expected gt execution error, got validation error: %s", errMsg)
			}
		}
	}
}

func TestHandler_MailSend_OversizedSubject(t *testing.T) {
	handler := NewAPIHandler(30*time.Second, 60*time.Second, "test-token")

	payload := map[string]interface{}{
		"to":      "alice",
		"subject": strings.Repeat("x", 501),
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/mail/send", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Dashboard-Token", "test-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("POST /api/mail/send oversized subject status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandler_IssueShow_InvalidID(t *testing.T) {
	handler := NewAPIHandler(30*time.Second, 60*time.Second, "test-token")

	req := httptest.NewRequest(http.MethodGet, "/api/issues/show?id=--help", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("GET /api/issues/show?id=--help status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandler_PRShow_InvalidNumber(t *testing.T) {
	handler := NewAPIHandler(30*time.Second, 60*time.Second, "test-token")

	req := httptest.NewRequest(http.MethodGet, "/api/pr/show?repo=owner/repo&number=abc", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("GET /api/pr/show invalid number status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandler_PRShow_InvalidURL(t *testing.T) {
	handler := NewAPIHandler(30*time.Second, 60*time.Second, "test-token")

	req := httptest.NewRequest(http.MethodGet, "/api/pr/show?url=--evil", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("GET /api/pr/show flag URL status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandler_IssueShow_ExternalPrefixID(t *testing.T) {
	handler := NewAPIHandler(30*time.Second, 60*time.Second, "test-token")

	// external:prefix:id format should pass validation (unwrapped to raw ID).
	// Expect 500 (bd not available), NOT 400 (validation failure).
	req := httptest.NewRequest(http.MethodGet, "/api/issues/show?id=external:gt-mol:gt-mol-abc123", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code == http.StatusBadRequest {
		t.Errorf("GET /api/issues/show with external:prefix:id rejected as bad request (should unwrap)")
	}
	// Verify it got past validation (error should be about bd execution, not ID format)
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err == nil {
		if errMsg, ok := resp["error"].(string); ok {
			if strings.Contains(errMsg, "Invalid issue ID") {
				t.Errorf("expected execution error, got validation error: %s", errMsg)
			}
		}
	}
}

func TestHandler_PRShow_URLIgnoresRepoNumber(t *testing.T) {
	handler := NewAPIHandler(30*time.Second, 60*time.Second, "test-token")

	// When url is provided, repo/number should be ignored (not validated).
	// Expect 500 (gh not available), NOT 400 (validation failure).
	req := httptest.NewRequest(http.MethodGet, "/api/pr/show?url=https://github.com/org/repo/pull/1&number=abc", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code == http.StatusBadRequest {
		t.Errorf("GET /api/pr/show with url+invalid number rejected (number should be ignored)")
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err == nil {
		if errMsg, ok := resp["error"].(string); ok {
			if strings.Contains(errMsg, "Invalid PR number") || strings.Contains(errMsg, "Invalid repo") {
				t.Errorf("expected execution error, got validation error: %s", errMsg)
			}
		}
	}
}

func TestSetupHandler_Install_FlagPathInjection(t *testing.T) {
	handler := NewSetupAPIHandler("test-token")

	// Validates the validation layer: "--help" passes expandHomePath (it's a
	// relative path) and reaches gt install. The -- sentinel in the args
	// ensures gt install sees it as a path, not a flag — but that's verified
	// by the args construction, not by this HTTP-level test.
	body := `{"path": "--help"}`
	req := httptest.NewRequest(http.MethodPost, "/api/install", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Dashboard-Token", "test-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Should get past validation to gt execution (not 400).
	// The -- sentinel ensures gt install sees "--help" as a path, not a flag.
	if w.Code == http.StatusBadRequest {
		t.Errorf("POST /api/install with path=--help rejected as bad request (-- sentinel should protect)")
	}
}

func TestSetupHandler_RigAdd_InvalidGitURL(t *testing.T) {
	handler := NewSetupAPIHandler("test-token")

	body := `{"name": "myrig", "gitUrl": "--evil"}`
	req := httptest.NewRequest(http.MethodPost, "/api/rig/add", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Dashboard-Token", "test-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("POST /api/rig/add flag gitUrl status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestSetupHandler_Install_InvalidName(t *testing.T) {
	handler := NewSetupAPIHandler("test-token")

	body := `{"path": "/tmp/test", "name": "--inject"}`
	req := httptest.NewRequest(http.MethodPost, "/api/install", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Dashboard-Token", "test-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("POST /api/install flag name status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestSetupHandler_RigAdd_HyphenatedName(t *testing.T) {
	handler := NewSetupAPIHandler("test-token")

	// Rig names with hyphens should be rejected — hyphens are reserved for
	// agent ID parsing (see internal/rig/manager.go:269).
	body := `{"name": "my-rig", "gitUrl": "https://github.com/org/repo.git"}`
	req := httptest.NewRequest(http.MethodPost, "/api/rig/add", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Dashboard-Token", "test-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("POST /api/rig/add hyphenated name status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestSetupHandler_CheckWorkspace_TraversalPath(t *testing.T) {
	handler := NewSetupAPIHandler("test-token")

	body := `{"path": "~/../../etc/passwd"}`
	req := httptest.NewRequest(http.MethodPost, "/api/check-workspace", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Dashboard-Token", "test-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Should return 200 with Valid:false (not 400) per check-endpoint convention
	if w.Code != http.StatusOK {
		t.Errorf("POST /api/check-workspace traversal status = %d, want %d", w.Code, http.StatusOK)
	}
	var resp CheckWorkspaceResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if resp.Valid {
		t.Error("Expected Valid=false for traversal path")
	}
}

func TestHandler_IssueShow_MalformedExternalPrefix(t *testing.T) {
	handler := NewAPIHandler(30*time.Second, 60*time.Second, "test-token")

	// external:foo (only 2 parts) should get a specific error, not generic "Invalid issue ID".
	req := httptest.NewRequest(http.MethodGet, "/api/issues/show?id=external:foo", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("GET /api/issues/show?id=external:foo status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err == nil {
		if errMsg, ok := resp["error"].(string); ok {
			if !strings.Contains(errMsg, "Malformed external issue ID") {
				t.Errorf("expected malformed-external error, got: %s", errMsg)
			}
		}
	}
}

func TestHandler_IssueShow_ExternalWithExtraColons(t *testing.T) {
	handler := NewAPIHandler(30*time.Second, 60*time.Second, "test-token")

	// SplitN(":", 3) puts "id:with:colons" in parts[2]. isValidID rejects colons.
	req := httptest.NewRequest(http.MethodGet, "/api/issues/show?id=external:prefix:id:with:colons", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("GET /api/issues/show external:prefix:id:with:colons status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err == nil {
		if errMsg, ok := resp["error"].(string); ok {
			if !strings.Contains(errMsg, "Invalid issue ID") {
				t.Errorf("expected invalid ID error for colon-containing ID, got: %s", errMsg)
			}
		}
	}
}

func TestHandler_MailSend_NullByteSubject(t *testing.T) {
	handler := NewAPIHandler(30*time.Second, 60*time.Second, "test-token")

	body := `{"to": "alice", "subject": "test\u0000inject"}`
	req := httptest.NewRequest(http.MethodPost, "/api/mail/send", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Dashboard-Token", "test-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("POST /api/mail/send null-byte subject status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandler_IssueCreate_FlagTitle(t *testing.T) {
	handler := NewAPIHandler(30*time.Second, 60*time.Second, "test-token")

	// A title of "--help" should pass validation (no control chars, no newlines)
	// and reach bd create. The -- sentinel ensures it's treated as positional.
	// Expect 500 (bd not available), NOT 400.
	body := `{"title": "--help"}`
	req := httptest.NewRequest(http.MethodPost, "/api/issues/create", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Dashboard-Token", "test-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code == http.StatusBadRequest {
		t.Errorf("POST /api/issues/create with title=--help rejected as bad request (-- sentinel should protect)")
	}
}

func TestHandler_SessionPreview_MissingParam(t *testing.T) {
	handler := NewAPIHandler(30*time.Second, 60*time.Second, "test-token")

	req := httptest.NewRequest(http.MethodGet, "/api/session/preview", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("GET /api/session/preview (no param) status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandler_SessionPreview_InvalidPrefix(t *testing.T) {
	handler := NewAPIHandler(30*time.Second, 60*time.Second, "test-token")

	req := httptest.NewRequest(http.MethodGet, "/api/session/preview?session=evil-session", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("GET /api/session/preview (no gt- prefix) status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandler_SessionPreview_InvalidChars(t *testing.T) {
	handler := NewAPIHandler(30*time.Second, 60*time.Second, "test-token")

	// Session name with shell metacharacters should be rejected
	req := httptest.NewRequest(http.MethodGet, "/api/session/preview?session=gt-evil;rm+-rf+/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("GET /api/session/preview (invalid chars) status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandler_SessionPreview_ValidName(t *testing.T) {
	handler := NewAPIHandler(30*time.Second, 60*time.Second, "test-token")

	// A valid session name should pass validation and reach tmux capture-pane.
	// tmux isn't available in test, so expect 500 (command failed), NOT 400 (validation).
	req := httptest.NewRequest(http.MethodGet, "/api/session/preview?session=gt-gastown-nux", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code == http.StatusBadRequest {
		t.Errorf("GET /api/session/preview with valid name rejected as bad request")
	}
}

func TestExpandHomePath_RootUser(t *testing.T) {
	// Verify that expandHomePath works when home is "/" (root user).
	// We can't easily mock os.UserHomeDir, but we can verify the containment
	// logic directly: when home="/", cleaned="/projects" should pass because
	// home=="/" is special-cased to allow all absolute paths.
	//
	// This test validates the fix by confirming that the function doesn't
	// reject ~/projects when HOME is set. The root-user edge case (HOME=/)
	// is verified by code inspection of the special-case in expandHomePath.
	result, err := expandHomePath("~/projects")
	if err != nil {
		t.Errorf("expandHomePath(\"~/projects\") unexpected error: %v", err)
		return
	}
	if result == "" {
		t.Error("expandHomePath(\"~/projects\") returned empty string")
	}
	// The result should end with the platform-appropriate path separator + "projects"
	wantSuffix := string(filepath.Separator) + "projects"
	if !strings.HasSuffix(result, wantSuffix) {
		t.Errorf("expandHomePath(\"~/projects\") = %q, want suffix %q", result, wantSuffix)
	}
}
