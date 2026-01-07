/**
 * Gas Town GUI - Activity Feed Component
 *
 * Renders real-time activity events from the Gastown system.
 */

import { AGENT_TYPES, getAgentConfig, formatAgentName } from '../shared/agent-types.js';

// Event type configuration (uses shared agent colors where applicable)
const EVENT_CONFIG = {
  convoy_created: { icon: 'add_circle', color: '#22c55e', label: 'Convoy Created' },
  convoy_updated: { icon: 'update', color: '#3b82f6', label: 'Convoy Updated' },
  convoy_complete: { icon: 'check_circle', color: '#22c55e', label: 'Convoy Complete' },
  work_slung: { icon: 'send', color: '#a855f7', label: 'Work Dispatched' },
  work_complete: { icon: 'task_alt', color: '#22c55e', label: 'Work Complete' },
  work_failed: { icon: 'error', color: '#ef4444', label: 'Work Failed' },
  agent_spawned: { icon: 'person_add', color: AGENT_TYPES.polecat.color, label: 'Agent Spawned' },
  agent_despawned: { icon: 'person_remove', color: '#6b7280', label: 'Agent Despawned' },
  bead_created: { icon: 'bubble_chart', color: '#f59e0b', label: 'Bead Created' },
  bead_updated: { icon: 'edit', color: '#3b82f6', label: 'Bead Updated' },
  mail: { icon: 'mail', color: '#ec4899', label: 'Mail Sent' },
  mail_received: { icon: 'mail', color: '#ec4899', label: 'Mail Received' },
  system: { icon: 'info', color: '#6b7280', label: 'System' },
  error: { icon: 'error_outline', color: '#ef4444', label: 'Error' },
};

/**
 * Render the activity feed
 * @param {HTMLElement} container - The feed container
 * @param {Array} events - Array of event objects
 */
export function renderActivityFeed(container, events) {
  if (!container) return;

  if (!events || events.length === 0) {
    container.innerHTML = `
      <div class="feed-empty">
        <span class="material-icons">notifications_none</span>
        <p>No activity yet</p>
      </div>
    `;
    return;
  }

  // Check if we're adding new events (for animation)
  const existingIds = new Set(
    Array.from(container.querySelectorAll('.feed-item')).map(el => el.dataset.eventId)
  );

  const html = events.map((event, index) => {
    const isNew = !existingIds.has(event.id);
    return renderFeedItem(event, index, isNew);
  }).join('');

  container.innerHTML = html;
}

/**
 * Add a single event to the feed (for real-time updates)
 * @param {HTMLElement} container - The feed container
 * @param {Object} event - The event to add
 */
export function addEventToFeed(container, event) {
  if (!container) return;

  // Remove empty state if present
  const emptyState = container.querySelector('.feed-empty');
  if (emptyState) {
    emptyState.remove();
  }

  // Create new event element
  const div = document.createElement('div');
  div.innerHTML = renderFeedItem(event, 0, true);
  const newItem = div.firstElementChild;

  // Insert at the beginning with animation
  if (container.firstChild) {
    container.insertBefore(newItem, container.firstChild);
  } else {
    container.appendChild(newItem);
  }

  // Trigger animation
  requestAnimationFrame(() => {
    newItem.classList.add('animate-in');
  });

  // Limit items in DOM (keep last 100)
  const items = container.querySelectorAll('.feed-item');
  if (items.length > 100) {
    for (let i = 100; i < items.length; i++) {
      items[i].remove();
    }
  }
}

/**
 * Render a single feed item
 */
function renderFeedItem(event, index, isNew) {
  const type = event.type || 'system';
  const config = EVENT_CONFIG[type] || EVENT_CONFIG.system;

  return `
    <div class="feed-item ${isNew ? 'new-event' : ''}"
         data-event-id="${event.id || index}"
         style="--event-color: ${config.color}">
      <div class="feed-icon">
        <span class="material-icons" style="color: ${config.color}">${config.icon}</span>
      </div>
      <div class="feed-content">
        <div class="feed-header">
          <span class="feed-type">${config.label}</span>
          <span class="feed-time">${formatTime(event.timestamp)}</span>
        </div>
        <div class="feed-message">${formatMessage(event)}</div>
        ${event.details ? `
          <div class="feed-details">${escapeHtml(event.details)}</div>
        ` : ''}
        ${event.convoy_id ? `
          <div class="feed-meta">
            <span class="feed-tag">
              <span class="material-icons">local_shipping</span>
              ${event.convoy_id.slice(0, 8)}
            </span>
          </div>
        ` : ''}
      </div>
    </div>
  `;
}

/**
 * Format event message based on type
 */
function formatMessage(event) {
  const msg = event.message || event.summary || event.description || '';

  // Add special formatting for certain event types
  switch (event.type) {
    case 'work_slung':
      return `Slung <strong>${escapeHtml(event.bead || 'work')}</strong> to ${formatAgentBadge(event.target)}`;

    case 'agent_spawned':
      return `${formatAgentBadge(event.agent_id || event.agent_name, event.role)} joined`;

    case 'bead_created':
      return `Created bead <strong>${escapeHtml(event.bead_id || 'unknown')}</strong>`;

    case 'convoy_created':
      return `Convoy <strong>${escapeHtml(event.convoy_name || event.convoy_id || 'unknown')}</strong> created`;

    case 'mail':
    case 'mail_received':
      const fromConfig = getAgentConfig(event.actor || event.from);
      const toConfig = getAgentConfig(event.payload?.to || event.to);
      return `${formatAgentBadge(event.actor || event.from)} → ${formatAgentBadge(event.payload?.to || event.to)}: ${escapeHtml(truncate(event.payload?.subject || event.subject || msg, 40))}`;

    default:
      // For events with actor, show the actor badge
      if (event.actor) {
        return `${formatAgentBadge(event.actor)}: ${escapeHtml(msg)}`;
      }
      return escapeHtml(msg);
  }
}

/**
 * Create a small inline agent badge for feed items
 */
function formatAgentBadge(agentPath, role = null) {
  if (!agentPath) return '<span class="feed-agent">unknown</span>';
  const config = getAgentConfig(agentPath, role);
  const name = formatAgentName(agentPath);
  return `<span class="feed-agent" style="color: ${config.color}"><span class="material-icons" style="font-size: 12px">${config.icon}</span> ${escapeHtml(name)}</span>`;
}

/**
 * Format timestamp for display
 */
function formatTime(timestamp) {
  if (!timestamp) return '';

  const date = new Date(timestamp);
  const now = new Date();
  const diff = now - date;

  // Less than 1 minute
  if (diff < 60000) {
    const seconds = Math.floor(diff / 1000);
    return seconds <= 5 ? 'Just now' : `${seconds}s ago`;
  }

  // Less than 1 hour
  if (diff < 3600000) {
    return `${Math.floor(diff / 60000)}m ago`;
  }

  // Less than 24 hours - show time
  if (diff < 86400000) {
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }

  // Otherwise show date
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
