/**
 * Gas Town GUI - Sidebar Component
 *
 * Renders the agent tree and quick status in the sidebar.
 */

import { AGENT_TYPES, STATUS_ICONS, getAgentType, getAgentConfig, formatAgentName } from '../shared/agent-types.js';
import { api } from '../api.js';
import { showToast } from './toast.js';
import { escapeHtml, escapeAttr, truncate, capitalize } from '../utils/html.js';

// Simple pluralization for agent type labels
function pluralize(word, count) {
  if (count === 1) return word;
  // Handle special cases
  if (word.toLowerCase() === 'witness') return 'Witnesses';
  if (word.toLowerCase() === 'refinery') return 'Refineries';
  // Default: just add 's'
  return word + 's';
}

/**
 * Render the sidebar with agent tree
 * @param {HTMLElement} container - The sidebar container
 * @param {Object} status - Current town status
 */
export function renderSidebar(container, status) {
  if (!container || !status) return;

  // Combine town-level agents with rig agents (polecats, witnesses, refineries)
  const townAgents = status.agents || [];
  const rigAgents = [];

  for (const rig of status.rigs || []) {
    for (const agent of rig.agents || []) {
      rigAgents.push({
        ...agent,
        rig: rig.name,
        id: agent.address || `${rig.name}/${agent.name}`,
      });
    }
  }

  const agents = [...townAgents, ...rigAgents];
  const hook = status.hook;

  // Group agents by role
  const agentsByRole = groupByRole(agents);

  container.innerHTML = `
    <div class="sidebar-section">
      <h3 class="sidebar-title">
        <span class="material-icons">account_tree</span>
        Agents
      </h3>
      ${renderAgentTree(agentsByRole)}
    </div>

    <div class="sidebar-section">
      <h3 class="sidebar-title">
        <span class="material-icons">settings_applications</span>
        Services
      </h3>
      ${renderServiceControls(agentsByRole)}
    </div>

    ${hook ? renderHookSection(hook) : ''}

    <div class="sidebar-section">
      <h3 class="sidebar-title">
        <span class="material-icons">insights</span>
        Stats
      </h3>
      ${renderStats(status)}
    </div>
  `;

  // Add service control event listeners
  setupServiceControls(container);
}

/**
 * Group agents by their role
 */
function groupByRole(agents) {
  const groups = {
    mayor: [],
    deacon: [],
    polecat: [],
    witness: [],
    refinery: [],
    other: [],
  };

  // Map API role names to our group names
  const roleMap = {
    'coordinator': 'mayor',
    'health-check': 'deacon',
  };

  agents.forEach(agent => {
    const apiRole = agent.role?.toLowerCase() || 'other';
    // Map coordinator->mayor, health-check->deacon, or use role directly
    const role = roleMap[apiRole] || apiRole;
    if (groups[role]) {
      groups[role].push(agent);
    } else {
      groups.other.push(agent);
    }
  });

  return groups;
}

/**
 * Render the hierarchical agent tree
 */
function renderAgentTree(agentsByRole) {
  const roles = ['mayor', 'deacon', 'polecat', 'witness', 'refinery', 'other'];

  let html = '<ul class="tree-view">';

  roles.forEach(role => {
    const agents = agentsByRole[role];
    if (!agents || agents.length === 0) return;

    const config = AGENT_TYPES[role] || AGENT_TYPES.system;

    html += `
      <li class="tree-node expandable expanded">
        <div class="tree-node-content">
          <span class="material-icons tree-icon" style="color: ${config.color}">${config.icon}</span>
          <span class="tree-label" style="color: ${config.color}">${pluralize(config.label, agents.length)}</span>
          <span class="tree-badge">${agents.length}</span>
        </div>
        <ul class="tree-children">
          ${agents.map(agent => renderAgentNode(agent)).join('')}
        </ul>
      </li>
    `;
  });

  html += '</ul>';
  return html;
}

/**
 * Render a single agent node
 */
function renderAgentNode(agent) {
  const status = agent.running ? 'running' : (agent.status || 'idle');
  const statusIcon = STATUS_ICONS[status] || 'help';
  const config = getAgentConfig(agent.address || agent.id, agent.role);

  return `
    <li class="tree-node">
      <div class="tree-node-content agent-node" data-agent-id="${escapeAttr(agent.id || agent.address || '')}">
        <span class="material-icons tree-icon status-${escapeAttr(status)}" style="color: ${config.color}">${statusIcon}</span>
        <span class="tree-label" style="color: ${config.color}">${escapeHtml(agent.name || formatAgentName(agent.id) || 'Unknown')}</span>
        ${agent.current_task ? `<span class="tree-task">${escapeHtml(truncate(agent.current_task, 20))}</span>` : ''}
      </div>
    </li>
  `;
}

/**
 * Render the hook section (currently hooked work)
 */
function renderHookSection(hook) {
  return `
    <div class="sidebar-section hook-section">
      <h3 class="sidebar-title">
        <span class="material-icons">anchor</span>
        Hook
      </h3>
      <div class="hook-card">
        <div class="hook-bead">${escapeHtml(hook.bead_id || 'Unknown')}</div>
        <div class="hook-meta">
          <span class="hook-status status-${escapeAttr(hook.status || 'idle')}">${escapeHtml(hook.status || 'idle')}</span>
        </div>
        ${hook.title ? `<div class="hook-title">${escapeHtml(truncate(hook.title, 40))}</div>` : ''}
      </div>
    </div>
  `;
}

/**
 * Render stats section
 */
function renderStats(status) {
  const stats = [
    { label: 'Convoys', value: status.convoy_count || 0, icon: 'local_shipping' },
    { label: 'Active', value: status.active_agents || 0, icon: 'person' },
    { label: 'Pending', value: status.pending_tasks || 0, icon: 'pending' },
  ];

  return `
    <div class="stats-grid">
      ${stats.map(stat => `
        <div class="stat-item">
          <span class="material-icons stat-icon">${stat.icon}</span>
          <div class="stat-content">
            <div class="stat-value">${stat.value}</div>
            <div class="stat-label">${stat.label}</div>
          </div>
        </div>
      `).join('')}
    </div>
  `;
}

/**
 * Render service control buttons
 */
function renderServiceControls(agentsByRole) {
  const services = [
    { name: 'mayor', label: 'Mayor', icon: 'account_balance', color: AGENT_TYPES.mayor.color },
    { name: 'deacon', label: 'Deacon', icon: 'settings', color: AGENT_TYPES.deacon.color },
    { name: 'witness', label: 'Witness', icon: 'visibility', color: AGENT_TYPES.witness.color },
    { name: 'refinery', label: 'Refinery', icon: 'precision_manufacturing', color: AGENT_TYPES.refinery.color },
  ];

  return `
    <div class="service-controls">
      ${services.map(svc => {
        const agents = agentsByRole[svc.name] || [];
        const isRunning = agents.some(a => a.running);
        const statusClass = isRunning ? 'running' : 'stopped';

        return `
          <div class="service-item" data-service="${svc.name}">
            <div class="service-info">
              <span class="material-icons service-icon" style="color: ${svc.color}">${svc.icon}</span>
              <span class="service-name">${svc.label}</span>
              <span class="service-status status-${statusClass}">${isRunning ? 'Running' : 'Stopped'}</span>
            </div>
            <div class="service-actions">
              ${isRunning ? `
                <button class="btn btn-icon btn-xs btn-danger-ghost" data-action="stop" data-service="${svc.name}" title="Stop ${svc.label}">
                  <span class="material-icons">stop</span>
                </button>
                <button class="btn btn-icon btn-xs" data-action="restart" data-service="${svc.name}" title="Restart ${svc.label}">
                  <span class="material-icons">refresh</span>
                </button>
              ` : `
                <button class="btn btn-icon btn-xs btn-success-ghost" data-action="start" data-service="${svc.name}" title="Start ${svc.label}">
                  <span class="material-icons">play_arrow</span>
                </button>
              `}
            </div>
          </div>
        `;
      }).join('')}
    </div>
  `;
}

/**
 * Setup service control event listeners
 */
function setupServiceControls(container) {
  container.querySelectorAll('.service-actions [data-action]').forEach(btn => {
    btn.addEventListener('click', async (e) => {
      e.stopPropagation();
      const action = btn.dataset.action;
      const service = btn.dataset.service;
      await handleServiceAction(action, service, btn);
    });
  });
}

/**
 * Handle service start/stop/restart action
 */
async function handleServiceAction(action, service, btn) {
  const originalIcon = btn.innerHTML;
  btn.innerHTML = '<span class="material-icons spinning">sync</span>';
  btn.disabled = true;

  try {
    let result;
    switch (action) {
      case 'start':
        result = await api.startService(service);
        break;
      case 'stop':
        result = await api.stopService(service);
        break;
      case 'restart':
        result = await api.restartService(service);
        break;
    }

    if (result.success) {
      showToast(`${capitalize(service)} ${action}ed successfully`, 'success');
      // Trigger status refresh
      document.dispatchEvent(new CustomEvent('status:refresh'));
    } else {
      showToast(`Failed to ${action} ${service}: ${result.error}`, 'error');
    }
  } catch (err) {
    showToast(`Error: ${err.message}`, 'error');
  } finally {
    btn.innerHTML = originalIcon;
    btn.disabled = false;
  }
}

// Tree node toggle functionality and agent click handling
document.addEventListener('click', (e) => {
  const nodeContent = e.target.closest('.tree-node-content');
  if (!nodeContent) return;

  // Check if this is an agent node (has data-agent-id)
  const agentId = nodeContent.dataset.agentId;
  if (agentId) {
    e.stopPropagation();
    showAgentQuickActions(nodeContent, agentId);
    return;
  }

  // Otherwise, handle folder expand/collapse
  const node = nodeContent.closest('.tree-node.expandable');
  if (node) {
    node.classList.toggle('expanded');
    const icon = nodeContent.querySelector('.tree-icon');
    if (icon) {
      icon.textContent = node.classList.contains('expanded') ? 'folder_open' : 'folder';
    }
  }
});

/**
 * Show quick actions popover for an agent
 */
function showAgentQuickActions(nodeEl, agentId) {
  // Remove any existing popover
  const existing = document.querySelector('.agent-quick-actions');
  if (existing) {
    existing.remove();
    // If clicking same agent, just close
    if (existing.dataset.agentId === agentId) return;
  }

  const rect = nodeEl.getBoundingClientRect();
  const agentName = nodeEl.querySelector('.tree-label')?.textContent || agentId;
  const agentStatus = nodeEl.querySelector('.tree-icon')?.classList.contains('status-working') ? 'working' : 'idle';
  const currentTask = nodeEl.querySelector('.tree-task')?.textContent || 'No active task';

  const popover = document.createElement('div');
  popover.className = 'agent-quick-actions';
  popover.dataset.agentId = agentId;
  popover.innerHTML = `
    <div class="agent-popover-header">
      <span class="agent-popover-name">${escapeHtml(agentName)}</span>
      <span class="agent-popover-status status-${escapeAttr(agentStatus)}">${escapeHtml(agentStatus)}</span>
    </div>
    <div class="agent-popover-task">${escapeHtml(currentTask)}</div>
    <div class="agent-popover-actions">
      <button class="btn btn-sm btn-secondary" data-action="nudge" title="Send a nudge">
        <span class="material-icons">notifications</span> Nudge
      </button>
      <button class="btn btn-sm btn-secondary" data-action="mail" title="Send mail">
        <span class="material-icons">mail</span> Mail
      </button>
      <button class="btn btn-sm btn-secondary" data-action="view" title="View in Agents tab">
        <span class="material-icons">open_in_new</span> View
      </button>
    </div>
  `;

  // Position the popover
  popover.style.cssText = `
    position: fixed;
    top: ${rect.bottom + 8}px;
    left: ${rect.left}px;
    z-index: 9999;
    min-width: 220px;
    background: var(--bg-elevated);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-lg);
    padding: var(--space-md);
    animation: fadeIn 0.15s ease;
  `;

  document.body.appendChild(popover);

  // Adjust if off-screen
  const popRect = popover.getBoundingClientRect();
  if (popRect.right > window.innerWidth) {
    popover.style.left = `${window.innerWidth - popRect.width - 10}px`;
  }
  if (popRect.bottom > window.innerHeight) {
    popover.style.top = `${rect.top - popRect.height - 8}px`;
  }

  // Handle action clicks
  popover.addEventListener('click', (e) => {
    const actionBtn = e.target.closest('[data-action]');
    if (!actionBtn) return;

    const action = actionBtn.dataset.action;
    popover.remove();

    switch (action) {
      case 'nudge':
        openNudgeModal(agentId, agentName);
        break;
      case 'mail':
        openMailModal(agentId, agentName);
        break;
      case 'view':
        switchToAgentsTab(agentId);
        break;
    }
  });

  // Close on click outside
  setTimeout(() => {
    document.addEventListener('click', function closePopover(e) {
      if (!popover.contains(e.target) && !nodeEl.contains(e.target)) {
        popover.remove();
        document.removeEventListener('click', closePopover);
      }
    });
  }, 0);
}

/**
 * Open nudge modal for an agent
 */
function openNudgeModal(agentId, agentName) {
  const nudgeModal = document.getElementById('nudge-modal');
  if (nudgeModal) {
    const targetField = nudgeModal.querySelector('[name="to"]');
    if (targetField) targetField.value = agentId;
    const titleField = nudgeModal.querySelector('.modal-header h2');
    if (titleField) titleField.textContent = `Nudge ${agentName}`;

    // Show modal
    document.getElementById('modal-overlay')?.classList.remove('hidden');
    nudgeModal.classList.remove('hidden');
  }
}

/**
 * Open mail compose modal for an agent
 */
function openMailModal(agentId, agentName) {
  const mailModal = document.getElementById('compose-modal');
  if (mailModal) {
    const toField = mailModal.querySelector('[name="to"]');
    if (toField) {
      // If it's a select, try to select the option
      if (toField.tagName === 'SELECT') {
        const option = Array.from(toField.options).find(o => o.value === agentId);
        if (option) toField.value = agentId;
      } else {
        toField.value = agentId;
      }
    }

    // Show modal
    document.getElementById('modal-overlay')?.classList.remove('hidden');
    mailModal.classList.remove('hidden');
  }
}

/**
 * Switch to Agents tab and highlight the agent
 */
function switchToAgentsTab(agentId) {
  // Click the agents tab
  const agentsTab = document.querySelector('[data-view="agents"]');
  if (agentsTab) {
    agentsTab.click();

    // After a short delay, try to highlight the agent card
    setTimeout(() => {
      const agentCard = document.querySelector(`[data-agent-id="${agentId}"]`);
      if (agentCard) {
        agentCard.scrollIntoView({ behavior: 'smooth', block: 'center' });
        agentCard.classList.add('highlight');
        setTimeout(() => agentCard.classList.remove('highlight'), 2000);
      }
    }, 100);
  }
}
