#!/bin/bash

# Comprehensive manual test for gt-proxy-server and gt-proxy-client
# Tests: server startup, admin API cert issuance, mTLS exec, client e2e, rate limiting

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
PROXY_PORT=${PROXY_PORT:-9876}
PROXY_ADMIN_PORT=${PROXY_ADMIN_PORT:-9877}
TOWN_ROOT=${TOWN_ROOT:-${HOME}/gt}
PROXY_DIR=${PROXY_DIR:-/tmp/gt-proxy-test}
TEST_RIG="testrig"
TEST_POLECAT="testpolecat"
FAILURES=0

cleanup() {
    echo -e "\n${BLUE}Cleaning up...${NC}"
    if [[ -n "${PROXY_PID}" ]] && kill -0 "${PROXY_PID}" 2>/dev/null; then
        kill "${PROXY_PID}" 2>/dev/null || true
        wait "${PROXY_PID}" 2>/dev/null || true
    fi
    rm -rf "${PROXY_DIR}"
}
trap cleanup EXIT

log_section() {
    echo -e "\n${BLUE}══════════════════════════════════════${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}══════════════════════════════════════${NC}\n"
}

log_ok()   { echo -e "${GREEN}  OK $1${NC}"; }
log_fail() { echo -e "${RED}  FAIL $1${NC}"; FAILURES=$((FAILURES + 1)); }
log_info() { echo -e "${YELLOW}  -> $1${NC}"; }

# =============================================================================
# STEP 1: Build binaries
# =============================================================================
log_section "Step 1: Building proxy binaries"

rm -rf "${PROXY_DIR}"
mkdir -p "${PROXY_DIR}/ca" "${PROXY_DIR}/bin" "${PROXY_DIR}/logs"

go build -o "${PROXY_DIR}/bin/gt-proxy-server" ./cmd/gt-proxy-server && log_ok "gt-proxy-server" || log_fail "gt-proxy-server build"
go build -o "${PROXY_DIR}/bin/gt-proxy-client" ./cmd/gt-proxy-client && log_ok "gt-proxy-client" || log_fail "gt-proxy-client build"

# =============================================================================
# STEP 2: Start proxy server
# =============================================================================
log_section "Step 2: Starting proxy server"

log_info "Listening on 127.0.0.1:${PROXY_PORT}, admin on 127.0.0.1:${PROXY_ADMIN_PORT}"

"${PROXY_DIR}/bin/gt-proxy-server" \
    --listen "127.0.0.1:${PROXY_PORT}" \
    --admin-listen "127.0.0.1:${PROXY_ADMIN_PORT}" \
    --ca-dir "${PROXY_DIR}/ca" \
    --town-root "${TOWN_ROOT}" \
    > "${PROXY_DIR}/logs/server.log" 2>&1 &
PROXY_PID=$!

# Wait for the admin port to be ready
for i in $(seq 1 30); do
    if lsof -nP -iTCP:"${PROXY_ADMIN_PORT}" -sTCP:LISTEN >/dev/null 2>&1; then
        break
    fi
    if ! kill -0 "${PROXY_PID}" 2>/dev/null; then
        log_fail "Server exited prematurely"
        cat "${PROXY_DIR}/logs/server.log"
        exit 1
    fi
    sleep 0.2
done

if kill -0 "${PROXY_PID}" 2>/dev/null; then
    log_ok "Server started (PID: ${PROXY_PID})"
else
    log_fail "Server not running"
    cat "${PROXY_DIR}/logs/server.log"
    exit 1
fi

# =============================================================================
# STEP 3: Verify server health
# =============================================================================
log_section "Step 3: Verifying server health"

if lsof -nP -iTCP:"${PROXY_PORT}" -sTCP:LISTEN >/dev/null 2>&1; then
    log_ok "mTLS port ${PROXY_PORT} listening"
else
    log_fail "mTLS port ${PROXY_PORT} not listening"
fi

if lsof -nP -iTCP:"${PROXY_ADMIN_PORT}" -sTCP:LISTEN >/dev/null 2>&1; then
    log_ok "Admin port ${PROXY_ADMIN_PORT} listening"
else
    log_fail "Admin port ${PROXY_ADMIN_PORT} not listening"
fi

# =============================================================================
# STEP 4: Issue polecat certificate via admin API
# =============================================================================
log_section "Step 4: Issuing polecat certificate via admin API"

CERT_FILE="${PROXY_DIR}/client.crt"
KEY_FILE="${PROXY_DIR}/client.key"
CA_FILE="${PROXY_DIR}/ca-client.crt"

CERT_RESPONSE=$(curl -sf \
    -X POST \
    -H "Content-Type: application/json" \
    -d "{\"rig\":\"${TEST_RIG}\",\"name\":\"${TEST_POLECAT}\"}" \
    "http://127.0.0.1:${PROXY_ADMIN_PORT}/v1/admin/issue-cert" 2>/dev/null)

if [[ -z "${CERT_RESPONSE}" ]]; then
    log_fail "Admin API /v1/admin/issue-cert returned empty response"
    cat "${PROXY_DIR}/logs/server.log" | tail -10
    exit 1
fi

# Extract cert, key, and CA from JSON response
CERT_CN=$(echo "${CERT_RESPONSE}" | python3 -c "
import sys, json
d = json.load(sys.stdin)
open('${CERT_FILE}', 'w').write(d['cert'])
open('${KEY_FILE}', 'w').write(d['key'])
open('${CA_FILE}', 'w').write(d['ca'])
print(d['cn'])
")
chmod 600 "${KEY_FILE}"

if [[ -f "${CERT_FILE}" ]] && [[ -f "${KEY_FILE}" ]] && [[ -f "${CA_FILE}" ]]; then
    log_ok "Certificate issued: CN=${CERT_CN}"
else
    log_fail "Certificate files not created"
    exit 1
fi

# Helper function for mTLS curl requests to the proxy
proxy_curl() {
    curl -s \
        --cert "${CERT_FILE}" \
        --key "${KEY_FILE}" \
        --cacert "${CA_FILE}" \
        --resolve "gt-proxy-server:${PROXY_PORT}:127.0.0.1" \
        --max-time 10 \
        -X POST \
        -H "Content-Type: application/json" \
        "$@" \
        "https://gt-proxy-server:${PROXY_PORT}/v1/exec"
}

# =============================================================================
# STEP 5: Test mTLS exec — gt version
# =============================================================================
log_section "Step 5: Testing mTLS exec"

test_exec() {
    local desc="$1"
    local json_body="$2"
    local expect_code="${3:-200}"

    HTTP_CODE=$(curl -s -o "${PROXY_DIR}/resp.json" -w "%{http_code}" \
        --cert "${CERT_FILE}" \
        --key "${KEY_FILE}" \
        --cacert "${CA_FILE}" \
        --resolve "gt-proxy-server:${PROXY_PORT}:127.0.0.1" \
        --max-time 10 \
        -X POST \
        -H "Content-Type: application/json" \
        -d "${json_body}" \
        "https://gt-proxy-server:${PROXY_PORT}/v1/exec" 2>/dev/null || echo "000")

    if [[ "${HTTP_CODE}" == "000" ]]; then
        log_fail "${desc}: TLS connection failed"
        return 1
    elif [[ "${HTTP_CODE}" == "${expect_code}" ]]; then
        RESP=$(cat "${PROXY_DIR}/resp.json" 2>/dev/null)
        log_ok "${desc} (HTTP ${HTTP_CODE})"
        if [[ -n "${RESP}" ]]; then
            log_info "Response: ${RESP:0:120}"
        fi
        return 0
    else
        RESP=$(cat "${PROXY_DIR}/resp.json" 2>/dev/null)
        log_fail "${desc}: expected HTTP ${expect_code}, got ${HTTP_CODE}"
        log_info "Response: ${RESP:0:200}"
        return 1
    fi
}

test_exec "gt version"         '{"argv":["gt","version"]}'
test_exec "gt status"          '{"argv":["gt","status"]}'
test_exec "gt convoy --help"   '{"argv":["gt","convoy","--help"]}'
test_exec "bd list --status=open" '{"argv":["bd","list","--status=open"]}'

# =============================================================================
# STEP 6: Test forbidden commands
# =============================================================================
log_section "Step 6: Testing forbidden commands (should return 403)"

test_exec "forbidden: rm" '{"argv":["rm","-rf","/"]}' "403"
test_exec "forbidden: bash" '{"argv":["bash","-c","id"]}' "403"

# =============================================================================
# STEP 7: gt-proxy-client end-to-end
# =============================================================================
log_section "Step 7: gt-proxy-client end-to-end (symlinked as gt)"

# Create a symlink so toolNameFromArg0 returns "gt"
ln -sf "${PROXY_DIR}/bin/gt-proxy-client" "${PROXY_DIR}/bin/gt"
ln -sf "${PROXY_DIR}/bin/gt-proxy-client" "${PROXY_DIR}/bin/bd"

# The server cert has 127.0.0.1 as an IP SAN, so the Go TLS client
# can verify it when connecting by IP.
export GT_PROXY_URL="https://127.0.0.1:${PROXY_PORT}"
export GT_PROXY_CERT="${CERT_FILE}"
export GT_PROXY_KEY="${KEY_FILE}"
export GT_PROXY_CA="${CA_FILE}"

log_info "GT_PROXY_URL=${GT_PROXY_URL}"

# Test gt version
E2E_OUT=$("${PROXY_DIR}/bin/gt" version 2>"${PROXY_DIR}/e2e_gt_err.log" || true)
E2E_ERR=$(cat "${PROXY_DIR}/e2e_gt_err.log" 2>/dev/null)

if [[ -n "${E2E_OUT}" ]]; then
    log_ok "gt version via proxy-client"
    log_info "stdout: ${E2E_OUT:0:120}"
elif echo "${E2E_ERR}" | grep -q "proxy request failed"; then
    log_fail "gt-proxy-client could not connect to proxy"
    log_info "stderr: ${E2E_ERR:0:200}"
else
    # Connected but gt may exit non-zero (e.g. no town configured)
    log_ok "gt version via proxy-client (non-zero exit expected without town)"
    log_info "stderr: ${E2E_ERR:0:200}"
fi

# Test bd list
E2E_OUT=$("${PROXY_DIR}/bin/bd" list 2>"${PROXY_DIR}/e2e_bd_err.log" || true)
E2E_ERR=$(cat "${PROXY_DIR}/e2e_bd_err.log" 2>/dev/null)

if [[ -n "${E2E_OUT}" ]]; then
    log_ok "bd list via proxy-client"
    log_info "stdout: ${E2E_OUT:0:120}"
elif echo "${E2E_ERR}" | grep -q "proxy request failed"; then
    log_fail "bd proxy-client could not connect to proxy"
    log_info "stderr: ${E2E_ERR:0:200}"
else
    log_ok "bd list via proxy-client (non-zero exit expected without town)"
    log_info "stderr: ${E2E_ERR:0:200}"
fi

# Unset proxy env vars for remaining tests
unset GT_PROXY_URL GT_PROXY_CERT GT_PROXY_KEY GT_PROXY_CA

# =============================================================================
# STEP 8: Rate limiting test
# =============================================================================
log_section "Step 8: Testing rate limiting"

log_info "Sending 25 rapid requests (burst limit is 20)..."
SUCCESS_COUNT=0
RATE_LIMITED=0

for i in $(seq 1 25); do
    HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
        --cert "${CERT_FILE}" \
        --key "${KEY_FILE}" \
        --cacert "${CA_FILE}" \
        --resolve "gt-proxy-server:${PROXY_PORT}:127.0.0.1" \
        --max-time 5 \
        -X POST \
        -H "Content-Type: application/json" \
        -d '{"argv":["gt","version"]}' \
        "https://gt-proxy-server:${PROXY_PORT}/v1/exec" 2>/dev/null || echo "000")

    if [[ "${HTTP_CODE}" == "200" ]]; then
        SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
    elif [[ "${HTTP_CODE}" == "429" ]]; then
        RATE_LIMITED=$((RATE_LIMITED + 1))
    fi
done

log_ok "Results: ${SUCCESS_COUNT}/25 succeeded, ${RATE_LIMITED}/25 rate-limited (429)"
if [[ ${RATE_LIMITED} -gt 0 ]]; then
    log_ok "Rate limiting is active"
else
    log_info "Rate limiting not triggered (burst=20, might be expected for serial requests)"
fi

# =============================================================================
# STEP 9: Server logs
# =============================================================================
log_section "Step 9: Server log summary"

log_info "Last 10 lines of server log:"
tail -10 "${PROXY_DIR}/logs/server.log" | sed 's/^/    /'

# =============================================================================
# SUMMARY
# =============================================================================
log_section "Test Summary"

if [[ ${FAILURES} -eq 0 ]]; then
    echo -e "${GREEN}  All tests passed!${NC}"
else
    echo -e "${RED}  ${FAILURES} test(s) failed${NC}"
fi

echo ""
echo -e "${YELLOW}  Test Environment:${NC}"
echo "    Server PID:   ${PROXY_PID}"
echo "    mTLS Port:    ${PROXY_PORT}"
echo "    Admin Port:   ${PROXY_ADMIN_PORT}"
echo "    CA Directory: ${PROXY_DIR}/ca"
echo "    Server Log:   ${PROXY_DIR}/logs/server.log"
echo ""

exit ${FAILURES}
