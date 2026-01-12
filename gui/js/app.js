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
import { renderWorkList } from './components/work-list.js';
import { renderMailList } from './components/mail-list.js';
import { renderRigList } from './components/rig-list.js';
import { initPRList, loadPRs } from './components/pr-list.js';
import { initFormulaList, loadFormulas } from './components/formula-list.js';
import { initIssueList, loadIssues } from './components/issue-list.js';
import { initHealthCheck, loadHealthCheck } from './components/health-check.js';
import { initDashboard, loadDashboard } from './components/dashboard.js';
import { showToast } from './components/toast.js';
import { initModals } from './components/modals.js';
import { startTutorial, shouldShowTutorial } from './components/tutorial.js';
import { startOnboarding, shouldShowOnboarding, resetOnboarding } from './components/onboarding.js';

// DOM Elements
const elements = {
  townName: document.getElementById('town-name'),
  connectionStatus: document.getElementById('connection-status'),
  mailBadge: document.getElementById('mail-badge'),
  hookStatus: document.getElementById('hook-status'),
  statusMessage: document.getElementById('status-message'),
  agentTree: document.getElementById('agent-tree'),
  convoyList: document.getElementById('convoy-list'),
  workList: document.getElementById('work-list'),
  agentGrid: document.getElementById('agent-grid'),
  feedList: document.getElementById('feed-list'),
  mailList: document.getElementById('mail-list'),
  rigList: document.getElementById('rig-list'),
};

// Initialization guard to prevent double-init
let isInitialized = false;

// Loading state helpers
function showLoadingState(container, message = 'Loading...') {
  if (!container) return;
  container.innerHTML = `
    <div class="view-loading-state">
      <div class="spinner-large"></div>
      <span class="loading-text">${message}</span>
    </div>
  `;
}

function hideLoadingState(container) {
  // Loading state is automatically replaced when content renders
  // This is a no-op but kept for clarity
}

// Navigation
const navTabs = document.querySelectorAll('.nav-tab');
const views = document.querySelectorAll('.view');

// Initialize application
async function init() {
  // Prevent double initialization
  if (isInitialized) {
    console.log('[App] Already initialized, skipping');
    return;
  }
  isInitialized = true;

  console.log('[App] Initializing Gas Town GUI...');

  // Set up navigation
  setupNavigation();

  // Set up modals
  initModals();

  // Set up PR list
  initPRList();

  // Set up Formula list
  initFormulaList();

  // Set up Issue list
  initIssueList();

  // Set up Health check
  initHealthCheck();

  // Set up Dashboard
  initDashboard();

  // Set up convoy filters
  setupConvoyFilters();

  // Set up work filters
  setupWorkFilters();

  // Set up mail filters
  setupMailFilters();

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

  // Check for first-time users - show onboarding wizard
  const showOnboarding = await shouldShowOnboarding();
  if (showOnboarding) {
    setTimeout(() => startOnboarding(), 500);
  } else if (shouldShowTutorial()) {
    // Show tutorial only if onboarding was already completed
    setTimeout(() => startTutorial(), 1000);
  }

  // Listen for onboarding completion
  document.addEventListener('onboarding:complete', () => {
    loadInitialData();
  });

  // Listen for status refresh (from service controls)
  document.addEventListener('status:refresh', () => {
    loadInitialData();
  });

  // Listen for dashboard refresh
  document.addEventListener('dashboard:refresh', () => {
    loadDashboard();
  });

  // Listen for rigs refresh (from agent controls)
  document.addEventListener('rigs:refresh', () => {
    loadRigs();
  });

  // Listen for work refresh (from work actions)
  document.addEventListener('work:refresh', () => {
    loadWork();
  });

  // Listen for mail refresh (from read/unread actions)
  document.addEventListener('mail:refresh', () => {
    loadMail();
  });

  // Handle mail detail modal
  document.addEventListener('mail:detail', (e) => {
    const { mailId, mail } = e.detail;
    showMailDetailModal(mail);
  });
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
  if (viewId === 'dashboard') {
    loadDashboard();
  } else if (viewId === 'mail') {
    loadMail();
  } else if (viewId === 'agents') {
    loadAgents();
  } else if (viewId === 'work') {
    loadWork();
  } else if (viewId === 'rigs') {
    loadRigs();
  } else if (viewId === 'prs') {
    loadPRs();
  } else if (viewId === 'formulas') {
    loadFormulas();
  } else if (viewId === 'issues') {
    loadIssues();
  } else if (viewId === 'health') {
    loadHealthCheck();
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

    case 'bead_created':
      // Bead was created - refresh work list if visible
      if (state.currentView === 'work') {
        loadWork();
      }
      showToast('Work item created', 'success');
      break;

    case 'rig_added':
      // Rig was added - refresh rigs list and status
      showToast(`Rig added: ${message.data?.name || 'unknown'}`, 'success');
      api.getStatus(true); // Force refresh
      if (state.currentView === 'rigs') {
        loadRigs();
      }
      break;

    case 'mayor_message':
      // Mayor message sent - add to activity feed
      state.addEvent({
        id: message.data.id,
        type: 'mayor_message',
        timestamp: message.data.timestamp,
        target: message.data.target,
        message: message.data.message,
        status: message.data.status,
        response: message.data.response
      });
      break;

    case 'service_started':
      // Service started (possibly Mayor auto-started)
      if (message.data?.autoStarted) {
        showToast(`${message.data.service} auto-started`, 'success');
        state.addEvent({
          id: Date.now().toString(36),
          type: 'mayor_started',
          timestamp: new Date().toISOString(),
          autoStarted: true,
          service: message.data.service
        });
      }
      // Refresh status and update state to re-render sidebar
      api.getStatus().then(status => state.setStatus(status)).catch(console.error);
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
    // Load all critical data in parallel using Promise.allSettled
    // This way a slow/failing request doesn't block others
    const results = await Promise.allSettled([
      api.getStatus().then(status => {
        state.setStatus(status);
        return status;
      }),
      loadConvoys(),
      loadMayorMessageHistory(),
      loadDashboard(),
    ]);

    // Check results and log any failures
    const labels = ['status', 'convoys', 'mayor history', 'dashboard'];
    results.forEach((result, i) => {
      if (result.status === 'rejected') {
        console.error(`[App] Failed to load ${labels[i]}:`, result.reason);
      }
    });

    // If status failed, show warning
    if (results[0].status === 'rejected') {
      elements.statusMessage.textContent = 'Ready (status unavailable)';
      showToast('Some data failed to load', 'warning');
    } else {
      elements.statusMessage.textContent = 'Ready';
    }

    // Background preload of other data (don't await, let it load in background)
    preloadBackgroundData();
  } catch (err) {
    console.error('[App] Failed to load initial data:', err);
    elements.statusMessage.textContent = 'Error loading data';
    showToast('Failed to load data', 'error');
  }
}

// Preload data in background for faster modal/tab access
async function preloadBackgroundData() {
  try {
    // Wait 500ms to let initial UI settle, then preload in background
    await new Promise(resolve => setTimeout(resolve, 500));

    console.log('[App] Preloading background data...');

    // Preload these in parallel
    await Promise.allSettled([
      api.getAgents(),  // Preload agents list
      api.getRigs(),    // Preload rigs list
      loadPRs(),        // Preload PRs
      loadFormulas(),   // Preload formulas
      loadIssues(),     // Preload issues
    ]);

    console.log('[App] Background data preloaded');
  } catch (err) {
    console.error('[App] Failed to preload background data:', err);
    // Don't show error to user - this is background loading
  }
}

// Track convoy filter state
let showAllConvoys = false;

async function loadConvoys() {
  showLoadingState(elements.convoyList, 'Loading convoys...');
  try {
    const params = showAllConvoys ? { all: 'true' } : {};
    const convoys = await api.getConvoys(params);
    state.setConvoys(convoys);
  } catch (err) {
    console.error('[App] Failed to load convoys:', err);
    elements.convoyList.innerHTML = `
      <div class="empty-state">
        <span class="material-icons">error_outline</span>
        <p>Failed to load convoys</p>
      </div>
    `;
  }
}

// Load Mayor message history and add to activity feed
async function loadMayorMessageHistory() {
  try {
    const messages = await api.getMayorMessages(20);
    if (messages && messages.length > 0) {
      // Add messages to activity feed (oldest first so newest appear at top)
      for (const msg of messages.reverse()) {
        state.addEvent({
          id: msg.id,
          type: 'mayor_message',
          timestamp: msg.timestamp,
          target: msg.target,
          message: msg.message,
          status: msg.status,
          response: msg.response
        });
      }
      console.log(`[App] Loaded ${messages.length} Mayor messages into activity feed`);
    }
  } catch (err) {
    console.log('[App] No Mayor message history (may be first run):', err.message);
  }
}

// Setup convoy filter toggle
function setupConvoyFilters() {
  const activeBtn = document.getElementById('convoy-filter-active');
  const allBtn = document.getElementById('convoy-filter-all');
  const title = document.getElementById('convoy-view-title');

  if (activeBtn && allBtn) {
    activeBtn.addEventListener('click', () => {
      showAllConvoys = false;
      activeBtn.classList.remove('btn-ghost');
      activeBtn.classList.add('btn-secondary', 'filter-active');
      allBtn.classList.remove('btn-secondary', 'filter-active');
      allBtn.classList.add('btn-ghost');
      if (title) title.textContent = 'Active Convoys';
      loadConvoys();
    });

    allBtn.addEventListener('click', () => {
      showAllConvoys = true;
      allBtn.classList.remove('btn-ghost');
      allBtn.classList.add('btn-secondary', 'filter-active');
      activeBtn.classList.remove('btn-secondary', 'filter-active');
      activeBtn.classList.add('btn-ghost');
      if (title) title.textContent = 'All Convoys';
      loadConvoys();
    });
  }
}

// Track mail filter state
let mailFilter = 'mine'; // 'mine' = my inbox, 'all' = all system mail

async function loadMail() {
  showLoadingState(elements.mailList, 'Loading mail...');
  try {
    let mail;
    if (mailFilter === 'all') {
      // Get all mail from feed (paginated response)
      const response = await api.get('/api/mail/all');
      mail = response.items || response; // Handle both paginated and legacy responses
    } else {
      // Get my inbox only
      mail = await api.getMail();
    }
    state.setMail(mail || []);
  } catch (err) {
    console.error('[App] Failed to load mail:', err);
    elements.mailList.innerHTML = `
      <div class="empty-state">
        <span class="material-icons">error_outline</span>
        <p>Failed to load mail</p>
      </div>
    `;
  }
}

// Setup mail filter toggle
function setupMailFilters() {
  const mineBtn = document.getElementById('mail-filter-mine');
  const allBtn = document.getElementById('mail-filter-all');
  const title = document.getElementById('mail-view-title');

  if (mineBtn && allBtn) {
    mineBtn.addEventListener('click', () => {
      mailFilter = 'mine';
      mineBtn.classList.remove('btn-ghost');
      mineBtn.classList.add('btn-secondary', 'filter-active');
      allBtn.classList.remove('btn-secondary', 'filter-active');
      allBtn.classList.add('btn-ghost');
      if (title) title.textContent = 'My Inbox';
      loadMail();
    });

    allBtn.addEventListener('click', () => {
      mailFilter = 'all';
      allBtn.classList.remove('btn-ghost');
      allBtn.classList.add('btn-secondary', 'filter-active');
      mineBtn.classList.remove('btn-secondary', 'filter-active');
      mineBtn.classList.add('btn-ghost');
      if (title) title.textContent = 'All System Mail';
      loadMail();
    });
  }
}

async function loadAgents() {
  // Show loading state only if we don't have cached data
  const hasCache = state.getAgents().length > 0;
  if (!hasCache) {
    showLoadingState(elements.agentGrid, 'Loading agents...');
  }

  try {
    const response = await api.getAgents();
    // Combine agents and polecats into a flat list
    const allAgents = [
      ...(response.agents || []),
      ...(response.polecats || []).map(p => ({
        ...p,
        id: p.name,
        status: p.running ? 'working' : 'idle',
      })),
    ];
    state.setAgents(allAgents);
  } catch (err) {
    console.error('[App] Failed to load agents:', err);
    // Only show error if we don't have cached data
    if (!hasCache) {
      elements.agentGrid.innerHTML = `
        <div class="empty-state">
          <span class="material-icons">error_outline</span>
          <p>Failed to load agents</p>
        </div>
      `;
    }
  }
}

async function loadRigs() {
  // Show loading state only if we don't have cached data
  const hasCache = state.getRigs().length > 0;
  if (!hasCache) {
    showLoadingState(elements.rigList, 'Loading rigs...');
  } else {
    // Show cached data immediately
    renderRigList(elements.rigList, state.getRigs());
  }

  try {
    // Get rigs from status (has more details than /api/rigs)
    const status = await api.getStatus();
    const rigs = status.rigs || [];
    state.setStatus(status); // Update state
    renderRigList(elements.rigList, rigs);
  } catch (err) {
    console.error('[App] Failed to load rigs:', err);
    // Only show error if we don't have cached data
    if (!hasCache) {
      elements.rigList.innerHTML = `
        <div class="empty-state">
          <span class="material-icons">error_outline</span>
          <p>Failed to load rigs</p>
        </div>
      `;
    }
  }
}

// Track work filter state
let workFilter = 'closed'; // Default to showing completed work

async function loadWork() {
  showLoadingState(elements.workList, 'Loading work...');
  try {
    const params = workFilter === 'all' ? {} : { status: workFilter };
    const beads = await api.get(`/api/beads${workFilter !== 'all' ? `?status=${workFilter}` : ''}`);
    renderWorkList(elements.workList, beads || []);
  } catch (err) {
    console.error('[App] Failed to load work:', err);
    elements.workList.innerHTML = `
      <div class="empty-state">
        <span class="material-icons">error_outline</span>
        <p>Failed to load work</p>
      </div>
    `;
  }
}

// Setup work filter toggle
function setupWorkFilters() {
  const allBtn = document.getElementById('work-filter-all');
  const openBtn = document.getElementById('work-filter-open');
  const closedBtn = document.getElementById('work-filter-closed');
  const title = document.getElementById('work-view-title');

  const buttons = [allBtn, openBtn, closedBtn];

  function setActiveFilter(activeBtn, filter, titleText) {
    workFilter = filter;
    buttons.forEach(btn => {
      if (btn) {
        btn.classList.remove('btn-secondary', 'filter-active');
        btn.classList.add('btn-ghost');
      }
    });
    if (activeBtn) {
      activeBtn.classList.remove('btn-ghost');
      activeBtn.classList.add('btn-secondary', 'filter-active');
    }
    if (title) title.textContent = titleText;
    loadWork();
  }

  if (allBtn) {
    allBtn.addEventListener('click', () => setActiveFilter(allBtn, 'all', 'All Work'));
  }
  if (openBtn) {
    openBtn.addEventListener('click', () => setActiveFilter(openBtn, 'open', 'Open Tasks'));
  }
  if (closedBtn) {
    closedBtn.addEventListener('click', () => setActiveFilter(closedBtn, 'closed', 'Completed Work'));
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
    renderMailList(elements.mailList, mail, { isAllMail: mailFilter === 'all' });

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

    // Simple key shortcuts (no modifier)
    if (!e.ctrlKey && !e.metaKey && !e.altKey) {
      switch (e.key) {
        case '?':
          e.preventDefault();
          showKeyboardHelp();
          return;
        case '1':
        case 'd':
          switchView('dashboard');
          return;
        case '2':
        case 'c':
          switchView('convoys');
          return;
        case '3':
        case 'a':
          switchView('agents');
          return;
        case '4':
        case 'm':
          switchView('mail');
          return;
        case '5':
        case 'w':
          switchView('work');
          return;
        case '6':
          switchView('rigs');
          return;
        case '7':
          switchView('prs');
          return;
        case '8':
          switchView('formulas');
          return;
        case '9':
          switchView('issues');
          return;
        case '0':
        case 'h':
          switchView('health');
          return;
        case '/':
          e.preventDefault();
          // Focus search if available
          const searchInput = document.querySelector('.search-input');
          if (searchInput) searchInput.focus();
          return;
        case 'Escape':
          closeAllModals();
          return;
      }
    }

    // Ctrl/Cmd shortcuts
    if (e.ctrlKey || e.metaKey) {
      switch (e.key) {
        case 'n':
          e.preventDefault();
          document.getElementById('new-convoy-btn')?.click();
          break;
        case 'r':
          e.preventDefault();
          loadInitialData();
          showToast('Refreshing...', 'info', 1000);
          break;
        case 's':
          e.preventDefault();
          // Trigger sling modal
          document.getElementById('sling-btn')?.click();
          break;
        case 'k':
          e.preventDefault();
          // Quick command palette (future)
          showKeyboardHelp();
          break;
      }
    }

    // Alt shortcuts for quick actions
    if (e.altKey && !e.ctrlKey && !e.metaKey) {
      switch (e.key) {
        case 'n':
          e.preventDefault();
          // New bead
          document.querySelector('[data-modal-open="new-bead"]')?.click();
          break;
        case 'c':
          e.preventDefault();
          // New convoy
          document.getElementById('new-convoy-btn')?.click();
          break;
        case 'm':
          e.preventDefault();
          // Compose mail
          document.querySelector('[data-modal-open="mail-compose"]')?.click();
          break;
      }
    }
  });
}

function showKeyboardHelp() {
  // Try to open help modal
  const helpBtn = document.getElementById('help-btn');
  if (helpBtn) {
    helpBtn.click();
  } else {
    // Create a temporary keyboard help overlay
    const overlay = document.createElement('div');
    overlay.className = 'keyboard-help-overlay';
    overlay.innerHTML = `
      <div class="keyboard-help-modal">
        <div class="keyboard-help-header">
          <h2><span class="material-icons">keyboard</span> Keyboard Shortcuts</h2>
          <button class="btn btn-icon" onclick="this.closest('.keyboard-help-overlay').remove()">
            <span class="material-icons">close</span>
          </button>
        </div>
        <div class="keyboard-help-content">
          <div class="shortcut-group">
            <h3>Navigation</h3>
            <div class="shortcut-row"><kbd>1</kbd> or <kbd>D</kbd> <span>Dashboard</span></div>
            <div class="shortcut-row"><kbd>2</kbd> or <kbd>C</kbd> <span>Convoys</span></div>
            <div class="shortcut-row"><kbd>3</kbd> or <kbd>A</kbd> <span>Agents</span></div>
            <div class="shortcut-row"><kbd>4</kbd> or <kbd>M</kbd> <span>Mail</span></div>
            <div class="shortcut-row"><kbd>5</kbd> or <kbd>W</kbd> <span>Work</span></div>
            <div class="shortcut-row"><kbd>6</kbd> <span>Rigs</span></div>
            <div class="shortcut-row"><kbd>7</kbd> <span>Pull Requests</span></div>
            <div class="shortcut-row"><kbd>8</kbd> <span>Formulas</span></div>
            <div class="shortcut-row"><kbd>9</kbd> <span>Issues</span></div>
            <div class="shortcut-row"><kbd>0</kbd> or <kbd>H</kbd> <span>Health</span></div>
          </div>
          <div class="shortcut-group">
            <h3>Actions</h3>
            <div class="shortcut-row"><kbd>Ctrl</kbd>+<kbd>N</kbd> <span>New Convoy</span></div>
            <div class="shortcut-row"><kbd>Ctrl</kbd>+<kbd>R</kbd> <span>Refresh Data</span></div>
            <div class="shortcut-row"><kbd>Ctrl</kbd>+<kbd>S</kbd> <span>Sling Work</span></div>
            <div class="shortcut-row"><kbd>Alt</kbd>+<kbd>N</kbd> <span>New Bead</span></div>
            <div class="shortcut-row"><kbd>Alt</kbd>+<kbd>M</kbd> <span>Compose Mail</span></div>
            <div class="shortcut-row"><kbd>/</kbd> <span>Focus Search</span></div>
            <div class="shortcut-row"><kbd>Esc</kbd> <span>Close Modal</span></div>
            <div class="shortcut-row"><kbd>?</kbd> <span>Show This Help</span></div>
          </div>
        </div>
        <div class="keyboard-help-footer">
          Press <kbd>Esc</kbd> or click outside to close
        </div>
      </div>
    `;
    overlay.addEventListener('click', (e) => {
      if (e.target === overlay) overlay.remove();
    });
    document.body.appendChild(overlay);
  }
}

function closeAllModals() {
  const overlay = document.getElementById('modal-overlay');
  overlay.classList.add('hidden');
  document.querySelectorAll('.modal').forEach(m => m.classList.add('hidden'));
  // Also close keyboard help overlay
  document.querySelector('.keyboard-help-overlay')?.remove();
}

// Show mail detail modal
function showMailDetailModal(mail) {
  if (!mail) return;

  const modal = document.getElementById('mail-detail-modal');
  if (!modal) return;

  const subjectEl = modal.querySelector('#mail-detail-subject');
  const fromEl = modal.querySelector('#mail-detail-from');
  const toEl = modal.querySelector('#mail-detail-to');
  const timeEl = modal.querySelector('#mail-detail-time');
  const bodyEl = modal.querySelector('#mail-detail-body');

  if (subjectEl) subjectEl.textContent = mail.subject || '(No Subject)';
  if (fromEl) fromEl.textContent = mail.from || 'Unknown';
  if (toEl) toEl.textContent = mail.to || 'Unknown';
  if (timeEl) timeEl.textContent = new Date(mail.timestamp).toLocaleString();
  if (bodyEl) bodyEl.textContent = mail.message || mail.body || '';

  // Mark as read when viewing
  if (!mail.read && !mail.feedEvent) {
    api.markMailRead(mail.id).catch(err => console.warn('Failed to mark mail as read:', err));
  }

  document.getElementById('modal-overlay').classList.remove('hidden');
  modal.classList.remove('hidden');
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

// Mayor command bar
const mayorInput = document.getElementById('mayor-command-input');
const mayorSendBtn = document.getElementById('mayor-command-send');

async function sendToMayor() {
  const message = mayorInput.value.trim();
  if (!message) return;

  mayorSendBtn.disabled = true;
  mayorSendBtn.innerHTML = '<span class="material-icons spinning">sync</span>';

  try {
    const result = await api.nudge('mayor', message);
    if (result.success) {
      const truncatedMsg = message.substring(0, 40) + (message.length > 40 ? '...' : '');
      if (result.wasAutoStarted) {
        showToast('Mayor auto-started. Sent: ' + truncatedMsg, 'success');
      } else {
        showToast('Sent to Mayor: ' + truncatedMsg, 'success');
      }
      mayorInput.value = '';
      // Auto-open Mayor output panel so user can see what's happening
      showMayorOutput();
    } else {
      showToast('Failed: ' + (result.error || 'Unknown error'), 'error');
    }
  } catch (err) {
    showToast('Error: ' + err.message, 'error');
  } finally {
    mayorSendBtn.disabled = false;
    mayorSendBtn.innerHTML = '<span class="material-icons">send</span>';
  }
}

mayorSendBtn.addEventListener('click', sendToMayor);
mayorInput.addEventListener('keydown', (e) => {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault();
    sendToMayor();
  }
});

// Mayor output panel
const mayorViewBtn = document.getElementById('mayor-view-btn');
const mayorOutputPanel = document.getElementById('mayor-output-panel');
const mayorOutputContent = document.getElementById('mayor-output-content');
const mayorOutputClose = document.getElementById('mayor-output-close');
let mayorOutputRefreshInterval = null;

async function refreshMayorOutput() {
  try {
    const data = await api.getMayorOutput(80);
    if (data.output) {
      // Format output with some highlighting
      let output = data.output;
      // Highlight key phrases
      output = output.replace(/(Done\.|Success|Created|Complete)/gi, '<span style="color: #22c55e">$1</span>');
      output = output.replace(/(Error|Failed|Cannot)/gi, '<span style="color: #ef4444">$1</span>');
      output = output.replace(/(Thinking…|Working|Processing)/gi, '<span style="color: #f59e0b">$1</span>');
      output = output.replace(/(https?:\/\/[^\s]+)/g, '<a href="$1" target="_blank" style="color: #3b82f6">$1</a>');
      mayorOutputContent.innerHTML = `<pre>${output}</pre>`;
      // Scroll to bottom
      mayorOutputContent.scrollTop = mayorOutputContent.scrollHeight;
    } else {
      mayorOutputContent.innerHTML = `<pre style="color: var(--text-tertiary)">${data.running ? 'Mayor is running but no output yet...' : 'Mayor is not running. Send a message to auto-start.'}</pre>`;
    }
  } catch (err) {
    mayorOutputContent.innerHTML = `<pre style="color: #ef4444">Error loading output: ${err.message}</pre>`;
  }
}

function showMayorOutput() {
  mayorOutputPanel.style.display = 'block';
  refreshMayorOutput();
  // Auto-refresh every 2 seconds while open
  mayorOutputRefreshInterval = setInterval(refreshMayorOutput, 2000);
}

function hideMayorOutput() {
  mayorOutputPanel.style.display = 'none';
  if (mayorOutputRefreshInterval) {
    clearInterval(mayorOutputRefreshInterval);
    mayorOutputRefreshInterval = null;
  }
}

mayorViewBtn.addEventListener('click', () => {
  if (mayorOutputPanel.style.display === 'none') {
    showMayorOutput();
  } else {
    hideMayorOutput();
  }
});

mayorOutputClose.addEventListener('click', hideMayorOutput);

// Make panel draggable by header
const mayorOutputHeader = document.querySelector('.mayor-output-header');
let isDragging = false;
let dragOffsetX = 0;
let dragOffsetY = 0;

mayorOutputHeader.addEventListener('mousedown', (e) => {
  if (e.target.closest('.mayor-output-close')) return; // Don't drag when clicking close
  isDragging = true;
  const rect = mayorOutputPanel.getBoundingClientRect();
  dragOffsetX = e.clientX - rect.left;
  dragOffsetY = e.clientY - rect.top;
  mayorOutputPanel.style.transform = 'none'; // Remove centering transform
  mayorOutputPanel.style.left = rect.left + 'px';
  mayorOutputPanel.style.top = rect.top + 'px';
});

document.addEventListener('mousemove', (e) => {
  if (!isDragging) return;
  const x = Math.max(0, Math.min(window.innerWidth - 100, e.clientX - dragOffsetX));
  const y = Math.max(0, Math.min(window.innerHeight - 50, e.clientY - dragOffsetY));
  mayorOutputPanel.style.left = x + 'px';
  mayorOutputPanel.style.top = y + 'px';
});

document.addEventListener('mouseup', () => {
  isDragging = false;
});

// Initialize on DOM ready
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', init);
} else {
  init();
}

// Export for debugging
window.gastown = { state, api, ws, startTutorial, startOnboarding, resetOnboarding };
