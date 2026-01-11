#!/usr/bin/env node

const { execSync } = require('child_process');
const path = require('path');
const fs = require('fs');
const os = require('os');

console.log('Running gt npm package tests...\n');

let passed = 0;
let failed = 0;

function test(name, fn) {
  try {
    fn();
    console.log(`[PASS] ${name}`);
    passed++;
  } catch (err) {
    console.log(`[FAIL] ${name}: ${err.message}`);
    failed++;
  }
}

// Test 1: Check binary exists
test('Binary exists in bin directory', () => {
  const binaryName = os.platform() === 'win32' ? 'gt.exe' : 'gt';
  const binaryPath = path.join(__dirname, '..', 'bin', binaryName);
  if (!fs.existsSync(binaryPath)) {
    throw new Error(`Binary not found at ${binaryPath}`);
  }
});

// Test 2: Binary is executable (version check)
test('Binary executes and returns version', () => {
  const binaryName = os.platform() === 'win32' ? 'gt.exe' : 'gt';
  const binaryPath = path.join(__dirname, '..', 'bin', binaryName);
  const output = execSync(`"${binaryPath}" version`, { encoding: 'utf8' });
  if (!output.includes('gt version')) {
    throw new Error(`Unexpected version output: ${output}`);
  }
});

// Test 3: Wrapper script exists
test('Wrapper script (gt.js) exists', () => {
  const wrapperPath = path.join(__dirname, '..', 'bin', 'gt.js');
  if (!fs.existsSync(wrapperPath)) {
    throw new Error(`Wrapper not found at ${wrapperPath}`);
  }
});

// Summary
console.log(`\n${passed} passed, ${failed} failed`);
process.exit(failed > 0 ? 1 : 0);
