<!-- ox-hash: f1af0d27e6cd ver: 0.4.0 -->
Check SageOx project status including authentication, sync, and daemon health.

Use when:
- Checking if you're logged in or authenticated
- Verifying project initialization and configuration
- Checking sync status of ledger and team context
- Confirming the daemon is running and healthy
- Getting an overview of SageOx state for this repository

Keywords: status, auth, sync, health, logged in, initialized, daemon, check, state, connected

## Common Issues

### Not logged in
**Symptom:** Status shows authentication as missing or expired
**Solution:** Run `ox login` to authenticate with SageOx cloud

### Not initialized
**Symptom:** Status shows no SageOx configuration
**Solution:** Run `ox init` to initialize SageOx in this repository

### Daemon not running
**Symptom:** Status shows daemon as offline or unreachable
**Solution:** Run `ox daemon start` to start the background sync daemon

$ox status
