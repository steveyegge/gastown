package cmd

import (
	"testing"

	"github.com/steveyegge/gastown/internal/config"
)

func TestShouldRunDoneStrictVerification(t *testing.T) {
	t.Parallel()

	strict := &config.MergeQueueConfig{VerificationMode: config.VerificationModeStrict}
	advisory := &config.MergeQueueConfig{VerificationMode: config.VerificationModeAdvisory}

	tests := []struct {
		name        string
		mq          *config.MergeQueueConfig
		isNoMerge   bool
		convoy      *ConvoyInfo
		wantRunGate bool
	}{
		{name: "strict default", mq: strict, wantRunGate: true},
		{name: "advisory skips", mq: advisory, wantRunGate: false},
		{name: "no-merge skips", mq: strict, isNoMerge: true, wantRunGate: false},
		{name: "local convoy skips", mq: strict, convoy: &ConvoyInfo{MergeStrategy: "local"}, wantRunGate: false},
		{name: "direct convoy still verifies", mq: strict, convoy: &ConvoyInfo{MergeStrategy: "direct"}, wantRunGate: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := shouldRunDoneStrictVerification(tt.mq, tt.isNoMerge, tt.convoy)
			if got != tt.wantRunGate {
				t.Fatalf("shouldRunDoneStrictVerification() = %v, want %v", got, tt.wantRunGate)
			}
		})
	}
}
