/**
 * ViiB MediaHub - Empty State Components
 * 
 * Consistent empty state designs for pages with no content.
 * 
 * Components:
 * - EmptyState: Generic empty state with icon, title, description, and optional action
 * - EmptyLibrary: No songs in library
 * - EmptyPlaylists: No playlists created
 * - EmptySearch: No search results
 * - EmptyQueue: Queue is empty
 * 
 * @module EmptyState
 */

import React from 'react';
import { Music, ListMusic, Search, List, Disc, Users, Sparkles } from 'lucide-react';

interface EmptyStateProps {
  icon: React.ReactNode;
  title: string;
  description: string;
  action?: {
    label: string;
    onClick: () => void;
  };
}

/**
 * Generic empty state component
 */
export const EmptyState: React.FC<EmptyStateProps> = ({ icon, title, description, action }) => (
  <div className="flex flex-col items-center justify-center py-20 px-4 text-center animate-in fade-in duration-500 motion-reduce:animate-none motion-reduce:transition-none">
    <div className="w-24 h-24 rounded-full bg-surface-2 flex items-center justify-center mb-6">
      <div className="text-text-subtle">
        {icon}
      </div>
    </div>
    <h3 className="text-xl font-bold text-text-main mb-2">{title}</h3>
    <p className="text-text-secondary max-w-sm mb-6">{description}</p>
    {action && (
      <button
        onClick={action.onClick}
        className="bg-brand hover:bg-brand-hover text-black font-bold py-3 px-8 rounded-full transition-all duration-200 hover:scale-105 motion-reduce:transition-none motion-reduce:hover:transform-none"
      >
        {action.label}
      </button>
    )}
  </div>
);

/**
 * Empty library state
 */
export const EmptyLibrary: React.FC<{ onAddMusic?: () => void }> = ({ onAddMusic }) => (
  <EmptyState
    icon={<Music size={48} />}
    title="Your library is empty"
    description="Add some music to get started. You can scan a folder or download from Spotify."
    action={onAddMusic ? { label: 'Add Music', onClick: onAddMusic } : undefined}
  />
);

/**
 * Empty playlists state
 */
export const EmptyPlaylists: React.FC<{ onCreate?: () => void }> = ({ onCreate }) => (
  <EmptyState
    icon={<ListMusic size={48} />}
    title="No playlists yet"
    description="Create your first playlist to organize your favorite tracks."
    action={onCreate ? { label: 'Create Playlist', onClick: onCreate } : undefined}
  />
);

/**
 * Empty search results state
 */
export const EmptySearchResults: React.FC<{ query?: string }> = ({ query }) => (
  <EmptyState
    icon={<Search size={48} />}
    title="No results found"
    description={query ? `We couldn't find anything matching "${query}". Try a different search.` : 'Try searching for songs, artists, or albums.'}
  />
);

/**
 * Empty queue state
 */
export const EmptyQueue: React.FC = () => (
  <div className="flex flex-col items-center justify-center py-12 px-4 text-center">
    <List size={40} className="text-text-subtle mb-4" />
    <h4 className="text-lg font-bold text-text-main mb-1">Queue is empty</h4>
    <p className="text-text-secondary text-sm">Add songs to your queue to play them next.</p>
  </div>
);

/**
 * Empty albums state
 */
export const EmptyAlbums: React.FC = () => (
  <EmptyState
    icon={<Disc size={48} />}
    title="No albums yet"
    description="Albums will appear here once you add music to your library."
  />
);

/**
 * Empty artists state
 */
export const EmptyArtists: React.FC = () => (
  <EmptyState
    icon={<Users size={48} />}
    title="No artists yet"
    description="Artists will appear here once you add music to your library."
  />
);

/**
 * Empty smart mixes state
 */
export const EmptySmartMixes: React.FC = () => (
  <EmptyState
    icon={<Sparkles size={48} />}
    title="No smart mixes available"
    description="Listen to more music to generate personalized mixes based on your taste."
  />
);
