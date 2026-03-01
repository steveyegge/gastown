package config

import (
	"path/filepath"
	"time"
)

// Compiled-in defaults for operational thresholds.
// These are the values used when no config override is provided.
// Each was previously a hardcoded const scattered across the codebase.

// Session defaults.
const (
	DefaultClaudeStartTimeout      = 60 * time.Second
	DefaultShellReadyTimeout       = 5 * time.Second
	DefaultGracefulShutdownTimeout = 3 * time.Second
	DefaultBdCommandTimeout        = 30 * time.Second
	DefaultBdSubprocessTimeout     = 5 * time.Second
	DefaultGUPPViolationTimeout    = 30 * time.Minute
	DefaultHungSessionThreshold    = 30 * time.Minute
	DefaultStartupNudgeVerifyDelay = 5 * time.Second
	DefaultStartupNudgeMaxRetries  = 3
)

// Nudge defaults.
const (
	DefaultNudgeReadyTimeout      = 10 * time.Second
	DefaultNudgeRetryInterval     = 500 * time.Millisecond
	DefaultNudgeLockTimeout       = 30 * time.Second
	DefaultNudgeNormalTTL         = 30 * time.Minute
	DefaultNudgeUrgentTTL         = 2 * time.Hour
	DefaultNudgeMaxQueueDepth     = 50
	DefaultNudgeStaleClaimTimeout = 5 * time.Minute
)

// Daemon defaults.
const (
	DefaultMassDeathWindow                 = 30 * time.Second
	DefaultMassDeathThreshold              = 3
	DefaultDogIdleSessionTimeout           = 1 * time.Hour
	DefaultDogIdleRemoveTimeout            = 4 * time.Hour
	DefaultStaleWorkingTimeout             = 2 * time.Hour
	DefaultMaxDogPoolSize                  = 4
	DefaultMaxLifecycleMessageAge          = 6 * time.Hour
	DefaultSyncFailureEscalationThreshold  = 3
	DefaultDoctorMolCooldown               = 5 * time.Minute
)

// Deacon defaults.
const (
	DefaultDeaconPingTimeout               = 30 * time.Second
	DefaultDeaconConsecutiveFailures       = 3
	DefaultDeaconCooldown                  = 5 * time.Minute
	DefaultDeaconHeartbeatStaleThreshold   = 5 * time.Minute
	DefaultDeaconHeartbeatVeryStale        = 15 * time.Minute
	DefaultMaxRedispatches                 = 3
	DefaultRedispatchCooldown              = 5 * time.Minute
	DefaultMaxFeedsPerCycle                = 3
	DefaultFeedCooldown                    = 10 * time.Minute
)

// Polecat defaults.
const (
	DefaultPolecatHeartbeatStale = 3 * time.Minute
	DefaultPolecatDoltMaxRetries = 10
	DefaultPolecatDoltBaseBackoff = 500 * time.Millisecond
	DefaultPolecatDoltBackoffMax  = 30 * time.Second
	DefaultPolecatPendingMaxAge   = 5 * time.Minute
	DefaultPolecatNamepoolSize    = 50
)

// Dolt defaults.
const (
	DefaultDoltHealthCheckInterval = 30 * time.Second
	DefaultDoltCmdTimeout          = 15 * time.Second
	DefaultDoltMaxConnections      = 1000
	DefaultDoltSlowQueryThreshold  = 1 * time.Second
)

// Mail defaults.
const (
	DefaultMailIdleNotifyTimeout  = 3 * time.Second
	DefaultMailBdReadTimeout      = 60 * time.Second
	DefaultMailBdWriteTimeout     = 60 * time.Second
	DefaultMailMaxConcurrentAcks  = 8
)

// Web defaults.
const (
	DefaultWebMaxConcurrentCmds = 12
	DefaultWebMaxSubjectLen     = 500
	DefaultWebMaxBodyLen        = 100_000
)

// LoadOperationalConfig loads operational config from a town root.
// Returns a valid (possibly empty) config — never nil, never errors.
// Callers can use accessor methods that return defaults for nil sub-configs.
func LoadOperationalConfig(townRoot string) *OperationalConfig {
	settingsPath := filepath.Join(townRoot, "settings", "config.json")
	ts, err := LoadOrCreateTownSettings(settingsPath)
	if err != nil || ts == nil || ts.Operational == nil {
		return &OperationalConfig{}
	}
	return ts.Operational
}

// --- Accessor methods ---
// Each method reads from config with fallback to the compiled-in default.
// Nil-safe: works when OperationalConfig or any sub-struct is nil.

// GetSessionConfig returns the session thresholds, never nil.
func (c *OperationalConfig) GetSessionConfig() *SessionThresholds {
	if c != nil && c.Session != nil {
		return c.Session
	}
	return &SessionThresholds{}
}

// ClaudeStartTimeout returns the configured or default Claude start timeout.
func (s *SessionThresholds) ClaudeStartTimeoutD() time.Duration {
	if s != nil {
		return ParseDurationOrDefault(s.ClaudeStartTimeout, DefaultClaudeStartTimeout)
	}
	return DefaultClaudeStartTimeout
}

// ShellReadyTimeoutD returns the configured or default shell ready timeout.
func (s *SessionThresholds) ShellReadyTimeoutD() time.Duration {
	if s != nil {
		return ParseDurationOrDefault(s.ShellReadyTimeout, DefaultShellReadyTimeout)
	}
	return DefaultShellReadyTimeout
}

// GracefulShutdownTimeoutD returns the configured or default graceful shutdown timeout.
func (s *SessionThresholds) GracefulShutdownTimeoutD() time.Duration {
	if s != nil {
		return ParseDurationOrDefault(s.GracefulShutdownTimeout, DefaultGracefulShutdownTimeout)
	}
	return DefaultGracefulShutdownTimeout
}

// BdCommandTimeoutD returns the configured or default bd command timeout.
func (s *SessionThresholds) BdCommandTimeoutD() time.Duration {
	if s != nil {
		return ParseDurationOrDefault(s.BdCommandTimeout, DefaultBdCommandTimeout)
	}
	return DefaultBdCommandTimeout
}

// BdSubprocessTimeoutD returns the configured or default bd subprocess timeout.
func (s *SessionThresholds) BdSubprocessTimeoutD() time.Duration {
	if s != nil {
		return ParseDurationOrDefault(s.BdSubprocessTimeout, DefaultBdSubprocessTimeout)
	}
	return DefaultBdSubprocessTimeout
}

// GUPPViolationTimeoutD returns the configured or default GUPP violation timeout.
func (s *SessionThresholds) GUPPViolationTimeoutD() time.Duration {
	if s != nil {
		return ParseDurationOrDefault(s.GUPPViolationTimeout, DefaultGUPPViolationTimeout)
	}
	return DefaultGUPPViolationTimeout
}

// HungSessionThresholdD returns the configured or default hung session threshold.
func (s *SessionThresholds) HungSessionThresholdD() time.Duration {
	if s != nil {
		return ParseDurationOrDefault(s.HungSessionThreshold, DefaultHungSessionThreshold)
	}
	return DefaultHungSessionThreshold
}

// StartupNudgeVerifyDelayD returns the configured or default startup nudge verify delay.
func (s *SessionThresholds) StartupNudgeVerifyDelayD() time.Duration {
	if s != nil {
		return ParseDurationOrDefault(s.StartupNudgeVerifyDelay, DefaultStartupNudgeVerifyDelay)
	}
	return DefaultStartupNudgeVerifyDelay
}

// StartupNudgeMaxRetriesV returns the configured or default startup nudge max retries.
func (s *SessionThresholds) StartupNudgeMaxRetriesV() int {
	if s != nil && s.StartupNudgeMaxRetries != nil {
		return *s.StartupNudgeMaxRetries
	}
	return DefaultStartupNudgeMaxRetries
}

// --- Nudge accessors ---

// GetNudgeConfig returns the nudge thresholds, never nil.
func (c *OperationalConfig) GetNudgeConfig() *NudgeThresholds {
	if c != nil && c.Nudge != nil {
		return c.Nudge
	}
	return &NudgeThresholds{}
}

// ReadyTimeoutD returns the configured or default nudge ready timeout.
func (n *NudgeThresholds) ReadyTimeoutD() time.Duration {
	if n != nil {
		return ParseDurationOrDefault(n.ReadyTimeout, DefaultNudgeReadyTimeout)
	}
	return DefaultNudgeReadyTimeout
}

// RetryIntervalD returns the configured or default nudge retry interval.
func (n *NudgeThresholds) RetryIntervalD() time.Duration {
	if n != nil {
		return ParseDurationOrDefault(n.RetryInterval, DefaultNudgeRetryInterval)
	}
	return DefaultNudgeRetryInterval
}

// LockTimeoutD returns the configured or default nudge lock timeout.
func (n *NudgeThresholds) LockTimeoutD() time.Duration {
	if n != nil {
		return ParseDurationOrDefault(n.LockTimeout, DefaultNudgeLockTimeout)
	}
	return DefaultNudgeLockTimeout
}

// NormalTTLD returns the configured or default normal nudge TTL.
func (n *NudgeThresholds) NormalTTLD() time.Duration {
	if n != nil {
		return ParseDurationOrDefault(n.NormalTTL, DefaultNudgeNormalTTL)
	}
	return DefaultNudgeNormalTTL
}

// UrgentTTLD returns the configured or default urgent nudge TTL.
func (n *NudgeThresholds) UrgentTTLD() time.Duration {
	if n != nil {
		return ParseDurationOrDefault(n.UrgentTTL, DefaultNudgeUrgentTTL)
	}
	return DefaultNudgeUrgentTTL
}

// MaxQueueDepthV returns the configured or default max queue depth.
func (n *NudgeThresholds) MaxQueueDepthV() int {
	if n != nil && n.MaxQueueDepth != nil {
		return *n.MaxQueueDepth
	}
	return DefaultNudgeMaxQueueDepth
}

// StaleClaimThresholdD returns the configured or default stale claim threshold.
func (n *NudgeThresholds) StaleClaimThresholdD() time.Duration {
	if n != nil {
		return ParseDurationOrDefault(n.StaleClaimThreshold, DefaultNudgeStaleClaimTimeout)
	}
	return DefaultNudgeStaleClaimTimeout
}

// --- Daemon accessors ---

// GetDaemonConfig returns the daemon thresholds, never nil.
func (c *OperationalConfig) GetDaemonConfig() *DaemonThresholds {
	if c != nil && c.Daemon != nil {
		return c.Daemon
	}
	return &DaemonThresholds{}
}

// MassDeathWindowD returns the configured or default mass death window.
func (d *DaemonThresholds) MassDeathWindowD() time.Duration {
	if d != nil {
		return ParseDurationOrDefault(d.MassDeathWindow, DefaultMassDeathWindow)
	}
	return DefaultMassDeathWindow
}

// MassDeathThresholdV returns the configured or default mass death threshold.
func (d *DaemonThresholds) MassDeathThresholdV() int {
	if d != nil && d.MassDeathThreshold != nil {
		return *d.MassDeathThreshold
	}
	return DefaultMassDeathThreshold
}

// DogIdleSessionTimeoutD returns the configured or default dog idle session timeout.
func (d *DaemonThresholds) DogIdleSessionTimeoutD() time.Duration {
	if d != nil {
		return ParseDurationOrDefault(d.DogIdleSessionTimeout, DefaultDogIdleSessionTimeout)
	}
	return DefaultDogIdleSessionTimeout
}

// DogIdleRemoveTimeoutD returns the configured or default dog idle remove timeout.
func (d *DaemonThresholds) DogIdleRemoveTimeoutD() time.Duration {
	if d != nil {
		return ParseDurationOrDefault(d.DogIdleRemoveTimeout, DefaultDogIdleRemoveTimeout)
	}
	return DefaultDogIdleRemoveTimeout
}

// StaleWorkingTimeoutD returns the configured or default stale working timeout.
func (d *DaemonThresholds) StaleWorkingTimeoutD() time.Duration {
	if d != nil {
		return ParseDurationOrDefault(d.StaleWorkingTimeout, DefaultStaleWorkingTimeout)
	}
	return DefaultStaleWorkingTimeout
}

// MaxDogPoolSizeV returns the configured or default max dog pool size.
func (d *DaemonThresholds) MaxDogPoolSizeV() int {
	if d != nil && d.MaxDogPoolSize != nil {
		return *d.MaxDogPoolSize
	}
	return DefaultMaxDogPoolSize
}

// MaxLifecycleMessageAgeD returns the configured or default max lifecycle message age.
func (d *DaemonThresholds) MaxLifecycleMessageAgeD() time.Duration {
	if d != nil {
		return ParseDurationOrDefault(d.MaxLifecycleMessageAge, DefaultMaxLifecycleMessageAge)
	}
	return DefaultMaxLifecycleMessageAge
}

// SyncFailureEscalationThresholdV returns the configured or default threshold.
func (d *DaemonThresholds) SyncFailureEscalationThresholdV() int {
	if d != nil && d.SyncFailureEscalationThreshold != nil {
		return *d.SyncFailureEscalationThreshold
	}
	return DefaultSyncFailureEscalationThreshold
}

// DoctorMolCooldownD returns the configured or default doctor mol cooldown.
func (d *DaemonThresholds) DoctorMolCooldownD() time.Duration {
	if d != nil {
		return ParseDurationOrDefault(d.DoctorMolCooldown, DefaultDoctorMolCooldown)
	}
	return DefaultDoctorMolCooldown
}

// --- Deacon accessors ---

// GetDeaconConfig returns the deacon thresholds, never nil.
func (c *OperationalConfig) GetDeaconConfig() *DeaconThresholds {
	if c != nil && c.Deacon != nil {
		return c.Deacon
	}
	return &DeaconThresholds{}
}

// PingTimeoutD returns the configured or default deacon ping timeout.
func (d *DeaconThresholds) PingTimeoutD() time.Duration {
	if d != nil {
		return ParseDurationOrDefault(d.PingTimeout, DefaultDeaconPingTimeout)
	}
	return DefaultDeaconPingTimeout
}

// ConsecutiveFailuresV returns the configured or default consecutive failures.
func (d *DeaconThresholds) ConsecutiveFailuresV() int {
	if d != nil && d.ConsecutiveFailures != nil {
		return *d.ConsecutiveFailures
	}
	return DefaultDeaconConsecutiveFailures
}

// CooldownD returns the configured or default deacon cooldown.
func (d *DeaconThresholds) CooldownD() time.Duration {
	if d != nil {
		return ParseDurationOrDefault(d.Cooldown, DefaultDeaconCooldown)
	}
	return DefaultDeaconCooldown
}

// HeartbeatStaleThresholdD returns the configured or default heartbeat stale threshold.
func (d *DeaconThresholds) HeartbeatStaleThresholdD() time.Duration {
	if d != nil {
		return ParseDurationOrDefault(d.HeartbeatStaleThreshold, DefaultDeaconHeartbeatStaleThreshold)
	}
	return DefaultDeaconHeartbeatStaleThreshold
}

// HeartbeatVeryStaleThresholdD returns the configured or default heartbeat very stale threshold.
func (d *DeaconThresholds) HeartbeatVeryStaleThresholdD() time.Duration {
	if d != nil {
		return ParseDurationOrDefault(d.HeartbeatVeryStaleThreshold, DefaultDeaconHeartbeatVeryStale)
	}
	return DefaultDeaconHeartbeatVeryStale
}

// MaxRedispatchesV returns the configured or default max redispatches.
func (d *DeaconThresholds) MaxRedispatchesV() int {
	if d != nil && d.MaxRedispatches != nil {
		return *d.MaxRedispatches
	}
	return DefaultMaxRedispatches
}

// RedispatchCooldownD returns the configured or default redispatch cooldown.
func (d *DeaconThresholds) RedispatchCooldownD() time.Duration {
	if d != nil {
		return ParseDurationOrDefault(d.RedispatchCooldown, DefaultRedispatchCooldown)
	}
	return DefaultRedispatchCooldown
}

// MaxFeedsPerCycleV returns the configured or default max feeds per cycle.
func (d *DeaconThresholds) MaxFeedsPerCycleV() int {
	if d != nil && d.MaxFeedsPerCycle != nil {
		return *d.MaxFeedsPerCycle
	}
	return DefaultMaxFeedsPerCycle
}

// FeedCooldownD returns the configured or default feed cooldown.
func (d *DeaconThresholds) FeedCooldownD() time.Duration {
	if d != nil {
		return ParseDurationOrDefault(d.FeedCooldown, DefaultFeedCooldown)
	}
	return DefaultFeedCooldown
}

// --- Polecat accessors ---

// GetPolecatConfig returns the polecat thresholds, never nil.
func (c *OperationalConfig) GetPolecatConfig() *PolecatThresholds {
	if c != nil && c.Polecat != nil {
		return c.Polecat
	}
	return &PolecatThresholds{}
}

// HeartbeatStaleThresholdD returns the configured or default polecat heartbeat stale threshold.
func (p *PolecatThresholds) HeartbeatStaleThresholdD() time.Duration {
	if p != nil {
		return ParseDurationOrDefault(p.HeartbeatStaleThreshold, DefaultPolecatHeartbeatStale)
	}
	return DefaultPolecatHeartbeatStale
}

// DoltMaxRetriesV returns the configured or default Dolt max retries.
func (p *PolecatThresholds) DoltMaxRetriesV() int {
	if p != nil && p.DoltMaxRetries != nil {
		return *p.DoltMaxRetries
	}
	return DefaultPolecatDoltMaxRetries
}

// DoltBaseBackoffD returns the configured or default Dolt base backoff.
func (p *PolecatThresholds) DoltBaseBackoffD() time.Duration {
	if p != nil {
		return ParseDurationOrDefault(p.DoltBaseBackoff, DefaultPolecatDoltBaseBackoff)
	}
	return DefaultPolecatDoltBaseBackoff
}

// DoltBackoffMaxD returns the configured or default Dolt backoff max.
func (p *PolecatThresholds) DoltBackoffMaxD() time.Duration {
	if p != nil {
		return ParseDurationOrDefault(p.DoltBackoffMax, DefaultPolecatDoltBackoffMax)
	}
	return DefaultPolecatDoltBackoffMax
}

// PendingMaxAgeD returns the configured or default pending max age.
func (p *PolecatThresholds) PendingMaxAgeD() time.Duration {
	if p != nil {
		return ParseDurationOrDefault(p.PendingMaxAge, DefaultPolecatPendingMaxAge)
	}
	return DefaultPolecatPendingMaxAge
}

// NamepoolSizeV returns the configured or default namepool size.
func (p *PolecatThresholds) NamepoolSizeV() int {
	if p != nil && p.NamepoolSize != nil {
		return *p.NamepoolSize
	}
	return DefaultPolecatNamepoolSize
}

// --- Dolt accessors ---

// GetDoltConfig returns the dolt thresholds, never nil.
func (c *OperationalConfig) GetDoltConfig() *DoltThresholds {
	if c != nil && c.Dolt != nil {
		return c.Dolt
	}
	return &DoltThresholds{}
}

// HealthCheckIntervalD returns the configured or default health check interval.
func (dt *DoltThresholds) HealthCheckIntervalD() time.Duration {
	if dt != nil {
		return ParseDurationOrDefault(dt.HealthCheckInterval, DefaultDoltHealthCheckInterval)
	}
	return DefaultDoltHealthCheckInterval
}

// CmdTimeoutD returns the configured or default cmd timeout.
func (dt *DoltThresholds) CmdTimeoutD() time.Duration {
	if dt != nil {
		return ParseDurationOrDefault(dt.CmdTimeout, DefaultDoltCmdTimeout)
	}
	return DefaultDoltCmdTimeout
}

// MaxConnectionsV returns the configured or default max connections.
func (dt *DoltThresholds) MaxConnectionsV() int {
	if dt != nil && dt.MaxConnections != nil {
		return *dt.MaxConnections
	}
	return DefaultDoltMaxConnections
}

// SlowQueryThresholdD returns the configured or default slow query threshold.
func (dt *DoltThresholds) SlowQueryThresholdD() time.Duration {
	if dt != nil {
		return ParseDurationOrDefault(dt.SlowQueryThreshold, DefaultDoltSlowQueryThreshold)
	}
	return DefaultDoltSlowQueryThreshold
}

// --- Mail accessors ---

// GetMailConfig returns the mail thresholds, never nil.
func (c *OperationalConfig) GetMailConfig() *MailThresholds {
	if c != nil && c.Mail != nil {
		return c.Mail
	}
	return &MailThresholds{}
}

// IdleNotifyTimeoutD returns the configured or default idle notify timeout.
func (m *MailThresholds) IdleNotifyTimeoutD() time.Duration {
	if m != nil {
		return ParseDurationOrDefault(m.IdleNotifyTimeout, DefaultMailIdleNotifyTimeout)
	}
	return DefaultMailIdleNotifyTimeout
}

// BdReadTimeoutD returns the configured or default bd read timeout.
func (m *MailThresholds) BdReadTimeoutD() time.Duration {
	if m != nil {
		return ParseDurationOrDefault(m.BdReadTimeout, DefaultMailBdReadTimeout)
	}
	return DefaultMailBdReadTimeout
}

// BdWriteTimeoutD returns the configured or default bd write timeout.
func (m *MailThresholds) BdWriteTimeoutD() time.Duration {
	if m != nil {
		return ParseDurationOrDefault(m.BdWriteTimeout, DefaultMailBdWriteTimeout)
	}
	return DefaultMailBdWriteTimeout
}

// MaxConcurrentAckOpsV returns the configured or default max concurrent ack ops.
func (m *MailThresholds) MaxConcurrentAckOpsV() int {
	if m != nil && m.MaxConcurrentAckOps != nil {
		return *m.MaxConcurrentAckOps
	}
	return DefaultMailMaxConcurrentAcks
}

// --- Web accessors ---

// GetWebConfig returns the web thresholds, never nil.
func (c *OperationalConfig) GetWebConfig() *WebThresholds {
	if c != nil && c.Web != nil {
		return c.Web
	}
	return &WebThresholds{}
}

// MaxConcurrentCommandsV returns the configured or default max concurrent commands.
func (w *WebThresholds) MaxConcurrentCommandsV() int {
	if w != nil && w.MaxConcurrentCommands != nil {
		return *w.MaxConcurrentCommands
	}
	return DefaultWebMaxConcurrentCmds
}

// MaxSubjectLenV returns the configured or default max subject length.
func (w *WebThresholds) MaxSubjectLenV() int {
	if w != nil && w.MaxSubjectLen != nil {
		return *w.MaxSubjectLen
	}
	return DefaultWebMaxSubjectLen
}

// MaxBodyLenV returns the configured or default max body length.
func (w *WebThresholds) MaxBodyLenV() int {
	if w != nil && w.MaxBodyLen != nil {
		return *w.MaxBodyLen
	}
	return DefaultWebMaxBodyLen
}
