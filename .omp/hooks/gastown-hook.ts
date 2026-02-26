// Gas Town oh-my-pi (omp) hook — lifecycle integration for Gas Town agents.
// Mirrors the same events as Claude's settings-autonomous.json and pi-mono's gastown-hooks.js.
// Inspired by ProbabilityEngineer/pi-mono gastown integration:
// https://github.com/ProbabilityEngineer/pi-mono
//
// Events mapped:
//   session_start       → gt prime --hook (capture context)
//   before_agent_start  → inject captured context into system prompt
//   session.compacting  → inject compaction recovery instructions
//   tool_call           → gt tap guard pr-workflow (on git push/pr create)
//   session_shutdown    → gt costs record
//
// Loaded via: omp --hook gastown-hook.ts

export default function (pi) {
  const role = (process.env.GT_ROLE || "").toLowerCase();
  const autonomousRoles = new Set(["polecat", "witness", "refinery", "deacon"]);
  let primeContext = null;
  let contextInjected = false;

  // SessionStart — run gt prime and capture context for injection.
  pi.on("session_start", async (event, ctx) => {
    try {
      const result = await pi.exec("gt", ["prime", "--hook"]);
      if (result.code === 0 && result.stdout?.trim()) {
        primeContext = result.stdout.trim();
        console.error("[gastown] gt prime captured (" + primeContext.length + " chars)");
      } else {
        console.error("[gastown] gt prime returned no output (code=" + result.code + ")");
      }
    } catch (e) {
      console.error("[gastown] gt prime failed:", e.message);
    }

    // Check mail for autonomous roles.
    if (autonomousRoles.has(role)) {
      try {
        const mailResult = await pi.exec("gt", ["mail", "check", "--inject"]);
        if (mailResult.code === 0 && mailResult.stdout?.trim()) {
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
    }
  });

  // BeforeAgentStart — inject prime context into system prompt on first prompt.
  pi.on("before_agent_start", async (event, ctx) => {
    if (primeContext && !contextInjected) {
      contextInjected = true;
      console.error("[gastown] injecting prime context into session");
      return {
        message: {
          customType: "gastown-prime",
          content: primeContext,
          display: false,
        },
        systemPrompt: (event.systemPrompt || "") + "\n\n" + primeContext,
      };
    }
  });

  // Compaction — reload prime context after compaction so the agent recovers.
  pi.on("session_compact", async (event, ctx) => {
    contextInjected = false;
    primeContext = null;
    try {
      const result = await pi.exec("gt", ["prime", "--hook"]);
      if (result.code === 0 && result.stdout?.trim()) {
        primeContext = result.stdout.trim();
        console.error("[gastown] prime context refreshed after compaction");
      }
    } catch (e) {
      console.error("[gastown] gt prime refresh failed:", e.message);
    }
  });

  // PreToolUse — guard dangerous git operations via gt tap.
  pi.on("tool_call", async (event, ctx) => {
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

  // Shutdown — record API costs.
  pi.on("session_shutdown", async (event, ctx) => {
    try {
      await pi.exec("gt", ["costs", "record"]);
    } catch (e) {
      console.error("[gastown] gt costs record failed:", e.message);
    }
  });
}
