/**
 * ViiB MediaHub - Sidebar Navigation Component
 * 
 * Fixed left sidebar providing navigation between application sections.
 * Collapses to icons only on smaller screens.
 * 
 * Sections:
 * - Home: Landing page with smart mixes
 * - Songs: Complete song library
 * - Albums: Album grid view
 * - Artists: Artist grid view
 * - Genres: Genre listing
 * - Playlists: User playlists
 * - Smart Playlists: AI DJ generated playlists
 * - Liked Songs: Favorited songs
 * - Liked Albums: Favorited albums
 * - Spotify: Spotify integration hub
 * - Downloads: Download queue (with active count badge)
 * - Stats: Listening statistics
 * - Settings: Application configuration
 * 
 * Highlights active route and shows loading indicator during scans.
 * 
 * @module Sidebar
 */

import React, { useState } from 'react';
import { NavLink, useLocation } from 'react-router-dom';
import { 
  Home, Music, Disc, Mic2, ListMusic, Tags,
  Download, Search, Settings, Library, Sparkles, Loader2,
  ChevronLeft, ChevronRight, Menu, BarChart3, Heart
} from 'lucide-react';
import { useStore } from '../store';
import { SpotifyIcon } from './icons/SpotifyIcon';
import LargeLogo from './icons/Large-Logo1-clear-highres.png';
import SmallLogo from './icons/Icon_1_clear-high-res.png';

interface SidebarItemProps {
  to: string;
  icon: any;
  label: string;
  badge?: number;
  collapsed?: boolean;
}

const SidebarItem = ({ to, icon: Icon, label, badge, collapsed }: SidebarItemProps) => {
  return (
    <NavLink
      to={to}
      title={collapsed ? label : undefined}
      aria-label={label}
      className={({ isActive }) =>
        `flex items-center gap-4 px-4 py-3 text-sm font-medium transition-all duration-200 ease-out border-l-4 relative ${
          isActive
            ? 'border-brand bg-surface-2 text-brand ring-1 ring-brand/15'
            : 'border-transparent text-text-secondary hover:text-text-main hover:bg-surface-2/60'
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

export const Sidebar: React.FC = () => {
  const { isScanning, scanProgress, downloadCount, enrichmentStatus } = useStore();
  const [collapsed, setCollapsed] = useState(false);

  // Debug: Log enrichment status changes
  console.log('🎨 Sidebar render - enrichmentStatus:', { 
    isEnriching: enrichmentStatus.isEnriching, 
    totalSongs: enrichmentStatus.totalSongs,
    processedSongs: enrichmentStatus.processedSongs 
  });

  return (
    <div 
      className={`h-full bg-surface-0 flex flex-col border-r border-surface-3 transition-all duration-300 ease-out ${
        collapsed ? 'w-16' : 'w-64'
      }`}
    >
      <div className={`p-4 ${collapsed ? 'px-2 flex-col items-center' : 'p-6 items-center justify-between'} flex`}>
        {!collapsed && (
          <div className="flex items-center">
            <img 
              src={LargeLogo} 
              alt="ViiB MediaHub" 
              className="h-32 w-auto object-contain"
            />
          </div>
        )}
        {collapsed && (
          <div className="flex items-center justify-center">
            <img 
              src={SmallLogo} 
              alt="ViiB" 
              className="h-32 w-32 object-contain"
            />
          </div>
        )}
        <button
          onClick={() => setCollapsed(!collapsed)}
          className={`p-1.5 rounded-lg text-text-subtle hover:text-text-main hover:bg-surface-2/60 transition-colors ${collapsed ? 'mt-2' : ''}`}
          aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
        >
          {collapsed ? <ChevronRight size={20} /> : <ChevronLeft size={20} />}
        </button>
      </div>

      <nav className="flex-1 overflow-y-auto py-2" role="navigation" aria-label="Main navigation">
        <div className="space-y-1">
          <SidebarItem to="/" icon={Home} label="Home" collapsed={collapsed} />
          <SidebarItem to="/songs" icon={Music} label="Songs" collapsed={collapsed} />
          <SidebarItem to="/albums" icon={Disc} label="Albums" collapsed={collapsed} />
        <SidebarItem to="/artists" icon={Mic2} label="Artists" collapsed={collapsed} />
        <SidebarItem to="/genres" icon={Tags} label="Genres" collapsed={collapsed} />
        <SidebarItem to="/smart-playlists" icon={Sparkles} label="AI DJ" collapsed={collapsed} />
        <SidebarItem to="/playlists" icon={ListMusic} label="Playlists" collapsed={collapsed} />
        <SidebarItem to="/liked" icon={Heart} label="Liked Songs" collapsed={collapsed} />
        <SidebarItem to="/liked-albums" icon={Disc} label="Liked Albums" collapsed={collapsed} />
        </div>

        <div className={`my-4 border-t border-surface-highlight ${collapsed ? 'mx-2' : 'mx-4'}`} role="separator"></div>

        <div className="space-y-1">
          <SidebarItem to="/spotify" icon={SpotifyIcon} label="Spotify" collapsed={collapsed} />
          <SidebarItem to="/downloads" icon={Download} label="Downloads" badge={downloadCount} collapsed={collapsed} />
          <SidebarItem to="/search" icon={Search} label="Search" collapsed={collapsed} />
          <SidebarItem to="/stats" icon={BarChart3} label="Stats" collapsed={collapsed} />
        </div>
        
        <div className="space-y-1 mt-4">
          <SidebarItem to="/settings" icon={Settings} label="Settings" collapsed={collapsed} />
        </div>
      </nav>

      <div className={`p-4 border-t border-surface-3 bg-surface-0 ${collapsed ? 'px-2' : ''}`}>
        {isScanning ? (
          <div className={`flex flex-col gap-2 text-text-secondary ${collapsed ? 'items-center' : ''}`}>
            <div className={`flex items-center gap-3 text-brand ${collapsed ? 'justify-center' : ''}`}>
              <Loader2 size={20} className="animate-spin motion-reduce:animate-none" />
              {!collapsed && <span className="text-sm font-semibold">Scanning...</span>}
            </div>
            {!collapsed && scanProgress && <span className="text-xs text-text-subtle line-clamp-1">{scanProgress}</span>}
          </div>
        ) : enrichmentStatus.isEnriching ? (
          <div className={`flex flex-col gap-2 text-text-secondary ${collapsed ? 'items-center' : ''}`}>
            <div className={`flex items-center gap-3 text-brand ${collapsed ? 'justify-center' : ''}`}>
              <Sparkles size={20} className="animate-pulse motion-reduce:animate-none" />
              {!collapsed && (
                <span className="text-sm font-semibold">
                  {enrichmentStatus.message?.toLowerCase().includes('mood') ? 'Analyzing Moods...' : 'Enriching Genres...'}
                </span>
              )}
            </div>
            {!collapsed && (
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
          <div className={`flex flex-col gap-2 text-text-secondary ${collapsed ? 'items-center' : ''}`}>
            <div className={`flex items-center gap-3 text-accent-green ${collapsed ? 'justify-center' : ''}`}>
              <Sparkles size={20} />
              {!collapsed && (
                <span className="text-sm font-semibold">
                  {enrichmentStatus.message?.toLowerCase().includes('mood') ? 'Moods Updated!' : 'Genres Updated!'}
                </span>
              )}
            </div>
            {!collapsed && <span className="text-[10px] text-text-subtle line-clamp-1">{enrichmentStatus.message}</span>}
          </div>
        ) : (
          <div className={`flex items-center gap-3 text-text-secondary hover:text-text-main cursor-pointer transition-colors ${collapsed ? 'justify-center' : ''}`}>
            <Library size={20} />
            {!collapsed && (
              <div className="flex flex-col">
                <span className="text-sm font-semibold">Your Library</span>
                <span className="text-xs text-text-subtle">Local Media Collection</span>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
};