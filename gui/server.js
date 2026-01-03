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
const GT_ROOT = process.env.GT_ROOT || path.join(process.env.HOME, 'gt');

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

// Execute a Gas Town command
async function executeGT(args, options = {}) {
  const cmd = `gt ${args.join(' ')}`;
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
  const cmd = `bd ${args.join(' ')}`;
  console.log(`[BD] Executing: ${cmd}`);

  try {
    const { stdout, stderr } = await execAsync(cmd, {
      cwd: options.cwd || GT_ROOT,
      timeout: options.timeout || 30000
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
  const result = await executeGT(['status', '--json', '--fast']);
  if (result.success) {
    const data = parseJSON(result.data);
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
  const args = ['convoy', 'create', name, ...(issues || []), '--json'];
  if (notify) args.push('--notify', notify);

  const result = await executeGT(args);
  if (result.success) {
    broadcast({ type: 'convoy_created', data: parseJSON(result.data) });
    res.json({ success: true, data: parseJSON(result.data) });
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
  cmdArgs.push('--json');

  const result = await executeGT(cmdArgs);
  if (result.success) {
    broadcast({ type: 'work_slung', data: parseJSON(result.data) });
    res.json({ success: true, data: parseJSON(result.data) });
  } else {
    res.status(500).json({ error: result.error });
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
  const args = ['new', title];

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
  const args = query ? ['search', query] : ['list'];
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
  const args = ['list'];
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
  const result = await executeGT(['status', '--json']);
  if (result.success) {
    const data = parseJSON(result.data);
    // Extract agents from status response
    res.json(data?.agents || []);
  } else {
    res.status(500).json({ error: result.error });
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
