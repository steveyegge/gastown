// Gas Town OpenCode plugin: Full parity with Claude hooks via OpenCode events.
// 
// Provides equivalent functionality to Claude's hooks:
// - SessionStart → session.created event
// - UserPromptSubmit → message.updated event (user role filter)
// - PreCompact → experimental.session.compacting hook
// - Stop → session.idle event
//
// Instrumentation: All actions logged with timestamps for E2E test analysis

export const GasTown = async ({ $, directory }) => {
  const role = (process.env.GT_ROLE || "").toLowerCase();
  const sessionId = process.env.OPENCODE_SESSION_ID || "unknown";
  const autonomousRoles = new Set(["polecat", "witness", "refinery", "deacon"]);
  const interactiveRoles = new Set(["mayor", "crew"]);
  let didInit = false;
  let lastIdleTime = 0;
  let gtPath = null;

  // Structured logging with timestamps
  const log = (level, event, message, data = {}) => {
    const ts = new Date().toISOString();
    const payload = {
      ts,
      level,
      event,
      message,
      role,
      session: sessionId,
      ...data
    };
    const prefix = `[gastown] ${ts}`;
    if (level === 'error') {
      console.error(`${prefix} [ERROR] ${event}: ${message}`, data.error || '');
    } else {
      console.log(`${prefix} [${level.toUpperCase()}] ${event}: ${message}`);
    }
  };

  // Find gt binary - check common locations first
  const findGt = async () => {
    if (gtPath) return gtPath;
    
    const candidates = [
      process.env.GT_BINARY_PATH,           // Explicit override
      `${process.env.HOME}/go/bin/gt`,      // Default GOPATH
      `${process.env.HOME}/.local/bin/gt`,  // User local
      `${process.env.GOPATH || ""}/bin/gt`, // Custom GOPATH
      "/usr/local/bin/gt",                  // System install
    ].filter(Boolean);
    
    for (const candidate of candidates) {
      try {
        await $`${candidate} version`.quiet();
        gtPath = candidate;
        log('info', 'init', `Found gt binary at: ${gtPath}`);
        return gtPath;
      } catch {
        // Continue to next candidate
      }
    }
    
    log('error', 'init', 'gt binary not found', { 
      checked: candidates,
      hint: 'Set GT_BINARY_PATH env var to specify location'
    });
    return null;
  };

  const run = async (cmd) => {
    const startTime = Date.now();
    try {
      const gt = await findGt();
      if (!gt) {
        log('warn', 'run', `Skipping: ${cmd} (gt not found)`);
        return { success: false, skipped: true };
      }
      // Replace "gt" at start of command with full path
      const fullCmd = cmd.replace(/^gt(\s|$)/, `${gt}$1`);
      log('info', 'run', `Executing: ${cmd}`);
      
      const result = await $`/bin/sh -c ${fullCmd}`.cwd(directory);
      const duration = Date.now() - startTime;
      
      log('info', 'run', `Success: ${cmd}`, { duration_ms: duration, exit: 0 });
      return { success: true, duration };
    } catch (err) {
      const duration = Date.now() - startTime;
      log('error', 'run', `Failed: ${cmd}`, { 
        duration_ms: duration,
        error: err?.message || String(err),
        exit: err?.exitCode
      });
      return { success: false, duration, error: err?.message };
    }
  };

  // SessionStart equivalent
  const onSessionCreated = async () => {
    if (didInit) return;
    didInit = true;
    
    log('info', 'hook', 'session.created triggered');
    
    await run("gt prime");
    
    if (autonomousRoles.has(role)) {
      log('info', 'hook', `Autonomous role (${role}): checking mail`);
      await run("gt mail check --inject");
    }
    
    await run("gt nudge deacon session-started");
    
    log('info', 'hook', 'session.created complete');
  };

  // UserPromptSubmit equivalent for interactive roles
  const onUserMessage = async () => {
    if (interactiveRoles.has(role)) {
      log('info', 'hook', `message.updated triggered for interactive role (${role})`);
      await run("gt mail check --inject");
    }
  };

  // PreCompact equivalent
  const onPreCompact = async () => {
    log('info', 'hook', 'pre-compact triggered');
    await run("gt prime");
    log('info', 'hook', 'pre-compact complete');
  };

  // Stop equivalent (with debouncing)
  const onIdle = async () => {
    const now = Date.now();
    const timeSinceLastIdle = now - lastIdleTime;
    
    // Debounce: only run if idle for > 5 seconds
    if (timeSinceLastIdle > 5000) {
      log('info', 'hook', 'session.idle triggered', { debounce_ms: timeSinceLastIdle });
      await run("gt costs record");
      lastIdleTime = now;
    } else {
      log('debug', 'hook', 'session.idle debounced', { ms_remaining: 5000 - timeSinceLastIdle });
    }
  };

  // Log plugin initialization
  log('info', 'init', 'Plugin loaded', { 
    role, 
    directory, 
    GT_BINARY_PATH: process.env.GT_BINARY_PATH || '(not set)',
    autonomousRoles: [...autonomousRoles],
    interactiveRoles: [...interactiveRoles]
  });

  return {
    // Event-based hooks
    event: async ({ event }) => {
      switch (event?.type) {
        case "session.created":
          await onSessionCreated();
          break;
        
        case "message.updated":
          // Check if it's a user message (not assistant)
          if (event.properties?.info?.role === "user") {
            await onUserMessage();
          }
          break;
        
        case "session.idle":
          await onIdle();
          break;
          
        default:
          // Log unhandled events for debugging
          if (event?.type) {
            log('debug', 'event', `Unhandled event: ${event.type}`);
          }
      }
    },
    
    // Pre-compaction hook (runs BEFORE compaction starts)
    "experimental.session.compacting": async (input, output) => {
      await onPreCompact();
      // Can customize compaction prompt if needed:
      // output.context.push("Additional context");
    },
  };
};
