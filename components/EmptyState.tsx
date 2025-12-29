/**
 * ViiB MediaHub - Empty State Components
 * 
 * Consistent empty state designs for pages with no content. Uses composition
 * pattern with a generic EmptyState base and specialized variants for common cases.
 * All specialized components now use centralized copy from lib/emptyStateCopy.ts.
 * 
 * Architecture:
 * - EmptyState: Generic base component with icon, title, description, and optional actions
 * - Specialized components: Pre-configured variants using centralized copy constants
 * - Copy constants: All copy sourced from lib/emptyStateCopy.ts for consistency
 * 
 * Components:
 * - EmptyState: Generic empty state with icon, title, description, primary + secondary actions
 * - EmptyLibrary: No songs in library (with optional "Add Music" + "Open Settings" actions)
 * - EmptyPlaylists: No playlists created (with optional "Create Playlist" action)
 * - EmptySearchResults: No search results (with query context and optional "Clear" action)
 * - EmptyQueue: Queue is empty (compact variant for sidebar queue panel)
 * - EmptyAlbums: No albums in library
 * - EmptyArtists: No artists in library
 * - EmptySmartMixes: No smart mixes available
 * 
 * Usage:
 * - Use specialized components when available for consistent copy
 * - Use generic EmptyState with copy from lib/emptyStateCopy.ts for custom states
 * - primaryAction: Main CTA button (brand-colored)
 * - secondaryAction: Alternative action (text-only, subtle)
 * 
 * @module EmptyState
 */

import React from 'react';
import { Music, ListMusic, Search, List, Disc, Users, Sparkles } from 'lucide-react';
import { EMPTY_STATE } from '../lib/emptyStateCopy';

/**
 * Action button configuration for empty states.
 * Both primary and secondary actions use this interface.
 */
export interface EmptyStateAction {
  label: string;
  onClick: () => void;
}

interface EmptyStateProps {
  icon: React.ReactNode;
  title: string;
  description: string;
  action?: EmptyStateAction;
  secondaryAction?: EmptyStateAction;
}

/**
 * Generic empty state component
 */
export const EmptyState: React.FC<EmptyStateProps> = ({ icon, title, description, action, secondaryAction }) => (
  <div className="flex flex-col items-center justify-center py-20 px-4 text-center animate-in fade-in duration-500 motion-reduce:animate-none motion-reduce:transition-none">
    <div className="w-24 h-24 rounded-full bg-surface-2 flex items-center justify-center mb-6">
      <div className="text-text-subtle">
        {icon}
      </div>
    </div>
    <h3 className="text-xl font-bold text-text-main mb-2">{title}</h3>
    <p className="text-text-secondary max-w-sm mb-6">{description}</p>
    {(action || secondaryAction) && (
      <div className="flex items-center gap-3">
        {action && (
          <button
            onClick={action.onClick}
            className="bg-brand hover:bg-brand-hover text-black font-bold py-3 px-8 rounded-full transition-all duration-200 hover:scale-105 motion-reduce:transition-none motion-reduce:hover:transform-none"
          >
            {action.label}
          </button>
        )}
        {secondaryAction && (
          <button
            onClick={secondaryAction.onClick}
            className="text-text-secondary hover:text-text-main font-medium py-3 px-6 rounded-full transition-colors duration-200 hover:bg-surface-2"
          >
            {secondaryAction.label}
          </button>
        )}
      </div>
    )}
  </div>
);

/**
 * Empty library state - uses centralized copy from emptyStateCopy.ts
 */
export const EmptyLibrary: React.FC<{ onAddMusic?: () => void; onOpenSettings?: () => void }> = ({ 
  onAddMusic, 
  onOpenSettings 
}) => {
  const copy = EMPTY_STATE.library;
  return (
    <EmptyState
      icon={<copy.icon size={48} />}
      title={copy.title}
      description={copy.description}
      action={onAddMusic && copy.primaryAction ? { label: copy.primaryAction, onClick: onAddMusic } : undefined}
      secondaryAction={onOpenSettings && copy.secondaryAction ? { label: copy.secondaryAction, onClick: onOpenSettings } : undefined}
    />
  );
};

/**
 * Empty playlists state - uses centralized copy from emptyStateCopy.ts
 */
export const EmptyPlaylists: React.FC<{ onCreate?: () => void }> = ({ onCreate }) => {
  const copy = EMPTY_STATE.playlists;
  return (
    <EmptyState
      icon={<copy.icon size={48} />}
      title={copy.title}
      description={copy.description}
      action={onCreate && copy.primaryAction ? { label: copy.primaryAction, onClick: onCreate } : undefined}
    />
  );
};

/**
 * Empty search results state - uses centralized copy from emptyStateCopy.ts
 * The description can be customized with the search query for context.
 */
export const EmptySearchResults: React.FC<{ query?: string; onClear?: () => void; onBrowseGenres?: () => void }> = ({ 
  query,
  onClear,
  onBrowseGenres
}) => {
  const copy = EMPTY_STATE.search;
  const description = query 
    ? `We couldn't find anything matching "${query}". ${copy.description}` 
    : copy.description;
  
  return (
    <EmptyState
      icon={<copy.icon size={48} />}
      title={copy.title}
      description={description}
      action={onClear && copy.primaryAction ? { label: copy.primaryAction, onClick: onClear } : undefined}
      secondaryAction={onBrowseGenres && copy.secondaryAction ? { label: copy.secondaryAction, onClick: onBrowseGenres } : undefined}
    />
  );
};

/**
 * Empty queue state - compact variant for sidebar queue panel.
 * Uses centralized copy from emptyStateCopy.ts
 */
export const EmptyQueue: React.FC = () => {
  const copy = EMPTY_STATE.queue;
  return (
    <div className="flex flex-col items-center justify-center h-full py-12 px-4 text-center">
      <copy.icon size={40} className="text-text-subtle mb-4" />
      <h4 className="text-lg font-bold text-text-main mb-1">{copy.title}</h4>
      <p className="text-text-secondary text-sm">{copy.description}</p>
    </div>
  );
};

/**
 * Empty albums state - uses centralized copy from emptyStateCopy.ts
 */
export const EmptyAlbums: React.FC = () => {
  const copy = EMPTY_STATE.albums;
  return (
    <EmptyState
      icon={<copy.icon size={48} />}
      title={copy.title}
      description={copy.description}
    />
  );
};

/**
 * Empty artists state - uses centralized copy from emptyStateCopy.ts
 */
export const EmptyArtists: React.FC = () => {
  const copy = EMPTY_STATE.artists;
  return (
    <EmptyState
      icon={<copy.icon size={48} />}
      title={copy.title}
      description={copy.description}
    />
  );
};

/**
 * Empty smart mixes state - uses centralized copy from emptyStateCopy.ts
 */
export const EmptySmartMixes: React.FC = () => {
  const copy = EMPTY_STATE.smartMix;
  return (
    <EmptyState
      icon={<copy.icon size={48} />}
      title={copy.title}
      description={copy.description}
    />
  );
};
