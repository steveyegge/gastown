// Gas Town OpenCode plugin: hooks SessionStart/Compaction via events.
// Injects gt prime context into the system prompt via experimental.chat.system.transform.
export const GasTown = async ({ $, directory }) => {
  const role = (process.env.GT_ROLE || "").toLowerCase();
  const autonomousRoles = new Set(["polecat", "witness", "refinery", "deacon"]);
  let didInit = false;

  // Promise-based context loading ensures the system transform hook can
  // await the result even if session.created hasn't resolved yet.
  let primePromise = null;

  const captureRun = async (cmd) => {
    try {
      // .text() captures stdout as a string and suppresses terminal echo.
      return await $`/bin/sh -lc ${cmd}`.cwd(directory).text();
    } catch (err) {
      console.error(`[gastown] ${cmd} failed`, err?.message || err);
      return "";
    }
  };

  const loadPrime = async () => {
    let context = await captureRun("gt prime");
    if (autonomousRoles.has(role)) {
      const mail = await captureRun("gt mail check --inject");
      if (mail) {
        context += "\n" + mail;
      }
    }
    // NOTE: session-started nudge to deacon removed — it interrupted
    // the deacon's await-signal backoff. Deacon wakes on beads activity.
    return context;
  };

  // Check if OTEL is available
  const otelAvailable = !!(process.env.GT_OTEL_METRICS_URL || process.env.GT_OTEL_LOGS_URL);

  const agentType = "opencode";

  return {
    event: async ({ event }) => {
      if (event?.type === "session.created") {
        if (didInit) return;
        didInit = true;
        // Start loading prime context early; system.transform will await it.
        primePromise = loadPrime();

        // Emit agent.instantiate event for OTEL telemetry
        if (otelAvailable) {
          try {
            const role = (process.env.GT_ROLE || "").toLowerCase();
            const sessionID = event.properties?.info?.id || "";
            const runID = sessionID || "unknown";
            const rig = process.env.GT_RIG || "";
            await $`gt activity emit agent.instantiate --run-id ${runID} --role ${role} --session ${sessionID} --rig ${rig} --agent-type opencode`;
            console.error("[gastown] agent.instantiate event emitted");
          } catch (err) {
            console.error("[gastown] agent.instantiate failed", err?.message || err);
          }
        }
      }
      if (event?.type === "session.compacted") {
        // Reset so next system.transform gets fresh context.
        primePromise = loadPrime();
      }
      if (event?.type === "session.deleted") {
        const sessionID = event.properties?.info?.id;
        if (sessionID) {
          await $`gt costs record --session ${sessionID}`.catch(() => {});
        }
      }
        // Emit agent.terminate event for OTEL telemetry
        if (otelAvailable) {
          try {
            await $`gt activity emit agent.terminate --session ${sessionID} --agent-type opencode`; 
            console.error("[gastown] agent.terminate event emitted");
          } catch (err) {
            console.error("[gastown] agent.terminate failed", err?.message || err);
          }
        }
    },
    "experimental.chat.system.transform": async (input, output) => {
      // If session.created hasn't fired yet, start loading now.
      if (!primePromise) {
        primePromise = loadPrime();
      }
      const context = await primePromise;
      if (context) {
        output.system.push(context);
      } else {
        // Reset so next transform retries instead of pushing empty forever.
        primePromise = null;
      }
    },
    "experimental.session.compacting": async ({ sessionID }, output) => {
      const roleDisplay = role || "unknown";
      output.context.push(`
## Gas Town Multi-Agent System

**After Compaction:** Run \`gt prime\` to restore full context.
**Check Hook:** \`gt hook\` - if work present, execute immediately (GUPP).
**Role:** ${roleDisplay}
`);
    },
    "experimental.tool.use": async (toolCall, context) => {
      // Track tool calls as child spans for OTEL telemetry
      if (otelAvailable) {
        try {
          const sessionID = context.sessionId || "";
          const toolName = toolCall.name || "unknown";
          await $`gt activity emit tool_call --session ${sessionID} --tool ${toolName} --agent-type opencode`;
          console.error(`[gastown] tool_call event emitted for ${toolName}`);
        } catch (err) {
          console.error("[gastown] tool_call failed", err?.message || err);
        }
      }
    },
  };
