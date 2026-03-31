# Dogfooding: myndy_ios Integration

Faultline's first production dogfood target. myndy_ios is an actively developed iOS app
managed by Gas Town (mi- prefix, witness/refinery active).

## Architecture

```
myndy_ios (Sentry iOS SDK)
    │
    ▼ POST /api/2/envelope/
faultline server
    │
    ├── fingerprint + group → Dolt
    ├── threshold check (3 events/5min, or fatal=1)
    │
    ▼ bd new --rig myndy_ios
Gas Town (mi- beads)
    │
    ▼ polecat dispatched
autonomous fix → merge queue
```

## Faultline Configuration

```bash
# Add myndy_ios as project 2, routed to the myndy_ios rig
export FAULTLINE_PROJECTS="1:default_key:faultline,2:myndy_ios_sentry_key:myndy_ios"

# API URL for diagnostic links in beads (adjust for production)
export FAULTLINE_API_URL="http://localhost:8080"
```

- Project ID `2`: myndy_ios
- Public key: `myndy_ios_sentry_key` (replace with generated key)
- Target rig: `myndy_ios` (Gas Town rig with mi- prefix)

## DSN Discovery

Once faultline is running, the DSN can be retrieved via API:

```bash
curl http://localhost:8080/api/2/dsn/
```

Response:
```json
{
  "project_id": 2,
  "public_key": "myndy_ios_sentry_key",
  "rig": "myndy_ios",
  "dsn": "http://myndy_ios_sentry_key@localhost:8080/2",
  "endpoints": {
    "envelope": "http://localhost:8080/api/2/envelope/",
    "store": "http://localhost:8080/api/2/store/"
  }
}
```

## myndy_ios SDK Setup

Add to `Package.swift`:
```swift
dependencies: [
    .package(url: "https://github.com/getsentry/sentry-cocoa", from: "8.0.0"),
]
```

Initialize in `AppDelegate` or `@main`:
```swift
import Sentry

SentrySDK.start { options in
    options.dsn = "http://myndy_ios_sentry_key@<faultline-host>/2"
    options.environment = "dogfood"
    options.enableAutoSessionTracking = true
    options.attachStacktrace = true
    options.debug = true  // remove after verification
}
```

For local development, use a tunnel (e.g. ngrok) if the iOS device can't reach localhost.

## What Gets Captured

The Sentry iOS SDK (sentry-cocoa) automatically captures:

- **Crashes**: Mach exceptions, signals (SIGSEGV, SIGABRT, etc.)
- **Exceptions**: NSException, Swift Error
- **App lifecycle**: Sessions (foreground, background, crash-free rate)
- **Breadcrumbs**: UI interactions, network requests, navigation

Each event includes rich device context:
- Device model, family, architecture
- iOS version
- App version and build number
- Environment

Faultline extracts this context and includes it in beads filed to Gas Town,
so polecats diagnosing myndy_ios errors have full device context.

## Bead Labels

Beads created from myndy_ios events include:

| Label | Meaning |
|-------|---------|
| `platform:cocoa` | iOS/macOS SDK event |
| `error:unhandled` | New error pattern detected |
| `error:regression` | Previously resolved error reappeared |
| `critical` | Fatal-level error |

## Validation Checklist

1. **SDK compatibility**: Sentry iOS SDK envelope format is accepted
2. **Error-to-bead pipeline**: Events → fingerprint → threshold → `bd new --rig myndy_ios`
3. **Gas Town agentic loop**: Polecat picks up mi- bead → diagnoses → fixes → merge queue
4. **Regression detection**: Fix merged → new event within 24h → regression bead filed

## Monitoring

```bash
# Check events from myndy_ios
curl http://localhost:8080/api/2/issues/

# Check beads filed
curl http://localhost:8080/api/2/issues/?status=unresolved

# Dashboard
open http://localhost:8080/dashboard/projects/2/issues
```
