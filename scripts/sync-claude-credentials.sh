#!/usr/bin/env bash
# sync-claude-credentials.sh — Extract Claude Max/Corp OAuth token from macOS
# Keychain and push it to a K8s namespace as a secret.
#
# Usage:
#   ./scripts/sync-claude-credentials.sh [namespace]
#
# Defaults to gastown-uat namespace. The secret is named "claude-credentials"
# and contains a single key "credentials.json" with the OAuth token JSON.
#
# The agent entrypoint should mount this at ~/.claude/.credentials.json
# so Claude Code picks it up (plaintext fallback when no keychain is available).
#
# Token refresh: Claude Code tokens expire. Re-run this script after
# re-authenticating with `claude login` or `claude setup-token`.

set -euo pipefail

NAMESPACE="${1:-gastown-uat}"
SECRET_NAME="claude-credentials"
KEYCHAIN_SERVICE="Claude Code-credentials"
KEYCHAIN_ACCOUNT="${USER:-claude-code-user}"

echo "Extracting Claude credentials from macOS Keychain..."
echo "  Service: ${KEYCHAIN_SERVICE}"
echo "  Account: ${KEYCHAIN_ACCOUNT}"

RAW=$(security find-generic-password -s "${KEYCHAIN_SERVICE}" -a "${KEYCHAIN_ACCOUNT}" -w 2>/dev/null) || {
    echo "ERROR: No Claude credentials found in Keychain." >&2
    echo "  Run 'claude login' or 'claude setup-token' first." >&2
    exit 1
}

# Credentials are stored hex-encoded; decode to JSON
CREDS=$(python3 -c "
import sys
raw = sys.argv[1]
try:
    print(bytes.fromhex(raw).decode('utf-8'))
except ValueError:
    print(raw)
" "${RAW}")

# Validate it's valid JSON with expected structure
python3 -c "
import json, sys
data = json.loads(sys.argv[1])
oauth = data.get('claudeAiOauth', {})
if not oauth.get('accessToken'):
    print('ERROR: credentials missing accessToken', file=sys.stderr)
    sys.exit(1)
import datetime
exp = oauth.get('expiresAt', 0)
exp_dt = datetime.datetime.fromtimestamp(exp / 1000 if exp > 1e12 else exp)
now = datetime.datetime.now()
if exp_dt < now:
    print(f'WARNING: Token expired at {exp_dt}. Run claude login to refresh.', file=sys.stderr)
else:
    remaining = exp_dt - now
    print(f'  Token expires: {exp_dt} ({remaining} remaining)')
print(f'  Subscription: {oauth.get(\"subscriptionType\", \"unknown\")}')
" "${CREDS}" || exit 1

# Delete existing secret if present, then create
echo "Pushing to K8s namespace: ${NAMESPACE}"
kubectl delete secret "${SECRET_NAME}" -n "${NAMESPACE}" --ignore-not-found >/dev/null 2>&1
echo "${CREDS}" | kubectl create secret generic "${SECRET_NAME}" \
    -n "${NAMESPACE}" \
    --from-file=credentials.json=/dev/stdin 2>/dev/null

echo "Secret '${SECRET_NAME}' created in namespace '${NAMESPACE}'."
echo ""
echo "To use in agent pods, mount as:"
echo "  volumeMounts:"
echo "    - name: claude-creds"
echo "      mountPath: /home/agent/.claude/.credentials.json"
echo "      subPath: credentials.json"
echo "      readOnly: true"
echo "  volumes:"
echo "    - name: claude-creds"
echo "      secret:"
echo "        secretName: ${SECRET_NAME}"
