/** Renders the featured Home content item and its actions. */

import React from 'react';
import { Disc3, Play, UserRound } from 'lucide-react';
import { coverBackground } from '../../utils';
import { cn } from '../ui/cn';
import { HomeSpotlightItem } from './useHomeContent';

interface HomeSpotlightProps {
  item: HomeSpotlightItem | null;
  className?: string;
  compact?: boolean;
}

export const HomeSpotlight: React.FC<HomeSpotlightProps> = ({ item, className, compact = false }) => {
  if (!item) {
    return (
      <div className={cn('rounded-lg bg-gradient-to-br from-surface-3 to-surface-2 p-6', className)}>
        <p className="text-xs font-bold uppercase tracking-widest text-text-secondary">Library</p>
        <h2 className="mt-2 text-display font-bold text-text-main">Ready when you are</h2>
        <p className="mt-2 max-w-xl text-sm text-text-secondary">
          Add a music folder in Settings to browse albums, artists, and mixes.
        </p>
      </div>
    );
  }

  const FallbackIcon = item.kind === 'artist' ? UserRound : Disc3;
  const artworkStyle =
    item.kind === 'smartMix' && item.coverColors?.length
      ? { background: `linear-gradient(135deg, ${item.coverColors[0]}, ${item.coverColors[1]})` }
      : { background: coverBackground(item.imageUrl, item.fallbackSeed) };

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={item.onClick}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          item.onClick();
        }
      }}
      className={cn(
        'group relative flex cursor-pointer flex-col gap-5 overflow-hidden rounded-lg bg-gradient-to-br from-surface-3 via-surface-2 to-surface-2 p-5 transition-colors hover:from-surface-highlight hover:via-surface-3 hover:to-surface-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/70 sm:flex-row sm:items-center',
        compact ? 'min-h-[180px]' : 'min-h-[210px]',
        className
      )}
    >
      <div
        className={cn(
          'aspect-square w-32 flex-shrink-0 overflow-hidden bg-surface-3 shadow-2xl sm:w-40',
          item.kind === 'artist' ? 'rounded-full' : 'rounded-md'
        )}
        style={artworkStyle}
      >
        {!item.imageUrl && item.kind !== 'smartMix' ? (
          <span className="flex h-full w-full items-center justify-center text-text-main/40">
            <FallbackIcon size={48} aria-hidden="true" />
          </span>
        ) : null}
      </div>

      <div className="relative z-10 flex min-w-0 flex-1 flex-col justify-center">
        <p className="text-[11px] font-bold uppercase tracking-widest text-text-secondary">
          {item.kind === 'album' ? 'Featured Album' : item.kind === 'artist' ? 'Featured Artist' : 'Smart Mix'}
        </p>
        <h2 className={cn('mt-1 truncate font-bold tracking-tight text-text-main', compact ? 'text-section' : 'text-display')}>
          {item.title}
        </h2>
        <p className="mt-1 text-sm font-medium text-text-secondary">{item.subtitle}</p>
        <p className="mt-1 line-clamp-1 text-xs text-text-subtle">{item.description}</p>
        <div className="mt-4 flex items-center gap-3">
          {item.onPlay ? (
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                item.onPlay?.();
              }}
              className="inline-flex h-10 items-center gap-2 rounded-full bg-brand px-6 text-sm font-bold text-surface-0 transition-transform hover:scale-105 hover:bg-brand-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/70"
            >
              <Play size={16} className="fill-current" aria-hidden="true" />
              Play
            </button>
          ) : null}
          <span className="rounded-full border border-surface-border px-4 py-2 text-xs font-bold text-text-main transition-colors group-hover:border-text-secondary">
            View
          </span>
        </div>
      </div>
    </div>
  );
};
