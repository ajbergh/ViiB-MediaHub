import React from 'react';
import { useLocation } from 'react-router-dom';
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
  [/^\/smart-playlists/, 'AI DJ'],
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
    <header className="md:hidden flex-shrink-0 h-14 bg-surface-0 border-b border-surface-3 flex items-center justify-between px-3 z-30">
      <button
        onClick={onOpenMenu}
        className="p-2 rounded-lg text-text-main hover:bg-surface-2/60 transition-colors"
        aria-label="Open navigation menu"
      >
        <Menu size={22} />
      </button>
      <div className="flex items-center gap-2 min-w-0">
        <img src={SmallLogo} alt="" aria-hidden="true" className="h-7 w-7 object-contain" />
        <span className="text-sm font-semibold text-text-main truncate">{title}</span>
      </div>
      <div className="w-10" aria-hidden="true" />
    </header>
  );
};
