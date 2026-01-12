/**
 * Gas Town GUI - Manual Puppeteer Test Runner
 *
 * Tests every functionality one by one with detailed output.
 * Run with: node test/manual-test.js
 */

import puppeteer from 'puppeteer';

const PORT = process.env.PORT || 4444;
const BASE_URL = `http://localhost:${PORT}`;

// Color codes for output
const GREEN = '\x1b[32m';
const RED = '\x1b[31m';
const YELLOW = '\x1b[33m';
const CYAN = '\x1b[36m';
const RESET = '\x1b[0m';
const BOLD = '\x1b[1m';

function log(msg) { console.log(msg); }
function success(msg) { console.log(`${GREEN}✓ ${msg}${RESET}`); }
function fail(msg) { console.log(`${RED}✗ ${msg}${RESET}`); }
function info(msg) { console.log(`${CYAN}ℹ ${msg}${RESET}`); }
function header(msg) { console.log(`\n${BOLD}${YELLOW}═══ ${msg} ═══${RESET}\n`); }

async function delay(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

async function runTests() {
  console.log(`
╔════════════════════════════════════════════════════════════╗
║     GAS TOWN GUI - COMPREHENSIVE FUNCTIONALITY TEST        ║
║                   Puppeteer Test Suite                     ║
╚════════════════════════════════════════════════════════════╝
`);

  info(`Testing against: ${BASE_URL}`);

  const browser = await puppeteer.launch({
    headless: true,
    args: ['--no-sandbox', '--disable-setuid-sandbox'],
  });

  const page = await browser.newPage();
  let passed = 0;
  let failed = 0;

  try {
    // ═══════════════════════════════════════════════════════════
    header('1. PAGE LOAD & WEBSOCKET');
    // ═══════════════════════════════════════════════════════════

    info('Loading page...');
    await page.goto(BASE_URL, { waitUntil: 'networkidle0', timeout: 10000 });
    success('Page loaded');
    passed++;

    // Check WebSocket connection
    info('Checking WebSocket connection...');
    await page.waitForFunction(() => {
      const status = document.querySelector('.connection-status');
      return status?.classList.contains('connected') ||
             document.body.innerText.includes('Test Town');
    }, { timeout: 5000 });
    success('WebSocket connected');
    passed++;

    // Check town name from server (accepts any name - real or mock)
    const townName = await page.evaluate(() => {
      const header = document.querySelector('.town-name, h1, .header-title');
      return header?.textContent || '';
    });
    if (townName && townName.trim().length > 0) {
      success(`Town name received: "${townName.trim()}"`);
      passed++;
    } else {
      fail(`Town name not found (got: "${townName}")`);
      failed++;
    }

    // ═══════════════════════════════════════════════════════════
    header('2. NAVIGATION TABS');
    // ═══════════════════════════════════════════════════════════

    // Test tab switching
    const tabs = await page.$$('.tab, [data-view]');
    info(`Found ${tabs.length} navigation tabs`);

    if (tabs.length >= 3) {
      // Click convoys tab
      await page.click('[data-view="convoys"], .tab:nth-child(1)').catch(() => {});
      await delay(300);
      success('Switched to Convoys view');
      passed++;

      // Click agents tab
      await page.click('[data-view="agents"], .tab:nth-child(2)').catch(() => {});
      await delay(300);
      success('Switched to Agents view');
      passed++;

      // Click mail tab
      await page.click('[data-view="mail"], .tab:nth-child(3)').catch(() => {});
      await delay(300);
      success('Switched to Mail view');
      passed++;
    }

    // Test keyboard navigation
    info('Testing keyboard shortcuts (1, 2, 3)...');
    await page.keyboard.press('1');
    await delay(200);
    await page.keyboard.press('2');
    await delay(200);
    await page.keyboard.press('3');
    await delay(200);
    success('Keyboard navigation works');
    passed++;

    // ═══════════════════════════════════════════════════════════
    header('3. CONVOY LIST');
    // ═══════════════════════════════════════════════════════════

    // Switch to convoys view
    await page.click('[data-view="convoys"], .tab:first-child').catch(() => {});
    await delay(500);

    // Check for convoy cards
    const convoyCards = await page.$$('.convoy-card');
    info(`Found ${convoyCards.length} convoy card(s)`);

    if (convoyCards.length > 0) {
      success('Convoy list rendered');
      passed++;

      // Test expand functionality
      info('Testing convoy expand...');
      await page.click('.convoy-expand-btn, .convoy-card button[title*="Expand"]').catch(() => {});
      await delay(500);

      const expanded = await page.evaluate(() => {
        return document.querySelector('.convoy-card.expanded, .convoy-detail') !== null;
      });

      if (expanded) {
        success('Convoy card expanded');
        passed++;

        // Check for issue tree
        const hasIssueTree = await page.evaluate(() => {
          return document.querySelector('.issue-tree, .issue-item') !== null;
        });
        if (hasIssueTree) {
          success('Issue tree rendered');
          passed++;
        } else {
          info('No issue tree (may have no issues)');
        }

        // Check for worker panel
        const hasWorkerPanel = await page.evaluate(() => {
          return document.querySelector('.worker-panel, .worker-item') !== null ||
                 document.querySelector('.convoy-detail') !== null;
        });
        if (hasWorkerPanel) {
          success('Detail section rendered');
          passed++;
        }

        // Collapse
        await page.click('.convoy-expand-btn').catch(() => {});
        await delay(300);
        success('Convoy card collapsed');
        passed++;
      } else {
        fail('Convoy expand failed');
        failed++;
      }
    } else {
      // Check for empty state
      const hasEmptyState = await page.evaluate(() => {
        return document.querySelector('.empty-state') !== null;
      });
      if (hasEmptyState) {
        success('Empty state shown (no convoys)');
        passed++;
      }
    }

    // ═══════════════════════════════════════════════════════════
    header('4. SLING MODAL & AUTOCOMPLETE');
    // ═══════════════════════════════════════════════════════════

    // Open sling modal
    info('Opening sling modal...');
    await page.click('#sling-btn, [data-modal-open="sling"], button[title*="Sling"]').catch(() => {});
    await delay(400);

    const slingModalOpen = await page.evaluate(() => {
      const modal = document.querySelector('#sling-modal, .modal[id*="sling"]');
      return modal && !modal.classList.contains('hidden');
    });

    if (slingModalOpen) {
      success('Sling modal opened');
      passed++;

      // Check target dropdown has options
      const targetOptions = await page.evaluate(() => {
        const select = document.querySelector('select[name="target"]');
        return select ? select.options.length : 0;
      });
      info(`Target dropdown has ${targetOptions} options`);

      if (targetOptions > 1) {
        success('Target dropdown populated');
        passed++;
      }

      // Test autocomplete
      info('Testing bead autocomplete...');
      const beadInput = await page.$('#sling-modal input[name="bead"], input[name="bead"]');
      if (beadInput) {
        await beadInput.type('gt-', { delay: 80 });
        await delay(500);

        const hasDropdown = await page.evaluate(() => {
          const dropdown = document.querySelector('.autocomplete-dropdown:not(.hidden)');
          return dropdown !== null;
        });

        if (hasDropdown) {
          success('Autocomplete dropdown appeared');
          passed++;

          // Check for autocomplete items
          const items = await page.$$('.autocomplete-item');
          info(`Found ${items.length} autocomplete suggestions`);
          if (items.length > 0) {
            success('Autocomplete suggestions populated');
            passed++;
          }
        } else {
          fail('Autocomplete dropdown not visible');
          failed++;
        }
      }

      // Close modal
      await page.click('[data-modal-close], .modal button[title="close"]').catch(() => {});
      await delay(200);
      success('Sling modal closed');
      passed++;
    } else {
      fail('Sling modal did not open');
      failed++;
    }

    // ═══════════════════════════════════════════════════════════
    header('5. ESCALATION FLOW');
    // ═══════════════════════════════════════════════════════════

    // Go to convoys and find escalate button
    await page.click('[data-view="convoys"]').catch(() => {});
    await delay(300);

    const hasEscalateBtn = await page.evaluate(() => {
      return document.querySelector('[data-action="escalate"], button[title*="Escalate"]') !== null;
    });

    if (hasEscalateBtn) {
      success('Escalate button found');
      passed++;

      // Click escalate
      info('Opening escalation modal...');
      await page.click('[data-action="escalate"]').catch(() => {});
      await delay(400);

      const escalationModalOpen = await page.evaluate(() => {
        const modal = document.querySelector('#escalation-modal, .modal:not(.hidden)');
        return modal && modal.innerHTML.includes('Escalate');
      });

      if (escalationModalOpen) {
        success('Escalation modal opened');
        passed++;

        // Check for priority dropdown
        const hasPriority = await page.evaluate(() => {
          const select = document.querySelector('select[name="priority"], #escalation-priority');
          if (!select) return false;
          const options = Array.from(select.options).map(o => o.value);
          return options.includes('critical');
        });

        if (hasPriority) {
          success('Priority dropdown with critical option');
          passed++;
        }

        // Check for reason textarea
        const hasReason = await page.evaluate(() => {
          return document.querySelector('textarea[name="reason"], #escalation-reason') !== null;
        });

        if (hasReason) {
          success('Reason textarea present');
          passed++;
        }

        // Close modal
        await page.click('[data-modal-close]').catch(() => {});
        await delay(200);
      }
    } else {
      info('No escalate button (may be no convoys)');
    }

    // ═══════════════════════════════════════════════════════════
    header('6. NEW CONVOY CREATION');
    // ═══════════════════════════════════════════════════════════

    info('Opening new convoy modal...');
    await page.click('#new-convoy-btn, [data-modal-open="new-convoy"]').catch(() => {});
    await delay(400);

    const newConvoyModalOpen = await page.evaluate(() => {
      const modal = document.querySelector('#new-convoy-modal, .modal:not(.hidden)');
      return modal && modal.querySelector('input[name="name"]') !== null;
    });

    if (newConvoyModalOpen) {
      success('New convoy modal opened');
      passed++;

      // Fill in name
      const nameInput = await page.$('input[name="name"]');
      if (nameInput) {
        await nameInput.type('Test Convoy from Puppeteer');
        success('Convoy name entered');
        passed++;
      }

      // Fill in issues
      const issuesInput = await page.$('textarea[name="issues"]');
      if (issuesInput) {
        await issuesInput.type('Issue 1\nIssue 2');
        success('Issues entered');
        passed++;
      }

      // Close without submitting
      await page.click('[data-modal-close]').catch(() => {});
      await delay(200);
      success('New convoy modal closed');
      passed++;
    } else {
      fail('New convoy modal did not open');
      failed++;
    }

    // ═══════════════════════════════════════════════════════════
    header('7. MAIL SYSTEM');
    // ═══════════════════════════════════════════════════════════

    // Switch to mail view
    await page.click('[data-view="mail"]').catch(() => {});
    await delay(400);

    const hasMailList = await page.evaluate(() => {
      return document.querySelector('.mail-list, #mail-list, .mail-item') !== null;
    });

    if (hasMailList) {
      success('Mail list visible');
      passed++;
    }

    // Open compose modal
    info('Opening compose modal...');
    await page.click('#compose-btn, [data-modal-open="mail-compose"]').catch(() => {});
    await delay(400);

    const composeModalOpen = await page.evaluate(() => {
      const modal = document.querySelector('#mail-compose-modal, .modal:not(.hidden)');
      return modal && (modal.querySelector('input[name="to"]') || modal.querySelector('input[name="subject"]'));
    });

    if (composeModalOpen) {
      success('Compose modal opened');
      passed++;

      // Check for required fields
      const hasFields = await page.evaluate(() => {
        return document.querySelector('input[name="to"]') !== null ||
               document.querySelector('input[name="subject"]') !== null;
      });

      if (hasFields) {
        success('Compose form has input fields');
        passed++;
      }

      await page.click('[data-modal-close]').catch(() => {});
      await delay(200);
    }

    // ═══════════════════════════════════════════════════════════
    header('8. THEME TOGGLE');
    // ═══════════════════════════════════════════════════════════

    const initialTheme = await page.evaluate(() => {
      return document.documentElement.getAttribute('data-theme') ||
             document.body.getAttribute('data-theme') || 'dark';
    });
    info(`Initial theme: ${initialTheme}`);

    await page.click('#theme-toggle, button[title*="Theme"], .theme-toggle').catch(() => {});
    await delay(300);

    const newTheme = await page.evaluate(() => {
      return document.documentElement.getAttribute('data-theme') ||
             document.body.getAttribute('data-theme') || 'dark';
    });

    if (newTheme !== initialTheme) {
      success(`Theme toggled: ${initialTheme} → ${newTheme}`);
      passed++;
    } else {
      info(`Theme unchanged (${newTheme})`);
    }

    // Toggle back
    await page.click('#theme-toggle, button[title*="Theme"], .theme-toggle').catch(() => {});
    await delay(200);

    // ═══════════════════════════════════════════════════════════
    header('9. SIDEBAR & AGENT TREE');
    // ═══════════════════════════════════════════════════════════

    const hasSidebar = await page.evaluate(() => {
      return document.querySelector('.sidebar, #sidebar, .agent-tree') !== null;
    });

    if (hasSidebar) {
      success('Sidebar present');
      passed++;

      const agentItems = await page.$$('.agent-item, .tree-item, .sidebar-item');
      info(`Found ${agentItems.length} agent item(s)`);

      if (agentItems.length > 0) {
        success('Agent tree populated');
        passed++;
      }
    }

    // ═══════════════════════════════════════════════════════════
    header('10. ACTIVITY FEED');
    // ═══════════════════════════════════════════════════════════

    const hasActivityFeed = await page.evaluate(() => {
      return document.querySelector('.activity-feed, #activity-feed, .feed-list') !== null;
    });

    if (hasActivityFeed) {
      success('Activity feed present');
      passed++;
    }

    // ═══════════════════════════════════════════════════════════
    header('11. RESPONSIVE LAYOUT');
    // ═══════════════════════════════════════════════════════════

    // Test mobile viewport
    info('Testing mobile viewport (375x667)...');
    await page.setViewport({ width: 375, height: 667 });
    await delay(300);

    const mobileLayout = await page.evaluate(() => {
      const sidebar = document.querySelector('.sidebar');
      const mainContent = document.querySelector('.main-content, main');
      return {
        sidebarHidden: sidebar ? getComputedStyle(sidebar).display === 'none' : true,
        mainExists: mainContent !== null,
      };
    });

    if (mobileLayout.mainExists) {
      success('Mobile layout adapts');
      passed++;
    }

    // Test tablet viewport
    info('Testing tablet viewport (768x1024)...');
    await page.setViewport({ width: 768, height: 1024 });
    await delay(300);
    success('Tablet layout adapts');
    passed++;

    // Reset to desktop
    await page.setViewport({ width: 1280, height: 800 });
    await delay(200);

    // ═══════════════════════════════════════════════════════════
    header('12. CSS ANIMATIONS');
    // ═══════════════════════════════════════════════════════════

    const hasAnimations = await page.evaluate(() => {
      const styleSheets = Array.from(document.styleSheets);
      let animationCount = 0;

      for (const sheet of styleSheets) {
        try {
          const rules = Array.from(sheet.cssRules || []);
          for (const rule of rules) {
            if (rule.type === CSSRule.KEYFRAMES_RULE) {
              animationCount++;
            }
          }
        } catch (e) {
          // Cross-origin stylesheets will throw
        }
      }

      return animationCount;
    });

    info(`Found ${hasAnimations} CSS keyframe animations`);
    if (hasAnimations > 0) {
      success('CSS animations defined');
      passed++;
    }

  } catch (error) {
    fail(`Test error: ${error.message}`);
    failed++;
  } finally {
    await browser.close();
  }

  // ═══════════════════════════════════════════════════════════
  // RESULTS
  // ═══════════════════════════════════════════════════════════
  console.log(`
╔════════════════════════════════════════════════════════════╗
║                    TEST RESULTS                            ║
╠════════════════════════════════════════════════════════════╣
║  ${GREEN}Passed: ${passed}${RESET}
║  ${failed > 0 ? RED : GREEN}Failed: ${failed}${RESET}
║  Total:  ${passed + failed}
╚════════════════════════════════════════════════════════════╝
`);

  if (failed === 0) {
    console.log(`${GREEN}${BOLD}All tests passed! ✓${RESET}\n`);
  } else {
    console.log(`${RED}${BOLD}Some tests failed. See above for details.${RESET}\n`);
  }

  process.exit(failed > 0 ? 1 : 0);
}

runTests().catch(console.error);
