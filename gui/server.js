/**
 * Gas Town GUI Bridge Server
 *
 * Node.js server that bridges the browser UI to the Gas Town CLI.
 * - Executes gt/bd commands via child_process
 * - Streams real-time events via WebSocket
 * - Serves static files
 */

import express from 'express';
import { createServer } from 'http';
import { WebSocketServer } from 'ws';
import { spawn, exec } from 'child_process';
import { promisify } from 'util';
import path from 'path';
import fs from 'fs';
import os from 'os';
import readline from 'readline';
import { fileURLToPath } from 'url';
import cors from 'cors';

const execAsync = promisify(exec);
const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const app = express();
const server = createServer(app);
const wss = new WebSocketServer({ server });

const PORT = process.env.PORT || 3000;
const HOME = process.env.HOME || require('os').homedir();
const GT_ROOT = process.env.GT_ROOT || path.join(HOME, 'gt');

// Middleware
app.use(cors());
app.use(express.json());
app.use(express.static(__dirname));

// Store connected WebSocket clients
const clients = new Set();

// Broadcast to all connected clients
function broadcast(data) {
  const message = JSON.stringify(data);
  clients.forEach(client => {
    if (client.readyState === 1) { // OPEN
      client.send(message);
    }
  });
}

// Quote arguments that contain spaces
function quoteArg(arg) {
  if (arg.includes(' ') || arg.includes('"') || arg.includes("'")) {
    // Escape any existing double quotes and wrap in double quotes
    return `"${arg.replace(/"/g, '\\"')}"`;
  }
  return arg;
}

// Get running tmux sessions for polecats
async function getRunningPolecats() {
  try {
    const { stdout } = await execAsync('tmux ls 2>/dev/null || echo ""');
    const sessions = new Set();
    // Parse tmux ls output: "gt-rig-polecat: 1 windows (created ...)"
    for (const line of stdout.split('\n')) {
      const match = line.match(/^(gt-[^:]+):/);
      if (match) {
        // Convert "gt-hytopia-map-compression-capable" to "hytopia-map-compression/capable"
        const parts = match[1].replace('gt-', '').split('-');
        if (parts.length >= 2) {
          const name = parts.pop();
          const rig = parts.join('-');
          sessions.add(`${rig}/${name}`);
        }
      }
    }
    return sessions;
  } catch {
    return new Set();
  }
}

// Get polecat output from tmux (last N lines)
async function getPolecatOutput(sessionName, lines = 50) {
  try {
    const { stdout } = await execAsync(`tmux capture-pane -t ${sessionName} -p 2>/dev/null | tail -${lines}`);
    return stdout.trim();
  } catch {
    return null;
  }
}

// Execute a Gas Town command
async function executeGT(args, options = {}) {
  const cmd = `gt ${args.map(quoteArg).join(' ')}`;
  console.log(`[GT] Executing: ${cmd}`);

  try {
    const { stdout, stderr } = await execAsync(cmd, {
      cwd: options.cwd || GT_ROOT,
      timeout: options.timeout || 30000,
      env: { ...process.env, ...options.env }
    });

    if (stderr && !options.ignoreStderr) {
      console.warn(`[GT] stderr: ${stderr}`);
    }

    return { success: true, data: stdout.trim() };
  } catch (error) {
    console.error(`[GT] Error: ${error.message}`);
    return { success: false, error: error.message };
  }
}

// Execute a Beads command
async function executeBD(args, options = {}) {
  const cmd = `bd ${args.map(quoteArg).join(' ')}`;
  console.log(`[BD] Executing: ${cmd}`);

  // Set BEADS_DIR to ensure bd finds the database
  const beadsDir = path.join(GT_ROOT, '.beads');

  try {
    const { stdout, stderr } = await execAsync(cmd, {
      cwd: options.cwd || GT_ROOT,
      timeout: options.timeout || 30000,
      env: { ...process.env, BEADS_DIR: beadsDir }
    });

    return { success: true, data: stdout.trim() };
  } catch (error) {
    return { success: false, error: error.message };
  }
}

// Parse JSON output from commands
function parseJSON(output) {
  try {
    return JSON.parse(output);
  } catch {
    return null;
  }
}

// ============= REST API Endpoints =============

// Town status overview
app.get('/api/status', async (req, res) => {
  const [result, runningPolecats] = await Promise.all([
    executeGT(['status', '--json', '--fast']),
    getRunningPolecats()
  ]);

  if (result.success) {
    const data = parseJSON(result.data);
    if (data) {
      // Enhance rigs with running state from tmux and git_url from config
      for (const rig of data.rigs || []) {
        // Try to read rig config to get git_url
        try {
          const rigConfigPath = path.join(GT_ROOT, rig.name, 'config.json');
          if (fs.existsSync(rigConfigPath)) {
            const rigConfig = JSON.parse(fs.readFileSync(rigConfigPath, 'utf8'));
            rig.git_url = rigConfig.git_url || null;
          }
        } catch (e) {
          // Config not found or invalid, continue
        }

        for (const hook of rig.hooks || []) {
          // Check if this polecat has a running tmux session
          const agentPath = hook.agent; // e.g., "hytopia-map-compression/capable"
          hook.running = runningPolecats.has(agentPath);

          // Also check polecats subdirectory format
          const polecatPath = agentPath.replace(/\//, '/polecats/');
          if (!hook.running && runningPolecats.has(polecatPath)) {
            hook.running = true;
          }
        }
      }
      data.runningPolecats = Array.from(runningPolecats);
    }
    res.json(data || { raw: result.data });
  } else {
    res.status(500).json({ error: result.error });
  }
});

// List convoys
app.get('/api/convoys', async (req, res) => {
  const args = ['convoy', 'list', '--json'];
  if (req.query.all === 'true') args.push('--all');
  if (req.query.status) args.push(`--status=${req.query.status}`);

  const result = await executeGT(args);
  if (result.success) {
    const data = parseJSON(result.data);
    res.json(data || []);
  } else {
    res.status(500).json({ error: result.error });
  }
});

// Get convoy details
app.get('/api/convoy/:id', async (req, res) => {
  const result = await executeGT(['convoy', 'status', req.params.id, '--json']);
  if (result.success) {
    const data = parseJSON(result.data);
    res.json(data || { id: req.params.id, raw: result.data });
  } else {
    res.status(500).json({ error: result.error });
  }
});

// Create convoy
app.post('/api/convoy', async (req, res) => {
  const { name, issues, notify } = req.body;
  const args = ['convoy', 'create', name, ...(issues || [])];
  if (notify) args.push('--notify', notify);

  const result = await executeGT(args);
  if (result.success) {
    // Parse convoy ID from text output (e.g., "Created convoy: convoy-abc123")
    const match = result.data.match(/(?:Created|created)\s*(?:convoy)?:?\s*(\S+)/i);
    const convoyId = match ? match[1] : result.data.trim();
    broadcast({ type: 'convoy_created', data: { convoy_id: convoyId, name } });
    res.json({ success: true, convoy_id: convoyId, raw: result.data });
  } else {
    res.status(500).json({ error: result.error });
  }
});

// Sling work
app.post('/api/sling', async (req, res) => {
  const { bead, target, molecule, quality, args: slingArgs } = req.body;
  const cmdArgs = ['sling', bead];

  if (target) cmdArgs.push(target);
  if (molecule) cmdArgs.push('--molecule', molecule);
  if (quality) cmdArgs.push(`--quality=${quality}`);
  if (slingArgs) cmdArgs.push('--args', slingArgs);

  // Sling spawns a polecat which can take 60+ seconds
  // Use ignoreStderr since sling has many non-fatal warnings
  const result = await executeGT(cmdArgs, { timeout: 90000, ignoreStderr: true });

  // Check for success indicators in output - sling can have warnings but still succeed
  const output = result.data || result.error || '';
  const workAttached = output.includes('Work attached to hook') || output.includes('✓ Work attached');
  const promptSent = output.includes('Start prompt sent') || output.includes('▶ Start prompt sent');
  const polecatSpawned = output.includes('Polecat') && output.includes('spawned');

  // Consider success if work was attached or prompt was sent
  const actualSuccess = result.success || workAttached || promptSent;

  if (actualSuccess) {
    const jsonData = parseJSON(result.data);
    const responseData = {
      bead,
      target,
      workAttached,
      promptSent,
      polecatSpawned,
      raw: output
    };
    broadcast({ type: 'work_slung', data: jsonData || responseData });
    res.json({ success: true, data: jsonData || responseData, raw: output });
  } else {
    res.status(500).json({ error: result.error || 'Sling failed - no work attached' });
  }
});

// Get mail inbox
app.get('/api/mail', async (req, res) => {
  const result = await executeGT(['mail', 'inbox', '--json']);
  if (result.success) {
    const data = parseJSON(result.data);
    res.json(data || []);
  } else {
    res.status(500).json({ error: result.error });
  }
});

// Send mail
app.post('/api/mail', async (req, res) => {
  const { to, subject, message, priority } = req.body;
  const args = ['mail', 'send', to, '-s', subject, '-m', message];
  if (priority) args.push('--priority', priority);

  const result = await executeGT(args);
  if (result.success) {
    res.json({ success: true });
  } else {
    res.status(500).json({ error: result.error });
  }
});

// Get all mail from feed (for observability)
app.get('/api/mail/all', async (req, res) => {
  try {
    const feedPath = path.join(GT_ROOT, '.feed.jsonl');
    if (!fs.existsSync(feedPath)) {
      return res.json([]);
    }

    const fileStream = fs.createReadStream(feedPath);
    const rl = readline.createInterface({
      input: fileStream,
      crlfDelay: Infinity
    });

    const mailEvents = [];
    for await (const line of rl) {
      if (!line.trim()) continue;
      try {
        const event = JSON.parse(line);
        if (event.type === 'mail') {
          // Transform feed event to mail-like object
          mailEvents.push({
            id: `feed-${event.ts}-${mailEvents.length}`,
            from: event.actor || 'unknown',
            to: event.payload?.to || 'unknown',
            subject: event.payload?.subject || event.summary || '(No Subject)',
            body: event.payload?.body || event.payload?.message || '',
            timestamp: event.ts,
            read: true, // Feed mail is historical
            priority: event.payload?.priority || 'normal',
            feedEvent: true, // Mark as feed-sourced
          });
        }
      } catch (e) {
        // Skip malformed lines
      }
    }

    // Sort newest first
    mailEvents.sort((a, b) => new Date(b.timestamp) - new Date(a.timestamp));
    res.json(mailEvents);
  } catch (err) {
    console.error('[API] Failed to read feed for mail:', err);
    res.status(500).json({ error: 'Failed to read mail feed' });
  }
});

// Get single mail message
app.get('/api/mail/:id', async (req, res) => {
  const { id } = req.params;

  try {
    const result = await executeGT(['mail', 'read', id, '--json']);
    if (result.success) {
      const mail = parseJSON(result.data);
      res.json(mail || { id, error: 'Not found' });
    } else {
      res.status(404).json({ error: 'Mail not found' });
    }
  } catch (err) {
    res.status(500).json({ error: err.message });
  }
});

// Mark mail as read
app.post('/api/mail/:id/read', async (req, res) => {
  const { id } = req.params;

  try {
    const result = await executeGT(['mail', 'mark-read', id]);
    if (result.success) {
      res.json({ success: true, id, read: true });
    } else {
      res.status(500).json({ error: result.error || 'Failed to mark as read' });
    }
  } catch (err) {
    res.status(500).json({ error: err.message });
  }
});

// Mark mail as unread
app.post('/api/mail/:id/unread', async (req, res) => {
  const { id } = req.params;

  try {
    const result = await executeGT(['mail', 'mark-unread', id]);
    if (result.success) {
      res.json({ success: true, id, read: false });
    } else {
      res.status(500).json({ error: result.error || 'Failed to mark as unread' });
    }
  } catch (err) {
    res.status(500).json({ error: err.message });
  }
});

// ============= Beads API =============

// Create a new bead (issue)
app.post('/api/beads', async (req, res) => {
  const { title, description, priority, labels } = req.body;

  if (!title) {
    return res.status(400).json({ error: 'Title is required' });
  }

  // Build bd new command
  // bd new "title" --description "..." --priority high --label bug --label enhancement
  // Use --no-daemon to avoid timeout issues
  const args = ['--no-daemon', 'new', title];

  if (description) {
    args.push('--description', description);
  }
  if (priority && priority !== 'normal') {
    args.push('--priority', priority);
  }
  if (labels && Array.isArray(labels) && labels.length > 0) {
    labels.forEach(label => {
      args.push('--label', label);
    });
  }

  const result = await executeBD(args);

  if (result.success) {
    // Parse the bead ID from output (format: "Created bead: gt-abc123")
    const match = result.data.match(/(?:Created|created)\s*(?:bead|issue)?:?\s*(\S+)/i);
    const beadId = match ? match[1] : result.data.trim();

    broadcast({ type: 'bead_created', data: { bead_id: beadId, title } });
    res.json({ success: true, bead_id: beadId, raw: result.data });
  } else {
    res.status(500).json({ success: false, error: result.error });
  }
});

// Search beads
app.get('/api/beads/search', async (req, res) => {
  const query = req.query.q || '';

  // bd search "query" or bd list if no query
  // Use --no-daemon to avoid timeout issues
  const args = query ? ['--no-daemon', 'search', query] : ['--no-daemon', 'list'];
  args.push('--json');

  const result = await executeBD(args);

  if (result.success) {
    const data = parseJSON(result.data);
    res.json(data || []);
  } else {
    // Return empty array on error (may just be no results)
    res.json([]);
  }
});

// List all beads
app.get('/api/beads', async (req, res) => {
  const status = req.query.status;
  // Use --no-daemon to avoid timeout issues
  const args = ['--no-daemon', 'list'];
  if (status) args.push(`--status=${status}`);
  args.push('--json');

  const result = await executeBD(args);

  if (result.success) {
    const data = parseJSON(result.data);
    res.json(data || []);
  } else {
    res.json([]);
  }
});

// ============= Work Actions =============

// Mark work as done
app.post('/api/work/:beadId/done', async (req, res) => {
  const { beadId } = req.params;
  const { summary } = req.body;

  console.log(`[Work] Marking ${beadId} as done...`);

  const args = ['done', beadId];
  if (summary) {
    args.push('-m', summary);
  }

  const result = await executeBD(args);

  if (result.success) {
    broadcast({ type: 'work_done', data: { beadId, summary } });
    res.json({ success: true, beadId, message: `${beadId} marked as done`, raw: result.data });
  } else {
    res.status(500).json({ success: false, error: result.error });
  }
});

// Park work (temporarily set aside)
app.post('/api/work/:beadId/park', async (req, res) => {
  const { beadId } = req.params;
  const { reason } = req.body;

  console.log(`[Work] Parking ${beadId}...`);

  const args = ['park', beadId];
  if (reason) {
    args.push('-m', reason);
  }

  const result = await executeBD(args);

  if (result.success) {
    broadcast({ type: 'work_parked', data: { beadId, reason } });
    res.json({ success: true, beadId, message: `${beadId} parked`, raw: result.data });
  } else {
    res.status(500).json({ success: false, error: result.error });
  }
});

// Release work (unassign from agent)
app.post('/api/work/:beadId/release', async (req, res) => {
  const { beadId } = req.params;

  console.log(`[Work] Releasing ${beadId}...`);

  const result = await executeBD(['release', beadId]);

  if (result.success) {
    broadcast({ type: 'work_released', data: { beadId } });
    res.json({ success: true, beadId, message: `${beadId} released`, raw: result.data });
  } else {
    res.status(500).json({ success: false, error: result.error });
  }
});

// Reassign work to a different agent
app.post('/api/work/:beadId/reassign', async (req, res) => {
  const { beadId } = req.params;
  const { target } = req.body;

  if (!target) {
    return res.status(400).json({ error: 'Target is required' });
  }

  console.log(`[Work] Reassigning ${beadId} to ${target}...`);

  const result = await executeBD(['reassign', beadId, target]);

  if (result.success) {
    broadcast({ type: 'work_reassigned', data: { beadId, target } });
    res.json({ success: true, beadId, target, message: `${beadId} reassigned to ${target}`, raw: result.data });
  } else {
    res.status(500).json({ success: false, error: result.error });
  }
});

// Get bead details
app.get('/api/bead/:beadId', async (req, res) => {
  const { beadId } = req.params;

  const result = await executeBD(['show', beadId, '--json']);

  if (result.success) {
    const data = parseJSON(result.data);
    res.json(data || { id: beadId });
  } else {
    res.status(404).json({ error: 'Bead not found' });
  }
});

// Nudge agent
app.post('/api/nudge', async (req, res) => {
  const { target, message } = req.body;
  const result = await executeGT(['nudge', target, '-m', message]);
  if (result.success) {
    res.json({ success: true });
  } else {
    res.status(500).json({ error: result.error });
  }
});

// Get agent list
app.get('/api/agents', async (req, res) => {
  const [result, runningPolecats] = await Promise.all([
    executeGT(['status', '--json', '--fast'], { timeout: 60000 }),
    getRunningPolecats()
  ]);

  if (result.success) {
    const data = parseJSON(result.data);
    const agents = data?.agents || [];

    // Enhance agents with running state
    for (const agent of agents) {
      agent.running = runningPolecats.has(agent.address?.replace(/\/$/, ''));
    }

    // Also include running polecats from rigs
    const polecats = [];
    for (const rig of data?.rigs || []) {
      for (const hook of rig.hooks || []) {
        const isRunning = runningPolecats.has(hook.agent) ||
          runningPolecats.has(hook.agent?.replace(/\//, '/polecats/'));
        polecats.push({
          name: hook.agent,
          rig: rig.name,
          role: hook.role,
          running: isRunning,
          has_work: hook.has_work,
          hook_bead: hook.hook_bead
        });
      }
    }

    res.json({ agents, polecats, runningPolecats: Array.from(runningPolecats) });
  } else {
    res.status(500).json({ error: result.error });
  }
});

// Get polecat output (what they're working on)
app.get('/api/polecat/:rig/:name/output', async (req, res) => {
  const { rig, name } = req.params;
  const lines = parseInt(req.query.lines) || 50;
  const sessionName = `gt-${rig}-${name}`;

  const output = await getPolecatOutput(sessionName, lines);
  if (output !== null) {
    res.json({ session: sessionName, output, running: true });
  } else {
    res.json({ session: sessionName, output: null, running: false });
  }
});

// Get full agent transcript (Claude session log)
app.get('/api/polecat/:rig/:name/transcript', async (req, res) => {
  const { rig, name } = req.params;
  const sessionName = `gt-${rig}-${name}`;

  try {
    // First try to get tmux output (full history)
    const output = await getPolecatOutput(sessionName, 2000);

    // Also try to find Claude session transcript files
    // Claude Code typically stores transcripts in ~/.claude/projects/ or .claude/ directories
    let transcriptContent = null;
    const transcriptPaths = [
      path.join(GT_ROOT, rig, '.claude', 'sessions'),
      path.join(GT_ROOT, rig, '.claude', 'transcripts'),
      path.join(os.homedir(), '.claude', 'projects', rig, 'sessions'),
    ];

    for (const transcriptPath of transcriptPaths) {
      try {
        if (fs.existsSync(transcriptPath)) {
          // Find most recent transcript file
          const files = fs.readdirSync(transcriptPath)
            .filter(f => f.endsWith('.json') || f.endsWith('.md') || f.endsWith('.jsonl'))
            .map(f => ({
              name: f,
              time: fs.statSync(path.join(transcriptPath, f)).mtime.getTime()
            }))
            .sort((a, b) => b.time - a.time);

          if (files.length > 0) {
            transcriptContent = fs.readFileSync(
              path.join(transcriptPath, files[0].name),
              'utf-8'
            );
            break;
          }
        }
      } catch (e) {
        // Ignore errors, try next path
      }
    }

    res.json({
      session: sessionName,
      rig,
      name,
      running: output !== null,
      output: output || '(No tmux output available)',
      transcript: transcriptContent,
      hasTranscript: !!transcriptContent,
    });
  } catch (err) {
    res.status(500).json({ error: err.message });
  }
});

// Start a polecat/agent
app.post('/api/polecat/:rig/:name/start', async (req, res) => {
  const { rig, name } = req.params;
  const agentPath = `${rig}/${name}`;

  console.log(`[Agent] Starting ${agentPath}...`);

  try {
    // Use gt polecat spawn to start the agent
    const result = await executeGT(['polecat', 'spawn', agentPath], { timeout: 30000 });

    if (result.success) {
      broadcast({ type: 'agent_started', data: { rig, name, agentPath } });
      res.json({ success: true, message: `Started ${agentPath}`, raw: result.data });
    } else {
      res.status(500).json({ success: false, error: result.error });
    }
  } catch (err) {
    console.error(`[Agent] Failed to start ${agentPath}:`, err);
    res.status(500).json({ success: false, error: err.message });
  }
});

// Stop a polecat/agent
app.post('/api/polecat/:rig/:name/stop', async (req, res) => {
  const { rig, name } = req.params;
  const sessionName = `gt-${rig}-${name}`;

  console.log(`[Agent] Stopping ${rig}/${name}...`);

  try {
    // Kill the tmux session
    await execAsync(`tmux kill-session -t ${sessionName} 2>/dev/null`);
    broadcast({ type: 'agent_stopped', data: { rig, name, session: sessionName } });
    res.json({ success: true, message: `Stopped ${rig}/${name}` });
  } catch (err) {
    // Session might not exist, which is fine
    if (err.message.includes("can't find session")) {
      res.json({ success: true, message: `${rig}/${name} was not running` });
    } else {
      console.error(`[Agent] Failed to stop ${rig}/${name}:`, err);
      res.status(500).json({ success: false, error: err.message });
    }
  }
});

// Restart a polecat/agent (stop then start)
app.post('/api/polecat/:rig/:name/restart', async (req, res) => {
  const { rig, name } = req.params;
  const agentPath = `${rig}/${name}`;
  const sessionName = `gt-${rig}-${name}`;

  console.log(`[Agent] Restarting ${agentPath}...`);

  try {
    // First try to kill existing session (ignore errors)
    try {
      await execAsync(`tmux kill-session -t ${sessionName} 2>/dev/null`);
    } catch {
      // Ignore - session might not exist
    }

    // Wait a moment for cleanup
    await new Promise(resolve => setTimeout(resolve, 500));

    // Start the agent
    const result = await executeGT(['polecat', 'spawn', agentPath], { timeout: 30000 });

    if (result.success) {
      broadcast({ type: 'agent_restarted', data: { rig, name, agentPath } });
      res.json({ success: true, message: `Restarted ${agentPath}`, raw: result.data });
    } else {
      res.status(500).json({ success: false, error: result.error });
    }
  } catch (err) {
    console.error(`[Agent] Failed to restart ${agentPath}:`, err);
    res.status(500).json({ success: false, error: err.message });
  }
});

// Get hook status
app.get('/api/hook', async (req, res) => {
  const result = await executeGT(['hook', 'status', '--json']);
  if (result.success) {
    const data = parseJSON(result.data);
    res.json(data || { hooked: null });
  } else {
    res.status(500).json({ error: result.error });
  }
});

// Health check
app.get('/api/health', (req, res) => {
  res.json({ status: 'ok', timestamp: new Date().toISOString() });
});

// ============= Setup & Onboarding API =============

// Get setup status (for onboarding wizard)
app.get('/api/setup/status', async (req, res) => {
  const status = {
    gt_installed: false,
    gt_version: null,
    bd_installed: false,
    bd_version: null,
    workspace_initialized: false,
    workspace_path: GT_ROOT,
    rigs: [],
  };

  // Check gt
  try {
    const gtResult = await execAsync('gt version', { timeout: 5000 });
    status.gt_installed = true;
    status.gt_version = gtResult.stdout.trim().split('\n')[0];
  } catch {
    status.gt_installed = false;
  }

  // Check bd
  try {
    const bdResult = await execAsync('bd version', { timeout: 5000 });
    status.bd_installed = true;
    status.bd_version = bdResult.stdout.trim().split('\n')[0];
  } catch {
    status.bd_installed = false;
  }

  // Check workspace
  try {
    const fs = await import('fs');
    const path = await import('path');
    const mayorPath = path.join(GT_ROOT, 'mayor');
    status.workspace_initialized = fs.existsSync(mayorPath);
  } catch {
    status.workspace_initialized = false;
  }

  // Get rigs
  try {
    const rigResult = await executeGT(['rig', 'list']);
    if (rigResult.success) {
      // Parse text output
      const rigs = [];
      const lines = rigResult.data.split('\n');
      for (const line of lines) {
        const match = line.match(/^  ([a-zA-Z0-9_-]+)$/);
        if (match) {
          rigs.push({ name: match[1] });
        }
      }
      status.rigs = rigs;
    }
  } catch {
    status.rigs = [];
  }

  res.json(status);
});

// Add a rig (project)
app.post('/api/rigs', async (req, res) => {
  const { name, url } = req.body;

  if (!name || !url) {
    return res.status(400).json({ error: 'Name and URL are required' });
  }

  const result = await executeGT(['rig', 'add', name, url]);

  if (result.success) {
    broadcast({ type: 'rig_added', data: { name, url } });
    res.json({ success: true, name, raw: result.data });
  } else {
    res.status(500).json({ success: false, error: result.error });
  }
});

// List rigs
app.get('/api/rigs', async (req, res) => {
  const result = await executeGT(['rig', 'list']);

  if (result.success) {
    // Parse text output: "  rigname\n    Polecats: 0..."
    const rigs = [];
    const lines = result.data.split('\n');
    for (const line of lines) {
      // Rig names are indented with 2 spaces, not 4
      const match = line.match(/^  ([a-zA-Z0-9_-]+)$/);
      if (match) {
        rigs.push({ name: match[1] });
      }
    }
    res.json(rigs);
  } else {
    res.json([]);
  }
});

// Run gt doctor
app.get('/api/doctor', async (req, res) => {
  const result = await executeGT(['doctor', '--json']);

  if (result.success) {
    const data = parseJSON(result.data);
    res.json(data || { raw: result.data });
  } else {
    res.status(500).json({ error: result.error });
  }
});

// ============= Service Controls (Mayor, Witness, Refinery) =============

// Start a service
app.post('/api/service/:name/up', async (req, res) => {
  const { name } = req.params;
  const validServices = ['mayor', 'witness', 'refinery', 'deacon'];

  if (!validServices.includes(name.toLowerCase())) {
    return res.status(400).json({ error: `Invalid service: ${name}. Valid services: ${validServices.join(', ')}` });
  }

  console.log(`[Service] Starting ${name}...`);

  try {
    const result = await executeGT([name, 'up'], { timeout: 30000 });

    if (result.success) {
      broadcast({ type: 'service_started', data: { service: name } });
      res.json({ success: true, service: name, message: `${name} started`, raw: result.data });
    } else {
      res.status(500).json({ success: false, error: result.error });
    }
  } catch (err) {
    console.error(`[Service] Failed to start ${name}:`, err);
    res.status(500).json({ success: false, error: err.message });
  }
});

// Stop a service
app.post('/api/service/:name/down', async (req, res) => {
  const { name } = req.params;
  const validServices = ['mayor', 'witness', 'refinery', 'deacon'];

  if (!validServices.includes(name.toLowerCase())) {
    return res.status(400).json({ error: `Invalid service: ${name}. Valid services: ${validServices.join(', ')}` });
  }

  console.log(`[Service] Stopping ${name}...`);

  try {
    const result = await executeGT([name, 'down'], { timeout: 10000 });

    if (result.success) {
      broadcast({ type: 'service_stopped', data: { service: name } });
      res.json({ success: true, service: name, message: `${name} stopped`, raw: result.data });
    } else {
      // Try killing tmux session directly
      const sessionName = `gt-${name}`;
      try {
        await execAsync(`tmux kill-session -t ${sessionName} 2>/dev/null`);
        broadcast({ type: 'service_stopped', data: { service: name } });
        res.json({ success: true, service: name, message: `${name} stopped via tmux` });
      } catch {
        res.status(500).json({ success: false, error: result.error });
      }
    }
  } catch (err) {
    console.error(`[Service] Failed to stop ${name}:`, err);
    res.status(500).json({ success: false, error: err.message });
  }
});

// Restart a service
app.post('/api/service/:name/restart', async (req, res) => {
  const { name } = req.params;
  const validServices = ['mayor', 'witness', 'refinery', 'deacon'];

  if (!validServices.includes(name.toLowerCase())) {
    return res.status(400).json({ error: `Invalid service: ${name}. Valid services: ${validServices.join(', ')}` });
  }

  console.log(`[Service] Restarting ${name}...`);

  try {
    // Stop first
    try {
      await executeGT([name, 'down'], { timeout: 10000 });
    } catch {
      // Ignore stop errors
    }

    // Wait a moment
    await new Promise(resolve => setTimeout(resolve, 1000));

    // Start
    const result = await executeGT([name, 'up'], { timeout: 30000 });

    if (result.success) {
      broadcast({ type: 'service_restarted', data: { service: name } });
      res.json({ success: true, service: name, message: `${name} restarted`, raw: result.data });
    } else {
      res.status(500).json({ success: false, error: result.error });
    }
  } catch (err) {
    console.error(`[Service] Failed to restart ${name}:`, err);
    res.status(500).json({ success: false, error: err.message });
  }
});

// Get service status
app.get('/api/service/:name/status', async (req, res) => {
  const { name } = req.params;

  try {
    const runningPolecats = await getRunningPolecats();
    const sessionName = `gt-${name}`;

    // Check if service has a tmux session
    const { stdout } = await execAsync('tmux ls 2>/dev/null || echo ""');
    const running = stdout.includes(sessionName);

    res.json({ service: name, running, session: running ? sessionName : null });
  } catch (err) {
    res.json({ service: name, running: false, error: err.message });
  }
});

// ============= Formula Management =============

// List all formulas
app.get('/api/formulas', async (req, res) => {
  const result = await executeGT(['formula', 'list', '--json']);

  if (result.success) {
    const formulas = parseJSON(result.data) || [];
    res.json(formulas);
  } else {
    // Fallback: try bd formula list
    try {
      const { stdout } = await execAsync('bd formula list --json', {
        cwd: GT_ROOT,
        timeout: 10000
      });
      const formulas = JSON.parse(stdout || '[]');
      res.json(formulas);
    } catch {
      res.json([]);
    }
  }
});

// Search formulas
app.get('/api/formulas/search', async (req, res) => {
  const query = req.query.q || '';

  try {
    // Get all formulas and filter
    const result = await executeGT(['formula', 'list', '--json']);
    const formulas = parseJSON(result.data) || [];

    const filtered = formulas.filter(f => {
      const name = (f.name || '').toLowerCase();
      const desc = (f.description || '').toLowerCase();
      const q = query.toLowerCase();
      return name.includes(q) || desc.includes(q);
    });

    res.json(filtered);
  } catch {
    res.json([]);
  }
});

// Get formula details
app.get('/api/formula/:name', async (req, res) => {
  const { name } = req.params;
  const result = await executeGT(['formula', 'show', name, '--json']);

  if (result.success) {
    const formula = parseJSON(result.data) || {};
    res.json(formula);
  } else {
    res.status(404).json({ error: 'Formula not found' });
  }
});

// Create a new formula
app.post('/api/formulas', async (req, res) => {
  const { name, description, template } = req.body;

  if (!name) {
    return res.status(400).json({ error: 'Name is required' });
  }

  const args = ['formula', 'create', name];
  if (description) {
    args.push('--description', description);
  }
  if (template) {
    args.push('--template', template);
  }

  const result = await executeGT(args);

  if (result.success) {
    broadcast({ type: 'formula_created', data: { name } });
    res.json({ success: true, name, raw: result.data });
  } else {
    res.status(500).json({ success: false, error: result.error });
  }
});

// Use a formula (create work from formula)
app.post('/api/formula/:name/use', async (req, res) => {
  const { name } = req.params;
  const { target, args: formulaArgs } = req.body;

  const cmdArgs = ['formula', 'use', name];
  if (target) {
    cmdArgs.push('--target', target);
  }
  if (formulaArgs) {
    cmdArgs.push('--args', formulaArgs);
  }

  const result = await executeGT(cmdArgs, { timeout: 30000 });

  if (result.success) {
    broadcast({ type: 'formula_used', data: { name, target } });
    res.json({ success: true, name, target, raw: result.data });
  } else {
    res.status(500).json({ success: false, error: result.error });
  }
});

// ============= GitHub Integration =============

// Extract GitHub repo from git_url
function extractGitHubRepo(gitUrl) {
  if (!gitUrl) return null;
  // Handle: https://github.com/owner/repo, git@github.com:owner/repo, etc.
  const match = gitUrl.match(/github\.com[:/]([^/]+\/[^/.\s]+)/);
  if (match) {
    // Remove .git suffix if present
    return match[1].replace(/\.git$/, '');
  }
  return null;
}

// Get all GitHub PRs across rigs
app.get('/api/github/prs', async (req, res) => {
  const state = req.query.state || 'open'; // open, closed, all
  const allPRs = [];

  try {
    // Get status to find rigs and their git_urls
    const result = await executeGT(['status', '--json']);
    if (!result.success) {
      return res.status(500).json({ error: 'Failed to get status' });
    }

    const data = parseJSON(result.data) || {};
    const rigs = data.rigs || [];

    // Read config.json for each rig to get git_url
    for (const rig of rigs) {
      try {
        const rigConfigPath = path.join(GT_ROOT, rig.name, 'config.json');
        if (fs.existsSync(rigConfigPath)) {
          const rigConfig = JSON.parse(fs.readFileSync(rigConfigPath, 'utf8'));
          const repo = extractGitHubRepo(rigConfig.git_url);

          if (repo) {
            // Fetch PRs for this repo using gh CLI
            try {
              const { stdout } = await execAsync(
                `gh pr list --repo ${repo} --state ${state} --json number,title,author,createdAt,updatedAt,url,headRefName,state,isDraft,reviewDecision --limit 20`,
                { timeout: 15000 }
              );

              const prs = JSON.parse(stdout || '[]');
              prs.forEach(pr => {
                allPRs.push({
                  ...pr,
                  rig: rig.name,
                  repo: repo
                });
              });
            } catch (ghErr) {
              console.error(`[GitHub] Failed to fetch PRs for ${repo}:`, ghErr.message);
            }
          }
        }
      } catch (e) {
        console.error(`[GitHub] Error reading rig config for ${rig.name}:`, e.message);
      }
    }

    // Sort by updated date descending
    allPRs.sort((a, b) => new Date(b.updatedAt) - new Date(a.updatedAt));

    res.json(allPRs);
  } catch (err) {
    console.error('[GitHub] Error fetching PRs:', err);
    res.status(500).json({ error: err.message });
  }
});

// Get PR details
app.get('/api/github/pr/:repo/:number', async (req, res) => {
  const { repo, number } = req.params;

  try {
    const { stdout } = await execAsync(
      `gh pr view ${number} --repo ${repo} --json number,title,author,body,createdAt,updatedAt,url,headRefName,baseRefName,state,isDraft,additions,deletions,commits,files,reviews,comments`,
      { timeout: 15000 }
    );

    const pr = JSON.parse(stdout);
    res.json(pr);
  } catch (err) {
    console.error(`[GitHub] Error fetching PR #${number}:`, err.message);
    res.status(500).json({ error: err.message });
  }
});

// Get GitHub issues across all rigs
app.get('/api/github/issues', async (req, res) => {
  const state = req.query.state || 'open'; // open, closed, all
  const allIssues = [];

  try {
    // Get status to find rigs and their git_urls
    const result = await executeGT(['status', '--json']);
    if (!result.success) {
      return res.status(500).json({ error: 'Failed to get status' });
    }

    const status = parseJSON(result.data);
    const rigs = status?.rigs || [];

    // Extract GitHub repos from rigs
    for (const rig of rigs) {
      let repoUrl = rig.git_url || rig.github_url;

      // Try to read from config if not in status
      if (!repoUrl && rig.path) {
        try {
          const configPath = path.join(rig.path, '.bd', 'config.json');
          const configContent = await fs.readFile(configPath, 'utf-8');
          const config = JSON.parse(configContent);
          repoUrl = config.git_url || config.github_url || config.remote?.github;
        } catch (e) {
          // No config
        }
      }

      if (repoUrl) {
        // Extract owner/repo from URL
        const match = repoUrl.match(/github\.com[\/:]([^\/]+)\/([^\/\.]+)/);
        if (match) {
          const repo = `${match[1]}/${match[2]}`;
          try {
            // Use gh CLI to list issues
            const { stdout } = await execAsync(
              `gh issue list --repo ${repo} --state ${state} --json number,title,author,labels,createdAt,updatedAt,url,state,body --limit 50`,
              { timeout: 15000 }
            );
            const issues = JSON.parse(stdout || '[]');
            issues.forEach(issue => {
              allIssues.push({
                ...issue,
                repo,
                rig: rig.name,
              });
            });
          } catch (e) {
            console.warn(`[GitHub] Failed to fetch issues for ${repo}:`, e.message);
          }
        }
      }
    }

    // Sort by updatedAt descending
    allIssues.sort((a, b) => new Date(b.updatedAt) - new Date(a.updatedAt));

    res.json(allIssues);
  } catch (err) {
    console.error('[GitHub] Error fetching issues:', err);
    res.status(500).json({ error: err.message });
  }
});

// Get GitHub issue details
app.get('/api/github/issue/:repo/:number', async (req, res) => {
  const { repo, number } = req.params;

  try {
    const { stdout } = await execAsync(
      `gh issue view ${number} --repo ${repo} --json number,title,author,body,createdAt,updatedAt,url,state,labels,comments,assignees`,
      { timeout: 15000 }
    );

    const issue = JSON.parse(stdout);
    res.json(issue);
  } catch (err) {
    console.error(`[GitHub] Error fetching issue #${number}:`, err.message);
    res.status(500).json({ error: err.message });
  }
});

// ============= WebSocket for Real-time Events =============

// Start activity stream
let activityProcess = null;

function startActivityStream() {
  if (activityProcess) return;

  console.log('[WS] Starting activity stream...');

  activityProcess = spawn('bd', ['activity', '--follow'], {
    cwd: GT_ROOT,
    shell: true
  });

  activityProcess.stdout.on('data', (data) => {
    const lines = data.toString().split('\n').filter(Boolean);
    lines.forEach(line => {
      const event = parseActivityLine(line);
      if (event) {
        broadcast({ type: 'activity', data: event });
      }
    });
  });

  activityProcess.stderr.on('data', (data) => {
    console.error(`[BD Activity] stderr: ${data}`);
  });

  activityProcess.on('close', (code) => {
    console.log(`[BD Activity] Process exited with code ${code}`);
    activityProcess = null;
    // Restart after delay if clients connected
    if (clients.size > 0) {
      setTimeout(startActivityStream, 5000);
    }
  });
}

// Parse activity line from bd activity output
// Format: [HH:MM:SS] SYMBOL BEAD_ID action · description
function parseActivityLine(line) {
  const match = line.match(/^\[(\d{2}:\d{2}:\d{2})\]\s+([+\u2192\u2713\u2717\u2298\ud83d\udccc])\s+(\S+)\s+(.+)$/u);
  if (!match) return null;

  const [, time, symbol, target, rest] = match;
  const [action, ...descParts] = rest.split(' · ');

  const typeMap = {
    '+': 'create',
    '\u2192': 'update',   // →
    '\u2713': 'complete', // ✓
    '\u2717': 'fail',     // ✗
    '\u2298': 'delete',   // ⊘
    '\ud83d\udccc': 'pin' // 📌
  };

  return {
    time,
    type: typeMap[symbol] || 'unknown',
    target,
    action: action.trim(),
    message: descParts.join(' · ').trim(),
    timestamp: new Date().toISOString()
  };
}

// WebSocket connection handler
wss.on('connection', (ws) => {
  console.log('[WS] Client connected');
  clients.add(ws);

  // Start activity stream if first client
  if (clients.size === 1) {
    startActivityStream();
  }

  // Send initial status
  executeGT(['status', '--json', '--fast']).then(result => {
    if (result.success) {
      ws.send(JSON.stringify({ type: 'status', data: parseJSON(result.data) }));
    }
  });

  ws.on('close', () => {
    console.log('[WS] Client disconnected');
    clients.delete(ws);

    // Stop activity stream if no clients
    if (clients.size === 0 && activityProcess) {
      activityProcess.kill();
      activityProcess = null;
    }
  });

  ws.on('error', (error) => {
    console.error('[WS] Error:', error);
  });
});

// ============= Start Server =============

server.listen(PORT, () => {
  console.log(`
╔══════════════════════════════════════════════════════════╗
║              GAS TOWN GUI SERVER                         ║
╠══════════════════════════════════════════════════════════╣
║  URL:        http://localhost:${PORT}                       ║
║  GT_ROOT:    ${GT_ROOT.padEnd(40)}║
║  WebSocket:  ws://localhost:${PORT}/ws                      ║
╚══════════════════════════════════════════════════════════╝
  `);
});

// Graceful shutdown
process.on('SIGINT', () => {
  console.log('\n[Server] Shutting down...');
  if (activityProcess) {
    activityProcess.kill();
  }
  wss.close();
  server.close(() => {
    process.exit(0);
  });
});
