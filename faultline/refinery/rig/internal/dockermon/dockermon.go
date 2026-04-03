// Package dockermon monitors Docker containers for health check failures,
// resource pressure, and lifecycle events. Containers opt in via the
// faultline.monitor=true label. Two goroutines run concurrently: an event
// watcher (Docker /events stream) and a stats poller (configurable interval).
package dockermon

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/google/uuid"
)

// Status represents the aggregate health state of a monitored container.
type Status string

const (
	StatusHealthy  Status = "healthy"
	StatusDegraded Status = "degraded"
	StatusDown     Status = "down"
)

// CheckStatus represents the result of an individual check.
type CheckStatus string

const (
	CheckOK       CheckStatus = "ok"
	CheckWarning  CheckStatus = "warning"
	CheckCritical CheckStatus = "critical"
)

// Container is a discovered Docker container with the faultline.monitor=true label.
type Container struct {
	ID            string
	ProjectID     *int64
	ContainerID   string // Docker container ID
	ContainerName string
	ServiceName   string
	Image         string
	Enabled       bool
	Thresholds    sql.NullString // JSON overrides
	DiscoveredAt  time.Time
	LastSeenAt    time.Time
}

// CheckResult is the outcome of a single check against a container.
type CheckResult struct {
	ContainerID string // FK to Container.ID (our ID, not Docker's)
	ProjectID   *int64
	CheckType   string // health, memory, cpu, restart, stopped, disk
	Status      CheckStatus
	Value       *float64
	Message     string
	CheckedAt   time.Time
}

// MonitorState is the persisted state of a monitored container.
type MonitorState struct {
	ContainerID         string // FK to Container.ID
	Status              Status
	LastTransitionAt    *time.Time
	LastCheckAt         *time.Time
	ConsecutiveFailures int
}

// OnStateChangeFunc is called when a container transitions between health states.
type OnStateChangeFunc func(c Container, oldStatus, newStatus Status)

// Thresholds defines warning/critical thresholds for container checks.
type Thresholds struct {
	MemoryWarning    float64 // fraction, default 0.80
	MemoryCritical   float64 // fraction, default 0.95
	CPUWarning       float64 // throttle ratio, default 0.50
	CPUCritical      float64 // throttle ratio, default 0.80
	RestartWarning   int     // restarts in window, default 2
	RestartCritical  int     // restarts in window, default 5
	HealthFailWarning  int   // consecutive failures, default 1
	HealthFailCritical int   // consecutive failures, default 3
}

// DefaultThresholds returns the default threshold values from the design spec.
func DefaultThresholds() Thresholds {
	return Thresholds{
		MemoryWarning:      0.80,
		MemoryCritical:     0.95,
		CPUWarning:         0.50,
		CPUCritical:        0.80,
		RestartWarning:     2,
		RestartCritical:    5,
		HealthFailWarning:  1,
		HealthFailCritical: 3,
	}
}

// DockerClient abstracts the Docker daemon API for testability.
type DockerClient interface {
	ContainerList(ctx context.Context, options container.ListOptions) ([]types.Container, error)
	ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error)
	ContainerStatsOneShot(ctx context.Context, containerID string) (container.StatsResponseReader, error)
	Events(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error)
	Close() error
}

// DBProvider abstracts data access for the monitor.
type DBProvider interface {
	UpsertContainer(ctx context.Context, c *Container) error
	ListContainers(ctx context.Context) ([]Container, error)
	WriteCheckResults(ctx context.Context, results []CheckResult) error
	LoadMonitorState(ctx context.Context, containerID string) (*MonitorState, error)
	SaveMonitorState(ctx context.Context, state *MonitorState) error
	MarkContainerLastSeen(ctx context.Context, id string, at time.Time) error
}

// Monitor watches Docker containers for health and resource issues.
type Monitor struct {
	docker        DockerClient
	provider      DBProvider
	log           *slog.Logger
	pollInterval  time.Duration
	retryInterval time.Duration
	thresholds    Thresholds
	OnStateChange OnStateChangeFunc

	mu         sync.RWMutex
	containers map[string]*Container // Docker container ID → Container
	lastStatus map[string]Status     // our container ID → last known status
}

// New creates a Docker container monitor.
func New(docker DockerClient, provider DBProvider, log *slog.Logger) *Monitor {
	return &Monitor{
		docker:        docker,
		provider:      provider,
		log:           log,
		pollInterval:  30 * time.Second,
		retryInterval: 60 * time.Second,
		thresholds:    DefaultThresholds(),
		containers:    make(map[string]*Container),
		lastStatus:    make(map[string]Status),
	}
}

// SetPollInterval overrides the default 30s stats polling interval.
func (m *Monitor) SetPollInterval(d time.Duration) {
	m.pollInterval = d
}

// SetRetryInterval overrides the default 60s retry interval when the socket is unavailable.
func (m *Monitor) SetRetryInterval(d time.Duration) {
	m.retryInterval = d
}

// SetThresholds overrides the default thresholds.
func (m *Monitor) SetThresholds(t Thresholds) {
	m.thresholds = t
}

// Run discovers labeled containers, then starts the event watcher and stats
// poller as concurrent goroutines. Blocks until ctx is cancelled. If the Docker
// socket is unavailable, logs a warning and retries every retryInterval.
func (m *Monitor) Run(ctx context.Context) {
	for {
		err := m.discoverContainers(ctx)
		if err != nil {
			m.log.Warn("dockermon: socket unavailable, will retry",
				"err", err, "retry_in", m.retryInterval)
			select {
			case <-ctx.Done():
				return
			case <-time.After(m.retryInterval):
				continue
			}
		}
		break
	}

	m.mu.RLock()
	count := len(m.containers)
	m.mu.RUnlock()
	m.log.Info("dockermon: started", "containers", count, "poll_interval", m.pollInterval)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		m.watchEvents(ctx)
	}()
	go func() {
		defer wg.Done()
		m.pollStats(ctx)
	}()
	wg.Wait()
}

// discoverContainers lists running containers with faultline.monitor=true and
// registers them.
func (m *Monitor) discoverContainers(ctx context.Context) error {
	f := filters.NewArgs()
	f.Add("label", "faultline.monitor=true")
	f.Add("status", "running")

	list, err := m.docker.ContainerList(ctx, container.ListOptions{Filters: f})
	if err != nil {
		return fmt.Errorf("container list: %w", err)
	}

	now := time.Now().UTC()
	for _, dc := range list {
		c := containerFromDocker(dc, now)
		if err := m.provider.UpsertContainer(ctx, c); err != nil {
			m.log.Error("dockermon: upsert container", "err", err, "container", c.ContainerName)
			continue
		}
		m.mu.Lock()
		m.containers[dc.ID] = c
		m.mu.Unlock()
	}
	return nil
}

// watchEvents subscribes to Docker daemon events and handles container
// lifecycle changes (start, die, oom, stop, health_status).
func (m *Monitor) watchEvents(ctx context.Context) {
	for {
		if err := m.streamEvents(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			m.log.Warn("dockermon: event stream error, reconnecting",
				"err", err, "retry_in", m.retryInterval)
			select {
			case <-ctx.Done():
				return
			case <-time.After(m.retryInterval):
				continue
			}
		}
	}
}

func (m *Monitor) streamEvents(ctx context.Context) error {
	f := filters.NewArgs()
	f.Add("type", string(events.ContainerEventType))

	msgCh, errCh := m.docker.Events(ctx, events.ListOptions{Filters: f})

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errCh:
			if err == io.EOF || err == nil {
				return nil
			}
			return err
		case msg := <-msgCh:
			m.handleEvent(ctx, msg)
		}
	}
}

func (m *Monitor) handleEvent(ctx context.Context, msg events.Message) {
	labels := msg.Actor.Attributes
	if labels["faultline.monitor"] != "true" {
		return
	}

	dockerID := msg.Actor.ID
	now := time.Now().UTC()

	switch msg.Action {
	case events.ActionStart:
		m.handleContainerStart(ctx, dockerID, now)

	case events.ActionDie:
		exitCode := labels["exitCode"]
		if exitCode != "0" {
			m.handleContainerDie(ctx, dockerID, exitCode, now)
		}

	case events.ActionOOM:
		m.handleContainerOOM(ctx, dockerID, now)

	case events.ActionStop:
		m.handleContainerStop(ctx, dockerID, now)

	default:
		if msg.Action == "health_status: unhealthy" {
			m.handleHealthUnhealthy(ctx, dockerID, now)
		}
	}
}

func (m *Monitor) handleContainerStart(ctx context.Context, dockerID string, now time.Time) {
	inspect, err := m.docker.ContainerInspect(ctx, dockerID)
	if err != nil {
		m.log.Error("dockermon: inspect on start", "err", err, "docker_id", dockerID)
		return
	}
	c := containerFromInspect(inspect, now)
	if err := m.provider.UpsertContainer(ctx, c); err != nil {
		m.log.Error("dockermon: upsert on start", "err", err)
		return
	}
	m.mu.Lock()
	m.containers[dockerID] = c
	m.mu.Unlock()
	m.log.Info("dockermon: container started", "name", c.ContainerName)
}

func (m *Monitor) handleContainerDie(ctx context.Context, dockerID, exitCode string, now time.Time) {
	c := m.getContainer(dockerID)
	if c == nil {
		return
	}
	result := CheckResult{
		ContainerID: c.ID,
		ProjectID:   c.ProjectID,
		CheckType:   "stopped",
		Status:      CheckCritical,
		Message:     fmt.Sprintf("container died with exit code %s", exitCode),
		CheckedAt:   now,
	}
	m.processResults(ctx, *c, []CheckResult{result})
}

func (m *Monitor) handleContainerOOM(ctx context.Context, dockerID string, now time.Time) {
	c := m.getContainer(dockerID)
	if c == nil {
		return
	}
	result := CheckResult{
		ContainerID: c.ID,
		ProjectID:   c.ProjectID,
		CheckType:   "memory",
		Status:      CheckCritical,
		Message:     fmt.Sprintf("Container OOMKilled: %s", c.ServiceName),
		CheckedAt:   now,
	}
	m.processResults(ctx, *c, []CheckResult{result})
}

func (m *Monitor) handleContainerStop(ctx context.Context, dockerID string, now time.Time) {
	m.mu.Lock()
	delete(m.containers, dockerID)
	m.mu.Unlock()

	c := m.getContainer(dockerID)
	if c != nil {
		_ = m.provider.MarkContainerLastSeen(ctx, c.ID, now)
	}
}

func (m *Monitor) handleHealthUnhealthy(ctx context.Context, dockerID string, now time.Time) {
	c := m.getContainer(dockerID)
	if c == nil {
		return
	}

	state, err := m.provider.LoadMonitorState(ctx, c.ID)
	if err != nil {
		state = &MonitorState{ContainerID: c.ID, Status: StatusHealthy}
	}
	state.ConsecutiveFailures++

	status := CheckOK
	msg := fmt.Sprintf("health check failure %d", state.ConsecutiveFailures)
	if state.ConsecutiveFailures >= m.thresholds.HealthFailCritical {
		status = CheckCritical
	} else if state.ConsecutiveFailures >= m.thresholds.HealthFailWarning {
		status = CheckWarning
	}

	result := CheckResult{
		ContainerID: c.ID,
		ProjectID:   c.ProjectID,
		CheckType:   "health",
		Status:      status,
		Message:     msg,
		CheckedAt:   now,
	}
	m.processResults(ctx, *c, []CheckResult{result})
}

// pollStats periodically collects memory and CPU stats from all tracked containers.
func (m *Monitor) pollStats(ctx context.Context) {
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.pollOnce(ctx)
		}
	}
}

func (m *Monitor) pollOnce(ctx context.Context) {
	m.mu.RLock()
	snapshot := make([]*Container, 0, len(m.containers))
	for _, c := range m.containers {
		snapshot = append(snapshot, c)
	}
	m.mu.RUnlock()

	for _, c := range snapshot {
		results := m.checkContainer(ctx, c)
		if len(results) > 0 {
			m.processResults(ctx, *c, results)
		}
	}
}

func (m *Monitor) checkContainer(ctx context.Context, c *Container) []CheckResult {
	statsReader, err := m.docker.ContainerStatsOneShot(ctx, c.ContainerID)
	if err != nil {
		return []CheckResult{{
			ContainerID: c.ID,
			ProjectID:   c.ProjectID,
			CheckType:   "connection",
			Status:      CheckCritical,
			Message:     fmt.Sprintf("stats unavailable: %v", err),
			CheckedAt:   time.Now().UTC(),
		}}
	}
	defer statsReader.Body.Close()

	stats, err := decodeStats(statsReader.Body)
	if err != nil {
		return []CheckResult{{
			ContainerID: c.ID,
			ProjectID:   c.ProjectID,
			CheckType:   "connection",
			Status:      CheckCritical,
			Message:     fmt.Sprintf("stats decode error: %v", err),
			CheckedAt:   time.Now().UTC(),
		}}
	}

	now := time.Now().UTC()
	_ = m.provider.MarkContainerLastSeen(ctx, c.ID, now)

	var results []CheckResult

	// Memory check.
	if stats.MemoryLimit > 0 {
		memPct := float64(stats.MemoryUsage) / float64(stats.MemoryLimit)
		memResult := CheckResult{
			ContainerID: c.ID,
			ProjectID:   c.ProjectID,
			CheckType:   "memory",
			Value:       &memPct,
			CheckedAt:   now,
		}
		switch {
		case memPct >= m.thresholds.MemoryCritical:
			memResult.Status = CheckCritical
			memResult.Message = fmt.Sprintf("memory at %.0f%% of limit", memPct*100)
		case memPct >= m.thresholds.MemoryWarning:
			memResult.Status = CheckWarning
			memResult.Message = fmt.Sprintf("memory at %.0f%% of limit", memPct*100)
		default:
			memResult.Status = CheckOK
		}
		results = append(results, memResult)
	}

	// CPU throttle check.
	if stats.ThrottledPeriods+stats.TotalPeriods > 0 {
		throttleRatio := float64(stats.ThrottledPeriods) / float64(stats.TotalPeriods)
		cpuResult := CheckResult{
			ContainerID: c.ID,
			ProjectID:   c.ProjectID,
			CheckType:   "cpu",
			Value:       &throttleRatio,
			CheckedAt:   now,
		}
		switch {
		case throttleRatio >= m.thresholds.CPUCritical:
			cpuResult.Status = CheckCritical
			cpuResult.Message = fmt.Sprintf("%.0f%% of CPU periods throttled", throttleRatio*100)
		case throttleRatio >= m.thresholds.CPUWarning:
			cpuResult.Status = CheckWarning
			cpuResult.Message = fmt.Sprintf("%.0f%% of CPU periods throttled", throttleRatio*100)
		default:
			cpuResult.Status = CheckOK
		}
		results = append(results, cpuResult)
	}

	return results
}

// processResults persists check results and evaluates state transitions.
func (m *Monitor) processResults(ctx context.Context, c Container, results []CheckResult) {
	if len(results) == 0 {
		return
	}

	for i := range results {
		if results[i].ContainerID == "" {
			results[i].ContainerID = c.ID
		}
		if results[i].ProjectID == nil {
			results[i].ProjectID = c.ProjectID
		}
		if results[i].CheckedAt.IsZero() {
			results[i].CheckedAt = time.Now().UTC()
		}
	}

	if err := m.provider.WriteCheckResults(ctx, results); err != nil {
		m.log.Error("dockermon: write check results", "err", err, "container", c.ContainerName)
	}

	newStatus := evaluateStatus(results)

	state, err := m.provider.LoadMonitorState(ctx, c.ID)
	if err != nil {
		m.log.Error("dockermon: load state", "err", err, "container", c.ContainerName)
		state = &MonitorState{ContainerID: c.ID, Status: StatusHealthy}
	}

	m.mu.RLock()
	oldStatus, known := m.lastStatus[c.ID]
	m.mu.RUnlock()
	if !known {
		oldStatus = state.Status
	}

	now := time.Now().UTC()
	state.LastCheckAt = &now

	if newStatus == StatusDown {
		state.ConsecutiveFailures++
	} else {
		state.ConsecutiveFailures = 0
	}

	if newStatus != oldStatus {
		state.Status = newStatus
		state.LastTransitionAt = &now
		m.mu.Lock()
		m.lastStatus[c.ID] = newStatus
		m.mu.Unlock()

		m.log.Info("dockermon: state change",
			"container", c.ContainerName,
			"old_status", string(oldStatus),
			"new_status", string(newStatus),
		)

		if m.OnStateChange != nil {
			m.OnStateChange(c, oldStatus, newStatus)
		}
	} else {
		m.mu.Lock()
		m.lastStatus[c.ID] = newStatus
		m.mu.Unlock()
		state.Status = newStatus
	}

	if err := m.provider.SaveMonitorState(ctx, state); err != nil {
		m.log.Error("dockermon: save state", "err", err, "container", c.ContainerName)
	}
}

// evaluateStatus derives aggregate status from individual check results.
func evaluateStatus(results []CheckResult) Status {
	for _, r := range results {
		if r.Status == CheckCritical {
			return StatusDown
		}
	}
	for _, r := range results {
		if r.Status == CheckWarning {
			return StatusDegraded
		}
	}
	return StatusHealthy
}

// Fingerprint generates a stable fingerprint for issue deduplication.
// Uses container name (stable across restarts) rather than container ID.
func Fingerprint(containerName, checkType string) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("dockermon|%s|%s", containerName, checkType)))
	return fmt.Sprintf("%x", h[:])
}

// NewCheckResult is a convenience constructor.
func NewCheckResult(containerID, checkType string, status CheckStatus, value *float64, message string) CheckResult {
	return CheckResult{
		ContainerID: containerID,
		CheckType:   checkType,
		Status:      status,
		Value:       value,
		Message:     message,
		CheckedAt:   time.Now().UTC(),
	}
}

func (m *Monitor) getContainer(dockerID string) *Container {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.containers[dockerID]
}

func containerFromDocker(dc types.Container, now time.Time) *Container {
	name := ""
	if len(dc.Names) > 0 {
		name = dc.Names[0]
		if len(name) > 0 && name[0] == '/' {
			name = name[1:]
		}
	}

	var projectID *int64
	// faultline.project label is resolved to a project_id at a higher layer.

	return &Container{
		ID:            uuid.New().String(),
		ProjectID:     projectID,
		ContainerID:   dc.ID,
		ContainerName: name,
		ServiceName:   dc.Labels["com.docker.compose.service"],
		Image:         dc.Image,
		Enabled:       true,
		DiscoveredAt:  now,
		LastSeenAt:    now,
	}
}

func containerFromInspect(info types.ContainerJSON, now time.Time) *Container {
	name := info.Name
	if len(name) > 0 && name[0] == '/' {
		name = name[1:]
	}

	return &Container{
		ID:            uuid.New().String(),
		ContainerID:   info.ID,
		ContainerName: name,
		ServiceName:   info.Config.Labels["com.docker.compose.service"],
		Image:         info.Config.Image,
		Enabled:       true,
		DiscoveredAt:  now,
		LastSeenAt:    now,
	}
}
