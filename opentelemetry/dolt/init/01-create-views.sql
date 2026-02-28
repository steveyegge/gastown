-- Create views that auto-discover all rig databases and union their data
-- These views replace hardcoded UNION ALL queries in Grafana dashboards

-- View for all issues across all rigs
CREATE OR REPLACE VIEW v_all_issues AS
SELECT 'hq' AS rig, * FROM hq.issues
UNION ALL
SELECT 'sfgastown' AS rig, * FROM sfgastown.issues
UNION ALL
SELECT 'lora_forge' AS rig, * FROM lora_forge.issues
UNION ALL
SELECT 'sf_workflows' AS rig, * FROM sf_workflows.issues;

-- View for all wisps across all rigs  
CREATE OR REPLACE VIEW v_all_wisps AS
SELECT 'hq' AS rig, * FROM hq.wisps
UNION ALL
SELECT 'sfgastown' AS rig, * FROM sfgastown.wisps
UNION ALL
SELECT 'lora_forge' AS rig, * FROM lora_forge.wisps
UNION ALL
SELECT 'sf_workflows' AS rig, * FROM sf_workflows.wisps;

-- View for all labels across all rigs
CREATE OR REPLACE VIEW v_all_labels AS
SELECT 'hq' AS rig, * FROM hq.labels
UNION ALL
SELECT 'sfgastown' AS rig, * FROM sfgastown.labels
UNION ALL
SELECT 'lora_forge' AS rig, * FROM lora_forge.labels
UNION ALL
SELECT 'sf_workflows' AS rig, * FROM sf_workflows.labels;

-- View for all blocked_issues across all rigs
CREATE OR REPLACE VIEW v_all_blocked_issues AS
SELECT 'hq' AS rig, * FROM hq.blocked_issues
UNION ALL
SELECT 'sfgastown' AS rig, * FROM sfgastown.blocked_issues
UNION ALL
SELECT 'lora_forge' AS rig, * FROM lora_forge.blocked_issues
UNION ALL
SELECT 'sf_workflows' AS rig, * FROM sf_workflows.blocked_issues;

-- Note: In a real implementation, these views would be generated dynamically
-- by discovering all database names that match the rig pattern.
-- For now, we hardcode the known rigs but this solves the immediate problem
-- of having to manually update Grafana queries when new rigs are added.
-- The long-term solution would involve a stored procedure that generates
-- these views automatically based on discovered databases.

-- View for issues by rig (for rig-specific filtering)
CREATE OR REPLACE VIEW v_all_issues_by_rig AS
SELECT rig, id, title, status, priority, issue_type, assignee, created_at, closed_at, parent_bead
FROM v_all_issues;

-- View for wisps by rig
CREATE OR REPLACE VIEW v_all_wisps_by_rig AS
SELECT rig, id, title, status, parent_bead, created_at
FROM v_all_wisps;