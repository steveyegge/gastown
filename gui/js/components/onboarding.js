/**
 * Gas Town GUI - Onboarding Wizard
 *
 * Step-by-step guided setup for first-time users.
 * Actually performs actions, not just explains concepts.
 */

import { api } from '../api.js';
import { showToast } from './toast.js';
import { openModal } from './modals.js';

// Onboarding steps - each step has validation and action
const ONBOARDING_STEPS = [
  {
    id: 'welcome',
    title: 'Welcome to Gas Town',
    subtitle: 'Multi-Agent Orchestrator for Claude Code',
    icon: 'waving_hand',
    content: `
      <div class="onboard-welcome">
        <p class="onboard-lead">Gas Town helps you run multiple AI agents working together on your codebase.</p>
        <div class="onboard-flow">
          <div class="flow-step">
            <span class="flow-icon">📝</span>
            <span class="flow-label">Create Issue</span>
          </div>
          <div class="flow-arrow">→</div>
          <div class="flow-step">
            <span class="flow-icon">🚀</span>
            <span class="flow-label">Assign Work</span>
          </div>
          <div class="flow-arrow">→</div>
          <div class="flow-step">
            <span class="flow-icon">🤖</span>
            <span class="flow-label">Agent Works</span>
          </div>
          <div class="flow-arrow">→</div>
          <div class="flow-step">
            <span class="flow-icon">✅</span>
            <span class="flow-label">Done!</span>
          </div>
        </div>
        <p class="onboard-note">This wizard will guide you through your first workflow.</p>
      </div>
    `,
    validate: null,
    action: null,
  },
  {
    id: 'check-setup',
    title: 'Checking Your Setup',
    subtitle: 'Making sure everything is installed',
    icon: 'build_circle',
    content: `
      <div class="onboard-checks">
        <div class="check-item" data-check="gt">
          <span class="check-status"><span class="material-icons">hourglass_empty</span></span>
          <span class="check-label">Gas Town CLI (gt)</span>
          <span class="check-detail"></span>
        </div>
        <div class="check-item" data-check="bd">
          <span class="check-status"><span class="material-icons">hourglass_empty</span></span>
          <span class="check-label">Beads CLI (bd)</span>
          <span class="check-detail"></span>
        </div>
        <div class="check-item" data-check="workspace">
          <span class="check-status"><span class="material-icons">hourglass_empty</span></span>
          <span class="check-label">Workspace initialized</span>
          <span class="check-detail"></span>
        </div>
        <div class="check-item" data-check="rigs">
          <span class="check-status"><span class="material-icons">hourglass_empty</span></span>
          <span class="check-label">Projects (rigs) configured</span>
          <span class="check-detail"></span>
        </div>
      </div>
      <div class="onboard-setup-result hidden">
        <div class="setup-success hidden">
          <span class="material-icons">check_circle</span>
          <span>All systems ready!</span>
        </div>
        <div class="setup-issues hidden">
          <span class="material-icons">warning</span>
          <span>Some setup needed - we'll help you fix it.</span>
        </div>
      </div>
    `,
    validate: null,  // Informational only - don't block progress
    action: null,
  },
  {
    id: 'add-rig',
    title: 'Connect a Project',
    subtitle: 'Add your GitHub repository as a "rig"',
    icon: 'link',
    skipIf: async () => {
      const status = await api.getSetupStatus();
      return status.rigs && status.rigs.length > 0;
    },
    content: `
      <div class="onboard-form">
        <p class="onboard-explain">A <strong>rig</strong> is a project container. It connects your GitHub repo to Gas Town so agents can work on it.</p>
        <div class="form-group">
          <label for="onboard-rig-name">Project Name</label>
          <input type="text" id="onboard-rig-name" placeholder="e.g., my-app, backend, website" required autocomplete="off">
          <span class="form-hint">Short name for your project (lowercase, no spaces)</span>
        </div>
        <div class="form-group">
          <label for="onboard-rig-url">GitHub URL</label>
          <input type="text" id="onboard-rig-url" placeholder="https://github.com/you/repo.git" required autocomplete="off">
          <span class="form-hint">Full URL to your git repository</span>
        </div>
        <div class="onboard-action-result hidden"></div>
      </div>
    `,
    validate: () => {
      const name = document.getElementById('onboard-rig-name')?.value?.trim();
      const url = document.getElementById('onboard-rig-url')?.value?.trim();
      if (!name) return { valid: false, error: 'Please enter a project name' };
      if (!url) return { valid: false, error: 'Please enter a GitHub URL' };
      if (!/^[a-z0-9-]+$/.test(name)) return { valid: false, error: 'Name must be lowercase letters, numbers, and hyphens only' };
      if (!url.includes('github.com')) return { valid: false, error: 'Please enter a valid GitHub URL' };
      return { valid: true };
    },
    action: async () => {
      const name = document.getElementById('onboard-rig-name').value.trim();
      const url = document.getElementById('onboard-rig-url').value.trim();
      return await api.addRig(name, url);
    },
  },
  {
    id: 'create-bead',
    title: 'Create Your First Issue',
    subtitle: 'Beads are git-tracked issues that agents work on',
    icon: 'add_task',
    content: `
      <div class="onboard-form">
        <p class="onboard-explain">A <strong>bead</strong> is a work item (like a GitHub issue). Let's create your first one!</p>
        <div class="form-group">
          <label for="onboard-bead-title">What needs to be done?</label>
          <input type="text" id="onboard-bead-title" placeholder="e.g., Fix login bug, Add dark mode, Update README" required>
        </div>
        <div class="form-group">
          <label for="onboard-bead-desc">Description (optional)</label>
          <textarea id="onboard-bead-desc" rows="3" placeholder="More details about the task..."></textarea>
        </div>
        <div class="created-bead hidden">
          <span class="material-icons">check_circle</span>
          <span>Created: <code class="bead-id"></code></span>
        </div>
      </div>
    `,
    validate: () => {
      const title = document.getElementById('onboard-bead-title')?.value?.trim();
      if (!title) return { valid: false, error: 'Please enter a title for your issue' };
      return { valid: true };
    },
    action: async () => {
      const title = document.getElementById('onboard-bead-title').value.trim();
      const desc = document.getElementById('onboard-bead-desc')?.value?.trim() || '';
      const result = await api.createBead(title, { description: desc });
      if (result.success && result.bead_id) {
        window._onboardingBeadId = result.bead_id;
        const el = document.querySelector('.created-bead');
        if (el) {
          el.classList.remove('hidden');
          el.querySelector('.bead-id').textContent = result.bead_id;
        }
      }
      return result;
    },
  },
  {
    id: 'create-convoy',
    title: 'Track Your Work',
    subtitle: 'Convoys group related issues for tracking',
    icon: 'local_shipping',
    content: `
      <div class="onboard-form">
        <p class="onboard-explain">A <strong>convoy</strong> tracks progress across one or more issues. Think of it as your dashboard for a feature or task.</p>
        <div class="form-group">
          <label for="onboard-convoy-name">Convoy Name</label>
          <input type="text" id="onboard-convoy-name" placeholder="e.g., First Task, Bug Fix, New Feature" required>
        </div>
        <div class="form-group">
          <label>Issue to Track</label>
          <div class="selected-bead">
            <span class="material-icons">task_alt</span>
            <code id="onboard-convoy-bead"></code>
          </div>
        </div>
        <div class="created-convoy hidden">
          <span class="material-icons">check_circle</span>
          <span>Convoy created!</span>
        </div>
      </div>
    `,
    onShow: () => {
      const beadEl = document.getElementById('onboard-convoy-bead');
      if (beadEl && window._onboardingBeadId) {
        beadEl.textContent = window._onboardingBeadId;
      }
    },
    validate: () => {
      const name = document.getElementById('onboard-convoy-name')?.value?.trim();
      if (!name) return { valid: false, error: 'Please enter a convoy name' };
      return { valid: true };
    },
    action: async () => {
      const name = document.getElementById('onboard-convoy-name').value.trim();
      const beadId = window._onboardingBeadId;
      const result = await api.createConvoy(name, beadId ? [beadId] : []);
      if (result.success) {
        document.querySelector('.created-convoy')?.classList.remove('hidden');
      }
      return result;
    },
  },
  {
    id: 'sling-work',
    title: 'Assign to an Agent',
    subtitle: '"Sling" the work to a polecat (worker agent)',
    icon: 'send',
    content: `
      <div class="onboard-form">
        <p class="onboard-explain"><strong>Slinging</strong> assigns work to an agent. The agent will wake up, see the work on its "hook", and start working automatically.</p>
        <div class="form-group">
          <label>Issue to Sling</label>
          <div class="selected-bead">
            <span class="material-icons">task_alt</span>
            <code id="onboard-sling-bead"></code>
          </div>
        </div>
        <div class="form-group">
          <label for="onboard-sling-target">Target Project</label>
          <select id="onboard-sling-target" required>
            <option value="">Select a rig...</option>
          </select>
        </div>
        <div class="sling-preview hidden">
          <div class="sling-arrow-container">
            <div class="sling-from">
              <span class="material-icons">description</span>
              <span id="sling-preview-bead"></span>
            </div>
            <div class="sling-arrow">
              <span class="material-icons">arrow_forward</span>
            </div>
            <div class="sling-to">
              <span class="material-icons">smart_toy</span>
              <span id="sling-preview-target">Polecat</span>
            </div>
          </div>
        </div>
        <div class="sling-success hidden">
          <span class="material-icons">rocket_launch</span>
          <span>Work assigned! Agent will start working.</span>
        </div>
      </div>
    `,
    onShow: async () => {
      const beadEl = document.getElementById('onboard-sling-bead');
      if (beadEl && window._onboardingBeadId) {
        beadEl.textContent = window._onboardingBeadId;
      }
      // Populate targets dropdown
      const targetSelect = document.getElementById('onboard-sling-target');
      if (targetSelect) {
        try {
          const status = await api.getSetupStatus();
          const rigs = status.rigs || [];
          targetSelect.innerHTML = '<option value="">Select a rig...</option>';
          rigs.forEach(rig => {
            const opt = document.createElement('option');
            opt.value = rig.name || rig;
            opt.textContent = rig.name || rig;
            targetSelect.appendChild(opt);
          });
        } catch (e) {
          console.error('Failed to load rigs:', e);
        }
      }
    },
    validate: () => {
      const target = document.getElementById('onboard-sling-target')?.value;
      if (!target) return { valid: false, error: 'Please select a target project' };
      return { valid: true };
    },
    action: async () => {
      const beadId = window._onboardingBeadId;
      const target = document.getElementById('onboard-sling-target').value;
      if (!beadId) return { success: false, error: 'No bead to sling' };
      const result = await api.sling(beadId, target);
      if (result.success) {
        document.querySelector('.sling-success')?.classList.remove('hidden');
      }
      return result;
    },
  },
  {
    id: 'complete',
    title: 'You\'re All Set!',
    subtitle: 'Your first workflow is running',
    icon: 'celebration',
    content: `
      <div class="onboard-complete">
        <div class="complete-icon">🎉</div>
        <p class="complete-message">Congratulations! You've just:</p>
        <ul class="complete-list">
          <li><span class="material-icons">check</span> Created your first bead (issue)</li>
          <li><span class="material-icons">check</span> Set up a convoy to track it</li>
          <li><span class="material-icons">check</span> Assigned it to an agent</li>
        </ul>
        <div class="next-steps">
          <h4>What happens next?</h4>
          <p>The agent will work on your task. You can monitor progress in the <strong>Convoys</strong> dashboard.</p>
          <div class="tips">
            <div class="tip">
              <span class="material-icons">visibility</span>
              <span>Watch the <strong>Activity</strong> feed for real-time updates</span>
            </div>
            <div class="tip">
              <span class="material-icons">keyboard</span>
              <span>Press <kbd>?</kbd> anytime for keyboard shortcuts</span>
            </div>
            <div class="tip">
              <span class="material-icons">help</span>
              <span>Click <strong>Help</strong> for the full guide</span>
            </div>
          </div>
        </div>
      </div>
    `,
    validate: null,
    action: null,
  },
];

// State
let currentStep = 0;
let wizardModal = null;
let setupStatus = null;

/**
 * Check if onboarding should be shown
 */
export async function shouldShowOnboarding() {
  try {
    const status = await api.getSetupStatus();
    setupStatus = status;
    // Show onboarding if:
    // 1. Never completed onboarding before
    // 2. OR no rigs configured
    // 3. OR no convoys exist
    const completed = localStorage.getItem('gastown-onboarding-complete');
    if (completed) return false;

    return !status.rigs || status.rigs.length === 0;
  } catch {
    return false;
  }
}

/**
 * Start the onboarding wizard
 */
export function startOnboarding() {
  currentStep = 0;
  createWizardModal();
  showStep(0);
}

/**
 * Create the wizard modal
 */
function createWizardModal() {
  // Remove existing
  const existing = document.getElementById('onboarding-wizard');
  if (existing) existing.remove();

  wizardModal = document.createElement('div');
  wizardModal.id = 'onboarding-wizard';
  wizardModal.className = 'onboarding-wizard';
  wizardModal.innerHTML = `
    <div class="wizard-backdrop"></div>
    <div class="wizard-container">
      <div class="wizard-progress">
        ${ONBOARDING_STEPS.map((step, i) => `
          <div class="progress-step" data-step="${i}">
            <div class="progress-dot">
              <span class="material-icons">${step.icon}</span>
            </div>
            <span class="progress-label">${step.title}</span>
          </div>
        `).join('')}
      </div>
      <div class="wizard-content">
        <div class="wizard-header">
          <div class="wizard-icon">
            <span class="material-icons"></span>
          </div>
          <div class="wizard-titles">
            <h2 class="wizard-title"></h2>
            <p class="wizard-subtitle"></p>
          </div>
          <button class="wizard-close" title="Skip setup">
            <span class="material-icons">close</span>
          </button>
        </div>
        <div class="wizard-body"></div>
        <div class="wizard-error hidden">
          <span class="material-icons">error</span>
          <span class="error-message"></span>
        </div>
        <div class="wizard-footer">
          <button class="btn btn-secondary wizard-back" disabled>
            <span class="material-icons">arrow_back</span>
            Back
          </button>
          <div class="wizard-step-indicator">
            Step <span class="current-step">1</span> of <span class="total-steps">${ONBOARDING_STEPS.length}</span>
          </div>
          <button class="btn btn-primary wizard-next">
            Next
            <span class="material-icons">arrow_forward</span>
          </button>
        </div>
      </div>
    </div>
  `;

  document.body.appendChild(wizardModal);

  // Event listeners
  wizardModal.querySelector('.wizard-close').addEventListener('click', skipOnboarding);
  wizardModal.querySelector('.wizard-back').addEventListener('click', prevStep);
  wizardModal.querySelector('.wizard-next').addEventListener('click', nextStep);
  wizardModal.querySelector('.wizard-backdrop').addEventListener('click', (e) => {
    if (e.target === e.currentTarget) {
      // Don't close on backdrop click - they need to complete or skip
    }
  });
}

/**
 * Show a specific step
 */
async function showStep(stepIndex) {
  if (stepIndex < 0 || stepIndex >= ONBOARDING_STEPS.length) return;

  const step = ONBOARDING_STEPS[stepIndex];

  // Check if step should be skipped
  if (step.skipIf) {
    const shouldSkip = await step.skipIf();
    if (shouldSkip) {
      // Skip to next step
      if (stepIndex < ONBOARDING_STEPS.length - 1) {
        showStep(stepIndex + 1);
        return;
      }
    }
  }

  currentStep = stepIndex;

  // Update progress
  wizardModal.querySelectorAll('.progress-step').forEach((el, i) => {
    el.classList.toggle('active', i === stepIndex);
    el.classList.toggle('completed', i < stepIndex);
  });

  // Update header
  wizardModal.querySelector('.wizard-icon .material-icons').textContent = step.icon;
  wizardModal.querySelector('.wizard-title').textContent = step.title;
  wizardModal.querySelector('.wizard-subtitle').textContent = step.subtitle;

  // Update body
  wizardModal.querySelector('.wizard-body').innerHTML = step.content;

  // Update footer
  wizardModal.querySelector('.current-step').textContent = stepIndex + 1;
  wizardModal.querySelector('.wizard-back').disabled = stepIndex === 0;

  const nextBtn = wizardModal.querySelector('.wizard-next');
  if (stepIndex === ONBOARDING_STEPS.length - 1) {
    nextBtn.innerHTML = 'Finish <span class="material-icons">check</span>';
  } else if (step.action) {
    nextBtn.innerHTML = 'Continue <span class="material-icons">arrow_forward</span>';
  } else {
    nextBtn.innerHTML = 'Next <span class="material-icons">arrow_forward</span>';
  }

  // Clear error
  hideError();

  // Run onShow callback
  if (step.onShow) {
    await step.onShow();
  }

  // Run validation for check-setup step
  if (step.id === 'check-setup') {
    await runSetupChecks();
  }
}

/**
 * Run setup checks
 */
async function runSetupChecks() {
  const checks = {
    gt: { status: 'checking', detail: '' },
    bd: { status: 'checking', detail: '' },
    workspace: { status: 'checking', detail: '' },
    rigs: { status: 'checking', detail: '' },
  };

  const updateCheck = (id, status, detail = '') => {
    const el = wizardModal.querySelector(`[data-check="${id}"]`);
    if (!el) return;

    const iconEl = el.querySelector('.check-status .material-icons');
    const detailEl = el.querySelector('.check-detail');

    checks[id] = { status, detail };

    if (status === 'checking') {
      iconEl.textContent = 'hourglass_empty';
      el.className = 'check-item checking';
    } else if (status === 'ok') {
      iconEl.textContent = 'check_circle';
      el.className = 'check-item success';
    } else if (status === 'warning') {
      iconEl.textContent = 'warning';
      el.className = 'check-item warning';
    } else if (status === 'error') {
      iconEl.textContent = 'error';
      el.className = 'check-item error';
    }

    detailEl.textContent = detail;
  };

  try {
    const status = await api.getSetupStatus();
    setupStatus = status;

    // Check gt
    await delay(200);
    if (status.gt_installed) {
      updateCheck('gt', 'ok', status.gt_version || 'Installed');
    } else {
      updateCheck('gt', 'error', 'Not found');
    }

    // Check bd
    await delay(200);
    if (status.bd_installed) {
      updateCheck('bd', 'ok', status.bd_version || 'Installed');
    } else {
      updateCheck('bd', 'error', 'Not found');
    }

    // Check workspace
    await delay(200);
    if (status.workspace_initialized) {
      updateCheck('workspace', 'ok', status.workspace_path || '~/gt');
    } else {
      updateCheck('workspace', 'warning', 'Run: gt install ~/gt');
    }

    // Check rigs
    await delay(200);
    if (status.rigs && status.rigs.length > 0) {
      updateCheck('rigs', 'ok', `${status.rigs.length} project(s)`);
    } else {
      updateCheck('rigs', 'warning', 'No projects yet');
    }

    // Show result
    const resultEl = wizardModal.querySelector('.onboard-setup-result');
    resultEl?.classList.remove('hidden');

    const allOk = Object.values(checks).every(c => c.status === 'ok');
    wizardModal.querySelector('.setup-success')?.classList.toggle('hidden', !allOk);
    wizardModal.querySelector('.setup-issues')?.classList.toggle('hidden', allOk);

    return { valid: true };
  } catch (err) {
    updateCheck('gt', 'error', 'Check failed');
    updateCheck('bd', 'error', 'Check failed');
    updateCheck('workspace', 'error', 'Check failed');
    updateCheck('rigs', 'error', 'Check failed');
    return { valid: false, error: err.message };
  }
}

/**
 * Next step
 */
async function nextStep() {
  const step = ONBOARDING_STEPS[currentStep];
  const nextBtn = wizardModal.querySelector('.wizard-next');

  // Validate current step
  if (step.validate) {
    const result = await step.validate();
    if (!result.valid) {
      showError(result.error || 'Validation failed');
      return;
    }
  }

  // Run action if present
  if (step.action) {
    nextBtn.disabled = true;
    nextBtn.innerHTML = '<span class="material-icons spinning">sync</span> Working...';

    try {
      const result = await step.action();
      if (result && !result.success) {
        showError(result.error || 'Action failed');
        nextBtn.disabled = false;
        nextBtn.innerHTML = 'Retry <span class="material-icons">refresh</span>';
        return;
      }
    } catch (err) {
      showError(err.message);
      nextBtn.disabled = false;
      nextBtn.innerHTML = 'Retry <span class="material-icons">refresh</span>';
      return;
    }

    nextBtn.disabled = false;
  }

  // Move to next step or finish
  if (currentStep < ONBOARDING_STEPS.length - 1) {
    showStep(currentStep + 1);
  } else {
    completeOnboarding();
  }
}

/**
 * Previous step
 */
function prevStep() {
  if (currentStep > 0) {
    showStep(currentStep - 1);
  }
}

/**
 * Show error
 */
function showError(message) {
  const errorEl = wizardModal.querySelector('.wizard-error');
  errorEl.querySelector('.error-message').textContent = message;
  errorEl.classList.remove('hidden');
}

/**
 * Hide error
 */
function hideError() {
  wizardModal.querySelector('.wizard-error')?.classList.add('hidden');
}

/**
 * Complete onboarding
 */
function completeOnboarding() {
  localStorage.setItem('gastown-onboarding-complete', 'true');
  closeWizard();
  showToast('Setup complete! Welcome to Gas Town.', 'success');

  // Refresh the main view
  document.dispatchEvent(new CustomEvent('onboarding:complete'));
}

/**
 * Skip onboarding
 */
function skipOnboarding() {
  if (confirm('Skip the setup wizard? You can access it later from the Help menu.')) {
    localStorage.setItem('gastown-onboarding-skipped', 'true');
    closeWizard();
  }
}

/**
 * Close wizard
 */
function closeWizard() {
  if (wizardModal) {
    wizardModal.remove();
    wizardModal = null;
  }
}

/**
 * Reset onboarding (for testing)
 */
export function resetOnboarding() {
  localStorage.removeItem('gastown-onboarding-complete');
  localStorage.removeItem('gastown-onboarding-skipped');
}

// Utility
function delay(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}
