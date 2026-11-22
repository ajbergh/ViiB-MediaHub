/** Renders a Home album card and its supplied navigation/playback actions. */

import React from 'react';
import { Play } from 'lucide-react';
import { Album } from '../../types';
import { coverBackground } from '../../utils';
import { cn } from '../ui/cn';

interface HomeAlbumCardProps {
  album: Album;
  onClick: () => void;
  onPlay: () => void;
  onContextMenu: (e: React.MouseEvent) => void;
  size?: 'default' | 'compact';
  className?: string;
}

export const HomeAlbumCard: React.FC<HomeAlbumCardProps> = ({
  album,
  onClick,
  onPlay,
  onContextMenu,
  size = 'default',
  className,
}) => {
  const compact = size === 'compact';

  return (
    <div
      onContextMenu={onContextMenu}
      className={cn(
        'group relative flex-shrink-0 rounded-lg bg-transparent p-3 text-left transition-colors hover:bg-surface-hover',
        compact ? 'w-40' : 'w-44',
        className
      )}
    >
      <div
        className="relative mb-3 aspect-square w-full overflow-hidden rounded-md bg-surface-3 shadow-xl"
        style={{ background: coverBackground(album.coverUrl, album.name) }}
      >
        {!album.coverUrl && (
          <span className="flex h-full w-full items-center justify-center text-display font-bold text-text-main/20">
            {album.name.charAt(0)}
          </span>
        )}
        <button
          type="button"
          onClick={onPlay}
          className="absolute bottom-2 right-2 z-10 flex h-11 w-11 translate-y-0 items-center justify-center rounded-full bg-brand text-surface-0 opacity-100 shadow-xl transition-all md:translate-y-2 md:opacity-0 md:group-hover:translate-y-0 md:group-hover:opacity-100 md:group-focus-within:translate-y-0 md:group-focus-within:opacity-100 hover:scale-105 hover:bg-brand-hover focus-visible:translate-y-0 focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/70"
          aria-label={`Play ${album.name}`}
        >
          <Play size={20} className="ml-0.5 fill-current" aria-hidden="true" />
        </button>
      </div>
      <button
        type="button"
        onClick={onClick}
        className="block w-full cursor-pointer rounded text-left focus-visible:outline-none after:absolute after:inset-0 after:rounded-lg focus-visible:after:ring-2 focus-visible:after:ring-brand/70"
      >
        <span className="block truncate text-sm font-semibold text-text-main">
          {album.name}
        </span>
        <span className="mt-1 block truncate text-xs text-text-secondary">{album.artist}</span>
      </button>
    </div>
  );
};
