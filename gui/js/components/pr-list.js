/**
 * Gas Town GUI - GitHub Pull Requests Component
 *
 * Displays GitHub PRs across all connected rigs.
 */

import { api } from '../api.js';
import { showToast } from './toast.js';
import { formatRelativeTime } from '../utils/formatting.js';

// State
let currentState = 'open';
let prs = [];

/**
 * Initialize the PR list component
 */
export function initPRList() {
  // Set up filter tabs
  const filterTabs = document.querySelectorAll('.pr-state-tab');
  filterTabs.forEach(tab => {
    tab.addEventListener('click', () => {
      const state = tab.dataset.state;
      setActiveState(state);
    });
  });

  // Set up refresh button
  const refreshBtn = document.getElementById('pr-refresh');
  if (refreshBtn) {
    refreshBtn.addEventListener('click', () => loadPRs());
  }
}

/**
 * Set active filter state
 */
function setActiveState(state) {
  currentState = state;

  // Update tabs
  document.querySelectorAll('.pr-state-tab').forEach(tab => {
    tab.classList.toggle('active', tab.dataset.state === state);
  });

  loadPRs();
}

/**
 * Load PRs from API
 */
export async function loadPRs() {
  const container = document.getElementById('pr-list-container');
  if (!container) return;

  container.innerHTML = '<div class="loading-state"><span class="loading-spinner"></span> Loading PRs...</div>';

  try {
    prs = await api.getGitHubPRs(currentState);

    if (prs.length === 0) {
      container.innerHTML = `
        <div class="empty-state">
          <span class="material-icons">merge_type</span>
          <p>No ${currentState} pull requests found</p>
        </div>
      `;
      return;
    }

    container.innerHTML = '';
    prs.forEach(pr => {
      container.appendChild(createPRCard(pr));
    });
  } catch (err) {
    console.error('[PRList] Failed to load PRs:', err);
    container.innerHTML = `
      <div class="error-state">
        <span class="material-icons">error</span>
        <p>Failed to load PRs</p>
        <small>${escapeHtml(err.message)}</small>
      </div>
    `;
  }
}

/**
 * Create a PR card element
 */
function createPRCard(pr) {
  const card = document.createElement('div');
  card.className = 'pr-card';
  card.dataset.prNumber = pr.number;
  card.dataset.repo = pr.repo;

  const stateIcon = getStateIcon(pr);
  const reviewIcon = getReviewIcon(pr.reviewDecision);
  const authorName = pr.author?.login || 'unknown';
  const timeAgo = formatRelativeTime(pr.updatedAt);

  card.innerHTML = `
    <div class="pr-icon ${pr.state.toLowerCase()} ${pr.isDraft ? 'draft' : ''}">
      <span class="material-icons">${stateIcon}</span>
    </div>
    <div class="pr-content">
      <div class="pr-header">
        <span class="pr-number">#${pr.number}</span>
        <span class="pr-title">${escapeHtml(pr.title)}</span>
        ${pr.isDraft ? '<span class="pr-draft-badge">Draft</span>' : ''}
      </div>
      <div class="pr-meta">
        <span class="pr-repo" title="Repository">
          <span class="material-icons">folder</span>
          ${escapeHtml(pr.rig)}
        </span>
        <span class="pr-branch" title="Branch">
          <span class="material-icons">account_tree</span>
          ${escapeHtml(pr.headRefName)}
        </span>
        <span class="pr-author" title="Author">
          <span class="material-icons">person</span>
          ${escapeHtml(authorName)}
        </span>
        <span class="pr-time" title="Last updated">
          <span class="material-icons">schedule</span>
          ${timeAgo}
        </span>
        ${reviewIcon ? `<span class="pr-review ${pr.reviewDecision?.toLowerCase()}" title="Review status">${reviewIcon}</span>` : ''}
      </div>
    </div>
    <div class="pr-actions">
      <a href="${pr.url}" target="_blank" class="btn btn-sm btn-icon" title="Open in GitHub">
        <span class="material-icons">open_in_new</span>
      </a>
    </div>
  `;

  // Click to show details
  card.addEventListener('click', (e) => {
    if (!e.target.closest('.pr-actions')) {
      showPRDetail(pr);
    }
  });

  return card;
}

/**
 * Get icon for PR state
 */
function getStateIcon(pr) {
  if (pr.isDraft) return 'edit_note';
  switch (pr.state?.toUpperCase()) {
    case 'OPEN': return 'merge_type';
    case 'CLOSED': return 'close';
    case 'MERGED': return 'merge';
    default: return 'merge_type';
  }
}

/**
 * Get review status icon
 */
function getReviewIcon(reviewDecision) {
  switch (reviewDecision?.toUpperCase()) {
    case 'APPROVED':
      return '<span class="material-icons approved">check_circle</span>';
    case 'CHANGES_REQUESTED':
      return '<span class="material-icons changes-requested">change_circle</span>';
    case 'REVIEW_REQUIRED':
      return '<span class="material-icons review-required">pending</span>';
    default:
      return null;
  }
}

/**
 * Show PR detail modal
 */
async function showPRDetail(pr) {
  // For now, just open in GitHub
  // Could expand to show a detail modal with diffs, comments, etc.
  window.open(pr.url, '_blank');
}

/**
 * Render PR list (called from app.js after data load)
 */
export function renderPRList(prData) {
  prs = prData || [];

  const container = document.getElementById('pr-list-container');
  if (!container) return;

  if (prs.length === 0) {
    container.innerHTML = `
      <div class="empty-state">
        <span class="material-icons">merge_type</span>
        <p>No pull requests found</p>
      </div>
    `;
    return;
  }

  container.innerHTML = '';
  prs.forEach(pr => {
    container.appendChild(createPRCard(pr));
  });
}

// Utility
function escapeHtml(str) {
  if (!str) return '';
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}
