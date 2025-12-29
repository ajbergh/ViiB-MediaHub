/**
 * ViiB MediaHub - Layout Component
 * 
 * Root layout wrapper providing the application structure:
 * - Sidebar navigation (left)
 * - Main content area (center, scrollable)
 * - Floating queue panel (right, toggleable)
 * - Player controls (bottom, fixed)
 * - Global context menu layer
 * - Toast notifications
 * - Global keyboard navigation
 * 
 * Handles right-click prevention on the main container to enable
 * custom context menus throughout the application.
 * 
 * @module Layout
 */

import React from 'react';
import { Sidebar } from './Sidebar';
import { Player } from './Player';
import { Queue } from './Queue';
import { ContextMenu } from './ContextMenu';
import { ToastContainer } from './Toast';
import { useKeyboardNavigation } from '../hooks/useKeyboardNavigation';

export const Layout: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  // Enable global keyboard navigation (Space, arrows, Escape, etc.)
  useKeyboardNavigation();

  return (
    <div className="flex flex-col h-screen bg-surface-0 text-text-main" onContextMenu={(e) => e.preventDefault()}>
      <div className="flex flex-1 overflow-hidden relative">
        <Sidebar />
        <main className="flex-1 overflow-y-auto bg-surface-0 relative">
          {children}
        </main>
        {/* Floating Queue Panel */}
        <Queue />
      </div>
      <Player />
      {/* Global Context Menu Layer */}
      <ContextMenu />
      {/* Toast Notifications */}
      <ToastContainer />
    </div>
  );
};