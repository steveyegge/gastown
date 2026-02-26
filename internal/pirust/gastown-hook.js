// Gas Town Extension for pi-rust (QuickJS runtime)
//
// Context injection: runs gt prime + mail check at startup, then injects
// via tmux send-keys after a delay so the TUI processes it as real input.
//
// Events used:
//   startup    → gt prime --hook, mail check, tmux inject
//   tool_call  → gt tap guard pr-workflow (on git push/pr create)
//   agent_end  → gt costs record

export default (pi) => {
  const role = (process.env.GT_ROLE || "").toLowerCase();
  const autonomousRoles = new Set(["polecat", "witness", "refinery", "deacon"]);

  pi.on("startup", async (event) => {
    let context = null;

    // Capture gt prime context.
    try {
      const result = await pi.exec("gt", ["prime", "--hook"]);
      if (result.code === 0 && result.stdout?.trim()) {
        context = result.stdout.trim();
        console.error("[gastown] gt prime captured (" + context.length + " chars)");
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
          if (context) {
            context += "\n\n" + mailResult.stdout.trim();
          } else {
            context = mailResult.stdout.trim();
          }
          console.error("[gastown] mail context appended");
        }
      } catch (e) {
        console.error("[gastown] gt mail check failed:", e.message);
      }
    }

    // Inject context as real tmux input after TUI is ready.
    if (context) {
      await new Promise(resolve => setTimeout(resolve, 3000));
      try {
        const tmpFile = "/tmp/gt-prime-inject-" + process.pid + ".txt";
        await pi.exec("python3", ["-c",
          "import sys; open(sys.argv[1],'w').write(sys.argv[2])",
          tmpFile, context
        ]);
        await pi.exec("tmux", ["load-buffer", tmpFile]);
        await pi.exec("tmux", ["paste-buffer"]);
        await new Promise(resolve => setTimeout(resolve, 500));
        await pi.exec("tmux", ["send-keys", "Enter"]);
        await pi.exec("rm", [tmpFile]);
        console.error("[gastown] prime context injected via tmux");
      } catch (e) {
        console.error("[gastown] tmux inject failed:", e.message);
      }
    }
  });

  // ToolCall — guard dangerous git operations via gt tap.
  pi.on("tool_call", async (event) => {
    if (event.toolName === "bash" && event.input?.command) {
      const cmd = event.input.command;
      if (
        cmd.includes("git push") ||
        cmd.includes("gh pr create") ||
        cmd.includes("git checkout -b")
      ) {
        console.error("[gastown] Guarding git operation:", cmd);
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

  // AgentEnd — record API costs.
  pi.on("agent_end", async () => {
    try {
      await pi.exec("gt", ["costs", "record"]);
      console.error("[gastown] Costs recorded");
    } catch (e) {
      console.error("[gastown] gt costs record failed:", e.message);
    }
  });
};
