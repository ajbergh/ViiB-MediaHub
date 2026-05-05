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
        'group flex min-w-[260px] flex-shrink-0 cursor-pointer items-center overflow-hidden rounded-lg bg-surface-2 text-left ring-1 ring-transparent transition-all hover:bg-surface-3 hover:ring-surface-border focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/70',
        compact ? 'h-16' : 'h-20',
        className
      )}
    >
      <div
        className={cn('h-full flex-shrink-0 bg-surface-3', compact ? 'w-16' : 'w-20')}
        style={{ background: coverBackground(item.imageUrl, item.fallbackSeed) }}
      />
      <div className="min-w-0 flex-1 px-4">
        <h3 className="truncate font-semibold text-text-main">{item.title}</h3>
        <p className="mt-1 truncate text-sm text-text-secondary">{item.subtitle}</p>
      </div>
      {item.onPlay ? (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            item.onPlay?.();
          }}
          className="mr-3 flex h-9 w-9 flex-shrink-0 translate-x-2 items-center justify-center rounded-full bg-brand text-black opacity-0 transition-all group-hover:translate-x-0 group-hover:opacity-100 hover:scale-105 focus-visible:translate-x-0 focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/70 focus-visible:ring-offset-2 focus-visible:ring-offset-surface-2"
          aria-label={`Play ${item.title}`}
        >
          <Play size={16} className="ml-0.5 fill-current" aria-hidden="true" />
        </button>
      ) : null}
    </div>
  );
};
