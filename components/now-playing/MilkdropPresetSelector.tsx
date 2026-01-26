/**
 * ViiB MediaHub - Milkdrop Preset Selector Component
 * 
 * Modal/panel for browsing and selecting Milkdrop presets.
 * 
 * Features:
 * - Search/filter presets by name
 * - Favorite presets section at top
 * - Alphabetically sorted preset list
 * - Current preset highlighted
 * - Click to select and apply preset
 * 
 * @module MilkdropPresetSelector
 */

import React, { useState, useMemo } from 'react';
import { X, Search, Star, Shuffle } from 'lucide-react';
import { Button } from '../ui/Button';

interface MilkdropPresetSelectorProps {
  /** All available preset keys */
  presets: string[];
  /** Currently active preset key */
  currentPreset: string | null;
  /** User's favorite preset keys */
  favorites: string[];
  /** Called when a preset is selected */
  onSelect: (preset: string) => void;
  /** Called when favorite status is toggled */
  onToggleFavorite: (preset: string) => void;
  /** Called to close the selector */
  onClose: () => void;
  /** Optional class names */
  className?: string;
}

/**
 * MilkdropPresetSelector - Browse and select Milkdrop presets
 */
export const MilkdropPresetSelector: React.FC<MilkdropPresetSelectorProps> = ({
  presets,
  currentPreset,
  favorites,
  onSelect,
  onToggleFavorite,
  onClose,
  className = ''
}) => {
  const [searchQuery, setSearchQuery] = useState('');
  
  // Filter and organize presets
  const { favoritePresets, filteredPresets } = useMemo(() => {
    const query = searchQuery.toLowerCase().trim();
    
    // Filter presets by search query
    const filtered = query
      ? presets.filter(p => p.toLowerCase().includes(query))
      : presets;
    
    // Separate favorites from the filtered list
    const favs = filtered.filter(p => favorites.includes(p));
    const nonFavs = filtered.filter(p => !favorites.includes(p));
    
    // Sort both lists alphabetically
    favs.sort((a, b) => a.localeCompare(b));
    nonFavs.sort((a, b) => a.localeCompare(b));
    
    return {
      favoritePresets: favs,
      filteredPresets: nonFavs
    };
  }, [presets, favorites, searchQuery]);
  
  // Select a random preset
  const selectRandom = () => {
    if (presets.length === 0) return;
    const randomIndex = Math.floor(Math.random() * presets.length);
    onSelect(presets[randomIndex]);
  };
  
  // Handle preset click
  const handleSelect = (preset: string) => {
    onSelect(preset);
    onClose();
  };
  
  return (
    <div className={`fixed inset-0 z-[200] flex items-center justify-center bg-black/60 backdrop-blur-sm ${className}`}>
      <div className="bg-surface-1 rounded-xl shadow-2xl w-full max-w-lg max-h-[80vh] flex flex-col overflow-hidden animate-in fade-in zoom-in-95 duration-200">
        {/* Header */}
        <div className="flex items-center justify-between p-4 border-b border-surface-3">
          <h2 className="text-lg font-semibold text-text-main">
            Milkdrop Presets
          </h2>
          <div className="flex items-center gap-2">
            <Button
              variant="ghost"
              onClick={selectRandom}
              className="p-1.5 text-text-secondary hover:text-text-main"
              title="Random preset"
            >
              <Shuffle size={18} />
            </Button>
            <Button
              variant="ghost"
              onClick={onClose}
              className="p-1.5 text-text-secondary hover:text-text-main"
            >
              <X size={20} />
            </Button>
          </div>
        </div>
        
        {/* Search */}
        <div className="p-3 border-b border-surface-3">
          <div className="relative">
            <Search 
              size={18} 
              className="absolute left-3 top-1/2 -translate-y-1/2 text-text-subtle" 
            />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Search presets..."
              className="w-full pl-10 pr-4 py-2 bg-surface-2 border border-surface-3 rounded-lg text-text-main placeholder:text-text-subtle focus:outline-none focus:ring-2 focus:ring-brand/50 focus:border-brand"
              autoFocus
            />
          </div>
        </div>
        
        {/* Preset list */}
        <div className="flex-1 overflow-y-auto p-2">
          {/* Favorites section */}
          {favoritePresets.length > 0 && (
            <div className="mb-4">
              <div className="flex items-center gap-2 px-2 py-1 text-xs font-medium text-text-secondary uppercase tracking-wider">
                <Star size={12} className="text-accent-yellow fill-current" />
                Favorites ({favoritePresets.length})
              </div>
              <div className="space-y-0.5">
                {favoritePresets.map((preset) => (
                  <PresetItem
                    key={preset}
                    preset={preset}
                    isActive={preset === currentPreset}
                    isFavorite={true}
                    onSelect={handleSelect}
                    onToggleFavorite={onToggleFavorite}
                  />
                ))}
              </div>
            </div>
          )}
          
          {/* All presets section */}
          <div>
            {favoritePresets.length > 0 && filteredPresets.length > 0 && (
              <div className="px-2 py-1 text-xs font-medium text-text-secondary uppercase tracking-wider">
                All Presets ({filteredPresets.length})
              </div>
            )}
            <div className="space-y-0.5">
              {filteredPresets.map((preset) => (
                <PresetItem
                  key={preset}
                  preset={preset}
                  isActive={preset === currentPreset}
                  isFavorite={false}
                  onSelect={handleSelect}
                  onToggleFavorite={onToggleFavorite}
                />
              ))}
            </div>
          </div>
          
          {/* Empty state */}
          {favoritePresets.length === 0 && filteredPresets.length === 0 && (
            <div className="flex flex-col items-center justify-center py-12 text-text-subtle">
              <Search size={32} className="mb-2 opacity-50" />
              <p className="text-sm">No presets found</p>
              {searchQuery && (
                <button
                  onClick={() => setSearchQuery('')}
                  className="mt-2 text-xs text-brand hover:underline"
                >
                  Clear search
                </button>
              )}
            </div>
          )}
        </div>
        
        {/* Footer with count */}
        <div className="px-4 py-2 border-t border-surface-3 text-xs text-text-subtle">
          {presets.length} presets available
          {currentPreset && (
            <span className="ml-2">
              • Current: <span className="text-text-secondary">{formatPresetName(currentPreset)}</span>
            </span>
          )}
        </div>
      </div>
    </div>
  );
};

/**
 * Individual preset item in the list
 */
interface PresetItemProps {
  preset: string;
  isActive: boolean;
  isFavorite: boolean;
  onSelect: (preset: string) => void;
  onToggleFavorite: (preset: string) => void;
}

const PresetItem: React.FC<PresetItemProps> = ({
  preset,
  isActive,
  isFavorite,
  onSelect,
  onToggleFavorite
}) => {
  return (
    <div
      className={`
        group flex items-center gap-2 px-3 py-2 rounded-lg cursor-pointer
        transition-colors duration-150
        ${isActive 
          ? 'bg-brand/20 text-text-main' 
          : 'hover:bg-surface-2 text-text-secondary hover:text-text-main'
        }
      `}
      onClick={() => onSelect(preset)}
    >
      <span className="flex-1 truncate text-sm">
        {formatPresetName(preset)}
      </span>
      
      {isActive && (
        <span className="text-xs font-medium text-brand">Playing</span>
      )}
      
      <button
        onClick={(e) => {
          e.stopPropagation();
          onToggleFavorite(preset);
        }}
        className={`
          p-1 rounded opacity-0 group-hover:opacity-100 transition-opacity
          ${isFavorite 
            ? 'text-accent-yellow opacity-100' 
            : 'text-text-subtle hover:text-accent-yellow'
          }
        `}
        title={isFavorite ? 'Remove from favorites' : 'Add to favorites'}
      >
        <Star 
          size={14} 
          className={isFavorite ? 'fill-current' : ''} 
        />
      </button>
    </div>
  );
};

/**
 * Format preset name for display
 * Removes file extension, replaces underscores, etc.
 */
function formatPresetName(preset: string): string {
  return preset
    .replace(/\.milk$/i, '')
    .replace(/_/g, ' ')
    .replace(/ - /g, ' – ');
}

export default MilkdropPresetSelector;
