---
name: playwright
description: >
  Browser automation for E2E testing using Playwright. Enables navigation,
  form filling, clicking, waiting for elements, and taking screenshots.
allowed-tools: "Bash(npx playwright:*),Bash(node:*),Read,Write"
version: "1.0.0"
author: "Gas Town"
license: "MIT"
---

[SKILL-ACTIVE: playwright v1.0.0]

# Playwright - Browser Automation for E2E Testing

Automate browser interactions for end-to-end testing using Playwright.

## Prerequisites

```bash
# Check if playwright is available
npx playwright --version

# If not installed, install it (one-time setup)
npm init playwright@latest
```

## Common Patterns

### Navigate to URL

```javascript
const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage();
  await page.goto('https://example.com');
  // ... do work
  await browser.close();
})();
```

### Fill Forms

```javascript
// Fill a text input
await page.fill('input[name="username"]', 'myuser');
await page.fill('input[name="password"]', 'mypassword');

// Select from dropdown
await page.selectOption('select#country', 'US');

// Check a checkbox
await page.check('input[type="checkbox"]');
```

### Click Elements

```javascript
// Click by selector
await page.click('button[type="submit"]');

// Click by text
await page.click('text=Sign In');

// Click and wait for navigation
await Promise.all([
  page.waitForNavigation(),
  page.click('a.next-page')
]);
```

### Wait for Selectors

```javascript
// Wait for element to appear
await page.waitForSelector('.loading-complete');

// Wait with timeout
await page.waitForSelector('.result', { timeout: 5000 });

// Wait for element to be hidden
await page.waitForSelector('.spinner', { state: 'hidden' });
```

### Take Screenshots

```javascript
// Full page screenshot
await page.screenshot({ path: 'screenshot.png', fullPage: true });

// Element screenshot
const element = await page.$('.hero');
await element.screenshot({ path: 'hero.png' });
```

## One-Liner Script Pattern

For quick automation tasks, use a node one-liner:

```bash
node -e "
const { chromium } = require('playwright');
(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage();
  await page.goto('https://example.com');
  console.log(await page.title());
  await browser.close();
})();
"
```

## Test File Pattern

For repeatable E2E tests:

```javascript
// tests/e2e/login.spec.js
const { test, expect } = require('@playwright/test');

test('user can login', async ({ page }) => {
  await page.goto('/login');
  await page.fill('[data-testid="email"]', 'user@example.com');
  await page.fill('[data-testid="password"]', 'password');
  await page.click('[data-testid="submit"]');
  await expect(page).toHaveURL('/dashboard');
});
```

Run with:
```bash
npx playwright test tests/e2e/login.spec.js
```

## Debugging

```bash
# Run with headed browser (visible)
npx playwright test --headed

# Run with Playwright Inspector
npx playwright test --debug

# Generate code by recording
npx playwright codegen https://example.com
```

## Best Practices

1. **Use data-testid attributes** for reliable selectors
2. **Wait explicitly** rather than using arbitrary timeouts
3. **Clean up** - always close browser in finally block
4. **Screenshots on failure** for debugging
5. **Avoid flaky selectors** like nth-child or dynamic classes
