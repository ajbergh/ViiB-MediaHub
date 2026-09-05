/**
 * ViiB MediaHub - DJ Library Browser V2 Component
 *
 * Virtualized library content hosted by DJLibraryDrawer with:
 * - Collapsible playlist/genre sidebar
 * - Enhanced track table with more columns
 * - Visual styling matching PCDJ DEX reference
 * - Load-to-deck indicators
 * - Drag-to-deck support
 *
 * @module components/dj/v2/DJLibraryBrowserV2
 */

import React, { useState, useMemo, useCallback, useEffect, useRef, memo } from 'react';
import { TableVirtuoso, type TableComponents } from 'react-virtuoso';
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
  Library,
  SlidersHorizontal
} from 'lucide-react';

type SortKey = 'title' | 'artist' | 'album' | 'duration' | 'bpm' | 'key' | 'genre';
type SortDirection = 'asc' | 'desc';
const DJ_TRACK_DRAG_MIME = 'application/x-viib-dj-track';

type OptionalColumn = 'bpm' | 'key' | 'album' | 'time' | 'genre';
type ResizableColumn = 'title' | 'artist' | 'album';

const COLUMN_VISIBILITY_STORAGE_KEY = 'viib.dj.library.columnVisibility';
const COLUMN_WIDTHS_STORAGE_KEY = 'viib.dj.library.columnWidths';

const DEFAULT_COLUMN_VISIBILITY: Record<OptionalColumn, boolean> = {
  bpm: true,
  key: true,
  album: true,
  time: true,
  genre: true,
};

const DEFAULT_COLUMN_WIDTHS: Record<ResizableColumn, number> = {
  title: 220,
  artist: 150,
  album: 150,
};

// Track color coding - persisted in session via Map
const TRACK_COLORS = ['#ef4444', '#f97316', '#eab308', '#22c55e', '#3b82f6', '#a855f7', '#ec4899', '#ffffff'] as const;
const trackColorMap = new Map<string, string>();

// Session cache for analyzed musical keys (populated when tracks are loaded into decks)
const analyzedKeyCache = new Map<string, string>();

function loadColumnVisibility(): Record<OptionalColumn, boolean> {
  if (typeof window === 'undefined') return DEFAULT_COLUMN_VISIBILITY;
  try {
    const stored = window.localStorage.getItem(COLUMN_VISIBILITY_STORAGE_KEY);
    if (!stored) return DEFAULT_COLUMN_VISIBILITY;
    return { ...DEFAULT_COLUMN_VISIBILITY, ...JSON.parse(stored) };
  } catch {
    return DEFAULT_COLUMN_VISIBILITY;
  }
}

function loadColumnWidths(): Record<ResizableColumn, number> {
  if (typeof window === 'undefined') return DEFAULT_COLUMN_WIDTHS;
  try {
    const stored = window.localStorage.getItem(COLUMN_WIDTHS_STORAGE_KEY);
    if (!stored) return DEFAULT_COLUMN_WIDTHS;
    const parsed = JSON.parse(stored);
    return {
      title: Math.max(140, Math.min(420, Number(parsed.title) || DEFAULT_COLUMN_WIDTHS.title)),
      artist: Math.max(100, Math.min(320, Number(parsed.artist) || DEFAULT_COLUMN_WIDTHS.artist)),
      album: Math.max(100, Math.min(320, Number(parsed.album) || DEFAULT_COLUMN_WIDTHS.album)),
    };
  } catch {
    return DEFAULT_COLUMN_WIDTHS;
  }
}

interface PlaylistCategory {
  id: string;
  label: string;
  icon: React.ReactNode;
  expanded?: boolean;
  children?: { id: string; label: string; count?: number }[];
}

// Memoized table row to prevent unnecessary re-renders
const TrackRowCells = memo(({
  song,
  index,
  loadedDeck,
  onLoadToDeck,
  trackColor,
  onSetTrackColor,
  songKey,
  keyCompatibility,
  columnVisibility,
  columnWidths,
}: {
  song: Song;
  index: number;
  loadedDeck: 'A' | 'B' | null;
  onLoadToDeck: (song: Song, deck: DeckId) => void;
  trackColor: string | undefined;
  onSetTrackColor: (songId: string, color: string | null) => void;
  songKey: string | null;           // Musical key from analysis cache
  keyCompatibility: number | null;  // 0-1 score, null if no key data
  columnVisibility: Record<OptionalColumn, boolean>;
  columnWidths: Record<ResizableColumn, number>;
}) => {
  const [showColorPicker, setShowColorPicker] = React.useState(false);

  return (
    <>
      {/* Color label */}
      <td className="px-1 py-1 w-7 relative">
        <button
          className="w-6 h-6 rounded-full border border-[#444] hover:border-neutral-300 transition-colors flex-shrink-0 dj-focus-ring"
          style={{ backgroundColor: trackColor || '#333' }}
          onClick={(e) => { e.stopPropagation(); setShowColorPicker(!showColorPicker); }}
          aria-label={trackColor ? `Track color set — change` : 'Set track color label'}
          title={trackColor ? 'Change color label' : 'Set track color label'}
        />
        {showColorPicker && (
          <div className="absolute top-full left-0 z-50 bg-[#1a1a1a] border border-[#444] rounded p-1 flex gap-0.5 shadow-lg"
            onMouseLeave={() => setShowColorPicker(false)}>
            {TRACK_COLORS.map(c => (
              <button
                key={c}
                className={`w-6 h-6 rounded-full border transition-transform hover:scale-110 ${
                  trackColor === c ? 'border-white scale-110' : 'border-transparent'
                }`}
                style={{ backgroundColor: c }}
                onClick={(e) => { e.stopPropagation(); onSetTrackColor(song.id, c); setShowColorPicker(false); }}
              />
            ))}
            <button
              className="w-6 h-6 rounded-full border border-[#555] bg-[#333] text-[10px] text-neutral-400 flex items-center justify-center hover:border-white"
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
            aria-label={`Load ${song.title} to Deck A`}
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
            aria-label={`Load ${song.title} to Deck B`}
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
      <td className="px-2 py-1.5" style={{ width: columnWidths.title }}>
        <div className="flex items-center gap-1.5">
          {loadedDeck && (
            <span className={`text-[10px] ${loadedDeck === 'A' ? 'text-blue-400' : 'text-purple-400'}`}>
              ▶
            </span>
          )}
          <span className="text-neutral-100 truncate block" style={{ maxWidth: columnWidths.title - 24 }} title={song.title}>
            {song.title}
          </span>
        </div>
      </td>

      {/* Artist */}
      <td className="px-2 py-1.5" style={{ width: columnWidths.artist }}>
        <span className="text-neutral-400 truncate block" style={{ maxWidth: columnWidths.artist - 16 }} title={song.artist}>
          {song.artist}
        </span>
      </td>

      {/* BPM */}
      {columnVisibility.bpm && (
        <td className="px-2 py-1.5 w-12 text-right">
          <span className={`font-mono ${song.bpm ? 'text-green-400' : 'text-neutral-600'}`}>
            {song.bpm || '-'}
          </span>
        </td>
      )}

      {/* Key with harmonic compatibility */}
      {columnVisibility.key && (
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
      )}

      {/* Album */}
      {columnVisibility.album && (
        <td className="px-2 py-1.5 hidden xl:table-cell" style={{ width: columnWidths.album }}>
          <span className="text-neutral-500 truncate block" style={{ maxWidth: columnWidths.album - 16 }} title={song.album}>
            {song.album}
          </span>
        </td>
      )}

      {/* Time */}
      {columnVisibility.time && (
        <td className="px-2 py-1.5 w-14 text-right">
          <span className="font-mono text-neutral-400">
            {formatDuration(song.duration)}
          </span>
        </td>
      )}

      {/* Genre */}
      {columnVisibility.genre && (
        <td className="px-2 py-1.5 w-20 hidden lg:table-cell">
          <span className="text-neutral-500 truncate block text-[10px]">
            {song.genre?.[0] || '-'}
          </span>
        </td>
      )}
    </>
  );
});

TrackRowCells.displayName = 'TrackRowCells';

interface DJLibraryBrowserV2Props {
  autoFocusSearch?: boolean;
}

interface LibraryTableContext {
  isLoadedOnDeck: (songId: string) => 'A' | 'B' | null;
  handleLoadToDeck: (song: Song, deck: DeckId) => Promise<void>;
}

// Stable component identities preserve rows, focus and scroll across UI updates.
const libraryTableComponents: TableComponents<Song, LibraryTableContext> = {
  Table: ({ style, context: _context, ...props }) => (
    <table {...props} style={{ ...style, width: '100%', borderCollapse: 'collapse' }} />
  ),
  TableRow: ({ item, context, ...props }) => {
    const { isLoadedOnDeck, handleLoadToDeck } = context!;
    const loadedDeck = item ? isLoadedOnDeck(item.id) : null;
    const deckIndicatorClass = loadedDeck === 'A'
      ? 'bg-blue-600/15 border-l-2 border-blue-500 shadow-[inset_0_0_12px_rgba(59,130,246,0.08)]'
      : loadedDeck === 'B'
        ? 'bg-purple-600/15 border-l-2 border-purple-500 shadow-[inset_0_0_12px_rgba(139,92,246,0.08)]'
        : 'border-l-2 border-transparent';
    return (
      <tr
        {...props}
        draggable={!!item}
        onDragStart={(e) => {
          if (!item) return;
          const payload = JSON.stringify({ type: 'viib-dj-track', songId: item.id });
          e.dataTransfer.effectAllowed = 'copy';
          e.dataTransfer.setData(DJ_TRACK_DRAG_MIME, payload);
          e.dataTransfer.setData('text/plain', item.id);
        }}
        className={`hover:bg-white/5 transition-colors text-xs ${deckIndicatorClass}`}
        onDoubleClick={() => item && handleLoadToDeck(item, 'A')}
      />
    );
  },
};

const SortHeader: React.FC<{
    label: string;
    sortKeyValue: SortKey;
    className?: string;
    width?: number;
    resizable?: ResizableColumn;
    sortKey: SortKey;
    sortDirection: SortDirection;
    handleSort: (key: SortKey) => void;
    startColumnResize: (event: React.MouseEvent, column: ResizableColumn) => void;
  }> =
    ({ label, sortKeyValue, className, width, resizable, sortKey, sortDirection, handleSort, startColumnResize }) => (
      <th
        className={`relative px-2 py-1.5 text-left text-[10px] font-medium text-neutral-500 uppercase tracking-wider cursor-pointer hover:text-neutral-300 transition-colors ${className || ''}`}
        style={width ? { width } : undefined}
        tabIndex={0}
        aria-sort={sortKey === sortKeyValue ? (sortDirection === 'asc' ? 'ascending' : 'descending') : 'none'}
        onKeyDown={event => {
          if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); handleSort(sortKeyValue); }
        }}
        onClick={() => handleSort(sortKeyValue)}
      >
        <div className="flex items-center gap-0.5">
          {label}
          {sortKey === sortKeyValue && (
            sortDirection === 'asc' ? <ChevronUp size={10} /> : <ChevronDown size={10} />
          )}
        </div>
        {resizable && (
          <span
            role='separator'
            aria-orientation='vertical'
            aria-label={`Resize ${label} column`}
            className='absolute right-0 top-1 bottom-1 w-1 cursor-col-resize rounded bg-transparent hover:bg-brand/50'
            onClick={event => event.stopPropagation()}
            onMouseDown={event => startColumnResize(event, resizable)}
          />
        )}
      </th>
    );


export const DJLibraryBrowserV2: React.FC<DJLibraryBrowserV2Props> = ({ autoFocusSearch = false }) => {
  const songs = useStore(state => state.songs);
  const playlists = useStore(state => state.playlists);
  const djDeckATrack = useStore(state => state.djDeckA.track);
  const djDeckBTrack = useStore(state => state.djDeckB.track);
  const djDeckAKey = useStore(state => state.djDeckA.key);
  const djDeckBKey = useStore(state => state.djDeckB.key);
  const djDeckAIsPlaying = useStore(state => state.djDeckA.isPlaying);
  const djDeckBIsPlaying = useStore(state => state.djDeckB.isPlaying);

  const { loadTrack } = useDJAudioEngineActions();
  const searchInputRef = useRef<HTMLInputElement>(null);
  const resizeCleanupRef = useRef<(() => void) | null>(null);
  useEffect(() => () => resizeCleanupRef.current?.(), []);
  const resizingColumnRef = useRef<{
    column: ResizableColumn;
    startX: number;
    startWidth: number;
  } | null>(null);

  const [searchQuery, setSearchQuery] = useState('');
  const [sortKey, setSortKey] = useState<SortKey>('title');
  const [sortDirection, setSortDirection] = useState<SortDirection>('asc');
  const [colorVersion, setColorVersion] = useState(0); // Force re-render on color change
  const [columnMenuOpen, setColumnMenuOpen] = useState(false);
  const [columnVisibility, setColumnVisibility] = useState<Record<OptionalColumn, boolean>>(loadColumnVisibility);
  const [columnWidths, setColumnWidths] = useState<Record<ResizableColumn, number>>(loadColumnWidths);

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

  useEffect(() => {
    if (!autoFocusSearch) { resizeCleanupRef.current?.(); return; }
    const frame = requestAnimationFrame(() => {
      searchInputRef.current?.focus({ preventScroll: true });
      searchInputRef.current?.select();
    });
    return () => cancelAnimationFrame(frame);
  }, [autoFocusSearch]);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    window.localStorage.setItem(COLUMN_VISIBILITY_STORAGE_KEY, JSON.stringify(columnVisibility));
  }, [columnVisibility]);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    window.localStorage.setItem(COLUMN_WIDTHS_STORAGE_KEY, JSON.stringify(columnWidths));
  }, [columnWidths]);

  // Track color coding handlers
  const handleSetTrackColor = useCallback((songId: string, color: string | null) => {
    if (color) {
      trackColorMap.set(songId, color);
    } else {
      trackColorMap.delete(songId);
    }
    setColorVersion(v => v + 1); // Trigger re-render
  }, []);

  const toggleColumnVisibility = useCallback((column: OptionalColumn) => {
    setColumnVisibility(current => ({
      ...current,
      [column]: !current[column],
    }));
  }, []);

  const startColumnResize = useCallback((event: React.MouseEvent, column: ResizableColumn) => {
    event.preventDefault();
    event.stopPropagation();
    resizeCleanupRef.current?.();
    resizingColumnRef.current = {
      column,
      startX: event.clientX,
      startWidth: columnWidths[column],
    };

    const handleMouseMove = (moveEvent: MouseEvent) => {
      const resize = resizingColumnRef.current;
      if (!resize) return;
      const nextWidth = Math.max(100, Math.min(440, resize.startWidth + moveEvent.clientX - resize.startX));
      setColumnWidths(current => ({
        ...current,
        [resize.column]: nextWidth,
      }));
    };

    const handleMouseUp = () => {
      resizingColumnRef.current = null;
      resizeCleanupRef.current = null;
      document.removeEventListener('mousemove', handleMouseMove);
      document.removeEventListener('mouseup', handleMouseUp);
    };

    resizeCleanupRef.current = handleMouseUp;
    document.addEventListener('mousemove', handleMouseMove);
    document.addEventListener('mouseup', handleMouseUp);
  }, [columnWidths]);

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
      const songIds = new Set(playlist.songIds);
      return songs.filter(s => songIds.has(s.id));
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

  const tableContext = useMemo(() => ({ isLoadedOnDeck, handleLoadToDeck }), [isLoadedOnDeck, handleLoadToDeck]);

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


  return (
    <div className="h-full flex bg-surface-0" onKeyDown={event => {
      if (event.key === 'Escape' && columnMenuOpen) {
        event.preventDefault();
        event.stopPropagation();
        setColumnMenuOpen(false);
      }
    }}>
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
            aria-label={sidebarCollapsed ? 'Expand browser sidebar' : 'Collapse browser sidebar'}
            aria-expanded={!sidebarCollapsed}
            title={sidebarCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}
            className="w-8 h-8 flex items-center justify-center hover:bg-white/10 rounded transition-colors text-neutral-500 hover:text-neutral-300 dj-focus-ring"
          >
            {sidebarCollapsed ? <ChevronRight size={16} /> : <ChevronLeft size={16} />}
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
        <div className="relative flex items-center gap-3 px-3 py-2 border-b border-white/10 bg-[#1a1a1a]">
          <div className="relative flex-1 max-w-md">
            <Search size={14} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-neutral-600" />
            <input
              ref={searchInputRef}
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Search..."
              aria-label="Search DJ library"
              className="w-full pl-8 pr-3 py-1.5 bg-surface-2 border border-white/10 rounded
                         text-xs text-neutral-100 placeholder-neutral-600
                         focus:outline-none focus:ring-1 focus:ring-brand focus:border-transparent"
            />
          </div>
          <div className="text-[11px] text-neutral-500">
            {filteredSongs.length} tracks
          </div>
          <button
            type="button"
            onClick={() => setColumnMenuOpen(open => !open)}
            aria-expanded={columnMenuOpen}
            aria-haspopup="menu"
            title="Library columns"
            className="h-8 px-2 rounded border border-white/10 bg-[#222] text-neutral-400 hover:text-neutral-100 hover:bg-[#2a2a2a] flex items-center gap-1.5 text-[11px] font-medium"
          >
            <SlidersHorizontal size={13} aria-hidden />
            Columns
          </button>
          {columnMenuOpen && (
            <div
              role="menu"
              className="absolute right-3 top-[42px] z-50 w-40 rounded-md border border-[#444] bg-[#151515] p-1.5 shadow-xl"
            >
              {([
                ['bpm', 'BPM'],
                ['key', 'Key'],
                ['album', 'Album'],
                ['time', 'Time'],
                ['genre', 'Genre'],
              ] as Array<[OptionalColumn, string]>).map(([column, label]) => (
                <label
                  key={column}
                  className="flex items-center gap-2 px-2 py-1.5 rounded text-[11px] text-neutral-300 hover:bg-white/5"
                >
                  <input
                    type="checkbox"
                    checked={columnVisibility[column]}
                    onChange={() => toggleColumnVisibility(column)}
                    className="accent-brand"
                  />
                  {label}
                </label>
              ))}
            </div>
          )}
        </div>

        {/* Virtualized track table */}
        <div className="flex-1 overflow-hidden">
          {filteredSongs.length === 0 ? (
            <div className="flex items-center justify-center h-24 text-neutral-600 text-sm">
              {searchQuery ? 'No tracks match your search' : 'No tracks in library'}
            </div>
          ) : (
            <TableVirtuoso
              data={filteredSongs}
              className="h-full"
              computeItemKey={(_idx, song) => song.id}
              fixedHeaderContent={() => (
                <tr className="border-b border-white/10 bg-[#1a1a1a]">
                  <th className="w-5 px-1 py-1.5 text-left text-[10px] font-medium text-neutral-500 bg-[#1a1a1a]" title="Color label">🎨</th>
                  <th className="w-10 px-2 py-1.5 text-left text-[10px] font-medium text-neutral-500 bg-[#1a1a1a]">#</th>
                  <th className="w-20 px-2 py-1.5 text-left text-[10px] font-medium text-neutral-500 bg-[#1a1a1a]">LOAD</th>
                  <SortHeader sortKey={sortKey} sortDirection={sortDirection} handleSort={handleSort} startColumnResize={startColumnResize} label="Title" sortKeyValue="title" width={columnWidths.title} resizable="title" />
                  <SortHeader sortKey={sortKey} sortDirection={sortDirection} handleSort={handleSort} startColumnResize={startColumnResize} label="Artist" sortKeyValue="artist" width={columnWidths.artist} resizable="artist" />
                  {columnVisibility.bpm && <SortHeader sortKey={sortKey} sortDirection={sortDirection} handleSort={handleSort} startColumnResize={startColumnResize} label="BPM" sortKeyValue="bpm" className="w-12 text-right" />}
                  {columnVisibility.key && <SortHeader sortKey={sortKey} sortDirection={sortDirection} handleSort={handleSort} startColumnResize={startColumnResize} label="Key" sortKeyValue="key" className="w-14 text-center" />}
                  {columnVisibility.album && <SortHeader sortKey={sortKey} sortDirection={sortDirection} handleSort={handleSort} startColumnResize={startColumnResize} label="Album" sortKeyValue="album" className="hidden xl:table-cell" width={columnWidths.album} resizable="album" />}
                  {columnVisibility.time && <SortHeader sortKey={sortKey} sortDirection={sortDirection} handleSort={handleSort} startColumnResize={startColumnResize} label="Time" sortKeyValue="duration" className="w-14 text-right" />}
                  {columnVisibility.genre && <SortHeader sortKey={sortKey} sortDirection={sortDirection} handleSort={handleSort} startColumnResize={startColumnResize} label="Genre" sortKeyValue="genre" className="w-20 hidden lg:table-cell" />}
                </tr>
              )}
              itemContent={(index, song) => (
                <TrackRowCells
                  song={song}
                  index={index}
                  loadedDeck={isLoadedOnDeck(song.id)}
                  onLoadToDeck={handleLoadToDeck}
                  trackColor={trackColorMap.get(song.id)}
                  onSetTrackColor={handleSetTrackColor}
                  songKey={analyzedKeyCache.get(song.id) || null}
                  keyCompatibility={computeKeyCompat(analyzedKeyCache.get(song.id))}
                  columnVisibility={columnVisibility}
                  columnWidths={columnWidths}
                />
              )}
              components={libraryTableComponents}
              context={tableContext}
            />
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
