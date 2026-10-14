/**
 * ViiB MediaHub - Smart Mix Card Component
 * 
 * A specialized card component for displaying Smart Mix playlists with
 * gradient backgrounds derived from the mix's cover colors.
 * 
 * Features:
 * - Gradient background from mix.coverColors (2-color linear gradient)
 * - Hover-reveal play button with smooth animation
 * - Consistent sizing and layout for horizontal scroll carousels
 * - Description section below gradient header
 * - Context menu support for additional actions
 * 
 * Usage:
 * - Home page Smart Mixes horizontal scroll
 * - Smart Playlists page mix grid (if needed)
 * 
 * Props:
 * - mix: SmartMix object with id, name, description, songIds, coverColors
 * - onPlay: Callback when play button is clicked
 * - onClick: Callback when card is clicked (navigate to detail)
 * - onContextMenu: Callback for right-click context menu
 * 
 * @module SmartMixCard
 */

import React from 'react';
import { Play } from 'lucide-react';
import { SmartMix } from '../types';

interface SmartMixCardProps {
  mix: SmartMix;
  onPlay: () => void;
  onClick: () => void;
  onContextMenu: (e: React.MouseEvent) => void;
}

/**
 * SmartMixCard - Gradient card component for Smart Mix playlists.
 * 
 * The gradient is constructed from mix.coverColors[0] and mix.coverColors[1],
 * creating a 135-degree diagonal gradient for visual appeal.
 */
export const SmartMixCard: React.FC<SmartMixCardProps> = ({
  mix,
  onPlay,
  onClick,
  onContextMenu,
}) => {
  const handlePlayClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    onPlay();
  };

  return (
    <div
      className="group relative w-44 flex-shrink-0 cursor-pointer snap-start rounded-lg bg-transparent p-3 transition-colors hover:bg-surface-hover"
      onClick={onClick}
      onContextMenu={onContextMenu}
    >
      {/* Square Cover Artwork with Spotify Mix Badge and Play Button */}
      <div
        className="relative mb-3 flex aspect-square w-full flex-col justify-between overflow-hidden rounded-md p-3.5 shadow-xl"
        style={{
          background: `linear-gradient(145deg, ${mix.coverColors[0]}, ${mix.coverColors[1]})`,
        }}
      >
        <span className="w-fit rounded bg-surface-0/40 px-2 py-0.5 text-[10px] font-bold uppercase tracking-widest text-text-main backdrop-blur-sm">
          Mix • {mix.songIds.length}
        </span>

        <h3 className="line-clamp-2 text-lg font-extrabold leading-tight tracking-tight text-text-main drop-shadow">
          {mix.name}
        </h3>

        {/* Play Button - Appears on Hover */}
        <button
          type="button"
          onClick={handlePlayClick}
          className="absolute bottom-2 right-2 flex h-11 w-11 translate-y-2 items-center justify-center rounded-full bg-brand text-surface-0 opacity-0 shadow-xl transition-all group-hover:translate-y-0 group-hover:opacity-100 hover:scale-105 hover:bg-brand-hover"
          aria-label={`Play ${mix.name}`}
        >
          <Play size={20} className="ml-0.5 fill-current" />
        </button>
      </div>

      {/* Title & Description */}
      <h4 className="truncate text-sm font-semibold text-text-main">{mix.name}</h4>
      <p className="mt-1 line-clamp-2 text-xs text-text-secondary">
        {mix.description}
      </p>
    </div>
  );
};
