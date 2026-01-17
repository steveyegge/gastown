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

  const run = async (cmd) => {
    try {
      await $`/bin/sh -lc ${cmd}`.cwd(directory);
    } catch (err) {
      console.error(`[gastown] ${cmd} failed`, err?.message || err);
    }
  };

  // SessionStart equivalent
  const onSessionCreated = async () => {
    if (didInit) return;
    didInit = true;
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
    await run("gt prime");
  };

  // Stop equivalent (with debouncing)
  const onIdle = async () => {
    const now = Date.now();
    // Debounce: only run if idle for > 5 seconds
    if (now - lastIdleTime > 5000) {
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
