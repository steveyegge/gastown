/**
 * Gas Town GUI - Work List Component
 *
 * Renders the list of beads (tasks/work items) with status and completion info.
 */

import { api } from '../api.js';
import { showToast } from './toast.js';
import { escapeHtml, truncate } from '../utils/html.js';

// Issue type icons
const TYPE_ICONS = {
  task: 'task_alt',
  bug: 'bug_report',
  feature: 'star',
  message: 'mail',
  convoy: 'local_shipping',
  agent: 'smart_toy',
  chore: 'build',
  epic: 'flag',
};

// Status configuration
const STATUS_CONFIG = {
  open: { icon: 'radio_button_unchecked', class: 'status-open', label: 'Open' },
  closed: { icon: 'check_circle', class: 'status-closed', label: 'Completed' },
  'in-progress': { icon: 'pending', class: 'status-progress', label: 'In Progress' },
  in_progress: { icon: 'pending', class: 'status-progress', label: 'In Progress' },
  blocked: { icon: 'block', class: 'status-blocked', label: 'Blocked' },
};

// GitHub repo mapping for known rigs
// Format: { rigName: 'org/repo' } or { beadPrefix: 'org/repo' }
// NOTE: This can be configured/extended by the user
const GITHUB_REPOS = {
  // Rig name → GitHub org/repo
  'hytopia-map-compression': 'web3dev1337/hytopia-map-compression',
  'testproject': null, // No GitHub URL known

  // Bead ID prefixes (optional - if beads use different prefixes)
  // 'hq': 'web3dev1337/hytopia-map-compression',
};

/**
 * Get GitHub repo for a bead based on its ID or rig
 */
function getGitHubRepoForBead(beadId) {
  if (!beadId) return null;

  // Try to match by bead prefix (e.g., "hq-123" → "hq")
  const prefixMatch = beadId.match(/^([a-z]+)-/i);
  if (prefixMatch) {
    const prefix = prefixMatch[1].toLowerCase();
    if (GITHUB_REPOS[prefix]) return GITHUB_REPOS[prefix];
  }

  // Try to match by rig name directly
  for (const [key, repo] of Object.entries(GITHUB_REPOS)) {
    if (repo && beadId.toLowerCase().includes(key.toLowerCase())) {
      return repo;
    }
  }

  // Default: try the first available repo
  for (const repo of Object.values(GITHUB_REPOS)) {
    if (repo) return repo;
  }

  return null;
}

/**
 * Parse close_reason for commit/PR references and make them clickable
 */
function parseCloseReason(text, beadId) {
  if (!text) return '';

  let result = escapeHtml(text);
  const repo = getGitHubRepoForBead(beadId);

  // Replace commit references with links
  result = result.replace(/commit\s+([a-f0-9]{7,40})/gi, (match, hash) => {
    if (repo) {
      // Link to actual GitHub commit
      const url = `https://github.com/${repo}/commit/${hash}`;
      return `<a href="${url}" target="_blank" class="commit-link" data-commit="${hash}" title="View on GitHub">${match}</a>`;
    } else {
      // Fallback: copy to clipboard
      return `<a href="#" class="commit-link commit-copy" data-commit="${hash}" title="Click to copy">${match}</a>`;
    }
  });

  // Replace PR references with links
  result = result.replace(/PR\s*#?(\d+)/gi, (match, num) => {
    if (repo) {
      // Link to actual GitHub PR
      const url = `https://github.com/${repo}/pull/${num}`;
      return `<a href="${url}" target="_blank" class="pr-link" data-pr="${num}" title="View on GitHub">${match}</a>`;
    } else {
      // Fallback: copy to clipboard
      return `<a href="#" class="pr-link pr-copy" data-pr="${num}" title="Click to copy">${match}</a>`;
    }
  });

  return result;
}

/**
 * Render the work list
 * @param {HTMLElement} container - The list container
 * @param {Array} beads - Array of bead objects
 */
export function renderWorkList(container, beads) {
  if (!container) return;

  // Show all work types except internal ones (messages, convoys, agents)
  const hiddenTypes = ['message', 'convoy', 'agent', 'gate', 'role', 'event', 'slot'];
  const tasks = beads.filter(b => !hiddenTypes.includes(b.issue_type));

  if (!tasks || tasks.length === 0) {
    container.innerHTML = `
      <div class="empty-state">
        <span class="material-icons empty-icon">task_alt</span>
        <h3>No Work Found</h3>
        <p>Create a new task to track work</p>
      </div>
    `;
    return;
  }

  container.innerHTML = tasks.map((bead, index) => renderBeadCard(bead, index)).join('');

  // Add click handlers for cards
  container.querySelectorAll('.bead-card').forEach(card => {
    card.addEventListener('click', (e) => {
      // Don't trigger if clicking a link
      if (e.target.closest('a')) return;

      const beadId = card.dataset.beadId;
      showBeadDetail(beadId, beads.find(b => b.id === beadId));
    });
  });

  // Add click handlers for copy-only links (no GitHub repo configured)
  container.querySelectorAll('.commit-copy').forEach(link => {
    link.addEventListener('click', (e) => {
      e.preventDefault();
      e.stopPropagation();
      const hash = link.dataset.commit;
      navigator.clipboard.writeText(hash).then(() => {
        showCopyToast(`Copied: ${hash}`);
      });
    });
  });

  container.querySelectorAll('.pr-copy').forEach(link => {
    link.addEventListener('click', (e) => {
      e.preventDefault();
      e.stopPropagation();
      const pr = link.dataset.pr;
      navigator.clipboard.writeText(`#${pr}`).then(() => {
        showCopyToast(`Copied: PR #${pr}`);
      });
    });
  });

  // For links with actual GitHub URLs, just prevent card click propagation
  container.querySelectorAll('.commit-link:not(.commit-copy), .pr-link:not(.pr-copy)').forEach(link => {
    link.addEventListener('click', (e) => {
      e.stopPropagation(); // Don't trigger card click, but let the link navigate
    });
  });

  // Add action button handlers
  container.querySelectorAll('.bead-actions [data-action]').forEach(btn => {
    btn.addEventListener('click', async (e) => {
      e.stopPropagation();
      const action = btn.dataset.action;
      const beadId = btn.dataset.beadId;
      await handleWorkAction(action, beadId, btn);
    });
  });
}

/**
 * Handle work action (done, park, release, reassign)
 */
async function handleWorkAction(action, beadId, btn) {
  const originalIcon = btn.innerHTML;
  btn.innerHTML = '<span class="material-icons spinning">sync</span>';
  btn.disabled = true;

  try {
    let result;
    switch (action) {
      case 'done':
        const summary = prompt('Enter completion summary (optional):');
        if (summary === null) {
          // User cancelled
          btn.innerHTML = originalIcon;
          btn.disabled = false;
          return;
        }
        result = await api.markWorkDone(beadId, summary || 'Completed via GUI');
        break;

      case 'park':
        const reason = prompt('Enter reason for parking:');
        if (!reason) {
          btn.innerHTML = originalIcon;
          btn.disabled = false;
          return;
        }
        result = await api.parkWork(beadId, reason);
        break;

      case 'release':
        if (!confirm('Release this work item?')) {
          btn.innerHTML = originalIcon;
          btn.disabled = false;
          return;
        }
        result = await api.releaseWork(beadId);
        break;

      case 'reassign':
        const target = prompt('Enter target agent address:');
        if (!target) {
          btn.innerHTML = originalIcon;
          btn.disabled = false;
          return;
        }
        result = await api.reassignWork(beadId, target);
        break;
    }

    if (result && result.success) {
      showToast(`Work ${action === 'done' ? 'completed' : action + 'ed'}: ${beadId}`, 'success');
      // Trigger work list refresh
      document.dispatchEvent(new CustomEvent('work:refresh'));
    } else if (result) {
      showToast(`Failed: ${result.error || 'Unknown error'}`, 'error');
    }
  } catch (err) {
    showToast(`Error: ${err.message}`, 'error');
  } finally {
    btn.innerHTML = originalIcon;
    btn.disabled = false;
  }
}

/**
 * Render a single bead card
 */
function renderBeadCard(bead, index) {
  const status = bead.status || 'open';
  const statusConfig = STATUS_CONFIG[status] || STATUS_CONFIG.open;
  const typeIcon = TYPE_ICONS[bead.issue_type] || 'assignment';
  const assignee = bead.assignee ? bead.assignee.split('/').pop() : null;

  return `
    <div class="bead-card ${statusConfig.class} animate-spawn stagger-${Math.min(index, 6)}"
         data-bead-id="${bead.id}">
      <div class="bead-header">
        <div class="bead-status">
          <span class="material-icons">${statusConfig.icon}</span>
        </div>
        <div class="bead-info">
          <h3 class="bead-title">${escapeHtml(bead.title)}</h3>
          <div class="bead-meta">
            <span class="bead-id">#${bead.id}</span>
            <span class="bead-type">
              <span class="material-icons">${typeIcon}</span>
              ${bead.issue_type || 'task'}
            </span>
            ${assignee ? `
              <span class="bead-assignee">
                <span class="material-icons">person</span>
                ${escapeHtml(assignee)}
              </span>
            ` : ''}
          </div>
        </div>
        <div class="bead-priority priority-${bead.priority || 2}">
          P${bead.priority || 2}
        </div>
      </div>

      ${bead.close_reason ? `
        <div class="bead-result">
          <span class="material-icons">check</span>
          <span class="result-text">${parseCloseReason(truncate(bead.close_reason, 150), bead.id)}</span>
        </div>
      ` : ''}

      <div class="bead-footer">
        <div class="bead-time">
          ${bead.closed_at ? `Completed ${formatTime(bead.closed_at)}` : `Created ${formatTime(bead.created_at)}`}
        </div>
        ${status !== 'closed' ? `
          <div class="bead-actions">
            <button class="btn btn-xs btn-success-ghost" data-action="done" data-bead-id="${bead.id}" title="Mark as done">
              <span class="material-icons">check_circle</span>
            </button>
            <button class="btn btn-xs btn-ghost" data-action="park" data-bead-id="${bead.id}" title="Park work">
              <span class="material-icons">pause_circle</span>
            </button>
            <button class="btn btn-xs btn-ghost" data-action="release" data-bead-id="${bead.id}" title="Release work">
              <span class="material-icons">cancel</span>
            </button>
            <button class="btn btn-xs btn-ghost" data-action="reassign" data-bead-id="${bead.id}" title="Reassign work">
              <span class="material-icons">person_add</span>
            </button>
          </div>
        ` : ''}
      </div>
    </div>
  `;
}

/**
 * Show bead detail modal
 */
function showBeadDetail(beadId, bead) {
  const event = new CustomEvent('bead:detail', { detail: { beadId, bead } });
  document.dispatchEvent(event);
}

/**
 * Show a small toast when copying
 */
function showCopyToast(message) {
  // Try to use the existing toast system
  const event = new CustomEvent('toast:show', { detail: { message, type: 'success', duration: 2000 } });
  document.dispatchEvent(event);

  // Fallback: create a simple toast if no handler
  setTimeout(() => {
    const existingToast = document.querySelector('.copy-toast');
    if (existingToast) existingToast.remove();

    const toast = document.createElement('div');
    toast.className = 'copy-toast';
    toast.textContent = message;
    toast.style.cssText = `
      position: fixed;
      bottom: 20px;
      left: 50%;
      transform: translateX(-50%);
      background: var(--bg-elevated, #333);
      color: var(--text-primary, #fff);
      padding: 8px 16px;
      border-radius: 4px;
      font-size: 13px;
      z-index: 9999;
      animation: fadeInUp 0.2s ease;
    `;
    document.body.appendChild(toast);
    setTimeout(() => toast.remove(), 2000);
  }, 0);
}

function formatTime(timestamp) {
  if (!timestamp) return '';
  const date = new Date(timestamp);
  const now = new Date();
  const diff = now - date;

  if (diff < 60000) return 'just now';
  if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`;
  if (diff < 86400000) return `${Math.floor(diff / 3600000)}h ago`;
  return date.toLocaleDateString();
}
