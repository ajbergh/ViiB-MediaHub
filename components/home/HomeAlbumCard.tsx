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
      role="button"
      tabIndex={0}
      onClick={onClick}
      onContextMenu={onContextMenu}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          onClick();
        }
      }}
      className={cn(
        'group relative flex-shrink-0 cursor-pointer rounded-lg bg-transparent p-3 text-left transition-colors hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/70',
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
          onClick={(e) => {
            e.stopPropagation();
            onPlay();
          }}
          className="absolute bottom-2 right-2 flex h-11 w-11 translate-y-2 items-center justify-center rounded-full bg-brand text-surface-0 opacity-0 shadow-xl transition-all group-hover:translate-y-0 group-hover:opacity-100 hover:scale-105 hover:bg-brand-hover focus-visible:translate-y-0 focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/70"
          aria-label={`Play ${album.name}`}
        >
          <Play size={20} className="ml-0.5 fill-current" aria-hidden="true" />
        </button>
      </div>
      <h3 className="truncate text-sm font-semibold text-text-main">
        {album.name}
      </h3>
      <p className="mt-1 truncate text-xs text-text-secondary">{album.artist}</p>
    </div>
  );
};
