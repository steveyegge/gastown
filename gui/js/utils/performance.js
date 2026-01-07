/**
 * Gas Town GUI - Performance Utilities
 *
 * Utilities for optimizing rendering and reducing jank.
 */

/**
 * Debounce a function
 * @param {Function} fn - Function to debounce
 * @param {number} delay - Delay in ms
 */
export function debounce(fn, delay = 200) {
  let timeoutId;
  return (...args) => {
    clearTimeout(timeoutId);
    timeoutId = setTimeout(() => fn(...args), delay);
  };
}

/**
 * Throttle a function
 * @param {Function} fn - Function to throttle
 * @param {number} limit - Minimum time between calls in ms
 */
export function throttle(fn, limit = 100) {
  let inThrottle;
  return (...args) => {
    if (!inThrottle) {
      fn(...args);
      inThrottle = true;
      setTimeout(() => inThrottle = false, limit);
    }
  };
}

/**
 * Request idle callback with fallback
 * @param {Function} callback - Callback to run when idle
 * @param {number} timeout - Max wait time in ms
 */
export function whenIdle(callback, timeout = 2000) {
  if ('requestIdleCallback' in window) {
    return requestIdleCallback(callback, { timeout });
  }
  return setTimeout(callback, 1);
}

/**
 * Batch DOM reads and writes to avoid layout thrashing
 */
class DOMBatcher {
  constructor() {
    this.readQueue = [];
    this.writeQueue = [];
    this.scheduled = false;
  }

  read(fn) {
    this.readQueue.push(fn);
    this.schedule();
    return this;
  }

  write(fn) {
    this.writeQueue.push(fn);
    this.schedule();
    return this;
  }

  schedule() {
    if (!this.scheduled) {
      this.scheduled = true;
      requestAnimationFrame(() => this.flush());
    }
  }

  flush() {
    // Execute all reads first
    while (this.readQueue.length) {
      this.readQueue.shift()();
    }
    // Then execute all writes
    while (this.writeQueue.length) {
      this.writeQueue.shift()();
    }
    this.scheduled = false;
  }
}

export const domBatcher = new DOMBatcher();

/**
 * Create a virtual scroll container
 * Only renders visible items for large lists
 */
export class VirtualScroller {
  constructor(container, options = {}) {
    this.container = container;
    this.itemHeight = options.itemHeight || 50;
    this.buffer = options.buffer || 5;
    this.items = [];
    this.renderItem = options.renderItem || (item => `<div>${item}</div>`);

    this.viewport = document.createElement('div');
    this.viewport.className = 'virtual-scroll-viewport';
    this.viewport.style.cssText = 'overflow-y: auto; height: 100%;';

    this.content = document.createElement('div');
    this.content.className = 'virtual-scroll-content';

    this.viewport.appendChild(this.content);
    this.container.appendChild(this.viewport);

    this.viewport.addEventListener('scroll', throttle(() => this.render(), 16));
  }

  setItems(items) {
    this.items = items;
    this.content.style.height = `${items.length * this.itemHeight}px`;
    this.render();
  }

  render() {
    const scrollTop = this.viewport.scrollTop;
    const viewportHeight = this.viewport.clientHeight;

    const startIndex = Math.max(0, Math.floor(scrollTop / this.itemHeight) - this.buffer);
    const endIndex = Math.min(
      this.items.length,
      Math.ceil((scrollTop + viewportHeight) / this.itemHeight) + this.buffer
    );

    const fragment = document.createDocumentFragment();

    for (let i = startIndex; i < endIndex; i++) {
      const item = this.items[i];
      const itemEl = document.createElement('div');
      itemEl.className = 'virtual-scroll-item';
      itemEl.style.cssText = `
        position: absolute;
        top: ${i * this.itemHeight}px;
        left: 0;
        right: 0;
        height: ${this.itemHeight}px;
      `;
      itemEl.innerHTML = this.renderItem(item, i);
      fragment.appendChild(itemEl);
    }

    this.content.innerHTML = '';
    this.content.appendChild(fragment);
  }
}

/**
 * Intersection Observer wrapper for lazy loading
 */
export function observeVisibility(elements, callback, options = {}) {
  const observer = new IntersectionObserver((entries) => {
    entries.forEach(entry => {
      if (entry.isIntersecting) {
        callback(entry.target);
        if (options.once !== false) {
          observer.unobserve(entry.target);
        }
      }
    });
  }, {
    root: options.root || null,
    rootMargin: options.rootMargin || '50px',
    threshold: options.threshold || 0,
  });

  if (typeof elements === 'string') {
    elements = document.querySelectorAll(elements);
  }

  elements.forEach(el => observer.observe(el));

  return observer;
}

/**
 * Memoize function results
 */
export function memoize(fn, keyFn = JSON.stringify) {
  const cache = new Map();
  return (...args) => {
    const key = keyFn(args);
    if (cache.has(key)) {
      return cache.get(key);
    }
    const result = fn(...args);
    cache.set(key, result);
    return result;
  };
}

/**
 * Performance measurement utilities
 */
export const perf = {
  marks: new Map(),

  start(name) {
    this.marks.set(name, performance.now());
  },

  end(name, log = true) {
    const start = this.marks.get(name);
    if (!start) return 0;

    const duration = performance.now() - start;
    this.marks.delete(name);

    if (log) {
      console.log(`[Perf] ${name}: ${duration.toFixed(2)}ms`);
    }
    return duration;
  },

  measure(name, fn) {
    this.start(name);
    const result = fn();
    this.end(name);
    return result;
  },

  async measureAsync(name, fn) {
    this.start(name);
    const result = await fn();
    this.end(name);
    return result;
  },
};

/**
 * Animation frame loop with delta time
 */
export class AnimationLoop {
  constructor(callback) {
    this.callback = callback;
    this.running = false;
    this.lastTime = 0;
    this.frameId = null;
  }

  start() {
    if (this.running) return;
    this.running = true;
    this.lastTime = performance.now();
    this.tick();
  }

  stop() {
    this.running = false;
    if (this.frameId) {
      cancelAnimationFrame(this.frameId);
      this.frameId = null;
    }
  }

  tick() {
    if (!this.running) return;

    const now = performance.now();
    const delta = now - this.lastTime;
    this.lastTime = now;

    this.callback(delta, now);
    this.frameId = requestAnimationFrame(() => this.tick());
  }
}

/**
 * Create a smooth counter animation
 */
export function animateCounter(element, targetValue, options = {}) {
  const duration = options.duration || 500;
  const easing = options.easing || (t => t * (2 - t)); // Ease out quad
  const formatter = options.formatter || (v => Math.round(v).toLocaleString());

  const startValue = parseFloat(element.textContent.replace(/[^0-9.-]/g, '')) || 0;
  const diff = targetValue - startValue;
  const startTime = performance.now();

  function update() {
    const elapsed = performance.now() - startTime;
    const progress = Math.min(elapsed / duration, 1);
    const easedProgress = easing(progress);
    const currentValue = startValue + diff * easedProgress;

    element.textContent = formatter(currentValue);
    element.classList.add('counter-change');

    if (progress < 1) {
      requestAnimationFrame(update);
    } else {
      setTimeout(() => element.classList.remove('counter-change'), 300);
    }
  }

  requestAnimationFrame(update);
}
