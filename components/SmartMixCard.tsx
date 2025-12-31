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
      className="flex-shrink-0 w-80 bg-surface-2 rounded-xl overflow-hidden group cursor-pointer border border-transparent hover:border-surface-border transition-all relative snap-start"
      onClick={onClick}
      onContextMenu={onContextMenu}
    >
      {/* Gradient Header with Title and Play Button */}
      <div
        className="h-40 p-6 flex flex-col justify-end relative"
        style={{
          background: `linear-gradient(135deg, ${mix.coverColors[0]}, ${mix.coverColors[1]})`,
        }}
      >
        <h3 className="text-2xl font-bold text-white shadow-black drop-shadow-md">
          {mix.name}
        </h3>
        <p className="text-white/80 text-sm font-medium drop-shadow">
          {mix.songIds.length} tracks
        </p>

        {/* Play Button - Appears on Hover */}
        <button
          onClick={handlePlayClick}
          className="absolute bottom-4 right-4 w-12 h-12 bg-brand text-black rounded-full flex items-center justify-center opacity-0 group-hover:opacity-100 transform translate-y-4 group-hover:translate-y-0 transition-all shadow-xl hover:scale-105 hover:bg-brand-hover"
          aria-label={`Play ${mix.name}`}
        >
          <Play size={24} className="fill-current ml-1" />
        </button>
      </div>

      {/* Description Section */}
      <div className="p-4">
        <p className="text-text-secondary text-sm line-clamp-2">
          {mix.description}
        </p>
      </div>
    </div>
  );
};
