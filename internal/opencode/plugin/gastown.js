import fs from 'fs';
import path from 'path';

/**
 * Gas Town OpenCode plugin: Full parity with Claude hooks via OpenCode events.
 */
export const GasTown = async (args) => {
  const { $, directory, client } = args;
  const role = (process.env.GT_ROLE || "").toLowerCase();
  let sessionId = process.env.OPENCODE_SESSION_ID || "unknown";
  const autonomousRoles = new Set(["polecat", "witness", "refinery", "deacon"]);
  const interactiveRoles = new Set(["mayor", "crew"]);
  let didInit = false;
  let promptSent = false;
  let lastIdleTime = 0;
  let idleCount = 0;
  let gtPath = null;
  const pluginLogFile = process.env.GASTOWN_PLUGIN_LOG;

    const log = (level, event, message, data = {}) => {
    const ts = new Date().toISOString();
    const prefix = `[gastown] ${ts}`;
    
    // Truncate message and data for logging
    const truncate = (str, max = 500) => {
      if (typeof str !== 'string') str = JSON.stringify(str);
      return str.length > max ? str.substring(0, max) + '...' : str;
    };

    const displayMessage = truncate(message);
    const dataStr = Object.keys(data).length ? JSON.stringify(data, (key, value) => {
      if (typeof value === 'string') return truncate(value, 200);
      return value;
    }) : '';
    
    const logLine = `${prefix} [${level.toUpperCase()}] ${event}: ${displayMessage} ${dataStr}`;
    
    if (pluginLogFile) {
      try {
        fs.appendFileSync(pluginLogFile, logLine + '\n');
      } catch (e) {}
    }
    
    if (level === 'error') {
      console.error(logLine);
    } else {
      console.log(logLine);
    }
  };

  log('info', 'init', 'Plugin initializing', { 
    role, 
    directory, 
    GT_BINARY_PATH: process.env.GT_BINARY_PATH,
    GASTOWN_PLUGIN_LOG: pluginLogFile,
    cwd: process.cwd()
  });

  const findGt = async () => {
    if (gtPath) return gtPath;
    const candidates = [process.env.GT_BINARY_PATH, `${process.env.HOME}/go/bin/gt`, `${process.env.HOME}/.local/bin/gt`, "/usr/local/bin/gt"].filter(Boolean);
    for (const candidate of candidates) {
      if (fs.existsSync(candidate)) {
        if (candidate === process.env.GT_BINARY_PATH) {
          gtPath = candidate;
          return gtPath;
        }
        try {
          if (typeof $ === 'function') {
            const { exitCode } = await $`${candidate} version`.quiet();
            if (exitCode === 0) {
              gtPath = candidate;
              return gtPath;
            }
          } else {
            gtPath = candidate;
            return gtPath;
          }
        } catch (e) {}
      }
    }
    return null;
  };

  const run = async (cmd) => {
    try {
      const gt = await findGt();
      if (!gt) {
        log('warn', 'run', `Skipping: ${cmd} (gt not found)`);
        return;
      }
      const fullCmd = cmd.replace(/^gt(\s|$)/, `${gt}$1`);
      log('info', 'run', `Executing: ${cmd}`);
      if (typeof $ === 'function') {
        await $`/bin/sh -c ${fullCmd}`.cwd(directory);
        log('info', 'run', `Success: ${cmd}`);
      }
    } catch (err) {
      log('error', 'run', `Failed: ${cmd}`, { error: err?.message || String(err) });
    }
  };

  const inspectState = async () => {
    log('info', 'init', `Inspecting state in ${directory}`);
    try {
      const files = fs.readdirSync(directory);
      log('info', 'init', `Files: ${files.join(', ')}`);
      
      if (typeof $ === 'function') {
        const status = await $`git status`.cwd(directory).quiet();
        log('info', 'init', `Git Status: ${String(status.stdout).trim()}`);
        
        const branch = await $`git branch --show-current`.cwd(directory).quiet();
        log('info', 'init', `Git Branch: ${String(branch.stdout).trim()}`);

        if (files.filter(f => !f.startsWith('.')).length === 0 && fs.existsSync(path.join(directory, '.git'))) {
           log('warn', 'init', 'Directory appears empty but .git exists. Attempting checkout...');
           try {
             // Use --force to ensure we overwrite anything or fix partial checkouts
             await $`git checkout -f main || git checkout -f master || git checkout -f -b main`.cwd(directory).quiet();
             const newFiles = fs.readdirSync(directory);
             log('info', 'init', `Files after checkout: ${newFiles.join(', ')}`);
             
             // If still empty, try to see all branches
             if (newFiles.filter(f => !f.startsWith('.')).length === 0) {
                const branches = await $`git branch -a`.cwd(directory).quiet();
                log('info', 'init', `Available branches:\n${String(branches.stdout).trim()}`);
                
                const remoteBranches = await $`git branch -r`.cwd(directory).quiet();
                if (String(remoteBranches.stdout).includes('origin/main')) {
                   await $`git checkout -f main`.cwd(directory).quiet();
                } else if (String(remoteBranches.stdout).includes('origin/master')) {
                   await $`git checkout -f master`.cwd(directory).quiet();
                }
                
                const finalFiles = fs.readdirSync(directory);
                log('info', 'init', `Files after secondary checkout: ${finalFiles.join(', ')}`);
              }
            } catch (checkoutErr) {
              log('error', 'init', 'Checkout failed', { error: String(checkoutErr) });
            }
         }
      }
    } catch (e) {
      log('error', 'init', 'State inspection failed', { error: String(e) });
    }
  };

  const injectPrompt = async () => {
    if (promptSent) return false;
    const xdgConfig = process.env.XDG_CONFIG_HOME;
    if (!xdgConfig) return false;
    const promptFile = path.join(xdgConfig, "gastown_prompt.txt");
    if (!fs.existsSync(promptFile)) return false;
    const prompt = fs.readFileSync(promptFile, 'utf8');
    log('info', 'test', `Injecting prompt from file (session: ${sessionId})`);
    if (!client) {
      log('error', 'test', 'Cannot inject prompt: client is undefined');
      return false;
    }
    let sent = false;
    if (typeof client.sendUserMessage === 'function') {
      try {
        await client.sendUserMessage(prompt);
        sent = true;
        log('info', 'test', 'Prompt injected via client.sendUserMessage');
      } catch (e) { log('error', 'test', 'client.sendUserMessage failed', { error: String(e) }); }
    }
    if (!sent && client.tui?.appendPrompt && client.tui?.submitPrompt) {
      try {
        await client.tui.appendPrompt({ body: { text: prompt } });
        await new Promise(resolve => setTimeout(resolve, 1000));
        await client.tui.submitPrompt();
        sent = true;
        log('info', 'test', 'Prompt injected via TUI API');
      } catch (e) { log('error', 'test', 'TUI API failed', { error: String(e) }); }
    }
    if (!sent && sessionId !== "unknown" && client.session?.prompt) {
      try {
        await client.session.prompt({ 
          path: { id: sessionId }, 
          body: { parts: [{ type: 'text', text: prompt }] } 
        });
        sent = true;
        log('info', 'test', 'Prompt injected via session.prompt');
      } catch (e) { log('error', 'test', 'session.prompt failed', { error: String(e) }); }
    }
    if (sent) {
      promptSent = true;
      console.log("GASTOWN_READY >");
      return true;
    }
    return false;
  };

  const onSessionCreated = async () => {
    if (didInit) return;
    didInit = true;
    console.log("GASTOWN_READY >");
    await inspectState();
    await injectPrompt();
    await run("gt prime");
    if (autonomousRoles.has(role)) await run("gt mail check --inject");
    await run("gt nudge deacon session-started");
  };

  let initAttempts = 0;
  const proactiveInit = async () => {
    if (didInit && promptSent) return;
    initAttempts++;
    if (!promptSent) await injectPrompt();
    if (!didInit && promptSent) await onSessionCreated();
    if (!didInit || !promptSent) {
      if (initAttempts < 60) setTimeout(proactiveInit, 1000);
    }
  };

  setTimeout(proactiveInit, 3000);

  return {
    event: async ({ event }) => {
      log('debug', 'event', `Received event: ${event?.type}`, { props: Object.keys(event?.properties || {}) });
      switch (event?.type) {
        case "session.created":
        case "session.updated":
        case "session.status":
          if (!didInit && event.properties?.status !== "error") {
            sessionId = event.properties?.sessionID || event.properties?.info?.id || event.properties?.id || sessionId;
            log('info', 'hook', `Session event: ${event.type}, sessionId: ${sessionId}`);
            await onSessionCreated();
          }
          break;
        case "session.idle":
          idleCount++;
          log('info', 'idle', `Session idle detected (count: ${idleCount})`);
          if (role === "polecat") {
            await run("gt costs record");
            if (idleCount >= 3) {
              log('info', 'completion', 'GASTOWN_TASK_COMPLETE: Multiple idle events detected');
            }
          }
          break;
        case "session.error":
          const errObj = event.properties?.error || {};
          const errName = errObj.name || 'UnknownError';
          const errDataFields = errObj.data || {};
          const errMessage = errDataFields.message || errDataFields.providerID || JSON.stringify(errDataFields);
          const statusCode = errDataFields.statusCode ? ` (HTTP ${errDataFields.statusCode})` : '';
          const retryable = errDataFields.isRetryable ? ' [retryable]' : '';
          log('error', 'session', `GASTOWN_ERROR: ${errName}${statusCode}${retryable}: ${errMessage}`);
          break;
        case "message.updated":
          const msgInfo = event.properties?.info || {};
          log('debug', 'message', `Message from ${msgInfo?.role}`, { id: msgInfo?.id });
          break;
        case "message.part.updated":
          // This is where actual message content lives!
          const part = event.properties?.part;
          const delta = event.properties?.delta;
          const msgId = event.properties?.messageID || event.properties?.id;
          
          if (part?.type === 'text' && part?.text) {
            const textSnippet = part.text.length > 200 ? part.text.substring(0, 200) + '...' : part.text;
            log('info', 'content', `[Msg: ${msgId}] [${part.text.length} chars] ${textSnippet}`);
            
            // Check for completion indicators in the text
            const completionPatterns = [
              /tests?\s+(pass|passed|passing|succeed|success)/i,
              /fix(ed)?\s+(the\s+)?(bug|issue|problem)/i,
              /change.*complete/i,
              /successfully\s+(fixed|changed|updated)/i,
              /\ba\s*-\s*b\b/,  // The actual fix: a - b
              /subtraction.*instead.*addition/i,
              /done.*with.*task/i,
              /all\s+tests\s+passed/i,
              /bug\s+has\s+been\s+fixed/i
            ];
            
            for (const pattern of completionPatterns) {
              if (pattern.test(part.text)) {
                log('info', 'completion', `GASTOWN_TASK_COMPLETE: Matched pattern ${pattern} in message ${msgId}`);
                break;
              }
            }
          }
          if (delta) {
            log('debug', 'delta', `Delta [Msg: ${msgId}]: ${delta.length > 100 ? delta.substring(0, 100) + '...' : delta}`);
          }
          break;
        case "session.compacted":
          await run("gt prime");
          break;
      }
    },
  };
};
