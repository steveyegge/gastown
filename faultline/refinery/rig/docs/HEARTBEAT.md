# Faultline Heartbeat

Sentry SDKs are passive — they only report when errors occur. A healthy
service sends nothing, making it invisible on the faultline dashboard.

The heartbeat solves this: a lightweight POST on startup (and optionally
on an interval) that tells faultline "I'm alive." No error events are
created — it just updates the project's last-seen timestamp.

## Endpoint

```
POST /api/{project_id}/heartbeat
X-Sentry-Auth: Sentry sentry_key={public_key}
```

Returns `{"status": "ok"}`. No body required.

## Go (gtfaultline)

Automatic — `gtfaultline.Init()` sends a heartbeat on startup.

## Python

Add after `sentry_sdk.init()`:

```python
import os, threading, time, requests

def _faultline_heartbeat():
    """Send periodic heartbeat to faultline."""
    dsn = os.environ.get("FAULTLINE_DSN", "")
    if not dsn:
        return
    # Parse DSN: http://key@host:port/project_id
    try:
        from urllib.parse import urlparse
        parsed = urlparse(dsn)
        key = parsed.username
        base = f"{parsed.scheme}://{parsed.hostname}"
        if parsed.port:
            base += f":{parsed.port}"
        project_id = parsed.path.strip("/")
        url = f"{base}/api/{project_id}/heartbeat"
        headers = {"X-Sentry-Auth": f"Sentry sentry_key={key}"}
    except Exception:
        return

    while True:
        try:
            requests.post(url, headers=headers, timeout=5)
        except Exception:
            pass
        time.sleep(300)  # every 5 minutes

# Start heartbeat in background thread
threading.Thread(target=_faultline_heartbeat, daemon=True).start()
```

Or as a one-liner on startup (no interval):

```python
def faultline_heartbeat():
    import os, requests
    from urllib.parse import urlparse
    dsn = os.environ.get("FAULTLINE_DSN", "")
    if not dsn:
        return
    parsed = urlparse(dsn)
    base = f"{parsed.scheme}://{parsed.hostname}" + (f":{parsed.port}" if parsed.port else "")
    try:
        requests.post(
            f"{base}/api/{parsed.path.strip('/')}/heartbeat",
            headers={"X-Sentry-Auth": f"Sentry sentry_key={parsed.username}"},
            timeout=5,
        )
    except Exception:
        pass

faultline_heartbeat()
```

## Node.js / TypeScript

```typescript
function faultlineHeartbeat() {
  const dsn = process.env.FAULTLINE_DSN;
  if (!dsn) return;
  try {
    const url = new URL(dsn);
    const key = url.username;
    const projectId = url.pathname.replace(/\//g, "");
    const base = `${url.protocol}//${url.host}`;
    fetch(`${base}/api/${projectId}/heartbeat`, {
      method: "POST",
      headers: { "X-Sentry-Auth": `Sentry sentry_key=${key}` },
    }).catch(() => {});
  } catch {}
}

// On startup
faultlineHeartbeat();
// Optionally every 5 minutes
setInterval(faultlineHeartbeat, 300_000);
```

## Swift (iOS)

```swift
func faultlineHeartbeat() {
    guard let dsnString = ProcessInfo.processInfo.environment["FAULTLINE_DSN"],
          let dsn = URL(string: dsnString) else { return }
    let key = dsn.user ?? ""
    let projectId = dsn.path.replacingOccurrences(of: "/", with: "")
    let base = "\(dsn.scheme ?? "https")://\(dsn.host ?? "")\(dsn.port.map { ":\($0)" } ?? "")"
    var request = URLRequest(url: URL(string: "\(base)/api/\(projectId)/heartbeat")!)
    request.httpMethod = "POST"
    request.setValue("Sentry sentry_key=\(key)", forHTTPHeaderField: "X-Sentry-Auth")
    URLSession.shared.dataTask(with: request).resume()
}
```

## What it does

- Updates `last_heartbeat` on the project in faultline's database
- Dashboard shows "● Running" if heartbeat received within 24 hours
- No events, issues, or beads are created
- Failures are silently ignored (non-blocking)
