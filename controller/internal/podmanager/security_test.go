package podmanager

import (
	"bytes"
	"log/slog"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func testLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestParseResourceCaps(t *testing.T) {
	tests := []struct {
		name      string
		maxCPU    string
		maxMemory string
		wantCPU   bool
		wantMem   bool
	}{
		{"both set", "2", "4Gi", true, true},
		{"cpu only", "1", "", true, false},
		{"memory only", "", "2Gi", false, true},
		{"neither", "", "", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caps := ParseResourceCaps(tt.maxCPU, tt.maxMemory)
			if tt.wantCPU && caps.MaxCPU.IsZero() {
				t.Error("expected non-zero MaxCPU")
			}
			if !tt.wantCPU && !caps.MaxCPU.IsZero() {
				t.Error("expected zero MaxCPU")
			}
			if tt.wantMem && caps.MaxMemory.IsZero() {
				t.Error("expected non-zero MaxMemory")
			}
			if !tt.wantMem && !caps.MaxMemory.IsZero() {
				t.Error("expected zero MaxMemory")
			}
		})
	}
}

func TestClampResources_NoCaps(t *testing.T) {
	var buf bytes.Buffer
	logger := testLogger(&buf)

	reqs := corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("8"),
			corev1.ResourceMemory: resource.MustParse("16Gi"),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("4"),
			corev1.ResourceMemory: resource.MustParse("8Gi"),
		},
	}
	caps := ResourceCaps{} // zero = no caps

	result := ClampResources(reqs, caps, logger)

	if result.Limits.Cpu().Cmp(resource.MustParse("8")) != 0 {
		t.Errorf("CPU limit should be unchanged, got %s", result.Limits.Cpu())
	}
	if result.Limits.Memory().Cmp(resource.MustParse("16Gi")) != 0 {
		t.Errorf("Memory limit should be unchanged, got %s", result.Limits.Memory())
	}
	if buf.Len() > 0 {
		t.Error("expected no log warnings with no caps")
	}
}

func TestClampResources_BelowCap(t *testing.T) {
	var buf bytes.Buffer
	logger := testLogger(&buf)

	reqs := corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		},
	}
	caps := ParseResourceCaps("2", "4Gi")

	result := ClampResources(reqs, caps, logger)

	if result.Limits.Cpu().Cmp(resource.MustParse("1")) != 0 {
		t.Errorf("CPU limit should be unchanged, got %s", result.Limits.Cpu())
	}
	if result.Limits.Memory().Cmp(resource.MustParse("2Gi")) != 0 {
		t.Errorf("Memory limit should be unchanged, got %s", result.Limits.Memory())
	}
	if buf.Len() > 0 {
		t.Error("expected no log warnings when below cap")
	}
}

func TestClampResources_AboveCap(t *testing.T) {
	var buf bytes.Buffer
	logger := testLogger(&buf)

	reqs := corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("8"),
			corev1.ResourceMemory: resource.MustParse("16Gi"),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("4"),
			corev1.ResourceMemory: resource.MustParse("8Gi"),
		},
	}
	caps := ParseResourceCaps("2", "4Gi")

	result := ClampResources(reqs, caps, logger)

	if result.Limits.Cpu().Cmp(resource.MustParse("2")) != 0 {
		t.Errorf("CPU limit should be clamped to 2, got %s", result.Limits.Cpu())
	}
	if result.Limits.Memory().Cmp(resource.MustParse("4Gi")) != 0 {
		t.Errorf("Memory limit should be clamped to 4Gi, got %s", result.Limits.Memory())
	}
	if result.Requests.Cpu().Cmp(resource.MustParse("2")) != 0 {
		t.Errorf("CPU request should be clamped to 2, got %s", result.Requests.Cpu())
	}
	if result.Requests.Memory().Cmp(resource.MustParse("4Gi")) != 0 {
		t.Errorf("Memory request should be clamped to 4Gi, got %s", result.Requests.Memory())
	}

	logOutput := buf.String()
	if logOutput == "" {
		t.Error("expected warning log output when clamping")
	}
}

func TestClampResources_PartialCap(t *testing.T) {
	var buf bytes.Buffer
	logger := testLogger(&buf)

	reqs := corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("8"),
			corev1.ResourceMemory: resource.MustParse("16Gi"),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("4"),
			corev1.ResourceMemory: resource.MustParse("8Gi"),
		},
	}
	caps := ParseResourceCaps("4", "") // only CPU cap, no memory cap

	result := ClampResources(reqs, caps, logger)

	if result.Limits.Cpu().Cmp(resource.MustParse("4")) != 0 {
		t.Errorf("CPU limit should be clamped to 4, got %s", result.Limits.Cpu())
	}
	// Memory should remain unchanged
	if result.Limits.Memory().Cmp(resource.MustParse("16Gi")) != 0 {
		t.Errorf("Memory limit should be unchanged, got %s", result.Limits.Memory())
	}
}

func TestClampResources_ExactCap(t *testing.T) {
	var buf bytes.Buffer
	logger := testLogger(&buf)

	reqs := corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("2"),
			corev1.ResourceMemory: resource.MustParse("4Gi"),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
	}
	caps := ParseResourceCaps("2", "4Gi")

	result := ClampResources(reqs, caps, logger)

	if result.Limits.Cpu().Cmp(resource.MustParse("2")) != 0 {
		t.Errorf("CPU limit should be unchanged at cap, got %s", result.Limits.Cpu())
	}
	if result.Limits.Memory().Cmp(resource.MustParse("4Gi")) != 0 {
		t.Errorf("Memory limit should be unchanged at cap, got %s", result.Limits.Memory())
	}
	if buf.Len() > 0 {
		t.Error("expected no warnings at exact cap boundary")
	}
}

func TestClampResources_EmptyLists(t *testing.T) {
	var buf bytes.Buffer
	logger := testLogger(&buf)

	reqs := corev1.ResourceRequirements{} // nil maps
	caps := ParseResourceCaps("2", "4Gi")

	result := ClampResources(reqs, caps, logger)

	if len(result.Limits) != 0 {
		t.Error("expected empty limits to remain empty")
	}
	if len(result.Requests) != 0 {
		t.Error("expected empty requests to remain empty")
	}
}

func TestClampResources_DoesNotMutateOriginal(t *testing.T) {
	var buf bytes.Buffer
	logger := testLogger(&buf)

	reqs := corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse("8"),
		},
	}
	caps := ParseResourceCaps("2", "")

	_ = ClampResources(reqs, caps, logger)

	// Original should be unchanged
	if reqs.Limits.Cpu().Cmp(resource.MustParse("8")) != 0 {
		t.Errorf("original limits were mutated: got %s", reqs.Limits.Cpu())
	}
}

func TestSidecarPullPolicy(t *testing.T) {
	tests := []struct {
		name   string
		spec   ToolchainSidecarSpec
		expect corev1.PullPolicy
	}{
		{
			name:   "custom image always pulls",
			spec:   ToolchainSidecarSpec{Image: "myregistry.io/custom-tools:v1"},
			expect: corev1.PullAlways,
		},
		{
			name:   "profile with latest tag always pulls",
			spec:   ToolchainSidecarSpec{Profile: "toolchain-full", Image: "ghcr.io/org/toolchain:latest"},
			expect: corev1.PullAlways,
		},
		{
			name:   "profile with no tag always pulls",
			spec:   ToolchainSidecarSpec{Profile: "toolchain-full", Image: "ghcr.io/org/toolchain"},
			expect: corev1.PullAlways,
		},
		{
			name:   "profile with pinned tag uses IfNotPresent",
			spec:   ToolchainSidecarSpec{Profile: "toolchain-full", Image: "ghcr.io/org/toolchain:v1.2.3"},
			expect: corev1.PullIfNotPresent,
		},
		{
			name:   "profile with digest uses IfNotPresent",
			spec:   ToolchainSidecarSpec{Profile: "toolchain-full", Image: "ghcr.io/org/toolchain@sha256:abc123def456"},
			expect: corev1.PullIfNotPresent,
		},
		{
			name:   "profile with port in registry and pinned tag",
			spec:   ToolchainSidecarSpec{Profile: "custom", Image: "registry.local:5000/tools:v2.0"},
			expect: corev1.PullIfNotPresent,
		},
		{
			name:   "profile with port in registry and latest tag",
			spec:   ToolchainSidecarSpec{Profile: "custom", Image: "registry.local:5000/tools:latest"},
			expect: corev1.PullAlways,
		},
		{
			name:   "profile with port in registry and no tag",
			spec:   ToolchainSidecarSpec{Profile: "custom", Image: "registry.local:5000/tools"},
			expect: corev1.PullAlways,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SidecarPullPolicy(&tt.spec)
			if got != tt.expect {
				t.Errorf("SidecarPullPolicy() = %s, want %s", got, tt.expect)
			}
		})
	}
}

func TestHasLatestOrNoTag(t *testing.T) {
	tests := []struct {
		image string
		want  bool
	}{
		{"nginx", true},
		{"nginx:latest", true},
		{"nginx:1.21", false},
		{"ghcr.io/org/img:latest", true},
		{"ghcr.io/org/img:v1.0", false},
		{"ghcr.io/org/img", true},
		{"ghcr.io/org/img@sha256:abc123", false},
		{"registry.local:5000/img", true},
		{"registry.local:5000/img:latest", true},
		{"registry.local:5000/img:v1", false},
	}

	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			got := hasLatestOrNoTag(tt.image)
			if got != tt.want {
				t.Errorf("hasLatestOrNoTag(%q) = %v, want %v", tt.image, got, tt.want)
			}
		})
	}
}
