/**
 * Gas Town GUI - Work List Component
 *
 * Renders the list of beads (tasks/work items) with status and completion info.
 */

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

// GitHub repo detection from close_reason patterns
const GITHUB_PATTERNS = [
  // "commit a8f756e" or "commit a8f756e → CATALOG.md"
  /commit\s+([a-f0-9]{7,40})/gi,
  // "PR #123" or "pr #123"
  /pr\s*#?(\d+)/gi,
  // Full GitHub URLs
  /github\.com\/([^/]+)\/([^/]+)\/(?:commit|pull)\/([a-f0-9]+|\d+)/gi,
];

/**
 * Parse close_reason for commit/PR references and make them clickable
 */
function parseCloseReason(text, beadId) {
  if (!text) return '';

  let result = escapeHtml(text);

  // Try to detect the repo from context (beads often have repo in id like "hq-xxx")
  // For now, we'll make commits/PRs clickable with a generic search link

  // Replace commit references with links
  result = result.replace(/commit\s+([a-f0-9]{7,40})/gi, (match, hash) => {
    // Make it a clickable link that copies the hash or searches
    return `<a href="#" class="commit-link" data-commit="${hash}" title="Click to copy commit hash">${match}</a>`;
  });

  // Replace PR references with links
  result = result.replace(/PR\s*#?(\d+)/gi, (match, num) => {
    return `<a href="#" class="pr-link" data-pr="${num}" title="Pull Request #${num}">${match}</a>`;
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

  // Add click handlers for commit links (copy to clipboard)
  container.querySelectorAll('.commit-link').forEach(link => {
    link.addEventListener('click', (e) => {
      e.preventDefault();
      e.stopPropagation();
      const hash = link.dataset.commit;
      navigator.clipboard.writeText(hash).then(() => {
        showCopyToast(`Copied: ${hash}`);
      });
    });
  });

  // Add click handlers for PR links
  container.querySelectorAll('.pr-link').forEach(link => {
    link.addEventListener('click', (e) => {
      e.preventDefault();
      e.stopPropagation();
      const pr = link.dataset.pr;
      navigator.clipboard.writeText(`#${pr}`).then(() => {
        showCopyToast(`Copied: PR #${pr}`);
      });
    });
  });
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
