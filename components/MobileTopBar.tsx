/** Displays the current route title and mobile navigation control. */

import React from 'react';
import { useLocation } from 'react-router';
import { Menu } from 'lucide-react';
import SmallLogo from './icons/Icon_1_clear-high-res.png';

const ROUTE_TITLES: Array<[RegExp, string]> = [
  [/^\/$/, 'Home'],

  [/^\/dj/, 'DJ Mode'],
  [/^\/songs/, 'Songs'],
  [/^\/albums(?:\/|$)/, 'Albums'],
  [/^\/artists(?:\/|$)/, 'Artists'],
  [/^\/genres(?:\/|$)/, 'Genres'],
  [/^\/playlists/, 'Playlists'],
  [/^\/liked-albums/, 'Liked Albums'],
  [/^\/liked/, 'Liked Songs'],
  [/^\/smart-playlists/, 'AI Smart Mix'],
  [/^\/smart-mix/, 'Smart Mix'],
  [/^\/spotify/, 'Spotify'],
  [/^\/downloads/, 'Downloads'],
  [/^\/search/, 'Search'],
  [/^\/stats/, 'Stats'],
  [/^\/settings/, 'Settings'],
];

const titleForPath = (path: string) => {
  for (const [re, title] of ROUTE_TITLES) {
    if (re.test(path)) return title;
  }
  return 'ViiB MediaHub';
};

export interface MobileTopBarProps {
  onOpenMenu: () => void;
}

export const MobileTopBar: React.FC<MobileTopBarProps> = ({ onOpenMenu }) => {
  const location = useLocation();
  const title = titleForPath(location.pathname);

  return (
    <header className="md:hidden flex-shrink-0 h-12 bg-surface-1 border-b border-surface-border flex items-center justify-between px-3 z-30">
      <button
        onClick={onOpenMenu}
        className="p-1.5 rounded-md text-text-main hover:bg-surface-hover transition-colors"
        aria-label="Open navigation menu"
      >
        <Menu size={20} />
      </button>
      <div className="flex items-center gap-2 min-w-0">
        <img src={SmallLogo} alt="" aria-hidden="true" className="h-5 w-5 object-contain" />
        <span className="text-sm font-bold text-text-main truncate">{title}</span>
      </div>
      <div className="w-8" aria-hidden="true" />
    </header>
  );
};
