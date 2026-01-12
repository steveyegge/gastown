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
import { spawn, execFile } from 'child_process';
import { promisify } from 'util';
import path from 'path';
import fs from 'fs';
import fsPromises from 'fs/promises';
import os from 'os';
import readline from 'readline';
import { fileURLToPath } from 'url';
import cors from 'cors';

const execFileAsync = promisify(execFile);
const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const app = express();
const server = createServer(app);
const wss = new WebSocketServer({ server });

const PORT = process.env.PORT || 3000;
const HOST = process.env.HOST || '127.0.0.1';
const HOME = process.env.HOME || require('os').homedir();
const GT_ROOT = process.env.GT_ROOT || path.join(HOME, 'gt');

// Simple in-memory cache with TTL
const cache = new Map();
const CACHE_TTL = {
  status: 5000,       // 5 seconds for status (frequently changing)
  convoys: 10000,     // 10 seconds for convoys
  mail: 15000,        // 15 seconds for mail list
  agents: 15000,      // 15 seconds for agents
  rigs: 30000,        // 30 seconds for rigs (rarely changes)
  formulas: 60000,    // 1 minute for formulas (rarely changes)
  github_prs: 30000,  // 30 seconds for GitHub PRs
  github_issues: 30000, // 30 seconds for GitHub issues
  doctor: 30000,      // 30 seconds for doctor
};

const mailFeedCache = {
  mtimeMs: 0,
  size: 0,
  events: null,
};

function getCached(key) {
  const entry = cache.get(key);
  if (entry && Date.now() < entry.expires) {
    return entry.data;
  }
  cache.delete(key);
  return null;
}

function setCache(key, data, ttl) {
  cache.set(key, { data, expires: Date.now() + ttl });
}

// Rig config cache TTL (5 minutes - rig configs rarely change)
const RIG_CONFIG_TTL = 300000;

/**
 * Get rig configuration with caching
 * @param {string} rigName - Name of the rig
 * @returns {Promise<Object|null>} - Rig config or null if not found
 */
async function getRigConfig(rigName) {
  const cacheKey = `rig-config:${rigName}`;
  const cached = getCached(cacheKey);
  if (cached !== null) return cached;

  try {
    const rigConfigPath = path.join(GT_ROOT, rigName, 'config.json');
    const rigConfigContent = await fsPromises.readFile(rigConfigPath, 'utf8');
    const config = JSON.parse(rigConfigContent);
    setCache(cacheKey, config, RIG_CONFIG_TTL);
    return config;
  } catch (e) {
    // Config not found or invalid - cache null to avoid repeated reads
    setCache(cacheKey, null, 60000); // Cache null for 1 minute
    return null;
  }
}

// Cache cleanup interval - removes expired entries to prevent memory leaks
const CACHE_CLEANUP_INTERVAL = 60000; // 1 minute
setInterval(() => {
  const now = Date.now();
  let cleaned = 0;
  for (const [key, entry] of cache.entries()) {
    if (now >= entry.expires) {
      cache.delete(key);
      cleaned++;
    }
  }
  if (cleaned > 0) {
    console.log(`[Cache] Cleaned ${cleaned} expired entries, ${cache.size} remaining`);
  }
}, CACHE_CLEANUP_INTERVAL);

// Pending requests map - prevents duplicate concurrent requests for same data
const pendingRequests = new Map();

// Get or create a pending request - deduplicates concurrent calls
function getPendingOrExecute(key, executor) {
  // Return cached data if available
  const cached = getCached(key);
  if (cached) return Promise.resolve(cached);

  // Return existing pending request if one is in flight
  if (pendingRequests.has(key)) {
    return pendingRequests.get(key);
  }

  // Execute and store promise
  const promise = executor().finally(() => {
    pendingRequests.delete(key);
  });
  pendingRequests.set(key, promise);
  return promise;
}

// Middleware
app.disable('x-powered-by');

const defaultOrigins = [
  `http://localhost:${PORT}`,
  `http://127.0.0.1:${PORT}`,
];
const allowedOrigins = process.env.CORS_ORIGINS
  ? process.env.CORS_ORIGINS.split(',').map(origin => origin.trim()).filter(Boolean)
  : defaultOrigins;
const allowAllOrigins = allowedOrigins.includes('*');
const allowNullOrigin = process.env.ALLOW_NULL_ORIGIN === 'true';

app.use(cors({
  origin: (origin, callback) => {
    if (!origin) return callback(null, true);
    if (allowAllOrigins) return callback(null, true);
    if (origin === 'null') return callback(allowNullOrigin ? null : new Error('CORS origin not allowed'), allowNullOrigin);
    if (allowedOrigins.includes(origin)) return callback(null, true);
    return callback(new Error('CORS origin not allowed'));
  },
}));
app.use(express.json({ limit: '1mb' }));
app.use('/assets', express.static(path.join(__dirname, 'assets')));
app.use('/css', express.static(path.join(__dirname, 'css')));
// Add cache-control headers for JS files to improve load times
app.use('/js', express.static(path.join(__dirname, 'js'), {
  maxAge: '1h',
  setHeaders: (res, filePath) => {
    // Set cache-control for JS files
    if (filePath.endsWith('.js')) {
      res.setHeader('Cache-Control', 'public, max-age=3600');
    }
  }
}));
app.get('/', (req, res) => {
  res.sendFile(path.join(__dirname, 'index.html'));
});
app.get('/favicon.ico', (req, res) => {
  res.sendFile(path.join(__dirname, 'assets', 'favicon.ico'));
});

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

// Safely quote shell arguments to prevent command injection
// Escapes all shell metacharacters and wraps in single quotes
function quoteArg(arg) {
  if (arg === null || arg === undefined) return "''";
  const str = String(arg);
  // Single quotes are the safest - only need to escape single quotes themselves
  // Replace each ' with '\'' (end quote, escaped quote, start quote)
  return "'" + str.replace(/'/g, "'\\''") + "'";
}

const SAFE_SEGMENT_RE = /^[A-Za-z0-9._-]+$/;

function isSafeSegment(value) {
  if (typeof value !== 'string' || value.length === 0 || value.length > 128) return false;
  if (value === '.' || value === '..') return false;
  return SAFE_SEGMENT_RE.test(value);
}

function validateRigAndName(req, res) {
  const { rig, name } = req.params;
  if (!isSafeSegment(rig) || !isSafeSegment(name)) {
    res.status(400).json({ error: 'Invalid rig or agent name' });
    return false;
  }
  return true;
}

// Check if a specific tmux session is running
async function isSessionRunning(sessionName) {
  try {
    const { stdout } = await execFileAsync('tmux', ['has-session', '-t', sessionName]);
    return true;
  } catch {
    return false;
  }
}

// Mayor message history (in-memory, last 100 messages)
const mayorMessageHistory = [];
const MAX_MESSAGE_HISTORY = 100;

function addMayorMessage(target, message, status, response = null) {
  const entry = {
    id: Date.now().toString(36) + Math.random().toString(36).substr(2, 5),
    timestamp: new Date().toISOString(),
    target,
    message,
    status, // 'sent', 'failed', 'auto-started'
    response
  };
  mayorMessageHistory.unshift(entry);
  if (mayorMessageHistory.length > MAX_MESSAGE_HISTORY) {
    mayorMessageHistory.pop();
  }
  // Broadcast to connected clients
  broadcast({ type: 'mayor_message', data: entry });
  return entry;
}

// Get running tmux sessions for polecats
async function getRunningPolecats() {
  try {
    const { stdout } = await execFileAsync('tmux', ['ls']);
    const sessions = new Set();
    // Parse tmux ls output: "gt-rig-polecat: 1 windows (created ...)"
    for (const line of String(stdout || '').split('\n')) {
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

// Parse GitHub URL to extract owner/repo
function parseGitHubUrl(url) {
  if (!url) return null;

  // Handle various GitHub URL formats:
  // https://github.com/owner/repo
  // https://github.com/owner/repo.git
  // git@github.com:owner/repo.git
  // ssh://git@github.com/owner/repo.git

  let match = url.match(/github\.com[/:]([^/]+)\/([^/.\s]+)/);
  if (match) {
    return { owner: match[1], repo: match[2].replace(/\.git$/, '') };
  }
  return null;
}

// Get default branch for a GitHub repo
async function getDefaultBranch(url) {
  const parsed = parseGitHubUrl(url);
  if (!parsed) {
    console.log(`[GitHub] Could not parse URL: ${url}`);
    return null;
  }

  try {
    // Use gh api to get repo info including default branch
    const { stdout } = await execFileAsync('gh', [
      'api', `repos/${parsed.owner}/${parsed.repo}`, '--jq', '.default_branch'
    ], { timeout: 10000 });

    const branch = String(stdout || '').trim();
    if (branch) {
      console.log(`[GitHub] Detected default branch for ${parsed.owner}/${parsed.repo}: ${branch}`);
      return branch;
    }
  } catch (err) {
    console.warn(`[GitHub] Could not detect default branch for ${url}:`, err.message);
  }

  return null;
}

// Get polecat output from tmux (last N lines)
async function getPolecatOutput(sessionName, lines = 50) {
  try {
    const safeLines = Math.max(1, Math.min(10000, parseInt(lines, 10) || 50));
    const { stdout } = await execFileAsync('tmux', ['capture-pane', '-t', sessionName, '-p']);
    const output = String(stdout || '');
    if (!output) return '';
    const outputLines = output.split('\n');
    return outputLines.slice(-safeLines).join('\n').trim();
  } catch {
    return null;
  }
}

// Execute a Gas Town command
async function executeGT(args, options = {}) {
  const cmd = `gt ${args.join(' ')}`;
  console.log(`[GT] Executing: ${cmd}`);

  try {
    const { stdout, stderr } = await execFileAsync('gt', args, {
      cwd: options.cwd || GT_ROOT,
      timeout: options.timeout || 30000,
      env: { ...process.env, ...options.env }
    });

    if (stderr && !options.ignoreStderr) {
      console.warn(`[GT] stderr: ${stderr}`);
    }

    return { success: true, data: String(stdout || '').trim() };
  } catch (error) {
    // Combine stdout and stderr for error output
    const output = String(error.stdout || '') + '\n' + String(error.stderr || '');
    const trimmedOutput = output.trim();

    // Check if this looks like a real error (contains "Error:" or "error:")
    const looksLikeError = /\bError:/i.test(trimmedOutput) || error.code !== 0;

    // Commands like 'gt doctor' or 'gt status' exit with code 1 when issues found, but still have useful output
    // However, if output contains "Error:" it's a real error, not just informational
    if (trimmedOutput && !looksLikeError) {
      console.warn(`[GT] Command exited with non-zero but has output: ${error.message}`);
      console.warn(`[GT] Output:\n${trimmedOutput}`);
      return { success: true, data: trimmedOutput, exitCode: error.code };
    }

    console.error(`[GT] Error: ${error.message}`);
    if (trimmedOutput) console.error(`[GT] Output:\n${trimmedOutput}`);
    return { success: false, error: trimmedOutput || error.message, exitCode: error.code };
  }
}

// Execute a Beads command
async function executeBD(args, options = {}) {
  const cmd = `bd ${args.join(' ')}`;
  console.log(`[BD] Executing: ${cmd}`);

  // Set BEADS_DIR to ensure bd finds the database
  const beadsDir = path.join(GT_ROOT, '.beads');

  try {
    const { stdout } = await execFileAsync('bd', args, {
      cwd: options.cwd || GT_ROOT,
      timeout: options.timeout || 30000,
      env: { ...process.env, BEADS_DIR: beadsDir }
    });

    return { success: true, data: String(stdout || '').trim() };
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

async function loadMailFeedEvents(feedPath) {
  const stats = await fsPromises.stat(feedPath);
  if (mailFeedCache.events &&
      mailFeedCache.mtimeMs === stats.mtimeMs &&
      mailFeedCache.size === stats.size) {
    return mailFeedCache.events;
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
    } catch {
      // Skip malformed lines
    }
  }

  // Sort newest first
  mailEvents.sort((a, b) => new Date(b.timestamp) - new Date(a.timestamp));

  mailFeedCache.events = mailEvents;
  mailFeedCache.mtimeMs = stats.mtimeMs;
  mailFeedCache.size = stats.size;

  return mailEvents;
}

// ============= REST API Endpoints =============

// Town status overview
app.get('/api/status', async (req, res) => {
  // Check cache first (skip if ?refresh=true)
  if (req.query.refresh !== 'true') {
    const cached = getCached('status');
    if (cached) {
      return res.json(cached);
    }
  }

  const [result, runningPolecats] = await Promise.all([
    executeGT(['status', '--json', '--fast']),
    getRunningPolecats()
  ]);

  if (result.success) {
    const data = parseJSON(result.data);
    if (data) {
      // Enhance rigs with running state from tmux and git_url from config
      for (const rig of data.rigs || []) {
        // Get git_url from cached rig config
        const rigConfig = await getRigConfig(rig.name);
        if (rigConfig) {
          rig.git_url = rigConfig.git_url || null;
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

      // Cache the result
      setCache('status', data, CACHE_TTL.status);
    }
    res.json(data || { raw: result.data });
  } else {
    res.status(500).json({ error: result.error });
  }
});

// List convoys
app.get('/api/convoys', async (req, res) => {
  const cacheKey = `convoys_${req.query.all || 'false'}_${req.query.status || 'all'}`;

  // Check cache
  if (req.query.refresh !== 'true') {
    const cached = getCached(cacheKey);
    if (cached) return res.json(cached);
  }

  const args = ['convoy', 'list', '--json'];
  if (req.query.all === 'true') args.push('--all');
  if (req.query.status) args.push(`--status=${req.query.status}`);

  const result = await executeGT(args);
  if (result.success) {
    const data = parseJSON(result.data) || [];
    setCache(cacheKey, data, CACHE_TTL.convoys);
    res.json(data);
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
    // Check for common errors and provide helpful messages
    const errorMsg = result.error || '';

    // Formula not found error
    const formulaMatch = errorMsg.match(/formula '([^']+)' not found/);
    if (formulaMatch) {
      const formulaName = formulaMatch[1];
      return res.status(400).json({
        error: `Formula '${formulaName}' not found`,
        errorType: 'formula_missing',
        formula: formulaName,
        hint: `Create the formula at ~/.beads/formulas/${formulaName}.toml or try a different quality level`,
        fix: {
          action: 'create_formula',
          formula: formulaName,
          command: `mkdir -p ~/.beads/formulas && cat > ~/.beads/formulas/${formulaName}.toml`
        }
      });
    }

    // Bead not found error
    if (errorMsg.includes('bead') && errorMsg.includes('not found')) {
      return res.status(400).json({
        error: 'Bead not found',
        errorType: 'bead_missing',
        hint: 'The issue/bead ID does not exist. Check the ID or create a new bead.',
        fix: {
          action: 'search_beads',
          command: 'bd list'
        }
      });
    }

    res.status(500).json({ error: errorMsg || 'Sling failed - no work attached' });
  }
});

// Get available sling targets
app.get('/api/targets', async (req, res) => {
  try {
    // Get status which includes rigs and agents
    const statusResult = await getPendingOrExecute('status', async () => {
      const result = await executeGT(['status', '--json', '--fast']);
      if (result.success) {
        const data = parseJSON(result.data) || {};
        setCache('status', data, CACHE_TTL.status);
        return data;
      }
      return null;
    });

    const status = statusResult || {};
    const rigs = status.rigs || [];
    const targets = [];

    // Global agents
    targets.push({
      id: 'mayor',
      name: 'Mayor',
      type: 'global',
      icon: 'account_balance',
      description: 'Global coordinator - dispatches work across all projects'
    });
    targets.push({
      id: 'deacon',
      name: 'Deacon',
      type: 'global',
      icon: 'health_and_safety',
      description: 'Health monitor - can dispatch to dogs'
    });
    targets.push({
      id: 'deacon/dogs',
      name: 'Deacon Dogs',
      type: 'global',
      icon: 'pets',
      description: 'Auto-dispatch to an idle dog worker'
    });

    // Rigs (can spawn polecats)
    rigs.forEach(rig => {
      targets.push({
        id: rig.name,
        name: rig.name,
        type: 'rig',
        icon: 'folder_special',
        description: `Auto-spawn polecat in ${rig.name}`
      });

      // Existing agents in rig
      if (rig.agents) {
        rig.agents.forEach(agent => {
          if (agent.running) {
            targets.push({
              id: `${rig.name}/${agent.name}`,
              name: `${rig.name}/${agent.name}`,
              type: 'agent',
              role: agent.role,
              icon: agent.role === 'witness' ? 'visibility' :
                    agent.role === 'refinery' ? 'merge_type' : 'engineering',
              description: `${agent.role} in ${rig.name}`,
              running: agent.running,
              has_work: agent.has_work
            });
          }
        });
      }
    });

    res.json(targets);
  } catch (err) {
    console.error('[API] Error getting targets:', err);
    res.status(500).json({ error: err.message });
  }
});

// Escalate issue to human overseer
app.post('/api/escalate', async (req, res) => {
  // UI sends: convoy_id, reason, priority
  // gt escalate expects: <topic> -s <severity> -m <message>
  const { convoy_id, reason, priority } = req.body;

  if (!reason) {
    return res.status(400).json({ error: 'Reason is required' });
  }

  // Map priority to severity: normal→MEDIUM, high→HIGH, critical→CRITICAL
  const severityMap = {
    normal: 'MEDIUM',
    high: 'HIGH',
    critical: 'CRITICAL'
  };
  const severity = severityMap[priority] || 'MEDIUM';

  // Build topic from convoy context
  const topic = convoy_id
    ? `Convoy ${convoy_id.slice(0, 8)} needs attention`
    : 'Issue needs attention';

  const args = ['escalate', topic, '-s', severity, '-m', reason];

  const result = await executeGT(args);
  if (result.success) {
    broadcast({ type: 'escalation', data: { convoy_id, reason, priority, severity } });
    res.json({ success: true, data: result.data });
  } else {
    res.status(500).json({ error: result.error });
  }
});

// Get mail inbox
app.get('/api/mail', async (req, res) => {
  // Check cache
  if (req.query.refresh !== 'true') {
    const cached = getCached('mail');
    if (cached) return res.json(cached);
  }

  const result = await executeGT(['mail', 'inbox', '--json']);
  if (result.success) {
    const data = parseJSON(result.data) || [];
    setCache('mail', data, CACHE_TTL.mail);
    res.json(data);
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

// Get all mail from feed (for observability) with pagination
app.get('/api/mail/all', async (req, res) => {
  try {
    // Pagination params (default: page 1, 50 items per page)
    const page = Math.max(1, parseInt(req.query.page, 10) || 1);
    const limit = Math.min(200, Math.max(1, parseInt(req.query.limit, 10) || 50));
    const offset = (page - 1) * limit;

    const feedPath = path.join(GT_ROOT, '.events.jsonl');
    try {
      await fsPromises.access(feedPath);
    } catch {
      mailFeedCache.events = null;
      return res.json({ items: [], total: 0, page, limit, hasMore: false });
    }

    const mailEvents = await loadMailFeedEvents(feedPath);

    // Apply pagination
    const total = mailEvents.length;
    const paginatedItems = mailEvents.slice(offset, offset + limit);
    const hasMore = offset + limit < total;

    res.json({
      items: paginatedItems,
      total,
      page,
      limit,
      hasMore
    });
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

// ============= Nudge API =============

// Send a message to Mayor (or other agent)
app.post('/api/nudge', async (req, res) => {
  const { target, message, autoStart = true } = req.body;

  if (!message) {
    return res.status(400).json({ error: 'Message is required' });
  }

  // Default to mayor if no target specified
  const nudgeTarget = target || 'mayor';
  const sessionName = `gt-${nudgeTarget}`;

  try {
    // Check if target session is running
    const isRunning = await isSessionRunning(sessionName);
    let wasAutoStarted = false;

    if (!isRunning) {
      console.log(`[Nudge] Session ${sessionName} not running`);

      // Auto-start Mayor if requested
      if (nudgeTarget === 'mayor' && autoStart) {
        console.log(`[Nudge] Auto-starting Mayor...`);
        const startResult = await executeGT(['mayor', 'start'], { timeout: 30000 });

        if (!startResult.success) {
          const entry = addMayorMessage(nudgeTarget, message, 'failed', 'Failed to auto-start Mayor');
          return res.status(500).json({
            error: 'Mayor not running and failed to auto-start',
            details: startResult.error,
            messageId: entry.id
          });
        }

        wasAutoStarted = true;
        console.log(`[Nudge] Mayor auto-started successfully`);

        // Wait a moment for Mayor to initialize
        await new Promise(resolve => setTimeout(resolve, 2000));

        // Broadcast that Mayor was started
        broadcast({ type: 'service_started', data: { service: 'mayor', autoStarted: true } });
      } else if (!isRunning) {
        const entry = addMayorMessage(nudgeTarget, message, 'failed', `Session ${sessionName} not running`);
        return res.status(400).json({
          error: `${nudgeTarget} is not running`,
          hint: nudgeTarget === 'mayor' ? 'Set autoStart: true to start Mayor automatically' : `Start the ${nudgeTarget} service first`,
          messageId: entry.id
        });
      }
    }

    // Send the nudge
    const result = await executeGT(['nudge', nudgeTarget, message], { timeout: 10000 });

    if (result.success) {
      const status = wasAutoStarted ? 'auto-started' : 'sent';
      const entry = addMayorMessage(nudgeTarget, message, status);
      res.json({
        success: true,
        target: nudgeTarget,
        message,
        wasAutoStarted,
        messageId: entry.id
      });
    } else {
      const entry = addMayorMessage(nudgeTarget, message, 'failed', result.error);
      res.status(500).json({
        error: result.error || 'Failed to send message',
        messageId: entry.id
      });
    }
  } catch (err) {
    const entry = addMayorMessage(nudgeTarget, message, 'failed', err.message);
    res.status(500).json({ error: err.message, messageId: entry.id });
  }
});

// Get Mayor message history
app.get('/api/mayor/messages', (req, res) => {
  const limit = Math.min(parseInt(req.query.limit) || 50, MAX_MESSAGE_HISTORY);
  res.json(mayorMessageHistory.slice(0, limit));
});

// ============= Beads API =============

// Create a new bead (issue)
app.post('/api/beads', async (req, res) => {
  const { title, description, priority, labels } = req.body;

  if (!title) {
    return res.status(400).json({ error: 'Title is required' });
  }

  // Map word priorities to bd's P0-P4 format
  const priorityMap = {
    'urgent': 'P0',      // Urgent/Critical = highest priority
    'critical': 'P0',
    'high': 'P1',
    'normal': 'P2',
    'low': 'P3',
    'backlog': 'P4',
  };

  // Build bd new command
  // bd new "title" --description "..." --priority P1 --label bug --label enhancement
  // Use --no-daemon to avoid timeout issues
  const args = ['--no-daemon', 'new', title];

  if (description) {
    args.push('--description', description);
  }
  if (priority && priority !== 'normal') {
    const mappedPriority = priorityMap[priority] || priority;
    args.push('--priority', mappedPriority);
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

// Get related PRs/commits for a bead
app.get('/api/bead/:beadId/links', async (req, res) => {
  const { beadId } = req.params;
  const links = { prs: [], commits: [] };

  try {
    // Get bead details to check close time for matching
    const beadResult = await executeBD(['show', beadId, '--json']);
    let beadClosedAt = null;
    if (beadResult.success) {
      const beadData = parseJSON(beadResult.data);
      const bead = Array.isArray(beadData) ? beadData[0] : beadData;
      if (bead && bead.closed_at) {
        beadClosedAt = new Date(bead.closed_at);
      }
    }

    // Get list of rig names
    const rigsResult = await executeGT(['rig', 'list']);
    if (!rigsResult.success) {
      return res.json(links);
    }

    // Parse rig names from output (lines with exactly 2 spaces before name, no colon)
    const rigNames = rigsResult.data
      .split('\n')
      .filter(line => line.match(/^  \S/) && !line.includes(':'))
      .map(line => line.trim());

    console.log(`[Links] Found rigs: ${rigNames.join(', ')}`);

    // Get repo URL for each rig by checking git remote
    for (const rigName of rigNames) {
      const rigPath = path.join(GT_ROOT, rigName, 'mayor', 'rig');

      try {
        const { stdout } = await execFileAsync('git', ['-C', rigPath, 'remote', 'get-url', 'origin'], { timeout: 5000 });
        const repoUrl = String(stdout || '').trim();

        // Extract owner/repo from GitHub URL
        const repoMatch = repoUrl.match(/github\.com[/:]([^/]+\/[^/.\s]+)/);
        if (!repoMatch) continue;
        const repo = repoMatch[1].replace(/\.git$/, '');

        // Search for PRs (title, body, branch containing bead ID, or polecat PRs near close time)
        try {
          const { stdout: prOutput } = await execFileAsync(
            'gh',
            ['pr', 'list', '--repo', repo, '--state', 'all', '--limit', '20', '--json', 'number,title,url,state,headRefName,body,createdAt,updatedAt'],
            { timeout: 10000 }
          );
          const prs = JSON.parse(String(prOutput || '') || '[]');

          for (const pr of prs) {
            // Check if PR is related to this bead
            let isRelated =
              (pr.title && pr.title.includes(beadId)) ||
              (pr.headRefName && pr.headRefName.includes(beadId)) ||
              (pr.body && pr.body.includes(beadId));

            // Also match polecat PRs created/updated within 1 hour of bead close time
            if (!isRelated && beadClosedAt && pr.headRefName && pr.headRefName.startsWith('polecat/')) {
              const prUpdated = new Date(pr.updatedAt || pr.createdAt);
              const timeDiff = Math.abs(beadClosedAt - prUpdated);
              const oneHour = 60 * 60 * 1000;
              if (timeDiff < oneHour) {
                isRelated = true;
              }
            }

            if (isRelated) {
              links.prs.push({
                repo,
                number: pr.number,
                title: pr.title,
                url: pr.url,
                state: pr.state,
                branch: pr.headRefName,
              });
            }
          }
        } catch (ghErr) {
          console.log(`[Links] Could not search ${repo}: ${ghErr.message}`);
        }
      } catch (gitErr) {
        // Skip rigs without git repos
        console.log(`[Links] Could not get repo for ${rigName}: ${gitErr.message}`);
      }
    }

    res.json(links);
  } catch (err) {
    console.error('[Links] Error:', err);
    res.json(links);
  }
});

// Get agent list
app.get('/api/agents', async (req, res) => {
  // Check cache
  if (req.query.refresh !== 'true') {
    const cached = getCached('agents');
    if (cached) return res.json(cached);
  }

  const [result, runningPolecats] = await Promise.all([
    executeGT(['status', '--json', '--fast'], { timeout: 30000 }),
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

    const response = { agents, polecats, runningPolecats: Array.from(runningPolecats) };
    setCache('agents', response, CACHE_TTL.agents);
    res.json(response);
  } else {
    res.status(500).json({ error: result.error });
  }
});

// Get Mayor output (tmux buffer)
app.get('/api/mayor/output', async (req, res) => {
  const lines = parseInt(req.query.lines) || 100;
  const sessionName = 'gt-mayor';

  try {
    const output = await getPolecatOutput(sessionName, lines);
    const isRunning = await isSessionRunning(sessionName);

    if (output !== null) {
      res.json({
        session: sessionName,
        output,
        running: isRunning,
        // Include recent messages sent to Mayor for context
        recentMessages: mayorMessageHistory.slice(0, 10)
      });
    } else {
      res.json({ session: sessionName, output: null, running: isRunning, recentMessages: [] });
    }
  } catch (err) {
    res.status(500).json({ error: err.message });
  }
});

// Get polecat output (what they're working on)
app.get('/api/polecat/:rig/:name/output', async (req, res) => {
  if (!validateRigAndName(req, res)) return;
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
  if (!validateRigAndName(req, res)) return;
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
        await fsPromises.access(transcriptPath);
        // Find most recent transcript file
        const dirFiles = await fsPromises.readdir(transcriptPath);
        const filteredFiles = dirFiles.filter(f =>
          f.endsWith('.json') || f.endsWith('.md') || f.endsWith('.jsonl')
        );

        const filesWithTime = await Promise.all(
          filteredFiles.map(async f => {
            const stat = await fsPromises.stat(path.join(transcriptPath, f));
            return { name: f, time: stat.mtime.getTime() };
          })
        );
        filesWithTime.sort((a, b) => b.time - a.time);

        if (filesWithTime.length > 0) {
          transcriptContent = await fsPromises.readFile(
            path.join(transcriptPath, filesWithTime[0].name),
            'utf-8'
          );
          break;
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
  if (!validateRigAndName(req, res)) return;
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
  if (!validateRigAndName(req, res)) return;
  const { rig, name } = req.params;
  const sessionName = `gt-${rig}-${name}`;

  console.log(`[Agent] Stopping ${rig}/${name}...`);

  try {
    // Kill the tmux session
    await execFileAsync('tmux', ['kill-session', '-t', sessionName]);
    broadcast({ type: 'agent_stopped', data: { rig, name, session: sessionName } });
    res.json({ success: true, message: `Stopped ${rig}/${name}` });
  } catch (err) {
    // Session might not exist, which is fine
    const errText = `${err.stderr || ''} ${err.message || ''}`;
    if (errText.includes("can't find session")) {
      res.json({ success: true, message: `${rig}/${name} was not running` });
    } else {
      console.error(`[Agent] Failed to stop ${rig}/${name}:`, err);
      res.status(500).json({ success: false, error: err.message });
    }
  }
});

// Restart a polecat/agent (stop then start)
app.post('/api/polecat/:rig/:name/restart', async (req, res) => {
  if (!validateRigAndName(req, res)) return;
  const { rig, name } = req.params;
  const agentPath = `${rig}/${name}`;
  const sessionName = `gt-${rig}-${name}`;

  console.log(`[Agent] Restarting ${agentPath}...`);

  try {
    // First try to kill existing session (ignore errors)
    try {
      await execFileAsync('tmux', ['kill-session', '-t', sessionName]);
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
    const gtResult = await execFileAsync('gt', ['version'], { timeout: 5000 });
    status.gt_installed = true;
    status.gt_version = String(gtResult.stdout || '').trim().split('\n')[0];
  } catch {
    status.gt_installed = false;
  }

  // Check bd
  try {
    const bdResult = await execFileAsync('bd', ['version'], { timeout: 5000 });
    status.bd_installed = true;
    status.bd_version = String(bdResult.stdout || '').trim().split('\n')[0];
  } catch {
    status.bd_installed = false;
  }

  // Check workspace
  try {
    const mayorPath = path.join(GT_ROOT, 'mayor');
    await fsPromises.access(mayorPath);
    status.workspace_initialized = true;
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

  // Detect default branch from GitHub API (handles main vs master)
  // NOTE: --branch flag requires gt to be rebuilt from source (not in current binary)
  const defaultBranch = await getDefaultBranch(url);
  if (defaultBranch) {
    console.log(`[Rig] Detected default branch: ${defaultBranch} (gt --branch flag pending rebuild)`);
  }

  // Rig operations can take 90+ seconds for large repos
  // TODO: Pass --branch when gt is rebuilt: ['rig', 'add', name, url, '--branch', defaultBranch]
  const result = await executeGT(['rig', 'add', name, url], { timeout: 120000 });

  // Check if rig add actually succeeded (not just "has output")
  // If the output contains "Error:", it's a real failure even if success=true
  const hasError = result.data && (result.data.includes('Error:') || result.data.includes('error:'));

  if (result.success && !hasError) {
    // Create agent beads for witness and refinery (targeted, not gt doctor --fix)
    const agentRoles = ['witness', 'refinery'];
    for (const role of agentRoles) {
      const beadResult = await executeBD([
        'create',
        `Setup ${role} for ${name}`,  // Title is required
        '--type', 'agent',
        '--agent-rig', name,
        '--role-type', role,
        '--silent'
      ]);
      if (!beadResult.success) {
        console.warn(`[BD] Failed to create ${role} bead for ${name}:`, beadResult.error);
      } else {
        console.log(`[BD] Created ${role} agent bead for ${name}`);
      }
    }

    broadcast({ type: 'rig_added', data: { name, url } });
    res.json({ success: true, name, raw: result.data });
  } else {
    const errorMsg = hasError ? result.data : (result.error || 'Failed to add rig');
    res.status(500).json({ success: false, error: errorMsg });
  }
});

// List rigs
app.get('/api/rigs', async (req, res) => {
  // Check cache
  if (req.query.refresh !== 'true') {
    const cached = getCached('rigs');
    if (cached) return res.json(cached);
  }

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
    setCache('rigs', rigs, CACHE_TTL.rigs);
    res.json(rigs);
  } else {
    res.json([]);
  }
});

// Remove a rig
app.delete('/api/rigs/:name', async (req, res) => {
  const { name } = req.params;

  if (!name) {
    return res.status(400).json({ error: 'Rig name is required' });
  }

  const result = await executeGT(['rig', 'remove', name]);

  if (result.success) {
    broadcast({ type: 'rig_removed', data: { name } });
    res.json({ success: true, name, raw: result.data });
  } else {
    res.status(500).json({ success: false, error: result.error });
  }
});

// Run gt doctor
app.get('/api/doctor', async (req, res) => {
  // Check cache first (skip if ?refresh=true)
  if (req.query.refresh !== 'true') {
    const cached = getCached('doctor');
    if (cached) {
      return res.json(cached);
    }
  }

  // First try with --json flag (gt doctor can take 15-20s)
  let result = await executeGT(['doctor', '--json'], { timeout: 25000 });

  if (result.success) {
    const data = parseJSON(result.data);
    if (data) {
      setCache('doctor', data, 30000); // 30s cache
      return res.json(data);
    }
    // If JSON parse failed, return raw output
    const response = { raw: result.data, checks: [] };
    setCache('doctor', response, 30000);
    return res.json(response);
  }

  // Fallback: try without --json flag (gt doctor can take 15-20s)
  result = await executeGT(['doctor'], { timeout: 25000 });

  if (result.success) {
    // Parse text output into structured format with details
    const lines = result.data.split('\n');
    const checks = [];
    let currentCheck = null;

    for (const line of lines) {
      // Parse status lines: "✓ check-name: description" or "✗ check-name: description"
      const checkMatch = line.match(/^([✓✔✗✘×⚠!])\s*([^:]+):\s*(.+)$/);

      if (checkMatch) {
        // Save previous check
        if (currentCheck) checks.push(currentCheck);

        const [, symbol, checkName, description] = checkMatch;
        const status = '✓✔'.includes(symbol) ? 'pass' : '✗✘×'.includes(symbol) ? 'fail' : 'warn';

        currentCheck = {
          id: checkName.trim(),
          name: checkName.trim(),
          description: description.trim(),
          status,
          details: [],
          fix: null
        };
      } else if (currentCheck) {
        // Capture detail lines (indented)
        const detailMatch = line.match(/^\s{4}(.+)$/);
        if (detailMatch) {
          const detail = detailMatch[1].trim();
          // Check if it's a fix command
          if (detail.startsWith('→')) {
            currentCheck.fix = detail.substring(1).trim();
          } else {
            currentCheck.details.push(detail);
          }
        }
      }
    }

    // Don't forget last check
    if (currentCheck) checks.push(currentCheck);

    // Parse summary line
    const summaryMatch = result.data.match(/(\d+)\s*checks?,\s*(\d+)\s*passed?,\s*(\d+)\s*warnings?,\s*(\d+)\s*errors?/);
    const summary = summaryMatch ? {
      total: parseInt(summaryMatch[1]),
      passed: parseInt(summaryMatch[2]),
      warnings: parseInt(summaryMatch[3]),
      errors: parseInt(summaryMatch[4])
    } : null;

    const response = { checks, summary, raw: result.data };
    setCache('doctor', response, 30000);
    return res.json(response);
  }

  // Both failed - return error but with 200 to avoid breaking the UI
  const response = {
    checks: [],
    raw: result.error || 'gt doctor command not available',
    error: result.error
  };
  setCache('doctor', response, 10000); // Short cache for errors
  res.json(response);
});

// Run gt doctor --fix
app.post('/api/doctor/fix', async (req, res) => {
  try {
    const result = await executeGT(['doctor', '--fix'], { timeout: 60000 });
    // Clear doctor cache so next check shows fresh results
    cache.delete('doctor');
    if (result.success) {
      res.json({ success: true, output: result.data });
    } else {
      res.json({ success: false, error: result.error, output: result.data || '' });
    }
  } catch (err) {
    res.status(500).json({ error: err.message });
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
    const result = await executeGT([name, 'start'], { timeout: 30000 });

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
        await execFileAsync('tmux', ['kill-session', '-t', sessionName]);
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
    const result = await executeGT([name, 'start'], { timeout: 30000 });

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
    let running = false;
    try {
      const { stdout } = await execFileAsync('tmux', ['ls']);
      running = String(stdout || '').includes(sessionName);
    } catch {
      running = false;
    }

    res.json({ service: name, running, session: running ? sessionName : null });
  } catch (err) {
    res.json({ service: name, running: false, error: err.message });
  }
});

// ============= Formula Management =============

// List all formulas
app.get('/api/formulas', async (req, res) => {
  // Check cache
  if (req.query.refresh !== 'true') {
    const cached = getCached('formulas');
    if (cached) return res.json(cached);
  }

  // Try gt formula list first
  let result = await executeGT(['formula', 'list', '--json']);

  if (result.success) {
    const formulas = parseJSON(result.data);
    if (formulas) {
      setCache('formulas', formulas, CACHE_TTL.formulas);
      return res.json(formulas);
    }
  }

  // Try without --json flag
  result = await executeGT(['formula', 'list']);
  if (result.success && result.data) {
    // Parse text output: "  formula-name - description"
    const lines = result.data.split('\n');
    const formulas = [];
    for (const line of lines) {
      const match = line.match(/^\s+(\S+)\s*(?:-\s*(.+))?$/);
      if (match) {
        formulas.push({ name: match[1], description: match[2] || '' });
      }
    }
    if (formulas.length > 0) {
      setCache('formulas', formulas, CACHE_TTL.formulas);
      return res.json(formulas);
    }
  }

  // Fallback: try bd formula list
  try {
    const { stdout } = await execFileAsync('bd', ['formula', 'list', '--json'], {
      cwd: GT_ROOT,
      timeout: 10000
    });
    const formulas = JSON.parse(String(stdout || '') || '[]');
    setCache('formulas', formulas, CACHE_TTL.formulas);
    return res.json(formulas);
  } catch {
    // Final fallback - empty array
    return res.json([]);
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
  const cacheKey = `github_prs_${state}`;

  // Check cache first (skip if ?refresh=true)
  if (req.query.refresh !== 'true') {
    const cached = getCached(cacheKey);
    if (cached) {
      return res.json(cached);
    }
  }

  try {
    // Get status with --fast flag (uses cached status data from gt)
    const result = await executeGT(['status', '--json', '--fast']);
    if (!result.success) {
      return res.status(500).json({ error: 'Failed to get status' });
    }

    const data = parseJSON(result.data) || {};
    const rigs = data.rigs || [];

    // Enrich rigs with git_url from config.json (not in raw status)
    for (const rig of rigs) {
      if (!rig.git_url) {
        const rigConfig = await getRigConfig(rig.name);
        if (rigConfig) {
          rig.git_url = rigConfig.git_url || null;
        }
      }
    }
    console.log(`[GitHub] Found ${rigs.length} rigs, ${rigs.filter(r => r.git_url).length} with git_url`);

    // Extract repos from rigs
    const repoPromises = rigs
      .filter(rig => rig.git_url)
      .map(rig => {
        const repo = extractGitHubRepo(rig.git_url);
        if (!repo) return Promise.resolve([]);

        // Fetch PRs in parallel
        return execFileAsync(
          'gh',
          ['pr', 'list', '--repo', repo, '--state', state, '--json', 'number,title,author,createdAt,updatedAt,url,headRefName,state,isDraft,reviewDecision', '--limit', '20'],
          { timeout: 10000 }
        )
          .then(({ stdout }) => {
            const prs = JSON.parse(String(stdout || '') || '[]');
            return prs.map(pr => ({ ...pr, rig: rig.name, repo }));
          })
          .catch(err => {
            console.error(`[GitHub] Failed to fetch PRs for ${repo}:`, err.message);
            return [];
          });
      });

    // Wait for all PR fetches to complete in parallel
    const results = await Promise.all(repoPromises);
    const allPRs = results.flat();

    // Sort by updated date descending
    allPRs.sort((a, b) => new Date(b.updatedAt) - new Date(a.updatedAt));

    // Cache the result
    setCache(cacheKey, allPRs, CACHE_TTL.github_prs);

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
    const { stdout } = await execFileAsync(
      'gh',
      ['pr', 'view', String(number), '--repo', repo, '--json', 'number,title,author,body,createdAt,updatedAt,url,headRefName,baseRefName,state,isDraft,additions,deletions,commits,files,reviews,comments'],
      { timeout: 15000 }
    );

    const pr = JSON.parse(String(stdout || '') || '{}');
    res.json(pr);
  } catch (err) {
    console.error(`[GitHub] Error fetching PR #${number}:`, err.message);
    res.status(500).json({ error: err.message });
  }
});

// Get GitHub issues across all rigs
app.get('/api/github/issues', async (req, res) => {
  const state = req.query.state || 'open'; // open, closed, all
  const cacheKey = `github_issues_${state}`;

  // Check cache
  if (req.query.refresh !== 'true') {
    const cached = getCached(cacheKey);
    if (cached) return res.json(cached);
  }

  try {
    // Get status with --fast flag
    const result = await executeGT(['status', '--json', '--fast']);
    if (!result.success) {
      return res.status(500).json({ error: 'Failed to get status' });
    }

    const status = parseJSON(result.data);
    const rigs = status?.rigs || [];

    // Enrich rigs with git_url from config.json
    for (const rig of rigs) {
      if (!rig.git_url) {
        const rigConfig = await getRigConfig(rig.name);
        if (rigConfig) {
          rig.git_url = rigConfig.git_url || null;
        }
      }
    }

    // Fetch issues in parallel (like PRs)
    const issuePromises = rigs
      .filter(rig => rig.git_url)
      .map(rig => {
        const repo = extractGitHubRepo(rig.git_url);
        if (!repo) return Promise.resolve([]);

        return execFileAsync(
          'gh',
          ['issue', 'list', '--repo', repo, '--state', state, '--json', 'number,title,author,labels,createdAt,updatedAt,url,state', '--limit', '30'],
          { timeout: 10000 }
        )
          .then(({ stdout }) => {
            const issues = JSON.parse(String(stdout || '') || '[]');
            return issues.map(issue => ({ ...issue, repo, rig: rig.name }));
          })
          .catch(err => {
            console.warn(`[GitHub] Failed to fetch issues for ${repo}:`, err.message);
            return [];
          });
      });

    // Wait for all fetches in parallel
    const results = await Promise.all(issuePromises);
    const allIssues = results.flat();

    // Sort by updatedAt descending
    allIssues.sort((a, b) => new Date(b.updatedAt) - new Date(a.updatedAt));

    // Cache result
    setCache(cacheKey, allIssues, CACHE_TTL.github_issues);

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
    const { stdout } = await execFileAsync(
      'gh',
      ['issue', 'view', String(number), '--repo', repo, '--json', 'number,title,author,body,createdAt,updatedAt,url,state,labels,comments,assignees'],
      { timeout: 15000 }
    );

    const issue = JSON.parse(String(stdout || '') || '{}');
    res.json(issue);
  } catch (err) {
    console.error(`[GitHub] Error fetching issue #${number}:`, err.message);
    res.status(500).json({ error: err.message });
  }
});

// List all GitHub repos for current user
app.get('/api/github/repos', async (req, res) => {
  const limit = parseInt(req.query.limit) || 100;
  const visibility = req.query.visibility; // public, private, or omit for all
  const cacheKey = `github_repos_${visibility || 'all'}_${limit}`;

  // Check cache (repos don't change often)
  if (req.query.refresh !== 'true') {
    const cached = getCached(cacheKey);
    if (cached) return res.json(cached);
  }

  try {
    const args = ['repo', 'list', '--limit', String(limit), '--json', 'name,nameWithOwner,description,url,isPrivate,isFork,pushedAt,primaryLanguage,stargazerCount'];
    if (visibility && visibility !== 'all') {
      args.push('--visibility', visibility);
    }
    const { stdout } = await execFileAsync('gh', args, { timeout: 30000 });

    const repos = JSON.parse(String(stdout || '') || '[]');

    // Sort by most recently pushed
    repos.sort((a, b) => new Date(b.pushedAt) - new Date(a.pushedAt));

    // Cache for 5 minutes
    setCache(cacheKey, repos, 5 * 60 * 1000);

    res.json(repos);
  } catch (err) {
    console.error('[GitHub] Error listing repos:', err.message);
    res.status(500).json({ error: err.message });
  }
});

// ============= WebSocket for Real-time Events =============

// Start activity stream
let activityProcess = null;

function startActivityStream() {
  if (activityProcess) return;

  console.log('[WS] Starting activity stream...');

  // Use gt feed for comprehensive activity (beads + gt events + convoys)
  activityProcess = spawn('gt', ['feed', '--plain', '--follow'], {
    cwd: GT_ROOT
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

// Parse activity line from gt feed output
// Format: [HH:MM:SS] SYMBOL TARGET action · description
function parseActivityLine(line) {
  // Match various unicode symbols used by gt feed
  const match = line.match(/^\[(\d{2}:\d{2}:\d{2})\]\s+(.+?)\s+(\S+)\s+(.+)$/u);
  if (!match) return null;

  const [, time, symbol, target, rest] = match;
  const [action, ...descParts] = rest.split(' · ');

  // Map symbols to event types (beads + gt events)
  const typeMap = {
    '+': 'bead_created',
    '→': 'bead_updated',
    '✓': 'work_complete',
    '✗': 'work_failed',
    '⊘': 'bead_deleted',
    '📌': 'bead_pinned',
    '🦉': 'patrol_started',
    '⚡': 'agent_nudged',
    '🎯': 'work_slung',
    '🤝': 'handoff',
    '⚙': 'merge_started',
    '🚀': 'convoy_created',
    '📦': 'convoy_updated',
  };

  const eventType = typeMap[symbol.trim()] || 'system';

  return {
    id: `${Date.now()}-${Math.random().toString(36).substr(2, 5)}`,
    time,
    type: eventType,
    target,
    action: action.trim(),
    message: descParts.join(' · ').trim(),
    summary: `${action.trim()}${descParts.length ? ': ' + descParts.join(' · ').trim() : ''}`,
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

  // Send initial status - uses deduplication to prevent duplicate concurrent requests
  getPendingOrExecute('status', async () => {
    const result = await executeGT(['status', '--json', '--fast']);
    if (result.success) {
      const data = parseJSON(result.data);
      if (data) setCache('status', data, CACHE_TTL.status);
      return data;
    }
    return null;
  }).then(data => {
    if (data && ws.readyState === 1) { // OPEN
      ws.send(JSON.stringify({ type: 'status', data }));
    }
  }).catch(err => {
    console.error('[WS] Error getting initial status:', err.message);
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

server.listen(PORT, HOST, () => {
  const displayHost = HOST === '0.0.0.0' || HOST === '::' ? 'localhost' : HOST;
  console.log(`
╔══════════════════════════════════════════════════════════╗
║              GAS TOWN GUI SERVER                         ║
╠══════════════════════════════════════════════════════════╣
║  URL:        http://${displayHost}:${PORT}                       ║
║  GT_ROOT:    ${GT_ROOT.padEnd(40)}║
║  WebSocket:  ws://${displayHost}:${PORT}/ws                      ║
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
