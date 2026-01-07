/**
 * Gas Town GUI - Global Test Setup
 *
 * Starts mock server before all tests and stops it after.
 */

import { startMockServer, stopMockServer } from './mock-server.js';

let server;

export async function setup() {
  console.log('[Test Setup] Starting mock server...');
  server = await startMockServer();
  console.log('[Test Setup] Mock server started');
}

export async function teardown() {
  console.log('[Test Teardown] Stopping mock server...');
  await stopMockServer();
  console.log('[Test Teardown] Mock server stopped');
}
