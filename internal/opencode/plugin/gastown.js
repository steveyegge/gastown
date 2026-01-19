// Gas Town OpenCode plugin: Full parity with Claude hooks via OpenCode events.
// 
// Provides equivalent functionality to Claude's hooks:
// - SessionStart → session.created event
// - UserPromptSubmit → message.updated event (user role filter)
// - PreCompact → experimental.session.compacting hook
// - Stop → session.idle event
export const GasTown = async ({ $, directory }) => {
  const role = (process.env.GT_ROLE || "").toLowerCase();
  const autonomousRoles = new Set(["polecat", "witness", "refinery", "deacon"]);
  const interactiveRoles = new Set(["mayor", "crew"]);
  let didInit = false;
  let lastIdleTime = 0;
  let gtPath = null;

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
        // Try to run with --version to verify it works
        await $`${candidate} version`.quiet();
        gtPath = candidate;
        console.log(`[gastown] Found gt at: ${gtPath}`);
        return gtPath;
      } catch {
        // Continue to next candidate
      }
    }
    
    console.error("[gastown] gt binary not found in any known location");
    console.error("[gastown] Checked:", candidates.join(", "));
    console.error("[gastown] Set GT_BINARY_PATH env var to specify location");
    return null;
  };

  const run = async (cmd) => {
    try {
      const gt = await findGt();
      if (!gt) {
        console.error(`[gastown] Skipping: ${cmd} (gt not found)`);
        return;
      }
      // Replace "gt" at start of command with full path
      const fullCmd = cmd.replace(/^gt(\s|$)/, `${gt}$1`);
      console.log(`[gastown] Running: ${fullCmd}`);
      await $`/bin/sh -c ${fullCmd}`.cwd(directory);
      console.log(`[gastown] Success: ${cmd}`);
    } catch (err) {
      console.error(`[gastown] ${cmd} failed`, err?.message || err);
    }
  };

  // SessionStart equivalent
  const onSessionCreated = async () => {
    if (didInit) return;
    didInit = true;
    console.log("[gastown] session.created hook triggered");
    await run("gt prime");
    if (autonomousRoles.has(role)) {
      await run("gt mail check --inject");
    }
    await run("gt nudge deacon session-started");
  };

  // UserPromptSubmit equivalent for interactive roles
  const onUserMessage = async () => {
    if (interactiveRoles.has(role)) {
      await run("gt mail check --inject");
    }
  };

  // PreCompact equivalent
  const onPreCompact = async () => {
    console.log("[gastown] pre-compact hook triggered");
    await run("gt prime");
  };

  // Stop equivalent (with debouncing)
  const onIdle = async () => {
    const now = Date.now();
    // Debounce: only run if idle for > 5 seconds
    if (now - lastIdleTime > 5000) {
      console.log("[gastown] idle hook triggered");
      await run("gt costs record");
      lastIdleTime = now;
    }
  };

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
