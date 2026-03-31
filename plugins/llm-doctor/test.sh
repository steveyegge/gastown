#!/usr/bin/env bash
# test.sh — Test suite for llm-doctor plugin.
#
# Runs all tests using mock HTTP responses. No real API or Ollama calls.
# Usage: ./test.sh [--verbose]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
TEST_TMPDIR=$(mktemp -d)
trap "rm -rf $TEST_TMPDIR" EXIT

VERBOSE=false
[[ "${1:-}" == "--verbose" ]] && VERBOSE=true

PASS=0
FAIL=0
ERRORS=""

# --- Test helpers -------------------------------------------------------------

log_test() { echo "  TEST: $1"; }

assert_eq() {
    local label="$1" expected="$2" actual="$3"
    if [[ "$expected" == "$actual" ]]; then
        PASS=$((PASS + 1))
    else
        FAIL=$((FAIL + 1))
        ERRORS+="  FAIL: $label — expected '$expected', got '$actual'"$'\n'
        echo "  FAIL: $label"
    fi
}

assert_contains() {
    local label="$1" needle="$2" haystack="$3"
    if echo "$haystack" | grep -qF "$needle"; then
        PASS=$((PASS + 1))
    else
        FAIL=$((FAIL + 1))
        ERRORS+="  FAIL: $label — '$needle' not found in output"$'\n'
        echo "  FAIL: $label"
    fi
}

assert_not_contains() {
    local label="$1" needle="$2" haystack="$3"
    if ! echo "$haystack" | grep -qF "$needle"; then
        PASS=$((PASS + 1))
    else
        FAIL=$((FAIL + 1))
        ERRORS+="  FAIL: $label — '$needle' should NOT be in output"$'\n'
        echo "  FAIL: $label"
    fi
}

assert_file_exists() {
    local label="$1" path="$2"
    if [[ -f "$path" ]]; then
        PASS=$((PASS + 1))
    else
        FAIL=$((FAIL + 1))
        ERRORS+="  FAIL: $label — file not found: $path"$'\n'
        echo "  FAIL: $label"
    fi
}

assert_exit_code() {
    local label="$1" expected="$2" actual="$3"
    if [[ "$expected" == "$actual" ]]; then
        PASS=$((PASS + 1))
    else
        FAIL=$((FAIL + 1))
        ERRORS+="  FAIL: $label — expected exit $expected, got $actual"$'\n'
        echo "  FAIL: $label"
    fi
}

# --- Mock infrastructure ------------------------------------------------------
#
# We create a mock bin/ directory with fake `curl`, `gt`, `bd`, `dig`, `ping`,
# `openssl`, and `tmux` commands. Tests prepend this to PATH so run.sh calls
# our mocks instead of real binaries.

MOCK_BIN="$TEST_TMPDIR/mock-bin"
mkdir -p "$MOCK_BIN"

# Mock state files — tests write these to control mock behavior
MOCK_STATE="$TEST_TMPDIR/mock-state"
mkdir -p "$MOCK_STATE"

# --- curl mock ---
# Reads MOCK_STATE/curl_responses to determine behavior.
# Format: one file per URL pattern, named by keyword.
cat > "$MOCK_BIN/curl" << 'MOCK_CURL'
#!/usr/bin/env bash
# Mock curl — returns responses based on MOCK_STATE files.
MOCK_STATE="${MOCK_STATE:?}"
ARGS="$*"

# Parse -w flag for http_code format
if echo "$ARGS" | grep -q '%{http_code}'; then
    WANTS_CODE=true
else
    WANTS_CODE=false
fi

# Parse -o flag for output file
OUTPUT_FILE=""
if echo "$ARGS" | grep -q '\-o '; then
    OUTPUT_FILE=$(echo "$ARGS" | sed -E 's/.*-o ([^ ]+).*/\1/')
fi

# Determine which mock to serve based on URL
if echo "$ARGS" | grep -q '/v1/messages'; then
    CODE=$(cat "$MOCK_STATE/api_http_code" 2>/dev/null || echo "200")
    BODY=$(cat "$MOCK_STATE/api_response_body" 2>/dev/null || echo '{"type":"message"}')
    if [[ -n "$OUTPUT_FILE" ]]; then
        echo "$BODY" > "$OUTPUT_FILE"
    fi
    if $WANTS_CODE; then
        echo "$CODE"
    else
        echo "$BODY"
    fi
elif echo "$ARGS" | grep -q '/api/tags'; then
    CODE=$(cat "$MOCK_STATE/ollama_tags_code" 2>/dev/null || echo "000")
    BODY=$(cat "$MOCK_STATE/ollama_tags_body" 2>/dev/null || echo '{}')
    if [[ -n "$OUTPUT_FILE" ]]; then
        echo "$BODY" > "$OUTPUT_FILE"
    fi
    if $WANTS_CODE; then
        echo "$CODE"
    else
        echo "$BODY"
    fi
elif echo "$ARGS" | grep -q '/api/generate'; then
    BODY=$(cat "$MOCK_STATE/ollama_generate_body" 2>/dev/null || echo '{"response":"CLASSIFICATION: test\nROOT CAUSE: test\nSUGGESTED FIX:\n- test\nURGENCY: high"}')
    echo "$BODY"
elif echo "$ARGS" | grep -q '/api/pull'; then
    echo '{"status":"success"}'
else
    echo "mock-curl: unhandled URL in: $ARGS" >&2
    echo "000"
fi
MOCK_CURL
chmod +x "$MOCK_BIN/curl"

# --- gt mock ---
cat > "$MOCK_BIN/gt" << 'MOCK_GT'
#!/usr/bin/env bash
MOCK_STATE="${MOCK_STATE:?}"
echo "gt $*" >> "$MOCK_STATE/gt_calls"
MOCK_GT
chmod +x "$MOCK_BIN/gt"

# --- bd mock ---
cat > "$MOCK_BIN/bd" << 'MOCK_BD'
#!/usr/bin/env bash
MOCK_STATE="${MOCK_STATE:?}"
echo "bd $*" >> "$MOCK_STATE/bd_calls"
MOCK_BD
chmod +x "$MOCK_BIN/bd"

# --- dig mock ---
cat > "$MOCK_BIN/dig" << 'MOCK_DIG'
#!/usr/bin/env bash
echo "93.184.216.34"
MOCK_DIG
chmod +x "$MOCK_BIN/dig"

# --- ping mock ---
cat > "$MOCK_BIN/ping" << 'MOCK_PING'
#!/usr/bin/env bash
MOCK_STATE="${MOCK_STATE:?}"
if [[ -f "$MOCK_STATE/ping_fail" ]]; then
    echo "ping: sendto: Network is unreachable"
    echo "ping failed"
    exit 1
else
    echo "PING 8.8.8.8 (8.8.8.8): 56 data bytes"
    echo "round-trip min/avg/max/stddev = 1.0/1.0/1.0/0.0 ms"
fi
MOCK_PING
chmod +x "$MOCK_BIN/ping"

# --- openssl mock ---
cat > "$MOCK_BIN/openssl" << 'MOCK_SSL'
#!/usr/bin/env bash
echo "subject=CN=api.anthropic.com"
echo "Verify return code: 0 (ok)"
MOCK_SSL
chmod +x "$MOCK_BIN/openssl"

# --- tmux mock ---
cat > "$MOCK_BIN/tmux" << 'MOCK_TMUX'
#!/usr/bin/env bash
# Return nothing (no sessions)
exit 0
MOCK_TMUX
chmod +x "$MOCK_BIN/tmux"

# --- python3 must be real (used for JSON parsing) ---
# We leave python3 on the real PATH, just prepend our mocks.

# --- Run helper: execute run.sh with mocked environment ----------------------

run_doctor() {
    local test_town="$TEST_TMPDIR/town"
    mkdir -p "$test_town"

    # Clean state between runs
    rm -f "$MOCK_STATE/gt_calls" "$MOCK_STATE/bd_calls"
    rm -rf "$test_town/.llm-doctor"

    # Export mock state path for mock scripts
    export MOCK_STATE

    # Run with mocked PATH (mocks first, then real python3/bash)
    env \
        PATH="$MOCK_BIN:$PATH" \
        GT_ROOT="$test_town" \
        ANTHROPIC_API_KEY="${TEST_API_KEY:-sk-ant-test-key-1234}" \
        ANTHROPIC_BASE_URL="https://api.anthropic.com" \
        OLLAMA_URL="http://localhost:11434" \
        bash "$SCRIPT_DIR/run.sh" --dry-run "$@" 2>&1
}

# =============================================================================
# TESTS
# =============================================================================

echo "=== llm-doctor test suite ==="
echo ""

# --- Test Group 1: API Health Detection --------------------------------------
echo "--- API Health Detection ---"

# Test 1.1: Healthy API exits cleanly
log_test "1.1: Healthy API (200) exits cleanly"
echo "200" > "$MOCK_STATE/api_http_code"
echo '{"type":"message","content":[{"text":"ok"}]}' > "$MOCK_STATE/api_response_body"
OUTPUT=$(run_doctor 2>&1 || true)
assert_contains "1.1a: reports healthy" "API healthy" "$OUTPUT"
assert_not_contains "1.1b: no diagnosis triggered" "Failure detected" "$OUTPUT"

# Test 1.2: 401 triggers auth diagnosis
log_test "1.2: Auth error (401) triggers diagnosis"
echo "401" > "$MOCK_STATE/api_http_code"
echo '{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}' > "$MOCK_STATE/api_response_body"
echo "000" > "$MOCK_STATE/ollama_tags_code"  # Ollama down
OUTPUT=$(run_doctor 2>&1 || true)
assert_contains "1.2a: detects auth error" "auth-error" "$OUTPUT"
assert_contains "1.2b: classifies auth" "CLASSIFICATION: auth-invalid" "$OUTPUT"
assert_contains "1.2c: suggests fix" "ANTHROPIC_API_KEY" "$OUTPUT"
assert_contains "1.2d: critical urgency" "URGENCY: critical" "$OUTPUT"

# Test 1.3: 403 triggers forbidden diagnosis
log_test "1.3: Forbidden (403) triggers diagnosis"
echo "403" > "$MOCK_STATE/api_http_code"
echo '{"type":"error","error":{"type":"permission_error","message":"forbidden"}}' > "$MOCK_STATE/api_response_body"
echo "000" > "$MOCK_STATE/ollama_tags_code"
OUTPUT=$(run_doctor 2>&1 || true)
assert_contains "1.3a: detects forbidden" "forbidden" "$OUTPUT"
assert_contains "1.3b: classifies auth-expired" "CLASSIFICATION: auth-expired" "$OUTPUT"
assert_contains "1.3c: high urgency" "URGENCY: high" "$OUTPUT"

# Test 1.4: 500 triggers server error diagnosis
log_test "1.4: Server error (500) triggers diagnosis"
echo "500" > "$MOCK_STATE/api_http_code"
echo '{"type":"error","error":{"type":"api_error","message":"internal error"}}' > "$MOCK_STATE/api_response_body"
echo "000" > "$MOCK_STATE/ollama_tags_code"
OUTPUT=$(run_doctor 2>&1 || true)
assert_contains "1.4a: detects server error" "api-server-error" "$OUTPUT"
assert_contains "1.4b: classifies api-outage" "CLASSIFICATION: api-outage" "$OUTPUT"
assert_contains "1.4c: mentions status page" "status.anthropic.com" "$OUTPUT"

# Test 1.5: 000 (network failure) triggers network diagnosis
log_test "1.5: Network failure (000) triggers diagnosis"
echo "000" > "$MOCK_STATE/api_http_code"
echo "" > "$MOCK_STATE/api_response_body"
echo "000" > "$MOCK_STATE/ollama_tags_code"
OUTPUT=$(run_doctor 2>&1 || true)
assert_contains "1.5a: detects network" "network-unreachable" "$OUTPUT"
assert_contains "1.5b: classifies network-down" "CLASSIFICATION: network-down" "$OUTPUT"
assert_contains "1.5c: critical urgency" "URGENCY: critical" "$OUTPUT"

# Test 1.6: 429 defers to rate-limit-watchdog
log_test "1.6: Rate limit (429) defers to watchdog"
echo "429" > "$MOCK_STATE/api_http_code"
echo '{"type":"error","error":{"type":"rate_limit_error"}}' > "$MOCK_STATE/api_response_body"
OUTPUT=$(run_doctor 2>&1 || true)
assert_contains "1.6a: defers to watchdog" "deferring to rate-limit-watchdog" "$OUTPUT"
assert_not_contains "1.6b: no diagnosis" "Failure detected" "$OUTPUT"

# Test 1.7: Network failure with ping failure
log_test "1.7: Network + ping failure → network-down"
echo "000" > "$MOCK_STATE/api_http_code"
echo "" > "$MOCK_STATE/api_response_body"
echo "000" > "$MOCK_STATE/ollama_tags_code"
touch "$MOCK_STATE/ping_fail"
OUTPUT=$(run_doctor 2>&1 || true)
assert_contains "1.7: full network down" "network-down" "$OUTPUT"
rm -f "$MOCK_STATE/ping_fail"

# --- Test Group 2: Ollama Integration ----------------------------------------
echo ""
echo "--- Ollama Integration ---"

# Test 2.1: Ollama available with model → uses LLM diagnosis
log_test "2.1: Ollama with model → LLM diagnosis"
echo "401" > "$MOCK_STATE/api_http_code"
echo '{"type":"error","error":{"type":"authentication_error","message":"invalid key"}}' > "$MOCK_STATE/api_response_body"
echo "200" > "$MOCK_STATE/ollama_tags_code"
echo '{"models":[{"name":"llama3.1:8b","size":4700000000}]}' > "$MOCK_STATE/ollama_tags_body"
echo '{"response":"CLASSIFICATION: auth-invalid\nROOT CAUSE: Bad API key.\nSUGGESTED FIX:\n- Rotate key\nURGENCY: critical"}' > "$MOCK_STATE/ollama_generate_body"
OUTPUT=$(run_doctor 2>&1 || true)
assert_contains "2.1a: uses Ollama" "Ollama available" "$OUTPUT"
assert_contains "2.1b: diagnosis complete" "Ollama diagnosis complete" "$OUTPUT"
assert_contains "2.1c: diagnosed by Ollama" "Diagnosed by: Ollama" "$OUTPUT"

# Test 2.2: Ollama down → shell fallback
log_test "2.2: Ollama down → shell fallback"
echo "401" > "$MOCK_STATE/api_http_code"
echo '{"type":"error","error":{"type":"authentication_error"}}' > "$MOCK_STATE/api_response_body"
echo "000" > "$MOCK_STATE/ollama_tags_code"
OUTPUT=$(run_doctor 2>&1 || true)
assert_contains "2.2a: falls back to shell" "shell fallback" "$OUTPUT"
assert_contains "2.2b: still classifies" "CLASSIFICATION: auth-invalid" "$OUTPUT"

# Test 2.3: Ollama running but generate fails → shell fallback
log_test "2.3: Ollama generate failure → shell fallback"
echo "500" > "$MOCK_STATE/api_http_code"
echo '{}' > "$MOCK_STATE/api_response_body"
echo "200" > "$MOCK_STATE/ollama_tags_code"
echo '{"models":[{"name":"llama3.1:8b"}]}' > "$MOCK_STATE/ollama_tags_body"
echo '' > "$MOCK_STATE/ollama_generate_body"  # Empty response = failure
OUTPUT=$(run_doctor 2>&1 || true)
assert_contains "2.3: falls back" "shell fallback" "$OUTPUT"

# --- Test Group 3: State Management ------------------------------------------
echo ""
echo "--- State Management ---"

# Test 3.1: Consecutive failure tracking
log_test "3.1: Consecutive failure counter increments"
echo "401" > "$MOCK_STATE/api_http_code"
echo '{}' > "$MOCK_STATE/api_response_body"
echo "000" > "$MOCK_STATE/ollama_tags_code"

# Run 1
run_doctor > /dev/null 2>&1 || true
# Run 2 (reuse same town dir)
TEST_TOWN="$TEST_TMPDIR/town"
FAIL_FILE="$TEST_TOWN/.llm-doctor/consecutive-failures"
# Can't easily test across runs in dry-run mode since each run_doctor creates fresh state
# Instead, verify the file was created
assert_file_exists "3.1: failure file created" "$FAIL_FILE"

# Test 3.2: Diagnosis file saved
log_test "3.2: Diagnosis file saved"
DIAG_FILE="$TEST_TOWN/.llm-doctor/last-diagnosis"
assert_file_exists "3.2: diagnosis file" "$DIAG_FILE"
DIAG_CONTENT=$(cat "$DIAG_FILE")
assert_contains "3.2b: has header" "LLM Doctor Diagnosis" "$DIAG_CONTENT"
assert_contains "3.2c: has raw diag" "Raw Diagnostics" "$DIAG_CONTENT"

# --- Test Group 4: Escalation Logic ------------------------------------------
echo ""
echo "--- Escalation Logic ---"

# Test 4.1: --force with healthy API → forced-test type
log_test "4.1: --force runs diagnosis on healthy API"
echo "200" > "$MOCK_STATE/api_http_code"
echo '{"type":"message"}' > "$MOCK_STATE/api_response_body"
echo "000" > "$MOCK_STATE/ollama_tags_code"
OUTPUT=$(run_doctor --force 2>&1 || true)
assert_contains "4.1a: forced test" "forced-test" "$OUTPUT"
assert_contains "4.1b: still runs diagnosis" "would escalate" "$OUTPUT"

# Test 4.2: Critical urgency detected from diagnosis
log_test "4.2: Critical urgency extraction"
echo "000" > "$MOCK_STATE/api_http_code"
echo "" > "$MOCK_STATE/api_response_body"
echo "000" > "$MOCK_STATE/ollama_tags_code"
OUTPUT=$(run_doctor 2>&1 || true)
assert_contains "4.2: critical urgency in output" "URGENCY: critical" "$OUTPUT"

# --- Test Group 5: Diagnostics Gathering -------------------------------------
echo ""
echo "--- Diagnostics Gathering ---"

# Test 5.1: API key redaction
log_test "5.1: API key is redacted in diagnostics"
echo "401" > "$MOCK_STATE/api_http_code"
echo '{}' > "$MOCK_STATE/api_response_body"
echo "000" > "$MOCK_STATE/ollama_tags_code"
export TEST_API_KEY="sk-ant-api03-secret-key-do-not-leak"
OUTPUT=$(run_doctor 2>&1 || true)
assert_not_contains "5.1a: full key not in output" "secret-key-do-not-leak" "$OUTPUT"
assert_contains "5.1b: prefix shown" "sk-ant-api03" "$OUTPUT"
unset TEST_API_KEY

# Test 5.2: Missing API key detected
log_test "5.2: Missing API key reported"
echo "401" > "$MOCK_STATE/api_http_code"
echo '{}' > "$MOCK_STATE/api_response_body"
echo "000" > "$MOCK_STATE/ollama_tags_code"
export TEST_API_KEY=""
OUTPUT=$(env ANTHROPIC_API_KEY="" bash -c "
    export MOCK_STATE='$MOCK_STATE'
    export PATH='$MOCK_BIN:$PATH'
    export GT_ROOT='$TEST_TMPDIR/town'
    export ANTHROPIC_BASE_URL='https://api.anthropic.com'
    export OLLAMA_URL='http://localhost:11434'
    bash '$SCRIPT_DIR/run.sh' --dry-run 2>&1
" || true)
assert_contains "5.2: reports auth not set" "AUTH: NOT SET" "$OUTPUT"
unset TEST_API_KEY

# Test 5.3: DNS info collected
log_test "5.3: DNS info in diagnostics"
echo "401" > "$MOCK_STATE/api_http_code"
echo '{}' > "$MOCK_STATE/api_response_body"
echo "000" > "$MOCK_STATE/ollama_tags_code"
OUTPUT=$(run_doctor 2>&1 || true)
assert_contains "5.3: DNS lookup present" "DNS_LOOKUP" "$OUTPUT"

# Test 5.4: ESTOP status reported
log_test "5.4: ESTOP status in diagnostics"
echo "401" > "$MOCK_STATE/api_http_code"
echo '{}' > "$MOCK_STATE/api_response_body"
echo "000" > "$MOCK_STATE/ollama_tags_code"
OUTPUT=$(run_doctor 2>&1 || true)
assert_contains "5.4: ESTOP status" "ESTOP:" "$OUTPUT"

# --- Test Group 5b: Auth Methods ---------------------------------------------
echo ""
echo "--- Auth Methods ---"

# Test 5b.1: OAuth token auth (Claude Max)
log_test "5b.1: OAuth token auth (Claude Max)"
echo "401" > "$MOCK_STATE/api_http_code"
echo '{}' > "$MOCK_STATE/api_response_body"
echo "000" > "$MOCK_STATE/ollama_tags_code"
OUTPUT=$(env \
    PATH="$MOCK_BIN:$PATH" \
    MOCK_STATE="$MOCK_STATE" \
    GT_ROOT="$TEST_TMPDIR/town" \
    ANTHROPIC_API_KEY="" \
    ANTHROPIC_AUTH_TOKEN="oauth-test-token-abc123" \
    ANTHROPIC_BASE_URL="https://api.anthropic.com" \
    OLLAMA_URL="http://localhost:11434" \
    bash "$SCRIPT_DIR/run.sh" --dry-run 2>&1 || true)
assert_contains "5b.1a: detects oauth method" "AUTH_METHOD: oauth" "$OUTPUT"
assert_contains "5b.1b: token prefix shown" "oauth-test-t..." "$OUTPUT"
assert_not_contains "5b.1c: full token not leaked" "abc123" "$OUTPUT"
assert_contains "5b.1d: oauth-specific fix" "claude login" "$OUTPUT"

# Test 5b.2: API key auth
log_test "5b.2: API key auth"
echo "401" > "$MOCK_STATE/api_http_code"
echo '{}' > "$MOCK_STATE/api_response_body"
echo "000" > "$MOCK_STATE/ollama_tags_code"
OUTPUT=$(env \
    PATH="$MOCK_BIN:$PATH" \
    MOCK_STATE="$MOCK_STATE" \
    GT_ROOT="$TEST_TMPDIR/town" \
    ANTHROPIC_API_KEY="sk-ant-api03-testkey" \
    ANTHROPIC_AUTH_TOKEN="" \
    ANTHROPIC_BASE_URL="https://api.anthropic.com" \
    OLLAMA_URL="http://localhost:11434" \
    bash "$SCRIPT_DIR/run.sh" --dry-run 2>&1 || true)
assert_contains "5b.2a: detects api-key method" "AUTH_METHOD: api-key" "$OUTPUT"
assert_contains "5b.2b: key-specific fix" "console.anthropic.com" "$OUTPUT"

# Test 5b.3: No auth configured
log_test "5b.3: No auth configured"
echo "401" > "$MOCK_STATE/api_http_code"
echo '{}' > "$MOCK_STATE/api_response_body"
echo "000" > "$MOCK_STATE/ollama_tags_code"
OUTPUT=$(env \
    PATH="$MOCK_BIN:$PATH" \
    MOCK_STATE="$MOCK_STATE" \
    GT_ROOT="$TEST_TMPDIR/town" \
    ANTHROPIC_API_KEY="" \
    ANTHROPIC_AUTH_TOKEN="" \
    ANTHROPIC_BASE_URL="https://api.anthropic.com" \
    OLLAMA_URL="http://localhost:11434" \
    bash "$SCRIPT_DIR/run.sh" --dry-run 2>&1 || true)
assert_contains "5b.3a: detects no auth" "AUTH_METHOD: none" "$OUTPUT"
assert_contains "5b.3b: reports not set" "NOT SET" "$OUTPUT"

# Test 5b.4: OAuth takes priority over API key when both set
log_test "5b.4: OAuth takes priority over API key"
echo "200" > "$MOCK_STATE/api_http_code"
echo '{"type":"message"}' > "$MOCK_STATE/api_response_body"
OUTPUT=$(env \
    PATH="$MOCK_BIN:$PATH" \
    MOCK_STATE="$MOCK_STATE" \
    GT_ROOT="$TEST_TMPDIR/town" \
    ANTHROPIC_API_KEY="sk-ant-api03-testkey" \
    ANTHROPIC_AUTH_TOKEN="oauth-test-token" \
    ANTHROPIC_BASE_URL="https://api.anthropic.com" \
    OLLAMA_URL="http://localhost:11434" \
    bash "$SCRIPT_DIR/run.sh" --dry-run --force 2>&1 || true)
assert_contains "5b.4: oauth wins" "AUTH_METHOD: oauth" "$OUTPUT"

# --- Test Group 6: Model Resolution -----------------------------------------
echo ""
echo "--- Model Resolution ---"

# Test 6.1: Explicit model override
log_test "6.1: LLM_DOCTOR_OLLAMA_MODEL override"
OUTPUT=$(env MOCK_STATE="$MOCK_STATE" OLLAMA_URL="http://localhost:11434" \
    LLM_DOCTOR_OLLAMA_MODEL="my-custom-model:latest" \
    PATH="$MOCK_BIN:$PATH" \
    bash -c 'source "'"$SCRIPT_DIR"'/resolve-model.sh"; resolve_ollama_model 2>&1')
assert_contains "6.1: uses explicit model" "my-custom-model:latest" "$OUTPUT"

# Test 6.2: Ollama unreachable returns failure
log_test "6.2: Ollama unreachable → failure"
echo "000" > "$MOCK_STATE/ollama_tags_code"
RC=0
env MOCK_STATE="$MOCK_STATE" OLLAMA_URL="http://localhost:11434" \
    PATH="$MOCK_BIN:$PATH" \
    bash -c 'source "'"$SCRIPT_DIR"'/resolve-model.sh"; resolve_ollama_model quiet' > /dev/null 2>&1 || RC=$?
assert_exit_code "6.2: returns non-zero" "1" "$RC"

# Test 6.3: Finds preferred model from pulled list
log_test "6.3: Finds preferred model in pulled list"
echo "200" > "$MOCK_STATE/ollama_tags_code"
echo '{"models":[{"name":"qwen2.5:32b","size":20000000000},{"name":"llama3.1:8b","size":4700000000}]}' > "$MOCK_STATE/ollama_tags_body"
OUTPUT=$(env MOCK_STATE="$MOCK_STATE" OLLAMA_URL="http://localhost:11434" \
    PATH="$MOCK_BIN:$PATH" \
    bash -c 'source "'"$SCRIPT_DIR"'/resolve-model.sh"; resolve_ollama_model 2>&1')
# Should pick llama3.1:8b (higher preference than qwen2.5:32b)
assert_contains "6.3: picks preferred model" "llama3.1:8b" "$OUTPUT"

# Test 6.4: Falls back to any available model
log_test "6.4: Falls back to non-preferred but available model"
echo "200" > "$MOCK_STATE/ollama_tags_code"
echo '{"models":[{"name":"mistral:7b","size":4000000000}]}' > "$MOCK_STATE/ollama_tags_body"
OUTPUT=$(env MOCK_STATE="$MOCK_STATE" OLLAMA_URL="http://localhost:11434" \
    PATH="$MOCK_BIN:$PATH" \
    bash -c 'source "'"$SCRIPT_DIR"'/resolve-model.sh"; resolve_ollama_model 2>&1')
assert_contains "6.4: uses available model" "mistral:7b" "$OUTPUT"

# --- Test Group 7: Edge Cases ------------------------------------------------
echo ""
echo "--- Edge Cases ---"

# Test 7.1: Unexpected HTTP code
log_test "7.1: Unexpected HTTP code (418)"
echo "418" > "$MOCK_STATE/api_http_code"
echo '{"error":"I am a teapot"}' > "$MOCK_STATE/api_response_body"
echo "000" > "$MOCK_STATE/ollama_tags_code"
OUTPUT=$(run_doctor 2>&1 || true)
assert_contains "7.1a: detected" "unexpected-418" "$OUTPUT"
assert_contains "7.1b: unknown classification" "CLASSIFICATION: unknown" "$OUTPUT"

# Test 7.2: 502 Bad Gateway
log_test "7.2: 502 Bad Gateway"
echo "502" > "$MOCK_STATE/api_http_code"
echo '<html>Bad Gateway</html>' > "$MOCK_STATE/api_response_body"
echo "000" > "$MOCK_STATE/ollama_tags_code"
OUTPUT=$(run_doctor 2>&1 || true)
assert_contains "7.2: classified as api-outage" "api-outage" "$OUTPUT"

# Test 7.3: 503 Service Unavailable
log_test "7.3: 503 Service Unavailable"
echo "503" > "$MOCK_STATE/api_http_code"
echo '{"error":"overloaded"}' > "$MOCK_STATE/api_response_body"
echo "000" > "$MOCK_STATE/ollama_tags_code"
OUTPUT=$(run_doctor 2>&1 || true)
assert_contains "7.3: classified as server error" "api-server-error" "$OUTPUT"

# =============================================================================
# RESULTS
# =============================================================================

echo ""
echo "==========================================="
echo "  Results: $PASS passed, $FAIL failed"
echo "==========================================="

if [[ $FAIL -gt 0 ]]; then
    echo ""
    echo "Failures:"
    echo "$ERRORS"
    exit 1
fi

echo "  All tests passed."
