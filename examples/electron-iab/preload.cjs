const { contextBridge, ipcRenderer } = require('electron');
contextBridge.exposeInMainWorld('iabExample', {
  state: () => ipcRenderer.invoke('iab-example:action', { action: 'state' }),
  action: (action, value) => ipcRenderer.invoke('iab-example:action', { action, value }),
  onChanged: callback => {
    const listener = () => callback();
    ipcRenderer.on('iab-example:changed', listener);
    return () => ipcRenderer.removeListener('iab-example:changed', listener);
  },
});
