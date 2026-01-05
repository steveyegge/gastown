/**
 * Gas Town GUI - Autocomplete Component
 *
 * Provides autocomplete functionality for inputs with search.
 * Supports both local and remote data sources.
 */

import { debounce as debounceFn } from '../utils/performance.js';

/**
 * Initialize autocomplete on an input element
 * @param {HTMLInputElement} input - Input element to attach autocomplete to
 * @param {Object} options - Configuration options
 * @param {Function} options.search - Async function that returns suggestions
 * @param {Function} options.renderItem - Function to render a suggestion item
 * @param {Function} options.onSelect - Callback when item is selected
 * @param {number} options.minChars - Minimum characters before searching (default: 2)
 * @param {number} options.debounceDelay - Debounce delay in ms (default: 200)
 */
export function initAutocomplete(input, options) {
  const {
    search,
    renderItem = defaultRenderItem,
    onSelect,
    minChars = 2,
    debounceDelay = 200,
  } = options;

  // Create dropdown container
  const dropdown = document.createElement('div');
  dropdown.className = 'autocomplete-dropdown hidden';
  input.parentElement.style.position = 'relative';
  input.parentElement.appendChild(dropdown);

  let selectedIndex = -1;
  let suggestions = [];

  // Debounced search handler
  const debouncedSearch = debounceFn(async (query) => {
    try {
      suggestions = await search(query);
      renderDropdown(suggestions);
    } catch (err) {
      console.error('[Autocomplete] Search error:', err);
      hideDropdown();
    }
  }, debounceDelay);

  // Input handler with debounce
  input.addEventListener('input', () => {
    const query = input.value.trim();

    if (query.length < minChars) {
      hideDropdown();
      return;
    }

    debouncedSearch(query);
  });

  // Keyboard navigation
  input.addEventListener('keydown', (e) => {
    if (!suggestions.length) return;

    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault();
        selectedIndex = Math.min(selectedIndex + 1, suggestions.length - 1);
        updateSelection();
        break;
      case 'ArrowUp':
        e.preventDefault();
        selectedIndex = Math.max(selectedIndex - 1, 0);
        updateSelection();
        break;
      case 'Enter':
        if (selectedIndex >= 0) {
          e.preventDefault();
          selectItem(suggestions[selectedIndex]);
        }
        break;
      case 'Escape':
        hideDropdown();
        break;
    }
  });

  // Hide on blur (with delay for click handling)
  input.addEventListener('blur', () => {
    setTimeout(hideDropdown, 200);
  });

  // Click on input shows dropdown if we have suggestions
  input.addEventListener('focus', () => {
    if (suggestions.length && input.value.length >= minChars) {
      showDropdown();
    }
  });

  function renderDropdown(items) {
    if (!items.length) {
      dropdown.innerHTML = '<div class="autocomplete-empty">No results found</div>';
      showDropdown();
      return;
    }

    selectedIndex = -1;
    dropdown.innerHTML = items.map((item, i) => {
      const html = renderItem(item);
      return `<div class="autocomplete-item" data-index="${i}">${html}</div>`;
    }).join('');

    // Click handlers
    dropdown.querySelectorAll('.autocomplete-item').forEach(el => {
      el.addEventListener('click', () => {
        const index = parseInt(el.dataset.index, 10);
        selectItem(items[index]);
      });
      el.addEventListener('mouseenter', () => {
        selectedIndex = parseInt(el.dataset.index, 10);
        updateSelection();
      });
    });

    showDropdown();
  }

  function updateSelection() {
    dropdown.querySelectorAll('.autocomplete-item').forEach((el, i) => {
      el.classList.toggle('selected', i === selectedIndex);
    });

    // Scroll into view
    const selected = dropdown.querySelector('.autocomplete-item.selected');
    if (selected) {
      selected.scrollIntoView({ block: 'nearest' });
    }
  }

  function selectItem(item) {
    if (onSelect) {
      onSelect(item, input);
    } else {
      input.value = item.value || item.id || item;
    }
    hideDropdown();
    suggestions = [];
  }

  function showDropdown() {
    dropdown.classList.remove('hidden');
  }

  function hideDropdown() {
    dropdown.classList.add('hidden');
    selectedIndex = -1;
  }

  // Return cleanup function
  return {
    destroy() {
      clearTimeout(debounceTimer);
      dropdown.remove();
    },
    refresh(newSuggestions) {
      suggestions = newSuggestions;
      renderDropdown(suggestions);
    }
  };
}

// Default item renderer
function defaultRenderItem(item) {
  if (typeof item === 'string') {
    return `<span>${escapeHtml(item)}</span>`;
  }
  return `
    <span class="autocomplete-label">${escapeHtml(item.label || item.name || item.id)}</span>
    ${item.description ? `<span class="autocomplete-desc">${escapeHtml(item.description)}</span>` : ''}
  `;
}

// Render function for beads/issues
export function renderBeadItem(item) {
  const statusClass = item.status || 'open';
  return `
    <div class="bead-item">
      <span class="bead-id">${escapeHtml(item.id)}</span>
      <span class="bead-title">${escapeHtml(item.title || item.name || '')}</span>
      <span class="bead-status status-${statusClass}">${statusClass}</span>
    </div>
  `;
}

// Render function for agents/targets
export function renderAgentItem(item) {
  const statusClass = item.status || 'idle';
  return `
    <div class="agent-item">
      <span class="agent-name">${escapeHtml(item.name || item.id)}</span>
      <span class="agent-path">${escapeHtml(item.path || '')}</span>
      <span class="agent-status status-${statusClass}">${statusClass}</span>
    </div>
  `;
}

// Utility function
function escapeHtml(str) {
  if (!str) return '';
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}
