import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    // E2E tests are slower
    testTimeout: 60000,
    hookTimeout: 30000,

    // Run tests sequentially (browser tests can conflict)
    pool: 'forks',
    poolOptions: {
      forks: {
        singleFork: true,
      },
    },

    // Include test files
    include: ['test/**/*.test.js'],

    // Global setup - start mock server before tests
    globalSetup: './test/globalSetup.js',

    // Global setup
    globals: true,

    // Reporter
    reporters: ['verbose'],

    // Retry flaky tests
    retry: 1,
  },
});
