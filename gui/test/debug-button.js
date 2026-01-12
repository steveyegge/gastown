/**
 * Debug script to check why button has zero size
 */

import { startMockServer, stopMockServer } from './mock-server.js';
import puppeteer from 'puppeteer';

async function debug() {
  await startMockServer();

  const browser = await puppeteer.launch({
    headless: true,
    args: ['--no-sandbox']
  });
  const page = await browser.newPage();

  // Add console logging
  page.on('console', msg => console.log('[Browser]', msg.text()));
  page.on('pageerror', error => console.log('[Page Error]', error.message));

  const port = process.env.PORT || 5678;
  console.log(`Navigating to http://localhost:${port}`);
  await page.goto(`http://localhost:${port}`, { waitUntil: 'networkidle2', timeout: 30000 });

  // Check what stylesheets are loaded
  const stylesheets = await page.evaluate(() => {
    return Array.from(document.styleSheets).map(sheet => {
      let rulesCount = 'unknown';
      try {
        rulesCount = sheet.cssRules?.length || 0;
      } catch (e) {
        rulesCount = 'cross-origin';
      }
      return { href: sheet.href, rulesCount };
    });
  });
  console.log('Loaded stylesheets:');
  stylesheets.forEach(s => console.log(' ', s.href, '- rules:', s.rulesCount));

  // Check CSS variables
  const cssVars = await page.evaluate(() => {
    const root = document.documentElement;
    const style = getComputedStyle(root);
    return {
      spaceSm: style.getPropertyValue('--space-sm'),
      spaceMd: style.getPropertyValue('--space-md'),
    };
  });
  console.log('CSS Variables:', cssVars);

  // Check button and its container hierarchy
  const containerInfo = await page.evaluate(() => {
    const btn = document.querySelector('#sling-btn');
    if (!btn) return { exists: false };

    const btnStyle = window.getComputedStyle(btn);
    const btnRect = btn.getBoundingClientRect();

    const parent = btn.parentElement;
    const parentStyle = window.getComputedStyle(parent);
    const parentRect = parent.getBoundingClientRect();

    const grandparent = parent?.parentElement;
    const grandparentStyle = grandparent ? window.getComputedStyle(grandparent) : null;
    const grandparentRect = grandparent?.getBoundingClientRect();

    // Check ancestors for display:none
    let ancestor = btn;
    let hiddenAncestor = null;
    while (ancestor) {
      const style = window.getComputedStyle(ancestor);
      if (style.display === 'none' || style.visibility === 'hidden') {
        hiddenAncestor = ancestor.tagName + '.' + ancestor.className;
        break;
      }
      ancestor = ancestor.parentElement;
    }

    return {
      button: {
        innerHTML: btn.innerHTML.substring(0, 100),
        display: btnStyle.display,
        visibility: btnStyle.visibility,
        padding: btnStyle.padding,
        width: btnRect.width,
        height: btnRect.height,
      },
      parent: {
        tag: parent?.tagName,
        className: parent?.className,
        display: parentStyle.display,
        width: parentRect.width,
        height: parentRect.height,
      },
      grandparent: grandparent ? {
        tag: grandparent?.tagName,
        className: grandparent?.className,
        display: grandparentStyle?.display,
        width: grandparentRect?.width,
        height: grandparentRect?.height,
      } : null,
      hiddenAncestor,
    };
  });
  console.log('Container info:', JSON.stringify(containerInfo, null, 2));

  // Get the current view
  const currentView = await page.evaluate(() => {
    const activeView = document.querySelector('.view.active');
    return activeView ? activeView.id : 'none';
  });
  console.log('Active view:', currentView);

  await browser.close();
  await stopMockServer();
  console.log('Done');
}

debug().catch(console.error);
