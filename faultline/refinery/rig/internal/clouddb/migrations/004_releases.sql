-- Phase 1: Releases — aggregated release data from events.
CREATE TABLE IF NOT EXISTS releases (
    project_id      BIGINT NOT NULL,
    version         VARCHAR(200) NOT NULL,
    first_seen      DATETIME(6) NOT NULL,
    last_seen       DATETIME(6) NOT NULL,
    event_count     INT NOT NULL DEFAULT 0,
    session_count   INT NOT NULL DEFAULT 0,
    crash_free_rate DOUBLE NOT NULL DEFAULT 1.0,
    UNIQUE KEY uq_project_version (project_id, version),
    INDEX idx_project_lastseen (project_id, last_seen)
);
