
import React from 'react';
import { NavLink, useLocation } from 'react-router-dom';
import { 
  Home, Music, Disc, Mic2, ListMusic, 
  Wifi, Download, Search, Settings, Library, Sparkles, Loader2
} from 'lucide-react';
import { useStore } from '../store';

const SidebarItem = ({ to, icon: Icon, label }: { to: string; icon: any; label: string }) => {
  return (
    <NavLink
      to={to}
      className={({ isActive }) =>
        `flex items-center gap-4 px-4 py-3 text-sm font-medium transition-colors duration-200 border-l-4 ${
          isActive
            ? 'border-brand bg-surface-highlight text-text-main'
            : 'border-transparent text-text-secondary hover:text-text-main hover:bg-surface-highlight/50'
        }`
      }
    >
      <Icon size={20} />
      <span>{label}</span>
    </NavLink>
  );
};

export const Sidebar: React.FC = () => {
  const { isScanning, scanProgress } = useStore();

  return (
    <div className="w-64 h-full bg-surface-0 flex flex-col border-r border-surface-highlight">
      <div className="p-6">
        <div className="flex items-center gap-2">
            <div className="w-1 h-6 bg-brand rounded-full"></div>
            <div className="w-1 h-4 bg-brand rounded-full"></div>
            <div className="w-1 h-2 bg-brand rounded-full"></div>
            <h1 className="text-xl font-bold tracking-tight text-text-main ml-2">ViiB MediaHub</h1>
        </div>
      </div>

      <nav className="flex-1 overflow-y-auto py-2">
        <div className="space-y-1">
          <SidebarItem to="/" icon={Home} label="Home" />
          <SidebarItem to="/songs" icon={Music} label="Songs" />
          <SidebarItem to="/albums" icon={Disc} label="Albums" />
          <SidebarItem to="/artists" icon={Mic2} label="Artists" />
          <SidebarItem to="/playlists" icon={ListMusic} label="Playlists" />
          {/* Smart Mixes shortcut - technically playlists, but nice to highlight */}
        </div>

        <div className="my-4 border-t border-surface-highlight mx-4"></div>

        <div className="space-y-1">
          <SidebarItem to="/spotify" icon={Wifi} label="Spotify" />
          <SidebarItem to="/downloads" icon={Download} label="Downloads" />
          <SidebarItem to="/search" icon={Search} label="Search" />
        </div>
        
        <div className="space-y-1 mt-4">
             <SidebarItem to="/settings" icon={Settings} label="Settings" />
        </div>
      </nav>

      <div className="p-4 border-t border-surface-highlight bg-gradient-to-t from-surface-0 to-surface-0/50">
        {isScanning ? (
            <div className="flex flex-col gap-2 text-text-secondary">
                <div className="flex items-center gap-3 text-brand">
                    <Loader2 size={20} className="animate-spin" />
                    <span className="text-sm font-semibold">Importing...</span>
                </div>
                <span className="text-[10px] font-mono text-text-subtle line-clamp-1">{scanProgress}</span>
            </div>
        ) : (
            <div className="flex items-center gap-3 text-text-secondary hover:text-text-main cursor-pointer transition-colors">
                <Library size={20} />
                <div className="flex flex-col">
                    <span className="text-sm font-semibold">Your Library</span>
                    <span className="text-xs text-text-subtle">Local Media Collection</span>
                </div>
            </div>
        )}
      </div>
    </div>
  );
};