/**
 * Gas Town GUI - HTML Utilities
 *
 * Shared utilities for HTML escaping and string manipulation.
 * Centralized to avoid duplication across components.
 */

// Reusable element for escaping (avoids creating new elements each time)
const escapeEl = document.createElement('div');

/**
 * Escape HTML entities to prevent XSS
 * @param {string} str - String to escape
 * @returns {string} - Escaped string safe for innerHTML
 */
export function escapeHtml(str) {
  if (!str) return '';
  escapeEl.textContent = str;
  return escapeEl.innerHTML;
}

/**
 * Escape string for use in HTML attributes
 * @param {string} str - String to escape
 * @returns {string} - Escaped string safe for attributes
 */
export function escapeAttr(str) {
  if (!str) return '';
  return str
    .replace(/&/g, '&amp;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
}

/**
 * Truncate string to specified length
 * @param {string} str - String to truncate
 * @param {number} length - Max length
 * @returns {string} - Truncated string with ellipsis if needed
 */
export function truncate(str, length) {
  if (!str) return '';
  return str.length > length ? str.slice(0, length) + '...' : str;
}

/**
 * Capitalize first letter of string
 * @param {string} str - String to capitalize
 * @returns {string} - Capitalized string
 */
export function capitalize(str) {
  if (!str) return '';
  return str.charAt(0).toUpperCase() + str.slice(1);
}

/**
 * Format a number with commas
 * @param {number} num - Number to format
 * @returns {string} - Formatted number string
 */
export function formatNumber(num) {
  if (num === null || num === undefined) return '0';
  return num.toLocaleString();
}

/**
 * Wrap an async operation with button loading state
 * Shows spinner while operation runs, restores original content when done.
 * @param {HTMLButtonElement} btn - Button element
 * @param {Function} asyncFn - Async function to execute
 * @returns {Promise<any>} - Result of the async function
 */
export async function withButtonLoading(btn, asyncFn) {
  if (!btn) return asyncFn();

  const originalContent = btn.innerHTML;
  btn.innerHTML = '<span class="material-icons spinning">sync</span>';
  btn.disabled = true;

  try {
    return await asyncFn();
  } finally {
    btn.innerHTML = originalContent;
    btn.disabled = false;
  }
}
