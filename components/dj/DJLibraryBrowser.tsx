/**
 * ViiB MediaHub - DJ Library Browser Component
 * 
 * Bottom panel showing the track library with DJ-specific columns.
 * Allows loading tracks to decks via drag-drop or context menu.
 * 
 * Features:
 * - Sortable columns (Name, Artist, Album, BPM, Key, Time)
 * - Search/filter
 * - Load to Deck A/B buttons
 * - Highlight currently loaded tracks
 * - Compatible key/BPM filtering (Phase 2+)
 * 
 * @module components/dj/DJLibraryBrowser
 */

import React, { useState, useMemo, useCallback } from 'react';
import { TableVirtuoso } from 'react-virtuoso';
import { useStore } from '../../store';
import { useDJAudioEngine } from '../../hooks/useDJAudioEngine';
import type { DeckId } from '../../slices/djMixerSlice';
import { Song } from '../../types';
import { Search, Upload, ChevronUp, ChevronDown } from 'lucide-react';

type SortKey = 'title' | 'artist' | 'album' | 'duration' | 'bpm' | 'genre';
type SortDirection = 'asc' | 'desc';

export const DJLibraryBrowser: React.FC = () => {
  const songs = useStore(state => state.songs);
  // Only subscribe to track info, not entire deck state (avoids re-renders on position updates)
  const djDeckATrack = useStore(state => state.djDeckA.track);
  const djDeckBTrack = useStore(state => state.djDeckB.track);
  
  // Use DJ audio engine hook for loading tracks (includes waveform fetching)
  const { loadTrack, initialize } = useDJAudioEngine();

  const [searchQuery, setSearchQuery] = useState('');
  const [sortKey, setSortKey] = useState<SortKey>('title');
  const [sortDirection, setSortDirection] = useState<SortDirection>('asc');

  // Filter and sort songs
  const filteredSongs = useMemo(() => {
    let result = [...songs];

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
        case 'genre':
          comparison = (a.genre?.[0] || '').localeCompare(b.genre?.[0] || '');
          break;
      }

      return sortDirection === 'asc' ? comparison : -comparison;
    });

    return result;
  }, [songs, searchQuery, sortKey, sortDirection]);

  const handleSort = useCallback((key: SortKey) => {
    if (sortKey === key) {
      setSortDirection(d => d === 'asc' ? 'desc' : 'asc');
    } else {
      setSortKey(key);
      setSortDirection('asc');
    }
  }, [sortKey]);

  const handleLoadToDeck = useCallback(async (song: Song, deck: DeckId) => {
    console.log(`🎧 DJLibraryBrowser: Loading ${song.title} to Deck ${deck}`);
    try {
      await loadTrack(deck, song);
      console.log(`🎧 DJLibraryBrowser: Successfully loaded ${song.title} to Deck ${deck}`);
    } catch (error) {
      console.error(`🎧 DJLibraryBrowser: Failed to load ${song.title} to Deck ${deck}:`, error);
    }
  }, [loadTrack]);

  const isLoadedOnDeck = useCallback((songId: string) => {
    if (djDeckATrack?.id === songId) return 'A';
    if (djDeckBTrack?.id === songId) return 'B';
    return null;
  }, [djDeckATrack, djDeckBTrack]);

  const SortHeader: React.FC<{ label: string; sortKeyValue: SortKey; width?: string }> = 
    ({ label, sortKeyValue, width }) => (
      <th 
        className={`px-3 py-2 text-left text-xs font-medium text-neutral-400 uppercase tracking-wider cursor-pointer hover:text-neutral-200 ${width || ''}`}
        onClick={() => handleSort(sortKeyValue)}
      >
        <div className="flex items-center gap-1">
          {label}
          {sortKey === sortKeyValue && (
            sortDirection === 'asc' ? <ChevronUp size={14} /> : <ChevronDown size={14} />
          )}
        </div>
      </th>
    );

  return (
    <div className="h-full flex flex-col">
      {/* Header with search */}
      <div className="flex items-center gap-4 p-3 border-b border-white/10">
        <div className="relative flex-1 max-w-md">
          <Search size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-neutral-500" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="Search library..."
            aria-label="Search DJ library"
            className="w-full pl-9 pr-4 py-2 bg-surface-2 border border-white/10 rounded-lg
                       text-sm text-neutral-100 placeholder-neutral-500
                       focus:outline-none focus:ring-1 focus:ring-brand"
          />
        </div>
        <div className="text-sm text-neutral-400">
          {filteredSongs.length} tracks
        </div>
      </div>

      {/* Virtualized track table */}
      <div className="flex-1 overflow-hidden">
        {filteredSongs.length === 0 ? (
          <div className="flex items-center justify-center h-32 text-neutral-500">
            {searchQuery ? 'No tracks match your search' : 'No tracks in library'}
          </div>
        ) : (
          <TableVirtuoso
            data={filteredSongs}
            className="h-full"
            computeItemKey={(_idx, song) => song.id}
            fixedHeaderContent={() => (
              <tr className="bg-surface-1">
                <th className="w-24 px-3 py-2 text-left text-xs font-medium text-neutral-400 uppercase tracking-wider bg-surface-1">
                  Load
                </th>
                <SortHeader label="Title" sortKeyValue="title" />
                <SortHeader label="Artist" sortKeyValue="artist" />
                <SortHeader label="Album" sortKeyValue="album" />
                <SortHeader label="Genre" sortKeyValue="genre" width="w-24" />
                <SortHeader label="Time" sortKeyValue="duration" width="w-16" />
                <SortHeader label="BPM" sortKeyValue="bpm" width="w-16" />
              </tr>
            )}
            itemContent={(_idx, song) => {
              const loadedDeck = isLoadedOnDeck(song.id);
              return (
                <>
                  <td className={`px-3 py-2 ${loadedDeck ? 'bg-brand/10' : ''}`}>
                    <div className="flex gap-1">
                      <button
                        onClick={() => handleLoadToDeck(song, 'A')}
                        aria-label={`Load ${song.title} to Deck A`}
                        className={`
                          px-2 py-1 text-xs rounded font-medium transition-colors
                          ${loadedDeck === 'A'
                            ? 'bg-blue-600 text-white'
                            : 'bg-surface-2 text-neutral-400 hover:bg-blue-600/50 hover:text-white'}
                        `}
                        title="Load to Deck A"
                      >
                        A
                      </button>
                      <button
                        onClick={() => handleLoadToDeck(song, 'B')}
                        aria-label={`Load ${song.title} to Deck B`}
                        className={`
                          px-2 py-1 text-xs rounded font-medium transition-colors
                          ${loadedDeck === 'B'
                            ? 'bg-purple-600 text-white'
                            : 'bg-surface-2 text-neutral-400 hover:bg-purple-600/50 hover:text-white'}
                        `}
                        title="Load to Deck B"
                      >
                        B
                      </button>
                    </div>
                  </td>

                  <td className={`px-3 py-2 ${loadedDeck ? 'bg-brand/10' : ''}`}>
                    <div className="flex items-center gap-2">
                      {loadedDeck && (
                        <span className={`text-xs font-bold ${loadedDeck === 'A' ? 'text-blue-400' : 'text-purple-400'}`} aria-hidden="true">
                          ▶
                        </span>
                      )}
                      <span className="text-sm text-neutral-100 truncate max-w-[200px]">
                        {song.title}
                      </span>
                    </div>
                  </td>

                  <td className={`px-3 py-2 ${loadedDeck ? 'bg-brand/10' : ''}`}>
                    <span className="text-sm text-neutral-400 truncate max-w-[150px] block">
                      {song.artist}
                    </span>
                  </td>

                  <td className={`px-3 py-2 ${loadedDeck ? 'bg-brand/10' : ''}`}>
                    <span className="text-sm text-neutral-500 truncate max-w-[150px] block">
                      {song.album}
                    </span>
                  </td>

                  <td className={`px-3 py-2 ${loadedDeck ? 'bg-brand/10' : ''}`}>
                    <span className="text-xs text-neutral-500 truncate block">
                      {song.genre?.[0] || '-'}
                    </span>
                  </td>

                  <td className={`px-3 py-2 ${loadedDeck ? 'bg-brand/10' : ''}`}>
                    <span className="text-sm font-mono text-neutral-400">
                      {formatDuration(song.duration)}
                    </span>
                  </td>

                  <td className={`px-3 py-2 ${loadedDeck ? 'bg-brand/10' : ''}`}>
                    <span className={`text-sm font-mono ${song.bpm ? 'text-brand' : 'text-neutral-600'}`}>
                      {song.bpm || '-'}
                    </span>
                  </td>
                </>
              );
            }}
            components={{
              Table: ({ style, ...props }) => (
                <table {...props} style={{ ...style, width: '100%' }} className="w-full" />
              ),
              TableRow: ({ item, ...props }) => {
                const loadedDeck = item ? isLoadedOnDeck(item.id) : null;
                return (
                  <tr
                    {...props}
                    className={`hover:bg-white/5 transition-colors ${loadedDeck ? 'bg-brand/5' : ''}`}
                  />
                );
              },
            }}
          />
        )}
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

export default DJLibraryBrowser;
