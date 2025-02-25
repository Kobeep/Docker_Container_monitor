// Spectron E2E test
const { Application } = require('spectron');
const assert = require('chai').assert;
const path = require('path');

describe('Electron App Tests', function() {
  this.timeout(10000); // Increase if needed
  let app;

  before(async () => {
    // Path to local electron bin
    const electronPath = path.join(__dirname, '..', 'node_modules', '.bin', 'electron');
    // App root
    const appPath = path.join(__dirname, '..');

    app = new Application({
      path: electronPath,
      args: [appPath]
    });

    await app.start();
  });

  after(async () => {
    if (app && app.isRunning()) {
      await app.stop();
    }
  });

  it('shows the main window', async () => {
    const count = await app.client.getWindowCount();
    assert.equal(count, 1, 'Main window not found');
  });

  it('has correct title', async () => {
    const title = await app.client.getTitle();
    assert.equal(title, 'Electron Docker Monitor', 'Title mismatch');
  });

  it('renders UI elements', async () => {
    // Example: check if a button or text is present
    const button = await app.client.$('#btnStart');
    const exists = await button.isExisting();
    assert.isTrue(exists, 'btnStart not found');
  });
});
