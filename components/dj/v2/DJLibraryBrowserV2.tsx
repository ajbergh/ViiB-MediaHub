/**
 * ViiB MediaHub - DJ Library Browser V2 Component
 * 
 * Enhanced bottom panel library browser with:
 * - Collapsible playlist/genre sidebar
 * - Enhanced track table with more columns
 * - Visual styling matching PCDJ DEX reference
 * - Load-to-deck indicators
 * - Drag-to-deck support
 * 
 * @module components/dj/v2/DJLibraryBrowserV2
 */

import React, { useState, useMemo, useCallback, useEffect, memo } from 'react';
import { useStore } from '../../../store';
import { useDJAudioEngineActions } from '../../../hooks/useDJAudioEngine';
import { getKeyCompatibility } from '../../../lib/keyDetection';
import type { DeckId } from '../../../slices/djMixerSlice';
import type { Song } from '../../../types';
import { 
  Search, 
  ChevronUp, 
  ChevronDown, 
  ChevronRight, 
  ChevronLeft,
  Folder, 
  FolderOpen,
  Music2,
  ListMusic,
  CloudDownload,
  Library
} from 'lucide-react';

type SortKey = 'title' | 'artist' | 'album' | 'duration' | 'bpm' | 'key' | 'genre';
type SortDirection = 'asc' | 'desc';

// Track color coding - persisted in session via Map
const TRACK_COLORS = ['#ef4444', '#f97316', '#eab308', '#22c55e', '#3b82f6', '#a855f7', '#ec4899', '#ffffff'] as const;
const trackColorMap = new Map<string, string>();

// Session cache for analyzed musical keys (populated when tracks are loaded into decks)
const analyzedKeyCache = new Map<string, string>();

interface PlaylistCategory {
  id: string;
  label: string;
  icon: React.ReactNode;
  expanded?: boolean;
  children?: { id: string; label: string; count?: number }[];
}

// Memoized table row to prevent unnecessary re-renders
const TrackRow = memo(({ 
  song, 
  index,
  loadedDeck, 
  onLoadToDeck,
  trackColor,
  onSetTrackColor,
  songKey,
  keyCompatibility,
}: {
  song: Song;
  index: number;
  loadedDeck: 'A' | 'B' | null;
  onLoadToDeck: (song: Song, deck: DeckId) => void;
  trackColor: string | undefined;
  onSetTrackColor: (songId: string, color: string | null) => void;
  songKey: string | null;           // Musical key from analysis cache
  keyCompatibility: number | null;  // 0-1 score, null if no key data
}) => {
  const [showColorPicker, setShowColorPicker] = React.useState(false);
  const deckIndicatorClass = loadedDeck === 'A' 
    ? 'bg-blue-600/15 border-l-2 border-blue-500 shadow-[inset_0_0_12px_rgba(59,130,246,0.08)]' 
    : loadedDeck === 'B' 
      ? 'bg-purple-600/15 border-l-2 border-purple-500 shadow-[inset_0_0_12px_rgba(139,92,246,0.08)]'
      : 'border-l-2 border-transparent';

  return (
    <tr 
      className={`
        hover:bg-white/5 transition-colors text-xs
        ${deckIndicatorClass}
      `}
      onDoubleClick={() => onLoadToDeck(song, 'A')}
    >
      {/* Color label */}
      <td className="px-1 py-1.5 w-5 relative">
        <button
          className="w-3 h-3 rounded-full border border-[#444] hover:border-neutral-300 transition-colors"
          style={{ backgroundColor: trackColor || '#333' }}
          onClick={(e) => { e.stopPropagation(); setShowColorPicker(!showColorPicker); }}
          title="Set track color label"
        />
        {showColorPicker && (
          <div className="absolute top-full left-0 z-50 bg-[#1a1a1a] border border-[#444] rounded p-1 flex gap-0.5 shadow-lg"
            onMouseLeave={() => setShowColorPicker(false)}>
            {TRACK_COLORS.map(c => (
              <button
                key={c}
                className={`w-4 h-4 rounded-full border transition-transform hover:scale-125 ${
                  trackColor === c ? 'border-white scale-110' : 'border-transparent'
                }`}
                style={{ backgroundColor: c }}
                onClick={(e) => { e.stopPropagation(); onSetTrackColor(song.id, c); setShowColorPicker(false); }}
              />
            ))}
            <button
              className="w-4 h-4 rounded-full border border-[#555] bg-[#333] text-[8px] text-neutral-400 flex items-center justify-center hover:border-white"
              onClick={(e) => { e.stopPropagation(); onSetTrackColor(song.id, null); setShowColorPicker(false); }}
              title="Remove color"
            >
              ✕
            </button>
          </div>
        )}
      </td>
      
      {/* Row number */}
      <td className="px-2 py-1.5 text-neutral-600 font-mono w-10 text-right">
        {index + 1}
      </td>
      
      {/* Load buttons */}
      <td className="px-2 py-1.5 w-20">
        <div className="flex gap-1">
          <button
            onClick={(e) => { e.stopPropagation(); onLoadToDeck(song, 'A'); }}
            className={`
              w-8 h-7 text-[11px] rounded font-bold transition-all
              ${loadedDeck === 'A' 
                ? 'bg-blue-600 text-white shadow-[0_0_8px_rgba(59,130,246,0.5)]' 
                : 'bg-surface-2 text-neutral-500 hover:bg-blue-600/50 hover:text-white'}
            `}
            title="Load to Deck A"
          >
            {loadedDeck === 'A' ? '▶A' : 'A'}
          </button>
          <button
            onClick={(e) => { e.stopPropagation(); onLoadToDeck(song, 'B'); }}
            className={`
              w-8 h-7 text-[11px] rounded font-bold transition-all
              ${loadedDeck === 'B' 
                ? 'bg-purple-600 text-white shadow-[0_0_8px_rgba(139,92,246,0.5)]' 
                : 'bg-surface-2 text-neutral-500 hover:bg-purple-600/50 hover:text-white'}
            `}
            title="Load to Deck B"
          >
            {loadedDeck === 'B' ? '▶B' : 'B'}
          </button>
        </div>
      </td>
      
      {/* Title */}
      <td className="px-2 py-1.5">
        <div className="flex items-center gap-1.5">
          {loadedDeck && (
            <span className={`text-[10px] ${loadedDeck === 'A' ? 'text-blue-400' : 'text-purple-400'}`}>
              ▶
            </span>
          )}
          <span className="text-neutral-100 truncate max-w-[180px]" title={song.title}>
            {song.title}
          </span>
        </div>
      </td>
      
      {/* Artist */}
      <td className="px-2 py-1.5">
        <span className="text-neutral-400 truncate max-w-[120px] block" title={song.artist}>
          {song.artist}
        </span>
      </td>
      
      {/* BPM */}
      <td className="px-2 py-1.5 w-12 text-right">
        <span className={`font-mono ${song.bpm ? 'text-green-400' : 'text-neutral-600'}`}>
          {song.bpm || '-'}
        </span>
      </td>
      
      {/* Key with harmonic compatibility */}
      <td className="px-2 py-1.5 w-14 text-center">
        {songKey ? (
          <span className={`font-mono text-[10px] px-1 py-0.5 rounded ${
            keyCompatibility === null ? 'text-emerald-400'
            : keyCompatibility >= 0.85 ? 'text-green-300 bg-green-500/20 font-bold'
            : keyCompatibility >= 0.7 ? 'text-yellow-300 bg-yellow-500/15'
            : keyCompatibility >= 0.5 ? 'text-orange-400 bg-orange-500/10'
            : 'text-neutral-600'
          }`}
            title={keyCompatibility !== null ? `Harmonic compatibility: ${Math.round(keyCompatibility * 100)}%` : undefined}
          >
            {songKey}
          </span>
        ) : (
          <span className="text-neutral-600">-</span>
        )}
      </td>
      
      {/* Album */}
      <td className="px-2 py-1.5 hidden xl:table-cell">
        <span className="text-neutral-500 truncate max-w-[120px] block" title={song.album}>
          {song.album}
        </span>
      </td>
      
      {/* Time */}
      <td className="px-2 py-1.5 w-14 text-right">
        <span className="font-mono text-neutral-400">
          {formatDuration(song.duration)}
        </span>
      </td>
      
      {/* Genre */}
      <td className="px-2 py-1.5 w-20 hidden lg:table-cell">
        <span className="text-neutral-500 truncate block text-[10px]">
          {song.genre?.[0] || '-'}
        </span>
      </td>
    </tr>
  );
});

TrackRow.displayName = 'TrackRow';

export const DJLibraryBrowserV2: React.FC = () => {
  const songs = useStore(state => state.songs);
  const playlists = useStore(state => state.playlists);
  const djDeckATrack = useStore(state => state.djDeckA.track);
  const djDeckBTrack = useStore(state => state.djDeckB.track);
  const djDeckAKey = useStore(state => state.djDeckA.key);
  const djDeckBKey = useStore(state => state.djDeckB.key);
  const djDeckAIsPlaying = useStore(state => state.djDeckA.isPlaying);
  const djDeckBIsPlaying = useStore(state => state.djDeckB.isPlaying);
  
  const { loadTrack } = useDJAudioEngineActions();

  const [searchQuery, setSearchQuery] = useState('');
  const [sortKey, setSortKey] = useState<SortKey>('title');
  const [sortDirection, setSortDirection] = useState<SortDirection>('asc');
  const [colorVersion, setColorVersion] = useState(0); // Force re-render on color change

  // Determine active deck key for harmonic compatibility
  // Prefer playing deck, fall back to whichever has a track loaded
  const activeKey = useMemo(() => {
    if (djDeckAIsPlaying && djDeckAKey) return djDeckAKey;
    if (djDeckBIsPlaying && djDeckBKey) return djDeckBKey;
    return djDeckAKey || djDeckBKey || null;
  }, [djDeckAKey, djDeckBKey, djDeckAIsPlaying, djDeckBIsPlaying]);

  // Compute key compatibility for a song
  const computeKeyCompat = useCallback((songKey: string | undefined | null): number | null => {
    if (!activeKey || !songKey) return null;
    return getKeyCompatibility(activeKey, songKey);
  }, [activeKey]);

  // Populate analyzed key cache when decks have tracks with detected keys
  const [keyCacheVersion, setKeyCacheVersion] = useState(0);
  useEffect(() => {
    let changed = false;
    if (djDeckATrack?.id && djDeckAKey && analyzedKeyCache.get(djDeckATrack.id) !== djDeckAKey) {
      analyzedKeyCache.set(djDeckATrack.id, djDeckAKey);
      changed = true;
    }
    if (djDeckBTrack?.id && djDeckBKey && analyzedKeyCache.get(djDeckBTrack.id) !== djDeckBKey) {
      analyzedKeyCache.set(djDeckBTrack.id, djDeckBKey);
      changed = true;
    }
    if (changed) setKeyCacheVersion(v => v + 1);
  }, [djDeckATrack?.id, djDeckAKey, djDeckBTrack?.id, djDeckBKey]);

  // Track color coding handlers
  const handleSetTrackColor = useCallback((songId: string, color: string | null) => {
    if (color) {
      trackColorMap.set(songId, color);
    } else {
      trackColorMap.delete(songId);
    }
    setColorVersion(v => v + 1); // Trigger re-render
  }, []);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [selectedCategory, setSelectedCategory] = useState<string>('all');
  const [expandedCategories, setExpandedCategories] = useState<Set<string>>(new Set(['playlists', 'genres']));

  // Generate genre list from songs
  const genres = useMemo(() => {
    const genreSet = new Set<string>();
    songs.forEach(song => {
      song.genre?.forEach(g => genreSet.add(g));
    });
    return Array.from(genreSet).sort();
  }, [songs]);

  // Categories for sidebar
  const categories: PlaylistCategory[] = useMemo(() => [
    {
      id: 'all',
      label: 'All Tracks',
      icon: <Library size={14} />,
    },
    {
      id: 'playlists',
      label: 'Playlists',
      icon: <ListMusic size={14} />,
      expanded: expandedCategories.has('playlists'),
      children: playlists.map(p => ({ id: `playlist-${p.id}`, label: p.name, count: p.songIds.length }))
    },
    {
      id: 'genres',
      label: 'Genres',
      icon: <Music2 size={14} />,
      expanded: expandedCategories.has('genres'),
      children: genres.map(g => ({ id: `genre-${g}`, label: g }))
    },
  ], [playlists, genres, expandedCategories]);

  // Filter songs based on selected category
  const categoryFilteredSongs = useMemo(() => {
    if (selectedCategory === 'all') return songs;
    
    if (selectedCategory.startsWith('playlist-')) {
      const playlistId = selectedCategory.replace('playlist-', '');
      const playlist = playlists.find(p => p.id === playlistId);
      if (!playlist) return [];
      return songs.filter(s => playlist.songIds.includes(s.id));
    }
    
    if (selectedCategory.startsWith('genre-')) {
      const genre = selectedCategory.replace('genre-', '');
      return songs.filter(s => s.genre?.includes(genre));
    }
    
    return songs;
  }, [songs, playlists, selectedCategory]);

  // Filter and sort songs
  const filteredSongs = useMemo(() => {
    let result = [...categoryFilteredSongs];

    // Filter by search query
    if (searchQuery) {
      const query = searchQuery.toLowerCase();
      result = result.filter(song => 
        song.title.toLowerCase().includes(query) ||
        song.artist.toLowerCase().includes(query) ||
        song.album.toLowerCase().includes(query) ||
        (song.genre && song.genre.some(g => g.toLowerCase().includes(query)))
      );
    }

    // Sort
    result.sort((a, b) => {
      let comparison = 0;
      
      switch (sortKey) {
        case 'title':
          comparison = a.title.localeCompare(b.title);
          break;
        case 'artist':
          comparison = a.artist.localeCompare(b.artist);
          break;
        case 'album':
          comparison = a.album.localeCompare(b.album);
          break;
        case 'duration':
          comparison = (a.duration || 0) - (b.duration || 0);
          break;
        case 'bpm':
          comparison = (a.bpm || 0) - (b.bpm || 0);
          break;
        case 'key':
          comparison = (analyzedKeyCache.get(a.id) || '').localeCompare(analyzedKeyCache.get(b.id) || '');
          break;
        case 'genre':
          comparison = (a.genre?.[0] || '').localeCompare(b.genre?.[0] || '');
          break;
      }

      return sortDirection === 'asc' ? comparison : -comparison;
    });

    return result;
  }, [categoryFilteredSongs, searchQuery, sortKey, sortDirection, keyCacheVersion]);

  const handleSort = useCallback((key: SortKey) => {
    if (sortKey === key) {
      setSortDirection(d => d === 'asc' ? 'desc' : 'asc');
    } else {
      setSortKey(key);
      setSortDirection('asc');
    }
  }, [sortKey]);

  const handleLoadToDeck = useCallback(async (song: Song, deck: DeckId) => {
    try {
      await loadTrack(deck, song);
    } catch (error) {
      console.error(`Failed to load ${song.title} to Deck ${deck}:`, error);
    }
  }, [loadTrack]);

  const isLoadedOnDeck = useCallback((songId: string): 'A' | 'B' | null => {
    if (djDeckATrack?.id === songId) return 'A';
    if (djDeckBTrack?.id === songId) return 'B';
    return null;
  }, [djDeckATrack, djDeckBTrack]);

  const toggleCategory = useCallback((categoryId: string) => {
    setExpandedCategories(prev => {
      const next = new Set(prev);
      if (next.has(categoryId)) {
        next.delete(categoryId);
      } else {
        next.add(categoryId);
      }
      return next;
    });
  }, []);

  const SortHeader: React.FC<{ label: string; sortKeyValue: SortKey; className?: string }> = 
    ({ label, sortKeyValue, className }) => (
      <th 
        className={`px-2 py-1.5 text-left text-[10px] font-medium text-neutral-500 uppercase tracking-wider cursor-pointer hover:text-neutral-300 transition-colors ${className || ''}`}
        onClick={() => handleSort(sortKeyValue)}
      >
        <div className="flex items-center gap-0.5">
          {label}
          {sortKey === sortKeyValue && (
            sortDirection === 'asc' ? <ChevronUp size={10} /> : <ChevronDown size={10} />
          )}
        </div>
      </th>
    );

  return (
    <div className="h-full flex bg-surface-0">
      {/* Sidebar */}
      <div 
        className={`
          flex-shrink-0 border-r border-white/10 bg-[#161616] transition-all duration-200
          ${sidebarCollapsed ? 'w-10' : 'w-48'}
        `}
      >
        {/* Sidebar header with collapse toggle */}
        <div className="flex items-center justify-between p-2 border-b border-white/10">
          {!sidebarCollapsed && (
            <span className="text-xs font-medium text-neutral-400">BROWSER</span>
          )}
          <button
            onClick={() => setSidebarCollapsed(!sidebarCollapsed)}
            className="p-1 hover:bg-white/10 rounded transition-colors text-neutral-500 hover:text-neutral-300"
          >
            {sidebarCollapsed ? <ChevronRight size={14} /> : <ChevronLeft size={14} />}
          </button>
        </div>

        {/* Category list */}
        {!sidebarCollapsed && (
          <div className="overflow-y-auto h-[calc(100%-36px)]">
            {categories.map(category => (
              <div key={category.id}>
                <button
                  onClick={() => {
                    if (category.children) {
                      toggleCategory(category.id);
                    } else {
                      setSelectedCategory(category.id);
                    }
                  }}
                  className={`
                    w-full flex items-center gap-2 px-2 py-1.5 text-xs transition-colors
                    ${selectedCategory === category.id 
                      ? 'bg-brand/20 text-brand' 
                      : 'text-neutral-400 hover:bg-white/5 hover:text-neutral-200'}
                  `}
                >
                  {category.children && (
                    <span className="text-neutral-600">
                      {category.expanded ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
                    </span>
                  )}
                  <span className="text-neutral-500">{category.icon}</span>
                  <span className="truncate">{category.label}</span>
                </button>
                
                {/* Children */}
                {category.expanded && category.children && (
                  <div className="ml-4 border-l border-white/5">
                    {category.children.map(child => (
                      <button
                        key={child.id}
                        onClick={() => setSelectedCategory(child.id)}
                        className={`
                          w-full flex items-center justify-between px-3 py-1 text-[11px] transition-colors
                          ${selectedCategory === child.id 
                            ? 'bg-brand/20 text-brand' 
                            : 'text-neutral-500 hover:bg-white/5 hover:text-neutral-300'}
                        `}
                      >
                        <span className="truncate">{child.label}</span>
                        {child.count !== undefined && (
                          <span className="text-neutral-600 text-[10px]">{child.count}</span>
                        )}
                      </button>
                    ))}
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Main content */}
      <div className="flex-1 flex flex-col min-w-0">
        {/* Search header */}
        <div className="flex items-center gap-3 px-3 py-2 border-b border-white/10 bg-[#1a1a1a]">
          <div className="relative flex-1 max-w-md">
            <Search size={14} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-neutral-600" />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Search..."
              className="w-full pl-8 pr-3 py-1.5 bg-surface-2 border border-white/10 rounded
                         text-xs text-neutral-100 placeholder-neutral-600
                         focus:outline-none focus:ring-1 focus:ring-brand focus:border-transparent"
            />
          </div>
          <div className="text-[11px] text-neutral-500">
            {filteredSongs.length} tracks
          </div>
        </div>

        {/* Track table */}
        <div className="flex-1 overflow-auto">
          <table className="w-full border-collapse">
            <thead className="sticky top-0 bg-[#1a1a1a] z-10">
              <tr className="border-b border-white/10">
                <th className="w-5 px-1 py-1.5 text-left text-[10px] font-medium text-neutral-500" title="Color label">🎨</th>
                <th className="w-10 px-2 py-1.5 text-left text-[10px] font-medium text-neutral-500">#</th>
                <th className="w-20 px-2 py-1.5 text-left text-[10px] font-medium text-neutral-500">LOAD</th>
                <SortHeader label="Title" sortKeyValue="title" />
                <SortHeader label="Artist" sortKeyValue="artist" />
                <SortHeader label="BPM" sortKeyValue="bpm" className="w-12 text-right" />
                <SortHeader label="Key" sortKeyValue="key" className="w-14 text-center" />
                <SortHeader label="Album" sortKeyValue="album" className="hidden xl:table-cell" />
                <SortHeader label="Time" sortKeyValue="duration" className="w-14 text-right" />
                <SortHeader label="Genre" sortKeyValue="genre" className="w-20 hidden lg:table-cell" />
              </tr>
            </thead>
            <tbody className="divide-y divide-white/5">
              {filteredSongs.map((song, index) => (
                <TrackRow
                  key={song.id}
                  song={song}
                  index={index}
                  loadedDeck={isLoadedOnDeck(song.id)}
                  onLoadToDeck={handleLoadToDeck}
                  trackColor={trackColorMap.get(song.id)}
                  onSetTrackColor={handleSetTrackColor}
                  songKey={analyzedKeyCache.get(song.id) || null}
                  keyCompatibility={computeKeyCompat(analyzedKeyCache.get(song.id))}
                />
              ))}
            </tbody>
          </table>

          {filteredSongs.length === 0 && (
            <div className="flex items-center justify-center h-24 text-neutral-600 text-sm">
              {searchQuery ? 'No tracks match your search' : 'No tracks in library'}
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

function formatDuration(seconds: number | undefined): string {
  if (!seconds) return '-:--';
  const mins = Math.floor(seconds / 60);
  const secs = Math.floor(seconds % 60);
  return `${mins}:${secs.toString().padStart(2, '0')}`;
}

export default DJLibraryBrowserV2;
