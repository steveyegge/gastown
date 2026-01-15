package exitcode

import (
	"errors"
	"fmt"
	"testing"
)

func TestNew(t *testing.T) {
	err := New(ErrBeadNotFound, "bead not found")
	if err.Code != ErrBeadNotFound {
		t.Errorf("Code = %d, want %d", err.Code, ErrBeadNotFound)
	}
	if err.Message != "bead not found" {
		t.Errorf("Message = %q, want %q", err.Message, "bead not found")
	}
}

func TestWrap(t *testing.T) {
	cause := errors.New("underlying error")
	err := Wrap(ErrNetwork, "connection failed", cause)

	if err.Code != ErrNetwork {
		t.Errorf("Code = %d, want %d", err.Code, ErrNetwork)
	}
	if !errors.Is(err, cause) {
		t.Error("Wrap should preserve cause for errors.Is")
	}
}

func TestError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *Error
		want string
	}{
		{
			name: "without cause",
			err:  New(ErrBeadNotFound, "bead gt-abc not found"),
			want: "bead gt-abc not found",
		},
		{
			name: "with cause",
			err:  Wrap(ErrNetwork, "connection failed", errors.New("timeout")),
			want: "connection failed: timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil error", nil, Success},
		{"coded error", New(ErrBeadNotFound, "not found"), ErrBeadNotFound},
		{"wrapped coded", Wrap(ErrTimeout, "timed out", errors.New("ctx")), ErrTimeout},
		{"plain error", errors.New("plain"), ErrGeneral},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Code(tt.err); got != tt.want {
				t.Errorf("Code() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestIs(t *testing.T) {
	err := New(ErrHookOccupied, "hook busy")

	if !Is(err, ErrHookOccupied) {
		t.Error("Is should return true for matching code")
	}
	if Is(err, ErrBeadNotFound) {
		t.Error("Is should return false for non-matching code")
	}
}

func TestNewf(t *testing.T) {
	err := Newf(ErrBeadNotFound, "bead %s not found in %s", "gt-abc", "queue")
	if err.Code != ErrBeadNotFound {
		t.Errorf("Code = %d, want %d", err.Code, ErrBeadNotFound)
	}
	want := "bead gt-abc not found in queue"
	if err.Message != want {
		t.Errorf("Message = %q, want %q", err.Message, want)
	}
}

func TestConvenienceConstructors(t *testing.T) {
	tests := []struct {
		name     string
		err      *Error
		wantCode int
		wantMsg  string
	}{
		{
			name:     "BeadNotFound",
			err:      BeadNotFound("gt-abc"),
			wantCode: ErrBeadNotFound,
			wantMsg:  "bead not found: gt-abc",
		},
		{
			name:     "AgentNotFound",
			err:      AgentNotFound("worker-1"),
			wantCode: ErrAgentNotFound,
			wantMsg:  "agent not found: worker-1",
		},
		{
			name:     "RigNotFound",
			err:      RigNotFound("dev-rig"),
			wantCode: ErrRigNotFound,
			wantMsg:  "rig not found: dev-rig",
		},
		{
			name:     "FileNotFound",
			err:      FileNotFound("/path/to/file"),
			wantCode: ErrFileNotFound,
			wantMsg:  "file not found: /path/to/file",
		},
		{
			name:     "PermissionDenied",
			err:      PermissionDenied("cannot write to config"),
			wantCode: ErrPermission,
			wantMsg:  "cannot write to config",
		},
		{
			name:     "HookOccupied",
			err:      HookOccupied("post-commit"),
			wantCode: ErrHookOccupied,
			wantMsg:  "hook already occupied: post-commit",
		},
		{
			name:     "Timeout",
			err:      Timeout("db query"),
			wantCode: ErrTimeout,
			wantMsg:  "operation timed out: db query",
		},
		{
			name:     "AlreadyExists",
			err:      AlreadyExists("bead gt-xyz"),
			wantCode: ErrAlreadyExists,
			wantMsg:  "bead gt-xyz already exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Code != tt.wantCode {
				t.Errorf("Code = %d, want %d", tt.err.Code, tt.wantCode)
			}
			if tt.err.Message != tt.wantMsg {
				t.Errorf("Message = %q, want %q", tt.err.Message, tt.wantMsg)
			}
		})
	}
}

func TestCodeWithWrappedErrors(t *testing.T) {
	// Test that Code() extracts codes from wrapped errors (via fmt.Errorf %w)
	original := BeadNotFound("gt-abc")
	wrapped := fmt.Errorf("failed to process: %w", original)
	doubleWrapped := fmt.Errorf("operation failed: %w", wrapped)

	tests := []struct {
		name string
		err  error
		want int
	}{
		{"original", original, ErrBeadNotFound},
		{"single wrapped", wrapped, ErrBeadNotFound},
		{"double wrapped", doubleWrapped, ErrBeadNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Code(tt.err); got != tt.want {
				t.Errorf("Code() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestIsWithWrappedErrors(t *testing.T) {
	original := HookOccupied("pre-commit")
	wrapped := fmt.Errorf("cannot run: %w", original)

	if !Is(wrapped, ErrHookOccupied) {
		t.Error("Is should work with wrapped errors")
	}
	if Is(wrapped, ErrBeadNotFound) {
		t.Error("Is should return false for non-matching wrapped errors")
	}
}

func TestErrorUnwrap(t *testing.T) {
	cause := errors.New("connection refused")
	err := Wrap(ErrNetwork, "API call failed", cause)

	// Test Unwrap
	if err.Unwrap() != cause {
		t.Error("Unwrap should return the cause")
	}

	// Test errors.Unwrap
	if errors.Unwrap(err) != cause {
		t.Error("errors.Unwrap should work with Error")
	}

	// Test error without cause
	errNoCause := New(ErrBeadNotFound, "not found")
	if errNoCause.Unwrap() != nil {
		t.Error("Unwrap should return nil when no cause")
	}
}

func TestConvenienceConstructorsWithCode(t *testing.T) {
	// Verify convenience constructors work correctly with Code() extraction
	constructors := []struct {
		name string
		err  error
		want int
	}{
		{"BeadNotFound", BeadNotFound("x"), ErrBeadNotFound},
		{"AgentNotFound", AgentNotFound("x"), ErrAgentNotFound},
		{"RigNotFound", RigNotFound("x"), ErrRigNotFound},
		{"FileNotFound", FileNotFound("x"), ErrFileNotFound},
		{"PermissionDenied", PermissionDenied("x"), ErrPermission},
		{"HookOccupied", HookOccupied("x"), ErrHookOccupied},
		{"Timeout", Timeout("x"), ErrTimeout},
		{"AlreadyExists", AlreadyExists("x"), ErrAlreadyExists},
	}

	for _, tt := range constructors {
		t.Run(tt.name, func(t *testing.T) {
			if got := Code(tt.err); got != tt.want {
				t.Errorf("Code() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestErrorInterface(t *testing.T) {
	// Verify Error satisfies error interface
	var _ error = &Error{}
	var _ error = New(ErrGeneral, "test")
	var _ error = Wrap(ErrGeneral, "test", nil)
	var _ error = BeadNotFound("test")
}

func TestWrapf(t *testing.T) {
	cause := errors.New("connection refused")
	err := Wrapf(ErrNetwork, cause, "failed to connect to %s on port %d", "localhost", 8080)

	if err.Code != ErrNetwork {
		t.Errorf("Code = %d, want %d", err.Code, ErrNetwork)
	}
	wantMsg := "failed to connect to localhost on port 8080"
	if err.Message != wantMsg {
		t.Errorf("Message = %q, want %q", err.Message, wantMsg)
	}
	if err.Cause != cause {
		t.Error("Wrapf should preserve cause")
	}
	wantErr := "failed to connect to localhost on port 8080: connection refused"
	if err.Error() != wantErr {
		t.Errorf("Error() = %q, want %q", err.Error(), wantErr)
	}
}
