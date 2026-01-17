// Gas Town OpenCode plugin: hooks SessionStart/Compaction via events.
export const GasTown = async ({ $, directory }) => {
  const role = (process.env.GT_ROLE || "").toLowerCase();
  const autonomousRoles = new Set(["polecat", "witness", "refinery", "deacon"]);
  let didInit = false;

  const run = async (cmd) => {
    try {
      await $`/bin/sh -lc ${cmd}`.cwd(directory);
    } catch (err) {
      console.error(`[gastown] ${cmd} failed`, err?.message || err);
    }
  };

  const onSessionCreated = async () => {
    if (didInit) return;
    didInit = true;
    await run("gt prime");
    if (autonomousRoles.has(role)) {
      await run("gt mail check --inject");
    }
    await run("gt nudge deacon session-started");
  };

  const onSessionCompacted = async () => {
    // Re-inject Gas Town context after compaction
    await run("gt prime");
  };

  return {
    event: async ({ event }) => {
      if (event?.type === "session.created") {
        await onSessionCreated();
      }
      if (event?.type === "session.compacted") {
        await onSessionCompacted();
      }
    },
    // Customize compaction to preserve critical Gas Town context
    "experimental.session.compacting": async ({ sessionID }, output) => {
      const roleDisplay = role || "unknown";
      output.context.push(`
## Gas Town Multi-Agent System

You are working in the Gas Town multi-agent workspace.

**Critical Actions After Compaction:**
- Run \`gt prime\` to restore full Gas Town context
- Check your hook with \`gt mol status\` or \`gt hook\`
- If work is hooked, execute immediately per GUPP (Gas Town Universal Propulsion Principle)

**Current Session:**
- Role: ${roleDisplay}
- Session ID: ${sessionID}

**Remember:** The hook having work IS your assignment. Execute without waiting for confirmation.
`);
    },
  };
};
