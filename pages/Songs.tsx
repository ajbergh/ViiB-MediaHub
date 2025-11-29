/**
 * ViiB MediaHub - Songs Page
 * 
 * Displays the complete song library with virtualized scrolling for performance.
 * 
 * Features:
 * - Virtualized list using react-virtuoso for large libraries
 * - Multiple sort options (title, artist, album, duration, plays)
 * - Search/filter functionality
 * - Header with play all button
 * - Context menu support for each song
 * 
 * @module Songs
 */

import React, { useState, useEffect, useMemo } from 'react';
import { useStore, useAlbumCovers } from '../store';
import { Play, Clock, MoreHorizontal, Search, ChevronDown, ArrowUpDown } from 'lucide-react';
import { formatTime, generateGradient } from '../utils';
import { ContextMenuType, Song } from '../types';
import { Virtuoso, Components } from 'react-virtuoso';

type SongSortOption = 'recent' | 'title-asc' | 'title-desc' | 'artist-asc' | 'artist-desc' | 'album-asc' | 'album-desc' | 'duration-asc' | 'duration-desc' | 'plays-desc';

const sortLabels: Record<SongSortOption, string> = {
  'recent': 'Recently Added',
  'title-asc': 'Title (A-Z)',
  'title-desc': 'Title (Z-A)',
  'artist-asc': 'Artist (A-Z)',
  'artist-desc': 'Artist (Z-A)',
  'album-asc': 'Album (A-Z)',
  'album-desc': 'Album (Z-A)',
  'duration-asc': 'Duration (Short)',
  'duration-desc': 'Duration (Long)',
  'plays-desc': 'Most Played',
};

// Context interface for the Virtuoso list
interface SongsContext {
    filter: string;
    setFilter: (val: string) => void;
    sortBy: SongSortOption;
    setSortBy: (val: SongSortOption) => void;
    showSortMenu: boolean;
    setShowSortMenu: (val: boolean) => void;
}

// Define Header outside to maintain stability
const SongsHeader: React.FC<{ context?: SongsContext }> = ({ context }) => {
    // Safety check for context
    const { filter, setFilter, sortBy, setSortBy, showSortMenu, setShowSortMenu } = context || { 
        filter: '', 
        setFilter: () => {}, 
        sortBy: 'recent' as SongSortOption, 
        setSortBy: () => {},
        showSortMenu: false,
        setShowSortMenu: () => {}
    };

    return (
        <div className="p-8 pb-0">
            <div className="flex flex-col md:flex-row items-start md:items-center justify-between mb-8 gap-4">
                <h1 className="text-3xl font-bold">All Songs</h1>
                <div className="flex items-center gap-3 w-full md:w-auto">
                    {/* Sort Dropdown */}
                    <div className="relative">
                        <button
                            onClick={() => setShowSortMenu(!showSortMenu)}
                            className="flex items-center gap-2 px-4 py-2 bg-surface-highlight hover:bg-surface-hover rounded-full text-sm text-text-main transition-colors border border-transparent hover:border-surface-slider whitespace-nowrap"
                        >
                            <ArrowUpDown size={16} className="text-text-secondary" />
                            <span className="hidden sm:inline">{sortLabels[sortBy]}</span>
                            <ChevronDown size={16} className={`text-text-secondary transition-transform ${showSortMenu ? 'rotate-180' : ''}`} />
                        </button>
                        
                        {showSortMenu && (
                            <>
                                <div className="fixed inset-0 z-40" onClick={() => setShowSortMenu(false)} />
                                <div className="absolute right-0 top-full mt-2 bg-surface-2 border border-surface-3 rounded-lg shadow-xl z-50 py-1 min-w-[180px]">
                                    {(Object.keys(sortLabels) as SongSortOption[]).map((option) => (
                                        <button
                                            key={option}
                                            onClick={() => { setSortBy(option); setShowSortMenu(false); }}
                                            className={`w-full text-left px-4 py-2 text-sm hover:bg-surface-hover transition-colors ${sortBy === option ? 'text-brand font-medium' : 'text-text-main'}`}
                                        >
                                            {sortLabels[option]}
                                        </button>
                                    ))}
                                </div>
                            </>
                        )}
                    </div>

                    {/* Search Input */}
                    <div className="relative flex-1 md:w-72">
                        <Search className="absolute left-3 top-1/2 -translate-y-1/2 text-text-secondary" size={18} />
                        <input 
                            type="text" 
                            placeholder="Search songs..."
                            value={filter}
                            onChange={(e) => setFilter(e.target.value)}
                            className="w-full bg-surface-highlight border border-transparent focus:border-surface-slider rounded-full py-2 pl-10 pr-4 text-sm text-text-main outline-none placeholder-text-subtle"
                        />
                    </div>
                </div>
            </div>

            {/* Table Header */}
            <div className="bg-surface-1 rounded-t-lg sticky top-0 z-10 border-b border-surface-3 grid grid-cols-[50px_4fr_3fr_3fr_100px_50px] gap-4 px-4 py-3 text-text-secondary text-xs uppercase tracking-wider font-medium shadow-md">
                <div className="text-center">#</div>
                <div>Title</div>
                <div>Album</div>
                <div>Artist</div>
                <div className="flex justify-end pr-2"><Clock size={16} /></div>
                <div></div>
            </div>
        </div>
    );
};

const Footer = () => <div className="h-32 bg-transparent" />;

export const Songs: React.FC = () => {
  const { songs, playSong, currentSong, isPlaying, openContextMenu } = useStore();
  const albumCovers = useAlbumCovers();
  const [filter, setFilter] = useState('');
  const [sortBy, setSortBy] = useState<SongSortOption>('recent');
  const [showSortMenu, setShowSortMenu] = useState(false);
  const [scrollParent, setScrollParent] = useState<HTMLElement | null>(null);

  useEffect(() => {
    // Find the main scroll container from Layout
    setScrollParent(document.querySelector('main'));
  }, []);

  const sortedAndFilteredSongs = useMemo(() => {
    // First filter
    let result = songs.filter(
      s => s.title.toLowerCase().includes(filter.toLowerCase()) || 
           s.artist.toLowerCase().includes(filter.toLowerCase()) ||
           s.album.toLowerCase().includes(filter.toLowerCase())
    );
    
    // Then sort
    switch (sortBy) {
      case 'recent':
        return result.sort((a, b) => (b.addedAt || 0) - (a.addedAt || 0));
      case 'title-asc':
        return result.sort((a, b) => a.title.localeCompare(b.title));
      case 'title-desc':
        return result.sort((a, b) => b.title.localeCompare(a.title));
      case 'artist-asc':
        return result.sort((a, b) => a.artist.localeCompare(b.artist));
      case 'artist-desc':
        return result.sort((a, b) => b.artist.localeCompare(a.artist));
      case 'album-asc':
        return result.sort((a, b) => a.album.localeCompare(b.album));
      case 'album-desc':
        return result.sort((a, b) => b.album.localeCompare(a.album));
      case 'duration-asc':
        return result.sort((a, b) => a.duration - b.duration);
      case 'duration-desc':
        return result.sort((a, b) => b.duration - a.duration);
      case 'plays-desc':
        return result.sort((a, b) => (b.playCount || 0) - (a.playCount || 0));
      default:
        return result;
    }
  }, [songs, filter, sortBy]);

  // Memoize components to prevent re-renders of the list structure
  const components: Components<any, any> = useMemo(() => ({
    Header: SongsHeader,
    Footer: Footer
  }), []);

  return (
    <div className="h-full">
        <Virtuoso
            useWindowScroll={false}
            customScrollParent={scrollParent}
            data={sortedAndFilteredSongs}
            context={{ filter, setFilter, sortBy, setSortBy, showSortMenu, setShowSortMenu }}
            components={components}
            itemContent={(index, song) => {
               const isCurrent = currentSong?.id === song.id;
               const displayCover = song.coverUrl || albumCovers[song.album];
               
               return (
                <div className="bg-surface-1 px-8"> {/* Wrapper to match page padding visually for bg */}
                    <div 
                        className={`grid grid-cols-[50px_4fr_3fr_3fr_100px_50px] gap-4 px-4 py-3 items-center hover:bg-surface-hover group transition-colors cursor-pointer border-b border-transparent hover:border-surface-highlight ${isCurrent ? 'bg-surface-hover' : 'bg-surface-1'}`}
                        onClick={() => playSong(song)}
                        onContextMenu={(e) => openContextMenu(e, ContextMenuType.SONG, song)}
                    >
                        <div className="text-center text-text-subtle font-mono text-sm relative h-full flex items-center justify-center">
                            <span className="group-hover:hidden">{isCurrent && isPlaying ? <div className="w-3 h-3 bg-brand rounded-full animate-pulse"></div> : index + 1}</span>
                            <Play size={16} className="hidden group-hover:block text-text-main fill-current absolute" />
                        </div>
                        <div className="flex items-center gap-3 overflow-hidden">
                            <div className="w-10 h-10 flex-shrink-0 rounded bg-surface-3 overflow-hidden relative">
                                    {displayCover ? (
                                    <img src={displayCover} alt={song.album} className="w-full h-full object-cover" />
                                    ) : (
                                    <div className="w-full h-full" style={{ background: generateGradient(song.album) }}></div>
                                    )}
                            </div>
                            <div className="flex flex-col truncate">
                                <span className={`font-medium truncate ${isCurrent ? 'text-brand' : 'text-text-main'}`}>{song.title}</span>
                            </div>
                        </div>
                        <div className="text-text-secondary text-sm truncate">{song.album}</div>
                        <div className="text-text-secondary text-sm truncate">{song.artist}</div>
                        <div className="text-text-secondary text-sm font-mono text-right pr-2">{formatTime(song.duration)}</div>
                        
                        <div className="flex justify-center relative">
                            <button 
                                onClick={(e) => openContextMenu(e, ContextMenuType.SONG, song)}
                                className={`text-text-subtle hover:text-text-main transition-opacity opacity-0 group-hover:opacity-100`}
                            >
                                <MoreHorizontal size={20} />
                            </button>
                        </div>
                    </div>
                </div>
               );
            }}
        />
        
        {sortedAndFilteredSongs.length === 0 && (
             <div className="p-12 text-center text-text-subtle absolute top-40 w-full pointer-events-none">
                <p>No songs found.</p>
             </div>
        )}
    </div>
  );
};