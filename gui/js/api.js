/**
 * Gas Town GUI - API Client
 *
 * Handles communication with the Node bridge server.
 * - REST API for commands
 * - WebSocket for real-time updates
 */

const API_BASE = window.location.origin;
const WS_PROTOCOL = window.location.protocol === 'https:' ? 'wss' : 'ws';
const WS_URL = `${WS_PROTOCOL}://${window.location.host}/ws`;

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
      const errorData = await response.json().catch(() => ({ error: response.statusText }));
      // Create error with message but also attach the full error data
      const error = new Error(errorData.error || 'Request failed');
      // Attach structured error data for better error handling
      if (errorData.errorType) {
        error.errorType = errorData.errorType;
        error.errorData = errorData;
      }
      throw error;
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

  getMailMessage(id) {
    return this.get(`/api/mail/${encodeURIComponent(id)}`);
  },

  sendMail(to, subject, message, priority = 'normal') {
    return this.post('/api/mail', { to, subject, message, priority });
  },

  markMailRead(id) {
    return this.post(`/api/mail/${encodeURIComponent(id)}/read`);
  },

  markMailUnread(id) {
    return this.post(`/api/mail/${encodeURIComponent(id)}/unread`);
  },

  // === Agents ===
  getAgents() {
    return this.get('/api/agents');
  },

  nudge(target, message, autoStart = true) {
    return this.post('/api/nudge', { target, message, autoStart });
  },

  getMayorMessages(limit = 50) {
    return this.get(`/api/mayor/messages?limit=${limit}`);
  },

  getMayorOutput(lines = 100) {
    return this.get(`/api/mayor/output?lines=${lines}`);
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

  getBead(beadId) {
    return this.get(`/api/bead/${encodeURIComponent(beadId)}`);
  },

  getBeadLinks(beadId) {
    return this.get(`/api/bead/${encodeURIComponent(beadId)}/links`);
  },

  // === Work Actions ===
  markWorkDone(beadId, summary) {
    return this.post(`/api/work/${encodeURIComponent(beadId)}/done`, { summary });
  },

  parkWork(beadId, reason) {
    return this.post(`/api/work/${encodeURIComponent(beadId)}/park`, { reason });
  },

  releaseWork(beadId) {
    return this.post(`/api/work/${encodeURIComponent(beadId)}/release`);
  },

  reassignWork(beadId, target) {
    return this.post(`/api/work/${encodeURIComponent(beadId)}/reassign`, { target });
  },

  searchBeads(query) {
    return this.get(`/api/beads/search?q=${encodeURIComponent(query)}`);
  },

  searchFormulas(query) {
    return this.get(`/api/formulas/search?q=${encodeURIComponent(query)}`);
  },

  getFormulas() {
    return this.get('/api/formulas');
  },

  getFormula(name) {
    return this.get(`/api/formula/${encodeURIComponent(name)}`);
  },

  createFormula(name, description, template) {
    return this.post('/api/formulas', { name, description, template });
  },

  useFormula(name, target, args) {
    return this.post(`/api/formula/${encodeURIComponent(name)}/use`, { target, args });
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

  removeRig(name) {
    return this.request(`/api/rigs/${encodeURIComponent(name)}`, { method: 'DELETE' });
  },

  runDoctor(options = {}) {
    const params = options.refresh ? '?refresh=true' : '';
    return this.get(`/api/doctor${params}`);
  },

  runDoctorFix() {
    return this.post('/api/doctor/fix');
  },

  // === Polecat Output ===
  getPeekOutput(rig, name) {
    return this.get(`/api/polecat/${encodeURIComponent(rig)}/${encodeURIComponent(name)}/output`);
  },

  getAgentTranscript(rig, name) {
    return this.get(`/api/polecat/${encodeURIComponent(rig)}/${encodeURIComponent(name)}/transcript`);
  },

  // === Agent Controls ===
  startAgent(rig, name) {
    return this.post(`/api/polecat/${encodeURIComponent(rig)}/${encodeURIComponent(name)}/start`);
  },

  stopAgent(rig, name) {
    return this.post(`/api/polecat/${encodeURIComponent(rig)}/${encodeURIComponent(name)}/stop`);
  },

  restartAgent(rig, name) {
    return this.post(`/api/polecat/${encodeURIComponent(rig)}/${encodeURIComponent(name)}/restart`);
  },

  // === Service Controls ===
  startService(name) {
    return this.post(`/api/service/${encodeURIComponent(name)}/up`);
  },

  stopService(name) {
    return this.post(`/api/service/${encodeURIComponent(name)}/down`);
  },

  restartService(name) {
    return this.post(`/api/service/${encodeURIComponent(name)}/restart`);
  },

  getServiceStatus(name) {
    return this.get(`/api/service/${encodeURIComponent(name)}/status`);
  },

  // === GitHub Integration ===
  getGitHubPRs(state = 'open') {
    return this.get(`/api/github/prs?state=${encodeURIComponent(state)}`);
  },

  getGitHubPR(repo, number) {
    return this.get(`/api/github/pr/${encodeURIComponent(repo)}/${number}`);
  },

  // === GitHub Issues ===
  getGitHubIssues(state = 'open') {
    return this.get(`/api/github/issues?state=${encodeURIComponent(state)}`);
  },

  getGitHubIssue(repo, number) {
    return this.get(`/api/github/issue/${encodeURIComponent(repo)}/${number}`);
  },

  // === GitHub Repos ===
  getGitHubRepos(options = {}) {
    const params = new URLSearchParams();
    if (options.limit) params.set('limit', options.limit);
    if (options.visibility) params.set('visibility', options.visibility);
    if (options.refresh) params.set('refresh', 'true');
    const query = params.toString();
    return this.get(`/api/github/repos${query ? '?' + query : ''}`);
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
