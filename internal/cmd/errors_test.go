package cmd

import (
	"errors"
	"fmt"
	"testing"
)

func TestSilentExitError_Error(t *testing.T) {
	tests := []struct {
		name string
		code int
		want string
	}{
		{"zero code", 0, "exit 0"},
		{"success code", 1, "exit 1"},
		{"error code", 2, "exit 2"},
		{"custom code", 42, "exit 42"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &SilentExitError{Code: tt.code}
			got := e.Error()
			if got != tt.want {
				t.Errorf("SilentExitError.Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewSilentExit(t *testing.T) {
	tests := []struct {
		code int
	}{
		{0},
		{1},
		{2},
		{127},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("code_%d", tt.code), func(t *testing.T) {
			err := NewSilentExit(tt.code)
			if err == nil {
				t.Fatal("NewSilentExit should return non-nil")
			}
			if err.Code != tt.code {
				t.Errorf("NewSilentExit(%d).Code = %d, want %d", tt.code, err.Code, tt.code)
			}
		})
	}
}

func TestIsSilentExit(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		wantCode     int
		wantIsSilent bool
	}{
		{"nil error", nil, 0, false},
		{"silent exit code 0", NewSilentExit(0), 0, true},
		{"silent exit code 1", NewSilentExit(1), 1, true},
		{"silent exit code 2", NewSilentExit(2), 2, true},
		{"other error", errors.New("some error"), 0, false},
		{"wrapped silent exit", fmt.Errorf("wrapped: %w", NewSilentExit(5)), 5, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, isSilent := IsSilentExit(tt.err)
			if isSilent != tt.wantIsSilent {
				t.Errorf("IsSilentExit(%v) isSilent = %v, want %v", tt.err, isSilent, tt.wantIsSilent)
			}
			if code != tt.wantCode {
				t.Errorf("IsSilentExit(%v) code = %d, want %d", tt.err, code, tt.wantCode)
			}
		})
	}
}

func TestSilentExitError_Is(t *testing.T) {
	err := NewSilentExit(1)
	var target *SilentExitError
	if !errors.As(err, &target) {
		t.Error("errors.As should find SilentExitError")
	}
	if target.Code != 1 {
		t.Errorf("errors.As extracted code = %d, want 1", target.Code)
	}
}
