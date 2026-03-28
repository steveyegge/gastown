#!/bin/sh
# Generate a GitHub App installation access token.
# Requires: GH_APP_ID, GH_APP_INSTALLATION_ID, GH_APP_PEM_FILE env vars.
# Outputs the token to stdout. Exits non-zero on failure.
set -e

: "${GH_APP_ID:?missing GH_APP_ID}"
: "${GH_APP_INSTALLATION_ID:?missing GH_APP_INSTALLATION_ID}"
: "${GH_APP_PEM_FILE:?missing GH_APP_PEM_FILE}"

NOW=$(date +%s)
IAT=$((NOW - 60))
EXP=$((NOW + 600))

HEADER=$(printf '{"alg":"RS256","typ":"JWT"}' | openssl base64 -e -A | tr '+/' '-_' | tr -d '=')
PAYLOAD=$(printf '{"iat":%d,"exp":%d,"iss":"%s"}' "$IAT" "$EXP" "$GH_APP_ID" | openssl base64 -e -A | tr '+/' '-_' | tr -d '=')
SIG=$(printf '%s' "${HEADER}.${PAYLOAD}" | openssl dgst -sha256 -sign "$GH_APP_PEM_FILE" | openssl base64 -e -A | tr '+/' '-_' | tr -d '=')
JWT="${HEADER}.${PAYLOAD}.${SIG}"

RESPONSE=$(curl -sf -X POST \
  -H "Authorization: Bearer ${JWT}" \
  -H "Accept: application/vnd.github+json" \
  "https://api.github.com/app/installations/${GH_APP_INSTALLATION_ID}/access_tokens")

printf '%s' "$RESPONSE" | tr -d '\n ' | sed -n 's/.*"token":"\([^"]*\)".*/\1/p'
