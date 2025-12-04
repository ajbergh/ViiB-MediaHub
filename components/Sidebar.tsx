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
 * - Playlists: User playlists
 * - Spotify: Spotify integration hub
 * - Downloads: Download queue (with active count badge)
 * - Settings: Application configuration
 * 
 * Highlights active route and shows loading indicator during scans.
 * 
 * @module Sidebar
 */

import React, { useState } from 'react';
import { NavLink, useLocation } from 'react-router-dom';
import { 
  Home, Music, Disc, Mic2, ListMusic, 
  Wifi, Download, Search, Settings, Library, Sparkles, Loader2,
  ChevronLeft, ChevronRight, Menu
} from 'lucide-react';
import { useStore } from '../store';

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
        `flex items-center gap-4 px-4 py-3 text-sm font-medium transition-all duration-200 border-l-4 relative ${
          isActive
            ? 'border-brand bg-surface-highlight text-text-main'
            : 'border-transparent text-text-secondary hover:text-text-main hover:bg-surface-highlight/50'
        } ${collapsed ? 'justify-center px-2' : ''}`
      }
    >
      <Icon size={20} className="flex-shrink-0" aria-hidden="true" />
      {!collapsed && <span className="flex-1">{label}</span>}
      {!collapsed && badge !== undefined && badge > 0 && (
        <span className="bg-brand text-white text-[10px] font-bold px-1.5 py-0.5 rounded-full min-w-[18px] text-center" aria-label={`${badge} items`}>
          {badge}
        </span>
      )}
      {collapsed && badge !== undefined && badge > 0 && (
        <span className="absolute -top-1 -right-1 bg-brand text-white text-[10px] font-bold w-4 h-4 rounded-full flex items-center justify-center" aria-label={`${badge} items`}>
          {badge > 9 ? '9+' : badge}
        </span>
      )}
    </NavLink>
  );
};

export const Sidebar: React.FC = () => {
  const { isScanning, scanProgress, downloadCount } = useStore();
  const [collapsed, setCollapsed] = useState(false);

  return (
    <div 
      className={`h-full bg-surface-0 flex flex-col border-r border-surface-highlight transition-all duration-300 ${
        collapsed ? 'w-16' : 'w-64'
      }`}
    >
      <div className={`p-4 ${collapsed ? 'px-2' : 'p-6'} flex items-center justify-between`}>
        {!collapsed && (
          <div className="flex items-center gap-2">
            <div className="w-1 h-6 bg-brand rounded-full"></div>
            <div className="w-1 h-4 bg-brand rounded-full"></div>
            <div className="w-1 h-2 bg-brand rounded-full"></div>
            <h1 className="text-xl font-bold tracking-tight text-text-main ml-2">ViiB MediaHub</h1>
          </div>
        )}
        {collapsed && (
          <div className="flex items-center gap-1 mx-auto">
            <div className="w-1 h-5 bg-brand rounded-full"></div>
            <div className="w-1 h-3 bg-brand rounded-full"></div>
          </div>
        )}
        <button
          onClick={() => setCollapsed(!collapsed)}
          className={`p-1.5 rounded-lg text-text-subtle hover:text-text-main hover:bg-surface-hover transition-colors ${collapsed ? 'absolute right-1 top-4' : ''}`}
          aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
        >
          {collapsed ? <ChevronRight size={16} /> : <ChevronLeft size={16} />}
        </button>
      </div>

      <nav className="flex-1 overflow-y-auto py-2" role="navigation" aria-label="Main navigation">
        <div className="space-y-1">
          <SidebarItem to="/" icon={Home} label="Home" collapsed={collapsed} />
          <SidebarItem to="/songs" icon={Music} label="Songs" collapsed={collapsed} />
          <SidebarItem to="/albums" icon={Disc} label="Albums" collapsed={collapsed} />
          <SidebarItem to="/artists" icon={Mic2} label="Artists" collapsed={collapsed} />
          <SidebarItem to="/playlists" icon={ListMusic} label="Playlists" collapsed={collapsed} />
        </div>

        <div className={`my-4 border-t border-surface-highlight ${collapsed ? 'mx-2' : 'mx-4'}`} role="separator"></div>

        <div className="space-y-1">
          <SidebarItem to="/spotify" icon={Wifi} label="Spotify" collapsed={collapsed} />
          <SidebarItem to="/downloads" icon={Download} label="Downloads" badge={downloadCount} collapsed={collapsed} />
          <SidebarItem to="/search" icon={Search} label="Search" collapsed={collapsed} />
        </div>
        
        <div className="space-y-1 mt-4">
          <SidebarItem to="/settings" icon={Settings} label="Settings" collapsed={collapsed} />
        </div>
      </nav>

      <div className={`p-4 border-t border-surface-highlight bg-gradient-to-t from-surface-0 to-surface-0/50 ${collapsed ? 'px-2' : ''}`}>
        {isScanning ? (
          <div className={`flex flex-col gap-2 text-text-secondary ${collapsed ? 'items-center' : ''}`}>
            <div className={`flex items-center gap-3 text-brand ${collapsed ? 'justify-center' : ''}`}>
              <Loader2 size={20} className="animate-spin" />
              {!collapsed && <span className="text-sm font-semibold">Importing...</span>}
            </div>
            {!collapsed && <span className="text-[10px] font-mono text-text-subtle line-clamp-1">{scanProgress}</span>}
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