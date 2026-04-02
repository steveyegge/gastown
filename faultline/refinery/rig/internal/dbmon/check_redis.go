package dbmon

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

// RedisThresholds configures warning thresholds for Redis checks.
type RedisThresholds struct {
	MemoryPercent   float64 // fraction of maxmemory (default 0.90)
	ClientPercent   float64 // fraction of maxclients (default 0.80)
	ReplicationLagS int     // seconds (default 10)
}

var defaultRedisThresholds = RedisThresholds{
	MemoryPercent:   0.90,
	ClientPercent:   0.80,
	ReplicationLagS: 10,
}

// SlowLogEntry represents a Redis SLOWLOG entry.
type SlowLogEntry struct {
	ID       int64
	Time     time.Time
	Duration time.Duration
	Args     []string
}

// RedisClient abstracts the Redis commands needed for health checks.
type RedisClient interface {
	Ping(ctx context.Context) error
	Info(ctx context.Context, sections ...string) (string, error)
	SlowLogGet(ctx context.Context, count int64) ([]SlowLogEntry, error)
	Close() error
}

// RedisClientFactory creates a RedisClient from a connection string.
type RedisClientFactory func(connStr string) (RedisClient, error)

// RedisChecker performs health checks against a Redis target.
type RedisChecker struct {
	log           *slog.Logger
	thresholds    RedisThresholds
	clientFactory RedisClientFactory
	lastEvicted   map[string]int64 // databaseID → last evicted_keys value
}

// NewRedisChecker creates a Redis checker with optional threshold overrides.
func NewRedisChecker(log *slog.Logger, thresholds *RedisThresholds, factory ...RedisClientFactory) *RedisChecker {
	t := defaultRedisThresholds
	if thresholds != nil {
		if thresholds.MemoryPercent > 0 {
			t.MemoryPercent = thresholds.MemoryPercent
		}
		if thresholds.ClientPercent > 0 {
			t.ClientPercent = thresholds.ClientPercent
		}
		if thresholds.ReplicationLagS > 0 {
			t.ReplicationLagS = thresholds.ReplicationLagS
		}
	}
	var f RedisClientFactory
	if len(factory) > 0 {
		f = factory[0]
	}
	return &RedisChecker{
		log:         log,
		thresholds:  t,
		clientFactory: f,
		lastEvicted: make(map[string]int64),
	}
}

// CheckFunc returns a CheckFunc suitable for Monitor.RegisterChecker.
// Uses the client factory to create connections from target.ConnectionString.
func (rc *RedisChecker) CheckFunc() CheckFunc {
	return func(ctx context.Context, target DatabaseTarget) []CheckResult {
		if rc.clientFactory == nil {
			return []CheckResult{NewCheckResult(target.ID, "connection", CheckCritical, nil, "no redis client factory configured")}
		}
		client, err := rc.clientFactory(target.ConnectionString)
		if err != nil {
			return []CheckResult{NewCheckResult(target.ID, "ping", CheckCritical, nil, "connect failed: "+err.Error())}
		}
		defer client.Close()
		return rc.Check(ctx, target, client)
	}
}

// Check runs all Redis health checks against the given client.
func (rc *RedisChecker) Check(ctx context.Context, target DatabaseTarget, client RedisClient) []CheckResult {
	// PING — if this fails, skip remaining checks.
	if err := client.Ping(ctx); err != nil {
		return []CheckResult{NewCheckResult(target.ID, "ping", CheckCritical, nil, "ping failed: "+err.Error())}
	}

	results := []CheckResult{
		NewCheckResult(target.ID, "ping", CheckOK, nil, "PONG"),
	}

	results = append(results, rc.checkMemory(ctx, target, client)...)
	results = append(results, rc.checkEviction(ctx, target, client)...)
	results = append(results, rc.checkSlowlog(ctx, target, client)...)
	results = append(results, rc.checkClients(ctx, target, client)...)
	results = append(results, rc.checkReplication(ctx, target, client)...)

	return results
}

func (rc *RedisChecker) checkMemory(ctx context.Context, target DatabaseTarget, client RedisClient) []CheckResult {
	info, err := client.Info(ctx, "memory")
	if err != nil {
		return []CheckResult{NewCheckResult(target.ID, "memory", CheckCritical, nil, "INFO memory failed: "+err.Error())}
	}

	used := parseInfoInt(info, "used_memory")
	max := parseInfoInt(info, "maxmemory")

	if max == 0 {
		// maxmemory=0 means unlimited.
		pct := 0.0
		return []CheckResult{NewCheckResult(target.ID, "memory", CheckOK, &pct, "maxmemory unlimited")}
	}

	pct := float64(used) / float64(max)
	status := CheckOK
	msg := fmt.Sprintf("%.1f%% of maxmemory (%d/%d)", pct*100, used, max)
	if pct > rc.thresholds.MemoryPercent {
		status = CheckWarning
	}

	return []CheckResult{NewCheckResult(target.ID, "memory", status, &pct, msg)}
}

func (rc *RedisChecker) checkEviction(ctx context.Context, target DatabaseTarget, client RedisClient) []CheckResult {
	info, err := client.Info(ctx, "stats")
	if err != nil {
		return []CheckResult{NewCheckResult(target.ID, "eviction", CheckCritical, nil, "INFO stats failed: "+err.Error())}
	}

	evicted := parseInfoInt(info, "evicted_keys")

	prev, hasPrev := rc.lastEvicted[target.ID]
	rc.lastEvicted[target.ID] = evicted

	if !hasPrev {
		zero := 0.0
		return []CheckResult{NewCheckResult(target.ID, "eviction", CheckOK, &zero, "baseline established")}
	}

	delta := float64(evicted - prev)
	status := CheckOK
	msg := fmt.Sprintf("evicted_keys delta: %.0f", delta)
	if delta > 0 {
		status = CheckWarning
		msg = fmt.Sprintf("evicted_keys delta: %.0f (keys being evicted)", delta)
	}

	return []CheckResult{NewCheckResult(target.ID, "eviction", status, &delta, msg)}
}

func (rc *RedisChecker) checkSlowlog(ctx context.Context, target DatabaseTarget, client RedisClient) []CheckResult {
	entries, err := client.SlowLogGet(ctx, 10)
	if err != nil {
		return []CheckResult{NewCheckResult(target.ID, "slowlog", CheckCritical, nil, "SLOWLOG GET failed: "+err.Error())}
	}

	count := float64(len(entries))
	status := CheckOK
	msg := "no slow entries"
	if len(entries) > 0 {
		status = CheckWarning
		msg = fmt.Sprintf("%d slow entries", len(entries))
	}

	return []CheckResult{NewCheckResult(target.ID, "slowlog", status, &count, msg)}
}

func (rc *RedisChecker) checkClients(ctx context.Context, target DatabaseTarget, client RedisClient) []CheckResult {
	info, err := client.Info(ctx, "clients")
	if err != nil {
		return []CheckResult{NewCheckResult(target.ID, "clients", CheckCritical, nil, "INFO clients failed: "+err.Error())}
	}

	connected := parseInfoInt(info, "connected_clients")
	max := parseInfoInt(info, "maxclients")

	if max == 0 {
		v := float64(connected)
		return []CheckResult{NewCheckResult(target.ID, "clients", CheckOK, &v, "maxclients unknown")}
	}

	pct := float64(connected) / float64(max)
	status := CheckOK
	msg := fmt.Sprintf("%.1f%% of maxclients (%d/%d)", pct*100, connected, max)
	if pct > rc.thresholds.ClientPercent {
		status = CheckWarning
	}

	return []CheckResult{NewCheckResult(target.ID, "clients", status, &pct, msg)}
}

func (rc *RedisChecker) checkReplication(ctx context.Context, target DatabaseTarget, client RedisClient) []CheckResult {
	info, err := client.Info(ctx, "replication")
	if err != nil {
		return []CheckResult{NewCheckResult(target.ID, "replication", CheckCritical, nil, "INFO replication failed: "+err.Error())}
	}

	role := parseInfoString(info, "role")

	if role != "slave" {
		return []CheckResult{NewCheckResult(target.ID, "replication", CheckOK, nil, "role: "+role)}
	}

	linkStatus := parseInfoString(info, "master_link_status")
	if linkStatus == "down" {
		return []CheckResult{NewCheckResult(target.ID, "replication", CheckCritical, nil, "master_link_status: down")}
	}

	lagS := parseInfoInt(info, "master_last_io_seconds_ago")
	lag := float64(lagS)
	status := CheckOK
	msg := fmt.Sprintf("master_last_io_seconds_ago: %d", lagS)
	if lagS > int64(rc.thresholds.ReplicationLagS) {
		status = CheckWarning
		msg = fmt.Sprintf("replication lag %ds (threshold %ds)", lagS, rc.thresholds.ReplicationLagS)
	}

	return []CheckResult{NewCheckResult(target.ID, "replication", status, &lag, msg)}
}

// parseInfoInt extracts an integer value from Redis INFO output.
func parseInfoInt(info, key string) int64 {
	v := parseInfoString(info, key)
	if v == "" {
		return 0
	}
	n, _ := strconv.ParseInt(v, 10, 64)
	return n
}

// parseInfoString extracts a string value from Redis INFO output.
func parseInfoString(info, key string) string {
	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, key+":") {
			return strings.TrimPrefix(line, key+":")
		}
	}
	return ""
}
