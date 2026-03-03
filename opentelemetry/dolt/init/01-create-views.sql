-- Cross-rig views for Grafana dashboards.
-- Runs automatically on `gt dolt start` (see doltserver.go:runInitSQL).
-- Dolt does not support mixing named columns with *, so we list columns explicitly.

-- Core view: all issues across rigs (columns used by dashboard panels)
CREATE OR REPLACE VIEW v_all_issues AS
SELECT 'hq' AS rig, id, title, status, priority, issue_type, assignee, created_at, closed_at FROM hq.issues
UNION ALL
SELECT 'st' AS rig, id, title, status, priority, issue_type, assignee, created_at, closed_at FROM st.issues
UNION ALL
SELECT 'lora_forge' AS rig, id, title, status, priority, issue_type, assignee, created_at, closed_at FROM lora_forge.issues;

-- Core view: all wisps across rigs
CREATE OR REPLACE VIEW v_all_wisps AS
SELECT 'hq' AS rig, id, title, status, created_at FROM hq.wisps
UNION ALL
SELECT 'st' AS rig, id, title, status, created_at FROM st.wisps
UNION ALL
SELECT 'lora_forge' AS rig, id, title, status, created_at FROM lora_forge.wisps;

-- Labels across rigs
CREATE OR REPLACE VIEW v_all_labels AS
SELECT 'hq' AS rig, issue_id, label FROM hq.labels
UNION ALL
SELECT 'st' AS rig, issue_id, label FROM st.labels
UNION ALL
SELECT 'lora_forge' AS rig, issue_id, label FROM lora_forge.labels;
