/**
 * Gas Town GUI - Interactive Tutorial
 *
 * Step-by-step onboarding that guides users through actual operations.
 */

import { api } from '../api.js';
import { showToast } from './toast.js';

// Tutorial steps with actual actions
const TUTORIAL_STEPS = [
  {
    id: 'welcome',
    title: 'Welcome to Gas Town!',
    content: `
      <p>Gas Town is a <strong>multi-agent orchestrator</strong> for Claude Code.</p>
      <p>It helps you manage multiple AI agents working together on your codebase.</p>
      <div class="tutorial-concepts">
        <div class="concept-pill"><span class="material-icons">location_city</span> Town = Your workspace</div>
        <div class="concept-pill"><span class="material-icons">build</span> Rig = A project/repo</div>
        <div class="concept-pill"><span class="material-icons">group</span> Agents = AI workers</div>
      </div>
      <p>Let's walk through how it works!</p>
    `,
    action: null,
    highlight: null,
  },
  {
    id: 'agents',
    title: 'Meet Your Agents',
    content: `
      <p>Gas Town has different <strong>agent roles</strong>:</p>
      <div class="role-list">
        <div class="role-item role-mayor">
          <span class="material-icons">account_balance</span>
          <div>
            <strong>Mayor</strong>
            <span>Global coordinator - dispatches work across all projects</span>
          </div>
        </div>
        <div class="role-item role-deacon">
          <span class="material-icons">health_and_safety</span>
          <div>
            <strong>Deacon</strong>
            <span>Health monitor - watches over all agents</span>
          </div>
        </div>
        <div class="role-item role-polecat">
          <span class="material-icons">engineering</span>
          <div>
            <strong>Polecat</strong>
            <span>Ephemeral workers - spawned to do specific tasks</span>
          </div>
        </div>
        <div class="role-item role-witness">
          <span class="material-icons">visibility</span>
          <div>
            <strong>Witness</strong>
            <span>Per-rig coordinator - manages work within a project</span>
          </div>
        </div>
        <div class="role-item role-refinery">
          <span class="material-icons">merge_type</span>
          <div>
            <strong>Refinery</strong>
            <span>Merge queue manager - handles code integration</span>
          </div>
        </div>
      </div>
    `,
    action: null,
    highlight: '#agent-tree',
  },
  {
    id: 'sidebar',
    title: 'The Sidebar',
    content: `
      <p>The <strong>sidebar</strong> shows all your agents organized by role.</p>
      <p><strong>Try it:</strong> Click on any agent to see quick actions!</p>
      <ul>
        <li><span class="material-icons">notifications</span> <strong>Nudge</strong> - Ping an agent</li>
        <li><span class="material-icons">mail</span> <strong>Mail</strong> - Send a message</li>
        <li><span class="material-icons">open_in_new</span> <strong>View</strong> - See full details</li>
      </ul>
      <p>The colored dots show status: <span class="status-dot running"></span> running, <span class="status-dot working"></span> working, <span class="status-dot idle"></span> idle</p>
    `,
    action: null,
    highlight: '.sidebar',
  },
  {
    id: 'beads',
    title: 'Beads = Issues',
    content: `
      <p><strong>Beads</strong> are Git-tracked issues that agents work on.</p>
      <p>Each bead has:</p>
      <ul>
        <li>A unique ID (e.g., <code>gt-a3f2dd</code>)</li>
        <li>A title and description</li>
        <li>Status: pending → in_progress → done</li>
        <li>An assignee (the agent working on it)</li>
      </ul>
      <p>Beads are stored as JSONL files in your repo, so they're versioned with your code!</p>
    `,
    action: null,
    highlight: null,
  },
  {
    id: 'sling',
    title: 'Slinging Work',
    content: `
      <p><strong>Sling</strong> is how you assign work to agents.</p>
      <p>Think of it like "throwing" a task to a worker:</p>
      <div class="sling-demo">
        <div class="sling-from">
          <span class="material-icons">description</span>
          <span>Bead (issue)</span>
        </div>
        <div class="sling-arrow">
          <span class="material-icons">arrow_forward</span>
          <span>sling</span>
        </div>
        <div class="sling-to">
          <span class="material-icons">smart_toy</span>
          <span>Agent</span>
        </div>
      </div>
      <p>The agent catches the work on their <strong>hook</strong> and starts working!</p>
    `,
    action: null,
    highlight: '#sling-btn',
  },
  {
    id: 'convoys',
    title: 'Convoys = Batches',
    content: `
      <p>A <strong>Convoy</strong> groups related work items together.</p>
      <p>For example, a feature might have multiple beads:</p>
      <ul>
        <li>Design the API</li>
        <li>Implement backend</li>
        <li>Build frontend</li>
        <li>Write tests</li>
      </ul>
      <p>A convoy tracks progress across all these tasks and coordinates agents.</p>
    `,
    action: null,
    highlight: '#convoy-list',
  },
  {
    id: 'mail',
    title: 'Agent Communication',
    content: `
      <p>Agents communicate via <strong>mail</strong>:</p>
      <ul>
        <li><strong>Status updates</strong> - "I finished task X"</li>
        <li><strong>Questions</strong> - "Need clarification on Y"</li>
        <li><strong>Escalations</strong> - "I'm stuck, need help!"</li>
      </ul>
      <p>You (the human overseer) can also send mail to agents.</p>
      <p><strong>Tip:</strong> Click the <strong>Mail</strong> tab in the navigation bar, or press <kbd>3</kbd>.</p>
    `,
    action: null,
    highlight: null,
  },
  {
    id: 'keyboard',
    title: 'Keyboard Shortcuts',
    content: `
      <p>Speed up your workflow with shortcuts:</p>
      <div class="shortcut-grid">
        <div><kbd>1</kbd> Convoys</div>
        <div><kbd>2</kbd> Agents</div>
        <div><kbd>3</kbd> Mail</div>
        <div><kbd>?</kbd> Help</div>
        <div><kbd>Ctrl+N</kbd> New Convoy</div>
        <div><kbd>Ctrl+R</kbd> Refresh</div>
        <div><kbd>Esc</kbd> Close modal</div>
      </div>
    `,
    action: null,
    highlight: null,
  },
  {
    id: 'try-it',
    title: 'Try It Yourself!',
    content: `
      <p>You're ready to use Gas Town! Here's a quick workflow:</p>
      <ol class="workflow-checklist">
        <li>
          <span class="material-icons">check_circle</span>
          Create a bead: <code>bd new "Fix the login bug"</code>
        </li>
        <li>
          <span class="material-icons">check_circle</span>
          Click <strong>Sling</strong> to assign it to an agent
        </li>
        <li>
          <span class="material-icons">check_circle</span>
          Watch the agent work in the Activity feed
        </li>
        <li>
          <span class="material-icons">check_circle</span>
          Review the completed work
        </li>
      </ol>
      <p>Need help? Press <kbd>?</kbd> anytime or check the Help modal.</p>
    `,
    action: null,
    highlight: null,
  },
];

// Tutorial state
let currentStep = 0;
let tutorialModal = null;
let highlightOverlay = null;

/**
 * Start the tutorial
 */
export function startTutorial() {
  currentStep = 0;
  createTutorialModal();
  showStep(0);
}

/**
 * Create the tutorial modal
 */
function createTutorialModal() {
  // Remove existing
  const existing = document.getElementById('tutorial-modal');
  if (existing) existing.remove();

  tutorialModal = document.createElement('div');
  tutorialModal.id = 'tutorial-modal';
  tutorialModal.className = 'tutorial-modal';
  tutorialModal.innerHTML = `
    <div class="tutorial-content">
      <div class="tutorial-header">
        <div class="tutorial-progress">
          <span class="tutorial-step-num">1</span> / <span class="tutorial-total">${TUTORIAL_STEPS.length}</span>
        </div>
        <button class="tutorial-close" title="Skip tutorial">
          <span class="material-icons">close</span>
        </button>
      </div>
      <h2 class="tutorial-title"></h2>
      <div class="tutorial-body"></div>
      <div class="tutorial-footer">
        <button class="btn btn-secondary tutorial-prev" disabled>
          <span class="material-icons">arrow_back</span> Back
        </button>
        <div class="tutorial-dots"></div>
        <button class="btn btn-primary tutorial-next">
          Next <span class="material-icons">arrow_forward</span>
        </button>
      </div>
    </div>
  `;

  document.body.appendChild(tutorialModal);

  // Create highlight overlay
  highlightOverlay = document.createElement('div');
  highlightOverlay.id = 'tutorial-highlight';
  highlightOverlay.className = 'tutorial-highlight';
  document.body.appendChild(highlightOverlay);

  // Event listeners
  tutorialModal.querySelector('.tutorial-close').addEventListener('click', closeTutorial);
  tutorialModal.querySelector('.tutorial-prev').addEventListener('click', prevStep);
  tutorialModal.querySelector('.tutorial-next').addEventListener('click', nextStep);

  // Create dots
  const dotsContainer = tutorialModal.querySelector('.tutorial-dots');
  TUTORIAL_STEPS.forEach((_, i) => {
    const dot = document.createElement('button');
    dot.className = 'tutorial-dot';
    dot.dataset.step = i;
    dot.addEventListener('click', () => showStep(i));
    dotsContainer.appendChild(dot);
  });
}

/**
 * Show a specific step
 */
function showStep(stepIndex) {
  if (stepIndex < 0 || stepIndex >= TUTORIAL_STEPS.length) return;

  currentStep = stepIndex;
  const step = TUTORIAL_STEPS[stepIndex];

  // Update content
  tutorialModal.querySelector('.tutorial-step-num').textContent = stepIndex + 1;
  tutorialModal.querySelector('.tutorial-title').textContent = step.title;
  tutorialModal.querySelector('.tutorial-body').innerHTML = step.content;

  // Update buttons
  const prevBtn = tutorialModal.querySelector('.tutorial-prev');
  const nextBtn = tutorialModal.querySelector('.tutorial-next');
  prevBtn.disabled = stepIndex === 0;

  if (stepIndex === TUTORIAL_STEPS.length - 1) {
    nextBtn.innerHTML = 'Finish <span class="material-icons">check</span>';
  } else {
    nextBtn.innerHTML = 'Next <span class="material-icons">arrow_forward</span>';
  }

  // Update dots
  tutorialModal.querySelectorAll('.tutorial-dot').forEach((dot, i) => {
    dot.classList.toggle('active', i === stepIndex);
    dot.classList.toggle('completed', i < stepIndex);
  });

  // Highlight element if specified
  if (step.highlight) {
    const el = document.querySelector(step.highlight);
    if (el) {
      highlightElement(el);
    } else {
      hideHighlight();
    }
  } else {
    hideHighlight();
  }

  // Run action if specified
  if (step.action) {
    step.action();
  }
}

/**
 * Highlight an element
 */
function highlightElement(el) {
  const rect = el.getBoundingClientRect();
  const padding = 8;

  highlightOverlay.style.display = 'block';
  highlightOverlay.style.top = `${rect.top - padding}px`;
  highlightOverlay.style.left = `${rect.left - padding}px`;
  highlightOverlay.style.width = `${rect.width + padding * 2}px`;
  highlightOverlay.style.height = `${rect.height + padding * 2}px`;

  // Scroll element into view if needed
  el.scrollIntoView({ behavior: 'smooth', block: 'center' });
}

/**
 * Hide highlight
 */
function hideHighlight() {
  if (highlightOverlay) {
    highlightOverlay.style.display = 'none';
  }
}

/**
 * Next step
 */
function nextStep() {
  if (currentStep < TUTORIAL_STEPS.length - 1) {
    showStep(currentStep + 1);
  } else {
    closeTutorial();
    showToast('Tutorial complete! Press ? for help anytime.', 'success');
    localStorage.setItem('gastown-tutorial-complete', 'true');
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
 * Close tutorial
 */
function closeTutorial() {
  if (tutorialModal) {
    tutorialModal.remove();
    tutorialModal = null;
  }
  if (highlightOverlay) {
    highlightOverlay.remove();
    highlightOverlay = null;
  }
}

/**
 * Check if tutorial should auto-start
 */
export function shouldShowTutorial() {
  return !localStorage.getItem('gastown-tutorial-complete');
}

/**
 * Reset tutorial (for testing)
 */
export function resetTutorial() {
  localStorage.removeItem('gastown-tutorial-complete');
}
