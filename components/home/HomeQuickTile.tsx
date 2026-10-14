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
      role="button"
      tabIndex={0}
      onClick={item.onClick}
      onContextMenu={item.onContextMenu}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          item.onClick();
        }
      }}
      className={cn(
        'group flex min-w-0 cursor-pointer items-center overflow-hidden rounded-md bg-surface-2/90 text-left transition-colors hover:bg-surface-highlight focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/70',
        compact ? 'h-14' : 'h-16',
        className
      )}
    >
      <div
        className={cn('h-full flex-shrink-0 bg-surface-3 shadow-lg', compact ? 'w-14' : 'w-16')}
        style={{ background: coverBackground(item.imageUrl, item.fallbackSeed) }}
      />
      <div className="min-w-0 flex-1 px-3.5">
        <h3 className="truncate text-sm font-bold text-text-main">{item.title}</h3>
        {!compact && <p className="mt-0.5 truncate text-xs text-text-secondary">{item.subtitle}</p>}
      </div>
      {item.onPlay ? (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            item.onPlay?.();
          }}
          className="mr-3 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-full bg-brand text-surface-0 opacity-0 shadow-lg transition-all group-hover:opacity-100 hover:scale-105 hover:bg-brand-hover focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/70"
          aria-label={`Play ${item.title}`}
        >
          <Play size={16} className="ml-0.5 fill-current" aria-hidden="true" />
        </button>
      ) : null}
    </div>
  );
};
