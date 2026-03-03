# Proxy Manual Testing Guide

This directory contains two testing scripts for the `gt-proxy-server` and `gt-proxy-client`:

## Quick Smoke Test (2 minutes)

For a fast verification that the proxy is functional:

```bash
./test-proxy-smoke.sh
```

**What it does:**
- ✓ Builds binaries
- ✓ Starts server on port 9876
- ✓ Issues a test certificate
- ✓ Runs a single `gt version` command through the proxy
- ✓ Automatically cleans up on exit

**Best for:** Quick CI/CD checks, rapid iteration during development

---

## Comprehensive Manual Test (5-10 minutes)

For thorough validation with multiple test scenarios:

```bash
./test-proxy-manual.sh
```

**What it does:**
- ✓ Builds binaries
- ✓ Starts server and keeps it running
- ✓ Issues polecat certificate
- ✓ Tests client connection
- ✓ Runs multiple `gt` commands:
  - `gt --version`
  - `gt status`
  - `gt convoy --help`
- ✓ Tests rate limiting (25 rapid requests)
- ✓ Displays server health and logs
- ✓ Keeps server running for manual testing

**Best for:** Thorough verification, manual exploration, debugging

**Exit:** Press `Ctrl+C` to stop the server and cleanup.

---

## Manual Testing Examples

Once a testing script is running, you can manually test from another terminal:

### Basic Command Execution

```bash
curl -s \
  --cert /tmp/gt-proxy-quick-test/client.crt \
  --key /tmp/gt-proxy-quick-test/client.key \
  --cacert /tmp/gt-proxy-quick-test/ca-client.crt \
  --resolve gt-proxy-server:9876:127.0.0.1 \
  -X POST \
  -H "Content-Type: application/json" \
  -d '{"argv":["gt","version"]}' \
  https://gt-proxy-server:9876/v1/exec
```

### Test Different Commands

```bash
# List ready issues
curl -s --cert ... -d '{"argv":["bd","ready"]}' https://localhost:9876/v1/exec

# Show an issue
curl -s --cert ... -d '{"argv":["bd","show","123"]}' https://localhost:9876/v1/exec

# Check version
curl -s --cert ... -d '{"argv":["gt","version"]}' https://localhost:9876/v1/exec
```

### Rate Limiting Test

The proxy has default rate limits configured:
- **Per-client sustained rate:** 10 req/s
- **Per-client burst:** 20 requests
- **Global concurrent:** 32 processes

You can trigger rate limiting with rapid requests and expect HTTP 429:

```bash
for i in {1..50}; do
  curl -s --cert ... -d '{"argv":["version"]}' https://localhost:9876/v1/exec &
done
wait
```

### Monitoring Server Logs

```bash
# Smoke test logs
tail -f /tmp/gt-proxy-quick-test/server.log

# Manual test logs
tail -f /tmp/gt-proxy-test/logs/server.log
```

---

## Environment Variables

You can customize the test behavior with environment variables:

```bash
# Use a different port
PROXY_PORT=9999 ./test-proxy-smoke.sh

# Use a different town root (where gt/bd are located)
TOWN_ROOT=/custom/gt/path ./test-proxy-manual.sh

# Use a different CA directory
CA_DIR=/custom/ca/path ./test-proxy-manual.sh
```

---

## Troubleshooting

### Server fails to start
- Check if the port is already in use: `lsof -i :9876`
- Check server logs for errors
- Ensure `GT_TOWN` is set correctly (default: `~/gt`)

### Certificate errors
- Ensure OpenSSL is installed: `openssl version`
- Check CA files are created in the test directory
- Verify certificate CN matches the expected format: `gt-<rig>-<name>`

### Connection refused
- Verify server is running: `ps aux | grep gt-proxy-server`
- Check firewall settings: `netstat -an | grep 9876`
- Ensure certificate files exist and are readable

### HTTP 403 (Forbidden)
- Check that certificate CN matches format `gt-<rig>-<name>`
- Verify the subcommand is in the allowed list
- Check server logs for the actual error

### HTTP 429 (Too Many Requests)
- This is expected when rate limits are exceeded
- Wait a moment and retry
- It validates rate limiting is working correctly

---

## Testing Checklist

Before submitting a PR with proxy changes, verify:

- [ ] Smoke test passes
- [ ] Manual test passes
- [ ] Multiple commands execute successfully
- [ ] Rate limiting works (returns 429 when exceeded)
- [ ] Invalid certificates are rejected
- [ ] Server gracefully handles connection errors
- [ ] Logs are clear and informative

---

## Docker Integration Testing

For testing the proxy in a containerized environment:

```bash
# Build a test container image
docker build -f Dockerfile.e2e -t gastown-test:latest .

# Run tests in container
docker run --rm \
  -v /var/run/docker.sock:/var/run/docker.sock \
  gastown-test:latest \
  go test -v ./internal/proxy/...
```

See [Dockerfile.e2e](../Dockerfile.e2e) and the CI workflows for more details.
