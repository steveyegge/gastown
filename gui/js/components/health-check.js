/**
 * Gas Town GUI - Health Check Component
 *
 * Displays system health diagnostics from gt doctor command.
 */

import { api } from '../api.js';
import { showToast } from './toast.js';
import { escapeHtml } from '../utils/html.js';

let container = null;
let refreshBtn = null;
let currentFilter = 'all';
let lastResults = null;

/**
 * Initialize health check component
 */
export function initHealthCheck() {
  container = document.getElementById('health-check-container');
  refreshBtn = document.getElementById('health-refresh');

  if (refreshBtn) {
    refreshBtn.addEventListener('click', () => {
      loadHealthCheck();
    });
  }
}

/**
 * Load health check data
 */
export async function loadHealthCheck() {
  if (!container) return;

  // Show loading state
  container.innerHTML = `
    <div class="health-loading">
      <span class="loading-spinner"></span>
      <p>Running health diagnostics... (this may take 15-20 seconds)</p>
    </div>
  `;

  if (refreshBtn) {
    refreshBtn.disabled = true;
    refreshBtn.innerHTML = '<span class="material-icons spinning">sync</span> Running...';
  }

  try {
    const result = await api.runDoctor({ refresh: true });
    lastResults = result;
    renderHealthResults(result);
  } catch (err) {
    showToast(`Failed to run health check: ${err.message}`, 'error');
    container.innerHTML = `
      <div class="health-error">
        <span class="material-icons">error</span>
        <h3>Health Check Failed</h3>
        <p>${escapeHtml(err.message)}</p>
        <button class="btn btn-secondary" onclick="document.getElementById('health-refresh').click()">
          <span class="material-icons">refresh</span>
          Retry
        </button>
      </div>
    `;
  } finally {
    if (refreshBtn) {
      refreshBtn.disabled = false;
      refreshBtn.innerHTML = '<span class="material-icons">refresh</span> Run Doctor';
    }
  }
}

/**
 * Render health check results
 */
function renderHealthResults(data) {
  if (!container) return;

  const checks = data.checks || [];
  const summary = data.summary || {};

  // Calculate counts
  let passCount = 0, warnCount = 0, failCount = 0;
  checks.forEach(check => {
    const status = (check.status || '').toLowerCase();
    if (status === 'pass') passCount++;
    else if (status === 'warn') warnCount++;
    else if (status === 'fail') failCount++;
  });

  const overallStatus = failCount > 0 ? 'fail' : (warnCount > 0 ? 'warn' : 'pass');
  const statusLabels = {
    pass: 'All Systems Healthy',
    warn: 'Some Warnings',
    fail: 'Issues Detected'
  };
  const statusIcons = {
    pass: 'check_circle',
    warn: 'warning',
    fail: 'error'
  };

  // Check if any checks have fix commands
  const hasFixableIssues = checks.some(c => c.fix && (c.status === 'warn' || c.status === 'fail'));

  container.innerHTML = `
    <div class="health-summary health-${overallStatus}">
      <div class="health-summary-icon">
        <span class="material-icons">${statusIcons[overallStatus]}</span>
      </div>
      <div class="health-summary-info">
        <h2>${statusLabels[overallStatus]}</h2>
        <div class="health-summary-stats">
          <span class="health-stat pass">
            <span class="material-icons">check_circle</span>
            ${passCount} Passed
          </span>
          <span class="health-stat warn">
            <span class="material-icons">warning</span>
            ${warnCount} Warnings
          </span>
          <span class="health-stat fail">
            <span class="material-icons">error</span>
            ${failCount} Errors
          </span>
        </div>
      </div>
      <div class="health-summary-actions">
        ${hasFixableIssues ? `
          <button class="btn btn-primary" id="health-fix-all">
            <span class="material-icons">build</span>
            Fix All Issues
          </button>
        ` : ''}
      </div>
    </div>

    <div class="health-filters">
      <button class="health-filter-btn ${currentFilter === 'all' ? 'active' : ''}" data-filter="all">
        All (${checks.length})
      </button>
      <button class="health-filter-btn ${currentFilter === 'fail' ? 'active' : ''}" data-filter="fail">
        <span class="material-icons">error</span>
        Errors (${failCount})
      </button>
      <button class="health-filter-btn ${currentFilter === 'warn' ? 'active' : ''}" data-filter="warn">
        <span class="material-icons">warning</span>
        Warnings (${warnCount})
      </button>
      <button class="health-filter-btn ${currentFilter === 'pass' ? 'active' : ''}" data-filter="pass">
        <span class="material-icons">check_circle</span>
        Passed (${passCount})
      </button>
    </div>

    <div class="health-checks-scroll">
      <div class="health-checks" id="health-checks-list">
        ${renderFilteredChecks(checks)}
      </div>
    </div>

    <div class="health-footer">
      Last checked: ${new Date().toLocaleTimeString()}
      ${summary.total ? ` • ${summary.total} total checks` : ''}
    </div>
  `;

  // Add event listeners
  setupEventListeners(checks);
}

/**
 * Render filtered checks
 */
function renderFilteredChecks(checks) {
  const filtered = currentFilter === 'all'
    ? checks
    : checks.filter(c => c.status === currentFilter);

  if (filtered.length === 0) {
    return `
      <div class="health-empty">
        <span class="material-icons">filter_list</span>
        <p>No ${currentFilter === 'all' ? '' : currentFilter} checks to display.</p>
      </div>
    `;
  }

  // Sort: errors first, then warnings, then passed
  const sortOrder = { fail: 0, warn: 1, pass: 2 };
  const sorted = [...filtered].sort((a, b) =>
    (sortOrder[a.status] ?? 3) - (sortOrder[b.status] ?? 3)
  );

  return sorted.map(check => renderCheckItem(check)).join('');
}

/**
 * Render a single check item
 */
function renderCheckItem(check) {
  const statusConfig = {
    pass: { icon: 'check_circle', class: 'pass', label: 'Pass', color: '#4caf50' },
    warn: { icon: 'warning', class: 'warn', label: 'Warning', color: '#ff9800' },
    fail: { icon: 'error', class: 'fail', label: 'Error', color: '#f44336' },
  };

  const config = statusConfig[check.status] || statusConfig.warn;
  const hasDetails = check.details && check.details.length > 0;
  const hasFix = check.fix;

  return `
    <div class="health-check-item health-${config.class}" data-check-id="${escapeHtml(check.id)}">
      <div class="health-check-status" style="color: ${config.color}">
        <span class="material-icons">${config.icon}</span>
      </div>
      <div class="health-check-content">
        <div class="health-check-header">
          <span class="health-check-name">${escapeHtml(check.name)}</span>
          <span class="health-check-label" style="background: ${config.color}">${config.label}</span>
        </div>
        <div class="health-check-description">${escapeHtml(check.description)}</div>
        ${hasDetails ? `
          <div class="health-check-details">
            ${check.details.slice(0, 5).map(d => `<div class="health-detail-line">${escapeHtml(d)}</div>`).join('')}
            ${check.details.length > 5 ? `<div class="health-detail-more">... and ${check.details.length - 5} more</div>` : ''}
          </div>
        ` : ''}
        ${hasFix ? `
          <div class="health-check-fix">
            <span class="material-icons">arrow_forward</span>
            <code>${escapeHtml(check.fix)}</code>
            <button class="btn btn-sm btn-ghost copy-fix" data-fix="${escapeHtml(check.fix)}" title="Copy command">
              <span class="material-icons">content_copy</span>
            </button>
          </div>
        ` : ''}
      </div>
    </div>
  `;
}

/**
 * Setup event listeners
 */
function setupEventListeners(checks) {
  // Filter buttons
  container.querySelectorAll('.health-filter-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      currentFilter = btn.dataset.filter;
      // Update active state
      container.querySelectorAll('.health-filter-btn').forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      // Re-render checks
      const checksList = document.getElementById('health-checks-list');
      if (checksList) {
        checksList.innerHTML = renderFilteredChecks(checks);
        setupCopyButtons();
      }
    });
  });

  // Fix all button
  const fixAllBtn = document.getElementById('health-fix-all');
  if (fixAllBtn) {
    fixAllBtn.addEventListener('click', async () => {
      if (confirm('Run "gt doctor --fix" to automatically fix issues?')) {
        fixAllBtn.disabled = true;
        fixAllBtn.innerHTML = '<span class="material-icons spinning">sync</span> Fixing...';
        try {
          const result = await api.runDoctorFix();
          showToast('Doctor fix completed. Refreshing...', 'success');
          setTimeout(() => loadHealthCheck(), 1000);
        } catch (err) {
          showToast(`Fix failed: ${err.message}`, 'error');
          fixAllBtn.disabled = false;
          fixAllBtn.innerHTML = '<span class="material-icons">build</span> Fix All Issues';
        }
      }
    });
  }

  setupCopyButtons();
}

/**
 * Setup copy buttons
 */
function setupCopyButtons() {
  container.querySelectorAll('.copy-fix').forEach(btn => {
    btn.addEventListener('click', async (e) => {
      e.stopPropagation();
      const fix = btn.dataset.fix;
      try {
        await navigator.clipboard.writeText(fix);
        showToast('Command copied to clipboard', 'success');
        btn.innerHTML = '<span class="material-icons">check</span>';
        setTimeout(() => {
          btn.innerHTML = '<span class="material-icons">content_copy</span>';
        }, 2000);
      } catch (err) {
        showToast('Failed to copy', 'error');
      }
    });
  });
}
