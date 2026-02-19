package observe

import (
	"strings"
	"testing"
)

func TestRedactor_None(t *testing.T) {
	r := NewRedactor(RedactNone)
	input := "user@example.com connected from 192.168.1.1 with Bearer abc123token"
	got := r.Redact(input)
	if got != input {
		t.Errorf("none policy should not modify input\ngot:  %s\nwant: %s", got, input)
	}
}

func TestRedactor_UnknownPolicyTreatedAsNone(t *testing.T) {
	r := NewRedactor("foobar")
	input := "user@example.com 10.0.0.1"
	got := r.Redact(input)
	if got != input {
		t.Errorf("unknown policy should not modify input\ngot:  %s\nwant: %s", got, input)
	}
}

func TestRedactor_Standard(t *testing.T) {
	r := NewRedactor(RedactStandard)

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "email",
			input: "contact user@example.com for help",
			want:  "contact [REDACTED_EMAIL] for help",
		},
		{
			name:  "ipv4",
			input: "connection from 192.168.1.100 accepted",
			want:  "connection from [REDACTED_IP] accepted",
		},
		{
			name:  "bearer_token",
			input: "Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.payload.sig",
			want:  "Authorization: Bearer [REDACTED_TOKEN]",
		},
		{
			name:  "api_key",
			input: "api_key=sk_live_abcdefghijklmnop",
			want:  "[REDACTED_API_KEY]",
		},
		{
			name:  "secret_key",
			input: "secret_key=abcdefghijklmnopqr",
			want:  "[REDACTED_API_KEY]",
		},
		{
			name:  "uuid_not_redacted_in_standard",
			input: "request 550e8400-e29b-41d4-a716-446655440000 processed",
			want:  "request 550e8400-e29b-41d4-a716-446655440000 processed",
		},
		{
			name:  "no_sensitive_data",
			input: "server started on port 8080",
			want:  "server started on port 8080",
		},
		{
			name:  "multiple_emails",
			input: "from alice@a.com to bob@b.org",
			want:  "from [REDACTED_EMAIL] to [REDACTED_EMAIL]",
		},
		{
			name:  "multiple_ips",
			input: "src=10.0.0.1 dst=10.0.0.2",
			want:  "src=[REDACTED_IP] dst=[REDACTED_IP]",
		},
		{
			name:  "email_and_ip_same_line",
			input: "user@example.com logged in from 192.168.1.1",
			want:  "[REDACTED_EMAIL] logged in from [REDACTED_IP]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.Redact(tt.input)
			if got != tt.want {
				t.Errorf("got:  %s\nwant: %s", got, tt.want)
			}
		})
	}
}

func TestRedactor_Strict(t *testing.T) {
	r := NewRedactor(RedactStrict)

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "email",
			input: "user@example.com",
			want:  "[REDACTED_EMAIL]",
		},
		{
			name:  "ipv4",
			input: "10.0.0.1",
			want:  "[REDACTED_IP]",
		},
		{
			name:  "uuid",
			input: "id=550e8400-e29b-41d4-a716-446655440000",
			want:  "id=[REDACTED_UUID]",
		},
		{
			name:  "long_numeric",
			input: "account 1234567890123 charged",
			want:  "account [REDACTED_NUM] charged",
		},
		{
			name:  "short_numeric_unchanged",
			input: "port 8080 open",
			want:  "port 8080 open",
		},
		{
			name:  "uuid_case_insensitive",
			input: "id=AABBCCDD-1234-5678-9012-ABCDEFABCDEF",
			want:  "id=[REDACTED_UUID]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.Redact(tt.input)
			if got != tt.want {
				t.Errorf("got:  %s\nwant: %s", got, tt.want)
			}
		})
	}
}

func TestRedactor_Standard_VersionStringBehavior(t *testing.T) {
	r := NewRedactor(RedactStandard)

	// Version prefixed with 'v' — the \b boundary prevents matching because
	// 'v' is a word character adjacent to the digits.
	got := r.Redact("upgraded to v3.2.1.0")
	if strings.Contains(got, "[REDACTED_IP]") {
		t.Errorf("v-prefixed version should NOT match IP pattern, got: %s", got)
	}

	// Standalone quad-dotted version at word boundary DOES match (known trade-off).
	got = r.Redact("version 3.2.1.0 released")
	if !strings.Contains(got, "[REDACTED_IP]") {
		t.Errorf("standalone quad-dotted number should match IP pattern (known trade-off), got: %s", got)
	}
}

func TestRedactor_UnicodeContent(t *testing.T) {
	r := NewRedactor(RedactStandard)

	tests := []struct {
		name  string
		input string
	}{
		{"emoji", "🔥 server crashed user@test.com"},
		{"cjk", "ユーザー user@test.com がログイン"},
		{"mixed", "café error from 10.0.0.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.Redact(tt.input)
			if strings.Contains(got, "user@test.com") {
				t.Errorf("email not redacted in unicode content: %s", got)
			}
			if strings.Contains(got, "10.0.0.1") && strings.Contains(tt.input, "10.0.0.1") {
				t.Errorf("IP not redacted in unicode content: %s", got)
			}
		})
	}
}

func TestRedactor_EmptyString(t *testing.T) {
	r := NewRedactor(RedactStrict)
	got := r.Redact("")
	if got != "" {
		t.Errorf("empty string should remain empty, got: %q", got)
	}
}

func TestRedactor_LongLine(t *testing.T) {
	r := NewRedactor(RedactStandard)
	// A long line with an email buried inside.
	line := strings.Repeat("a", 10000) + " user@example.com " + strings.Repeat("b", 10000)
	got := r.Redact(line)
	if strings.Contains(got, "user@example.com") {
		t.Error("email not redacted in long line")
	}
}
