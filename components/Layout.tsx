/**
 * ViiB MediaHub - Layout Component
 *
 * Root layout wrapper providing the application structure:
 * - Mobile top bar (visible <md) with menu button + current route title
 * - Sidebar navigation (left, persistent ≥md, off-canvas drawer <md)
 * - Main content area (center, scrollable)
 * - Floating queue panel (right, toggleable)
 * - Player controls (bottom, fixed)
 * - Global context menu layer
 * - Toast notifications
 * - Global keyboard navigation
 *
 * @module Layout
 */

import React, { useState } from 'react';
import { useLocation } from 'react-router';
import { Sidebar } from './Sidebar';
import { Player } from './Player';
import { Queue } from './Queue';
import { ContextMenu } from './ContextMenu';
import { ToastContainer } from './Toast';
import { MobileTopBar } from './MobileTopBar';
import { useKeyboardNavigation } from '../hooks/useKeyboardNavigation';

export const Layout: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [mobileNavOpen, setMobileNavOpen] = useState(false);
  const location = useLocation();
  useKeyboardNavigation();

  // DJ routes manage their own audio engine — suppress the global player bar and mobile top bar
  const isDJRoute = location.pathname === '/dj';

  return (
    <div className="flex flex-col h-screen bg-surface-0 text-text-main" onContextMenu={(e) => e.preventDefault()}>
      {!isDJRoute && <MobileTopBar onOpenMenu={() => setMobileNavOpen(true)} />}
      <div className="flex flex-1 overflow-hidden relative">
        {/* Sidebar (rail on desktop, drawer on mobile) */}
        <Sidebar mobileOpen={mobileNavOpen} onMobileClose={() => setMobileNavOpen(false)} />
        <main className="flex-1 overflow-y-auto bg-surface-0 relative">
          {children}
        </main>
        {/* Floating Queue Panel */}
        {!isDJRoute && <Queue />}
      </div>
      {!isDJRoute && <Player />}
      {/* Global Context Menu Layer */}
      <ContextMenu />
      {/* Toast Notifications */}
      <ToastContainer />
    </div>
  );
};
