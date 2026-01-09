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

  return {
    event: async ({ event }) => {
      if (event?.type === "session.created") {
        await onSessionCreated();
      }
    },
  };
};
