package telemetry

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
)

// resetInitState resets the package-level telemetry init guard so tests run
// independently of each other.
func resetInitState(t *testing.T) {
	t.Helper()
	initMu.Lock()
	initDone = false
	globalProvider = nil
	initMu.Unlock()
	endpointProbeMu.Lock()
	endpointProbeCache = make(map[string]string)
	endpointProbeMu.Unlock()
	originalDial := dialEndpoint
	dialEndpoint = func(ctx context.Context, network, address string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, address)
	}
	t.Cleanup(func() {
		initMu.Lock()
		initDone = false
		globalProvider = nil
		initMu.Unlock()
		endpointProbeMu.Lock()
		endpointProbeCache = make(map[string]string)
		endpointProbeMu.Unlock()
		dialEndpoint = originalDial
	})
}

func TestInit_BothURLsUnset_ReturnsNil(t *testing.T) {
	resetInitState(t)
	t.Setenv(EnvMetricsURL, "")
	t.Setenv(EnvLogsURL, "")

	p, err := Init(context.Background(), "test-svc", "0.0.1")
	if err != nil {
		t.Fatalf("Init error: %v", err)
	}
	if p != nil {
		t.Error("expected nil provider when both URLs are unset")
	}
}

func TestInit_Idempotent_ReturnsFirstProvider(t *testing.T) {
	resetInitState(t)
	t.Setenv(EnvMetricsURL, "")
	t.Setenv(EnvLogsURL, "")

	p1, _ := Init(context.Background(), "test-svc", "0.0.1")
	p2, _ := Init(context.Background(), "test-svc", "0.0.1")

	if p1 != p2 {
		t.Error("second Init call should return the same provider as the first")
	}
}

func TestProvider_Shutdown_Idempotent(t *testing.T) {
	p := &Provider{}
	called := 0
	p.shutdowns = []func(context.Context) error{
		func(_ context.Context) error { called++; return nil },
	}

	ctx := context.Background()
	if err := p.Shutdown(ctx); err != nil {
		t.Fatalf("first Shutdown error: %v", err)
	}
	if err := p.Shutdown(ctx); err != nil {
		t.Fatalf("second Shutdown error: %v", err)
	}
	if called != 1 {
		t.Errorf("expected shutdown fn called once, called %d times", called)
	}
}

func TestProvider_Shutdown_CollectsErrors(t *testing.T) {
	p := &Provider{}
	p.shutdowns = []func(context.Context) error{
		func(_ context.Context) error { return errors.New("err1") },
		func(_ context.Context) error { return errors.New("err2") },
	}

	err := p.Shutdown(context.Background())
	if err == nil {
		t.Fatal("expected error from Shutdown when shutdown fns fail")
	}
}

func TestProvider_Shutdown_Empty(t *testing.T) {
	p := &Provider{}
	if err := p.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown with no fns should not error: %v", err)
	}
}

func TestProvider_Shutdown_ConcurrentSafe(t *testing.T) {
	p := &Provider{}
	called := 0
	var mu sync.Mutex
	p.shutdowns = []func(context.Context) error{
		func(_ context.Context) error {
			mu.Lock()
			called++
			mu.Unlock()
			return nil
		},
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = p.Shutdown(context.Background())
		}()
	}
	wg.Wait()

	if called != 1 {
		t.Errorf("expected shutdown fn called exactly once, called %d times", called)
	}
}

func TestFilterUnavailableLoopbackEndpoint_SkipsClosedPort(t *testing.T) {
	resetInitState(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	rawURL := "http://" + addr + "/v1/logs"
	if got := filterUnavailableLoopbackEndpoint(rawURL); got != "" {
		t.Fatalf("filterUnavailableLoopbackEndpoint(%q) = %q, want empty string", rawURL, got)
	}
}

func TestFilterUnavailableLoopbackEndpoint_KeepsReachablePort(t *testing.T) {
	resetInitState(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	rawURL := "http://" + ln.Addr().String() + "/v1/logs"
	if got := filterUnavailableLoopbackEndpoint(rawURL); got != rawURL {
		t.Fatalf("filterUnavailableLoopbackEndpoint(%q) = %q, want %q", rawURL, got, rawURL)
	}
}

func TestEffectiveURLsFromEnv_DropsUnavailableLoopbackEndpoints(t *testing.T) {
	resetInitState(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	t.Setenv(EnvMetricsURL, "http://"+addr+"/v1/metrics")
	t.Setenv(EnvLogsURL, "http://"+addr+"/v1/logs")

	metricsURL, logsURL := EffectiveURLsFromEnv()
	if metricsURL != "" || logsURL != "" {
		t.Fatalf("EffectiveURLsFromEnv() = (%q, %q), want empty endpoints", metricsURL, logsURL)
	}
}
