#!/usr/bin/env bash
# cloudflare-setup.sh — Configure Cloudflare DNS + proxy for faultline.live
#
# Prerequisites:
#   export CF_API_TOKEN="<cloudflare api token>"  (Zone:DNS:Edit, Zone:Zone Settings:Edit, Zone:Cache:Purge)
#   export CF_ZONE_ID="<zone id for faultline.live>"
#   export RAILWAY_DOMAIN="<railway app domain, e.g. faultline-production.up.railway.app>"
#
# Usage: ./scripts/cloudflare-setup.sh

set -euo pipefail

: "${CF_API_TOKEN:?Set CF_API_TOKEN}"
: "${CF_ZONE_ID:?Set CF_ZONE_ID}"
: "${RAILWAY_DOMAIN:?Set RAILWAY_DOMAIN}"

CF_API="https://api.cloudflare.com/client/v4"

cf() {
  curl -sf -H "Authorization: Bearer $CF_API_TOKEN" \
       -H "Content-Type: application/json" \
       "$@"
}

echo "=== Cloudflare setup for faultline.live ==="
echo "Zone: $CF_ZONE_ID"
echo "Railway: $RAILWAY_DOMAIN"
echo

# --- 1. DNS: CNAME root to Railway (proxied / orange cloud) ---
echo "--- Step 1: DNS CNAME record (proxied) ---"

# Check for existing root record.
existing=$(cf "$CF_API/zones/$CF_ZONE_ID/dns_records?name=faultline.live&type=CNAME" | jq -r '.result[0].id // empty')

dns_payload=$(cat <<EOF
{
  "type": "CNAME",
  "name": "faultline.live",
  "content": "$RAILWAY_DOMAIN",
  "proxied": true,
  "ttl": 1,
  "comment": "Railway relay service"
}
EOF
)

if [ -n "$existing" ]; then
  cf -X PUT "$CF_API/zones/$CF_ZONE_ID/dns_records/$existing" -d "$dns_payload" | jq '.success'
  echo "Updated existing CNAME record."
else
  cf -X POST "$CF_API/zones/$CF_ZONE_ID/dns_records" -d "$dns_payload" | jq '.success'
  echo "Created CNAME record."
fi

# --- 2. SSL/TLS: Full (strict) mode ---
echo
echo "--- Step 2: SSL/TLS → Full (strict) ---"
cf -X PATCH "$CF_API/zones/$CF_ZONE_ID/settings/ssl" \
  -d '{"value":"strict"}' | jq '.success'

# --- 3. Always Use HTTPS ---
echo
echo "--- Step 3: Always Use HTTPS ---"
cf -X PATCH "$CF_API/zones/$CF_ZONE_ID/settings/always_use_https" \
  -d '{"value":"on"}' | jq '.success'

# --- 4. HSTS ---
echo
echo "--- Step 4: HSTS (max-age 2 years, includeSubDomains, preload) ---"
cf -X PATCH "$CF_API/zones/$CF_ZONE_ID/settings/security_header" \
  -d '{
    "value": {
      "strict_transport_security": {
        "enabled": true,
        "max_age": 63072000,
        "include_subdomains": true,
        "preload": true,
        "nosniff": true
      }
    }
  }' | jq '.success'

# --- 5. Minimum TLS version ---
echo
echo "--- Step 5: Minimum TLS 1.2 ---"
cf -X PATCH "$CF_API/zones/$CF_ZONE_ID/settings/min_tls_version" \
  -d '{"value":"1.2"}' | jq '.success'

# --- 6. Cache rules: bypass /api/* and /relay/* ---
echo
echo "--- Step 6: Cache rules (bypass /api/*, /relay/*) ---"

# Delete existing rulesets of phase http_request_cache_settings to avoid conflicts.
existing_rulesets=$(cf "$CF_API/zones/$CF_ZONE_ID/rulesets?phase=http_request_cache_settings" \
  | jq -r '.result[]?.id // empty')

for rs_id in $existing_rulesets; do
  cf -X DELETE "$CF_API/zones/$CF_ZONE_ID/rulesets/$rs_id" > /dev/null 2>&1 || true
done

cf -X POST "$CF_API/zones/$CF_ZONE_ID/rulesets" \
  -d '{
    "name": "faultline cache bypass",
    "kind": "zone",
    "phase": "http_request_cache_settings",
    "rules": [
      {
        "expression": "(starts_with(http.request.uri.path, \"/api/\")) or (starts_with(http.request.uri.path, \"/relay/\"))",
        "description": "Bypass cache for API and relay endpoints",
        "action": "set_cache_settings",
        "action_parameters": {
          "cache": false
        }
      }
    ]
  }' | jq '.success'

# --- 7. Compression: pass through (do not re-compress) ---
echo
echo "--- Step 7: Gzip/Brotli passthrough ---"
# Cloudflare respects the origin's Content-Encoding. With "gzip" enabled at the
# zone level (default), CF will compress text responses but will NOT re-compress
# already-compressed payloads. SDK envelope payloads arrive with Content-Encoding: gzip
# from the Sentry SDK and pass through unchanged.
#
# Ensure compression is ON (default) so non-SDK responses still benefit.
cf -X PATCH "$CF_API/zones/$CF_ZONE_ID/settings/brotli" \
  -d '{"value":"on"}' | jq '.success'

# --- 8. Security headers via transform rule ---
echo
echo "--- Step 8: Response security headers (transform rule) ---"

# The application sets these headers itself (defense in depth), but we also set them
# at the Cloudflare edge as a belt-and-suspenders measure.
existing_transform=$(cf "$CF_API/zones/$CF_ZONE_ID/rulesets?phase=http_response_headers_transform" \
  | jq -r '.result[]?.id // empty')

for rs_id in $existing_transform; do
  cf -X DELETE "$CF_API/zones/$CF_ZONE_ID/rulesets/$rs_id" > /dev/null 2>&1 || true
done

cf -X POST "$CF_API/zones/$CF_ZONE_ID/rulesets" \
  -d '{
    "name": "faultline security headers",
    "kind": "zone",
    "phase": "http_response_headers_transform",
    "rules": [
      {
        "expression": "true",
        "description": "Security headers on all responses",
        "action": "rewrite",
        "action_parameters": {
          "headers": {
            "X-Content-Type-Options": { "operation": "set", "value": "nosniff" },
            "X-Frame-Options": { "operation": "set", "value": "DENY" },
            "Referrer-Policy": { "operation": "set", "value": "strict-origin-when-cross-origin" },
            "Permissions-Policy": { "operation": "set", "value": "camera=(), microphone=(), geolocation=()" }
          }
        }
      }
    ]
  }' | jq '.success'

echo
echo "=== Cloudflare setup complete ==="
echo
echo "Verify:"
echo "  curl -sI https://faultline.live/health"
echo "  # Should show cf-ray header + security headers + HSTS"
