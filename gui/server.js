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
      // Enhance rigs with running state from tmux
      for (const rig of data.rigs || []) {
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
    executeGT(['status', '--json']),
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
