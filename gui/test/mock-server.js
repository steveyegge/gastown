/**
 * Gas Town GUI - Mock Server for Testing
 *
 * Provides mock API responses for E2E testing without the Go backend.
 */

import express from 'express';
import { WebSocketServer } from 'ws';
import { createServer } from 'http';
import path from 'path';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

// Mock data
const mockData = {
  status: {
    name: 'Test Town',
    version: '0.1.0',
    uptime: 3600,
    hook: null,
    agents: [
      { id: 'agent-1', name: 'Mayor', role: 'mayor', status: 'idle' },
      { id: 'agent-2', name: 'Deacon-1', role: 'deacon', status: 'working', current_task: 'Processing convoy' },
      { id: 'agent-3', name: 'Polecat-1', role: 'polecat', status: 'idle' },
    ],
    convoy_count: 2,
    active_agents: 1,
    pending_tasks: 3,
  },

  convoys: [
    {
      id: 'convoy-abc123',
      name: 'Feature Implementation',
      status: 'running',
      priority: 'high',
      issues: [{ title: 'Add user authentication' }, { title: 'Create API endpoints' }],
      progress: 0.45,
      created_at: new Date(Date.now() - 3600000).toISOString(),
      agent_count: 2,
      task_count: 5,
    },
    {
      id: 'convoy-def456',
      name: 'Bug Fixes',
      status: 'pending',
      priority: 'normal',
      issues: [{ title: 'Fix login redirect' }],
      progress: 0,
      created_at: new Date(Date.now() - 7200000).toISOString(),
      agent_count: 0,
      task_count: 1,
    },
  ],

  mail: [
    {
      id: 'mail-1',
      from: 'System',
      subject: 'Welcome to Gas Town',
      message: 'Welcome to your new Gas Town installation. Get started by creating a convoy.',
      timestamp: new Date(Date.now() - 86400000).toISOString(),
      read: true,
      priority: 'normal',
    },
    {
      id: 'mail-2',
      from: 'Deacon-1',
      subject: 'Task Complete',
      message: 'The authentication module has been implemented successfully.',
      timestamp: new Date(Date.now() - 3600000).toISOString(),
      read: false,
      priority: 'normal',
    },
  ],

  events: [],
};

// Create Express app
const app = express();
app.use(express.json());

// Serve static files from gui directory
app.use(express.static(path.join(__dirname, '..')));

// API endpoints
app.get('/api/status', (req, res) => {
  res.json(mockData.status);
});

app.get('/api/health', (req, res) => {
  res.json({ status: 'ok', timestamp: new Date().toISOString() });
});

app.get('/api/convoys', (req, res) => {
  res.json(mockData.convoys);
});

app.get('/api/convoy/:id', (req, res) => {
  const convoy = mockData.convoys.find(c => c.id === req.params.id);
  if (convoy) {
    res.json(convoy);
  } else {
    res.status(404).json({ error: 'Convoy not found' });
  }
});

app.post('/api/convoy', (req, res) => {
  const { name, issues, notify } = req.body;
  const newConvoy = {
    id: `convoy-${Date.now()}`,
    name,
    issues: issues?.map(i => ({ title: i })) || [],
    status: 'pending',
    priority: 'normal',
    progress: 0,
    created_at: new Date().toISOString(),
    agent_count: 0,
    task_count: issues?.length || 0,
  };
  mockData.convoys.unshift(newConvoy);
  res.json(newConvoy);

  // Broadcast event via WebSocket
  broadcastEvent({
    type: 'convoy_created',
    data: newConvoy,
  });
});

app.post('/api/sling', (req, res) => {
  const { bead, target, molecule, quality } = req.body;
  const result = {
    id: `sling-${Date.now()}`,
    bead,
    target,
    molecule,
    quality,
    status: 'dispatched',
    timestamp: new Date().toISOString(),
  };
  res.json(result);

  // Broadcast event
  broadcastEvent({
    type: 'work_slung',
    data: result,
  });
});

app.get('/api/hook', (req, res) => {
  res.json(mockData.status.hook || { status: 'none' });
});

app.get('/api/mail', (req, res) => {
  res.json(mockData.mail);
});

app.post('/api/mail', (req, res) => {
  const { to, subject, message, priority } = req.body;
  const newMail = {
    id: `mail-${Date.now()}`,
    from: 'You',
    to,
    subject,
    message,
    priority: priority || 'normal',
    timestamp: new Date().toISOString(),
    read: true,
  };
  res.json({ success: true, mail: newMail });
});

app.get('/api/agents', (req, res) => {
  res.json(mockData.status.agents);
});

app.post('/api/nudge', (req, res) => {
  const { target, message } = req.body;
  res.json({ success: true, target, message });

  // Broadcast event
  broadcastEvent({
    type: 'activity',
    data: {
      type: 'system',
      message: `Nudged agent ${target}: ${message}`,
      timestamp: new Date().toISOString(),
    },
  });
});

// Create HTTP server
const server = createServer(app);

// WebSocket server
const wss = new WebSocketServer({ server, path: '/ws' });

const clients = new Set();

wss.on('connection', (ws) => {
  clients.add(ws);
  console.log('[WS] Client connected');

  // Send initial status
  ws.send(JSON.stringify({
    type: 'status',
    data: mockData.status,
  }));

  ws.on('close', () => {
    clients.delete(ws);
    console.log('[WS] Client disconnected');
  });
});

function broadcastEvent(event) {
  const message = JSON.stringify(event);
  clients.forEach(client => {
    if (client.readyState === 1) {
      client.send(message);
    }
  });

  // Also add to events
  mockData.events.unshift({
    id: `event-${Date.now()}`,
    ...event.data,
    timestamp: new Date().toISOString(),
  });
}

// Simulate periodic activity
let activityInterval;

function startActivitySimulation() {
  const activities = [
    { type: 'activity', message: 'Agent checking work queue' },
    { type: 'activity', message: 'Processing bead update' },
    { type: 'bead_updated', bead_id: 'bead-123', message: 'Status changed to in-progress' },
  ];

  activityInterval = setInterval(() => {
    const activity = activities[Math.floor(Math.random() * activities.length)];
    broadcastEvent({
      type: 'activity',
      data: {
        ...activity,
        id: `evt-${Date.now()}`,
        timestamp: new Date().toISOString(),
      },
    });
  }, 10000); // Every 10 seconds
}

function stopActivitySimulation() {
  if (activityInterval) {
    clearInterval(activityInterval);
  }
}

// Start server
const PORT = process.env.PORT || 4444;

export function startMockServer() {
  return new Promise((resolve) => {
    server.listen(PORT, () => {
      console.log(`[Mock Server] Running on http://localhost:${PORT}`);
      startActivitySimulation();
      resolve(server);
    });
  });
}

export function stopMockServer() {
  return new Promise((resolve) => {
    stopActivitySimulation();
    wss.close();
    server.close(() => {
      console.log('[Mock Server] Stopped');
      resolve();
    });
  });
}

// Run directly if executed as main
if (process.argv[1] === fileURLToPath(import.meta.url)) {
  startMockServer();
}
