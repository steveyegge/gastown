/**
 * Gas Town GUI - Mail List Component
 *
 * Renders the mail inbox with messages from the Gastown system.
 */

import { AGENT_TYPES, getAgentType, getAgentConfig, formatAgentName } from '../shared/agent-types.js';

// Priority icons and colors
const PRIORITY_CONFIG = {
  high: { icon: 'priority_high', class: 'priority-high' },
  normal: { icon: 'mail', class: 'priority-normal' },
  low: { icon: 'mail_outline', class: 'priority-low' },
};

/**
 * Get unique agents from mail list for filtering
 */
function getUniqueAgents(mail) {
  const agents = new Map();
  mail.forEach(m => {
    if (m.from) {
      const type = getAgentType(m.from);
      const name = formatAgentName(m.from);
      agents.set(m.from, { path: m.from, name, type });
    }
    if (m.to) {
      const type = getAgentType(m.to);
      const name = formatAgentName(m.to);
      agents.set(m.to, { path: m.to, name, type });
    }
  });
  return Array.from(agents.values()).sort((a, b) => a.name.localeCompare(b.name));
}

/**
 * Get unique rigs from mail list
 */
function getUniqueRigs(mail) {
  const rigs = new Set();
  mail.forEach(m => {
    if (m.from) {
      const rig = m.from.split('/')[0];
      if (rig && rig !== 'mayor' && rig !== 'human') rigs.add(rig);
    }
    if (m.to) {
      const rig = m.to.split('/')[0];
      if (rig && rig !== 'mayor' && rig !== 'human') rigs.add(rig);
    }
  });
  return Array.from(rigs).sort();
}


/**
 * Render the mail list
 * @param {HTMLElement} container - The mail list container
 * @param {Array} mail - Array of mail objects
 */
// Current filter state (module-level)
let currentFilters = {
  agentType: 'all',
  rig: 'all',
  search: '',
};

export function renderMailList(container, mail, options = {}) {
  if (!container) return;

  const isAllMail = options.isAllMail || (mail && mail[0]?.feedEvent);

  if (!mail || mail.length === 0) {
    container.innerHTML = `
      <div class="empty-state">
        <span class="material-icons empty-icon">${isAllMail ? 'forum' : 'mail'}</span>
        <h3>${isAllMail ? 'No System Mail' : 'No Mail'}</h3>
        <p>${isAllMail ? 'No mail activity in the system yet' : 'Your inbox is empty'}</p>
      </div>
    `;
    return;
  }

  // Build filter UI for all-mail view
  const filterHtml = isAllMail ? buildFilterUI(mail) : '';

  // Apply filters
  let filtered = [...mail];
  if (isAllMail) {
    if (currentFilters.agentType !== 'all') {
      filtered = filtered.filter(m =>
        getAgentType(m.from) === currentFilters.agentType ||
        getAgentType(m.to) === currentFilters.agentType
      );
    }
    if (currentFilters.rig !== 'all') {
      filtered = filtered.filter(m =>
        m.from?.startsWith(currentFilters.rig) ||
        m.to?.startsWith(currentFilters.rig)
      );
    }
    if (currentFilters.search) {
      const searchLower = currentFilters.search.toLowerCase();
      filtered = filtered.filter(m =>
        m.subject?.toLowerCase().includes(searchLower) ||
        m.from?.toLowerCase().includes(searchLower) ||
        m.to?.toLowerCase().includes(searchLower) ||
        m.body?.toLowerCase().includes(searchLower)
      );
    }
  }

  // Sort by date (newest first), then by read status
  const sorted = filtered.sort((a, b) => {
    // Unread first
    if (a.read !== b.read) return a.read ? 1 : -1;
    // Then by date
    return new Date(b.timestamp || 0) - new Date(a.timestamp || 0);
  });

  // Render
  const itemsHtml = sorted.length > 0
    ? sorted.map((item, index) => renderMailItem(item, index)).join('')
    : `<div class="empty-state small">
        <span class="material-icons">filter_list_off</span>
        <p>No mail matches your filters</p>
      </div>`;

  container.innerHTML = filterHtml + itemsHtml;

  // Add filter event handlers
  if (isAllMail) {
    setupFilterHandlers(container, mail, options);
  }

  // Add click handlers
  container.querySelectorAll('.mail-item').forEach(item => {
    item.addEventListener('click', () => {
      const mailId = item.dataset.mailId;
      showMailDetail(mailId, mail.find(m => m.id === mailId));
    });
  });
}

/**
 * Build filter UI HTML
 */
function buildFilterUI(mail) {
  const rigs = getUniqueRigs(mail);
  const agentTypes = Object.entries(AGENT_TYPES);

  return `
    <div class="mail-filters">
      <div class="filter-row">
        <div class="filter-group">
          <label>Agent Type</label>
          <select id="mail-agent-filter" class="filter-select">
            <option value="all">All Types</option>
            ${agentTypes.map(([key, config]) => `
              <option value="${key}" ${currentFilters.agentType === key ? 'selected' : ''}>
                ${config.label}
              </option>
            `).join('')}
          </select>
        </div>

        <div class="filter-group">
          <label>Rig</label>
          <select id="mail-rig-filter" class="filter-select">
            <option value="all">All Rigs</option>
            ${rigs.map(rig => `
              <option value="${rig}" ${currentFilters.rig === rig ? 'selected' : ''}>
                ${rig}
              </option>
            `).join('')}
          </select>
        </div>

        <div class="filter-group search-group">
          <label>Search</label>
          <input type="text" id="mail-search" class="filter-input"
                 placeholder="Search mail..." value="${escapeHtml(currentFilters.search)}">
        </div>

        <button class="btn btn-ghost btn-sm" id="mail-clear-filters" title="Clear filters">
          <span class="material-icons">clear</span>
        </button>
      </div>

      <div class="agent-legend">
        ${agentTypes.map(([key, config]) => `
          <span class="legend-item" style="--agent-color: ${config.color}">
            <span class="material-icons" style="color: ${config.color}">${config.icon}</span>
            ${config.label}
          </span>
        `).join('')}
      </div>
    </div>
  `;
}

/**
 * Setup filter event handlers
 */
function setupFilterHandlers(container, mail, options) {
  const agentFilter = container.querySelector('#mail-agent-filter');
  const rigFilter = container.querySelector('#mail-rig-filter');
  const searchInput = container.querySelector('#mail-search');
  const clearBtn = container.querySelector('#mail-clear-filters');

  if (agentFilter) {
    agentFilter.addEventListener('change', () => {
      currentFilters.agentType = agentFilter.value;
      renderMailList(container, mail, options);
    });
  }

  if (rigFilter) {
    rigFilter.addEventListener('change', () => {
      currentFilters.rig = rigFilter.value;
      renderMailList(container, mail, options);
    });
  }

  if (searchInput) {
    let searchTimeout;
    searchInput.addEventListener('input', () => {
      clearTimeout(searchTimeout);
      searchTimeout = setTimeout(() => {
        currentFilters.search = searchInput.value;
        renderMailList(container, mail, options);
      }, 300);
    });
  }

  if (clearBtn) {
    clearBtn.addEventListener('click', () => {
      currentFilters = { agentType: 'all', rig: 'all', search: '' };
      renderMailList(container, mail, options);
    });
  }
}

/**
 * Render a single mail item
 */
function renderMailItem(mail, index) {
  const priority = mail.priority || 'normal';
  const priorityConfig = PRIORITY_CONFIG[priority] || PRIORITY_CONFIG.normal;
  const isUnread = !mail.read;
  const isFeedMail = mail.feedEvent; // From all-mail view

  // Get agent types for color coding
  const fromType = getAgentType(mail.from);
  const toType = getAgentType(mail.to);
  const fromConfig = AGENT_TYPES[fromType] || AGENT_TYPES.system;
  const toConfig = AGENT_TYPES[toType] || AGENT_TYPES.system;

  // For feed mail, show both from and to with colors
  const fromTo = isFeedMail && mail.to
    ? `<span class="agent-badge" style="--agent-color: ${fromConfig.color}">
         <span class="material-icons">${fromConfig.icon}</span>
         ${formatAgentName(mail.from)}
       </span>
       <span class="mail-arrow">→</span>
       <span class="agent-badge" style="--agent-color: ${toConfig.color}">
         <span class="material-icons">${toConfig.icon}</span>
         ${formatAgentName(mail.to)}
       </span>`
    : `<span class="agent-badge" style="--agent-color: ${fromConfig.color}">
         <span class="material-icons">${fromConfig.icon}</span>
         ${escapeHtml(mail.from || 'System')}
       </span>`;

  return `
    <div class="mail-item ${isUnread ? 'unread' : ''} ${isFeedMail ? 'feed-mail' : ''} animate-spawn stagger-${Math.min(index, 6)}"
         data-mail-id="${mail.id}"
         style="--from-color: ${fromConfig.color}">
      <div class="mail-status">
        <span class="material-icons" style="color: ${fromConfig.color}">${fromConfig.icon}</span>
      </div>

      <div class="mail-content">
        <div class="mail-header">
          <span class="mail-from">${fromTo}</span>
          <span class="mail-time">${formatTime(mail.timestamp)}</span>
        </div>
        <div class="mail-subject ${isUnread ? 'unread' : ''}">${escapeHtml(mail.subject || '(No Subject)')}</div>
        <div class="mail-preview">${escapeHtml(truncate(mail.message || mail.body || '', 80))}</div>

        ${mail.tags?.length ? `
          <div class="mail-tags">
            ${mail.tags.map(tag => `
              <span class="mail-tag">${escapeHtml(tag)}</span>
            `).join('')}
          </div>
        ` : ''}
      </div>

      ${!isFeedMail ? `
        <div class="mail-actions">
          <button class="btn btn-icon btn-sm" title="Archive" data-action="archive">
            <span class="material-icons">archive</span>
          </button>
          <button class="btn btn-icon btn-sm" title="Delete" data-action="delete">
            <span class="material-icons">delete</span>
          </button>
        </div>
      ` : ''}
    </div>
  `;
}

/**
 * Format agent name for display (shorten long paths)
 */
function formatAgent(name) {
  if (!name) return 'unknown';
  // Shorten paths like "hytopia-map-compression/polecats/slit" to "slit"
  // or "hytopia-map-compression/witness" to "witness"
  const parts = name.split('/');
  if (parts.length > 1) {
    return escapeHtml(parts[parts.length - 1]);
  }
  return escapeHtml(name);
}

/**
 * Show mail detail modal
 */
function showMailDetail(mailId, mail) {
  if (!mail) return;

  // Mark as read
  const event = new CustomEvent('mail:read', { detail: { mailId } });
  document.dispatchEvent(event);

  // Show modal
  const modalEvent = new CustomEvent('mail:detail', {
    detail: { mailId, mail }
  });
  document.dispatchEvent(modalEvent);
}

/**
 * Format timestamp for display
 */
function formatTime(timestamp) {
  if (!timestamp) return '';

  const date = new Date(timestamp);
  const now = new Date();
  const diff = now - date;

  // Today - show time
  if (date.toDateString() === now.toDateString()) {
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }

  // This week - show day
  if (diff < 7 * 86400000) {
    return date.toLocaleDateString([], { weekday: 'short' });
  }

  // Older - show date
  return date.toLocaleDateString([], { month: 'short', day: 'numeric' });
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
