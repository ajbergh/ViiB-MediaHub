import React from 'react';
import { Clock, Play } from 'lucide-react';
import { Song } from '../../types';
import { coverBackground, formatTime } from '../../utils';

interface HomeRecentTracksProps {
  tracks: Song[];
  formatRelativeTime: (timestamp: number) => string;
  onPlay: (song: Song) => void;
  onContextMenu: (e: React.MouseEvent, song: Song) => void;
  limit?: number;
}

export const HomeRecentTracks: React.FC<HomeRecentTracksProps> = ({
  tracks,
  formatRelativeTime,
  onPlay,
  onContextMenu,
  limit = 8,
}) => {
  if (tracks.length === 0) return null;

  return (
    <section className="mb-10">
      <div className="mb-4 flex items-center gap-2">
        <Clock size={20} className="text-accent-blue" aria-hidden="true" />
        <h2 className="text-section font-semibold text-text-main">Recently Played</h2>
      </div>

      <div className="overflow-hidden rounded-xl bg-surface-2 ring-1 ring-surface-3">
        {tracks.slice(0, limit).map((song, index) => (
          <div
            key={`${song.id}-${index}`}
            role="button"
            tabIndex={0}
            onClick={() => onPlay(song)}
            onContextMenu={(e) => onContextMenu(e, song)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                onPlay(song);
              }
            }}
            className={`group flex cursor-pointer items-center gap-4 px-4 py-3 transition-colors hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/70 ${
              index > 0 ? 'border-t border-surface-3' : ''
            }`}
          >
            <div
              className="relative h-12 w-12 flex-shrink-0 overflow-hidden rounded-md bg-surface-3"
              style={{ background: coverBackground(song.coverUrl, song.album) }}
            >
              <span className="absolute inset-0 flex items-center justify-center bg-black/40 opacity-0 transition-opacity group-hover:opacity-100">
                <Play size={18} className="fill-current text-text-main" aria-hidden="true" />
              </span>
            </div>
            <div className="min-w-0 flex-1">
              <p className="truncate font-medium text-text-main group-hover:text-brand">{song.title}</p>
              <p className="truncate text-sm text-text-secondary">{song.artist}</p>
            </div>
            <div className="hidden text-sm font-mono text-text-secondary md:block">{formatTime(song.duration)}</div>
            {song.lastPlayed ? (
              <div className="min-w-[84px] text-right text-xs text-text-subtle">
                {formatRelativeTime(song.lastPlayed)}
              </div>
            ) : null}
          </div>
        ))}
      </div>
    </section>
  );
};
