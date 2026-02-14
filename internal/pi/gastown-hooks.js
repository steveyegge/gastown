// Gas Town Pi Extension — Simple Mode (Claude hooks equivalent)
// Deploys the same lifecycle hooks as Claude's settings-autonomous.json
// but using pi's extension API.
//
// Events mapped:
//   startup             → gt prime --hook (capture context)
//   before_agent_start  → inject captured context into system prompt
//   tool_call           → gt tap guard pr-workflow (on git push/pr create)
//   (no shutdown event) → gt costs record (NOT supported by pi extension API)
//
// Loaded via: pi -e gastown-hooks.js

export default (pi) => {
  let primeContext = null;
  let contextInjected = false;

  // Startup — run gt prime and capture context for injection
  pi.on("startup", async (event, context) => {
    try {
      const result = await pi.exec("gt", ["prime", "--hook"]);
      if (result.code === 0 && result.stdout.trim()) {
        primeContext = result.stdout.trim();
        console.error("[gastown] gt prime captured (" + primeContext.length + " chars)");
      } else {
        console.error("[gastown] gt prime returned no output (code=" + result.code + ")");
      }
    } catch (e) {
      console.error("[gastown] gt prime failed:", e.message);
    }

    // Check mail
    try {
      const mailResult = await pi.exec("gt", ["mail", "check", "--inject"]);
      if (mailResult.code === 0 && mailResult.stdout.trim()) {
        // Append mail context to prime context
        if (primeContext) {
          primeContext += "\n\n" + mailResult.stdout.trim();
        } else {
          primeContext = mailResult.stdout.trim();
        }
        console.error("[gastown] mail context appended");
      }
    } catch (e) {
      console.error("[gastown] gt mail check failed:", e.message);
    }
  });

  // BeforeAgentStart — inject prime context into the session
  pi.on("before_agent_start", async (event, context) => {
    // Inject prime context on first prompt
    if (primeContext && !contextInjected) {
      contextInjected = true;
      console.error("[gastown] injecting prime context into session");
      return {
        message: {
          customType: "gastown-prime",
          content: primeContext,
          display: false,
        },
        systemPrompt: event.systemPrompt + "\n\n" + primeContext,
      };
    }
  });

  // PreToolUse equivalent — guard dangerous git operations
  pi.on("tool_call", async (event, context) => {
    if (event.toolName === "bash" && event.input?.command) {
      const cmd = event.input.command;
      if (
        cmd.includes("git push") ||
        cmd.includes("gh pr create") ||
        cmd.includes("git checkout -b")
      ) {
        try {
          const result = await pi.exec("gt", ["tap", "guard", "pr-workflow"]);
          if (result.code !== 0) {
            return { block: true, reason: result.stderr || "gt tap guard rejected this operation" };
          }
        } catch (e) {
          console.error("[gastown] gt tap guard failed:", e.message);
        }
      }
    }
  });

  // NOTE: Pi extension API has no session shutdown/exit event.
  // Cost recording (gt costs record) must be handled externally,
  // e.g. by the witness or a wrapper script after pi exits.
};
