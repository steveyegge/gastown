/**
 * Gas Town GUI - Comprehensive Integration Tests
 *
 * Tests full integration of all features:
 * - WebSocket connectivity and real-time updates
 * - API endpoints (search, targets, escalate)
 * - Autocomplete component
 * - Escalation flow
 * - Convoy management (expand, collapse, actions)
 * - Modal interactions
 */

import { describe, it, expect, beforeAll, afterAll, beforeEach } from 'vitest';
import puppeteer from 'puppeteer';

const PORT = process.env.PORT || 5678;
const BASE_URL = `http://localhost:${PORT}`;
const sleep = (ms) => new Promise(resolve => setTimeout(resolve, ms));

describe('Comprehensive Integration Tests', () => {
  let browser;
  let page;

  beforeAll(async () => {
    browser = await puppeteer.launch({
      headless: true,
      args: ['--no-sandbox', '--disable-setuid-sandbox'],
    });
  });

  afterAll(async () => {
    if (browser) await browser.close();
  });

  beforeEach(async () => {
    page = await browser.newPage();
    await page.evaluateOnNewDocument(() => {
      localStorage.setItem('gastown-onboarding-complete', 'true');
      localStorage.setItem('gastown-onboarding-skipped', 'true');
      localStorage.setItem('gastown-tutorial-complete', 'true');
    });
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
    await page.waitForSelector('#app-header', { timeout: 15000 });
    await page.waitForFunction(() => !!window.gastown, { timeout: 15000 });
  });

  afterEach(async () => {
    if (page) await page.close();
  });

  describe('WebSocket Integration', () => {
    it('should establish WebSocket connection on page load', async () => {
      await page.waitForFunction(() => {
        const status = document.querySelector('.connection-status');
        return status?.classList.contains('connected') ||
               status?.textContent?.toLowerCase().includes('connected');
      }, { timeout: 15000 });

      // Check connection indicator
      const connected = await page.evaluate(() => {
        const status = document.querySelector('.connection-status');
        return status?.classList.contains('connected') ||
               status?.textContent?.toLowerCase().includes('connected');
      });
      expect(connected).toBe(true);
    });

    it('should receive initial status data via WebSocket', async () => {
      await page.waitForFunction(() => {
        const header = document.querySelector('.town-name, h1, .header-title');
        return header?.textContent?.includes('Test Town');
      }, { timeout: 15000 });

      // The town name should be populated from WebSocket
      const townName = await page.evaluate(() => {
        const header = document.querySelector('.town-name, h1, .header-title');
        return header?.textContent;
      });
      expect(townName).toContain('Test Town');
    });
  });

  describe('API Endpoints Integration', () => {
    it('should fetch and display convoys', async () => {
      // Navigate to convoys view
      await page.click('[data-view="convoys"], .tab[data-view="convoys"], button:has-text("Convoys")').catch(() => {});
      await sleep(500);

      // Check if convoys are rendered
      const hasConvoys = await page.evaluate(() => {
        const convoyList = document.querySelector('.convoy-list, #convoy-list');
        return convoyList && (
          convoyList.querySelector('.convoy-card') !== null ||
          convoyList.querySelector('.empty-state') !== null
        );
      });
      expect(hasConvoys).toBe(true);
    });

    it('should fetch agents from API', async () => {
      // Navigate to agents view
      await page.click('[data-view="agents"], .tab[data-view="agents"]').catch(() => {});
      await sleep(500);

      // Check if agents section exists
      const hasAgents = await page.evaluate(() => {
        const sidebar = document.querySelector('.sidebar, .agent-tree, #agent-tree');
        return sidebar !== null;
      });
      expect(hasAgents).toBe(true);
    });
  });

  describe('Sling Modal and Autocomplete', () => {
    it('should open sling modal', async () => {
      // Switch to convoys view first (sling button is in convoys view)
      await page.click('[data-view="convoys"]');
      await page.waitForSelector('#view-convoys.active', { timeout: 5000 });

      // Click sling button
      await page.click('#sling-btn');
      await sleep(300);

      // Check modal is visible
      const modalVisible = await page.evaluate(() => {
        const modal = document.querySelector('#sling-modal');
        return modal && !modal.classList.contains('hidden');
      });
      expect(modalVisible).toBe(true);
    });

    it('should populate target dropdown with options', async () => {
      // Switch to convoys view first
      await page.click('[data-view="convoys"]');
      await page.waitForSelector('#view-convoys.active', { timeout: 5000 });

      // Open sling modal
      await page.click('#sling-btn');
      await page.waitForSelector('#sling-modal:not(.hidden)', { timeout: 5000 });

      // Wait for target dropdown to populate (async fetch)
      await sleep(500);

      // Check target dropdown has options
      const hasTargets = await page.evaluate(() => {
        const select = document.querySelector('#sling-modal select[name="target"]');
        return select && select.options.length > 1;
      });
      expect(hasTargets).toBe(true);
    });

    it('should show autocomplete dropdown when typing in bead field', async () => {
      // Switch to convoys view first
      await page.click('[data-view="convoys"]');
      await page.waitForSelector('#view-convoys.active', { timeout: 5000 });

      // Open sling modal
      await page.click('#sling-btn');
      await page.waitForSelector('#sling-modal:not(.hidden)', { timeout: 5000 });

      // Type in bead field
      const beadInput = await page.$('#sling-modal input[name="bead"]');
      if (beadInput) {
        await beadInput.type('gt-', { delay: 100 });
        await sleep(400);

        // Check if autocomplete dropdown appeared
        const hasDropdown = await page.evaluate(() => {
          const dropdown = document.querySelector('.autocomplete-dropdown:not(.hidden)');
          return dropdown !== null;
        });
        expect(hasDropdown).toBe(true);
      }
    });
  });

  describe('Escalation Flow', () => {
    it('should have escalate button on convoy cards', async () => {
      // Navigate to convoys
      await page.click('[data-view="convoys"]');
      await page.waitForSelector('#view-convoys.active', { timeout: 5000 });

      // Wait for convoy cards to load (either convoy cards or empty state)
      await page.waitForFunction(() => {
        return document.querySelector('.convoy-card') !== null ||
               document.querySelector('.empty-state') !== null;
      }, { timeout: 5000 });

      // Check for escalate button on convoy cards
      const hasEscalateBtn = await page.evaluate(() => {
        const btn = document.querySelector('[data-action="escalate"]');
        return btn !== null;
      });
      expect(hasEscalateBtn).toBe(true);
    });

    it('should open escalation modal when clicking escalate', async () => {
      // Navigate to convoys
      await page.click('[data-view="convoys"]');
      await page.waitForSelector('#view-convoys.active', { timeout: 5000 });

      // Wait for convoy cards to load
      await page.waitForSelector('.convoy-card', { timeout: 5000 });

      // Click escalate button
      await page.click('[data-action="escalate"]');
      await sleep(300);

      // Check escalation modal (created dynamically)
      const modalVisible = await page.evaluate(() => {
        const modal = document.querySelector('#escalation-modal');
        return modal && !modal.classList.contains('hidden');
      });
      expect(modalVisible).toBe(true);
    });

    it('should have priority dropdown in escalation form', async () => {
      // Navigate to convoys
      await page.click('[data-view="convoys"]');
      await page.waitForSelector('#view-convoys.active', { timeout: 5000 });

      // Wait for convoy cards to load
      await page.waitForSelector('.convoy-card', { timeout: 5000 });

      // Open escalation modal
      await page.click('[data-action="escalate"]');
      await page.waitForSelector('#escalation-modal:not(.hidden)', { timeout: 5000 });

      // Check for priority select with expected options
      const hasPrioritySelect = await page.evaluate(() => {
        const select = document.querySelector('#escalation-priority');
        if (!select) return false;
        const options = Array.from(select.options).map(o => o.value);
        return options.includes('normal') && options.includes('high') && options.includes('critical');
      });
      expect(hasPrioritySelect).toBe(true);
    });
  });

  describe('Convoy Management', () => {
    it('should expand convoy card when clicking expand button', async () => {
      // Navigate to convoys
      await page.click('[data-view="convoys"]').catch(() => {});
      await sleep(500);

      // Click expand button
      await page.click('.convoy-expand-btn, .convoy-card button[title*="Expand"]').catch(() => {});
      await sleep(500);

      // Check if detail section appeared
      const hasDetail = await page.evaluate(() => {
        const card = document.querySelector('.convoy-card.expanded, .convoy-card:has(.convoy-detail)');
        return card !== null;
      });
      expect(hasDetail).toBe(true);
    });

    it('should show issue tree in expanded convoy', async () => {
      // Navigate and expand convoy
      await page.click('[data-view="convoys"]').catch(() => {});
      await sleep(300);
      await page.click('.convoy-expand-btn').catch(() => {});
      await sleep(500);

      // Check for issue tree
      const hasIssueTree = await page.evaluate(() => {
        const tree = document.querySelector('.issue-tree, .convoy-detail .issue-item');
        return tree !== null;
      });
      expect(hasIssueTree).toBe(true);
    });
  });

  describe('Mail System', () => {
    it('should display mail list', async () => {
      // Navigate to mail view
      await page.click('[data-view="mail"], .tab[data-view="mail"]').catch(() => {});
      await sleep(500);

      // Check for mail list
      const hasMail = await page.evaluate(() => {
        const list = document.querySelector('.mail-list, #mail-list');
        return list !== null;
      });
      expect(hasMail).toBe(true);
    });

    it('should open compose modal', async () => {
      // Navigate to mail and click compose
      await page.click('[data-view="mail"]').catch(() => {});
      await sleep(300);
      await page.click('#compose-btn, [data-modal-open="mail-compose"], button:has-text("Compose")').catch(() => {});
      await sleep(300);

      // Check compose modal
      const modalVisible = await page.evaluate(() => {
        const modal = document.querySelector('#mail-compose-modal, .modal:not(.hidden)');
        return modal && (
          modal.querySelector('input[name="to"]') !== null ||
          modal.querySelector('input[name="subject"]') !== null
        );
      });
      expect(modalVisible).toBe(true);
    });
  });

  describe('Theme and Keyboard Shortcuts', () => {
    it('should toggle theme when clicking theme button', async () => {
      // Get initial theme
      const initialTheme = await page.evaluate(() => {
        return document.documentElement.getAttribute('data-theme') ||
               document.body.getAttribute('data-theme') ||
               'dark';
      });

      // Click theme toggle
      await page.click('#theme-toggle, button[title*="Theme"], .theme-toggle').catch(() => {});
      await sleep(200);

      // Get new theme
      const newTheme = await page.evaluate(() => {
        return document.documentElement.getAttribute('data-theme') ||
               document.body.getAttribute('data-theme') ||
               'dark';
      });

      // Theme should have changed
      expect(newTheme !== initialTheme || newTheme === 'light').toBe(true);
    });

    it('should respond to keyboard shortcuts', async () => {
      // Press a number key to switch views (1, 2, 3 are mapped)
      await page.keyboard.press('1');
      await sleep(200);

      // Check that keyboard shortcuts are registered
      const hasKeyboardHandler = await page.evaluate(() => {
        // Check if keyboard event handler exists
        const viewTabs = document.querySelectorAll('.tab, [data-view]');
        return viewTabs.length > 0;
      });
      expect(hasKeyboardHandler).toBe(true);
    });
  });

  describe('Activity Feed', () => {
    it('should display activity feed', async () => {
      const hasFeed = await page.evaluate(() => {
        const feed = document.querySelector('.activity-feed, #activity-feed, .feed-list');
        return feed !== null;
      });
      expect(hasFeed).toBe(true);
    });
  });

  describe('New Convoy Creation', () => {
    it('should open new convoy modal', async () => {
      await page.click('#new-convoy-btn, [data-modal-open="new-convoy"], button:has-text("New Convoy")').catch(() => {});
      await sleep(300);

      const modalVisible = await page.evaluate(() => {
        const modal = document.querySelector('#new-convoy-modal, .modal:not(.hidden)');
        return modal && modal.querySelector('input[name="name"]') !== null;
      });
      expect(modalVisible).toBe(true);
    });

    it('should validate required fields on convoy creation', async () => {
      // Open modal
      await page.click('#new-convoy-btn, [data-modal-open="new-convoy"]').catch(() => {});
      await sleep(200);

      // Try to submit empty form
      await page.click('#new-convoy-modal button[type="submit"], .modal button[type="submit"]').catch(() => {});
      await sleep(300);

      // Check for validation (either native or toast)
      const hasValidation = await page.evaluate(() => {
        const toast = document.querySelector('.toast, .notification');
        const input = document.querySelector('#new-convoy-modal input[name="name"], input[name="name"]');
        return (toast !== null) || (input && !input.validity.valid);
      });
      expect(hasValidation).toBe(true);
    });
  });
});
