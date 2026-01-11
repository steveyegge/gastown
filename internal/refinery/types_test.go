package refinery

import (
	"errors"
	"testing"
)

func TestValidateTransition(t *testing.T) {
	tests := []struct {
		name    string
		from    MRStatus
		to      MRStatus
		wantErr bool
		errType error
	}{
		// Valid transitions
		{
			name:    "open to in_progress (claim)",
			from:    MROpen,
			to:      MRInProgress,
			wantErr: false,
		},
		{
			name:    "open to closed (manual rejection)",
			from:    MROpen,
			to:      MRClosed,
			wantErr: false,
		},
		{
			name:    "in_progress to closed (success/rejection)",
			from:    MRInProgress,
			to:      MRClosed,
			wantErr: false,
		},
		{
			name:    "in_progress to open (failure/reassign)",
			from:    MRInProgress,
			to:      MROpen,
			wantErr: false,
		},
		{
			name:    "same state (no-op)",
			from:    MROpen,
			to:      MROpen,
			wantErr: false,
		},
		{
			name:    "same state closed (no-op)",
			from:    MRClosed,
			to:      MRClosed,
			wantErr: false,
		},

		// Invalid transitions
		{
			name:    "closed to open (immutable)",
			from:    MRClosed,
			to:      MROpen,
			wantErr: true,
			errType: ErrClosedImmutable,
		},
		{
			name:    "closed to in_progress (immutable)",
			from:    MRClosed,
			to:      MRInProgress,
			wantErr: true,
			errType: ErrClosedImmutable,
		},
		{
			name:    "open to open is valid (no-op)",
			from:    MROpen,
			to:      MROpen,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTransition(tt.from, tt.to)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateTransition(%s, %s) expected error, got nil", tt.from, tt.to)
					return
				}
				if tt.errType != nil && !errors.Is(err, tt.errType) {
					t.Errorf("ValidateTransition(%s, %s) error = %v, want %v", tt.from, tt.to, err, tt.errType)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateTransition(%s, %s) unexpected error: %v", tt.from, tt.to, err)
				}
			}
		})
	}
}

func TestMergeRequest_Claim(t *testing.T) {
	t.Run("claim from open succeeds", func(t *testing.T) {
		mr := &MergeRequest{Status: MROpen}
		err := mr.Claim()
		if err != nil {
			t.Errorf("Claim() unexpected error: %v", err)
		}
		if mr.Status != MRInProgress {
			t.Errorf("Claim() status = %s, want %s", mr.Status, MRInProgress)
		}
	})

	t.Run("claim from in_progress fails", func(t *testing.T) {
		mr := &MergeRequest{Status: MRInProgress}
		err := mr.Claim()
		if err == nil {
			t.Error("Claim() expected error, got nil")
		}
		if !errors.Is(err, ErrInvalidTransition) {
			t.Errorf("Claim() error = %v, want %v", err, ErrInvalidTransition)
		}
	})

	t.Run("claim from closed fails", func(t *testing.T) {
		mr := &MergeRequest{Status: MRClosed}
		err := mr.Claim()
		if err == nil {
			t.Error("Claim() expected error, got nil")
		}
	})
}

func TestMergeRequest_Close(t *testing.T) {
	t.Run("close from in_progress succeeds", func(t *testing.T) {
		mr := &MergeRequest{Status: MRInProgress}
		err := mr.Close(CloseReasonMerged)
		if err != nil {
			t.Errorf("Close() unexpected error: %v", err)
		}
		if mr.Status != MRClosed {
			t.Errorf("Close() status = %s, want %s", mr.Status, MRClosed)
		}
		if mr.CloseReason != CloseReasonMerged {
			t.Errorf("Close() closeReason = %s, want %s", mr.CloseReason, CloseReasonMerged)
		}
	})

	t.Run("close from open succeeds (manual rejection)", func(t *testing.T) {
		mr := &MergeRequest{Status: MROpen}
		err := mr.Close(CloseReasonRejected)
		if err != nil {
			t.Errorf("Close() unexpected error: %v", err)
		}
		if mr.Status != MRClosed {
			t.Errorf("Close() status = %s, want %s", mr.Status, MRClosed)
		}
	})

	t.Run("close from closed fails", func(t *testing.T) {
		mr := &MergeRequest{Status: MRClosed, CloseReason: CloseReasonMerged}
		err := mr.Close(CloseReasonRejected)
		if err == nil {
			t.Error("Close() expected error, got nil")
		}
		if !errors.Is(err, ErrClosedImmutable) {
			t.Errorf("Close() error = %v, want %v", err, ErrClosedImmutable)
		}
	})
}

func TestMergeRequest_Reopen(t *testing.T) {
	t.Run("reopen from in_progress succeeds", func(t *testing.T) {
		mr := &MergeRequest{Status: MRInProgress}
		err := mr.Reopen()
		if err != nil {
			t.Errorf("Reopen() unexpected error: %v", err)
		}
		if mr.Status != MROpen {
			t.Errorf("Reopen() status = %s, want %s", mr.Status, MROpen)
		}
	})

	t.Run("reopen from open fails", func(t *testing.T) {
		mr := &MergeRequest{Status: MROpen}
		err := mr.Reopen()
		if err == nil {
			t.Error("Reopen() expected error, got nil")
		}
		if !errors.Is(err, ErrInvalidTransition) {
			t.Errorf("Reopen() error = %v, want %v", err, ErrInvalidTransition)
		}
	})

	t.Run("reopen from closed fails", func(t *testing.T) {
		mr := &MergeRequest{Status: MRClosed}
		err := mr.Reopen()
		if err == nil {
			t.Error("Reopen() expected error, got nil")
		}
	})

	t.Run("reopen clears close reason", func(t *testing.T) {
		mr := &MergeRequest{Status: MRInProgress, CloseReason: CloseReasonMerged}
		err := mr.Reopen()
		if err != nil {
			t.Errorf("Reopen() unexpected error: %v", err)
		}
		if mr.CloseReason != "" {
			t.Errorf("Reopen() closeReason = %s, want empty", mr.CloseReason)
		}
	})
}

func TestMergeRequest_SetStatus(t *testing.T) {
	t.Run("valid transition succeeds", func(t *testing.T) {
		mr := &MergeRequest{Status: MROpen}
		err := mr.SetStatus(MRInProgress)
		if err != nil {
			t.Errorf("SetStatus() unexpected error: %v", err)
		}
		if mr.Status != MRInProgress {
			t.Errorf("SetStatus() status = %s, want %s", mr.Status, MRInProgress)
		}
	})

	t.Run("invalid transition fails", func(t *testing.T) {
		mr := &MergeRequest{Status: MRClosed}
		err := mr.SetStatus(MROpen)
		if err == nil {
			t.Error("SetStatus() expected error, got nil")
		}
	})
}

func TestMergeRequest_StatusChecks(t *testing.T) {
	tests := []struct {
		status       MRStatus
		isClosed     bool
		isOpen       bool
		isInProgress bool
	}{
		{MROpen, false, true, false},
		{MRInProgress, false, false, true},
		{MRClosed, true, false, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			mr := &MergeRequest{Status: tt.status}
			if mr.IsClosed() != tt.isClosed {
				t.Errorf("IsClosed() = %v, want %v", mr.IsClosed(), tt.isClosed)
			}
			if mr.IsOpen() != tt.isOpen {
				t.Errorf("IsOpen() = %v, want %v", mr.IsOpen(), tt.isOpen)
			}
			if mr.IsInProgress() != tt.isInProgress {
				t.Errorf("IsInProgress() = %v, want %v", mr.IsInProgress(), tt.isInProgress)
			}
		})
	}
}

func TestFailureType_FailureLabel(t *testing.T) {
	tests := []struct {
		failureType FailureType
		wantLabel   string
	}{
		{FailureNone, ""},
		{FailureConflict, "needs-rebase"},
		{FailureTestsFail, "needs-fix"},
		{FailureBuildFail, "needs-fix"},
		{FailureFlakyTest, "needs-fix"},
		{FailurePushFail, "needs-retry"},
		{FailureFetch, ""},
		{FailureCheckout, ""},
	}

	for _, tt := range tests {
		t.Run(string(tt.failureType), func(t *testing.T) {
			got := tt.failureType.FailureLabel()
			if got != tt.wantLabel {
				t.Errorf("FailureLabel() = %q, want %q", got, tt.wantLabel)
			}
		})
	}
}

func TestFailureType_ShouldAssignToWorker(t *testing.T) {
	tests := []struct {
		failureType FailureType
		wantAssign  bool
	}{
		{FailureNone, false},
		{FailureConflict, true},
		{FailureTestsFail, true},
		{FailureBuildFail, true},
		{FailureFlakyTest, true},
		{FailurePushFail, false},
		{FailureFetch, false},
		{FailureCheckout, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.failureType), func(t *testing.T) {
			got := tt.failureType.ShouldAssignToWorker()
			if got != tt.wantAssign {
				t.Errorf("ShouldAssignToWorker() = %v, want %v", got, tt.wantAssign)
			}
		})
	}
}
