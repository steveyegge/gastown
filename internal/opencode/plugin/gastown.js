// Gas Town OpenCode plugin: integrates with Gas Town multi-agent system.
// Replaces trigger message with context before LLM processing.

const TRIGGER_MESSAGE = "[GT_AGENT_INIT]";

export const GasTown = async ({ client, $ }) => {
  const role = (process.env.GT_ROLE || "").toLowerCase();
  const autoInit = process.env.GT_AUTO_INIT === "1";
  const autonomousRoles = new Set(["polecat", "witness", "refinery", "deacon"]);

  // Track transformed sessions to avoid duplicate processing
  const transformedSessions = new Set();
  const injectedSessions = new Set();

  /**
   * Build Gas Town context from gt prime and optionally mail.
   */
  async function buildContext() {
    let contextParts = [];

    try {
      const primeOutput = await $`gt prime`.text();
      if (primeOutput?.trim()) {
        contextParts.push(primeOutput.trim());
      }
    } catch {
      // Silent skip
    }

    if (autonomousRoles.has(role)) {
      try {
        const mailOutput = await $`gt mail check --inject`.text();
        if (mailOutput?.trim()) {
          contextParts.push(`\n\n<gastown-mail>\n${mailOutput.trim()}\n</gastown-mail>`);
        }
      } catch {
        // Silent skip
      }
    }

    // Notify deacon (fire-and-forget)
    $`gt nudge deacon session-started`.text().catch(() => {});

    return contextParts.join("\n\n");
  }

  /**
   * Inject context string into session via API.
   * @param sessionID - Session to inject into
   * @param contextText - The context string to inject
   * @param sessionContext - Optional model/agent info for the request
   */
  async function injectContext(sessionID, contextText, sessionContext) {
    if (!contextText) return;

    try {
      await client.session.prompt({
        path: { id: sessionID },
        body: {
          noReply: true,
          model: sessionContext?.model,
          agent: sessionContext?.agent,
          parts: [{ type: "text", text: contextText, synthetic: true }],
        },
      });
    } catch {
      // Silent skip
    }
  }

  /**
   * Get model/agent context from session messages.
   */
  async function getSessionContext(sessionID) {
    try {
      const response = await client.session.messages({
        path: { id: sessionID },
        query: { limit: 50 },
      });

      for (const msg of response.data || []) {
        if (msg.info?.role === "user" && msg.info?.model) {
          return { model: msg.info.model, agent: msg.info.agent };
        }
      }
    } catch {
      // Silent skip
    }
    return undefined;
  }

  return {
    // Transform messages BEFORE LLM processing
    "experimental.chat.messages.transform": async (input, output) => {
      if (!autoInit || transformedSessions.has("done")) return;

      for (const msg of output.messages || []) {
        if (msg.info?.role === "user") {
          const text = msg.parts?.map(p => p.text || "").join("") || "";

          if (text.trim() === TRIGGER_MESSAGE) {
            transformedSessions.add("done");

            const context = await buildContext();
            if (context) {
              msg.parts = [{ type: "text", text: context }];
            }
            break;
          }
        }
      }
    },

    event: async ({ event }) => {
      // Track sessions for compaction handling
      if (event.type === "session.created") {
        const sessionID = event.properties?.info?.id || event.properties?.sessionID;
        if (sessionID) {
          injectedSessions.add(sessionID);
        }
      }

      // Re-inject context after compaction (context is lost during compaction)
      if (event.type === "session.compacted") {
        const sessionID = event.properties.sessionID;
        const sessionContext = await getSessionContext(sessionID);
        const contextText = await buildContext();
        await injectContext(sessionID, contextText, sessionContext);
      }

      if (event.type === "session.deleted") {
        const sessionID = event.properties?.info?.id;
        injectedSessions.delete(sessionID);
        
        // Record session cost (Stop hook equivalent)
        $`gt costs record --session ${sessionID}`.text().catch(() => {});
      }
    },

    // Preserve critical context during compaction
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
