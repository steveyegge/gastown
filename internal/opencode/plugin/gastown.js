// Gas Town OpenCode plugin: integrates with Gas Town multi-agent system.
// Replaces trigger message with context before LLM processing.

const TRIGGER_MESSAGE = "[GT_AGENT_INIT]";

export const GasTown = async ({ client, $ }) => {
  const role = (process.env.GT_ROLE || "").toLowerCase();
  const autoInit = process.env.GT_AUTO_INIT === "1";
  const autonomousRoles = new Set(["polecat", "witness", "refinery", "deacon"]);
  const defaultFallbackModels = [
    "openai/gpt-5.2-codex",
    "openai/gpt-5.1-codex",
    "openai/gpt-5.1",
  ];

  // Track transformed sessions to avoid duplicate processing
  const transformedSessions = new Set();
  const injectedSessions = new Set();
  const fallbackState = new Map();

  function parseFallbackModels() {
    const raw = process.env.GT_OPENCODE_MODEL_FALLBACKS || process.env.OPENCODE_MODEL_FALLBACKS;
    if (!raw) return defaultFallbackModels;
    return raw
      .split(",")
      .map(entry => entry.trim())
      .filter(Boolean);
  }

  function normalizeModel(value) {
    if (!value || typeof value !== "string") return "";
    return value.trim();
  }

  function splitModel(model) {
    if (!model || !model.includes("/")) return null;
    const [providerID, ...rest] = model.split("/");
    const modelID = rest.join("/").trim();
    if (!providerID || !modelID) return null;
    return { providerID, modelID };
  }

  function isCreditError(event) {
    const payload = JSON.stringify(event?.properties || {}).toLowerCase();
    return (
      payload.includes("insufficient") ||
      payload.includes("quota") ||
      payload.includes("credit") ||
      payload.includes("billing") ||
      payload.includes("402")
    );
  }

  function getNextFallbackModel(currentModel, fallbacks) {
    if (!fallbacks.length) return "";
    const current = normalizeModel(currentModel);
    const index = fallbacks.findIndex(model => model === current);
    if (index >= 0 && index + 1 < fallbacks.length) {
      return fallbacks[index + 1];
    }
    if (index === -1 && fallbacks.length > 0) {
      return fallbacks[0];
    }
    return "";
  }

  async function updateSessionModel(sessionID, model) {
    const parsed = splitModel(model);
    try {
      await client.session.update({
        path: { id: sessionID },
        body: { model: parsed || model },
      });
      return true;
    } catch {
      try {
        await client.session.update({
          path: { id: sessionID },
          body: { model },
        });
        return true;
      } catch {
        return false;
      }
    }
  }

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
   * Fetch mail injection payload for prompt-time updates.
   */
  async function fetchMailInject() {
    try {
      const mailOutput = await $`gt mail check --inject`.text();
      if (mailOutput?.trim()) {
        return `\n\n<gastown-mail>\n${mailOutput.trim()}\n</gastown-mail>`;
      }
    } catch {
      // Silent skip
    }
    return "";
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
      if (!autoInit) return;

      const triggerHandled = transformedSessions.has("done");

      for (const msg of output.messages || []) {
        if (msg.info?.role === "user") {
          const text = msg.parts?.map(p => p.text || "").join("") || "";

          if (text.trim() === TRIGGER_MESSAGE) {
            if (!triggerHandled) {
              transformedSessions.add("done");

              const context = await buildContext();
              if (context) {
                msg.parts = [{ type: "text", text: context }];
              }
            }
            break;
          }

          // Inject mail on user prompts (Claude UserPromptSubmit equivalent)
          const mailInject = await fetchMailInject();
          if (mailInject) {
            msg.parts = [{ type: "text", text: text + mailInject }];
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

      if (event.type === "session.error" && isCreditError(event)) {
        const sessionID = event.properties?.sessionID || event.properties?.info?.id;
        if (!sessionID) return;

        const fallbacks = parseFallbackModels();
        if (!fallbacks.length) return;

        const sessionContext = await getSessionContext(sessionID);
        const currentModel = normalizeModel(sessionContext?.model) || fallbackState.get(sessionID);
        const nextModel = getNextFallbackModel(currentModel, fallbacks);
        if (!nextModel || nextModel === currentModel || fallbackState.get(sessionID) === nextModel) return;

        const updated = await updateSessionModel(sessionID, nextModel);
        if (updated) {
          fallbackState.set(sessionID, nextModel);
          client.tui
            .showToast({
              body: {
                message: `Switched model to ${nextModel} after credit exhaustion`,
                variant: "warning",
              },
            })
            .catch(() => {});
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
        fallbackState.delete(sessionID);
        
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
