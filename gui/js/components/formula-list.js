/**
 * Gas Town GUI - Formula List Component
 *
 * Renders workflow formulas (templates) with actions to view, use, and create.
 */

import { api } from '../api.js';
import { showToast } from './toast.js';

let container = null;
let formulas = [];

/**
 * Initialize the formula list component
 */
export function initFormulaList() {
  container = document.getElementById('formula-list-container');
  if (!container) return;

  // Refresh button
  const refreshBtn = document.getElementById('formula-refresh');
  if (refreshBtn) {
    refreshBtn.addEventListener('click', () => loadFormulas());
  }

  // New formula button - open modal
  const newBtn = document.getElementById('new-formula-btn');
  if (newBtn) {
    newBtn.addEventListener('click', () => {
      document.getElementById('modal-overlay').classList.remove('hidden');
      document.getElementById('new-formula-modal').classList.remove('hidden');
    });
  }

  // New formula form
  const form = document.getElementById('new-formula-form');
  if (form) {
    form.addEventListener('submit', handleCreateFormula);
  }

  // Use formula form
  const useForm = document.getElementById('use-formula-form');
  if (useForm) {
    useForm.addEventListener('submit', handleUseFormula);
  }
}

/**
 * Load formulas from API
 */
export async function loadFormulas() {
  if (!container) {
    container = document.getElementById('formula-list-container');
  }
  if (!container) return;

  container.innerHTML = '<div class="loading-state"><span class="loading-spinner"></span> Loading formulas...</div>';

  try {
    formulas = await api.getFormulas();
    renderFormulas();
  } catch (err) {
    console.error('[Formulas] Load error:', err);
    container.innerHTML = `
      <div class="error-state">
        <span class="material-icons">error_outline</span>
        <p>Failed to load formulas: ${escapeHtml(err.message)}</p>
        <button class="btn btn-secondary" onclick="window.location.reload()">Retry</button>
      </div>
    `;
  }
}

/**
 * Render formula cards
 */
function renderFormulas() {
  if (!formulas || formulas.length === 0) {
    container.innerHTML = `
      <div class="empty-state">
        <span class="material-icons empty-icon">science</span>
        <h3>No Formulas</h3>
        <p>Create workflow templates to quickly spawn repeatable tasks</p>
        <button class="btn btn-primary" id="create-first-formula">
          <span class="material-icons">add</span>
          Create Formula
        </button>
      </div>
    `;
    const btn = container.querySelector('#create-first-formula');
    if (btn) {
      btn.addEventListener('click', () => {
        document.getElementById('modal-overlay').classList.remove('hidden');
        document.getElementById('new-formula-modal').classList.remove('hidden');
      });
    }
    return;
  }

  container.innerHTML = '';
  formulas.forEach((formula, index) => {
    const card = createFormulaCard(formula, index);
    container.appendChild(card);
  });
}

/**
 * Create a formula card element
 */
function createFormulaCard(formula, index) {
  const card = document.createElement('div');
  card.className = `formula-card animate-spawn stagger-${Math.min(index, 6)}`;
  card.dataset.formulaName = formula.name;

  const description = formula.description || 'No description';
  const templatePreview = formula.template ? formula.template.substring(0, 100) + (formula.template.length > 100 ? '...' : '') : 'No template';

  card.innerHTML = `
    <div class="formula-header">
      <div class="formula-icon">
        <span class="material-icons">science</span>
      </div>
      <div class="formula-info">
        <h3 class="formula-name">${escapeHtml(formula.name)}</h3>
        <p class="formula-description">${escapeHtml(description)}</p>
      </div>
    </div>
    <div class="formula-template">
      <code>${escapeHtml(templatePreview)}</code>
    </div>
    <div class="formula-actions">
      <button class="btn btn-sm btn-secondary" data-action="view" title="View full template">
        <span class="material-icons">visibility</span>
        View
      </button>
      <button class="btn btn-sm btn-primary" data-action="use" title="Use this formula">
        <span class="material-icons">play_arrow</span>
        Use
      </button>
    </div>
  `;

  // Add event listeners
  card.querySelector('[data-action="view"]').addEventListener('click', () => showFormulaDetails(formula));
  card.querySelector('[data-action="use"]').addEventListener('click', () => showUseFormulaModal(formula));

  return card;
}

/**
 * Show formula details in a modal or toast
 */
async function showFormulaDetails(formula) {
  try {
    const details = await api.getFormula(formula.name);

    // Create detail view
    const detailHtml = `
      <div class="formula-detail">
        <h3>${escapeHtml(details.name || formula.name)}</h3>
        <p class="description">${escapeHtml(details.description || 'No description')}</p>
        <div class="template-section">
          <h4>Template</h4>
          <pre class="template-code">${escapeHtml(details.template || 'No template defined')}</pre>
        </div>
        ${details.args ? `
          <div class="args-section">
            <h4>Arguments</h4>
            <pre class="args-code">${escapeHtml(JSON.stringify(details.args, null, 2))}</pre>
          </div>
        ` : ''}
      </div>
    `;

    // Use peek modal to display
    const peekModal = document.getElementById('peek-modal');
    const peekName = document.getElementById('peek-agent-name');
    const peekOutput = document.getElementById('peek-output');
    const peekStatus = document.getElementById('peek-status');

    if (peekModal && peekName && peekOutput) {
      peekName.textContent = `Formula: ${formula.name}`;
      peekStatus.innerHTML = '<span class="status-indicator running"></span><span class="status-text">Formula Details</span>';
      peekOutput.querySelector('.output-content').innerHTML = detailHtml;

      document.getElementById('modal-overlay').classList.remove('hidden');
      peekModal.classList.remove('hidden');
    }
  } catch (err) {
    showToast(`Failed to load formula: ${err.message}`, 'error');
  }
}

/**
 * Show use formula modal
 */
function showUseFormulaModal(formula) {
  const modal = document.getElementById('use-formula-modal');
  const nameInput = document.getElementById('use-formula-name');
  const targetSelect = document.getElementById('use-formula-target');

  if (modal && nameInput) {
    nameInput.value = formula.name;

    // Populate targets
    populateTargets(targetSelect);

    document.getElementById('modal-overlay').classList.remove('hidden');
    modal.classList.remove('hidden');
  }
}

/**
 * Populate target select with available rigs
 */
async function populateTargets(selectEl) {
  if (!selectEl) return;

  try {
    const targets = await api.getTargets();
    selectEl.innerHTML = '<option value="">Select target...</option>';

    if (Array.isArray(targets)) {
      targets.forEach(t => {
        const opt = document.createElement('option');
        opt.value = t.address || t.name || t;
        opt.textContent = t.name || t.address || t;
        selectEl.appendChild(opt);
      });
    }
  } catch (err) {
    console.error('[Formulas] Failed to load targets:', err);
  }
}

/**
 * Handle create formula form submission
 */
async function handleCreateFormula(e) {
  e.preventDefault();

  const form = e.target;
  const name = form.querySelector('#formula-name').value.trim();
  const description = form.querySelector('#formula-description').value.trim();
  const template = form.querySelector('#formula-template').value.trim();

  if (!name || !template) {
    showToast('Name and template are required', 'error');
    return;
  }

  const submitBtn = form.querySelector('[type="submit"]');
  const originalText = submitBtn.innerHTML;
  submitBtn.innerHTML = '<span class="material-icons spinning">sync</span> Creating...';
  submitBtn.disabled = true;

  try {
    await api.createFormula(name, description, template);
    showToast(`Formula "${name}" created`, 'success');

    // Close modal
    document.getElementById('modal-overlay').classList.add('hidden');
    document.getElementById('new-formula-modal').classList.add('hidden');
    form.reset();

    // Reload list
    await loadFormulas();
  } catch (err) {
    showToast(`Failed to create formula: ${err.message}`, 'error');
  } finally {
    submitBtn.innerHTML = originalText;
    submitBtn.disabled = false;
  }
}

/**
 * Handle use formula form submission
 */
async function handleUseFormula(e) {
  e.preventDefault();

  const form = e.target;
  const name = form.querySelector('#use-formula-name').value.trim();
  const target = form.querySelector('#use-formula-target').value;
  const args = form.querySelector('#use-formula-args').value.trim();

  if (!name || !target) {
    showToast('Formula name and target are required', 'error');
    return;
  }

  const submitBtn = form.querySelector('[type="submit"]');
  const originalText = submitBtn.innerHTML;
  submitBtn.innerHTML = '<span class="material-icons spinning">sync</span> Using...';
  submitBtn.disabled = true;

  try {
    await api.useFormula(name, target, args || undefined);
    showToast(`Formula "${name}" applied to ${target}`, 'success');

    // Close modal
    document.getElementById('modal-overlay').classList.add('hidden');
    document.getElementById('use-formula-modal').classList.add('hidden');
    form.reset();
  } catch (err) {
    showToast(`Failed to use formula: ${err.message}`, 'error');
  } finally {
    submitBtn.innerHTML = originalText;
    submitBtn.disabled = false;
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
