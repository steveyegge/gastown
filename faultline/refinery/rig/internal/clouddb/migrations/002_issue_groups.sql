-- Phase 1: Issue groups — one per unique fingerprint per project.
CREATE TABLE IF NOT EXISTS issue_groups (
    id               VARCHAR(36) PRIMARY KEY,
    project_id       BIGINT NOT NULL,
    fingerprint      VARCHAR(64) NOT NULL,
    title            VARCHAR(512) NOT NULL,
    culprit          VARCHAR(512),
    level            VARCHAR(16),
    platform         VARCHAR(64),
    status           VARCHAR(16) NOT NULL DEFAULT 'unresolved',
    first_seen       DATETIME(6) NOT NULL,
    last_seen        DATETIME(6) NOT NULL,
    event_count      INT NOT NULL DEFAULT 1,
    bead_id          VARCHAR(64),
    resolved_at      DATETIME(6),
    regressed_at     DATETIME(6),
    regression_count INT NOT NULL DEFAULT 0,
    UNIQUE KEY uq_project_fingerprint (project_id, fingerprint),
    INDEX idx_project (project_id),
    INDEX idx_status (status)
);
