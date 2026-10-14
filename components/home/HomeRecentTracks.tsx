/** Displays recent tracks and their supplied playback actions on Home. */

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
    <section className="mb-8">
      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-section font-bold tracking-tight text-text-main">Recently Played</h2>
      </div>

      <div className="space-y-0.5">
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
            className="group flex cursor-pointer items-center gap-3.5 rounded-md px-3 py-2 transition-colors hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/70"
          >
            <div
              className="relative h-10 w-10 flex-shrink-0 overflow-hidden rounded bg-surface-3"
              style={{ background: coverBackground(song.coverUrl, song.album) }}
            >
              <span className="absolute inset-0 flex items-center justify-center bg-surface-0/50 opacity-0 transition-opacity group-hover:opacity-100">
                <Play size={16} className="fill-current text-text-main" aria-hidden="true" />
              </span>
            </div>
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-medium text-text-main group-hover:text-brand">{song.title}</p>
              <p className="truncate text-xs text-text-secondary">{song.artist}</p>
            </div>
            <div className="hidden text-xs tabular-nums text-text-secondary md:block">{formatTime(song.duration)}</div>
            {song.lastPlayed ? (
              <div className="min-w-[76px] text-right text-xs text-text-subtle">
                {formatRelativeTime(song.lastPlayed)}
              </div>
            ) : null}
          </div>
        ))}
      </div>
    </section>
  );
};
