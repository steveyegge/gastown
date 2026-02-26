#!/usr/bin/env bash
# Gas Town — OpenTelemetry environment setup
#
# Source this file to enable full telemetry (gt + bd + Claude Code):
#
#   source opentelemetry/setup.sh
#
# Or add to ~/.zshrc / ~/.bashrc for persistent activation.

# ── Endpoints ──────────────────────────────────────────────────────────────
export GT_OTEL_METRICS_URL=http://localhost:8428/opentelemetry/api/v1/push
export GT_OTEL_LOGS_URL=http://localhost:9428/insert/opentelemetry/v1/logs

# ── bd telemetry ───────────────────────────────────────────────────────────
# bd uses its own var names; mirror the same endpoints.
export BD_OTEL_METRICS_URL="$GT_OTEL_METRICS_URL"
export BD_OTEL_LOGS_URL="$GT_OTEL_LOGS_URL"

# ── Claude Code telemetry ──────────────────────────────────────────────────
# Enables Claude Code's built-in OTLP metrics + logs export.
# gt injects these automatically for agent sessions, but they're also
# useful when running `claude` directly in a terminal.
export CLAUDE_CODE_ENABLE_TELEMETRY=1
# Metrics → VictoriaMetrics
export OTEL_METRICS_EXPORTER=otlp
export OTEL_METRIC_EXPORT_INTERVAL=1000
export OTEL_EXPORTER_OTLP_METRICS_ENDPOINT="$GT_OTEL_METRICS_URL"
# VictoriaMetrics and VictoriaLogs both require protobuf (reject JSON).
export OTEL_EXPORTER_OTLP_METRICS_PROTOCOL=http/protobuf
# Logs → VictoriaLogs
export OTEL_LOGS_EXPORTER=otlp
export OTEL_EXPORTER_OTLP_LOGS_ENDPOINT="$GT_OTEL_LOGS_URL"
export OTEL_EXPORTER_OTLP_LOGS_PROTOCOL=http/protobuf
# Log tool usage (which tools ran and their status).
export OTEL_LOG_TOOL_DETAILS=true
# Log tool output content (e.g. gt prime stdout as received by Claude).
export OTEL_LOG_TOOL_CONTENT=true
# Log user-turn messages (initial beacon + any human prompts to Claude).
export OTEL_LOG_USER_PROMPTS=true

echo "✓ Gas Town telemetry enabled"
echo ""
echo "  Push endpoints:"
echo "    Metrics → $GT_OTEL_METRICS_URL"
echo "    Logs    → $GT_OTEL_LOGS_URL"
echo ""
echo "  Query UIs:"
echo "    VictoriaMetrics → http://localhost:8428/vmui/#/?query=gastown_bd_calls_total"
echo "    VictoriaLogs    → http://localhost:9428/select/vmui/#/?query=service_name%3Agastown&view=liveTailing"
echo "    Grafana         → http://localhost:9429"
echo ""
echo "  Verify with:  gt status   (triggers bd calls → metrics + logs)"
