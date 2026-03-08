# macOS sandbox-exec Research Report

**Bead:** gt-6qt
**Date:** 2026-03-08
**Blocks:** gt-2pb (Spike: macOS sandbox-exec polecat isolation)

## 1. Is sandbox-exec still functional despite deprecation?

**YES — fully functional on macOS Sequoia 15.x despite deprecation.**

- Man page says DEPRECATED but it works without issues
- No runtime warnings observed during local testing on this machine
- macOS itself uses the underlying Seatbelt kernel extension extensively
- System sandbox profiles live in `/usr/share/sandbox/` and `/System/Library/Sandbox/Profiles/` (100+ profiles)
- Used in production by: Claude Code (Anthropic), OpenAI Codex, Bazel, Chromium
- Won't go away soon — Apple's own services depend on the kernel mechanism

The deprecation is a "please don't use this" signal, not an imminent removal. The kernel-level sandboxing subsystem (Seatbelt/MACF) must remain for Apple's own use.

## 2. Can it restrict filesystem paths (deny default, allow specific)?

**YES — core capability, works well.**

**Operations:** `file-read*`, `file-write*`, `file-read-data`, `file-read-metadata`, `file-write-create`, etc.

**Path filters:**
- `(literal "/exact/path")` — exact match
- `(subpath "/dir")` — dir and all descendants
- `(regex "^/pattern")` — POSIX regex

**Example (deny-default, allow specific):**
```scheme
(version 1)
(deny default)
(allow file-read* (subpath "/usr/lib"))
(allow file-read* (subpath "/System/Library"))
(allow file-read* file-write* (subpath (param "PROJECT_DIR")))
(allow file-write* (subpath "/private/tmp"))
```

**Parameterized paths:** `sandbox-exec -D PROJECT_DIR=/path -f profile.sb command`

**Local test confirmed:** Reading `/etc/passwd` denied with exit 134 when only `/usr` allowed.

## 3. Can it restrict network to loopback only?

**YES — works reliably.**

**Network operations:** `network*`, `network-outbound`, `network-inbound`, `network-bind`

**Filters:** `(local ip "localhost:*")`, `(remote ip "localhost:*")`, `(remote unix-socket)`

```scheme
;; Loopback-only profile
(allow network* (local ip "localhost:*"))
(allow network* (remote ip "localhost:*"))
(allow network* (remote unix-socket))
```

**Local test confirmed:** `curl` to external host denied with exit 6 under `(deny default)`.

OpenAI Codex found network enforcement "too effective" — their `network_access=true` config was silently ignored by seatbelt (GitHub issues #6807, #10390).

## 4. Can it restrict process spawning (allow only specific binaries)?

**YES — via `process-exec` and `process-fork` operations.**

```scheme
(allow process-exec (literal "/usr/bin/python3"))
(deny process-exec (literal "/bin/ls"))
```

**Key behaviors:**
- Children inherit parent's sandbox — no escape possible
- `process-exec` controls which binaries can be exec'd
- `process-fork` controls fork/vfork permission
- Note: `/bin/sh` redirects to `/bin/bash` on macOS, so both must be allowed

**Local test confirmed:** `/bin/ls` denied (exit 126) while `/bin/echo` allowed in same shell session.

## 5. Does it work with Node.js (Claude Code runtime)?

**YES — confirmed working with Node.js v25.6.0 on this machine.**

**Required SBPL rules for Node.js:**

| Rule | Why |
|------|-----|
| `(allow file-ioctl)` | Terminal raw mode / `setRawMode` |
| `(allow mach-host*)` | `os.cpus()` / CPU detection |
| `(allow pseudo-tty)` | PTY allocation |
| `(allow ipc-posix-sem)` | Semaphores |
| `(allow iokit-open)` | IOKit access |
| Device paths: `/dev/ptmx`, `/dev/ttys*` | PTY devices |
| `/dev/random`, `/dev/urandom` | Crypto/random |
| `/private/var/folders`, `/private/tmp` | Temp directories |

**Known issues WITHOUT these rules:**
- `setRawMode` fails with errno:1 (needs `file-ioctl`)
- `os.cpus()` returns empty array (needs `mach-host*`)
- npm/yarn serializes all work (cascading from zero CPUs)
- Python multiprocessing fails (needs `ipc-posix-sem`)

**Production users:** Claude Code (Anthropic), Codex (OpenAI), ai-jail

## 6. Does it interfere with code signing / Gatekeeper?

**Generally NO.**

- sandbox-exec does not check or enforce code signing
- Operates at different layer (kernel MAC framework)
- Binaries don't need special signing to be sandboxed
- `sandbox-exec` itself is SIP-protected at `/usr/bin/sandbox-exec`
- Coexists with SIP — complementary layers in defense-in-depth
- One minor interaction: sandboxed apps apply quarantine xattr to created files
- Running Node.js under sandbox-exec does NOT trigger Gatekeeper warnings

## 7. .sb Profile Syntax

### Header
```scheme
(version 1)
```

### Default Action
```scheme
(deny default)    ;; whitelist mode (recommended for security)
(allow default)   ;; blacklist mode
```

### Debug / Logging
```scheme
(debug deny)      ;; log denied operations to system log
(debug all)       ;; log all operations (verbose)
```

### Rule Structure
```scheme
(allow|deny  operation  [filter...])
```

### Complete Operation Reference

**File:** `file*`, `file-read*`, `file-read-data`, `file-read-metadata`, `file-read-xattr`, `file-write*`, `file-write-data`, `file-write-create`, `file-write-flags`, `file-write-mode`, `file-write-mount`, `file-write-owner`, `file-write-setugid`, `file-write-times`, `file-write-unmount`, `file-write-xattr`, `file-ioctl`, `file-revoke`, `file-chroot`

**Network:** `network*`, `network-outbound`, `network-inbound`, `network-bind`

**Process:** `process*`, `process-exec`, `process-fork`

**IPC:** `ipc*`, `ipc-posix*`, `ipc-posix-sem`, `ipc-posix-shm`, `ipc-sysv*`, `ipc-sysv-msg`, `ipc-sysv-sem`, `ipc-sysv-shm`

**Mach:** `mach*`, `mach-bootstrap`, `mach-lookup`, `mach-priv*`, `mach-priv-host-port`, `mach-priv-task-port`, `mach-task-name`, `mach-per-user-lookup`, `mach-host*`

**System:** `sysctl*`, `sysctl-read`, `sysctl-write`, `system*`, `system-acct`, `system-audit`, `system-fsctl`, `system-lcid`, `system-mac-label`, `system-nfssvc`, `system-reboot`, `system-set-time`, `system-socket`, `system-swap`, `system-write-bootstrap`

**Other:** `pseudo-tty`, `iokit-open`, `job-creation`, `process-info*`, `signal`, `send-signal`

### Filter Predicates

**Path filters:**
```scheme
(literal "/exact/path/to/file")
(subpath "/dir")                        ;; matches /dir and all descendants
(regex "^/pattern/.*\\.txt$")
```

**Network filters:**
```scheme
(local ip "localhost:*")
(remote ip "localhost:80")
(remote unix-socket)
(local tcp "*:8080")
```

**Mach service filters:**
```scheme
(global-name "com.apple.system.logger")
(local-name "com.example.service")
```

**Logical combinators:**
```scheme
(require-all (subpath "/tmp") (require-not (vnode-type SYMLINK)))
(require-any (literal "/path/a") (literal "/path/b"))
```

**Other filters:**
```scheme
(signing-identifier "com.example.app")
(target same-sandbox)
(sysctl-name "kern.hostname")
```

### Action Modifiers
```scheme
(deny (with no-report) file-write*)              ;; suppress violation log
(deny (with send-signal SIGUSR1) network*)
(allow (with report) sysctl (sysctl-name "...")) ;; log even though allowed
```

### Parameters
```scheme
;; CLI: sandbox-exec -D KEY=value -f profile.sb command
(allow file-read* file-write* (subpath (param "PROJECT_DIR")))

;; Conditional logic
(if (equal? (param "FEATURE") "YES")
  (allow network-outbound))
```

### Imports
```scheme
(import "/System/Library/Sandbox/Profiles/bsd.sb")
```

### Complete Node.js Sandbox Profile

```scheme
(version 1)
(deny default)
(debug deny)

;; Process control
(allow process-exec)
(allow process-fork)
(allow signal (target same-sandbox))
(allow process-info* (target same-sandbox))

;; System info (Node.js needs these)
(allow sysctl-read)
(allow mach-host*)
(allow mach-lookup)
(allow iokit-open)
(allow ipc-posix-sem)
(allow ipc-posix-shm-read*)

;; Terminal support
(allow file-ioctl)
(allow pseudo-tty)
(allow file-read* file-write* (literal "/dev/ptmx"))
(allow file-read* file-write* (regex "^/dev/ttys[0-9]+"))

;; Standard devices
(allow file-write* (literal "/dev/null"))
(allow file-write* (literal "/dev/zero"))
(allow file-read* (literal "/dev/random"))
(allow file-read* (literal "/dev/urandom"))

;; System read access (read-only)
(allow file-read* (subpath "/usr/lib"))
(allow file-read* (subpath "/usr/bin"))
(allow file-read* (subpath "/usr/sbin"))
(allow file-read* (subpath "/System"))
(allow file-read* (subpath "/Library"))
(allow file-read* (subpath "/private/etc"))
(allow file-read-metadata)

;; Homebrew (if Node.js installed via Homebrew)
(allow file-read* (subpath "/opt/homebrew"))

;; Project directory (read + write)
(allow file-read* file-write* (subpath (param "PROJECT_DIR")))

;; Temp directories
(allow file-read* file-write* (subpath (param "TMPDIR")))
(allow file-read* file-write* (subpath "/private/var/folders"))
(allow file-read* file-write* (subpath "/private/tmp"))

;; Network: loopback only
(allow network* (local ip "localhost:*"))
(allow network* (remote ip "localhost:*"))
(allow network* (remote unix-socket))
```

**Usage:**
```bash
sandbox-exec -D PROJECT_DIR=/path/to/project -D TMPDIR=$TMPDIR -f profile.sb node app.js
```

## 8. Alternatives Assessment

### App Sandbox (Entitlements-based)
- Requires `.app` bundle and code signing entitlements
- Cannot sandbox arbitrary CLI tools
- **NOT viable** for agent sandboxing use case

### Endpoint Security Framework
- Requires System Extension + Apple-issued entitlement (`com.apple.developer.endpoint-security.client`)
- Designed for security products (antivirus, MDM), not process sandboxing
- Massive overhead for this use case
- **NOT viable** for lightweight agent isolation

### Third-party tools (all wrap sandbox-exec underneath)
- **ai-jail** (Rust, ~880KB): Generates SBPL at runtime, supports Claude Code + Codex
- **claude-sandbox**: macOS-specific Claude Code sandboxing
- **scode**: "A Seatbelt for AI Coding"
- **agent-seatbelt-sandbox**: Focus on preventing data egress
- **Alcoholless**: Lightweight sandbox for Homebrew/AI agents

### Bottom Line

**sandbox-exec is the only practical option for CLI tool sandboxing on macOS.** No Apple-supported replacement exists for this use case. Both Anthropic (Claude Code) and OpenAI (Codex) use it in production. The deprecation is cosmetic — the kernel subsystem is permanent infrastructure.

Apple's Quinn "The Eskimo" from Developer Technical Support acknowledged this gap on Apple Developer Forums, noting that Endpoint Security is "a completely different mechanism" without providing a direct sandbox-exec replacement for CLI use cases.

## Local Test Results (macOS Sequoia, 2026-03-08)

| Test | Result |
|------|--------|
| Basic sandbox-exec invocation | Works, no warnings |
| Filesystem deny (read /etc/passwd with only /usr allowed) | Denied, exit 134 |
| Network deny (curl external host) | Denied, exit 6 |
| Node.js v25.6.0 under sandbox | Works, CPUs detected (14), platform correct |
| Process-exec restriction (deny /bin/ls, allow /bin/echo) | ls exit 126, echo works |
| /bin/sh → /bin/bash redirect | Must allow both for shell scripts |
