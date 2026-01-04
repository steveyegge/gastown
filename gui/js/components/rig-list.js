/**
 * Gas Town GUI - Rig List Component
 *
 * Renders the list of rigs (git projects) with their agents and status.
 */

import { AGENT_TYPES, STATUS_ICONS, STATUS_COLORS, getAgentConfig } from '../shared/agent-types.js';

/**
 * Render the rig list
 * @param {HTMLElement} container - The rig list container
 * @param {Array} rigs - Array of rig objects from status.rigs
 */
export function renderRigList(container, rigs) {
  if (!container) return;

  if (!rigs || rigs.length === 0) {
    container.innerHTML = `
      <div class="empty-state">
        <span class="material-icons empty-icon">folder_off</span>
        <h3>No Rigs</h3>
        <p>Add a rig to get started with multi-agent development</p>
      </div>
    `;
    return;
  }

  container.innerHTML = rigs.map((rig, index) => renderRigCard(rig, index)).join('');

  // Add event listeners
  container.querySelectorAll('.rig-card').forEach(card => {
    // GitHub link
    const githubBtn = card.querySelector('[data-action="github"]');
    if (githubBtn) {
      githubBtn.addEventListener('click', (e) => {
        e.stopPropagation();
        const url = githubBtn.dataset.url;
        if (url) window.open(url, '_blank');
      });
    }

    // Peek at agents
    card.querySelectorAll('[data-action="peek"]').forEach(btn => {
      btn.addEventListener('click', (e) => {
        e.stopPropagation();
        const agentId = btn.dataset.agentId;
        showAgentPeek(agentId);
      });
    });
  });
}

/**
 * Render a single rig card
 */
function renderRigCard(rig, index) {
  const polecatCount = rig.polecat_count || 0;
  const crewCount = rig.crew_count || 0;
  const runningAgents = (rig.agents || []).filter(a => a.running).length;
  const totalAgents = (rig.agents || []).length;

  // Get GitHub URL from config (will be added to API)
  const githubUrl = rig.git_url || null;

  return `
    <div class="rig-card animate-spawn stagger-${Math.min(index, 6)}" data-rig-name="${rig.name}">
      <div class="rig-header">
        <div class="rig-icon">
          <span class="material-icons">folder_special</span>
        </div>
        <div class="rig-info">
          <h3 class="rig-name">${escapeHtml(rig.name)}</h3>
          <div class="rig-meta">
            ${githubUrl ? `
              <a href="${githubUrl}" target="_blank" class="rig-github-link" title="Open on GitHub">
                <span class="material-icons">open_in_new</span>
                ${extractRepoName(githubUrl)}
              </a>
            ` : '<span class="rig-local">Local only</span>'}
          </div>
        </div>
        <div class="rig-status">
          <span class="status-dot ${runningAgents > 0 ? 'active' : ''}"></span>
          <span class="status-text">${runningAgents}/${totalAgents} active</span>
        </div>
      </div>

      <div class="rig-stats">
        <div class="rig-stat">
          <span class="material-icons" style="color: ${AGENT_TYPES.polecat.color}">${AGENT_TYPES.polecat.icon}</span>
          <span class="stat-value">${polecatCount}</span>
          <span class="stat-label">Polecats</span>
        </div>
        <div class="rig-stat">
          <span class="material-icons" style="color: ${AGENT_TYPES.witness.color}">${AGENT_TYPES.witness.icon}</span>
          <span class="stat-value">${rig.has_witness ? '1' : '0'}</span>
          <span class="stat-label">Witness</span>
        </div>
        <div class="rig-stat">
          <span class="material-icons" style="color: ${AGENT_TYPES.refinery.color}">${AGENT_TYPES.refinery.icon}</span>
          <span class="stat-value">${rig.has_refinery ? '1' : '0'}</span>
          <span class="stat-label">Refinery</span>
        </div>
        <div class="rig-stat">
          <span class="material-icons" style="color: ${AGENT_TYPES.crew.color}">${AGENT_TYPES.crew.icon}</span>
          <span class="stat-value">${crewCount}</span>
          <span class="stat-label">Crews</span>
        </div>
      </div>

      ${rig.agents && rig.agents.length > 0 ? `
        <div class="rig-agents">
          <div class="agents-header">
            <span class="material-icons">groups</span>
            Agents
          </div>
          <div class="agents-list">
            ${rig.agents.map(agent => renderRigAgent(agent, rig.name)).join('')}
          </div>
        </div>
      ` : ''}

      <div class="rig-actions">
        ${githubUrl ? `
          <button class="btn btn-sm btn-secondary" data-action="github" data-url="${githubUrl}" title="Open on GitHub">
            <span class="material-icons">open_in_new</span>
            GitHub
          </button>
        ` : ''}
        <button class="btn btn-sm btn-secondary" data-action="spawn" title="Spawn a new polecat">
          <span class="material-icons">add</span>
          Spawn Polecat
        </button>
        <button class="btn btn-sm btn-ghost" data-action="settings" title="Rig settings">
          <span class="material-icons">settings</span>
        </button>
      </div>
    </div>
  `;
}

/**
 * Render an agent row within a rig
 */
function renderRigAgent(agent, rigName) {
  const config = getAgentConfig(agent.address, agent.role);
  const status = agent.running ? 'running' : 'stopped';
  const statusColor = STATUS_COLORS[status];
  const statusIcon = STATUS_ICONS[status];

  return `
    <div class="rig-agent" data-agent-id="${agent.address}">
      <span class="agent-icon material-icons" style="color: ${config.color}">${config.icon}</span>
      <span class="agent-name">${escapeHtml(agent.name)}</span>
      <span class="agent-role" style="color: ${config.color}">${config.label}</span>
      <span class="agent-status">
        <span class="material-icons" style="color: ${statusColor}">${statusIcon}</span>
      </span>
      ${agent.has_work ? '<span class="agent-work-badge" title="Has work hooked">⚡</span>' : ''}
      <button class="btn btn-icon btn-xs" data-action="peek" data-agent-id="${agent.address}" title="Peek at output">
        <span class="material-icons">visibility</span>
      </button>
    </div>
  `;
}

/**
 * Extract repo name from GitHub URL
 */
function extractRepoName(url) {
  if (!url) return '';
  const match = url.match(/github\.com\/([^\/]+\/[^\/]+)/);
  return match ? match[1] : url;
}

/**
 * Show agent peek modal
 */
function showAgentPeek(agentId) {
  const event = new CustomEvent('agent:peek', { detail: { agentId } });
  document.dispatchEvent(event);
}

// Utility
function escapeHtml(str) {
  if (!str) return '';
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}
