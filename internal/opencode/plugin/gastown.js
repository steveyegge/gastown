import fs from 'fs';
import path from 'path';

export const GasTown = async (args) => {
  const { $, directory, client } = args;
  const role = (process.env.GT_ROLE || "").toLowerCase();
  const gtRig = process.env.GT_RIG || "unknown";
  const gtAgent = process.env.GT_POLECAT || process.env.BEADS_AGENT_NAME || "unknown";
  let sessionId = process.env.OPENCODE_SESSION_ID || "unknown";
  
  const autonomousRoles = new Set(["polecat", "witness", "refinery", "deacon"]);
  let didInit = false;
  let promptSent = false;
  let idleCount = 0;
  let gtPath = null;
  
  const testLogFile = process.env.GASTOWN_PLUGIN_LOG;
  const eventLogFile = process.env.GASTOWN_PLUGIN_LOG_EVENTS;

  let messageBuffers = new Map();
  let messageRoles = new Map();
  let sessionStatuses = new Map();
  
  let lastLogBody = "";
  let repeatCount = 0;
  let lastLogTimestamp = Date.now();
  let firstLogTimestamp = Date.now();

  const stripAnsi = (str) => {
    if (typeof str !== 'string') return str;
    return str.replace(/[\u001b\u009b][[()#;?]*(?:[0-9]{1,4}(?:;[0-9]{0,4})*)?[0-9A-ORZcf-nqry=><]/g, '');
  };

  const truncate = (str, max = 500) => {
    if (str === null || str === undefined) return String(str);
    if (typeof str !== 'string') str = JSON.stringify(str);
    return str.length > max ? str.substring(0, max) + '...' : str;
  };

  const writeToFile = (file, line) => {
    if (!file) return;
    try {
      fs.appendFileSync(file, line + '\n');
    } catch (e) {}
  };

  const flushMessageBuffer = (id) => {
    const text = messageBuffers.get(id);
    const mRole = messageRoles.get(id) || "unknown";
    if (text && text.trim().length > 0) {
      log('info', 'message_content', `[Msg: ${id}] [Role: ${mRole}]`, { 
        content: text 
      });
      messageBuffers.delete(id);
    }
  };

  const isMeaningful = (event) => {
    const meaningfulEvents = new Set(['init', 'hook', 'test', 'completion', 'message_content', 'session.error', 'error', 'run', 'tool']);
    return meaningfulEvents.has(event);
  };

  let lastLogBody = "";
  let repeatCount = 0;
  let lastLogTimestamp = Date.now();
  let firstLogTimestamp = Date.now();
  let repeatLineWritten = false;

  const log = (level, event, message, data = {}, eSessionId = sessionId) => {
    const now = Date.now();
    const roleUpper = role ? role.toUpperCase() : "UNKNOWN";
    const agentName = gtAgent !== "unknown" ? gtAgent.toUpperCase() : "";
    const rigName = gtRig !== "unknown" ? gtRig : "";
    const sessionPrefix = eSessionId && eSessionId !== "unknown" ? eSessionId.substring(0, 8) : "";
    
    const contextTag = `[${roleUpper}${agentName ? ':' + agentName : ''}${rigName ? ':' + rigName : ''}${sessionPrefix ? ':' + sessionPrefix : ''}]`;
    const { description, ...otherData } = data;
    const semanticAction = description ? ` [Action: ${description}]` : "";
    const displayMessage = truncate(message);
    const dataStr = Object.keys(otherData).length ? JSON.stringify(otherData, (key, value) => {
      if (typeof value === 'string') return truncate(value, 200);
      return value;
    }) : '';
    
    const logBody = `${level.toUpperCase()} ${event} ${displayMessage} ${dataStr} ${semanticAction}`;
    
    if (logBody === lastLogBody && !['message_content', 'delta', 'init', 'completion', 'tool'].includes(event)) {
      repeatCount++;
      const sinceLast = ((now - lastLogTimestamp) / 1000).toFixed(1);
      const indicator = repeatCount === 1 ? `\n    └─ 1(${((lastLogTimestamp - firstLogTimestamp)/1000).toFixed(1)}s) 2(${sinceLast}s)` : ` ${repeatCount + 1}(${sinceLast}s)`;
      
      writeToFile(eventLogFile, indicator);
      if (testLogFile && (isMeaningful(event) || level === 'error' || level === 'warn')) {
        writeToFile(testLogFile, indicator);
      }
      lastLogTimestamp = now;
      repeatLineWritten = true;
      return; 
    }
    
    if (repeatLineWritten) {
       writeToFile(eventLogFile, "\n");
       if (testLogFile) writeToFile(testLogFile, "\n");
    }

    lastLogBody = logBody;
    repeatCount = 0;
    firstLogTimestamp = now;
    lastLogTimestamp = now;
    repeatLineWritten = false;

    const ts = new Date().toISOString();
    const prefix = `[gastown] ${ts}`;
    const cleanMsg = displayMessage + (dataStr ? " " + dataStr : "") + semanticAction;
    const rawLogLine = `\n${contextTag} ${event}: ${cleanMsg}`;
    const fullRawLine = `${prefix} ${rawLogLine}`;
    
    writeToFile(eventLogFile, fullRawLine);
    if (testLogFile && (isMeaningful(event) || level === 'error' || level === 'warn')) {
      writeToFile(testLogFile, rawLogLine);
    }
    
    if (level === 'error') {
      console.error(rawLogLine);
    } else {
      console.log(rawLogLine);
    }
  };

  log('info', 'init', 'Plugin initializing', { 
    role, 
    directory, 
    GT_BINARY_PATH: process.env.GT_BINARY_PATH,
    cwd: process.cwd()
  });

  const findGt = async () => {
    if (gtPath) return gtPath;
    const candidates = [process.env.GT_BINARY_PATH, `${process.env.HOME}/go/bin/gt`, `${process.env.HOME}/.local/bin/gt`, "/usr/local/bin/gt"].filter(Boolean);
    for (const candidate of candidates) {
      if (fs.existsSync(candidate)) {
        gtPath = candidate;
        return gtPath;
      }
    }
    return null;
  };

  const run = async (cmd, description = "") => {
    try {
      const gt = await findGt();
      if (!gt) {
        log('warn', 'run', `Skipping: ${cmd} (gt not found)`);
        return;
      }
      const fullCmd = cmd.replace(/^gt(\s|$)/, `${gt}$1`);
      log('info', 'run', `Executing: ${cmd}`, { description });
      if (typeof $ === 'function') {
        const result = await $`/bin/sh -c ${fullCmd}`.cwd(directory).quiet();
        if (result.exitCode === 0) {
          log('info', 'run', `Success: ${cmd}`);
        } else {
          log('error', 'run', `Failed: ${cmd}`, { 
            exitCode: result.exitCode,
            stderr: truncate(result.stderr, 200)
          });
        }
        if (cmd.startsWith('gt done')) {
          log('info', 'completion', 'GASTOWN_TASK_COMPLETE: gt done executed');
        }
      }
    } catch (err) {
      log('error', 'run', `Exception during: ${cmd}`, { error: err?.message || String(err) });
    }
  };

  const inspectState = async () => {
    log('info', 'init', `Inspecting state in ${directory}`);
    try {
      const files = fs.readdirSync(directory);
      log('info', 'init', `Files found: ${files.join(', ')}`);
      if (typeof $ === 'function') {
        await $`git status`.cwd(directory).quiet();
        if (files.filter(f => !f.startsWith('.')).length === 0 && fs.existsSync(path.join(directory, '.git'))) {
           log('warn', 'init', 'Directory empty, .git exists. Recovering...');
           try {
             await $`git checkout -f main || git checkout -f master || git checkout -f -b main`.cwd(directory).quiet();
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
    let prompt = fs.readFileSync(promptFile, 'utf8');
    const additions = `\n\nOPENCODE INSTRUCTIONS:
- Use the 'gt_done' tool once you have completed and verified your work.
- Provide a clear summary of your changes in the 'summary' parameter.
- CRITICAL: Calling 'gt_done' is the ONLY official way to signal completion.`;
    prompt += additions;
    
    const logSession = sessionId !== "unknown" ? ` (session: ${sessionId})` : "";
    log('info', 'test', `Injecting assignment from hook${logSession}`);
    
    if (!client) {
      log('error', 'test', 'Cannot inject assignment: client is undefined');
      return false;
    }
    let sent = false;
    if (typeof client.sendUserMessage === 'function') {
      try {
        await client.sendUserMessage(prompt);
        sent = true;
      } catch (e) { log('error', 'test', 'client.sendUserMessage failed', { error: String(e) }); }
    }
    if (!sent && client.tui?.appendPrompt && client.tui?.submitPrompt) {
      try {
        await client.tui.appendPrompt({ body: { text: prompt } });
        await new Promise(resolve => setTimeout(resolve, 1000));
        await client.tui.submitPrompt();
        sent = true;
      } catch (e) { log('error', 'test', 'TUI API failed', { error: String(e) }); }
    }
    if (!sent && sessionId !== "unknown" && client.session?.prompt) {
      try {
        await client.session.prompt({ 
          path: { id: sessionId }, 
          body: { parts: [{ type: 'text', text: prompt }] } 
        });
        sent = true;
      } catch (e) { log('error', 'test', 'session.prompt failed', { error: String(e) }); }
    }
    if (sent) {
      promptSent = true;
      return true;
    }
    return false;
  };

  const onSessionCreated = async () => {
    if (didInit) return;
    didInit = true;
    await inspectState();
    await injectPrompt();
    log('info', 'init', 'Setting up agent workspace and syncing environment');
    await run("gt prime", "Registering agent and syncing local files");
    if (autonomousRoles.has(role)) {
      await run("gt mail check --inject", "Retrieving assigned task");
    }
    await run("gt nudge deacon session-started", "Signaling work start to monitor");
    log('info', 'init', 'Setup complete - agent is now processing task');
  };

  let initAttempts = 0;
  const proactiveInit = async () => {
    if (didInit && promptSent) return;
    initAttempts++;
    if (!promptSent) await injectPrompt();
    if (!didInit && promptSent) await onSessionCreated();
    if (!didInit || !promptSent) {
      if (initAttempts < 100) setTimeout(proactiveInit, 500);
    }
  };

  setTimeout(proactiveInit, 1000);

  return {
    tools: {
      gt_done: {
        description: "Call this tool when you have finished the assigned task and verified it with tests. This will signal completion to the Gas Town system.",
        parameters: {
          type: "object",
          properties: {
            summary: { type: "string", description: "A brief summary of what was accomplished." }
          },
          required: ["summary"]
        },
        execute: async (params) => {
          log('info', 'tool', `gt_done tool called`, { summary: params.summary });
          log('info', 'completion', `GASTOWN_TASK_COMPLETE: gt_done tool used`);
          return { status: "success", message: "Task completion signaled." };
        }
      }
    },
    event: async ({ event }) => {
      const eSessionId = event?.properties?.sessionID || event?.properties?.id || sessionId;
      if (eSessionId && eSessionId !== "unknown") {
        sessionId = eSessionId;
      }
      switch (event?.type) {
        case "session.created":
        case "session.updated":
        case "session.status":
          const status = event.properties?.status;
          const statusStr = typeof status === 'object' ? status.name || status.type || JSON.stringify(status) : String(status);
          if (statusStr === "undefined" || statusStr === "unknown") break;
          const prevStatus = sessionStatuses.get(eSessionId);
          if (statusStr !== prevStatus) {
            if (statusStr === "busy" || statusStr === "idle") {
              sessionStatuses.set(eSessionId, statusStr);
              if (statusStr === "busy" && !prevStatus) {
                log('info', 'hook', `[SESSION START] ID: ${eSessionId}`, {}, eSessionId);
                if (!didInit) await onSessionCreated();
              } else {
                log('info', 'hook', `[STATUS CHANGE] ${eSessionId}: ${prevStatus || 'init'} -> ${statusStr}`, {}, eSessionId);
              }
            }
          }
          break;
        case "session.idle":
          idleCount++;
          log('info', 'idle', `[SESSION IDLE] count: ${idleCount}`, {}, eSessionId);
          if (currentMessageId) flushMessageBuffer(currentMessageId);
          if (role === "polecat") {
            await run("gt costs record", "Updating task costs");
            if (idleCount >= 2) {
              log('info', 'completion', `GASTOWN_TASK_COMPLETE: Idle threshold reached`, {}, eSessionId);
            }
          }
          break;
        case "session.error":
          log('error', 'session', `GASTOWN_ERROR: ${JSON.stringify(event.properties?.error || {})}`, {}, eSessionId);
          break;
        case "message.updated":
          const msgInfo = event.properties?.info || {};
          const mId = msgInfo.id || event.properties?.id;
          if (mId) {
            if (currentMessageId && currentMessageId !== mId) flushMessageBuffer(currentMessageId);
            currentMessageId = mId;
            if (msgInfo.role) messageRoles.set(mId, msgInfo.role);
          }
          log('debug', 'message', `Message updated: id=${mId}, role=${msgInfo?.role}`, {}, eSessionId);
          break;
        case "message.part.updated":
          const part = event.properties?.part;
          const partMsgId = event.properties?.messageID || event.properties?.id || currentMessageId;
          if (partMsgId) {
            if (currentMessageId && currentMessageId !== partMsgId) flushMessageBuffer(currentMessageId);
            currentMessageId = partMsgId;
          }
          if (part?.type === 'text' && part?.text) messageBuffers.set(partMsgId, part.text);
          break;
        case "session.compacted":
          await run("gt prime", "Refreshes context after session compaction");
          break;
      }
    },
  };
};
