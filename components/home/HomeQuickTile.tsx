/** Renders a Home shortcut tile. */

import React from 'react';
import { Play } from 'lucide-react';
import { coverBackground } from '../../utils';
import { cn } from '../ui/cn';
import { HomeQuickTileItem } from './useHomeContent';

interface HomeQuickTileProps {
  item: HomeQuickTileItem;
  compact?: boolean;
  className?: string;
}

export const HomeQuickTile: React.FC<HomeQuickTileProps> = ({ item, compact = false, className }) => {
  return (
    <div
      onContextMenu={item.onContextMenu}
      className={cn(
        'group flex min-w-0 items-center overflow-hidden rounded-md bg-surface-2/90 transition-colors hover:bg-surface-highlight',
        compact ? 'h-14' : 'h-16',
        className
      )}
    >
      <button
        type="button"
        onClick={item.onClick}
        className="flex h-full min-w-0 flex-1 cursor-pointer items-center text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-brand/70"
      >
        <span
          className={cn('h-full flex-shrink-0 bg-surface-3 shadow-lg', compact ? 'w-14' : 'w-16')}
          style={{ background: coverBackground(item.imageUrl, item.fallbackSeed) }}
        />
        <span className="min-w-0 flex-1 px-3.5">
          <span className="block truncate text-sm font-bold text-text-main">{item.title}</span>
          {!compact && item.subtitle ? (
            <span className="mt-0.5 block truncate text-xs text-text-secondary">{item.subtitle}</span>
          ) : null}
        </span>
      </button>
      {item.onPlay ? (
        <button
          type="button"
          onClick={() => item.onPlay?.()}
          className="mr-2.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-full bg-brand text-surface-0 opacity-100 shadow-lg transition-all md:opacity-0 md:group-hover:opacity-100 md:group-focus-within:opacity-100 hover:scale-105 hover:bg-brand-hover focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/70"
          aria-label={`Play ${item.title}`}
        >
          <Play size={16} className="ml-0.5 fill-current" aria-hidden="true" />
        </button>
      ) : null}
    </div>
  );
};
