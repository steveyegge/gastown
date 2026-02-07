package connection

import (
	"io/fs"
	"testing"
	"time"
)

func TestNewK8sConnection(t *testing.T) {
	conn := NewK8sConnection(K8sConnectionConfig{
		PodName:   "gt-gastown-polecat-alpha",
		Namespace: "gastown-test",
	})

	if conn.PodName != "gt-gastown-polecat-alpha" {
		t.Errorf("PodName = %q, want %q", conn.PodName, "gt-gastown-polecat-alpha")
	}
	if conn.Namespace != "gastown-test" {
		t.Errorf("Namespace = %q, want %q", conn.Namespace, "gastown-test")
	}
	if conn.tmux == nil {
		t.Error("tmux should not be nil")
	}
	if conn.execTimeout != DefaultExecTimeout {
		t.Errorf("execTimeout = %v, want %v", conn.execTimeout, DefaultExecTimeout)
	}
}

func TestK8sConnection_Name(t *testing.T) {
	conn := NewK8sConnection(K8sConnectionConfig{
		PodName:   "gt-gastown-polecat-alpha",
		Namespace: "gastown-test",
	})
	if got := conn.Name(); got != "gt-gastown-polecat-alpha" {
		t.Errorf("Name() = %q, want %q", got, "gt-gastown-polecat-alpha")
	}
}

func TestK8sConnection_IsLocal(t *testing.T) {
	conn := NewK8sConnection(K8sConnectionConfig{
		PodName:   "gt-gastown-polecat-alpha",
		Namespace: "gastown-test",
	})
	if conn.IsLocal() {
		t.Error("IsLocal() should return false for K8s connections")
	}
}

func TestK8sConnection_ImplementsInterface(t *testing.T) {
	var _ Connection = (*K8sConnection)(nil)
}

func TestK8sConnection_KubectlBaseArgs(t *testing.T) {
	tests := []struct {
		name     string
		cfg      K8sConnectionConfig
		wantArgs []string
	}{
		{
			name: "basic",
			cfg: K8sConnectionConfig{
				PodName:   "my-pod",
				Namespace: "my-ns",
			},
			wantArgs: []string{"exec", "my-pod", "-n", "my-ns"},
		},
		{
			name: "with container",
			cfg: K8sConnectionConfig{
				PodName:   "my-pod",
				Namespace: "my-ns",
				Container: "sidecar",
			},
			wantArgs: []string{"exec", "my-pod", "-n", "my-ns", "-c", "sidecar"},
		},
		{
			name: "with kubeconfig",
			cfg: K8sConnectionConfig{
				PodName:    "my-pod",
				Namespace:  "my-ns",
				KubeConfig: "/home/user/.kube/config",
			},
			wantArgs: []string{"--kubeconfig", "/home/user/.kube/config", "exec", "my-pod", "-n", "my-ns"},
		},
		{
			name: "no namespace",
			cfg: K8sConnectionConfig{
				PodName: "my-pod",
			},
			wantArgs: []string{"exec", "my-pod"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := NewK8sConnection(tt.cfg)
			got := conn.kubectlBaseArgs()

			if len(got) != len(tt.wantArgs) {
				t.Errorf("kubectlBaseArgs() length = %d, want %d\ngot:  %v\nwant: %v",
					len(got), len(tt.wantArgs), got, tt.wantArgs)
				return
			}
			for i := range got {
				if got[i] != tt.wantArgs[i] {
					t.Errorf("kubectlBaseArgs()[%d] = %q, want %q\ngot:  %v\nwant: %v",
						i, got[i], tt.wantArgs[i], got, tt.wantArgs)
					return
				}
			}
		})
	}
}

func TestParseStatOutput(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		wantErr bool
		check   func(t *testing.T, fi FileInfo)
	}{
		{
			name:   "regular file",
			output: "/etc/hosts|1234|644|1700000000|regular file",
			check: func(t *testing.T, fi FileInfo) {
				if fi.Name() != "/etc/hosts" {
					t.Errorf("Name() = %q, want %q", fi.Name(), "/etc/hosts")
				}
				if fi.Size() != 1234 {
					t.Errorf("Size() = %d, want %d", fi.Size(), 1234)
				}
				if fi.Mode() != fs.FileMode(0644) {
					t.Errorf("Mode() = %o, want %o", fi.Mode(), 0644)
				}
				if fi.ModTime() != time.Unix(1700000000, 0) {
					t.Errorf("ModTime() mismatch")
				}
				if fi.IsDir() {
					t.Error("IsDir() should be false for regular file")
				}
			},
		},
		{
			name:   "directory",
			output: "/home/gt|4096|755|1700000000|directory",
			check: func(t *testing.T, fi FileInfo) {
				if !fi.IsDir() {
					t.Error("IsDir() should be true for directory")
				}
				if fi.Name() != "/home/gt" {
					t.Errorf("Name() = %q, want %q", fi.Name(), "/home/gt")
				}
			},
		},
		{
			name:   "with trailing newline",
			output: "/tmp/test|100|600|1700000000|regular file\n",
			check: func(t *testing.T, fi FileInfo) {
				if fi.Name() != "/tmp/test" {
					t.Errorf("Name() = %q, want %q", fi.Name(), "/tmp/test")
				}
			},
		},
		{
			name:    "invalid format",
			output:  "bad output",
			wantErr: true,
		},
		{
			name:    "invalid size",
			output:  "/tmp|notanumber|644|1700000000|regular file",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fi, err := parseStatOutput(tt.output)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, fi)
			}
		})
	}
}

func TestK8sConnection_DefaultExecTimeout(t *testing.T) {
	if DefaultExecTimeout != 30*time.Second {
		t.Errorf("DefaultExecTimeout = %v, want %v", DefaultExecTimeout, 30*time.Second)
	}
}
