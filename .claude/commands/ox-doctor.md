<!-- ox-hash: f4fc7ddade4a ver: 0.4.0 -->
Run diagnostic checks on SageOx configuration and integrations.

Use when:
- Something isn't working and you need to troubleshoot
- Verifying SageOx setup is correct after initialization
- Checking for configuration drift or broken integrations
- Debugging sync, auth, or daemon issues
- After upgrading ox CLI to verify compatibility

Keywords: doctor, diagnose, troubleshoot, fix, check, debug, health, verify, broken, issue

## Common Issues

### Setup required
**Symptom:** Doctor reports missing initialization
**Solution:** Run `ox init` to initialize SageOx in this repository

### Checks failed
**Symptom:** One or more diagnostic checks report failures
**Solution:** Follow the remediation steps doctor provides for each failed check

### Daemon not running
**Symptom:** Doctor reports daemon is unreachable
**Solution:** Run `ox daemon start` to start the background sync daemon

$ox doctor
