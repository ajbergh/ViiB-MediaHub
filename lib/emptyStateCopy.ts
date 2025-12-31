/**
 * ViiB MediaHub - Empty State Copy Constants
 * 
 * Single source of truth for all empty state content across the app.
 * This ensures consistency in messaging and makes copy changes easy.
 * 
 * Usage:
 * Import EMPTY_STATE and access by key (e.g., EMPTY_STATE.likedSongs)
 * Pass to EmptyState component from components/EmptyState.tsx
 * 
 * Structure:
 * - icon: Lucide icon component to display
 * - title: Main heading (bold, attention-grabbing)
 * - description: Supportive text explaining the state
 * - primaryAction: Main CTA button label (optional)
 * - secondaryAction: Alternative action label (optional)
 * 
 * Available Keys:
 * - library: No songs in library
 * - albums: No albums
 * - artists: No artists  
 * - playlists: No playlists
 * - queue: Queue is empty
 * - search: No search results
 * - likedSongs: No liked songs
 * - likedAlbums: No liked albums
 * - smartMix: No smart mix available
 * - recentlyPlayed: Nothing played yet
 * - stats: Not enough listening data
 * - scanning: Library scan in progress
 * 
 * @module emptyStateCopy
 */

import { Music, Disc, Users, ListMusic, Search, Heart, Sparkles, History, BarChart3, List, RefreshCw } from 'lucide-react';
import type { LucideIcon } from 'lucide-react';

export interface EmptyStateCopy {
  icon: LucideIcon;
  title: string;
  description: string;
  primaryAction?: string;
  secondaryAction?: string;
}

export const EMPTY_STATE: Record<string, EmptyStateCopy> = {
  library: {
    icon: Music,
    title: 'Your library is empty',
    description: 'Add some music to get started. You can scan a folder or download from Spotify.',
    primaryAction: 'Add Music',
    secondaryAction: 'Open Settings',
  },
  albums: {
    icon: Disc,
    title: 'No albums yet',
    description: 'Albums will appear here once you add music to your library.',
  },
  artists: {
    icon: Users,
    title: 'No artists yet',
    description: 'Artists will appear here once you add music to your library.',
  },
  playlists: {
    icon: ListMusic,
    title: 'No playlists yet',
    description: 'Create your first playlist to organize your favorite tracks.',
    primaryAction: 'Create Playlist',
  },
  queue: {
    icon: List,
    title: 'Your queue is empty',
    description: 'Add songs to your queue to line up what plays next.',
    primaryAction: 'Browse Library',
    secondaryAction: 'Start a Smart Mix',
  },
  search: {
    icon: Search,
    title: 'No results found',
    description: 'Try a different search or browse by artist, album, or genre.',
    primaryAction: 'Clear Search',
    secondaryAction: 'Browse Genres',
  },
  likedSongs: {
    icon: Heart,
    title: 'Nothing liked yet',
    description: "Tap the heart on songs you love so they're always close.",
    primaryAction: 'Explore Songs',
    secondaryAction: 'Play a Smart Mix',
  },
  likedAlbums: {
    icon: Heart,
    title: 'No liked albums yet',
    description: "Save albums you revisit so they're one click away.",
    primaryAction: 'Browse Albums',
    secondaryAction: 'Explore Artists',
  },
  smartMix: {
    icon: Sparkles,
    title: 'No mix yet',
    description: 'This mix needs some listening history to shape itself to you.',
    primaryAction: 'Play Something',
    secondaryAction: 'Import Music',
  },
  recentlyPlayed: {
    icon: History,
    title: 'Nothing played yet',
    description: 'Start listening and ViiB will keep your recent rotation here.',
    primaryAction: 'Play Something',
    secondaryAction: 'Start a Smart Mix',
  },
  stats: {
    icon: BarChart3,
    title: 'Not enough data yet',
    description: 'Listen a bit more and your stats will start to take shape.',
    primaryAction: 'Play Something',
    secondaryAction: 'View Library',
  },
  scanning: {
    icon: RefreshCw,
    title: 'Scanning your library',
    description: 'ViiB is checking your files and updating your collection.',
    primaryAction: 'View Progress',
    secondaryAction: 'Continue Browsing',
  },
} as const;
