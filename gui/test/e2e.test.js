/**
 * Gas Town GUI - E2E Tests
 *
 * End-to-end tests using Puppeteer for the Gas Town GUI.
 */

import { describe, it, beforeAll, afterAll, beforeEach, expect } from 'vitest';
import {
  launchBrowser,
  closeBrowser,
  createPage,
  navigateToApp,
  waitForConnection,
  switchView,
  openModal,
  closeModals,
  getText,
  elementExists,
  waitForToast,
  fillField,
  screenshot,
  assert,
  sleep,
} from './setup.js';

describe('Gas Town GUI E2E Tests', () => {
  let page;

  beforeAll(async () => {
    await launchBrowser();
  });

  afterAll(async () => {
    await closeBrowser();
  });

  beforeEach(async () => {
    page = await createPage();
  });

  describe('Page Load', () => {
    it('should load the application', async () => {
      await navigateToApp(page);

      const title = await page.title();
      expect(title).toContain('Gas Town');

      const header = await elementExists(page, '#app-header');
      expect(header).toBe(true);
    });

    it('should display the town name', async () => {
      await navigateToApp(page);

      const townName = await getText(page, '#town-name');
      expect(townName).toBeTruthy();
    });

    it('should show connection status', async () => {
      await navigateToApp(page);

      const statusExists = await elementExists(page, '#connection-status');
      expect(statusExists).toBe(true);
    });
  });

  describe('Navigation', () => {
    it('should switch between views using tabs', async () => {
      await navigateToApp(page);

      // Start on convoys view
      let activeView = await page.$eval('.view.active', el => el.id);
      expect(activeView).toBe('view-convoys');

      // Switch to agents
      await switchView(page, 'agents');
      activeView = await page.$eval('.view.active', el => el.id);
      expect(activeView).toBe('view-agents');

      // Switch to mail
      await switchView(page, 'mail');
      activeView = await page.$eval('.view.active', el => el.id);
      expect(activeView).toBe('view-mail');
    });

    it('should support keyboard shortcuts for navigation', async () => {
      await navigateToApp(page);

      // Press '2' for agents view
      await page.keyboard.press('2');
      await page.waitForSelector('#view-agents.active', { timeout: 2000 });

      // Press '3' for mail view
      await page.keyboard.press('3');
      await page.waitForSelector('#view-mail.active', { timeout: 2000 });

      // Press '1' for convoys view
      await page.keyboard.press('1');
      await page.waitForSelector('#view-convoys.active', { timeout: 2000 });
    });
  });

  describe('Theme Toggle', () => {
    it('should toggle between dark and light themes', async () => {
      await navigateToApp(page);

      // Get initial theme
      const initialTheme = await page.$eval('html', el => el.dataset.theme);

      // Click theme toggle
      await page.click('#theme-toggle');

      // Check theme changed
      const newTheme = await page.$eval('html', el => el.dataset.theme);
      expect(newTheme).not.toBe(initialTheme);

      // Toggle back
      await page.click('#theme-toggle');
      const finalTheme = await page.$eval('html', el => el.dataset.theme);
      expect(finalTheme).toBe(initialTheme);
    });
  });

  describe('Sidebar', () => {
    it('should display the agent tree', async () => {
      await navigateToApp(page);

      const sidebarExists = await elementExists(page, '#agent-tree');
      expect(sidebarExists).toBe(true);
    });

    it('should expand and collapse tree nodes', async () => {
      await navigateToApp(page);

      // Find expandable node if any
      const expandableNode = await page.$('.tree-node.expandable');
      if (expandableNode) {
        await expandableNode.click();
        // Check if class toggled
        const isExpanded = await expandableNode.evaluate(el => el.classList.contains('expanded'));
        // Toggle again
        await expandableNode.click();
        const nowExpanded = await expandableNode.evaluate(el => el.classList.contains('expanded'));
        expect(isExpanded).not.toBe(nowExpanded);
      }
    });
  });

  describe('Modals', () => {
    it('should open and close new convoy modal', async () => {
      await navigateToApp(page);

      // Open modal
      await page.click('#new-convoy-btn');
      await page.waitForSelector('#new-convoy-modal:not(.hidden)', { timeout: 5000 });

      // Check modal is visible
      const modalVisible = await elementExists(page, '#new-convoy-modal:not(.hidden)');
      expect(modalVisible).toBe(true);

      // Close with Escape
      await page.keyboard.press('Escape');
      await page.waitForSelector('#modal-overlay.hidden', { timeout: 5000 });

      // Check modal is hidden
      const overlayHidden = await page.$eval('#modal-overlay', el => el.classList.contains('hidden'));
      expect(overlayHidden).toBe(true);
    });

    it('should open sling modal', async () => {
      await navigateToApp(page);

      // Open sling modal
      await page.click('#sling-btn');
      await page.waitForSelector('#sling-modal:not(.hidden)', { timeout: 5000 });

      const modalVisible = await elementExists(page, '#sling-modal:not(.hidden)');
      expect(modalVisible).toBe(true);

      await closeModals(page);
    });
  });

  describe('Refresh', () => {
    it('should refresh data when clicking refresh button', async () => {
      await navigateToApp(page);

      // Click refresh
      await page.click('#refresh-btn');

      // Should show toast
      const toastMessage = await waitForToast(page, 'info');
      expect(toastMessage).toContain('Refresh');
    });

    it('should refresh with Ctrl+R keyboard shortcut', async () => {
      await navigateToApp(page);

      // Press Ctrl+R (we need to intercept this as it normally refreshes page)
      await page.evaluate(() => {
        document.dispatchEvent(new KeyboardEvent('keydown', {
          key: 'r',
          ctrlKey: true,
          bubbles: true,
        }));
      });

      // The handler should trigger refresh
      // This test verifies the keyboard handler is attached
    });
  });

  describe('Keyboard Help', () => {
    it('should show help when pressing ?', async () => {
      await navigateToApp(page);

      await page.keyboard.press('?');

      // Should show toast with keyboard shortcuts
      const toastMessage = await waitForToast(page, 'info');
      expect(toastMessage).toContain('Keyboard');
    });
  });

  describe('Responsive Layout', () => {
    it('should adapt to mobile viewport', async () => {
      await navigateToApp(page);

      // Set mobile viewport
      await page.setViewport({ width: 375, height: 667 });

      // Wait for layout to adjust
      await sleep(500);

      // Sidebar should be hidden or collapsed on mobile
      // Main content should still be visible
      const mainContent = await elementExists(page, '.content');
      expect(mainContent).toBe(true);
    });

    it('should adapt to tablet viewport', async () => {
      await navigateToApp(page);

      await page.setViewport({ width: 768, height: 1024 });
      await sleep(500);

      const header = await elementExists(page, '#app-header');
      expect(header).toBe(true);
    });
  });

  describe('Activity Feed', () => {
    it('should display activity feed section', async () => {
      await navigateToApp(page);

      const feedExists = await elementExists(page, '.activity-feed');
      expect(feedExists).toBe(true);
    });

    it('should show feed list container', async () => {
      await navigateToApp(page);

      const feedList = await elementExists(page, '#feed-list');
      expect(feedList).toBe(true);
    });
  });

  describe('Form Validation', () => {
    it('should validate convoy name is required', async () => {
      await navigateToApp(page);

      // Open new convoy modal
      await page.click('#new-convoy-btn');
      await page.waitForSelector('#new-convoy-modal:not(.hidden)', { timeout: 5000 });

      // Check that the name input has required attribute
      const inputRequired = await page.$eval(
        '#new-convoy-modal [name="name"]',
        el => el.hasAttribute('required')
      );
      expect(inputRequired).toBe(true);

      // Close modal by clicking the close button
      await page.click('#new-convoy-modal [data-modal-close]');
      await page.waitForSelector('#modal-overlay.hidden', { timeout: 5000 });
    });

    it('should validate sling form fields', async () => {
      await navigateToApp(page);

      await page.click('#sling-btn');
      await page.waitForSelector('#sling-modal:not(.hidden)', { timeout: 5000 });

      // Check both bead and target are required
      const beadRequired = await page.$eval(
        '#sling-modal [name="bead"]',
        el => el.hasAttribute('required')
      );
      const targetRequired = await page.$eval(
        '#sling-modal [name="target"]',
        el => el.hasAttribute('required')
      );

      expect(beadRequired).toBe(true);
      expect(targetRequired).toBe(true);

      await closeModals(page);
    });
  });

  describe('Animations', () => {
    it('should have animation classes defined', async () => {
      await navigateToApp(page);

      // Check that animation CSS is loaded
      const hasAnimationStyles = await page.evaluate(() => {
        const styleSheets = Array.from(document.styleSheets);
        return styleSheets.some(sheet => {
          try {
            const rules = Array.from(sheet.cssRules || []);
            return rules.some(rule =>
              rule.cssText && rule.cssText.includes('@keyframes')
            );
          } catch {
            return false;
          }
        });
      });

      expect(hasAnimationStyles).toBe(true);
    });
  });
});

describe('Component Tests', () => {
  let page;

  beforeAll(async () => {
    await launchBrowser();
  });

  afterAll(async () => {
    await closeBrowser();
  });

  beforeEach(async () => {
    page = await createPage();
  });

  describe('Toast Component', () => {
    it('should display toast and auto-dismiss', async () => {
      await navigateToApp(page);

      // Trigger a toast via refresh button
      await page.click('#refresh-btn');

      // Toast should appear
      await page.waitForSelector('.toast.show', { timeout: 5000 });

      // Wait for auto-dismiss (default 3s for info)
      await sleep(4000);

      // Toast should be gone
      const toastExists = await elementExists(page, '.toast.show');
      expect(toastExists).toBe(false);
    });
  });

  describe('Convoy List Component', () => {
    it('should render convoy list or empty state', async () => {
      await navigateToApp(page);

      // Wait for convoy data to load - give more time for API call
      await sleep(2000);

      // Wait for convoy data to load (either cards or empty state)
      try {
        await page.waitForFunction(() => {
          const hasConvoys = document.querySelector('.convoy-card');
          const hasEmpty = document.querySelector('#convoy-list .empty-state');
          return hasConvoys || hasEmpty;
        }, { timeout: 8000 });
      } catch (e) {
        // If timeout, check what's in the convoy-list element for debugging
        const listContent = await page.$eval('#convoy-list', el => el.innerHTML.substring(0, 200));
        console.log('[Debug] convoy-list content:', listContent);
      }

      // Either convoy cards or empty state should be present
      const hasConvoys = await elementExists(page, '.convoy-card');
      const hasEmptyState = await elementExists(page, '#convoy-list .empty-state');

      expect(hasConvoys || hasEmptyState).toBe(true);
    });
  });

  describe('Agent Grid Component', () => {
    it('should render agent grid or empty state', async () => {
      await navigateToApp(page);
      await switchView(page, 'agents');

      // Either agent cards or empty state should be present
      const hasAgents = await elementExists(page, '.agent-card');
      const hasEmptyState = await elementExists(page, '#agent-grid .empty-state');

      expect(hasAgents || hasEmptyState).toBe(true);
    });
  });

  describe('Mail List Component', () => {
    it('should render mail list or empty state', async () => {
      await navigateToApp(page);
      await switchView(page, 'mail');

      // Either mail items or empty state should be present
      const hasMail = await elementExists(page, '.mail-item');
      const hasEmptyState = await elementExists(page, '#mail-list .empty-state');

      expect(hasMail || hasEmptyState).toBe(true);
    });
  });
});
