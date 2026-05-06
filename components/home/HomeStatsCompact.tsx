import React from 'react';
import { Disc3, Library, Music, UsersRound } from 'lucide-react';
import { useNavigate } from 'react-router-dom';

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
    <section className="mb-10">
      <div className="mb-4 flex items-center gap-2 text-text-secondary">
        <Library size={18} aria-hidden="true" />
        <h2 className="text-card font-semibold text-text-main">Library Snapshot</h2>
      </div>
      <div className="grid gap-3 sm:grid-cols-3">
        {stats.map((stat) => {
          const Icon = stat.icon;
          return (
            <button
              key={stat.label}
              type="button"
              onClick={() => navigate(stat.path)}
              className="flex items-center gap-3 rounded-lg bg-surface-1 px-4 py-3 text-left ring-1 ring-surface-3 transition-colors hover:bg-surface-2 hover:ring-surface-border focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/70"
            >
              <span className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg bg-surface-2 text-brand">
                <Icon size={20} aria-hidden="true" />
              </span>
              <span className="min-w-0">
                <span className="block text-lg font-semibold text-text-main">{stat.value.toLocaleString()}</span>
                <span className="block truncate text-sm text-text-secondary">{stat.label}</span>
              </span>
            </button>
          );
        })}
      </div>
    </section>
  );
};
