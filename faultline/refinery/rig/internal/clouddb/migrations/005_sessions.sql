-- Phase 1: Sessions — Sentry session tracking.
CREATE TABLE IF NOT EXISTS sessions (
    session_id   VARCHAR(36) PRIMARY KEY,
    project_id   BIGINT NOT NULL,
    distinct_id  VARCHAR(512),
    status       VARCHAR(16) NOT NULL DEFAULT 'ok',
    errors       INT NOT NULL DEFAULT 0,
    started      DATETIME(6) NOT NULL,
    duration     DOUBLE,
    release_name VARCHAR(256),
    environment  VARCHAR(64),
    user_agent   VARCHAR(512),
    updated_at   DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    INDEX idx_project (project_id),
    INDEX idx_status (status)
);
