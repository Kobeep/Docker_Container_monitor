// Expose safe API to renderer
const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('electronAPI', {
  runMonitor: async (args) => {
    return await ipcRenderer.invoke('run-monitor', args);
  },
});
