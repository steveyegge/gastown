/**
 * Gas Town GUI - Main Application
 *
 * Entry point for the Gas Town web interface.
 * Handles initialization, state management, and component orchestration.
 */

import { api, ws } from './api.js';
import { state, subscribe } from './state.js';
import { renderSidebar } from './components/sidebar.js';
import { renderConvoyList } from './components/convoy-list.js';
import { renderAgentGrid } from './components/agent-grid.js';
import { renderActivityFeed } from './components/activity-feed.js';
import { renderMailList } from './components/mail-list.js';
import { showToast } from './components/toast.js';
import { initModals } from './components/modals.js';

// DOM Elements
const elements = {
  townName: document.getElementById('town-name'),
  connectionStatus: document.getElementById('connection-status'),
  mailBadge: document.getElementById('mail-badge'),
  hookStatus: document.getElementById('hook-status'),
  statusMessage: document.getElementById('status-message'),
  agentTree: document.getElementById('agent-tree'),
  convoyList: document.getElementById('convoy-list'),
  agentGrid: document.getElementById('agent-grid'),
  feedList: document.getElementById('feed-list'),
  mailList: document.getElementById('mail-list'),
};

// Navigation
const navTabs = document.querySelectorAll('.nav-tab');
const views = document.querySelectorAll('.view');

// Initialize application
async function init() {
  console.log('[App] Initializing Gas Town GUI...');

  // Set up navigation
  setupNavigation();

  // Set up modals
  initModals();

  // Set up keyboard shortcuts
  setupKeyboardShortcuts();

  // Set up theme toggle
  setupThemeToggle();

  // Subscribe to state changes FIRST (before loading data)
  subscribeToState();

  // Connect WebSocket
  connectWebSocket();

  // Load initial data
  await loadInitialData();

  console.log('[App] Initialization complete');
}

// Navigation setup
function setupNavigation() {
  navTabs.forEach(tab => {
    tab.addEventListener('click', () => {
      const viewId = tab.dataset.view;
      switchView(viewId);
    });
  });
}

function switchView(viewId) {
  // Update tabs
  navTabs.forEach(tab => {
    tab.classList.toggle('active', tab.dataset.view === viewId);
  });

  // Update views
  views.forEach(view => {
    view.classList.toggle('active', view.id === `view-${viewId}`);
  });

  // Load view-specific data
  if (viewId === 'mail') {
    loadMail();
  } else if (viewId === 'agents') {
    loadAgents();
  }
}

// WebSocket connection
function connectWebSocket() {
  updateConnectionStatus('connecting');

  ws.onopen = () => {
    console.log('[WS] Connected');
    updateConnectionStatus('connected');
    showToast('Connected to Gas Town', 'success');
  };

  ws.onclose = () => {
    console.log('[WS] Disconnected');
    updateConnectionStatus('disconnected');
    showToast('Disconnected from server', 'warning');

    // Attempt reconnect after 5 seconds
    setTimeout(connectWebSocket, 5000);
  };

  ws.onerror = (error) => {
    console.error('[WS] Error:', error);
    updateConnectionStatus('error');
  };

  ws.onmessage = (event) => {
    try {
      const message = JSON.parse(event.data);
      handleWebSocketMessage(message);
    } catch (err) {
      console.error('[WS] Parse error:', err);
    }
  };

  ws.connect();
}

function handleWebSocketMessage(message) {
  switch (message.type) {
    case 'status':
      state.setStatus(message.data);
      break;

    case 'activity':
      state.addEvent(message.data);
      break;

    case 'convoy_created':
    case 'convoy_updated':
      state.updateConvoy(message.data);
      break;

    case 'work_slung':
      showToast(`Work slung: ${message.data?.bead || 'unknown'}`, 'success');
      loadConvoys();
      break;

    default:
      console.log('[WS] Unknown message type:', message.type);
  }
}

function updateConnectionStatus(status) {
  const el = elements.connectionStatus;
  el.className = `connection-status ${status}`;

  const statusText = el.querySelector('.status-text');
  const statusMap = {
    connecting: 'Connecting...',
    connected: 'Connected',
    disconnected: 'Disconnected',
    error: 'Error',
  };
  statusText.textContent = statusMap[status] || status;
}

// Data loading
async function loadInitialData() {
  elements.statusMessage.textContent = 'Loading...';

  try {
    // Load status
    const status = await api.getStatus();
    state.setStatus(status);

    // Load convoys
    await loadConvoys();

    elements.statusMessage.textContent = 'Ready';
  } catch (err) {
    console.error('[App] Failed to load initial data:', err);
    elements.statusMessage.textContent = 'Error loading data';
    showToast('Failed to load data', 'error');
  }
}

async function loadConvoys() {
  try {
    const convoys = await api.getConvoys();
    state.setConvoys(convoys);
  } catch (err) {
    console.error('[App] Failed to load convoys:', err);
  }
}

async function loadMail() {
  try {
    const mail = await api.getMail();
    state.setMail(mail);
  } catch (err) {
    console.error('[App] Failed to load mail:', err);
  }
}

async function loadAgents() {
  try {
    const agents = await api.getAgents();
    state.setAgents(agents);
  } catch (err) {
    console.error('[App] Failed to load agents:', err);
  }
}

// State subscriptions
function subscribeToState() {
  // Status updates
  subscribe('status', (status) => {
    if (status?.name) {
      elements.townName.textContent = status.name;
    }

    // Update hook status
    if (status?.hook) {
      elements.hookStatus.classList.add('active');
      elements.hookStatus.querySelector('.hook-text').textContent = status.hook.bead_id;
    } else {
      elements.hookStatus.classList.remove('active');
      elements.hookStatus.querySelector('.hook-text').textContent = 'No work hooked';
    }

    // Render sidebar
    renderSidebar(elements.agentTree, status);
  });

  // Convoy updates
  subscribe('convoys', (convoys) => {
    renderConvoyList(elements.convoyList, convoys);
  });

  // Agent updates
  subscribe('agents', (agents) => {
    renderAgentGrid(elements.agentGrid, agents);
  });

  // Event updates
  subscribe('events', (events) => {
    renderActivityFeed(elements.feedList, events);
  });

  // Mail updates
  subscribe('mail', (mail) => {
    renderMailList(elements.mailList, mail);

    // Update badge
    const unread = mail.filter(m => !m.read).length;
    elements.mailBadge.textContent = unread;
    elements.mailBadge.classList.toggle('hidden', unread === 0);
  });
}

// Keyboard shortcuts
function setupKeyboardShortcuts() {
  document.addEventListener('keydown', (e) => {
    // Ignore if in input/textarea
    if (e.target.matches('input, textarea, select')) return;

    switch (e.key) {
      case '?':
        showKeyboardHelp();
        break;
      case '1':
        switchView('convoys');
        break;
      case '2':
        switchView('agents');
        break;
      case '3':
        switchView('mail');
        break;
      case 'n':
        if (e.ctrlKey || e.metaKey) {
          e.preventDefault();
          document.getElementById('new-convoy-btn').click();
        }
        break;
      case 'r':
        if (e.ctrlKey || e.metaKey) {
          e.preventDefault();
          loadInitialData();
        }
        break;
      case 'Escape':
        closeAllModals();
        break;
    }
  });
}

function showKeyboardHelp() {
  showToast(`
    Keyboard Shortcuts:
    1 - Convoys | 2 - Agents | 3 - Mail
    Ctrl+N - New Convoy | Ctrl+R - Refresh
    Esc - Close modal
  `, 'info', 5000);
}

function closeAllModals() {
  const overlay = document.getElementById('modal-overlay');
  overlay.classList.add('hidden');
  document.querySelectorAll('.modal').forEach(m => m.classList.add('hidden'));
}

// Theme toggle
function setupThemeToggle() {
  const btn = document.getElementById('theme-toggle');
  const icon = btn.querySelector('.material-icons');

  btn.addEventListener('click', () => {
    const html = document.documentElement;
    const isDark = html.dataset.theme === 'dark';
    html.dataset.theme = isDark ? 'light' : 'dark';
    icon.textContent = isDark ? 'light_mode' : 'dark_mode';
    localStorage.setItem('theme', html.dataset.theme);
  });

  // Load saved theme
  const savedTheme = localStorage.getItem('theme') || 'dark';
  document.documentElement.dataset.theme = savedTheme;
  icon.textContent = savedTheme === 'dark' ? 'dark_mode' : 'light_mode';
}

// Refresh button
document.getElementById('refresh-btn').addEventListener('click', () => {
  loadInitialData();
  showToast('Refreshing...', 'info', 1000);
});

// Initialize on DOM ready
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', init);
} else {
  init();
}

// Export for debugging
window.gastown = { state, api, ws };
