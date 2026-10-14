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
          onClick={(e) => {
            e.stopPropagation();
            onPlay();
          }}
          className="absolute bottom-1 right-1 flex h-11 w-11 translate-y-2 items-center justify-center rounded-full bg-brand text-surface-0 opacity-0 shadow-xl transition-all group-hover:translate-y-0 group-hover:opacity-100 hover:scale-105 hover:bg-brand-hover focus-visible:translate-y-0 focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/70"
          aria-label={`Play ${artist.name}`}
        >
          <Play size={20} className="ml-0.5 fill-current" aria-hidden="true" />
        </button>
      </div>
      <h3 className="truncate text-sm font-semibold text-text-main">
        {artist.name}
      </h3>
      <p className="mt-1 truncate text-xs text-text-secondary">Artist</p>
    </div>
  );
};
