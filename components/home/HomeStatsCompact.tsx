/** Displays compact library and listening statistics on Home. */

import React from 'react';
import { Disc3, Library, Music, UsersRound } from 'lucide-react';
import { useNavigate } from 'react-router';

interface HomeStatsCompactProps {
  songsCount: number;
  albumsCount: number;
  artistsCount: number;
}

export const HomeStatsCompact: React.FC<HomeStatsCompactProps> = ({
  songsCount,
  albumsCount,
  artistsCount,
}) => {
  const navigate = useNavigate();
  const stats = [
    { label: 'Songs', value: songsCount, icon: Music, path: '/songs' },
    { label: 'Albums', value: albumsCount, icon: Disc3, path: '/albums' },
    { label: 'Artists', value: artistsCount, icon: UsersRound, path: '/artists' },
  ];

  return (
    <section className="mb-6">
      <h2 className="mb-3 text-card font-bold tracking-tight text-text-main">Library Overview</h2>
      <div className="grid gap-3 sm:grid-cols-3">
        {stats.map((stat) => {
          const Icon = stat.icon;
          return (
            <button
              key={stat.label}
              type="button"
              onClick={() => navigate(stat.path)}
              className="flex items-center gap-3.5 rounded-lg bg-surface-2/80 px-4 py-3.5 text-left transition-colors hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/70"
            >
              <span className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-md bg-surface-3 text-text-secondary">
                <Icon size={18} aria-hidden="true" />
              </span>
              <span className="min-w-0">
                <span className="block text-lg font-bold tabular-nums text-text-main">{stat.value.toLocaleString()}</span>
                <span className="block truncate text-xs text-text-secondary">{stat.label}</span>
              </span>
            </button>
          );
        })}
      </div>
    </section>
  );
};
