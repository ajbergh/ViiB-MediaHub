/**
 * ViiB MediaHub - Sidebar Navigation Component
 *
 * Fixed left sidebar providing navigation between application sections.
 * - Desktop (≥md): persistent rail, can be collapsed to icons.
 * - Mobile (<md): off-canvas overlay drawer controlled by `mobileOpen` /
 *   `onMobileClose`. Closed by route change, Escape, or backdrop click.
 *
 * @module Sidebar
 */

import React, { useEffect, useRef, useState } from 'react';
import { NavLink, useLocation } from 'react-router-dom';
import {
  Home, Music, Disc, Mic2, ListMusic, Tags,
  Download, Search, Settings, Library, Sparkles, Loader2,
  ChevronLeft, ChevronRight, BarChart3, Heart, Disc3, X
} from 'lucide-react';
import { useStore } from '../store';
import { useIsMobile } from '../hooks/useMediaQuery';
import { SpotifyIcon } from './icons/SpotifyIcon';
import LargeLogo from './icons/Large-Logo1-clear-highres.png';
import SmallLogo from './icons/Icon_1_clear-high-res.png';

interface SidebarItemProps {
  to: string;
  icon: any;
  label: string;
  badge?: number;
  collapsed?: boolean;
  onNavigate?: () => void;
}

const SidebarItem = ({ to, icon: Icon, label, badge, collapsed, onNavigate }: SidebarItemProps) => {
  return (
    <NavLink
      to={to}
      title={collapsed ? label : undefined}
      aria-label={label}
      onClick={onNavigate}
      className={({ isActive }) =>
        `flex items-center gap-4 px-4 py-3 text-sm font-medium transition-all duration-200 ease-out relative ${
          isActive
            ? 'border-l-[3px] border-brand rounded-r-lg bg-brand/10 text-white ring-1 ring-brand/15'
            : 'border-l-[3px] border-transparent text-text-secondary hover:text-text-main hover:bg-surface-2/60'
        } ${collapsed ? 'justify-center px-2' : ''}`
      }
    >
      <Icon size={20} className="flex-shrink-0" aria-hidden="true" />
      {!collapsed && <span className="flex-1">{label}</span>}
      {!collapsed && badge !== undefined && badge > 0 && (
        <span className="bg-brand text-black/90 text-[10px] font-bold px-1.5 py-0.5 rounded-full min-w-[18px] text-center" aria-label={`${badge} items`}>
          {badge}
        </span>
      )}
      {collapsed && badge !== undefined && badge > 0 && (
        <span className="absolute -top-1 -right-1 bg-brand text-black/90 text-[10px] font-bold w-4 h-4 rounded-full flex items-center justify-center" aria-label={`${badge} items`}>
          {badge > 9 ? '9+' : badge}
        </span>
      )}
    </NavLink>
  );
};

export interface SidebarProps {
  mobileOpen?: boolean;
  onMobileClose?: () => void;
}

export const Sidebar: React.FC<SidebarProps> = ({ mobileOpen = false, onMobileClose }) => {
  const { isScanning, scanProgress, downloadCount, enrichmentStatus } = useStore();
  const [collapsed, setCollapsed] = useState(false);
  const isMobile = useIsMobile();
  const location = useLocation();
  const drawerRef = useRef<HTMLDivElement>(null);

  // On mobile, the drawer is never visually "collapsed"; it's either open full-width or hidden.
  // On DJ routes, force the sidebar to icon-rail mode to maximise the DJ canvas.
  const isDJRoute = location.pathname === '/dj';
  const effectiveCollapsed = isMobile ? false : (collapsed || isDJRoute);

  // Close drawer on route change (mobile only).
  useEffect(() => {
    if (isMobile && mobileOpen) onMobileClose?.();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [location.pathname]);

  // Escape closes drawer on mobile.
  useEffect(() => {
    if (!isMobile || !mobileOpen) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onMobileClose?.();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [isMobile, mobileOpen, onMobileClose]);

  // Move focus into the drawer when it opens on mobile.
  useEffect(() => {
    if (isMobile && mobileOpen && drawerRef.current) {
      drawerRef.current.focus();
    }
  }, [isMobile, mobileOpen]);

  const navContent = (
    <>
      <div className={`p-4 ${effectiveCollapsed ? 'px-2 flex-col items-center' : 'p-6 items-center justify-between'} flex`}>
        {!effectiveCollapsed && (
          <div className="flex items-center">
            <img
              src={LargeLogo}
              alt="ViiB MediaHub"
              className="h-32 w-auto object-contain"
            />
          </div>
        )}
        {effectiveCollapsed && (
          <div className="flex items-center justify-center">
            <img
              src={SmallLogo}
              alt="ViiB"
              className="h-32 w-32 object-contain"
            />
          </div>
        )}
        {isMobile ? (
          <button
            onClick={onMobileClose}
            className="p-1.5 rounded-lg text-text-subtle hover:text-text-main hover:bg-surface-2/60 transition-colors"
            aria-label="Close navigation"
          >
            <X size={20} />
          </button>
        ) : (
          <button
            onClick={() => setCollapsed(!collapsed)}
            className={`p-1.5 rounded-lg text-text-subtle hover:text-text-main hover:bg-surface-2/60 transition-colors ${effectiveCollapsed ? 'mt-2' : ''}`}
            aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
          >
            {collapsed ? <ChevronRight size={20} /> : <ChevronLeft size={20} />}
          </button>
        )}
      </div>

      <nav className="flex-1 overflow-y-auto py-2" role="navigation" aria-label="Main navigation">
        {/* Performance */}
        <div className="space-y-1">
          <SidebarItem to="/" icon={Home} label="Home" collapsed={effectiveCollapsed} onNavigate={onMobileClose} />
          <SidebarItem to="/dj" icon={Disc3} label="DJ Mode" collapsed={effectiveCollapsed} onNavigate={onMobileClose} />
        </div>

        <div className={`my-2 border-t border-surface-highlight/50 ${effectiveCollapsed ? 'mx-2' : 'mx-4'}`} role="separator"></div>

        {/* Library */}
        <div className="space-y-1">
          <SidebarItem to="/songs" icon={Music} label="Songs" collapsed={effectiveCollapsed} onNavigate={onMobileClose} />
          <SidebarItem to="/albums" icon={Disc} label="Albums" collapsed={effectiveCollapsed} onNavigate={onMobileClose} />
          <SidebarItem to="/artists" icon={Mic2} label="Artists" collapsed={effectiveCollapsed} onNavigate={onMobileClose} />
          <SidebarItem to="/genres" icon={Tags} label="Genres" collapsed={effectiveCollapsed} onNavigate={onMobileClose} />
          <SidebarItem to="/playlists" icon={ListMusic} label="Playlists" collapsed={effectiveCollapsed} onNavigate={onMobileClose} />
        </div>

        <div className={`my-2 border-t border-surface-highlight/50 ${effectiveCollapsed ? 'mx-2' : 'mx-4'}`} role="separator"></div>

        {/* Collections */}
        <div className="space-y-1">
          <SidebarItem to="/liked" icon={Heart} label="Liked Songs" collapsed={effectiveCollapsed} onNavigate={onMobileClose} />
          <SidebarItem to="/liked-albums" icon={Disc} label="Liked Albums" collapsed={effectiveCollapsed} onNavigate={onMobileClose} />
          <SidebarItem to="/smart-playlists" icon={Sparkles} label="AI DJ" collapsed={effectiveCollapsed} onNavigate={onMobileClose} />
        </div>

        <div className={`my-2 border-t border-surface-highlight/50 ${effectiveCollapsed ? 'mx-2' : 'mx-4'}`} role="separator"></div>

        {/* Services */}
        <div className="space-y-1">
          <SidebarItem to="/spotify" icon={SpotifyIcon} label="Spotify" collapsed={effectiveCollapsed} onNavigate={onMobileClose} />
          <SidebarItem to="/downloads" icon={Download} label="Downloads" badge={downloadCount} collapsed={effectiveCollapsed} onNavigate={onMobileClose} />
        </div>

        <div className={`my-2 border-t border-surface-highlight/50 ${effectiveCollapsed ? 'mx-2' : 'mx-4'}`} role="separator"></div>

        {/* Utilities */}
        <div className="space-y-1">
          <SidebarItem to="/search" icon={Search} label="Search" collapsed={effectiveCollapsed} onNavigate={onMobileClose} />
          <SidebarItem to="/stats" icon={BarChart3} label="Stats" collapsed={effectiveCollapsed} onNavigate={onMobileClose} />
          <SidebarItem to="/settings" icon={Settings} label="Settings" collapsed={effectiveCollapsed} onNavigate={onMobileClose} />
        </div>
      </nav>

      <div className={`p-4 border-t border-surface-3 bg-surface-0 ${effectiveCollapsed ? 'px-2' : ''}`}>
        {isScanning ? (
          <div className={`flex flex-col gap-2 text-text-secondary ${effectiveCollapsed ? 'items-center' : ''}`}>
            <div className={`flex items-center gap-3 text-brand ${effectiveCollapsed ? 'justify-center' : ''}`}>
              <Loader2 size={20} className="animate-spin motion-reduce:animate-none" />
              {!effectiveCollapsed && <span className="text-sm font-semibold">Scanning...</span>}
            </div>
            {!effectiveCollapsed && scanProgress && <span className="text-xs text-text-subtle line-clamp-1">{scanProgress}</span>}
          </div>
        ) : enrichmentStatus.isEnriching ? (
          <div className={`flex flex-col gap-2 text-text-secondary ${effectiveCollapsed ? 'items-center' : ''}`}>
            <div className={`flex items-center gap-3 text-brand ${effectiveCollapsed ? 'justify-center' : ''}`}>
              <Sparkles size={20} className="animate-pulse motion-reduce:animate-none" />
              {!effectiveCollapsed && (
                <span className="text-sm font-semibold">
                  {enrichmentStatus.message?.toLowerCase().includes('mood') ? 'Analyzing Moods...' : 'Enriching Genres...'}
                </span>
              )}
            </div>
            {!effectiveCollapsed && (
              <>
                <div className="w-full bg-surface-3 rounded-full h-1.5 overflow-hidden">
                  <div
                    className="bg-brand h-full rounded-full transition-all duration-300 ease-out motion-reduce:transition-none"
                    style={{
                      width: enrichmentStatus.totalSongs > 0
                        ? `${Math.round((enrichmentStatus.processedSongs / enrichmentStatus.totalSongs) * 100)}%`
                        : '0%'
                    }}
                  />
                </div>
                <span className="text-[10px] font-mono text-text-subtle">
                  {enrichmentStatus.processedSongs}/{enrichmentStatus.totalSongs} songs
                </span>
              </>
            )}
          </div>
        ) : enrichmentStatus.message && enrichmentStatus.message.includes('complete') ? (
          <div className={`flex flex-col gap-2 text-text-secondary ${effectiveCollapsed ? 'items-center' : ''}`}>
            <div className={`flex items-center gap-3 text-accent-green ${effectiveCollapsed ? 'justify-center' : ''}`}>
              <Sparkles size={20} />
              {!effectiveCollapsed && (
                <span className="text-sm font-semibold">
                  {enrichmentStatus.message?.toLowerCase().includes('mood') ? 'Moods Updated!' : 'Genres Updated!'}
                </span>
              )}
            </div>
            {!effectiveCollapsed && <span className="text-[10px] text-text-subtle line-clamp-1">{enrichmentStatus.message}</span>}
          </div>
        ) : (
          <div className={`flex items-center gap-3 text-text-secondary hover:text-text-main cursor-pointer transition-colors ${effectiveCollapsed ? 'justify-center' : ''}`}>
            <Library size={20} />
            {!effectiveCollapsed && (
              <div className="flex flex-col">
                <span className="text-sm font-semibold">Your Library</span>
                <span className="text-xs text-text-subtle">Local Media Collection</span>
              </div>
            )}
          </div>
        )}
      </div>
    </>
  );

  if (isMobile) {
    return (
      <>
        {/* Backdrop */}
        <div
          className={`fixed inset-0 z-40 bg-black/60 backdrop-blur-sm transition-opacity duration-200 ${
            mobileOpen ? 'opacity-100' : 'opacity-0 pointer-events-none'
          }`}
          aria-hidden={!mobileOpen}
          onClick={onMobileClose}
        />
        {/* Drawer */}
        <div
          ref={drawerRef}
          tabIndex={-1}
          role="dialog"
          aria-modal="true"
          aria-label="Main navigation"
          aria-hidden={!mobileOpen}
          className={`fixed top-0 left-0 bottom-0 z-50 w-72 max-w-[85vw] bg-surface-0 border-r border-surface-3 flex flex-col shadow-2xl transition-transform duration-200 ease-out ${
            mobileOpen ? 'translate-x-0' : '-translate-x-full'
          }`}
        >
          {navContent}
        </div>
      </>
    );
  }

  return (
    <div
      className={`h-full bg-surface-0 flex flex-col border-r border-surface-3 transition-all duration-300 ease-out ${
        effectiveCollapsed ? 'w-16' : 'w-64'
      }`}
    >
      {navContent}
    </div>
  );
};
