/**
 * ViiB MediaHub - Skeleton Components
 * 
 * Reusable skeleton loaders for loading states with shimmer animation.
 * 
 * Components:
 * - Skeleton: Base skeleton element with shimmer
 * - SkeletonAlbumCard: Album/playlist card placeholder
 * - SkeletonTrackRow: Track list row placeholder
 * - SkeletonAlbumGrid: Grid of album cards
 * - SkeletonTrackList: List of track rows
 * 
 * @module Skeleton
 */

import React from 'react';

interface SkeletonProps {
  className?: string;
}

/**
 * Base skeleton element with shimmer animation
 */
export const Skeleton: React.FC<SkeletonProps> = ({ className = '' }) => (
  <div 
    className={`bg-surface-3 rounded animate-pulse relative overflow-hidden ${className}`}
  >
    <div className="absolute inset-0 -translate-x-full animate-[shimmer_2s_infinite] bg-gradient-to-r from-transparent via-white/5 to-transparent" />
  </div>
);

/**
 * Album/playlist card skeleton
 */
export const SkeletonAlbumCard: React.FC = () => (
  <div className="bg-surface-2 p-4 rounded-lg">
    <Skeleton className="aspect-square mb-4 rounded-md" />
    <Skeleton className="h-4 w-3/4 mb-2" />
    <Skeleton className="h-3 w-1/2" />
  </div>
);

/**
 * Track row skeleton for song lists
 */
export const SkeletonTrackRow: React.FC = () => (
  <div className="flex items-center gap-4 px-4 py-3 bg-surface-1">
    <Skeleton className="w-8 h-4" />
    <Skeleton className="w-10 h-10 rounded" />
    <div className="flex-1 min-w-0">
      <Skeleton className="h-4 w-48 mb-2" />
      <Skeleton className="h-3 w-32" />
    </div>
    <Skeleton className="h-3 w-24 hidden md:block" />
    <Skeleton className="h-3 w-12" />
  </div>
);

/**
 * Grid of album card skeletons
 */
export const SkeletonAlbumGrid: React.FC<{ count?: number }> = ({ count = 10 }) => (
  <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-4">
    {Array.from({ length: count }).map((_, i) => (
      <SkeletonAlbumCard key={i} />
    ))}
  </div>
);

/**
 * List of track row skeletons
 */
export const SkeletonTrackList: React.FC<{ count?: number }> = ({ count = 8 }) => (
  <div className="space-y-0.5">
    {Array.from({ length: count }).map((_, i) => (
      <SkeletonTrackRow key={i} />
    ))}
  </div>
);

/**
 * Detail page header skeleton (album/artist/playlist detail)
 */
export const SkeletonDetailHeader: React.FC = () => (
  <div className="p-8 pt-16 flex flex-col md:flex-row gap-8 items-end">
    <Skeleton className="w-52 h-52 rounded-lg flex-shrink-0" />
    <div className="flex-1 w-full">
      <Skeleton className="h-3 w-16 mb-4" />
      <Skeleton className="h-12 w-3/4 mb-4" />
      <Skeleton className="h-4 w-1/2 mb-2" />
      <Skeleton className="h-4 w-1/3" />
    </div>
  </div>
);

/**
 * Full page loading skeleton for detail pages
 */
export const SkeletonDetailPage: React.FC = () => (
  <div className="animate-in fade-in duration-300">
    <SkeletonDetailHeader />
    <div className="px-8 pt-6">
      <div className="flex gap-4 mb-8">
        <Skeleton className="w-14 h-14 rounded-full" />
        <Skeleton className="w-24 h-10 rounded-full" />
        <Skeleton className="w-24 h-10 rounded-full" />
      </div>
      <SkeletonTrackList count={10} />
    </div>
  </div>
);
