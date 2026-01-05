/**
 * Gas Town GUI - GitHub Issues List Component
 *
 * Renders GitHub issues from connected repositories.
 */

import { api } from '../api.js';
import { showToast } from './toast.js';
import { formatRelativeTime } from '../utils/formatting.js';

let container = null;
let issues = [];
let currentState = 'open';

/**
 * Initialize the issue list component
 */
export function initIssueList() {
  container = document.getElementById('issue-list-container');
  if (!container) return;

  // State tabs
  document.querySelectorAll('.issue-state-tab').forEach(tab => {
    tab.addEventListener('click', () => {
      const state = tab.dataset.state;
      setActiveState(state);
      loadIssues();
    });
  });

  // Refresh button
  const refreshBtn = document.getElementById('issue-refresh');
  if (refreshBtn) {
    refreshBtn.addEventListener('click', () => loadIssues());
  }
}

/**
 * Set active state tab
 */
function setActiveState(state) {
  currentState = state;
  document.querySelectorAll('.issue-state-tab').forEach(tab => {
    tab.classList.toggle('active', tab.dataset.state === state);
  });
}

/**
 * Load issues from API
 */
export async function loadIssues() {
  if (!container) {
    container = document.getElementById('issue-list-container');
  }
  if (!container) return;

  container.innerHTML = '<div class="loading-state"><span class="loading-spinner"></span> Loading issues...</div>';

  try {
    issues = await api.getGitHubIssues(currentState);
    renderIssues();
  } catch (err) {
    console.error('[Issues] Load error:', err);
    container.innerHTML = `
      <div class="error-state">
        <span class="material-icons">error_outline</span>
        <p>Failed to load issues: ${escapeHtml(err.message)}</p>
        <button class="btn btn-secondary" onclick="window.location.reload()">Retry</button>
      </div>
    `;
  }
}

/**
 * Render issue cards
 */
function renderIssues() {
  if (!issues || issues.length === 0) {
    container.innerHTML = `
      <div class="empty-state">
        <span class="material-icons empty-icon">bug_report</span>
        <h3>No Issues</h3>
        <p>No ${currentState} issues found in connected repositories</p>
      </div>
    `;
    return;
  }

  container.innerHTML = '';
  issues.forEach((issue, index) => {
    const card = createIssueCard(issue, index);
    container.appendChild(card);
  });
}

/**
 * Create an issue card element
 */
function createIssueCard(issue, index) {
  const card = document.createElement('div');
  card.className = `issue-card issue-${issue.state} animate-spawn stagger-${Math.min(index, 6)}`;
  card.dataset.issueNumber = issue.number;
  card.dataset.repo = issue.repo;

  const labels = (issue.labels || []).map(l =>
    `<span class="issue-label" style="background: #${l.color || '6c757d'}">${escapeHtml(l.name)}</span>`
  ).join('');

  const assignees = (issue.assignees || []).map(a => a.login).join(', ');

  card.innerHTML = `
    <div class="issue-header">
      <div class="issue-state-icon">
        <span class="material-icons ${issue.state === 'open' ? 'text-success' : 'text-muted'}">
          ${issue.state === 'open' ? 'radio_button_unchecked' : 'check_circle'}
        </span>
      </div>
      <div class="issue-info">
        <a href="${issue.url}" target="_blank" class="issue-title">${escapeHtml(issue.title)}</a>
        <div class="issue-meta">
          <span class="issue-number">#${issue.number}</span>
          <span class="issue-repo">${escapeHtml(issue.repo)}</span>
          ${issue.rig ? `<span class="issue-rig">📁 ${escapeHtml(issue.rig)}</span>` : ''}
        </div>
      </div>
    </div>
    ${labels ? `<div class="issue-labels">${labels}</div>` : ''}
    <div class="issue-footer">
      <div class="issue-author">
        <span class="material-icons">person</span>
        ${escapeHtml(issue.author?.login || 'Unknown')}
      </div>
      ${assignees ? `
        <div class="issue-assignees">
          <span class="material-icons">assignment_ind</span>
          ${escapeHtml(assignees)}
        </div>
      ` : ''}
      <div class="issue-time">
        ${formatRelativeTime(issue.updatedAt)}
      </div>
      <div class="issue-actions">
        <button class="btn btn-xs btn-ghost" data-action="sling" title="Sling to worker">
          <span class="material-icons">send</span>
        </button>
        <a href="${issue.url}" target="_blank" class="btn btn-xs btn-ghost" title="Open on GitHub">
          <span class="material-icons">open_in_new</span>
        </a>
      </div>
    </div>
  `;

  // Add sling handler
  const slingBtn = card.querySelector('[data-action="sling"]');
  slingBtn.addEventListener('click', (e) => {
    e.stopPropagation();
    openSlingModal(issue);
  });

  return card;
}

/**
 * Open sling modal with issue pre-filled
 */
function openSlingModal(issue) {
  const modal = document.getElementById('sling-modal');
  const beadInput = modal?.querySelector('#sling-bead');

  if (modal && beadInput) {
    // Pre-fill with issue reference
    beadInput.value = `gh:${issue.repo}#${issue.number}`;

    document.getElementById('modal-overlay').classList.remove('hidden');
    modal.classList.remove('hidden');
  }
}

/**
 * Escape HTML to prevent XSS
 */
function escapeHtml(str) {
  if (!str) return '';
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}
