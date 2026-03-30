package cmd

import (
	"testing"
)

func TestScanForCredentials(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantHits int
		wantName string // partial match on first finding name
	}{
		{
			name:     "clean message",
			input:    "Hey witness, everything looks good. Meeting in 10 minutes.",
			wantHits: 0,
		},
		{
			name:     "Brazilian CPF unformatted",
			input:    "CPF: 08997148621 — acesso ao portal",
			wantHits: 1,
			wantName: "CPF",
		},
		{
			name:     "Brazilian CPF formatted",
			input:    "CPF: 089.971.486-21",
			wantHits: 1,
			wantName: "CPF",
		},
		{
			name:     "senha inline",
			input:    "senha: myS3cr3tPass!",
			wantHits: 1,
			wantName: "password",
		},
		{
			name:     "password equals",
			input:    "password=hunter2",
			wantHits: 1,
			wantName: "password",
		},
		{
			name:     "AWS access key",
			input:    "Key: AKIAIOSFODNN7EXAMPLE",
			wantHits: 1,
			wantName: "AWS access key",
		},
		{
			name:     "API token",
			input:    "api_token=sk-abc123longvalue456",
			wantHits: 1,
			wantName: "API token",
		},
		{
			name:     "private key",
			input:    "-----BEGIN RSA PRIVATE KEY-----\nMIIE...",
			wantHits: 1,
			wantName: "private key",
		},
		{
			name:     "multiple patterns",
			input:    "CPF: 08997148621\nsenha: ptX8XAAq4!",
			wantHits: 2,
		},
		{
			name:     "normal number not CPF",
			input:    "issue 12345 has 3 comments",
			wantHits: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := scanForCredentials(tt.input)
			if len(findings) != tt.wantHits {
				t.Errorf("scanForCredentials(%q): got %d findings, want %d; findings: %v",
					tt.input, len(findings), tt.wantHits, findings)
			}
			if tt.wantName != "" && len(findings) > 0 {
				found := false
				for _, f := range findings {
					if credScanContains(f.patternName, tt.wantName) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("scanForCredentials(%q): no finding with name containing %q; got %v",
						tt.input, tt.wantName, findings)
				}
			}
		})
	}
}

func TestWarnCredentials_noFindings(t *testing.T) {
	err := warnCredentials(nil, false)
	if err != nil {
		t.Errorf("warnCredentials(nil, false) = %v; want nil", err)
	}
}

func TestWarnCredentials_blocked(t *testing.T) {
	findings := []credentialScanResult{{patternName: "test", snippet: "test****"}}
	err := warnCredentials(findings, false)
	if err == nil {
		t.Error("warnCredentials with findings and allowCredentials=false: want error, got nil")
	}
}

func TestWarnCredentials_allowed(t *testing.T) {
	findings := []credentialScanResult{{patternName: "test", snippet: "test****"}}
	err := warnCredentials(findings, true)
	if err != nil {
		t.Errorf("warnCredentials with allowCredentials=true: want nil, got %v", err)
	}
}

func credScanContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
