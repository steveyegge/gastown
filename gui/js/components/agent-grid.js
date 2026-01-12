/**
 * Gas Town GUI - Agent Grid Component
 *
 * Renders agents in a responsive grid layout with status and actions.
 */

import { AGENT_TYPES, STATUS_ICONS, STATUS_COLORS, getAgentConfig, formatAgentName } from '../shared/agent-types.js';

/**
 * Render the agent grid
 * @param {HTMLElement} container - The grid container
 * @param {Array} agents - Array of agent objects
 */
export function renderAgentGrid(container, agents) {
  if (!container) return;

  if (!agents || agents.length === 0) {
    container.innerHTML = `
      <div class="empty-state">
        <span class="material-icons empty-icon">group</span>
        <h3>No Agents</h3>
        <p>Agents will appear here when work is dispatched</p>
      </div>
    `;
    return;
  }

  container.innerHTML = agents.map((agent, index) => renderAgentCard(agent, index)).join('');

  // Add event listeners for agent actions
  container.querySelectorAll('.agent-card').forEach(card => {
    card.addEventListener('click', (e) => {
      if (!e.target.closest('button')) {
        const agentId = card.dataset.agentId;
        showAgentDetail(agentId);
      }
    });

    // Nudge button
    const nudgeBtn = card.querySelector('[data-action="nudge"]');
    if (nudgeBtn) {
      nudgeBtn.addEventListener('click', (e) => {
        e.stopPropagation();
        const agentId = card.dataset.agentId;
        showNudgeModal(agentId);
      });
    }
  });
}

/**
 * Render a single agent card
 */
function renderAgentCard(agent, index) {
  const role = agent.role?.toLowerCase() || 'polecat';
  const agentConfig = getAgentConfig(agent.address || agent.id, role);
  const status = agent.running ? 'running' : (agent.status || 'idle');
  const statusIcon = STATUS_ICONS[status] || STATUS_ICONS.idle;
  const statusColor = STATUS_COLORS[status] || STATUS_COLORS.idle;

  return `
    <div class="agent-card animate-spawn stagger-${Math.min(index, 6)}"
         data-agent-id="${agent.id || agent.address}"
         style="--agent-color: ${agentConfig.color}">
      <div class="agent-header">
        <div class="agent-avatar" style="background-color: ${agentConfig.color}20; border-color: ${agentConfig.color}">
          <span class="material-icons" style="color: ${agentConfig.color}">${agentConfig.icon}</span>
        </div>
        <div class="agent-info">
          <h3 class="agent-name">${escapeHtml(agent.name || formatAgentName(agent.id))}</h3>
          <div class="agent-role" style="color: ${agentConfig.color}">${agentConfig.label}</div>
        </div>
        <div class="agent-status status-${status}">
          <span class="material-icons" style="color: ${statusColor}">${statusIcon}</span>
        </div>
      </div>

      ${agent.current_task ? `
        <div class="agent-task">
          <span class="material-icons">task</span>
          <span class="task-text">${escapeHtml(truncate(agent.current_task, 40))}</span>
        </div>
      ` : ''}

      ${agent.progress !== undefined ? `
        <div class="agent-progress">
          <div class="progress-bar small">
            <div class="progress-fill" style="width: ${Math.round(agent.progress * 100)}%"></div>
          </div>
        </div>
      ` : ''}

      <div class="agent-footer">
        <div class="agent-stats">
          ${renderAgentStats(agent)}
        </div>
        <div class="agent-actions">
          <button class="btn btn-icon btn-sm" title="Nudge Agent" data-action="nudge">
            <span class="material-icons">notifications_active</span>
          </button>
          <button class="btn btn-icon btn-sm" title="View Details" data-action="view">
            <span class="material-icons">info</span>
          </button>
        </div>
      </div>

      ${status === 'working' ? '<div class="agent-pulse"></div>' : ''}
    </div>
  `;
}

/**
 * Render agent statistics
 */
function renderAgentStats(agent) {
  const stats = [];

  if (agent.tasks_completed !== undefined) {
    stats.push(`
      <span class="agent-stat" title="Tasks Completed">
        <span class="material-icons">check</span>${agent.tasks_completed}
      </span>
    `);
  }

  if (agent.uptime) {
    stats.push(`
      <span class="agent-stat" title="Uptime">
        <span class="material-icons">timer</span>${formatDuration(agent.uptime)}
      </span>
    `);
  }

  if (agent.convoy_id) {
    stats.push(`
      <span class="agent-stat" title="Convoy">
        <span class="material-icons">local_shipping</span>${agent.convoy_id.slice(0, 6)}
      </span>
    `);
  }

  return stats.join('');
}

/**
 * Show agent detail modal
 */
function showAgentDetail(agentId) {
  const event = new CustomEvent('agent:detail', { detail: { agentId } });
  document.dispatchEvent(event);
}

/**
 * Show nudge modal for an agent
 */
function showNudgeModal(agentId) {
  const event = new CustomEvent('agent:nudge', { detail: { agentId } });
  document.dispatchEvent(event);
}

// Utility functions
function escapeHtml(str) {
  if (!str) return '';
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}

function truncate(str, length) {
  if (!str) return '';
  return str.length > length ? str.slice(0, length) + '...' : str;
}

function capitalize(str) {
  return str.charAt(0).toUpperCase() + str.slice(1);
}

function formatDuration(seconds) {
  if (!seconds) return '0s';
  if (seconds < 60) return `${Math.round(seconds)}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
  return `${Math.floor(seconds / 3600)}h`;
}
