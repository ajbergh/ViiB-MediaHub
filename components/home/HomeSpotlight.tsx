import React from 'react';
import { Disc3, Play, Sparkles, UserRound } from 'lucide-react';
import { coverBackground } from '../../utils';
import { cn } from '../ui/cn';
import { HomeSpotlightItem } from './useHomeContent';

interface HomeSpotlightProps {
  item: HomeSpotlightItem | null;
  className?: string;
  compact?: boolean;
}

const getKindIcon = (kind: HomeSpotlightItem['kind']) => {
  if (kind === 'artist') return UserRound;
  if (kind === 'smartMix') return Sparkles;
  return Disc3;
};

export const HomeSpotlight: React.FC<HomeSpotlightProps> = ({ item, className, compact = false }) => {
  if (!item) {
    return (
      <div className={cn('rounded-xl bg-surface-2 p-6 ring-1 ring-surface-3', className)}>
        <p className="text-sm font-semibold uppercase tracking-wide text-brand">Welcome Home</p>
        <h1 className="mt-3 text-display text-text-main">Let's ViiB</h1>
        <p className="mt-2 max-w-xl text-text-secondary">
          Add music to your library to unlock albums, artists, Smart Mixes, and personalized listening rows.
        </p>
      </div>
    );
  }

  const Icon = getKindIcon(item.kind);
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
        'group relative grid cursor-pointer gap-6 overflow-hidden rounded-xl bg-surface-2 p-5 ring-1 ring-surface-3 transition-all hover:bg-surface-3 hover:ring-surface-border focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/70 md:grid-cols-[minmax(0,1fr)_220px]',
        compact ? 'min-h-[220px]' : 'min-h-[280px]',
        className
      )}
    >
      <div className="relative z-10 flex min-w-0 flex-col justify-end">
        <div className="mb-4 inline-flex w-fit items-center gap-2 rounded-full bg-surface-1/80 px-3 py-1 text-sm font-semibold text-text-secondary ring-1 ring-surface-3">
          <Icon size={16} className="text-brand" aria-hidden="true" />
          <span>{item.subtitle}</span>
        </div>
        <h1 className={cn('max-w-3xl truncate text-text-main', compact ? 'text-section font-semibold' : 'text-display')}>
          {item.title}
        </h1>
        <p className="mt-3 max-w-2xl text-text-secondary">{item.description}</p>
        <div className="mt-6 flex items-center gap-3">
          {item.onPlay ? (
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                item.onPlay?.();
              }}
              className="inline-flex h-11 items-center gap-2 rounded-full bg-brand px-5 font-semibold text-black transition-transform hover:scale-105 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/70 focus-visible:ring-offset-2 focus-visible:ring-offset-surface-2 motion-reduce:transition-none motion-reduce:hover:transform-none"
            >
              <Play size={18} className="fill-current" aria-hidden="true" />
              Play
            </button>
          ) : null}
          <span className="text-sm font-medium text-text-secondary group-hover:text-text-main">Open details</span>
        </div>
      </div>

      <div className="relative z-10 hidden items-end justify-end md:flex">
        <div
          className={cn(
            'aspect-square w-full max-w-[220px] overflow-hidden bg-surface-3 shadow-2xl',
            item.kind === 'artist' ? 'rounded-full' : 'rounded-lg'
          )}
          style={artworkStyle}
        >
          {!item.imageUrl && item.kind !== 'smartMix' ? (
            <span className="flex h-full w-full items-center justify-center text-white/40">
              <Icon size={64} aria-hidden="true" />
            </span>
          ) : null}
        </div>
      </div>
    </div>
  );
};
