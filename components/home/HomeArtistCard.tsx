/** Renders a Home artist card with artwork and supplied actions. */

import React from 'react';
import { Play, UserRound } from 'lucide-react';
import { Artist } from '../../types';
import { cssUrl, generateGradient } from '../../utils';
import { cn } from '../ui/cn';

interface HomeArtistCardProps {
  artist: Artist;
  imageUrl?: string;
  onClick: () => void;
  onPlay: () => void;
  onContextMenu: (e: React.MouseEvent) => void;
  size?: 'default' | 'compact';
  className?: string;
}

export const HomeArtistCard: React.FC<HomeArtistCardProps> = ({
  artist,
  imageUrl,
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
      <div className="relative mx-auto mb-3 aspect-square w-full">
        <div
          className="h-full w-full overflow-hidden rounded-full bg-surface-3 shadow-xl"
          style={{ background: imageUrl ? `${cssUrl(imageUrl)} center/cover no-repeat` : generateGradient(artist.name) }}
        >
          {!imageUrl && (
            <span className="flex h-full w-full items-center justify-center text-text-main/60">
              <UserRound size={compact ? 36 : 44} aria-hidden="true" />
            </span>
          )}
        </div>
        <button
          type="button"
          onClick={onPlay}
          className="absolute bottom-1 right-1 z-10 flex h-11 w-11 translate-y-0 items-center justify-center rounded-full bg-brand text-surface-0 opacity-100 shadow-xl transition-all md:translate-y-2 md:opacity-0 md:group-hover:translate-y-0 md:group-hover:opacity-100 md:group-focus-within:translate-y-0 md:group-focus-within:opacity-100 hover:scale-105 hover:bg-brand-hover focus-visible:translate-y-0 focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/70"
          aria-label={`Play ${artist.name}`}
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
          {artist.name}
        </span>
        <span className="mt-1 block truncate text-xs text-text-secondary">Artist</span>
      </button>
    </div>
  );
};
