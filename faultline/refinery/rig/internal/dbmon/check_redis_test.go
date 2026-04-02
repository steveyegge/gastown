package dbmon

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

// mockRedisClient implements RedisClient for testing.
type mockRedisClient struct {
	pingErr    error
	infoData   map[string]string // section → response
	infoErr    error
	slowlog    []SlowLogEntry
	slowlogErr error
}

func (m *mockRedisClient) Ping(ctx context.Context) error {
	return m.pingErr
}

func (m *mockRedisClient) Info(ctx context.Context, sections ...string) (string, error) {
	if m.infoErr != nil {
		return "", m.infoErr
	}
	if len(sections) > 0 {
		if v, ok := m.infoData[sections[0]]; ok {
			return v, nil
		}
	}
	return "", nil
}

func (m *mockRedisClient) SlowLogGet(ctx context.Context, count int64) ([]SlowLogEntry, error) {
	return m.slowlog, m.slowlogErr
}

func (m *mockRedisClient) Close() error { return nil }

func redisTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestCheckRedis_PingOK(t *testing.T) {
	client := &mockRedisClient{
		infoData: map[string]string{
			"memory":      "used_memory:1000000\r\nmaxmemory:10000000\r\n",
			"stats":       "evicted_keys:0\r\n",
			"clients":     "connected_clients:5\r\nmaxclients:100\r\n",
			"replication": "role:master\r\nconnected_slaves:0\r\n",
		},
	}

	checker := NewRedisChecker(redisTestLogger(), nil)
	target := DatabaseTarget{ID: "redis-1", Name: "test-redis", DBType: "redis"}
	results := checker.Check(context.Background(), target, client)

	// Should have a ping result with OK status.
	var found bool
	for _, r := range results {
		if r.CheckType == "ping" {
			found = true
			if r.Status != CheckOK {
				t.Errorf("ping: expected ok, got %v", r.Status)
			}
		}
	}
	if !found {
		t.Error("expected a ping check result")
	}
}

func TestCheckRedis_PingFail(t *testing.T) {
	client := &mockRedisClient{
		pingErr: errors.New("connection refused"),
	}

	checker := NewRedisChecker(redisTestLogger(), nil)
	target := DatabaseTarget{ID: "redis-1", Name: "test-redis", DBType: "redis"}
	results := checker.Check(context.Background(), target, client)

	// When ping fails, should return a single critical result.
	if len(results) != 1 {
		t.Fatalf("expected 1 result on ping failure, got %d", len(results))
	}
	if results[0].Status != CheckCritical {
		t.Errorf("expected critical, got %v", results[0].Status)
	}
	if results[0].CheckType != "ping" {
		t.Errorf("expected check_type ping, got %s", results[0].CheckType)
	}
}

func TestCheckRedis_MemoryWarning(t *testing.T) {
	// used_memory is 95% of maxmemory → warning (threshold >90%)
	client := &mockRedisClient{
		infoData: map[string]string{
			"memory":      "used_memory:9500000\r\nmaxmemory:10000000\r\n",
			"stats":       "evicted_keys:0\r\n",
			"clients":     "connected_clients:5\r\nmaxclients:100\r\n",
			"replication": "role:master\r\nconnected_slaves:0\r\n",
		},
	}

	checker := NewRedisChecker(redisTestLogger(), nil)
	target := DatabaseTarget{ID: "redis-1", Name: "test-redis", DBType: "redis"}
	results := checker.Check(context.Background(), target, client)

	var found bool
	for _, r := range results {
		if r.CheckType == "memory" {
			found = true
			if r.Status != CheckWarning {
				t.Errorf("memory: expected warning at 95%% usage, got %v", r.Status)
			}
		}
	}
	if !found {
		t.Error("expected a memory check result")
	}
}

func TestCheckRedis_MemoryOKWhenNoMaxmemory(t *testing.T) {
	// maxmemory=0 means unlimited → always OK
	client := &mockRedisClient{
		infoData: map[string]string{
			"memory":      "used_memory:9500000\r\nmaxmemory:0\r\n",
			"stats":       "evicted_keys:0\r\n",
			"clients":     "connected_clients:5\r\nmaxclients:100\r\n",
			"replication": "role:master\r\nconnected_slaves:0\r\n",
		},
	}

	checker := NewRedisChecker(redisTestLogger(), nil)
	target := DatabaseTarget{ID: "redis-1", Name: "test-redis", DBType: "redis"}
	results := checker.Check(context.Background(), target, client)

	for _, r := range results {
		if r.CheckType == "memory" {
			if r.Status != CheckOK {
				t.Errorf("memory: expected ok when maxmemory=0 (unlimited), got %v", r.Status)
			}
		}
	}
}

func TestCheckRedis_ClientsWarning(t *testing.T) {
	// connected_clients at 85% of maxclients → warning (threshold >80%)
	client := &mockRedisClient{
		infoData: map[string]string{
			"memory":      "used_memory:1000000\r\nmaxmemory:10000000\r\n",
			"stats":       "evicted_keys:0\r\n",
			"clients":     "connected_clients:85\r\nmaxclients:100\r\n",
			"replication": "role:master\r\nconnected_slaves:0\r\n",
		},
	}

	checker := NewRedisChecker(redisTestLogger(), nil)
	target := DatabaseTarget{ID: "redis-1", Name: "test-redis", DBType: "redis"}
	results := checker.Check(context.Background(), target, client)

	var found bool
	for _, r := range results {
		if r.CheckType == "clients" {
			found = true
			if r.Status != CheckWarning {
				t.Errorf("clients: expected warning at 85%%, got %v", r.Status)
			}
		}
	}
	if !found {
		t.Error("expected a clients check result")
	}
}

func TestCheckRedis_EvictedKeysWarning(t *testing.T) {
	// evicted_keys > 0 with delta tracking
	checker := NewRedisChecker(redisTestLogger(), nil)
	target := DatabaseTarget{ID: "redis-1", Name: "test-redis", DBType: "redis"}

	// First call establishes baseline.
	client1 := &mockRedisClient{
		infoData: map[string]string{
			"memory":      "used_memory:1000000\r\nmaxmemory:10000000\r\n",
			"stats":       "evicted_keys:100\r\n",
			"clients":     "connected_clients:5\r\nmaxclients:100\r\n",
			"replication": "role:master\r\nconnected_slaves:0\r\n",
		},
	}
	checker.Check(context.Background(), target, client1)

	// Second call with higher evicted_keys → delta > 0 → warning.
	client2 := &mockRedisClient{
		infoData: map[string]string{
			"memory":      "used_memory:1000000\r\nmaxmemory:10000000\r\n",
			"stats":       "evicted_keys:150\r\n",
			"clients":     "connected_clients:5\r\nmaxclients:100\r\n",
			"replication": "role:master\r\nconnected_slaves:0\r\n",
		},
	}
	results := checker.Check(context.Background(), target, client2)

	var found bool
	for _, r := range results {
		if r.CheckType == "eviction" {
			found = true
			if r.Status != CheckWarning {
				t.Errorf("eviction: expected warning for delta 50, got %v", r.Status)
			}
			if r.Value == nil || *r.Value != 50 {
				t.Errorf("eviction: expected value 50, got %v", r.Value)
			}
		}
	}
	if !found {
		t.Error("expected an eviction check result")
	}
}

func TestCheckRedis_SlowlogEntries(t *testing.T) {
	client := &mockRedisClient{
		infoData: map[string]string{
			"memory":      "used_memory:1000000\r\nmaxmemory:10000000\r\n",
			"stats":       "evicted_keys:0\r\n",
			"clients":     "connected_clients:5\r\nmaxclients:100\r\n",
			"replication": "role:master\r\nconnected_slaves:0\r\n",
		},
		slowlog: []SlowLogEntry{
			{ID: 1, Time: time.Now(), Duration: 15 * time.Millisecond, Args: []string{"GET", "foo"}},
			{ID: 2, Time: time.Now(), Duration: 20 * time.Millisecond, Args: []string{"SET", "bar", "baz"}},
		},
	}

	checker := NewRedisChecker(redisTestLogger(), nil)
	target := DatabaseTarget{ID: "redis-1", Name: "test-redis", DBType: "redis"}
	results := checker.Check(context.Background(), target, client)

	var found bool
	for _, r := range results {
		if r.CheckType == "slowlog" {
			found = true
			if r.Status != CheckWarning {
				t.Errorf("slowlog: expected warning with entries, got %v", r.Status)
			}
			if r.Value == nil || *r.Value != 2 {
				t.Errorf("slowlog: expected value 2, got %v", r.Value)
			}
		}
	}
	if !found {
		t.Error("expected a slowlog check result")
	}
}

func TestCheckRedis_SlowlogEmpty(t *testing.T) {
	client := &mockRedisClient{
		infoData: map[string]string{
			"memory":      "used_memory:1000000\r\nmaxmemory:10000000\r\n",
			"stats":       "evicted_keys:0\r\n",
			"clients":     "connected_clients:5\r\nmaxclients:100\r\n",
			"replication": "role:master\r\nconnected_slaves:0\r\n",
		},
		slowlog: nil,
	}

	checker := NewRedisChecker(redisTestLogger(), nil)
	target := DatabaseTarget{ID: "redis-1", Name: "test-redis", DBType: "redis"}
	results := checker.Check(context.Background(), target, client)

	for _, r := range results {
		if r.CheckType == "slowlog" {
			if r.Status != CheckOK {
				t.Errorf("slowlog: expected ok with no entries, got %v", r.Status)
			}
		}
	}
}

func TestCheckRedis_ReplicationLagWarning(t *testing.T) {
	// Replica with master_last_io_seconds_ago > 10 → warning
	client := &mockRedisClient{
		infoData: map[string]string{
			"memory":      "used_memory:1000000\r\nmaxmemory:10000000\r\n",
			"stats":       "evicted_keys:0\r\n",
			"clients":     "connected_clients:5\r\nmaxclients:100\r\n",
			"replication": "role:slave\r\nmaster_link_status:up\r\nmaster_last_io_seconds_ago:15\r\n",
		},
	}

	checker := NewRedisChecker(redisTestLogger(), nil)
	target := DatabaseTarget{ID: "redis-1", Name: "test-redis", DBType: "redis"}
	results := checker.Check(context.Background(), target, client)

	var found bool
	for _, r := range results {
		if r.CheckType == "replication" {
			found = true
			if r.Status != CheckWarning {
				t.Errorf("replication: expected warning for lag 15s, got %v", r.Status)
			}
		}
	}
	if !found {
		t.Error("expected a replication check result")
	}
}

func TestCheckRedis_ReplicationLinkDown(t *testing.T) {
	client := &mockRedisClient{
		infoData: map[string]string{
			"memory":      "used_memory:1000000\r\nmaxmemory:10000000\r\n",
			"stats":       "evicted_keys:0\r\n",
			"clients":     "connected_clients:5\r\nmaxclients:100\r\n",
			"replication": "role:slave\r\nmaster_link_status:down\r\nmaster_last_io_seconds_ago:30\r\n",
		},
	}

	checker := NewRedisChecker(redisTestLogger(), nil)
	target := DatabaseTarget{ID: "redis-1", Name: "test-redis", DBType: "redis"}
	results := checker.Check(context.Background(), target, client)

	var found bool
	for _, r := range results {
		if r.CheckType == "replication" {
			found = true
			if r.Status != CheckCritical {
				t.Errorf("replication: expected critical when link down, got %v", r.Status)
			}
		}
	}
	if !found {
		t.Error("expected a replication check result")
	}
}

func TestCheckRedis_ReplicationMasterOK(t *testing.T) {
	// Master role → replication check should be OK (no lag concern)
	client := &mockRedisClient{
		infoData: map[string]string{
			"memory":      "used_memory:1000000\r\nmaxmemory:10000000\r\n",
			"stats":       "evicted_keys:0\r\n",
			"clients":     "connected_clients:5\r\nmaxclients:100\r\n",
			"replication": "role:master\r\nconnected_slaves:1\r\n",
		},
	}

	checker := NewRedisChecker(redisTestLogger(), nil)
	target := DatabaseTarget{ID: "redis-1", Name: "test-redis", DBType: "redis"}
	results := checker.Check(context.Background(), target, client)

	for _, r := range results {
		if r.CheckType == "replication" {
			if r.Status != CheckOK {
				t.Errorf("replication: expected ok for master, got %v", r.Status)
			}
		}
	}
}

func TestCheckRedis_AllChecksPresent(t *testing.T) {
	client := &mockRedisClient{
		infoData: map[string]string{
			"memory":      "used_memory:1000000\r\nmaxmemory:10000000\r\n",
			"stats":       "evicted_keys:0\r\n",
			"clients":     "connected_clients:5\r\nmaxclients:100\r\n",
			"replication": "role:master\r\nconnected_slaves:0\r\n",
		},
	}

	checker := NewRedisChecker(redisTestLogger(), nil)
	target := DatabaseTarget{ID: "redis-1", Name: "test-redis", DBType: "redis"}
	results := checker.Check(context.Background(), target, client)

	expected := map[string]bool{
		"ping":        false,
		"memory":      false,
		"eviction":    false,
		"slowlog":     false,
		"clients":     false,
		"replication": false,
	}

	for _, r := range results {
		if _, ok := expected[r.CheckType]; ok {
			expected[r.CheckType] = true
		}
		if r.DatabaseID != "redis-1" {
			t.Errorf("expected DatabaseID redis-1, got %s", r.DatabaseID)
		}
	}

	for check, found := range expected {
		if !found {
			t.Errorf("missing check type: %s", check)
		}
	}
}

func TestCheckRedis_AsCheckFunc(t *testing.T) {
	// Verify CheckRedis returns a valid CheckFunc that can be registered.
	checker := NewRedisChecker(redisTestLogger(), nil)
	fn := checker.CheckFunc()

	// CheckFunc should be assignable to the CheckFunc type.
	var _ CheckFunc = fn
}

func TestCheckRedis_CustomThresholds(t *testing.T) {
	// Custom thresholds override defaults.
	thresholds := &RedisThresholds{
		MemoryPercent:    0.50, // 50% instead of 90%
		ClientPercent:    0.50, // 50% instead of 80%
		ReplicationLagS:  5,   // 5s instead of 10s
	}

	// 60% memory usage → ok at default, warning at 50% threshold
	client := &mockRedisClient{
		infoData: map[string]string{
			"memory":      "used_memory:6000000\r\nmaxmemory:10000000\r\n",
			"stats":       "evicted_keys:0\r\n",
			"clients":     "connected_clients:5\r\nmaxclients:100\r\n",
			"replication": "role:master\r\nconnected_slaves:0\r\n",
		},
	}

	checker := NewRedisChecker(redisTestLogger(), thresholds)
	target := DatabaseTarget{ID: "redis-1", Name: "test-redis", DBType: "redis"}
	results := checker.Check(context.Background(), target, client)

	for _, r := range results {
		if r.CheckType == "memory" {
			if r.Status != CheckWarning {
				t.Errorf("memory: expected warning at 60%% with 50%% threshold, got %v", r.Status)
			}
		}
	}
}
