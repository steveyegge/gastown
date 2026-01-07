/**
 * Gas Town GUI - Test Setup
 *
 * Common setup and utilities for Puppeteer E2E tests.
 */

import puppeteer from 'puppeteer';

// Test configuration
// Use 5678 by default to avoid port conflicts with Claude orchestrator (3000)
// and to keep production server port (5555) free
const PORT = process.env.PORT || 5678;
export const CONFIG = {
  baseUrl: process.env.TEST_URL || `http://localhost:${PORT}`,
  headless: process.env.HEADLESS !== 'false',
  slowMo: parseInt(process.env.SLOW_MO) || 0,
  timeout: 30000,
  viewport: {
    width: 1280,
    height: 800,
  },
};

// Global browser instance
let browser = null;

/**
 * Launch browser for tests
 */
export async function launchBrowser() {
  if (!browser) {
    browser = await puppeteer.launch({
      headless: CONFIG.headless,
      slowMo: CONFIG.slowMo,
      args: [
        '--no-sandbox',
        '--disable-setuid-sandbox',
        '--disable-dev-shm-usage',
      ],
    });
  }
  return browser;
}

/**
 * Close browser after tests
 */
export async function closeBrowser() {
  if (browser) {
    await browser.close();
    browser = null;
  }
}

/**
 * Create a new page with default settings
 */
export async function createPage() {
  const b = await launchBrowser();
  const page = await b.newPage();

  await page.evaluateOnNewDocument(() => {
    localStorage.setItem('gastown-onboarding-complete', 'true');
    localStorage.setItem('gastown-onboarding-skipped', 'true');
    localStorage.setItem('gastown-tutorial-complete', 'true');
  });

  await page.setViewport(CONFIG.viewport);
  page.setDefaultTimeout(CONFIG.timeout);

  return page;
}

/**
 * Navigate to the GUI and wait for it to load
 */
export async function navigateToApp(page) {
  await page.goto(CONFIG.baseUrl, { waitUntil: 'domcontentloaded' });
  // Wait for app to initialize
  await page.waitForSelector('#app-header', { timeout: 15000 });
  await page.waitForFunction(() => !!window.gastown, { timeout: 15000 });
}

/**
 * Wait for WebSocket connection to be established
 */
export async function waitForConnection(page) {
  await page.waitForFunction(() => {
    const status = document.querySelector('#connection-status');
    return status?.classList.contains('connected') ||
      status?.textContent?.toLowerCase().includes('connected');
  }, { timeout: 15000 });
}

/**
 * Click a navigation tab and wait for view to switch
 */
export async function switchView(page, viewName) {
  await page.waitForSelector(`[data-view="${viewName}"]`, { timeout: 5000 });
  await page.click(`[data-view="${viewName}"]`);
  await page.waitForSelector(`#view-${viewName}.active`, { timeout: 5000 });
}

/**
 * Open a modal by clicking a button
 */
export async function openModal(page, modalId) {
  await page.click(`[data-modal-open="${modalId}"]`);
  await page.waitForSelector(`#${modalId}-modal:not(.hidden)`, { timeout: 5000 });
}

/**
 * Close all modals
 */
export async function closeModals(page) {
  await page.keyboard.press('Escape');
  await page.waitForSelector('#modal-overlay.hidden', { timeout: 5000 });
}

/**
 * Get text content of an element
 */
export async function getText(page, selector) {
  return page.$eval(selector, el => el.textContent.trim());
}

/**
 * Check if element exists
 */
export async function elementExists(page, selector) {
  return page.$(selector).then(el => el !== null);
}

/**
 * Wait for toast notification
 */
export async function waitForToast(page, type = null) {
  const selector = type ? `.toast.toast-${type}.show` : '.toast.show';
  await page.waitForSelector(selector, { timeout: 10000 });
  return getText(page, `${selector} .toast-message`);
}

/**
 * Fill a form field
 */
export async function fillField(page, selector, value) {
  await page.click(selector, { clickCount: 3 }); // Select all
  await page.type(selector, value);
}

/**
 * Take a screenshot for debugging
 */
export async function screenshot(page, name) {
  const timestamp = Date.now();
  await page.screenshot({
    path: `test/screenshots/${name}-${timestamp}.png`,
    fullPage: true,
  });
}

/**
 * Assert helper
 */
export function assert(condition, message) {
  if (!condition) {
    throw new Error(`Assertion failed: ${message}`);
  }
}

/**
 * Sleep for debugging
 */
export function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}
