/**
 * Gas Town GUI - Tooltip Utility
 *
 * Positions tooltips properly using fixed positioning to avoid z-index issues.
 */

// Track current tooltip element
let currentTooltip = null;

/**
 * Initialize tooltip positioning for all [data-tooltip] elements
 */
export function initTooltips() {
  // Use event delegation for tooltips
  document.addEventListener('mouseover', handleMouseOver);
  document.addEventListener('mouseout', handleMouseOut);
}

/**
 * Handle mouse over to position tooltip
 */
function handleMouseOver(e) {
  const el = e.target.closest('[data-tooltip]');
  if (!el || !el.dataset.tooltip) return;

  currentTooltip = el;
  positionTooltip(el);
}

/**
 * Handle mouse out to hide tooltip
 */
function handleMouseOut(e) {
  const el = e.target.closest('[data-tooltip]');
  if (!el) return;

  currentTooltip = null;
}

/**
 * Position the tooltip based on the element's position
 */
function positionTooltip(el) {
  const rect = el.getBoundingClientRect();
  const tooltip = el.querySelector('::after');

  // Calculate best position
  const spaceAbove = rect.top;
  const spaceBelow = window.innerHeight - rect.bottom;
  const spaceLeft = rect.left;
  const spaceRight = window.innerWidth - rect.right;

  // Create a style element for this specific tooltip position
  const styleId = 'tooltip-position-style';
  let styleEl = document.getElementById(styleId);
  if (!styleEl) {
    styleEl = document.createElement('style');
    styleEl.id = styleId;
    document.head.appendChild(styleEl);
  }

  // Calculate tooltip position (prefer above, fall back to below)
  let top, left;
  const tooltipHeight = 60; // Approximate height
  const tooltipWidth = 200; // Max width from CSS

  if (spaceAbove > tooltipHeight + 10) {
    // Position above
    top = rect.top - tooltipHeight - 10;
  } else {
    // Position below
    top = rect.bottom + 10;
  }

  // Center horizontally, but keep on screen
  left = rect.left + (rect.width / 2) - (tooltipWidth / 2);
  left = Math.max(10, Math.min(left, window.innerWidth - tooltipWidth - 10));

  // Apply position via CSS custom properties
  el.style.setProperty('--tooltip-top', `${top}px`);
  el.style.setProperty('--tooltip-left', `${left}px`);

  // Update the global style for ::after positioning
  styleEl.textContent = `
    [data-tooltip]:hover::after {
      top: var(--tooltip-top, auto);
      left: var(--tooltip-left, 50%);
      bottom: auto;
      transform: none;
    }
    [data-tooltip]:hover::before {
      display: none; /* Hide arrow for now since positioning is complex */
    }
  `;
}

/**
 * Manually show a tooltip at a specific position
 */
export function showTooltipAt(text, x, y) {
  const existing = document.getElementById('dynamic-tooltip');
  if (existing) existing.remove();

  const tooltip = document.createElement('div');
  tooltip.id = 'dynamic-tooltip';
  tooltip.className = 'dynamic-tooltip';
  tooltip.textContent = text;
  tooltip.style.cssText = `
    position: fixed;
    top: ${y}px;
    left: ${x}px;
    padding: 8px 12px;
    font-size: 12px;
    background: var(--bg-elevated);
    border: 1px solid var(--border-default);
    border-radius: 6px;
    box-shadow: 0 4px 12px rgba(0,0,0,0.3);
    z-index: 9999;
    pointer-events: none;
    max-width: 280px;
    animation: fadeIn 0.15s ease;
  `;

  document.body.appendChild(tooltip);

  // Adjust position if off-screen
  const rect = tooltip.getBoundingClientRect();
  if (rect.right > window.innerWidth) {
    tooltip.style.left = `${window.innerWidth - rect.width - 10}px`;
  }
  if (rect.bottom > window.innerHeight) {
    tooltip.style.top = `${y - rect.height - 10}px`;
  }

  return tooltip;
}

/**
 * Hide dynamic tooltip
 */
export function hideTooltip() {
  const existing = document.getElementById('dynamic-tooltip');
  if (existing) existing.remove();
}
