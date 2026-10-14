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
import { NavLink, useLocation } from 'react-router';
import {
  Home, Music, Disc, Mic2, ListMusic, Tags,
  Download, Search, Settings, Library, Sparkles, Loader2,
  ChevronLeft, ChevronRight, BarChart3, Heart, Disc3, Copy, X, Gauge
} from 'lucide-react';
import { useStore } from '../store';
import { jobsV2, type OperationJob } from '../services/jobsV2';
import { useIsMobile } from '../hooks/useMediaQuery';
import { SpotifyIcon } from './icons/SpotifyIcon';
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
        `group flex items-center gap-3 px-3 py-2 rounded-md text-[13px] font-semibold transition-colors duration-150 relative ${
          isActive
            ? 'bg-surface-highlight text-text-main'
            : 'text-text-secondary hover:text-text-main hover:bg-surface-hover'
        } ${collapsed ? 'justify-center px-2 py-2.5' : ''}`
      }
    >
      {({ isActive }) => (
        <>
          <Icon
            size={18}
            className={`flex-shrink-0 transition-colors ${
              isActive ? 'text-brand' : 'text-text-secondary group-hover:text-text-main'
            }`}
            aria-hidden="true"
          />
          {!collapsed && <span className="flex-1 truncate">{label}</span>}
          {!collapsed && badge !== undefined && badge > 0 && (
            <span
              className="bg-brand text-surface-0 text-[10px] font-bold px-1.5 py-0.5 rounded-full min-w-[18px] text-center leading-none"
              aria-label={`${badge} items`}
            >
              {badge}
            </span>
          )}
          {collapsed && badge !== undefined && badge > 0 && (
            <span
              className="absolute top-1 right-1 bg-brand text-surface-0 text-[9px] font-bold w-3.5 h-3.5 rounded-full flex items-center justify-center"
              aria-label={`${badge} items`}
            >
              {badge > 9 ? '9+' : badge}
            </span>
          )}
        </>
      )}
    </NavLink>
  );
};

export interface SidebarProps {
  mobileOpen?: boolean;
  onMobileClose?: () => void;
}

export const Sidebar: React.FC<SidebarProps> = ({ mobileOpen = false, onMobileClose }) => {
  const isScanning = useStore(state => state.isScanning);
  const scanProgress = useStore(state => state.scanProgress);
  const downloadCount = useStore(state => state.downloadCount);
  const enrichmentStatus = useStore(state => state.enrichmentStatus);
  const [collapsed, setCollapsed] = useState(false);
  const [jobs, setJobs] = useState<OperationJob[]>([]);
  const isMobile = useIsMobile();
  const location = useLocation();
  const drawerRef = useRef<HTMLDivElement>(null);

  const isDJRoute = location.pathname === '/dj';
  const effectiveCollapsed = isMobile ? false : (collapsed || isDJRoute);
  const activeAnalysisJob = jobs.find((job) => job.type === 'analyze_tracks' && job.status === 'running');

  useEffect(() => {
    const controller = new AbortController();
    void jobsV2.list('', controller.signal).then(({ jobs: listed }) => setJobs(listed)).catch(() => {
      // Browser-only use does not have the durable job service.
    });
    const unsubscribe = jobsV2.subscribe(setJobs);
    return () => {
      controller.abort();
      unsubscribe();
    };
  }, []);

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

  const hasStatusActivity = Boolean(
    activeAnalysisJob ||
    enrichmentStatus.isEnriching ||
    isScanning ||
    (enrichmentStatus.message && enrichmentStatus.message.includes('complete'))
  );

  const navContent = (
    <nav className="flex flex-col h-full gap-2" role="navigation" aria-label="Main navigation">
      {/* Top Primary Navigation Island */}
      <div className={`bg-surface-1 ${isDJRoute || isMobile ? '' : 'md:rounded-lg'} px-2 py-2.5 flex-shrink-0`}>
        <div className={`flex items-center mb-1.5 ${effectiveCollapsed ? 'flex-col gap-1.5 justify-center px-1' : 'justify-between px-2.5 py-1'}`}>
          <div className="flex items-center gap-2.5 min-w-0">
            <img
              src={SmallLogo}
              alt="ViiB MediaHub"
              className="h-6 w-6 object-contain flex-shrink-0"
            />
            {!effectiveCollapsed && (
              <span className="text-[15px] font-bold tracking-tight text-text-main truncate">
                ViiB
              </span>
            )}
          </div>
          {isMobile ? (
            <button
              onClick={onMobileClose}
              className="p-1 rounded-md text-text-secondary hover:text-text-main hover:bg-surface-hover transition-colors"
              aria-label="Close navigation"
            >
              <X size={18} />
            </button>
          ) : (
            <button
              onClick={() => setCollapsed(!collapsed)}
              className="p-1 rounded-md text-text-subtle hover:text-text-main hover:bg-surface-hover transition-colors"
              aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
              title={collapsed ? 'Expand Your Library' : 'Collapse Your Library'}
            >
              {collapsed ? <ChevronRight size={17} /> : <ChevronLeft size={17} />}
            </button>
          )}
        </div>

        <div className="space-y-0.5">
          <SidebarItem to="/" icon={Home} label="Home" collapsed={effectiveCollapsed} onNavigate={onMobileClose} />
          <SidebarItem to="/search" icon={Search} label="Search" collapsed={effectiveCollapsed} onNavigate={onMobileClose} />
          <SidebarItem to="/dj" icon={Disc3} label="DJ Mode" collapsed={effectiveCollapsed} onNavigate={onMobileClose} />
        </div>
      </div>

      {/* Bottom Library & Tools Island */}
      <div className={`bg-surface-1 ${isDJRoute || isMobile ? '' : 'md:rounded-lg'} flex-1 flex flex-col min-h-0 overflow-hidden`}>
        {/* Library Header */}
        <div className={`flex items-center gap-2.5 px-4 pt-3 pb-2 text-text-secondary flex-shrink-0 ${effectiveCollapsed ? 'justify-center px-2' : ''}`}>
          <Library size={18} className="flex-shrink-0" aria-hidden="true" />
          {!effectiveCollapsed && (
            <span className="text-[13px] font-bold tracking-tight text-text-secondary">
              Your Library
            </span>
          )}
        </div>

        {/* Scrollable Library Sections */}
        <div className="flex-1 overflow-y-auto px-2 pb-2 space-y-3">
          {/* Browse Library */}
          <div className="space-y-0.5">
            <SidebarItem to="/songs" icon={Music} label="Songs" collapsed={effectiveCollapsed} onNavigate={onMobileClose} />
            <SidebarItem to="/albums" icon={Disc} label="Albums" collapsed={effectiveCollapsed} onNavigate={onMobileClose} />
            <SidebarItem to="/artists" icon={Mic2} label="Artists" collapsed={effectiveCollapsed} onNavigate={onMobileClose} />
            <SidebarItem to="/genres" icon={Tags} label="Genres" collapsed={effectiveCollapsed} onNavigate={onMobileClose} />
            <SidebarItem to="/playlists" icon={ListMusic} label="Playlists" collapsed={effectiveCollapsed} onNavigate={onMobileClose} />
          </div>

          {/* Collections */}
          <div className="space-y-0.5">
            {!effectiveCollapsed && (
              <div className="px-3 pt-1 pb-1 text-[11px] font-bold uppercase tracking-wider text-text-subtle">
                Collections
              </div>
            )}
            {effectiveCollapsed && <div className="my-1 border-t border-surface-border mx-2" role="separator" />}
            <SidebarItem to="/liked" icon={Heart} label="Liked Songs" collapsed={effectiveCollapsed} onNavigate={onMobileClose} />
            <SidebarItem to="/liked-albums" icon={Disc} label="Liked Albums" collapsed={effectiveCollapsed} onNavigate={onMobileClose} />
            <SidebarItem to="/smart-playlists" icon={Sparkles} label="AI Smart Mix" collapsed={effectiveCollapsed} onNavigate={onMobileClose} />
          </div>

          {/* Services & Utilities */}
          <div className="space-y-0.5">
            {!effectiveCollapsed && (
              <div className="px-3 pt-1 pb-1 text-[11px] font-bold uppercase tracking-wider text-text-subtle">
                Workspace
              </div>
            )}
            {effectiveCollapsed && <div className="my-1 border-t border-surface-border mx-2" role="separator" />}
            <SidebarItem to="/spotify" icon={SpotifyIcon} label="Spotify" collapsed={effectiveCollapsed} onNavigate={onMobileClose} />
            <SidebarItem to="/downloads" icon={Download} label="Downloads" badge={downloadCount} collapsed={effectiveCollapsed} onNavigate={onMobileClose} />
            <SidebarItem to="/stats" icon={BarChart3} label="Stats" collapsed={effectiveCollapsed} onNavigate={onMobileClose} />
            <SidebarItem to="/duplicates" icon={Copy} label="Duplicates" collapsed={effectiveCollapsed} onNavigate={onMobileClose} />
            <SidebarItem to="/settings" icon={Settings} label="Settings" collapsed={effectiveCollapsed} onNavigate={onMobileClose} />
          </div>
        </div>

        {/* Status Footer (only rendered or compact when active) */}
        {hasStatusActivity && (
          <div className={`p-2.5 border-t border-surface-border bg-surface-2/60 flex-shrink-0 ${effectiveCollapsed ? 'px-1.5' : ''}`}>
            {activeAnalysisJob ? (
              <div
                role="status"
                aria-live="polite"
                aria-label={activeAnalysisJob.progressTotal > 0 ? `Analyzing ${activeAnalysisJob.progressCurrent} of ${activeAnalysisJob.progressTotal}` : 'Preparing track analysis'}
                className={`rounded-md bg-surface-3 p-2.5 ${effectiveCollapsed ? 'p-1.5' : ''}`}
              >
                <div className={`flex items-center gap-2.5 text-brand ${effectiveCollapsed ? 'justify-center' : ''}`}>
                  <Gauge size={16} className="animate-pulse motion-reduce:animate-none flex-shrink-0" />
                  {!effectiveCollapsed && (
                    <div className="min-w-0 flex-1">
                      <span className="block text-[11px] font-bold text-text-main truncate">
                        {activeAnalysisJob.progressTotal > 0 ? `Analyzing ${Math.min(activeAnalysisJob.progressCurrent, activeAnalysisJob.progressTotal)}/${activeAnalysisJob.progressTotal}` : 'Analyzing tracks'}
                      </span>
                      <span className="block truncate text-[10px] text-text-secondary">{activeAnalysisJob.message || 'Preparing your library…'}</span>
                    </div>
                  )}
                </div>
                {!effectiveCollapsed && (
                  <div className="mt-2 h-1 w-full overflow-hidden rounded-full bg-surface-0">
                    <div
                      className="h-full rounded-full bg-brand transition-all duration-300 ease-out motion-reduce:transition-none"
                      style={{ width: activeAnalysisJob.progressTotal > 0 ? `${Math.min(100, Math.round((activeAnalysisJob.progressCurrent / activeAnalysisJob.progressTotal) * 100))}%` : '12%' }}
                    />
                  </div>
                )}
              </div>
            ) : enrichmentStatus.isEnriching ? (
              <div
                role="status"
                aria-live="polite"
                aria-label="AI metadata enrichment in progress"
                className={`rounded-md bg-surface-3 p-2.5 ${effectiveCollapsed ? 'p-1.5' : ''}`}
              >
                <div className={`flex items-center gap-2.5 text-brand ${effectiveCollapsed ? 'justify-center' : ''}`}>
                  <Loader2 size={16} className="animate-spin motion-reduce:animate-none flex-shrink-0" />
                  {!effectiveCollapsed && (
                    <div className="min-w-0 flex-1">
                      <span className="block text-[11px] font-bold text-text-main truncate">Enriching metadata</span>
                      <span className="block truncate text-[10px] text-text-secondary">{enrichmentStatus.message || 'Updating library…'}</span>
                    </div>
                  )}
                </div>
                {!effectiveCollapsed && (
                  <>
                    <div className="mt-2 h-1 w-full overflow-hidden rounded-full bg-surface-0">
                      <div
                        className="h-full rounded-full bg-brand transition-all duration-300 ease-out motion-reduce:transition-none"
                        style={{
                          width: enrichmentStatus.totalSongs > 0
                            ? `${Math.min(100, Math.round((enrichmentStatus.processedSongs / enrichmentStatus.totalSongs) * 100))}%`
                            : '12%'
                        }}
                      />
                    </div>
                    <div className="mt-1 flex justify-between text-[10px] font-mono text-text-subtle">
                      <span>{enrichmentStatus.totalSongs > 0 ? `${enrichmentStatus.processedSongs}/${enrichmentStatus.totalSongs}` : 'Starting…'}</span>
                      {enrichmentStatus.totalBatches > 0 && <span>Batch {enrichmentStatus.currentBatch}/{enrichmentStatus.totalBatches}</span>}
                    </div>
                  </>
                )}
              </div>
            ) : isScanning ? (
              <div className={`flex items-center gap-2.5 px-1.5 py-1 text-text-secondary ${effectiveCollapsed ? 'justify-center' : ''}`}>
                <Loader2 size={16} className="animate-spin text-brand flex-shrink-0 motion-reduce:animate-none" />
                {!effectiveCollapsed && (
                  <div className="min-w-0 flex-1">
                    <span className="block text-xs font-semibold text-text-main truncate">
                      {scanProgress.toLowerCase().includes('ai enrichment') ? 'Enriching Library…' : 'Scanning Library…'}
                    </span>
                    {scanProgress && <span className="block text-[11px] text-text-subtle truncate">{scanProgress}</span>}
                  </div>
                )}
              </div>
            ) : (
              <div className={`flex items-center gap-2.5 px-1.5 py-1 text-text-secondary ${effectiveCollapsed ? 'justify-center' : ''}`}>
                <Sparkles size={16} className="text-brand flex-shrink-0" />
                {!effectiveCollapsed && (
                  <div className="min-w-0 flex-1">
                    <span className="block text-xs font-semibold text-text-main truncate">
                      {enrichmentStatus.message?.toLowerCase().includes('mood') ? 'Moods Updated' : 'Genres Updated'}
                    </span>
                    <span className="block text-[10px] text-text-subtle truncate">{enrichmentStatus.message}</span>
                  </div>
                )}
              </div>
            )}
          </div>
        )}
      </div>
    </nav>
  );

  if (isMobile) {
    return (
      <>
        <div
          className={`fixed inset-0 z-40 bg-surface-0/75 backdrop-blur-sm transition-opacity duration-200 ${
            mobileOpen ? 'opacity-100' : 'opacity-0 pointer-events-none'
          }`}
          aria-hidden={!mobileOpen}
          onClick={onMobileClose}
        />
        <div
          ref={drawerRef}
          tabIndex={-1}
          role="dialog"
          aria-modal="true"
          aria-label="Main navigation"
          aria-hidden={!mobileOpen}
          className={`fixed top-0 left-0 bottom-0 z-50 w-64 max-w-[85vw] bg-surface-1 border-r border-surface-border flex flex-col shadow-2xl transition-transform duration-200 ease-out ${
            mobileOpen ? 'translate-x-0' : '-translate-x-full'
          }`}
        >
          {navContent}
        </div>
      </>
    );
  }

  return (
    <aside
      className={`h-full flex flex-col flex-shrink-0 transition-all duration-200 ease-out ${
        isDJRoute ? 'bg-surface-1 border-r border-surface-border' : 'bg-surface-0'
      } ${effectiveCollapsed ? 'w-[68px]' : 'w-60'}`}
    >
      {navContent}
    </aside>
  );
};
