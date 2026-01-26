/**
 * ViiB MediaHub - UI State Slice
 * 
 * Zustand slice managing UI state and user interactions.
 * 
 * State:
 * - isQueueOpen: Queue panel visibility
 * - isNowPlayingOpen: Full-screen now playing view
 * - showSmartMixes: Smart mix section visibility on home
 * - hasCompletedSetup: First-launch wizard completion flag
 * - isPartyMode: Fullscreen immersive mode with minimal UI
 * - logs: Application log entries for debugging
 * - toasts: User-visible toast notifications (success, error, info, warning)
 * - contextMenu: Right-click menu state and position
 * - confirmDialog: Modal confirmation dialog state
 * 
 * Toast System:
 * - showToast({ type, message, duration?, action? }): Display notification
 * - dismissToast(id): Manually dismiss a toast
 * - Auto-dismisses after duration (default 4s), max 5 toasts displayed
 * 
 * Party Mode:
 * - Immersive fullscreen with minimal UI (just album art, visualizers, track info)
 * - Toggles native fullscreen in Wails or browser Fullscreen API
 * 
 * @module uiSlice
 */

import { StateCreator } from 'zustand';
import { AppState, UISlice } from './types';
import { enterFullscreen, exitFullscreen } from '../services/fullscreenService';

export const createUISlice: StateCreator<AppState, [], [], UISlice> = (set, get) => ({
  isQueueOpen: false,
  isNowPlayingOpen: false,
  showSmartMixes: true,
  hasCompletedSetup: false,
  isPartyMode: false,
  logs: [],
  toasts: [],
  contextMenu: {
    isOpen: false,
    x: 0,
    y: 0,
    type: null,
    data: null,
  },
  confirmDialog: null,
  
  // Local search persistence
  localSearchQuery: '',
  localSearchTab: 'all',

  setQueueOpen: (isOpen) => set({ isQueueOpen: isOpen }),
  setNowPlayingOpen: (isOpen) => set({ isNowPlayingOpen: isOpen }),
  setShowSmartMixes: (show) => set({ showSmartMixes: show }),
  setHasCompletedSetup: (completed) => set({ hasCompletedSetup: completed }),
  
  setPartyMode: (enabled) => {
    if (enabled) {
      enterFullscreen();
    } else {
      exitFullscreen();
    }
    set({ isPartyMode: enabled });
  },
  
  togglePartyMode: () => {
    const current = get().isPartyMode;
    if (current) {
      exitFullscreen();
    } else {
      enterFullscreen();
    }
    set({ isPartyMode: !current });
  },
  
  // Local search persistence
  setLocalSearchQuery: (query) => set({ localSearchQuery: query }),
  setLocalSearchTab: (tab) => set({ localSearchTab: tab }),

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

  showToast: (toast) => set((state) => ({
    toasts: [...state.toasts, { ...toast, id: Math.random().toString(36).substr(2, 9) }].slice(-5) // Max 5 toasts
  })),
  
  dismissToast: (id) => set((state) => ({
    toasts: state.toasts.filter(t => t.id !== id)
  })),

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