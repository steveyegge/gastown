/**
 * Gas Town GUI - API Client
 *
 * Handles communication with the Node bridge server.
 * - REST API for commands
 * - WebSocket for real-time updates
 */

const API_BASE = window.location.origin;
const WS_URL = `ws://${window.location.host}/ws`;

// REST API Client
export const api = {
  // Generic fetch wrapper
  async request(endpoint, options = {}) {
    const url = `${API_BASE}${endpoint}`;
    const config = {
      headers: {
        'Content-Type': 'application/json',
        ...options.headers,
      },
      ...options,
    };

    if (options.body && typeof options.body === 'object') {
      config.body = JSON.stringify(options.body);
    }

    const response = await fetch(url, config);

    if (!response.ok) {
      const error = await response.json().catch(() => ({ error: response.statusText }));
      throw new Error(error.error || 'Request failed');
    }

    return response.json();
  },

  // GET request
  get(endpoint) {
    return this.request(endpoint);
  },

  // POST request
  post(endpoint, body) {
    return this.request(endpoint, { method: 'POST', body });
  },

  // === Status ===
  getStatus() {
    return this.get('/api/status');
  },

  getHealth() {
    return this.get('/api/health');
  },

  // === Convoys ===
  getConvoys(params = {}) {
    const query = new URLSearchParams(params).toString();
    return this.get(`/api/convoys${query ? '?' + query : ''}`);
  },

  getConvoy(id) {
    return this.get(`/api/convoy/${id}`);
  },

  createConvoy(name, issues = [], notify = null) {
    return this.post('/api/convoy', { name, issues, notify });
  },

  // === Work ===
  sling(bead, target, options = {}) {
    return this.post('/api/sling', {
      bead,
      target,
      molecule: options.molecule,
      quality: options.quality,
      args: options.args,
    });
  },

  getHook() {
    return this.get('/api/hook');
  },

  // === Mail ===
  getMail() {
    return this.get('/api/mail');
  },

  sendMail(to, subject, message, priority = 'normal') {
    return this.post('/api/mail', { to, subject, message, priority });
  },

  // === Agents ===
  getAgents() {
    return this.get('/api/agents');
  },

  nudge(target, message) {
    return this.post('/api/nudge', { target, message });
  },

  // === Beads ===
  createBead(title, options = {}) {
    return this.post('/api/beads', {
      title,
      description: options.description,
      priority: options.priority,
      labels: options.labels,
    });
  },

  searchBeads(query) {
    return this.get(`/api/beads/search?q=${encodeURIComponent(query)}`);
  },

  searchFormulas(query) {
    return this.get(`/api/formulas/search?q=${encodeURIComponent(query)}`);
  },

  getTargets() {
    return this.get('/api/targets');
  },

  // === Escalation ===
  escalate(convoyId, reason, priority = 'normal') {
    return this.post('/api/escalate', { convoy_id: convoyId, reason, priority });
  },

  // === Setup & Onboarding ===
  getSetupStatus() {
    return this.get('/api/setup/status');
  },

  getRigs() {
    return this.get('/api/rigs');
  },

  addRig(name, url) {
    return this.post('/api/rigs', { name, url });
  },

  runDoctor() {
    return this.get('/api/doctor');
  },

  // === Polecat Output ===
  getPeekOutput(rig, name) {
    return this.get(`/api/polecat/${encodeURIComponent(rig)}/${encodeURIComponent(name)}/output`);
  },
};

// WebSocket Client
class WebSocketClient {
  constructor(url) {
    this.url = url;
    this.socket = null;
    this.reconnectAttempts = 0;
    this.maxReconnectAttempts = 10;
    this.reconnectDelay = 1000;
    this.listeners = {
      open: [],
      close: [],
      error: [],
      message: [],
    };
  }

  connect() {
    if (this.socket && this.socket.readyState === WebSocket.OPEN) {
      return;
    }

    try {
      this.socket = new WebSocket(this.url);

      this.socket.onopen = (event) => {
        this.reconnectAttempts = 0;
        this.listeners.open.forEach(cb => cb(event));
      };

      this.socket.onclose = (event) => {
        this.listeners.close.forEach(cb => cb(event));
        this.attemptReconnect();
      };

      this.socket.onerror = (event) => {
        this.listeners.error.forEach(cb => cb(event));
      };

      this.socket.onmessage = (event) => {
        this.listeners.message.forEach(cb => cb(event));
      };
    } catch (err) {
      console.error('[WS] Connection error:', err);
      this.attemptReconnect();
    }
  }

  attemptReconnect() {
    if (this.reconnectAttempts >= this.maxReconnectAttempts) {
      console.error('[WS] Max reconnect attempts reached');
      return;
    }

    this.reconnectAttempts++;
    const delay = this.reconnectDelay * Math.pow(2, this.reconnectAttempts - 1);
    console.log(`[WS] Reconnecting in ${delay}ms (attempt ${this.reconnectAttempts})`);

    setTimeout(() => this.connect(), delay);
  }

  send(data) {
    if (this.socket && this.socket.readyState === WebSocket.OPEN) {
      this.socket.send(JSON.stringify(data));
    } else {
      console.warn('[WS] Cannot send - not connected');
    }
  }

  set onopen(callback) {
    this.listeners.open.push(callback);
  }

  set onclose(callback) {
    this.listeners.close.push(callback);
  }

  set onerror(callback) {
    this.listeners.error.push(callback);
  }

  set onmessage(callback) {
    this.listeners.message.push(callback);
  }

  close() {
    if (this.socket) {
      this.socket.close();
    }
  }
}

export const ws = new WebSocketClient(WS_URL);
