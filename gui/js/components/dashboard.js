/**
 * Gas Town GUI - Dashboard Component
 *
 * System overview with metrics, health status, quick actions, and activity.
 * Designed with expert color theory and UI/UX principles.
 */

import { api } from '../api.js';
import { state } from '../state.js';
import { showToast } from './toast.js';
import { AGENT_TYPES, STATUS_COLORS, getAgentConfig } from '../shared/agent-types.js';

/**
 * Calculate agent work status from an array of agents
 * @returns {{ working: number, available: number, stopped: number, total: number, statusText: string }}
 */
function getAgentStats(agents) {
  const working = agents.filter(a => a.running && (a.has_work || a.hook)).length;
  const available = agents.filter(a => a.running && !a.has_work && !a.hook).length;
  const stopped = agents.filter(a => !a.running).length;
  const total = agents.length;

  let statusText;
  if (total === 0) {
    statusText = 'none';
  } else if (working > 0 && available > 0) {
    statusText = `${working} working, ${available} idle`;
  } else if (working > 0) {
    statusText = `${working} working`;
  } else if (available > 0) {
    statusText = `${available} available`;
  } else {
    statusText = `${stopped} stopped`;
  }

  return { working, available, stopped, total, statusText };
}

/**
 * Derive basic health status from status data (fast, no doctor call)
 * Returns a health object with checks array for calculateHealthStatus()
 */
function deriveHealthFromStatus(status) {
  const checks = [];
  const rigs = status.rigs || [];
  const agents = status.agents || [];

  // Check: Are there any rigs configured?
  if (rigs.length === 0) {
    checks.push({ name: 'Rigs', status: 'warn', message: 'No rigs configured' });
  } else {
    checks.push({ name: 'Rigs', status: 'pass', message: `${rigs.length} rig(s) configured` });
  }

  // Check: Are core agents (mayor, deacon) running?
  const mayor = agents.find(a => a.name === 'mayor' || a.role === 'coordinator');
  const deacon = agents.find(a => a.name === 'deacon' || a.role === 'health-check');

  if (mayor && mayor.running) {
    checks.push({ name: 'Mayor', status: 'pass', message: 'Running' });
  } else if (mayor) {
    checks.push({ name: 'Mayor', status: 'warn', message: 'Not running' });
  }

  if (deacon && deacon.running) {
    checks.push({ name: 'Deacon', status: 'pass', message: 'Running' });
  } else if (deacon) {
    checks.push({ name: 'Deacon', status: 'warn', message: 'Not running' });
  }

  // Check: Any polecats running in rigs?
  let runningPolecats = 0;
  let totalPolecats = 0;
  rigs.forEach(rig => {
    if (rig.agents) {
      rig.agents.forEach(agent => {
        totalPolecats++;
        if (agent.running) runningPolecats++;
      });
    }
  });

  if (totalPolecats > 0) {
    if (runningPolecats > 0) {
      checks.push({ name: 'Polecats', status: 'pass', message: `${runningPolecats}/${totalPolecats} running` });
    } else {
      checks.push({ name: 'Polecats', status: 'warn', message: 'None running' });
    }
  }

  return { checks };
}

let container = null;
let refreshBtn = null;

/**
 * Initialize dashboard component
 */
export function initDashboard() {
  container = document.getElementById('dashboard-container');
  refreshBtn = document.getElementById('dashboard-refresh');

  if (refreshBtn) {
    refreshBtn.addEventListener('click', loadDashboard);
  }

  // Listen for status updates
  document.addEventListener('status:updated', loadDashboard);
}

/**
 * Load and render dashboard
 */
export async function loadDashboard() {
  if (!container) return;

  // Show loading skeleton
  container.innerHTML = renderLoadingSkeleton();

  try {
    // Only load status - doctor is too slow for dashboard (15-20s)
    // User can click through to Health page for full diagnostics
    const statusResult = await api.getStatus().catch(() => null);

    const status = statusResult || state.get('status') || {};
    // Derive basic health from status data (fast, no doctor call)
    const health = deriveHealthFromStatus(status);

    renderDashboard(status, health);
  } catch (err) {
    showToast(`Failed to load dashboard: ${err.message}`, 'error');
    container.innerHTML = renderErrorState(err.message);
  }
}

/**
 * Render loading skeleton
 */
function renderLoadingSkeleton() {
  return `
    <div class="dashboard-loading">
      <div class="skeleton-grid">
        <div class="skeleton-card large"></div>
        <div class="skeleton-card"></div>
        <div class="skeleton-card"></div>
        <div class="skeleton-card"></div>
        <div class="skeleton-card wide"></div>
        <div class="skeleton-card wide"></div>
      </div>
    </div>
  `;
}

/**
 * Render error state
 */
function renderErrorState(message) {
  return `
    <div class="dashboard-error">
      <span class="material-icons">cloud_off</span>
      <h3>Unable to Load Dashboard</h3>
      <p>${escapeHtml(message)}</p>
      <button class="btn btn-secondary" onclick="document.getElementById('dashboard-refresh').click()">
        <span class="material-icons">refresh</span>
        Retry
      </button>
    </div>
  `;
}

/**
 * Render full dashboard
 */
function renderDashboard(status, health) {
  const rigs = status.rigs || [];
  const convoys = state.get('convoys') || [];
  const work = state.get('work') || [];
  const agents = state.get('agents') || [];
  const mail = state.get('mail') || [];

  // Calculate metrics
  const metrics = calculateMetrics(rigs, convoys, work, agents, mail);
  const healthStatus = calculateHealthStatus(health);

  container.innerHTML = `
    <!-- Health Banner -->
    ${renderHealthBanner(healthStatus)}

    <!-- Metrics Grid -->
    <div class="dashboard-metrics">
      ${renderMetricCard('local_shipping', 'Active Convoys', metrics.activeConvoys, metrics.totalConvoys, 'convoys', '#3b82f6')}
      ${renderMetricCard('task_alt', 'Open Work', metrics.openWork, metrics.totalWork, 'work', '#22c55e')}
      ${renderAgentMetricCard(metrics)}
      ${renderMetricCard('mail', 'Unread Mail', metrics.unreadMail, metrics.totalMail, 'mail', '#f59e0b')}
    </div>

    <!-- Main Content Grid -->
    <div class="dashboard-grid">
      <!-- Quick Actions -->
      <div class="dashboard-card quick-actions">
        <div class="card-header">
          <span class="material-icons">bolt</span>
          <h3>Quick Actions</h3>
        </div>
        <div class="card-body">
          ${renderQuickActions()}
        </div>
      </div>

      <!-- Agent Status -->
      <div class="dashboard-card agent-overview">
        <div class="card-header">
          <span class="material-icons">monitoring</span>
          <h3>Agent Status</h3>
        </div>
        <div class="card-body">
          ${renderAgentStatus(rigs, agents)}
        </div>
      </div>

      <!-- Recent Work -->
      <div class="dashboard-card recent-work">
        <div class="card-header">
          <span class="material-icons">history</span>
          <h3>Recent Activity</h3>
        </div>
        <div class="card-body">
          ${renderRecentWork(work)}
        </div>
      </div>

      <!-- Rig Overview -->
      <div class="dashboard-card rig-overview">
        <div class="card-header">
          <span class="material-icons">folder_special</span>
          <h3>Rigs</h3>
        </div>
        <div class="card-body">
          ${renderRigOverview(rigs)}
        </div>
      </div>
    </div>
  `;

  // Add event listeners for quick actions
  setupQuickActionHandlers();
}

/**
 * Calculate dashboard metrics
 */
function calculateMetrics(rigs, convoys, work, agents, mail) {
  const activeConvoys = convoys.filter(c => c.status !== 'completed' && c.status !== 'closed').length;
  const openWork = work.filter(w => w.status !== 'closed' && w.status !== 'done').length;
  const unreadMail = mail.filter(m => !m.read).length;

  // Collect all agents from rigs and get stats
  const allAgents = rigs.flatMap(rig => rig.agents || []);
  const agentStats = getAgentStats(allAgents);

  return {
    activeConvoys,
    totalConvoys: convoys.length,
    openWork,
    totalWork: work.length,
    ...agentStats,  // working, available, stopped, total, statusText
    unreadMail,
    totalMail: mail.length,
  };
}

/**
 * Calculate overall health status
 */
function calculateHealthStatus(health) {
  if (!health || (!health.checks && !health.results)) {
    return { status: 'unknown', label: 'Unknown', icon: 'help', color: '#6b7280' };
  }

  const checks = health.checks || health.results || [];
  let hasError = false;
  let hasWarning = false;

  checks.forEach(check => {
    const status = (check.status || check.result || '').toLowerCase();
    if (status === 'fail' || status === 'error') hasError = true;
    if (status === 'warn' || status === 'warning') hasWarning = true;
  });

  if (hasError) {
    return { status: 'error', label: 'Issues Detected', icon: 'error', color: '#ef4444' };
  }
  if (hasWarning) {
    return { status: 'warning', label: 'Warnings', icon: 'warning', color: '#f59e0b' };
  }
  return { status: 'healthy', label: 'All Systems Go', icon: 'check_circle', color: '#22c55e' };
}

/**
 * Render health banner
 */
function renderHealthBanner(healthStatus) {
  return `
    <div class="health-banner health-${healthStatus.status}" style="--health-color: ${healthStatus.color}">
      <div class="health-banner-icon">
        <span class="material-icons">${healthStatus.icon}</span>
      </div>
      <div class="health-banner-text">
        <span class="health-label">${healthStatus.label}</span>
        <span class="health-hint">System health check</span>
      </div>
      <button class="btn btn-sm btn-ghost" onclick="document.querySelector('[data-view=health]').click()">
        View Details
        <span class="material-icons">arrow_forward</span>
      </button>
    </div>
  `;
}

/**
 * Render metric card
 */
function renderMetricCard(icon, label, value, total, viewId, color) {
  const percentage = total > 0 ? Math.round((value / total) * 100) : 0;

  return `
    <div class="metric-card" data-navigate="${viewId}" style="--metric-color: ${color}">
      <div class="metric-icon">
        <span class="material-icons">${icon}</span>
      </div>
      <div class="metric-content">
        <div class="metric-value">${value}</div>
        <div class="metric-label">${label}</div>
        ${total > 0 ? `<div class="metric-secondary">${value} of ${total}</div>` : ''}
      </div>
      <div class="metric-progress">
        <div class="metric-progress-bar" style="width: ${percentage}%"></div>
      </div>
    </div>
  `;
}

/**
 * Render agent metric card with working/available distinction
 */
function renderAgentMetricCard(metrics) {
  const { working, available, total, statusText } = metrics;
  const running = working + available;
  const color = '#8b5cf6'; // Purple for agents

  // Calculate bar percentages
  const workingPct = total > 0 ? (working / total * 100) : 0;
  const availablePct = total > 0 ? (available / total * 100) : 0;

  return `
    <div class="metric-card" data-navigate="agents" style="--metric-color: ${color}">
      <div class="metric-icon">
        <span class="material-icons">smart_toy</span>
      </div>
      <div class="metric-content">
        <div class="metric-value">${running}</div>
        <div class="metric-label">Active Agents</div>
        <div class="metric-secondary">${statusText}</div>
      </div>
      <div class="metric-progress agent-progress">
        <div class="metric-progress-bar working" style="width: ${workingPct}%"></div>
        <div class="metric-progress-bar available" style="width: ${availablePct}%; left: ${workingPct}%"></div>
      </div>
    </div>
  `;
}

/**
 * Render quick actions
 */
function renderQuickActions() {
  const actions = [
    { id: 'new-bead', icon: 'add_circle', label: 'New Bead', color: '#22c55e', modal: 'new-bead' },
    { id: 'sling-work', icon: 'send', label: 'Sling Work', color: '#3b82f6', modal: 'sling' },
    { id: 'new-convoy', icon: 'local_shipping', label: 'New Convoy', color: '#a855f7', modal: 'new-convoy' },
    { id: 'compose-mail', icon: 'edit', label: 'Send Mail', color: '#f59e0b', modal: 'mail-compose' },
    { id: 'add-rig', icon: 'folder_special', label: 'Add Rig', color: '#06b6d4', modal: 'new-rig' },
    { id: 'run-doctor', icon: 'health_and_safety', label: 'Health Check', color: '#ec4899', action: 'doctor' },
  ];

  return `
    <div class="quick-actions-grid">
      ${actions.map(action => `
        <button class="quick-action-btn" data-quick-action="${action.id}"
                ${action.modal ? `data-modal-open="${action.modal}"` : `data-action="${action.action}"`}
                style="--action-color: ${action.color}">
          <span class="material-icons">${action.icon}</span>
          <span class="action-label">${action.label}</span>
        </button>
      `).join('')}
    </div>
  `;
}

/**
 * Render agent status
 * Shows distinction between: working (has task), available (running but idle), stopped
 */
function renderAgentStatus(rigs, agents) {
  const agentTypes = ['mayor', 'deacon', 'witness', 'refinery', 'polecat'];

  // Group all agents by type
  const agentsByType = {};
  agentTypes.forEach(type => { agentsByType[type] = []; });

  rigs.forEach(rig => {
    (rig.agents || []).forEach(agent => {
      const type = (agent.role || 'polecat').toLowerCase();
      if (agentsByType[type]) {
        agentsByType[type].push(agent);
      }
    });
  });

  return `
    <div class="agent-status-list">
      ${agentTypes.map(type => {
        const config = AGENT_TYPES[type];
        const stats = getAgentStats(agentsByType[type]);

        // Determine indicator icon/class
        let statusIcon, statusClass;
        if (stats.working > 0) {
          statusIcon = 'pending';
          statusClass = 'working';
        } else if (stats.available > 0) {
          statusIcon = 'check_circle';
          statusClass = 'active';
        } else {
          statusIcon = 'radio_button_unchecked';
          statusClass = '';
        }

        const workingPct = stats.total > 0 ? (stats.working / stats.total * 100) : 0;
        const availablePct = stats.total > 0 ? (stats.available / stats.total * 100) : 0;

        return `
          <div class="agent-status-row" style="--agent-color: ${config.color}">
            <div class="agent-status-icon">
              <span class="material-icons">${config.icon}</span>
            </div>
            <div class="agent-status-info">
              <span class="agent-type-label">${config.label}</span>
              <span class="agent-count">${stats.statusText}</span>
            </div>
            <div class="agent-status-bar">
              <div class="status-bar-fill working" style="width: ${workingPct}%"></div>
              <div class="status-bar-fill available" style="width: ${availablePct}%; left: ${workingPct}%"></div>
            </div>
            <span class="agent-status-indicator ${statusClass}">
              <span class="material-icons">${statusIcon}</span>
            </span>
          </div>
        `;
      }).join('')}
    </div>
  `;
}

/**
 * Render recent work
 */
function renderRecentWork(work) {
  if (!work || work.length === 0) {
    return `
      <div class="empty-state small">
        <span class="material-icons">inbox</span>
        <p>No recent work</p>
      </div>
    `;
  }

  // Show last 5 items sorted by date
  const recent = [...work]
    .sort((a, b) => new Date(b.updated_at || b.created_at || 0) - new Date(a.updated_at || a.created_at || 0))
    .slice(0, 5);

  return `
    <div class="recent-work-list">
      ${recent.map(item => {
        const statusColor = item.status === 'closed' || item.status === 'done' ? '#22c55e' :
                           item.status === 'in_progress' || item.status === 'in-progress' ? '#3b82f6' : '#6b7280';
        return `
          <div class="recent-work-item" data-bead-id="${item.id}">
            <span class="work-status-dot" style="background: ${statusColor}"></span>
            <div class="work-info">
              <span class="work-title">${escapeHtml(item.title || item.id)}</span>
              <span class="work-meta">${formatTimeAgo(item.updated_at || item.created_at)}</span>
            </div>
            <span class="work-status-badge" style="color: ${statusColor}">${item.status || 'open'}</span>
          </div>
        `;
      }).join('')}
    </div>
    <a href="#" class="view-all-link" onclick="document.querySelector('[data-view=work]').click(); return false;">
      View all work <span class="material-icons">arrow_forward</span>
    </a>
  `;
}

/**
 * Render rig overview
 */
function renderRigOverview(rigs) {
  if (!rigs || rigs.length === 0) {
    return `
      <div class="empty-state small">
        <span class="material-icons">folder_off</span>
        <p>No rigs configured</p>
        <button class="btn btn-sm btn-primary" data-modal-open="new-rig">
          <span class="material-icons">add</span>
          Add Rig
        </button>
      </div>
    `;
  }

  return `
    <div class="rig-overview-list">
      ${rigs.slice(0, 4).map(rig => {
        const stats = getAgentStats(rig.agents || []);
        const indicatorClass = stats.working > 0 ? 'working' : (stats.available > 0 ? 'active' : '');

        return `
          <div class="rig-overview-item" data-rig="${rig.name}">
            <div class="rig-icon">
              <span class="material-icons">folder_special</span>
            </div>
            <div class="rig-info">
              <span class="rig-name">${escapeHtml(rig.name)}</span>
              <span class="rig-agents">${stats.statusText}</span>
            </div>
            <span class="rig-status-indicator ${indicatorClass}"></span>
          </div>
        `;
      }).join('')}
    </div>
    ${rigs.length > 4 ? `
      <a href="#" class="view-all-link" onclick="document.querySelector('[data-view=rigs]').click(); return false;">
        View all ${rigs.length} rigs <span class="material-icons">arrow_forward</span>
      </a>
    ` : ''}
  `;
}

/**
 * Setup quick action handlers
 */
function setupQuickActionHandlers() {
  // Metric cards navigate to views
  container.querySelectorAll('.metric-card[data-navigate]').forEach(card => {
    card.addEventListener('click', () => {
      const viewId = card.dataset.navigate;
      document.querySelector(`[data-view="${viewId}"]`)?.click();
    });
  });

  // Doctor action
  container.querySelectorAll('[data-action="doctor"]').forEach(btn => {
    btn.addEventListener('click', () => {
      document.querySelector('[data-view="health"]')?.click();
    });
  });

  // Work items
  container.querySelectorAll('.recent-work-item').forEach(item => {
    item.addEventListener('click', () => {
      const beadId = item.dataset.beadId;
      document.dispatchEvent(new CustomEvent('bead:detail', { detail: { beadId, bead: { id: beadId } } }));
    });
  });

  // Rig items
  container.querySelectorAll('.rig-overview-item').forEach(item => {
    item.addEventListener('click', () => {
      document.querySelector('[data-view="rigs"]')?.click();
    });
  });
}

/**
 * Format time ago
 */
function formatTimeAgo(timestamp) {
  if (!timestamp) return '';
  const date = new Date(timestamp);
  const now = new Date();
  const diff = now - date;

  if (diff < 60000) return 'just now';
  if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`;
  if (diff < 86400000) return `${Math.floor(diff / 3600000)}h ago`;
  return `${Math.floor(diff / 86400000)}d ago`;
}

/**
 * Escape HTML
 */
function escapeHtml(str) {
  if (!str) return '';
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}
