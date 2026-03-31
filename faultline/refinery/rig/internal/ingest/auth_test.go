package ingest

import (
	"net/http"
	"testing"
)

func TestNewProjectAuth(t *testing.T) {
	// Two-part format (no rig).
	auth, err := NewProjectAuth([]string{"1:abc123", "2:def456"})
	if err != nil {
		t.Fatal(err)
	}
	if rig := auth.RigForProject(1); rig != "" {
		t.Errorf("expected empty rig, got %q", rig)
	}

	// Three-part format (with rig).
	auth, err = NewProjectAuth([]string{"1:abc123:faultline", "2:def456:myapp"})
	if err != nil {
		t.Fatal(err)
	}
	if rig := auth.RigForProject(1); rig != "faultline" {
		t.Errorf("expected faultline, got %q", rig)
	}
	if rig := auth.RigForProject(2); rig != "myapp" {
		t.Errorf("expected myapp, got %q", rig)
	}

	// Mixed format.
	_, err = NewProjectAuth([]string{"1:abc123:faultline", "2:def456"})
	if err != nil {
		t.Fatal(err)
	}

	// Bad format.
	_, err = NewProjectAuth([]string{"bad"})
	if err == nil {
		t.Fatal("expected error for bad pair")
	}
}

func TestAuthenticate(t *testing.T) {
	auth, _ := NewProjectAuth([]string{"42:mykey"})

	tests := []struct {
		name    string
		header  string
		query   string
		wantPID int64
		wantErr bool
	}{
		{
			name:    "X-Sentry-Auth header",
			header:  "Sentry sentry_key=mykey, sentry_version=7",
			wantPID: 42,
		},
		{
			name:    "query param",
			query:   "sentry_key=mykey",
			wantPID: 42,
		},
		{
			name:    "missing key",
			wantErr: true,
		},
		{
			name:    "wrong key",
			header:  "Sentry sentry_key=wrong",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", "/api/42/envelope/?"+tt.query, nil)
			if tt.header != "" {
				req.Header.Set("X-Sentry-Auth", tt.header)
			}
			pid, err := auth.Authenticate(req)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pid != tt.wantPID {
				t.Fatalf("got pid %d, want %d", pid, tt.wantPID)
			}
		})
	}
}
