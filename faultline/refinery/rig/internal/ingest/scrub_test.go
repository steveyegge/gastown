package ingest

import (
	"encoding/json"
	"strings"
	"testing"
)

// --- scrubString unit tests ---

func TestScrubString_Email(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"contact user@example.com now", "contact [Filtered] now"},
		{"alice+tag@sub.domain.co.uk", "[Filtered]"},
		{"no email here", "no email here"},
	}
	for _, tt := range tests {
		got := scrubString(tt.input)
		if got != tt.want {
			t.Errorf("scrubString(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestScrubString_CreditCard(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"visa", "card 4111111111111111 end", "card [Filtered] end"},
		{"visa spaces", "4111 1111 1111 1111", "[Filtered]"},
		{"mastercard", "5500000000000004", "[Filtered]"},
		{"amex", "378282246310005", "[Filtered]"},
		{"too short", "4111 1111", "4111 1111"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scrubString(tt.input)
			if got != tt.want {
				t.Errorf("scrubString(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestScrubString_SSN(t *testing.T) {
	got := scrubString("ssn is 123-45-6789")
	want := "ssn is [Filtered]"
	if got != want {
		t.Errorf("scrubString SSN = %q, want %q", got, want)
	}
}

func TestScrubString_BearerToken(t *testing.T) {
	got := scrubString("Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.test")
	if !strings.Contains(got, "Bearer [Filtered]") {
		t.Errorf("bearer token not scrubbed: %q", got)
	}
	if strings.Contains(got, "eyJ") {
		t.Errorf("token value leaked: %q", got)
	}
}

func TestScrubString_APIKey(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"api_key", `api_key: "sk_live_abcdef1234567890"`},
		{"secret_key", `secret_key=mysecretkeythatislong`},
		{"access_token", `access_token: "ghp_xxxxxxxxxxxxxxxxxxxx"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scrubString(tt.input)
			if got == tt.input {
				t.Errorf("api key not scrubbed: %q", got)
			}
		})
	}
}

func TestScrubString_Password(t *testing.T) {
	got := scrubString(`password: "hunter2"`)
	if got == `password: "hunter2"` {
		t.Errorf("password not scrubbed: %q", got)
	}
}

func TestScrubString_NoFalsePositive(t *testing.T) {
	safe := []string{
		"normal log message",
		"error code 404",
		"duration: 1.234s",
		"count=42",
	}
	for _, s := range safe {
		got := scrubString(s)
		if got != s {
			t.Errorf("false positive: scrubString(%q) = %q", s, got)
		}
	}
}

// --- ScrubEvent tests ---

func TestScrubEvent_SensitiveKeys(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"password", "password"},
		{"passwd", "passwd"},
		{"secret", "secret"},
		{"authorization", "authorization"},
		{"cookie", "cookie"},
		{"set-cookie", "set-cookie"},
		{"x-api-key", "x-api-key"},
		{"token", "token"},
		{"access_token", "access_token"},
		{"refresh_token", "refresh_token"},
		{"api_key", "api_key"},
		{"private_key", "private_key"},
		{"credit_card", "credit_card"},
		{"card_number", "card_number"},
		{"cvv", "cvv"},
		{"ssn", "ssn"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := map[string]interface{}{tt.key: "super-secret-value"}
			raw, _ := json.Marshal(input)
			out := ScrubEvent(raw)

			var result map[string]interface{}
			if err := json.Unmarshal(out, &result); err != nil {
				t.Fatalf("unmarshal scrubbed output: %v", err)
			}
			if result[tt.key] != redacted {
				t.Fatalf("key %q not redacted: got %q", tt.key, result[tt.key])
			}
		})
	}
}

func TestScrubEvent_SensitiveKeyCaseInsensitive(t *testing.T) {
	input := map[string]interface{}{"Password": "s3cret", "TOKEN": "abc123"}
	raw, _ := json.Marshal(input)
	out := ScrubEvent(raw)

	var result map[string]interface{}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result["Password"] != redacted {
		t.Fatalf("Password not redacted: got %q", result["Password"])
	}
	if result["TOKEN"] != redacted {
		t.Fatalf("TOKEN not redacted: got %q", result["TOKEN"])
	}
}

func TestScrubEvent_EmailRedaction(t *testing.T) {
	tests := []struct {
		name  string
		email string
	}{
		{"simple", "user@example.com"},
		{"plus addressing", "user+tag@example.com"},
		{"subdomain", "admin@mail.corp.example.com"},
		{"dots", "first.last@example.org"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := map[string]interface{}{"message": "Contact " + tt.email + " for help"}
			raw, _ := json.Marshal(input)
			out := ScrubEvent(raw)

			if strings.Contains(string(out), tt.email) {
				t.Fatalf("email %q not redacted in output: %s", tt.email, out)
			}
			if !strings.Contains(string(out), redacted) {
				t.Fatalf("redaction marker missing from output: %s", out)
			}
		})
	}
}

func TestScrubEvent_CreditCardRedaction(t *testing.T) {
	tests := []struct {
		name string
		cc   string
	}{
		{"visa no sep", "4111111111111111"},
		{"visa dashes", "4111-1111-1111-1111"},
		{"visa spaces", "4111 1111 1111 1111"},
		{"mastercard", "5500000000000004"},
		{"amex", "378282246310005"},
		{"discover", "6011111111111117"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := map[string]interface{}{"data": "CC: " + tt.cc}
			raw, _ := json.Marshal(input)
			out := ScrubEvent(raw)

			if strings.Contains(string(out), tt.cc) {
				t.Fatalf("credit card %q not redacted: %s", tt.cc, out)
			}
		})
	}
}

func TestScrubEvent_SSNRedaction(t *testing.T) {
	input := map[string]interface{}{"info": "SSN is 123-45-6789"}
	raw, _ := json.Marshal(input)
	out := ScrubEvent(raw)

	if strings.Contains(string(out), "123-45-6789") {
		t.Fatalf("SSN not redacted: %s", out)
	}
	if !strings.Contains(string(out), redacted) {
		t.Fatalf("redaction marker missing: %s", out)
	}
}

func TestScrubEvent_BearerTokenRedaction(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"lowercase", "bearer eyJhbGciOiJIUzI1NiJ9.test.sig"},
		{"uppercase", "Bearer eyJhbGciOiJIUzI1NiJ9.test.sig"},
		{"mixed case", "BEARER abc123def456"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := map[string]interface{}{"header": tt.input}
			raw, _ := json.Marshal(payload)
			out := ScrubEvent(raw)

			outStr := string(out)
			if !strings.Contains(outStr, "Bearer "+redacted) && !strings.Contains(outStr, "BEARER "+redacted) {
				// The regex is case-insensitive, replacement is "Bearer [Filtered]"
				if strings.Contains(outStr, "eyJ") || strings.Contains(outStr, "abc123") {
					t.Fatalf("bearer token not redacted: %s", out)
				}
			}
		})
	}
}

func TestScrubEvent_APIKeyRedaction(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"api_key equals", `api_key=sk_live_abcdef1234567890`},
		{"apikey colon", `apikey: "abcdef1234567890xx"`},
		{"secret_key", `secret_key="mysecretkey12345678"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := map[string]interface{}{"log": tt.input}
			raw, _ := json.Marshal(payload)
			out := ScrubEvent(raw)

			if strings.Contains(string(out), "abcdef1234567890") || strings.Contains(string(out), "mysecretkey") {
				t.Fatalf("API key not redacted: %s", out)
			}
		})
	}
}

func TestScrubEvent_PasswordPatternRedaction(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"password equals", `password=hunter2`},
		{"passwd colon", `passwd: "correcthorsebatterystaple"`},
		{"pwd quoted", `pwd="s3cur3!"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := map[string]interface{}{"log": tt.input}
			raw, _ := json.Marshal(payload)
			out := ScrubEvent(raw)

			if strings.Contains(string(out), "hunter2") || strings.Contains(string(out), "correcthorse") || strings.Contains(string(out), "s3cur3") {
				t.Fatalf("password not redacted: %s", out)
			}
		})
	}
}

func TestScrubEvent_NestedObjects(t *testing.T) {
	input := map[string]interface{}{
		"user": map[string]interface{}{
			"name":     "Alice",
			"password": "secret123",
			"email":    "alice@example.com",
		},
		"request": map[string]interface{}{
			"headers": map[string]interface{}{
				"authorization": "Bearer tok_abc123",
				"content-type":  "application/json",
			},
		},
	}
	raw, _ := json.Marshal(input)
	out := ScrubEvent(raw)

	var result map[string]interface{}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	user, ok := result["user"].(map[string]interface{})
	if !ok {
		t.Fatal("user field is not an object")
	}
	if user["password"] != redacted {
		t.Fatalf("nested password not redacted: %v", user["password"])
	}
	if user["name"] != "Alice" {
		t.Fatalf("non-sensitive field modified: %v", user["name"])
	}

	reqObj, ok := result["request"].(map[string]interface{})
	if !ok {
		t.Fatal("request field is not an object")
	}
	headers, ok := reqObj["headers"].(map[string]interface{})
	if !ok {
		t.Fatal("headers field is not an object")
	}
	if headers["authorization"] != redacted {
		t.Fatalf("nested authorization not redacted: %v", headers["authorization"])
	}
	if headers["content-type"] != "application/json" {
		t.Fatalf("non-sensitive header modified: %v", headers["content-type"])
	}
}

func TestScrubEvent_Arrays(t *testing.T) {
	input := map[string]interface{}{
		"breadcrumbs": []interface{}{
			map[string]interface{}{"message": "logged in as user@example.com"},
			map[string]interface{}{"message": "fetched data", "data": map[string]interface{}{"token": "abc"}},
		},
	}
	raw, _ := json.Marshal(input)
	out := ScrubEvent(raw)

	if strings.Contains(string(out), "user@example.com") {
		t.Fatalf("email in array not redacted: %s", out)
	}

	var result map[string]interface{}
	_ = json.Unmarshal(out, &result)
	crumbs, _ := result["breadcrumbs"].([]interface{})
	entryMap, _ := crumbs[1].(map[string]interface{})
	entry, _ := entryMap["data"].(map[string]interface{})
	if entry["token"] != redacted {
		t.Fatalf("token in array element not redacted: %v", entry["token"])
	}
}

func TestScrubEvent_NonSensitivePreserved(t *testing.T) {
	input := map[string]interface{}{
		"level":       "error",
		"logger":      "sentry.go",
		"message":     "something broke",
		"environment": "production",
		"tags": map[string]interface{}{
			"version": "1.2.3",
		},
	}
	raw, _ := json.Marshal(input)
	out := ScrubEvent(raw)

	var result map[string]interface{}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result["level"] != "error" {
		t.Fatalf("level changed: %v", result["level"])
	}
	if result["message"] != "something broke" {
		t.Fatalf("message changed: %v", result["message"])
	}
	tags, _ := result["tags"].(map[string]interface{})
	if tags["version"] != "1.2.3" {
		t.Fatalf("version tag changed: %v", tags["version"])
	}
}

func TestScrubEvent_InvalidJSON(t *testing.T) {
	raw := json.RawMessage(`not json but has user@example.com in it`)
	out := ScrubEvent(raw)

	if strings.Contains(string(out), "user@example.com") {
		t.Fatalf("email in invalid JSON not redacted: %s", out)
	}
	if !strings.Contains(string(out), redacted) {
		t.Fatalf("redaction marker missing from invalid JSON output: %s", out)
	}
}

func TestScrubEvent_PreservesStructure(t *testing.T) {
	input := json.RawMessage(`{
		"exception": {
			"values": [{"type": "TypeError", "value": "null is not an object"}]
		},
		"level": "error",
		"platform": "javascript",
		"timestamp": 1704067200
	}`)

	out := ScrubEvent(input)

	var result map[string]interface{}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("scrubbed output is not valid JSON: %v", err)
	}

	if result["level"] != "error" {
		t.Errorf("level changed: %v", result["level"])
	}
	if result["platform"] != "javascript" {
		t.Errorf("platform changed: %v", result["platform"])
	}

	exc, ok := result["exception"].(map[string]interface{})
	if !ok {
		t.Fatal("exception structure lost")
	}
	vals, ok := exc["values"].([]interface{})
	if !ok || len(vals) == 0 {
		t.Fatal("exception.values structure lost")
	}
}

func TestScrubEvent_EmptyObject(t *testing.T) {
	raw := json.RawMessage(`{}`)
	out := ScrubEvent(raw)
	if string(out) != "{}" {
		t.Fatalf("empty object changed: %s", out)
	}
}

func TestScrubEvent_NumericAndBoolPreserved(t *testing.T) {
	input := map[string]interface{}{
		"count":   float64(42),
		"enabled": true,
		"rate":    float64(3.14),
	}
	raw, _ := json.Marshal(input)
	out := ScrubEvent(raw)

	var result map[string]interface{}
	_ = json.Unmarshal(out, &result)
	if result["count"] != float64(42) {
		t.Fatalf("count changed: %v", result["count"])
	}
	if result["enabled"] != true {
		t.Fatalf("enabled changed: %v", result["enabled"])
	}
}

func TestScrubEvent_MultiplePIIInSingleString(t *testing.T) {
	input := map[string]interface{}{
		"log": "User user@example.com has SSN 123-45-6789 and CC 4111111111111111",
	}
	raw, _ := json.Marshal(input)
	out := ScrubEvent(raw)
	outStr := string(out)

	if strings.Contains(outStr, "user@example.com") {
		t.Fatalf("email not redacted in multi-PII string: %s", out)
	}
	if strings.Contains(outStr, "123-45-6789") {
		t.Fatalf("SSN not redacted in multi-PII string: %s", out)
	}
	if strings.Contains(outStr, "4111111111111111") {
		t.Fatalf("CC not redacted in multi-PII string: %s", out)
	}
}
