/**
 * ViiB MediaHub - Layout Component
 */
import React, { useState } from 'react';
import { NavLink, useLocation } from 'react-router-dom';
import { ShieldCheck } from 'lucide-react';
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
  const isDJRoute = location.pathname === '/dj';

  return (
    <div className="flex h-screen flex-col bg-surface-0 text-text-main" onContextMenu={(event) => event.preventDefault()}>
      {!isDJRoute && <MobileTopBar onOpenMenu={() => setMobileNavOpen(true)} />}
      <div className="relative flex flex-1 overflow-hidden">
        <Sidebar mobileOpen={mobileNavOpen} onMobileClose={() => setMobileNavOpen(false)} />
        <main className="relative flex-1 overflow-y-auto bg-surface-0">
          {!isDJRoute && location.pathname !== '/library-operations' && (
            <NavLink
              to="/library-operations"
              className="fixed right-5 top-5 z-20 hidden items-center gap-2 rounded-full border border-surface-highlight bg-surface-1/95 px-3 py-2 text-xs font-semibold text-text-secondary shadow-lg backdrop-blur hover:text-text-main lg:inline-flex"
              title="Library diagnostics, backup, recovery, and monitoring"
            >
              <ShieldCheck size={16} className="text-brand" />
              Library Health
            </NavLink>
          )}
          {children}
        </main>
        {!isDJRoute && <Queue />}
      </div>
      {!isDJRoute && <Player />}
      <ContextMenu />
      <ToastContainer />
    </div>
  );
};
