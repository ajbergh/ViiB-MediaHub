import { StateCreator } from 'zustand';
import { AppState, UISlice } from './types';

export const createUISlice: StateCreator<AppState, [], [], UISlice> = (set) => ({
  isQueueOpen: false,
  isNowPlayingOpen: false,
  showSmartMixes: true,
  logs: [],
  contextMenu: {
    isOpen: false,
    x: 0,
    y: 0,
    type: null,
    data: null,
  },
  confirmDialog: null,

  setQueueOpen: (isOpen) => set({ isQueueOpen: isOpen }),
  setNowPlayingOpen: (isOpen) => set({ isNowPlayingOpen: isOpen }),
  setShowSmartMixes: (show) => set({ showSmartMixes: show }),

  openContextMenu: (e, type, data) => {
    e.preventDefault();
    e.stopPropagation();
    set({ contextMenu: { isOpen: true, x: e.clientX, y: e.clientY, type, data } });
  },

  closeContextMenu: () => set((state) => ({ 
    contextMenu: { ...state.contextMenu, isOpen: false } 
  })),

  showConfirmDialog: (config) => set({ confirmDialog: config }),
  closeConfirmDialog: () => set({ confirmDialog: null }),

  addLog: (level, message, details) => set((state) => ({
      logs: [{
          id: Math.random().toString(36).substr(2, 9),
          timestamp: Date.now(),
          level,
          message,
          details: details ? JSON.parse(JSON.stringify(details)) : undefined
      }, ...state.logs].slice(0, 100)
  })),
  clearLogs: () => set({ logs: [] }),
});