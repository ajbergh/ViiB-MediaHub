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
        'group relative flex-shrink-0 cursor-pointer rounded-lg bg-surface-2 text-left ring-1 ring-transparent transition-all hover:bg-surface-3 hover:ring-surface-border focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/70',
        compact ? 'w-40 p-3' : 'w-48 p-4',
        className
      )}
    >
      <div
        className="relative mb-3 aspect-square w-full overflow-hidden rounded-md bg-surface-3 shadow-lg"
        style={{ background: coverBackground(album.coverUrl, album.name) }}
      >
        {!album.coverUrl && (
          <span className="flex h-full w-full items-center justify-center text-display font-bold text-white/20">
            {album.name.charAt(0)}
          </span>
        )}
        <span className="absolute inset-0 bg-black/30 opacity-0 transition-opacity group-hover:opacity-100" />
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            onPlay();
          }}
          className="absolute bottom-2 right-2 flex h-10 w-10 translate-y-2 items-center justify-center rounded-full bg-brand text-black opacity-0 shadow-xl transition-all group-hover:translate-y-0 group-hover:opacity-100 hover:scale-105 focus-visible:translate-y-0 focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/70 focus-visible:ring-offset-2 focus-visible:ring-offset-surface-2"
          aria-label={`Play ${album.name}`}
        >
          <Play size={18} className="ml-0.5 fill-current" aria-hidden="true" />
        </button>
      </div>
      <h3 className={cn('truncate font-semibold text-text-main', compact ? 'text-sm' : 'text-base')}>
        {album.name}
      </h3>
      <p className="mt-1 truncate text-sm text-text-secondary">{album.artist}</p>
      {!compact && (
        <p className="mt-2 text-xs text-text-subtle">
          {album.songCount} {album.songCount === 1 ? 'track' : 'tracks'}
        </p>
      )}
    </div>
  );
};
