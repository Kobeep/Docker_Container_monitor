// Electron main process
const { app, BrowserWindow, ipcMain } = require('electron');
const { spawn } = require('child_process');
const path = require('path');

function createWindow() {
  // Create main window
  const mainWindow = new BrowserWindow({
    width: 800,
    height: 600,
    webPreferences: {
      contextIsolation: true,
      preload: path.join(__dirname, 'preload.js')
    }
  });
  mainWindow.loadFile('index.html');
}

app.whenReady().then(() => {
  createWindow();
  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) {
      createWindow();
    }
  });
});

// Quit on close if not macOS
app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    app.quit();
  }
});

// Handle CLI calls
ipcMain.handle('run-monitor', async (event, args) => {
  return new Promise((resolve, reject) => {
    // Use 'monitor' from PATH or full path
    const cmd = spawn('monitor', args);
    let output = '';
    let errorOut = '';

    cmd.stdout.on('data', (data) => {
      output += data.toString();
    });

    cmd.stderr.on('data', (data) => {
      errorOut += data.toString();
    });

    cmd.on('close', (code) => {
      if (code === 0) {
        resolve(output.trim());
      } else {
        reject(errorOut.trim());
      }
    });
  });
});
