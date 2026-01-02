import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    // Unit tests are fast
    testTimeout: 10000,

    // Include only unit test files
    include: ['test/unit/**/*.test.js'],

    // No global setup needed for unit tests
    // (they don't need the mock server)

    // Global setup
    globals: true,

    // Reporter
    reporters: ['verbose'],
  },
});
