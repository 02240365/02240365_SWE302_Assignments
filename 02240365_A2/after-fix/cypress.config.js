// Cypress configuration (WEB303 Part 2).
// The store is served by the gateway (ui-service) on port 8080.
// Start everything first with:  ./run-all.sh test
const { defineConfig } = require('cypress');

module.exports = defineConfig({
  e2e: {
    baseUrl: 'http://localhost:8080',
    specPattern: 'cypress/e2e/**/*.cy.js',
    supportFile: 'cypress/support/e2e.js',
    viewportWidth: 1280,
    viewportHeight: 900,
    video: false,
    screenshotOnRunFailure: true,
    // Placing an order streams three notifications (about 3 seconds), so
    // the waits for toasts are given their own longer timeout in the tests
    // instead of raising this default for everything.
    defaultCommandTimeout: 6000,
    requestTimeout: 15000,
    responseTimeout: 30000,
  },
});
