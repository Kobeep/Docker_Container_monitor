// Main process
const { app, BrowserWindow, ipcMain } = require('electron');
const path = require('path');
const Docker = require('dockerode');
const { getSSHHosts } = require('./sshUtils');

let docker = new Docker();

function createWindow() {
  const win = new BrowserWindow({
    width: 900,
    height: 600,
    webPreferences: {
      contextIsolation: true,
      preload: path.join(__dirname, 'preload.js')
    }
  });
  win.loadFile('index.html');
}

app.whenReady().then(() => {
  createWindow();
  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) {
      createWindow();
    }
  });
});

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') app.quit();
});

ipcMain.handle('start-poll', async () => {
  return fetchContainers();
});

// SSH host list
ipcMain.handle('get-ssh-hosts', async () => {
  return getSSHHosts();
});

ipcMain.handle('connect-remote', async (event, hostAlias) => {
  return `Connected to remote host: ${hostAlias}`;
});

async function fetchContainers() {
  try {
    const containers = await docker.listContainers({ all: true });
    return containers.map((c) => ({
      name: c.Names[0],
      status: c.Status,
      ports: c.Ports
    }));
  } catch (err) {
    return [];
  }
}
