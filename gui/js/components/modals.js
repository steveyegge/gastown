/**
 * Gas Town GUI - Modals Component
 *
 * Handles all modal dialogs in the application.
 */

import { api } from '../api.js';
import { showToast } from './toast.js';
import { initAutocomplete, renderBeadItem, renderAgentItem } from './autocomplete.js';
import { state } from '../state.js';
import { escapeHtml, escapeAttr } from '../utils/html.js';
import { debounce } from '../utils/performance.js';

// Modal registry
const modals = new Map();

// References
let overlay = null;

// Peek modal state
let peekAutoRefreshInterval = null;
let currentPeekAgentId = null;

// GitHub repo mapping for known rigs (same as work-list.js)
const GITHUB_REPOS = {
  'hytopia-map-compression': 'web3dev1337/hytopia-map-compression',
  'testproject': null,
};

function getGitHubRepoForBead(beadId) {
  if (!beadId) return null;
  const prefixMatch = beadId.match(/^([a-z]+)-/i);
  if (prefixMatch) {
    const prefix = prefixMatch[1].toLowerCase();
    if (GITHUB_REPOS[prefix]) return GITHUB_REPOS[prefix];
  }
  for (const [key, repo] of Object.entries(GITHUB_REPOS)) {
    if (repo && beadId.toLowerCase().includes(key.toLowerCase())) {
      return repo;
    }
  }
  for (const repo of Object.values(GITHUB_REPOS)) {
    if (repo) return repo;
  }
  return null;
}

/**
 * Initialize modals system
 */
export function initModals() {
  overlay = document.getElementById('modal-overlay');

  // Register built-in modals
  registerModal('new-convoy', {
    element: document.getElementById('new-convoy-modal'),
    onOpen: initNewConvoyModal,
    onSubmit: handleNewConvoySubmit,
  });

  registerModal('new-bead', {
    element: document.getElementById('new-bead-modal'),
    onOpen: initNewBeadModal,
    onSubmit: handleNewBeadSubmit,
  });

  registerModal('sling', {
    element: document.getElementById('sling-modal'),
    onOpen: initSlingModal,
    onSubmit: handleSlingSubmit,
  });

  registerModal('mail-compose', {
    element: document.getElementById('mail-compose-modal'),
    onOpen: initMailComposeModal,
    onSubmit: handleMailComposeSubmit,
  });

  registerModal('help', {
    element: document.getElementById('help-modal'),
    onOpen: initHelpModal,
  });

  registerModal('new-rig', {
    element: document.getElementById('new-rig-modal'),
    onOpen: initNewRigModal,
    onSubmit: handleNewRigSubmit,
  });

  // Close on overlay click
  overlay?.addEventListener('click', (e) => {
    if (e.target === overlay) {
      closeAllModals();
    }
  });

  // Close buttons
  document.querySelectorAll('[data-modal-close]').forEach(btn => {
    btn.addEventListener('click', closeAllModals);
  });

  // Open buttons
  document.querySelectorAll('[data-modal-open]').forEach(btn => {
    btn.addEventListener('click', () => {
      const modalId = btn.dataset.modalOpen;
      openModal(modalId);
    });
  });

  // Form submissions
  document.querySelectorAll('.modal form').forEach(form => {
    form.addEventListener('submit', async (e) => {
      e.preventDefault();
      const modal = form.closest('.modal');
      if (!modal) return;

      const modalId = modal.id.replace('-modal', '');
      const config = modals.get(modalId);
      if (config?.onSubmit) {
        await config.onSubmit(form);
      }
    });
  });

  // Listen for custom modal events
  document.addEventListener('convoy:detail', (e) => {
    showConvoyDetailModal(e.detail.convoyId);
  });

  document.addEventListener('agent:detail', (e) => {
    showAgentDetailModal(e.detail.agentId);
  });

  document.addEventListener('agent:nudge', (e) => {
    showNudgeModal(e.detail.agentId);
  });

  document.addEventListener('mail:detail', (e) => {
    showMailDetailModal(e.detail.mailId, e.detail.mail);
  });

  document.addEventListener('convoy:escalate', (e) => {
    showEscalationModal(e.detail.convoyId, e.detail.convoyName);
  });

  document.addEventListener('mail:reply', (e) => {
    openModal('mail-compose', {
      replyTo: e.detail.mail.from,
      subject: e.detail.mail.subject,
    });
  });

  document.addEventListener('bead:detail', (e) => {
    showBeadDetailModal(e.detail.beadId, e.detail.bead);
  });

  document.addEventListener('agent:peek', (e) => {
    showPeekModal(e.detail.agentId);
  });

  // Register peek modal
  registerModal('peek', {
    element: document.getElementById('peek-modal'),
    onOpen: initPeekModal,
  });
}

/**
 * Register a modal
 */
export function registerModal(id, config) {
  modals.set(id, config);
}

/**
 * Open a modal by ID
 */
export function openModal(modalId, data = {}) {
  const config = modals.get(modalId);
  if (!config?.element) {
    console.warn(`Modal not found: ${modalId}`);
    return;
  }

  // Hide all modals first
  document.querySelectorAll('.modal').forEach(m => m.classList.add('hidden'));

  // Show overlay and modal
  overlay?.classList.remove('hidden');
  config.element.classList.remove('hidden');

  // Call onOpen callback
  if (config.onOpen) {
    config.onOpen(config.element, data);
  }

  // Focus first input
  const firstInput = config.element.querySelector('input, textarea, select');
  if (firstInput) {
    setTimeout(() => firstInput.focus(), 100);
  }
}

/**
 * Close all modals
 */
export function closeAllModals() {
  overlay?.classList.add('hidden');
  document.querySelectorAll('.modal').forEach(m => m.classList.add('hidden'));

  // Reset forms
  document.querySelectorAll('.modal form').forEach(form => form.reset());

  // Stop peek modal auto-refresh if active
  stopPeekAutoRefresh();
}

// Helper to stop peek auto-refresh from closeAllModals
function stopPeekAutoRefresh() {
  if (peekAutoRefreshInterval) {
    clearInterval(peekAutoRefreshInterval);
    peekAutoRefreshInterval = null;
  }
  currentPeekAgentId = null;
}

/**
 * Close specific modal
 */
export function closeModal(modalId) {
  const config = modals.get(modalId);
  if (config?.element) {
    config.element.classList.add('hidden');
  }

  // Check if any modal is still open
  const openModals = document.querySelectorAll('.modal:not(.hidden)');
  if (openModals.length === 0) {
    overlay?.classList.add('hidden');
  }
}

// === New Convoy Modal ===

function initNewConvoyModal(element, data) {
  // Clear any previous state
  const form = element.querySelector('form');
  if (form) form.reset();
}

async function handleNewConvoySubmit(form) {
  const name = form.querySelector('[name="name"]')?.value;
  const issuesText = form.querySelector('[name="issues"]')?.value || '';
  const notify = form.querySelector('[name="notify"]')?.value || null;

  if (!name) {
    showToast('Please enter a convoy name', 'warning');
    return;
  }

  // Parse issues (comma or newline separated)
  const issues = issuesText
    .split(/[,\n]/)
    .map(s => s.trim())
    .filter(Boolean);

  // Show loading state
  const submitBtn = form.querySelector('button[type="submit"]');
  const originalText = submitBtn?.innerHTML;
  if (submitBtn) {
    submitBtn.disabled = true;
    submitBtn.innerHTML = '<span class="material-icons spinning">sync</span> Creating...';
  }

  try {
    const result = await api.createConvoy(name, issues, notify);
    showToast(`Convoy "${name}" created`, 'success');
    closeAllModals();

    // Dispatch event for refresh
    document.dispatchEvent(new CustomEvent('convoy:created', { detail: result }));
  } catch (err) {
    showToast(`Failed to create convoy: ${err.message}`, 'error');
  } finally {
    if (submitBtn) {
      submitBtn.disabled = false;
      submitBtn.innerHTML = originalText;
    }
  }
}

// === New Bead Modal ===

function initNewBeadModal(element, data) {
  // Clear any previous state
  const form = element.querySelector('form');
  if (form) form.reset();
}

async function handleNewBeadSubmit(form) {
  const title = form.querySelector('[name="title"]')?.value;
  const description = form.querySelector('[name="description"]')?.value || '';
  const priority = form.querySelector('[name="priority"]')?.value || 'normal';
  const labelsText = form.querySelector('[name="labels"]')?.value || '';
  const slingNow = form.querySelector('[name="sling_now"]')?.checked || false;

  if (!title) {
    showToast('Please enter a title for the bead', 'warning');
    return;
  }

  // Parse labels
  const labels = labelsText
    .split(',')
    .map(s => s.trim())
    .filter(Boolean);

  // Close modal immediately and show progress toast
  showToast(`Creating work item "${title}"...`, 'info');
  closeAllModals();

  // Run in background (non-blocking)
  api.createBead(title, { description, priority, labels }).then(result => {
    if (result.success) {
      showToast(`Work item created: ${result.bead_id}`, 'success');

      // Dispatch event for UI refresh
      document.dispatchEvent(new CustomEvent('bead:created', { detail: result }));

      // If "sling now" was checked, open sling modal with bead pre-filled
      if (slingNow && result.bead_id) {
        setTimeout(() => {
          openModal('sling', { bead: result.bead_id });
        }, 100);
      }
    } else {
      showToast(`Failed to create work item: ${result.error}`, 'error');
    }
  }).catch(err => {
    showToast(`Failed to create work item: ${err.message}`, 'error');
  });
}

// === Sling Modal ===

// Track autocomplete instances for cleanup
let beadAutocomplete = null;

function initSlingModal(element, data) {
  // Pre-fill if data provided
  if (data.bead) {
    const beadInput = element.querySelector('[name="bead"]');
    if (beadInput) beadInput.value = data.bead;
  }
  if (data.target) {
    const targetInput = element.querySelector('[name="target"]');
    if (targetInput) targetInput.value = data.target;
  }

  // Initialize bead autocomplete
  const beadInput = element.querySelector('[name="bead"]');
  if (beadInput && !beadAutocomplete) {
    beadAutocomplete = initAutocomplete(beadInput, {
      search: async (query) => {
        // Search both beads and formulas
        try {
          const [beads, formulas] = await Promise.all([
            api.searchBeads(query).catch(() => []),
            api.searchFormulas(query).catch(() => []),
          ]);

          // Combine and dedupe results
          const results = [
            ...beads.map(b => ({ ...b, type: 'bead' })),
            ...formulas.map(f => ({ ...f, type: 'formula', id: f.name })),
          ];

          return results;
        } catch {
          // Fallback: provide local suggestions from convoys
          const convoys = state.get('convoys') || [];
          const beadMatches = [];
          convoys.forEach(convoy => {
            if (convoy.issues) {
              convoy.issues.forEach(issue => {
                const id = typeof issue === 'string' ? issue : issue.id;
                if (id && id.toLowerCase().includes(query.toLowerCase())) {
                  beadMatches.push({ id, title: typeof issue === 'object' ? issue.title : '', type: 'bead' });
                }
              });
            }
          });
          return beadMatches;
        }
      },
      renderItem: (item) => {
        if (item.type === 'formula') {
          return `
            <div class="bead-item formula">
              <span class="bead-icon">📜</span>
              <span class="bead-id">${escapeHtml(item.name || item.id)}</span>
              <span class="bead-desc">${escapeHtml(item.description || 'Formula')}</span>
            </div>
          `;
        }
        return renderBeadItem(item);
      },
      onSelect: (item, input) => {
        input.value = item.id || item.name;
      },
      minChars: 1,
      debounce: 150,
    });
  }

  // Populate target dropdown with agents
  populateTargetDropdown(element);
}

async function populateTargetDropdown(modalElement) {
  const targetSelect = modalElement.querySelector('[name="target"]');
  if (!targetSelect) return;

  // Show loading state
  targetSelect.innerHTML = '<option value="">Loading targets...</option>';
  targetSelect.disabled = true;

  try {
    // Get targets from API
    let targets = [];
    try {
      targets = await api.getTargets();
    } catch {
      // Fallback to agents from state
      targets = state.get('agents') || [];
    }

    // Reset and add placeholder
    targetSelect.innerHTML = '<option value="">Select target agent...</option>';
    targetSelect.disabled = false;

    // Group targets by type: global, rig, agent
    const groups = {
      global: { label: 'Global Agents', targets: [] },
      rig: { label: 'Rigs (auto-spawn polecat)', targets: [] },
      agent: { label: 'Running Agents', targets: [] },
    };

    targets.forEach(target => {
      const type = target.type || 'agent';
      if (groups[type]) {
        groups[type].targets.push(target);
      } else {
        groups.agent.targets.push(target);
      }
    });

    // Create optgroups for each non-empty group
    Object.entries(groups).forEach(([type, group]) => {
      if (group.targets.length === 0) return;

      const optgroup = document.createElement('optgroup');
      optgroup.label = group.label;

      group.targets.forEach(target => {
        const option = document.createElement('option');
        option.value = target.id;
        option.textContent = target.name || target.id;

        // Add status indicators
        if (target.has_work) {
          option.textContent += ' (busy)';
          option.className = 'target-busy';
        } else if (target.running === false) {
          option.textContent += ' (stopped)';
          option.className = 'target-stopped';
        }

        // Add description as title
        if (target.description) {
          option.title = target.description;
        }

        optgroup.appendChild(option);
      });

      targetSelect.appendChild(optgroup);
    });

    // If no targets at all, show helpful message
    if (targetSelect.options.length === 1) {
      const option = document.createElement('option');
      option.value = '';
      option.textContent = 'No targets available - add a rig first';
      option.disabled = true;
      targetSelect.appendChild(option);
    }
  } catch (err) {
    console.error('[Modals] Failed to populate targets:', err);
    targetSelect.innerHTML = '<option value="">Failed to load targets</option>';
    targetSelect.disabled = false;
  }
}

async function handleSlingSubmit(form) {
  const bead = form.querySelector('[name="bead"]')?.value;
  const target = form.querySelector('[name="target"]')?.value;
  const molecule = form.querySelector('[name="molecule"]')?.value || undefined;
  const quality = form.querySelector('[name="quality"]')?.value || undefined;

  if (!bead || !target) {
    showToast('Please enter both bead and target', 'warning');
    return;
  }

  // Close modal immediately and show progress toast
  showToast(`Slinging ${bead} → ${target}...`, 'info');
  closeAllModals();

  // Run in background (non-blocking)
  api.sling(bead, target, { molecule, quality }).then(result => {
    showToast(`Work slung: ${bead} → ${target}`, 'success');
    // Dispatch event
    document.dispatchEvent(new CustomEvent('work:slung', { detail: result }));
  }).catch(err => {
    // For sling errors, we can't show the fancy error in the modal (it's closed)
    // So just show a toast with the error message
    showToast(`Failed to sling work: ${err.message || 'Unknown error'}`, 'error');
  });
}

function showSlingError(form, errorData) {
  // Remove existing error
  const existing = form.querySelector('.sling-error');
  if (existing) existing.remove();

  const errorDiv = document.createElement('div');
  errorDiv.className = 'sling-error';

  if (errorData.errorType === 'formula_missing') {
    errorDiv.innerHTML = `
      <div class="sling-error-icon">
        <span class="material-icons">warning</span>
      </div>
      <div class="sling-error-content">
        <div class="sling-error-title">Formula Not Found</div>
        <div class="sling-error-message">
          <code>${escapeHtml(errorData.formula)}</code> doesn't exist yet.
        </div>
        <div class="sling-error-hint">${escapeHtml(errorData.hint)}</div>
        <div class="sling-error-actions">
          <button type="button" class="btn btn-secondary btn-sm" onclick="this.closest('form').querySelector('[name=quality]').value = ''; this.closest('.sling-error').remove(); showToast('Quality cleared - try again', 'info');">
            <span class="material-icons">remove_circle</span>
            Clear Quality & Retry
          </button>
        </div>
      </div>
    `;
  } else if (errorData.errorType === 'bead_missing') {
    errorDiv.innerHTML = `
      <div class="sling-error-icon">
        <span class="material-icons">search_off</span>
      </div>
      <div class="sling-error-content">
        <div class="sling-error-title">Bead Not Found</div>
        <div class="sling-error-message">${escapeHtml(errorData.hint)}</div>
      </div>
    `;
  } else {
    errorDiv.innerHTML = `
      <div class="sling-error-icon">
        <span class="material-icons">error</span>
      </div>
      <div class="sling-error-content">
        <div class="sling-error-title">Sling Failed</div>
        <div class="sling-error-message">${escapeHtml(errorData.error || 'Unknown error')}</div>
      </div>
    `;
  }

  // Insert before submit button
  const submitBtn = form.querySelector('button[type="submit"]');
  if (submitBtn?.parentElement) {
    submitBtn.parentElement.insertBefore(errorDiv, submitBtn);
  } else {
    form.appendChild(errorDiv);
  }
}

// === Mail Compose Modal ===

function initMailComposeModal(element, data) {
  // Populate recipient dropdown
  populateRecipientDropdown(element, data.replyTo);

  // Pre-fill subject if replying
  if (data.subject) {
    const subjectInput = element.querySelector('[name="subject"]');
    if (subjectInput) subjectInput.value = `Re: ${data.subject}`;
  }
}

async function populateRecipientDropdown(modalElement, preselect = null) {
  const toSelect = modalElement.querySelector('[name="to"]');
  if (!toSelect) return;

  // Keep first option (placeholder)
  const placeholder = toSelect.options[0];
  toSelect.innerHTML = '';
  toSelect.appendChild(placeholder);

  try {
    // Try to get agents from API first
    let agents = [];
    try {
      agents = await api.getAgents();
    } catch {
      // Fallback to agents from state
      agents = state.get('agents') || [];
    }

    // Add common recipients group
    const commonGroup = document.createElement('optgroup');
    commonGroup.label = 'Common Recipients';

    // Always include Mayor and Overseer
    const commonRecipients = [
      { id: 'mayor/', name: 'Mayor', role: 'mayor' },
      { id: 'human', name: 'Human Overseer', role: 'overseer' },
    ];

    commonRecipients.forEach(r => {
      const option = document.createElement('option');
      option.value = r.id;
      option.textContent = r.name;
      option.className = `recipient-${r.role}`;
      commonGroup.appendChild(option);
    });
    toSelect.appendChild(commonGroup);

    // Group agents by role
    const roleGroups = new Map();
    const roleOrder = ['deacon', 'witness', 'refinery', 'polecat'];

    agents.forEach(agent => {
      const role = (agent.role || 'worker').toLowerCase();
      if (!roleGroups.has(role)) {
        roleGroups.set(role, []);
      }
      roleGroups.get(role).push(agent);
    });

    // Create optgroups for each role
    roleOrder.forEach(role => {
      const roleAgents = roleGroups.get(role);
      if (!roleAgents || roleAgents.length === 0) return;

      const optgroup = document.createElement('optgroup');
      optgroup.label = capitalize(role) + 's';

      roleAgents.forEach(agent => {
        const option = document.createElement('option');
        option.value = agent.path || agent.id || agent.name;
        option.textContent = agent.name || agent.id;
        option.className = `recipient-${role}`;
        optgroup.appendChild(option);
      });

      toSelect.appendChild(optgroup);
    });

    // Add any remaining roles
    roleGroups.forEach((roleAgents, role) => {
      if (roleOrder.includes(role)) return;
      if (roleAgents.length === 0) return;

      const optgroup = document.createElement('optgroup');
      optgroup.label = capitalize(role) + 's';

      roleAgents.forEach(agent => {
        const option = document.createElement('option');
        option.value = agent.path || agent.id || agent.name;
        option.textContent = agent.name || agent.id;
        optgroup.appendChild(option);
      });

      toSelect.appendChild(optgroup);
    });

    // Pre-select if replying
    if (preselect) {
      toSelect.value = preselect;
    }

  } catch (err) {
    console.error('[Modals] Failed to populate recipients:', err);
  }
}

function capitalize(str) {
  return str.charAt(0).toUpperCase() + str.slice(1);
}

// === Help Modal ===

function initHelpModal(element) {
  // Set up tab switching
  const tabs = element.querySelectorAll('.help-tab');
  const panels = element.querySelectorAll('.help-panel');

  tabs.forEach(tab => {
    tab.addEventListener('click', () => {
      const tabId = tab.dataset.tab;

      // Update active tab
      tabs.forEach(t => t.classList.toggle('active', t === tab));

      // Update active panel
      panels.forEach(p => {
        p.classList.toggle('active', p.id === `help-${tabId}`);
      });
    });
  });
}

// === New Rig Modal ===

// Cache for GitHub repos
let cachedGitHubRepos = null;

function initNewRigModal(element, data) {
  const form = element.querySelector('form');
  if (form) form.reset();

  // Reset GitHub repo picker state
  const repoList = document.getElementById('github-repo-list');
  const pickerBtn = document.getElementById('github-repo-picker-btn');
  if (repoList) repoList.classList.add('hidden');
  if (pickerBtn) {
    pickerBtn.querySelector('.btn-text').textContent = 'Load My Repositories';
    pickerBtn.disabled = false;
  }

  // Set up GitHub repo picker button
  pickerBtn?.addEventListener('click', loadGitHubRepos, { once: true });

  // Set up search filtering with debounce
  const searchInput = document.getElementById('github-repo-search');
  const debouncedFilter = debounce((value) => filterGitHubRepos(value), 150);
  searchInput?.addEventListener('input', (e) => {
    debouncedFilter(e.target.value);
  });
}

async function loadGitHubRepos() {
  const pickerBtn = document.getElementById('github-repo-picker-btn');
  const repoList = document.getElementById('github-repo-list');
  const repoItems = document.getElementById('github-repo-items');

  if (!pickerBtn || !repoList || !repoItems) return;

  // Show loading state
  pickerBtn.disabled = true;
  pickerBtn.querySelector('.btn-text').textContent = 'Loading...';
  repoItems.innerHTML = '<div class="github-repo-loading"><span class="loading-spinner"></span> Loading repositories...</div>';
  repoList.classList.remove('hidden');

  try {
    // Use cached repos if available
    if (!cachedGitHubRepos) {
      cachedGitHubRepos = await api.getGitHubRepos({ limit: 100 });
    }

    renderGitHubRepos(cachedGitHubRepos);
    pickerBtn.querySelector('.btn-text').textContent = 'Refresh List';
    pickerBtn.disabled = false;

    // Re-add click listener for refresh
    pickerBtn.addEventListener('click', async () => {
      cachedGitHubRepos = null;
      await loadGitHubRepos();
    }, { once: true });

  } catch (err) {
    repoItems.innerHTML = `<div class="github-repo-empty">Failed to load repos: ${escapeHtml(err.message)}</div>`;
    pickerBtn.querySelector('.btn-text').textContent = 'Retry';
    pickerBtn.disabled = false;
    pickerBtn.addEventListener('click', loadGitHubRepos, { once: true });
  }
}

function renderGitHubRepos(repos) {
  const repoItems = document.getElementById('github-repo-items');
  if (!repoItems) return;

  if (!repos || repos.length === 0) {
    repoItems.innerHTML = '<div class="github-repo-empty">No repositories found</div>';
    return;
  }

  repoItems.innerHTML = repos.map(repo => `
    <div class="github-repo-item ${repo.isPrivate ? 'private' : ''}"
         data-name="${escapeAttr(repo.name)}"
         data-url="${escapeAttr(repo.url)}">
      <span class="material-icons repo-icon">${repo.isPrivate ? 'lock' : 'public'}</span>
      <div class="repo-info">
        <div class="repo-name">${escapeHtml(repo.nameWithOwner)}</div>
        <div class="repo-desc">${escapeHtml(repo.description || 'No description')}</div>
      </div>
      <div class="repo-meta">
        ${repo.primaryLanguage ? `
          <span class="repo-lang">
            <span class="lang-dot" style="background: ${getLanguageColor(repo.primaryLanguage.name)}"></span>
            ${escapeHtml(repo.primaryLanguage.name)}
          </span>
        ` : ''}
      </div>
    </div>
  `).join('');

  // Add click handlers
  repoItems.querySelectorAll('.github-repo-item').forEach(item => {
    item.addEventListener('click', () => selectGitHubRepo(item));
  });
}

function filterGitHubRepos(query) {
  if (!cachedGitHubRepos) return;

  const q = query.toLowerCase();
  const filtered = cachedGitHubRepos.filter(repo =>
    repo.name.toLowerCase().includes(q) ||
    repo.nameWithOwner.toLowerCase().includes(q) ||
    (repo.description || '').toLowerCase().includes(q)
  );
  renderGitHubRepos(filtered);
}

function selectGitHubRepo(item) {
  const name = item.dataset.name;
  const url = item.dataset.url;

  // Fill in the form fields
  const nameInput = document.getElementById('rig-name');
  const urlInput = document.getElementById('rig-url');

  if (nameInput) nameInput.value = name;
  if (urlInput) urlInput.value = url;

  // Hide the repo list
  const repoList = document.getElementById('github-repo-list');
  if (repoList) repoList.classList.add('hidden');

  // Show feedback
  showToast(`Selected: ${name}`, 'success');
}

function getLanguageColor(lang) {
  const colors = {
    'JavaScript': '#f1e05a',
    'TypeScript': '#3178c6',
    'Python': '#3572A5',
    'Go': '#00ADD8',
    'Rust': '#dea584',
    'Ruby': '#701516',
    'Java': '#b07219',
    'C#': '#178600',
    'C++': '#f34b7d',
    'C': '#555555',
    'PHP': '#4F5D95',
    'Swift': '#F05138',
    'Kotlin': '#A97BFF',
    'Markdown': '#083fa1',
  };
  return colors[lang] || '#8b949e';
}

async function handleNewRigSubmit(form) {
  const name = form.querySelector('[name="name"]')?.value?.trim();
  const url = form.querySelector('[name="url"]')?.value?.trim();

  if (!name || !url) {
    showToast('Please enter both name and path', 'warning');
    return;
  }

  // Validate name format (lowercase, numbers, hyphens only)
  if (!/^[a-z0-9-]+$/.test(name)) {
    showToast('Rig name must be lowercase letters, numbers, and hyphens only (no spaces)', 'warning');
    return;
  }

  // Close modal immediately and show progress toast
  showToast(`Adding rig "${name}"...`, 'info');
  closeAllModals();

  // Run in background (non-blocking)
  api.addRig(name, url).then(result => {
    if (result.success) {
      showToast(`Rig "${name}" added successfully`, 'success');
      // Trigger refresh
      document.dispatchEvent(new CustomEvent('rigs:refresh'));
    } else {
      showToast(`Failed to add rig: ${result.error}`, 'error');
    }
  }).catch(err => {
    showToast(`Failed to add rig: ${err.message}`, 'error');
  });
}

async function handleMailComposeSubmit(form) {
  const to = form.querySelector('[name="to"]')?.value;
  const subject = form.querySelector('[name="subject"]')?.value;
  const message = form.querySelector('[name="message"]')?.value;
  const priority = form.querySelector('[name="priority"]')?.value || 'normal';

  if (!to || !subject || !message) {
    showToast('Please fill in all fields', 'warning');
    return;
  }

  // Show loading state
  const submitBtn = form.querySelector('button[type="submit"]');
  const originalText = submitBtn?.innerHTML;
  if (submitBtn) {
    submitBtn.disabled = true;
    submitBtn.innerHTML = '<span class="material-icons spinning">sync</span> Sending...';
  }

  try {
    await api.sendMail(to, subject, message, priority);
    showToast('Mail sent', 'success');
    closeAllModals();
  } catch (err) {
    showToast(`Failed to send mail: ${err.message}`, 'error');
  } finally {
    if (submitBtn) {
      submitBtn.disabled = false;
      submitBtn.innerHTML = originalText;
    }
  }
}

// === Dynamic Modals ===

async function showConvoyDetailModal(convoyId) {
  // Show loading modal immediately
  const loadingContent = `
    <div class="modal-header">
      <h2>Convoy: ${escapeHtml(convoyId)}</h2>
      <button class="btn btn-icon" data-modal-close>
        <span class="material-icons">close</span>
      </button>
    </div>
    <div class="modal-body">
      <div class="loading-state">
        <span class="loading-spinner"></span>
        Loading convoy details...
      </div>
    </div>
  `;
  const modal = showDynamicModal('convoy-detail', loadingContent);

  try {
    const convoy = await api.getConvoy(convoyId);
    const content = `
      <div class="modal-header">
        <h2>Convoy: ${escapeHtml(convoy.name || convoy.id)}</h2>
        <button class="btn btn-icon" data-modal-close>
          <span class="material-icons">close</span>
        </button>
      </div>
      <div class="modal-body">
        <div class="detail-grid">
          <div class="detail-item">
            <label>ID</label>
            <span>${convoyId}</span>
          </div>
          <div class="detail-item">
            <label>Status</label>
            <span class="status-badge status-${convoy.status || 'pending'}">${convoy.status || 'pending'}</span>
          </div>
          <div class="detail-item">
            <label>Created</label>
            <span>${new Date(convoy.created_at).toLocaleString()}</span>
          </div>
          ${convoy.issues?.length ? `
            <div class="detail-item full-width">
              <label>Issues</label>
              <ul class="issue-list">
                ${convoy.issues.map(i => `<li>${escapeHtml(typeof i === 'string' ? i : i.title)}</li>`).join('')}
              </ul>
            </div>
          ` : ''}
        </div>
      </div>
    `;
    modal.innerHTML = content;

    // Re-add close button handler
    modal.querySelector('[data-modal-close]')?.addEventListener('click', closeAllModals);
  } catch (err) {
    modal.innerHTML = `
      <div class="modal-header">
        <h2>Convoy: ${escapeHtml(convoyId)}</h2>
        <button class="btn btn-icon" data-modal-close>
          <span class="material-icons">close</span>
        </button>
      </div>
      <div class="modal-body">
        <div class="error-state">
          <span class="material-icons">error_outline</span>
          <p>Failed to load convoy: ${escapeHtml(err.message)}</p>
        </div>
      </div>
    `;
    modal.querySelector('[data-modal-close]')?.addEventListener('click', closeAllModals);
  }
}

async function showAgentDetailModal(agentId) {
  // For now show a simple modal - can be expanded later
  const content = `
    <div class="modal-header">
      <h2>Agent Details</h2>
      <button class="btn btn-icon" data-modal-close>
        <span class="material-icons">close</span>
      </button>
    </div>
    <div class="modal-body">
      <p>Agent ID: <code>${escapeHtml(agentId)}</code></p>
      <p>Detailed agent view coming soon...</p>
    </div>
  `;
  showDynamicModal('agent-detail', content);
}

function showNudgeModal(agentId) {
  const content = `
    <div class="modal-header">
      <h2>Nudge Agent</h2>
      <button class="btn btn-icon" data-modal-close>
        <span class="material-icons">close</span>
      </button>
    </div>
    <div class="modal-body">
      <form id="nudge-form">
        <input type="hidden" name="agent_id" value="${escapeAttr(agentId)}">
        <div class="form-group">
          <label for="nudge-message">Message</label>
          <textarea id="nudge-message" name="message" rows="3" placeholder="Enter a message to send to the agent..."></textarea>
        </div>
        <div class="form-actions">
          <button type="button" class="btn btn-secondary" data-modal-close>Cancel</button>
          <button type="submit" class="btn btn-primary">
            <span class="material-icons">send</span>
            Send Nudge
          </button>
        </div>
      </form>
    </div>
  `;

  const modal = showDynamicModal('nudge', content);

  // Handle form submission
  const form = modal.querySelector('#nudge-form');
  form?.addEventListener('submit', async (e) => {
    e.preventDefault();
    const message = form.querySelector('[name="message"]')?.value;

    // Show loading state
    const submitBtn = form.querySelector('button[type="submit"]');
    const originalText = submitBtn?.innerHTML;
    if (submitBtn) {
      submitBtn.disabled = true;
      submitBtn.innerHTML = '<span class="material-icons spinning">sync</span> Sending...';
    }

    try {
      await api.nudge(agentId, message);
      showToast('Nudge sent', 'success');
      closeAllModals();
    } catch (err) {
      showToast(`Failed to nudge agent: ${err.message}`, 'error');
    } finally {
      if (submitBtn) {
        submitBtn.disabled = false;
        submitBtn.innerHTML = originalText;
      }
    }
  });
}

function showMailDetailModal(mailId, mail) {
  const content = `
    <div class="modal-header">
      <h2>${escapeHtml(mail.subject || '(No Subject)')}</h2>
      <button class="btn btn-icon" data-modal-close>
        <span class="material-icons">close</span>
      </button>
    </div>
    <div class="modal-body">
      <div class="mail-detail-meta">
        <div><strong>From:</strong> ${escapeHtml(mail.from || 'System')}</div>
        <div><strong>Date:</strong> ${new Date(mail.timestamp).toLocaleString()}</div>
        ${mail.priority && mail.priority !== 'normal' ? `<div><strong>Priority:</strong> ${mail.priority}</div>` : ''}
      </div>
      <div class="mail-detail-body">
        ${escapeHtml(mail.message || mail.body || '(No content)')}
      </div>
    </div>
    <div class="modal-footer">
      <button class="btn btn-secondary" onclick="document.dispatchEvent(new CustomEvent('mail:reply', { detail: { mail: ${JSON.stringify(mail)} } }))">
        <span class="material-icons">reply</span>
        Reply
      </button>
    </div>
  `;
  showDynamicModal('mail-detail', content);
}

// === Bead Detail Modal ===

function showBeadDetailModal(beadId, bead) {
  if (!bead) {
    showToast('Bead data not available', 'warning');
    return;
  }

  const statusIcons = {
    open: 'radio_button_unchecked',
    closed: 'check_circle',
    'in-progress': 'pending',
    in_progress: 'pending',
    blocked: 'block',
  };

  const typeIcons = {
    task: 'task_alt',
    bug: 'bug_report',
    feature: 'star',
    chore: 'build',
    epic: 'flag',
  };

  const statusIcon = statusIcons[bead.status] || 'help_outline';
  const typeIcon = typeIcons[bead.issue_type] || 'assignment';
  const assignee = bead.assignee ? bead.assignee.split('/').pop() : null;

  // Parse close_reason for links (pass beadId for GitHub URL lookup)
  const closeReasonHtml = bead.close_reason
    ? parseCloseReasonForModal(bead.close_reason, beadId)
    : '';

  const content = `
    <div class="modal-header bead-detail-header">
      <div class="bead-detail-title-row">
        <span class="material-icons status-icon status-${bead.status}">${statusIcon}</span>
        <h2>${escapeHtml(bead.title)}</h2>
      </div>
      <button class="btn btn-icon" data-modal-close>
        <span class="material-icons">close</span>
      </button>
    </div>
    <div class="modal-body bead-detail-body">
      <div class="bead-detail-meta">
        <div class="meta-row">
          <span class="meta-label">ID:</span>
          <code class="bead-id-code">${escapeHtml(beadId)}</code>
          <button class="btn btn-icon btn-xs copy-btn" data-copy="${beadId}" title="Copy ID">
            <span class="material-icons">content_copy</span>
          </button>
        </div>
        <div class="meta-row">
          <span class="meta-label">Type:</span>
          <span class="meta-value">
            <span class="material-icons">${typeIcon}</span>
            ${bead.issue_type || 'task'}
          </span>
        </div>
        <div class="meta-row">
          <span class="meta-label">Priority:</span>
          <span class="priority-badge priority-${bead.priority || 2}">P${bead.priority || 2}</span>
        </div>
        <div class="meta-row">
          <span class="meta-label">Status:</span>
          <span class="status-badge status-${bead.status}">${bead.status || 'open'}</span>
        </div>
        ${assignee ? `
          <div class="meta-row">
            <span class="meta-label">Assignee:</span>
            <span class="meta-value">
              <span class="material-icons">person</span>
              ${escapeHtml(assignee)}
            </span>
          </div>
        ` : ''}
        <div class="meta-row">
          <span class="meta-label">Created:</span>
          <span class="meta-value">${bead.created_at ? new Date(bead.created_at).toLocaleString() : 'Unknown'}</span>
        </div>
        ${bead.closed_at ? `
          <div class="meta-row">
            <span class="meta-label">Completed:</span>
            <span class="meta-value">${new Date(bead.closed_at).toLocaleString()}</span>
          </div>
        ` : ''}
      </div>

      ${bead.description ? `
        <div class="bead-detail-section">
          <h4>Description</h4>
          <div class="bead-description">${escapeHtml(bead.description)}</div>
        </div>
      ` : ''}

      ${bead.close_reason ? `
        <div class="bead-detail-section completion-section">
          <h4>
            <span class="material-icons">check_circle</span>
            Completion Summary
          </h4>
          <div class="bead-close-reason">${closeReasonHtml}</div>
        </div>
      ` : ''}

      ${bead.labels && bead.labels.length > 0 ? `
        <div class="bead-detail-section">
          <h4>Labels</h4>
          <div class="bead-labels">
            ${bead.labels.map(l => `<span class="label-tag">${escapeHtml(l)}</span>`).join('')}
          </div>
        </div>
      ` : ''}

      <div class="bead-detail-section bead-links-section" id="bead-links-section">
        <h4>
          <span class="material-icons">link</span>
          Related Links
        </h4>
        <div class="bead-links-content" id="bead-links-content">
          <div class="loading-inline">
            <span class="material-icons spinning">sync</span>
            Searching for PRs...
          </div>
        </div>
      </div>
    </div>
    <div class="modal-footer">
      <button class="btn btn-secondary" data-modal-close>Close</button>
      ${bead.status !== 'closed' ? `
        <button class="btn btn-primary sling-btn" data-bead-id="${beadId}">
          <span class="material-icons">send</span>
          Sling Work
        </button>
      ` : ''}
    </div>
  `;

  const modal = showDynamicModal('bead-detail', content);

  // Add sling button handler
  const slingBtn = modal.querySelector('.sling-btn');
  if (slingBtn) {
    slingBtn.addEventListener('click', () => {
      document.dispatchEvent(new CustomEvent('bead:sling', { detail: { beadId } }));
      closeAllModals();
    });
  }

  // Add copy button handlers
  modal.querySelectorAll('.copy-btn').forEach(btn => {
    btn.addEventListener('click', (e) => {
      e.stopPropagation();
      const text = btn.dataset.copy;
      navigator.clipboard.writeText(text).then(() => {
        showToast(`Copied: ${text}`, 'success');
      });
    });
  });

  // Add commit link handlers (copy-only, no GitHub URL)
  modal.querySelectorAll('.commit-copy').forEach(link => {
    link.addEventListener('click', (e) => {
      e.preventDefault();
      const hash = link.dataset.commit;
      navigator.clipboard.writeText(hash).then(() => {
        showToast(`Copied commit: ${hash}`, 'success');
      });
    });
  });

  // Add PR link handlers (copy-only, no GitHub URL)
  modal.querySelectorAll('.pr-copy').forEach(link => {
    link.addEventListener('click', (e) => {
      e.preventDefault();
      const pr = link.dataset.pr;
      navigator.clipboard.writeText(`#${pr}`).then(() => {
        showToast(`Copied: PR #${pr}`, 'success');
      });
    });
  });

  // Fetch and display related links (PRs, commits)
  fetchBeadLinks(beadId, modal);
}

async function fetchBeadLinks(beadId, modal) {
  const linksContent = modal.querySelector('#bead-links-content');
  if (!linksContent) return;

  try {
    const links = await api.getBeadLinks(beadId);

    if (!links.prs || links.prs.length === 0) {
      linksContent.innerHTML = `
        <div class="no-links">
          <span class="material-icons">link_off</span>
          No related PRs found
        </div>
      `;
      return;
    }

    const prHtml = links.prs.map(pr => {
      const stateIcon = pr.state === 'MERGED' ? 'merge' :
                        pr.state === 'CLOSED' ? 'close' :
                        'git_merge';
      const stateClass = pr.state.toLowerCase();
      return `
        <a href="${pr.url}" target="_blank" class="pr-link pr-state-${stateClass}">
          <span class="material-icons">${stateIcon}</span>
          <span class="pr-info">
            <span class="pr-title">${escapeHtml(pr.title)}</span>
            <span class="pr-meta">${pr.repo} #${pr.number}</span>
          </span>
          <span class="material-icons open-icon">open_in_new</span>
        </a>
      `;
    }).join('');

    linksContent.innerHTML = prHtml;
  } catch (err) {
    console.error('[Links] Error fetching links:', err);
    linksContent.innerHTML = `
      <div class="no-links">
        <span class="material-icons">error_outline</span>
        Could not fetch links
      </div>
    `;
  }
}

/**
 * Parse close_reason for the detail modal (with more formatting)
 */
function parseCloseReasonForModal(text, beadId) {
  if (!text) return '';

  let result = escapeHtml(text);
  const repo = getGitHubRepoForBead(beadId);

  // Replace commit references with styled links
  result = result.replace(/commit\s+([a-f0-9]{7,40})/gi, (match, hash) => {
    if (repo) {
      const url = `https://github.com/${repo}/commit/${hash}`;
      return `<a href="${url}" target="_blank" class="commit-link code-link" data-commit="${hash}" title="View on GitHub">
        <span class="material-icons">commit</span>${hash.substring(0, 7)}
      </a>`;
    } else {
      return `<a href="#" class="commit-copy code-link" data-commit="${hash}" title="Click to copy">
        <span class="material-icons">commit</span>${hash.substring(0, 7)}
      </a>`;
    }
  });

  // Replace PR references
  result = result.replace(/PR\s*#?(\d+)/gi, (match, num) => {
    if (repo) {
      const url = `https://github.com/${repo}/pull/${num}`;
      return `<a href="${url}" target="_blank" class="pr-link code-link" data-pr="${num}" title="View on GitHub">
        <span class="material-icons">merge</span>PR #${num}
      </a>`;
    } else {
      return `<a href="#" class="pr-copy code-link" data-pr="${num}" title="Click to copy">
        <span class="material-icons">merge</span>PR #${num}
      </a>`;
    }
  });

  // Replace file paths (→ filename.ext pattern)
  result = result.replace(/→\s*([A-Za-z0-9_.-]+\.[A-Za-z0-9]+)/g, (match, filename) => {
    return `→ <code class="filename">${filename}</code>`;
  });

  return result;
}

// === Escalation Modal ===

function showEscalationModal(convoyId, convoyName) {
  const content = `
    <div class="modal-header escalation-header">
      <h2>
        <span class="material-icons warning-icon">warning</span>
        Escalate Issue
      </h2>
      <button class="btn btn-icon" data-modal-close>
        <span class="material-icons">close</span>
      </button>
    </div>
    <div class="modal-body">
      <div class="escalation-info">
        <p>You are about to escalate convoy: <strong>${escapeHtml(convoyName || convoyId)}</strong></p>
        <p class="escalation-warning">This will notify the Mayor and may interrupt other workflows.</p>
      </div>
      <form id="escalation-form">
        <input type="hidden" name="convoy_id" value="${convoyId}">
        <div class="form-group">
          <label for="escalation-reason">Reason for Escalation</label>
          <textarea
            id="escalation-reason"
            name="reason"
            rows="4"
            required
            placeholder="Describe why this issue needs immediate attention..."
          ></textarea>
        </div>
        <div class="form-group">
          <label for="escalation-priority">Priority Level</label>
          <select id="escalation-priority" name="priority">
            <option value="normal">Normal - Needs attention soon</option>
            <option value="high">High - Blocking other work</option>
            <option value="critical">Critical - Production issue</option>
          </select>
        </div>
        <div class="form-group">
          <label class="checkbox-label">
            <input type="checkbox" name="block_others" value="true">
            Block new work assignments until resolved
          </label>
        </div>
        <div class="form-actions">
          <button type="button" class="btn btn-secondary" data-modal-close>Cancel</button>
          <button type="submit" class="btn btn-danger">
            <span class="material-icons">priority_high</span>
            Escalate
          </button>
        </div>
      </form>
    </div>
  `;

  const modal = showDynamicModal('escalation', content);

  // Handle form submission
  const form = modal.querySelector('#escalation-form');
  form?.addEventListener('submit', async (e) => {
    e.preventDefault();
    const reason = form.querySelector('[name="reason"]')?.value;
    const priority = form.querySelector('[name="priority"]')?.value || 'normal';

    if (!reason) {
      showToast('Please provide a reason for escalation', 'warning');
      return;
    }

    // Show loading state
    const submitBtn = form.querySelector('button[type="submit"]');
    const originalText = submitBtn?.innerHTML;
    if (submitBtn) {
      submitBtn.disabled = true;
      submitBtn.innerHTML = '<span class="material-icons spinning">sync</span> Escalating...';
    }

    try {
      await api.escalate(convoyId, reason, priority);
      showToast('Issue escalated to Mayor', 'success');
      closeAllModals();

      // Dispatch event for UI updates
      document.dispatchEvent(new CustomEvent('convoy:escalated', {
        detail: { convoyId, reason, priority }
      }));
    } catch (err) {
      showToast(`Failed to escalate: ${err.message}`, 'error');
    } finally {
      if (submitBtn) {
        submitBtn.disabled = false;
        submitBtn.innerHTML = originalText;
      }
    }
  });
}

/**
 * Show a dynamic modal with custom content
 */
function showDynamicModal(id, content) {
  // Remove existing dynamic modal if present
  const existing = document.getElementById(`${id}-modal`);
  if (existing) existing.remove();

  // Create new modal
  const modal = document.createElement('div');
  modal.id = `${id}-modal`;
  modal.className = 'modal';
  modal.innerHTML = content;

  // Add to overlay (not body - modals must be inside overlay)
  const modalOverlay = overlay || document.getElementById('modal-overlay');
  modalOverlay.appendChild(modal);

  // Register and show
  registerModal(id, { element: modal });
  overlay?.classList.remove('hidden');
  modal.classList.remove('hidden');

  // Wire up close buttons
  modal.querySelectorAll('[data-modal-close]').forEach(btn => {
    btn.addEventListener('click', closeAllModals);
  });

  return modal;
}

// === Peek Modal ===

function initPeekModal(element, data) {
  // Set up refresh button
  const refreshBtn = element.querySelector('#peek-refresh');
  if (refreshBtn) {
    refreshBtn.addEventListener('click', () => {
      if (currentPeekAgentId) {
        refreshPeekOutput(currentPeekAgentId);
      }
    });
  }

  // Set up auto-refresh toggle
  const autoRefreshToggle = element.querySelector('#peek-auto-refresh-toggle');
  if (autoRefreshToggle) {
    autoRefreshToggle.addEventListener('change', (e) => {
      if (e.target.checked) {
        startAutoRefresh();
      } else {
        stopAutoRefresh();
      }
    });
  }

  // Set up transcript button
  const transcriptBtn = element.querySelector('#peek-transcript');
  if (transcriptBtn) {
    transcriptBtn.addEventListener('click', () => {
      if (currentPeekAgentId) {
        showAgentTranscript(currentPeekAgentId);
      }
    });
  }
}

async function showPeekModal(agentId) {
  currentPeekAgentId = agentId;

  // Parse agent ID (format: "rig/name")
  const parts = agentId.split('/');
  const rig = parts[0];
  const name = parts[1] || parts[0];

  // Update header
  const headerEl = document.getElementById('peek-agent-name');
  if (headerEl) {
    headerEl.textContent = `Output: ${name}`;
  }

  // Reset auto-refresh state
  const autoRefreshToggle = document.getElementById('peek-auto-refresh-toggle');
  if (autoRefreshToggle) {
    autoRefreshToggle.checked = false;
  }
  stopAutoRefresh();

  // Open modal
  openModal('peek', { agentId, rig, name });

  // Fetch initial output
  await refreshPeekOutput(agentId);
}

async function refreshPeekOutput(agentId) {
  const parts = agentId.split('/');
  const rig = parts[0];
  const name = parts[1] || parts[0];

  const statusEl = document.getElementById('peek-status');
  const outputEl = document.getElementById('peek-output');
  const outputContent = outputEl?.querySelector('.output-content');

  if (statusEl) {
    statusEl.innerHTML = '<span class="loading-spinner"></span> Loading...';
  }

  try {
    const response = await api.getPeekOutput(rig, name);

    // Update status
    if (statusEl) {
      const statusClass = response.running ? 'status-running' : 'status-stopped';
      const statusText = response.running ? 'Running' : 'Stopped';
      const sessionInfo = response.session ? ` (${response.session})` : '';
      statusEl.innerHTML = `
        <span class="peek-status-badge ${statusClass}">
          <span class="material-icons">${response.running ? 'play_circle' : 'stop_circle'}</span>
          ${statusText}
        </span>
        <span class="peek-session-info">${sessionInfo}</span>
      `;
    }

    // Update output
    if (outputContent) {
      if (response.output && response.output.trim()) {
        outputContent.textContent = response.output;
        // Scroll to bottom
        outputEl.scrollTop = outputEl.scrollHeight;
      } else {
        outputContent.textContent = '(No output available)';
      }
    }
  } catch (err) {
    if (statusEl) {
      statusEl.innerHTML = `
        <span class="peek-status-badge status-error">
          <span class="material-icons">error</span>
          Error
        </span>
      `;
    }
    if (outputContent) {
      outputContent.textContent = `Failed to fetch output: ${err.message}`;
    }
    console.error('[Peek] Failed to fetch output:', err);
  }
}

function startAutoRefresh() {
  if (peekAutoRefreshInterval) return;

  peekAutoRefreshInterval = setInterval(() => {
    if (currentPeekAgentId) {
      refreshPeekOutput(currentPeekAgentId);
    }
  }, 2000); // Refresh every 2 seconds
}

function stopAutoRefresh() {
  if (peekAutoRefreshInterval) {
    clearInterval(peekAutoRefreshInterval);
    peekAutoRefreshInterval = null;
  }
}

async function showAgentTranscript(agentId) {
  const parts = agentId.split('/');
  const rig = parts[0];
  const name = parts[1] || parts[0];

  // Show loading in a modal
  const loadingContent = `
    <div class="modal-header">
      <h2>
        <span class="material-icons">article</span>
        Transcript: ${escapeHtml(name)}
      </h2>
      <button class="btn btn-icon" data-modal-close>
        <span class="material-icons">close</span>
      </button>
    </div>
    <div class="modal-body transcript-body">
      <div class="transcript-loading">
        <span class="loading-spinner"></span>
        <p>Loading transcript...</p>
      </div>
    </div>
  `;
  const modal = showDynamicModal('transcript', loadingContent);

  try {
    const response = await api.getAgentTranscript(rig, name);

    // Build transcript content
    let transcriptHtml = '';

    // Claude session transcript files
    if (response.transcripts && response.transcripts.length > 0) {
      transcriptHtml += `
        <div class="transcript-section">
          <h3>
            <span class="material-icons">history</span>
            Claude Session Transcripts
          </h3>
          <div class="transcript-files">
            ${response.transcripts.map(t => `
              <div class="transcript-file">
                <div class="transcript-file-header">
                  <span class="material-icons">description</span>
                  <span class="transcript-filename">${escapeHtml(t.filename)}</span>
                  <span class="transcript-date">${t.modified ? new Date(t.modified).toLocaleString() : ''}</span>
                </div>
                <pre class="transcript-content">${escapeHtml(t.content || '(Empty)')}</pre>
              </div>
            `).join('')}
          </div>
        </div>
      `;
    }

    // Tmux session output
    if (response.output) {
      transcriptHtml += `
        <div class="transcript-section">
          <h3>
            <span class="material-icons">terminal</span>
            Recent Tmux Output
          </h3>
          <pre class="transcript-content tmux-output">${escapeHtml(response.output)}</pre>
        </div>
      `;
    }

    // No content found
    if (!transcriptHtml) {
      transcriptHtml = `
        <div class="transcript-empty">
          <span class="material-icons">info</span>
          <p>No transcript data found for this agent.</p>
          <p class="hint">Transcripts are created when Claude sessions are run.</p>
        </div>
      `;
    }

    // Update modal content
    const modalBody = modal.querySelector('.modal-body');
    if (modalBody) {
      modalBody.innerHTML = transcriptHtml;
    }

  } catch (err) {
    const modalBody = modal.querySelector('.modal-body');
    if (modalBody) {
      modalBody.innerHTML = `
        <div class="transcript-error">
          <span class="material-icons">error</span>
          <p>Failed to load transcript: ${escapeHtml(err.message)}</p>
        </div>
      `;
    }
    console.error('[Transcript] Failed to fetch:', err);
  }
}

// Note: escapeHtml and escapeAttr imported from ../utils/html.js
