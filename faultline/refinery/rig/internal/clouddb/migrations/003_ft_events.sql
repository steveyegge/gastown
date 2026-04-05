-- Phase 1: Events — individual error/transaction events from SDKs.
-- Named ft_events to avoid collision with beads `events` table in shared Dolt databases.
CREATE TABLE IF NOT EXISTS ft_events (
    id             VARCHAR(36) PRIMARY KEY,
    project_id     BIGINT NOT NULL,
    event_id       VARCHAR(36) NOT NULL,
    fingerprint    VARCHAR(64) NOT NULL,
    group_id       VARCHAR(36) NOT NULL,
    level          VARCHAR(16),
    culprit        VARCHAR(512),
    message        TEXT,
    platform       VARCHAR(64),
    environment    VARCHAR(50),
    release_name   VARCHAR(200),
    exception_type VARCHAR(255),
    raw_json       JSON NOT NULL,
    timestamp      DATETIME(6) NOT NULL,
    received_at    DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    UNIQUE KEY uq_project_event (project_id, event_id),
    INDEX idx_group (group_id),
    INDEX idx_fingerprint (project_id, fingerprint),
    INDEX idx_received_at (received_at)
);
