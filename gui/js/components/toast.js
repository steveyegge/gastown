/**
 * Gas Town GUI - Toast Notification Component
 *
 * Provides non-intrusive notifications for user feedback.
 */

// Toast container reference
let toastContainer = null;

// Default durations by type (ms)
const TOAST_DURATIONS = {
  success: 3000,
  error: 5000,
  warning: 4000,
  info: 3000,
};

// Icons by type
const TOAST_ICONS = {
  success: 'check_circle',
  error: 'error',
  warning: 'warning',
  info: 'info',
};

/**
 * Initialize the toast container
 */
function ensureContainer() {
  if (!toastContainer) {
    toastContainer = document.getElementById('toast-container');
    if (!toastContainer) {
      toastContainer = document.createElement('div');
      toastContainer.id = 'toast-container';
      toastContainer.className = 'toast-container';
      document.body.appendChild(toastContainer);
    }
  }
  return toastContainer;
}

/**
 * Show a toast notification
 * @param {string} message - The message to display
 * @param {string} type - Toast type: 'success', 'error', 'warning', 'info'
 * @param {number} duration - Optional duration in ms (0 = persistent)
 * @returns {HTMLElement} The toast element
 */
export function showToast(message, type = 'info', duration = null) {
  const container = ensureContainer();
  const icon = TOAST_ICONS[type] || TOAST_ICONS.info;
  const finalDuration = duration !== null ? duration : TOAST_DURATIONS[type] || 3000;

  // Create toast element
  const toast = document.createElement('div');
  toast.className = `toast toast-${type}`;
  toast.innerHTML = `
    <span class="material-icons toast-icon">${icon}</span>
    <span class="toast-message">${escapeHtml(message)}</span>
    <button class="toast-close" aria-label="Close">
      <span class="material-icons">close</span>
    </button>
  `;

  // Add close handler
  const closeBtn = toast.querySelector('.toast-close');
  closeBtn.addEventListener('click', () => dismissToast(toast));

  // Add to container
  container.appendChild(toast);

  // Trigger entrance animation
  requestAnimationFrame(() => {
    toast.classList.add('show');
  });

  // Auto-dismiss if duration > 0
  if (finalDuration > 0) {
    setTimeout(() => dismissToast(toast), finalDuration);
  }

  return toast;
}

/**
 * Dismiss a toast notification
 * @param {HTMLElement} toast - The toast element to dismiss
 */
export function dismissToast(toast) {
  if (!toast || !toast.parentNode) return;

  // Trigger exit animation
  toast.classList.remove('show');
  toast.classList.add('hide');

  // Remove after animation
  setTimeout(() => {
    if (toast.parentNode) {
      toast.parentNode.removeChild(toast);
    }
  }, 300);
}

/**
 * Clear all toasts
 */
export function clearAllToasts() {
  const container = ensureContainer();
  const toasts = container.querySelectorAll('.toast');
  toasts.forEach(toast => dismissToast(toast));
}

/**
 * Show a success toast
 */
export function showSuccess(message, duration = null) {
  return showToast(message, 'success', duration);
}

/**
 * Show an error toast
 */
export function showError(message, duration = null) {
  return showToast(message, 'error', duration);
}

/**
 * Show a warning toast
 */
export function showWarning(message, duration = null) {
  return showToast(message, 'warning', duration);
}

/**
 * Show an info toast
 */
export function showInfo(message, duration = null) {
  return showToast(message, 'info', duration);
}

/**
 * Show a loading toast (persistent until dismissed)
 * @param {string} message - The loading message
 * @returns {Object} Object with dismiss() method
 */
export function showLoading(message) {
  const container = ensureContainer();

  const toast = document.createElement('div');
  toast.className = 'toast toast-loading';
  toast.innerHTML = `
    <span class="material-icons toast-icon spin">sync</span>
    <span class="toast-message">${escapeHtml(message)}</span>
  `;

  container.appendChild(toast);

  requestAnimationFrame(() => {
    toast.classList.add('show');
  });

  return {
    dismiss: () => dismissToast(toast),
    update: (newMessage) => {
      const msgEl = toast.querySelector('.toast-message');
      if (msgEl) msgEl.textContent = newMessage;
    },
    success: (newMessage) => {
      toast.className = 'toast toast-success show';
      toast.innerHTML = `
        <span class="material-icons toast-icon">check_circle</span>
        <span class="toast-message">${escapeHtml(newMessage)}</span>
      `;
      setTimeout(() => dismissToast(toast), 2000);
    },
    error: (newMessage) => {
      toast.className = 'toast toast-error show';
      toast.innerHTML = `
        <span class="material-icons toast-icon">error</span>
        <span class="toast-message">${escapeHtml(newMessage)}</span>
      `;
      setTimeout(() => dismissToast(toast), 4000);
    },
  };
}

// Utility function
function escapeHtml(str) {
  if (!str) return '';
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}
