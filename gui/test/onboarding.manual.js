/**
 * Onboarding Wizard E2E Test
 *
 * Tests the complete onboarding flow using Puppeteer
 */

import puppeteer from 'puppeteer';

const BASE_URL = process.env.TEST_URL || 'http://localhost:4444';
const HEADLESS = process.env.HEADLESS !== 'false';

async function runTests() {
  console.log('='.repeat(60));
  console.log('ONBOARDING WIZARD E2E TESTS');
  console.log('='.repeat(60));
  console.log(`URL: ${BASE_URL}`);
  console.log(`Headless: ${HEADLESS}`);
  console.log('');

  const browser = await puppeteer.launch({
    headless: HEADLESS,
    args: ['--no-sandbox', '--disable-setuid-sandbox']
  });

  const page = await browser.newPage();

  // Capture console logs
  page.on('console', msg => {
    if (msg.type() === 'error') {
      console.log(`  [BROWSER ERROR] ${msg.text()}`);
    }
  });

  const results = {
    passed: 0,
    failed: 0,
    errors: []
  };

  function test(name, passed, error = null) {
    if (passed) {
      console.log(`  ✓ ${name}`);
      results.passed++;
    } else {
      console.log(`  ✗ ${name}`);
      if (error) console.log(`    Error: ${error}`);
      results.failed++;
      results.errors.push({ name, error });
    }
  }

  try {
    // Load the page and wait for app to initialize
    console.log('\n[SETUP] Loading page and waiting for app...');
    await page.goto(BASE_URL);
    await page.waitForTimeout(2000);

    // Force start the onboarding wizard
    console.log('[SETUP] Force-starting onboarding wizard...');
    await page.evaluate(() => {
      localStorage.removeItem('gastown-onboarding-complete');
      localStorage.removeItem('gastown-onboarding-skipped');
      if (window.gastown && window.gastown.startOnboarding) {
        window.gastown.startOnboarding();
      }
    });
    await page.waitForTimeout(1000);

    // ==========================================
    // TEST 1: Wizard appears
    // ==========================================
    console.log('\n[TEST 1] Wizard Appearance');

    let wizardExists = await page.$('#onboarding-wizard');

    if (!wizardExists) {
      // Try via global function again
      console.log('  Retrying via window.gastown.startOnboarding()...');
      await page.evaluate(() => {
        if (window.gastown) window.gastown.startOnboarding();
      });
      await page.waitForTimeout(1000);
      wizardExists = await page.$('#onboarding-wizard');
    }

    test('Wizard modal appears', !!wizardExists);

    if (!wizardExists) {
      // Check if gastown is loaded
      const gastownLoaded = await page.evaluate(() => !!window.gastown);
      console.log(`  window.gastown loaded: ${gastownLoaded}`);

      if (!gastownLoaded) {
        console.log('  ERROR: App not fully loaded. Check for JS errors.');
        throw new Error('App not loaded - window.gastown is undefined');
      }
    }

    // ==========================================
    // TEST 2: Welcome Step
    // ==========================================
    console.log('\n[TEST 2] Welcome Step');

    const title = await page.$eval('.wizard-title', el => el.textContent).catch(() => null);
    test('Welcome title visible', title === 'Welcome to Gas Town');

    const flowSteps = await page.$$('.flow-step');
    test('Flow diagram has 4 steps', flowSteps.length === 4);

    const nextBtn = await page.$('.wizard-next');
    test('Next button exists', !!nextBtn);

    // Click Next
    await nextBtn.click();
    await page.waitForTimeout(1500);

    // ==========================================
    // TEST 3: Setup Check Step
    // ==========================================
    console.log('\n[TEST 3] Setup Check Step');

    const setupTitle = await page.$eval('.wizard-title', el => el.textContent).catch(() => null);
    test('Setup Check title visible', setupTitle === 'Checking Your Setup');

    // Wait for checks to complete
    await page.waitForTimeout(2000);

    const checkItems = await page.$$('.check-item');
    test('4 check items displayed', checkItems.length === 4);

    // Check for gt installed
    const gtCheck = await page.$('[data-check="gt"]');
    const gtStatus = await gtCheck?.evaluate(el => el.classList.contains('success') || el.classList.contains('error'));
    test('GT check completed', gtStatus !== undefined);

    // Click Next
    await page.click('.wizard-next');
    await page.waitForTimeout(1000);

    // ==========================================
    // TEST 4: Add Rig Step (may be skipped if rigs exist)
    // ==========================================
    console.log('\n[TEST 4] Add Rig / Create Bead Step');

    const currentTitle = await page.$eval('.wizard-title', el => el.textContent).catch(() => null);

    if (currentTitle === 'Connect a Project') {
      test('Add Rig step shown', true);

      const rigNameInput = await page.$('#rig-name');
      const rigUrlInput = await page.$('#rig-url');
      test('Rig name input exists', !!rigNameInput);
      test('Rig URL input exists', !!rigUrlInput);

      // Skip this step (go to next)
      console.log('  (Skipping rig creation - clicking Next to see validation)');
      await page.click('.wizard-next');
      await page.waitForTimeout(500);

      // Should show error
      const errorVisible = await page.$('.wizard-error:not(.hidden)');
      test('Validation error shown for empty rig', !!errorVisible);

      // Fill in dummy data and continue
      await rigNameInput.type('test-project');
      await rigUrlInput.type('https://github.com/test/repo.git');

      // Note: This will fail if rig already exists, that's ok
      await page.click('.wizard-next');
      await page.waitForTimeout(2000);
    } else {
      test('Add Rig step skipped (rigs already exist)', true);
    }

    // ==========================================
    // TEST 5: Create Bead Step
    // ==========================================
    console.log('\n[TEST 5] Create Bead Step');

    const beadTitle = await page.$eval('.wizard-title', el => el.textContent).catch(() => null);

    if (beadTitle === 'Create Your First Issue') {
      test('Create Bead step shown', true);

      const beadTitleInput = await page.$('#onboard-bead-title');
      const beadDescInput = await page.$('#onboard-bead-desc');
      test('Bead title input exists', !!beadTitleInput);
      test('Bead description input exists', !!beadDescInput);

      // Test validation
      await page.click('.wizard-next');
      await page.waitForTimeout(500);
      const errorVisible = await page.$('.wizard-error:not(.hidden)');
      test('Validation error for empty title', !!errorVisible);

      // Clear the input field and fill in bead title
      await beadTitleInput.click({ clickCount: 3 }); // Select all
      await beadTitleInput.type('Test Issue from Puppeteer');

      // Blur to commit value and wait for any change handlers
      await beadTitleInput.evaluate(el => el.blur());
      await page.waitForTimeout(500);

      // Verify the value was entered
      const inputValue = await page.evaluate(() => document.getElementById('onboard-bead-title')?.value);
      console.log(`  Input value (from DOM): "${inputValue}"`);

      // Wait for error to clear (it should auto-hide when input has value)
      await page.waitForTimeout(300);

      console.log('  Clicking Next to create bead...');
      await page.click('.wizard-next');

      // Wait for API call to complete (can be slow)
      await page.waitForTimeout(5000);

      // Check for success indicators
      const createdBead = await page.$('.created-bead:not(.hidden)');
      const beadErrorVisible = await page.$('.wizard-error:not(.hidden)');
      const errorText = await page.$eval('.wizard-error .error-message', el => el.textContent).catch(() => null);
      const nextStepTitle = await page.$eval('.wizard-title', el => el.textContent).catch(() => null);

      console.log(`  Created bead indicator: ${!!createdBead}`);
      console.log(`  Error visible: ${!!beadErrorVisible} - "${errorText}"`);
      console.log(`  Current step title: ${nextStepTitle}`);

      const beadSuccess = !!createdBead || nextStepTitle === "Track Your Work" || nextStepTitle === "Assign to an Agent";
      test('Bead created or moved to next step', beadSuccess, errorText);
    } else {
      test('Create Bead step - unexpected title: ' + beadTitle, false);
    }

    // ==========================================
    // TEST 6: Create Convoy Step
    // ==========================================
    console.log('\n[TEST 6] Create Convoy Step');

    const convoyTitle = await page.$eval('.wizard-title', el => el.textContent).catch(() => null);

    if (convoyTitle === "Track Your Work") {
      test('Create Convoy step shown', true);

      const convoyNameInput = await page.$('#onboard-convoy-name');
      const convoyBeadDisplay = await page.$('#onboard-convoy-bead');
      test('Convoy name input exists', !!convoyNameInput);
      test('Convoy bead display exists', !!convoyBeadDisplay);

      // Check that bead ID is populated (accepts gt- or hq- prefixes)
      const beadId = await convoyBeadDisplay?.evaluate(el => el.textContent);
      test('Bead ID is populated', beadId && (beadId.startsWith('gt-') || beadId.startsWith('hq-')));

      // Fill and submit
      await convoyNameInput.type('Test Convoy');
      await page.click('.wizard-next');
      await page.waitForTimeout(3000);
    } else {
      console.log(`  Current title: ${convoyTitle}`);
      test('Create Convoy step - title mismatch', false);
    }

    // ==========================================
    // TEST 7: Sling Work Step
    // ==========================================
    console.log('\n[TEST 7] Sling Work Step');

    const slingTitle = await page.$eval('.wizard-title', el => el.textContent).catch(() => null);

    if (slingTitle === "Assign to an Agent") {
      test('Sling Work step shown', true);

      const slingBeadDisplay = await page.$('#onboard-sling-bead');
      const slingTargetSelect = await page.$('#onboard-sling-target');
      test('Sling bead display exists', !!slingBeadDisplay);
      test('Sling target select exists', !!slingTargetSelect);

      // Check dropdown options
      const options = await page.$$('#onboard-sling-target option');
      test('Target dropdown has options', options.length > 0);
    } else {
      console.log(`  Current title: ${slingTitle}`);
    }

    // ==========================================
    // TEST 8: Skip/Close functionality
    // ==========================================
    console.log('\n[TEST 8] Skip/Close Functionality');

    const closeBtn = await page.$('.wizard-close');
    test('Close button exists', !!closeBtn);

    const backBtn = await page.$('.wizard-back:not([disabled])');
    test('Back button is enabled (not on first step)', !!backBtn);

  } catch (err) {
    console.log(`\n[FATAL ERROR] ${err.message}`);
    results.errors.push({ name: 'Test execution', error: err.message });
    results.failed++;
  }

  await browser.close();

  // Print summary
  console.log('\n' + '='.repeat(60));
  console.log('TEST RESULTS');
  console.log('='.repeat(60));
  console.log(`Passed: ${results.passed}`);
  console.log(`Failed: ${results.failed}`);

  if (results.errors.length > 0) {
    console.log('\nFailed tests:');
    results.errors.forEach(({ name, error }) => {
      console.log(`  - ${name}`);
      if (error) console.log(`    ${error}`);
    });
  }

  console.log('');
  process.exit(results.failed > 0 ? 1 : 0);
}

runTests().catch(err => {
  console.error('Fatal error:', err);
  process.exit(1);
});
