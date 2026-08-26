/**
 * ViiB MediaHub - Layout Component
 */
import React, { useEffect, useState } from 'react';
import { useLocation } from 'react-router';
import { useStore } from '../store';
import { syncSkinnyWindow } from '../services/skinnyWindowService';
import { Sidebar } from './Sidebar';
import { Player } from './Player';
import { Queue } from './Queue';
import { ContextMenu } from './ContextMenu';
import { ToastContainer } from './Toast';
import { MobileTopBar } from './MobileTopBar';
import { DesktopTitleBar } from './DesktopTitleBar';
import { useKeyboardNavigation } from '../hooks/useKeyboardNavigation';

export const Layout: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [mobileNavOpen, setMobileNavOpen] = useState(false);
  const location = useLocation();
  useKeyboardNavigation();
  const isDJRoute = location.pathname === '/dj';
  const isSkinnyMode = useStore((state) => state.isSkinnyMode);
  const isSkinnyAlwaysOnTop = useStore((state) => state.isSkinnyAlwaysOnTop);

  useEffect(() => {
    void syncSkinnyWindow(isSkinnyMode, isSkinnyMode && isSkinnyAlwaysOnTop);
  }, [isSkinnyAlwaysOnTop, isSkinnyMode]);

  useEffect(() => () => {
    void syncSkinnyWindow(false, false);
  }, []);

  return (
    <div className="flex h-screen flex-col bg-surface-0 text-text-main" onContextMenu={(event) => event.preventDefault()}>
      {!isSkinnyMode && (
        <>
          <DesktopTitleBar />
          {!isDJRoute && <MobileTopBar onOpenMenu={() => setMobileNavOpen(true)} />}
          <div className="relative flex flex-1 overflow-hidden">
            <Sidebar mobileOpen={mobileNavOpen} onMobileClose={() => setMobileNavOpen(false)} />
            <main className="relative flex-1 overflow-y-auto bg-surface-0">
              {children}
            </main>
            {!isDJRoute && <Queue />}
          </div>
        </>
      )}
      {(isSkinnyMode || !isDJRoute) && <Player />}
      <ContextMenu />
      <ToastContainer />
    </div>
  );
};
