// Expose API to renderer
const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('electronAPI', {
  startPoll: async () => {
    return await ipcRenderer.invoke('start-poll');
  },
  getSSHHosts: async () => {
    return await ipcRenderer.invoke('get-ssh-hosts');
  },
  connectRemote: async (hostAlias) => {
    return await ipcRenderer.invoke('connect-remote', hostAlias);
  }
});
