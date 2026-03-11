<!-- ox-hash: 31dd5815b853 ver: 0.4.0 -->
# SageOx Commands Reference

Essential ox commands for team context:

## Get Project Conventions
```bash
ox conventions
```
Returns verified SAGEOX.md content with coding standards and team patterns.

## Check Project Health
```bash
ox doctor
```
Diagnostic checks for SageOx configuration, signatures, and integration.

## Update Conventions
```bash
ox update
```
Sync latest conventions from cloud (requires authentication).

## Initialize SageOx
```bash
ox init
```
Enable SageOx for a new project (creates .sageox/ directory).

## Check Status
```bash
ox status
```
Check authentication, project initialization, sync, and daemon health.

## Session Recording
```bash
ox agent <id> session start   # begin recording
ox agent <id> session stop    # stop and save to ledger
```
Record agent sessions to the project ledger for team visibility.

## Diagnostics
```bash
ox doctor
```
Run diagnostic checks on SageOx configuration and integrations.

---
Run `ox --help` for full command list.
