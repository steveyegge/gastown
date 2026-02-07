package terminal

import (
	"testing"
)

func TestNewPodConnection(t *testing.T) {
	pc := NewPodConnection(PodConnectionConfig{
		AgentID:     "gastown/polecats/alpha",
		PodName:     "gt-gastown-polecat-alpha",
		Namespace:   "gastown-test",
		SessionName: "gt-gastown-alpha",
	})

	if pc.AgentID != "gastown/polecats/alpha" {
		t.Errorf("AgentID = %q, want %q", pc.AgentID, "gastown/polecats/alpha")
	}
	if pc.PodName != "gt-gastown-polecat-alpha" {
		t.Errorf("PodName = %q, want %q", pc.PodName, "gt-gastown-polecat-alpha")
	}
	if pc.Namespace != "gastown-test" {
		t.Errorf("Namespace = %q, want %q", pc.Namespace, "gastown-test")
	}
	if pc.SessionName != "gt-gastown-alpha" {
		t.Errorf("SessionName = %q, want %q", pc.SessionName, "gt-gastown-alpha")
	}
	if pc.ScreenSession != DefaultScreenSession {
		t.Errorf("ScreenSession = %q, want %q", pc.ScreenSession, DefaultScreenSession)
	}
	if pc.tmux == nil {
		t.Error("tmux should not be nil")
	}
	if pc.connected {
		t.Error("connected should be false initially")
	}
}

func TestNewPodConnection_CustomScreenSession(t *testing.T) {
	pc := NewPodConnection(PodConnectionConfig{
		AgentID:       "gastown/polecats/alpha",
		PodName:       "gt-gastown-polecat-alpha",
		Namespace:     "gastown-test",
		SessionName:   "gt-gastown-alpha",
		ScreenSession: "custom-session",
	})

	if pc.ScreenSession != "custom-session" {
		t.Errorf("ScreenSession = %q, want %q", pc.ScreenSession, "custom-session")
	}
}

func TestPodConnection_KubectlExecCommand(t *testing.T) {
	tests := []struct {
		name    string
		cfg     PodConnectionConfig
		wantCmd string
	}{
		{
			name: "basic",
			cfg: PodConnectionConfig{
				AgentID:     "gastown/polecats/alpha",
				PodName:     "gt-gastown-polecat-alpha",
				Namespace:   "gastown-test",
				SessionName: "gt-gastown-alpha",
			},
			wantCmd: "kubectl exec -it -n gastown-test gt-gastown-polecat-alpha -- screen -x agent",
		},
		{
			name: "with kubeconfig",
			cfg: PodConnectionConfig{
				AgentID:     "gastown/polecats/alpha",
				PodName:     "gt-gastown-polecat-alpha",
				Namespace:   "gastown-test",
				SessionName: "gt-gastown-alpha",
				KubeConfig:  "/home/user/.kube/config",
			},
			wantCmd: "kubectl --kubeconfig /home/user/.kube/config exec -it -n gastown-test gt-gastown-polecat-alpha -- screen -x agent",
		},
		{
			name: "custom screen session",
			cfg: PodConnectionConfig{
				AgentID:       "gastown/polecats/alpha",
				PodName:       "gt-gastown-polecat-alpha",
				Namespace:     "gastown-test",
				SessionName:   "gt-gastown-alpha",
				ScreenSession: "claude",
			},
			wantCmd: "kubectl exec -it -n gastown-test gt-gastown-polecat-alpha -- screen -x claude",
		},
		{
			name: "no namespace",
			cfg: PodConnectionConfig{
				AgentID:     "gastown/polecats/alpha",
				PodName:     "gt-gastown-polecat-alpha",
				SessionName: "gt-gastown-alpha",
			},
			wantCmd: "kubectl exec -it gt-gastown-polecat-alpha -- screen -x agent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := NewPodConnection(tt.cfg)
			got := pc.kubectlExecCommand()
			if got != tt.wantCmd {
				t.Errorf("kubectlExecCommand() = %q, want %q", got, tt.wantCmd)
			}
		})
	}
}

func TestPodConnection_IsConnected_Initial(t *testing.T) {
	pc := NewPodConnection(PodConnectionConfig{
		AgentID:     "gastown/polecats/alpha",
		PodName:     "gt-gastown-polecat-alpha",
		Namespace:   "gastown-test",
		SessionName: "gt-gastown-alpha",
	})

	if pc.IsConnected() {
		t.Error("IsConnected() should be false before Open()")
	}
}

func TestPodConnection_ReconnectCount_Initial(t *testing.T) {
	pc := NewPodConnection(PodConnectionConfig{
		AgentID:     "gastown/polecats/alpha",
		PodName:     "gt-gastown-polecat-alpha",
		Namespace:   "gastown-test",
		SessionName: "gt-gastown-alpha",
	})

	if pc.ReconnectCount() != 0 {
		t.Errorf("ReconnectCount() = %d, want 0", pc.ReconnectCount())
	}
}

func TestPodConnection_IsAlive_NotConnected(t *testing.T) {
	pc := NewPodConnection(PodConnectionConfig{
		AgentID:     "gastown/polecats/alpha",
		PodName:     "gt-gastown-polecat-alpha",
		Namespace:   "gastown-test",
		SessionName: "gt-gastown-alpha",
	})

	if pc.IsAlive() {
		t.Error("IsAlive() should be false when not connected")
	}
}

func TestDefaultScreenSession(t *testing.T) {
	if DefaultScreenSession != "agent" {
		t.Errorf("DefaultScreenSession = %q, want %q", DefaultScreenSession, "agent")
	}
}

func TestMaxReconnectAttempts(t *testing.T) {
	if MaxReconnectAttempts != 5 {
		t.Errorf("MaxReconnectAttempts = %d, want %d", MaxReconnectAttempts, 5)
	}
}
