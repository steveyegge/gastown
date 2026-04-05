-- Phase 1: Projects table — maps Sentry DSN keys to project metadata.
CREATE TABLE IF NOT EXISTS projects (
    id             BIGINT AUTO_INCREMENT PRIMARY KEY,
    name           VARCHAR(200) NOT NULL,
    slug           VARCHAR(100) UNIQUE NOT NULL,
    dsn_public_key VARCHAR(32) UNIQUE NOT NULL,
    created_at     DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6)
);
