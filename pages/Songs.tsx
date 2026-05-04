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
 * - Empty state for new users
 * - Loading skeleton during initial load
 * 
 * @module Songs
 */

import React, { useState, useEffect, useMemo } from 'react';
import { useStore, useAlbumCovers } from '../store';
import { Play, Clock, MoreHorizontal, Search, ChevronDown, ArrowUpDown, FolderPlus } from 'lucide-react';
import { formatTime, generateGradient } from '../utils';
import { ContextMenuType, Song } from '../types';
import { Virtuoso, Components } from 'react-virtuoso';
import { EmptyLibrary } from '../components/EmptyState';
import { SkeletonTrackList } from '../components/Skeleton';
import { useNavigate } from 'react-router-dom';
import { LikeButton } from '../components/LikeButton';
import { TextInput } from '../components/ui/TextInput';
import { Button } from '../components/ui/Button';
import { Menu, MenuItem } from '../components/ui/Menu';
import { ListHeader } from '../components/ui/Page';

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
      <ListHeader>
            <div className="flex flex-col md:flex-row items-start md:items-center justify-between mb-8 gap-4">
          <h1 className="text-display">All Songs</h1>
                <div className="flex items-center gap-3 w-full md:w-auto">
                    {/* Sort Dropdown */}
                    <div className="relative">
              <Button
                variant="secondary"
                onClick={() => setShowSortMenu(!showSortMenu)}
                className="rounded-full whitespace-nowrap"
                leftIcon={<ArrowUpDown size={16} className="text-text-secondary" aria-hidden="true" />}
                rightIcon={<ChevronDown size={16} className={`text-text-secondary transition-transform ${showSortMenu ? 'rotate-180' : ''}`} aria-hidden="true" />}
                aria-haspopup="menu"
                aria-expanded={showSortMenu}
              >
                <span className="hidden sm:inline">{sortLabels[sortBy]}</span>
              </Button>
                        
                        {showSortMenu && (
                            <>
                                <button
                                  type="button"
                                  tabIndex={-1}
                                  aria-label="Close sort menu"
                                  className="fixed inset-0 z-40"
                                  onClick={() => setShowSortMenu(false)}
                                />
                            <Menu
                              className="absolute right-0 top-full mt-2 z-50 min-w-[180px]"
                              onRequestClose={() => setShowSortMenu(false)}
                            >
                              {(Object.keys(sortLabels) as SongSortOption[]).map((option) => (
                                <MenuItem
                                  key={option}
                                  onClick={() => { setSortBy(option); setShowSortMenu(false); }}
                                  active={sortBy === option}
                                >
                                  {sortLabels[option]}
                                </MenuItem>
                              ))}
                            </Menu>
                            </>
                        )}
                    </div>

                    {/* Search Input */}
                    <div className="flex-1 md:w-72">
                      <TextInput
                        leftIcon={<Search size={18} className="text-text-secondary" aria-hidden="true" />}
                        type="text"
                        placeholder="Find a sound…"
                        aria-label="Search songs"
                        value={filter}
                        onChange={(e) => setFilter(e.target.value)}
                        className="rounded-full"
                      />
                    </div>
                </div>
            </div>

            {/* Table Header */}
            <div className="bg-surface-1 rounded-t-lg sticky top-0 z-10 border-b border-surface-3 grid grid-cols-[44px_1fr_52px_36px] md:grid-cols-[50px_4fr_3fr_3fr_60px_60px_50px] gap-2 md:gap-4 px-4 py-3 text-text-secondary text-xs uppercase tracking-wider font-medium shadow-md">
                <div className="text-center">#</div>
                <div>Title</div>
                <div className="hidden md:block">Album</div>
                <div className="hidden md:block">Artist</div>
                <div className="hidden md:flex justify-center">Plays</div>
                <div className="flex justify-end pr-2"><Clock size={16} /></div>
                <div></div>
            </div>
          </ListHeader>
    );
};

const Footer = () => <div className="h-32 bg-transparent" />;

export const Songs: React.FC = () => {
  const navigate = useNavigate();
  const { songs, playSong, currentSong, isPlaying, openContextMenu } = useStore();
  const isLibraryInitializing = useStore(state => state.isLibraryInitializing);
  const isScanning = useStore(state => state.isScanning);
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
    <div className="h-full animate-fade-in">
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
                <div className="bg-surface-1 px-2 sm:px-4 md:px-8"> {/* Wrapper to match page padding visually for bg */}
                    <div 
                        className={`grid grid-cols-[44px_1fr_52px_36px] md:grid-cols-[50px_4fr_3fr_3fr_60px_60px_40px_50px] gap-2 md:gap-4 px-4 py-3 items-center hover:bg-surface-hover group transition-colors cursor-pointer border-b border-transparent hover:border-surface-highlight ${isCurrent ? 'bg-surface-hover' : 'bg-surface-1'}`}
                        onClick={() => playSong(song, sortedAndFilteredSongs)}
                        onContextMenu={(e) => openContextMenu(e, ContextMenuType.SONG, song)}
                    >
                        <div className="text-center text-text-subtle font-mono text-sm relative h-full flex items-center justify-center">
                        <span className="group-hover:hidden">{isCurrent && isPlaying ? <div className="w-3 h-3 bg-accent-green rounded-full animate-pulse"></div> : index + 1}</span>
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
                              <span className={`font-medium truncate ${isCurrent ? 'text-accent-green' : 'text-text-main'}`}>{song.title}</span>
                              <span className="md:hidden text-xs text-text-subtle truncate mt-0.5">{song.artist}</span>
                            </div>
                        </div>
                        <div className="hidden md:block text-text-secondary text-sm truncate">{song.album}</div>
                        <div className="hidden md:block text-text-secondary text-sm truncate">{song.artist}</div>
                        <div className="hidden md:block text-text-secondary text-sm font-mono text-center">{song.playCount || 0}</div>
                        <div className="text-text-secondary text-sm font-mono text-right pr-2">{formatTime(song.duration)}</div>
                        
                        <div className="hidden md:flex justify-center">
                            <LikeButton songId={song.id} size={18} className="opacity-0 group-hover:opacity-100" />
                        </div>
                        
                        <div className="flex justify-center relative">
                          <Button
                            variant="ghost"
                            className="rounded-full p-2 opacity-0 group-hover:opacity-100"
                            onClick={(e) => {
                              e.stopPropagation();
                              openContextMenu(e, ContextMenuType.SONG, song);
                            }}
                            aria-label="More options"
                            title="More options"
                          >
                            <MoreHorizontal size={20} />
                          </Button>
                        </div>
                    </div>
                </div>
               );
            }}
        />
        
        {songs.length === 0 && !isLibraryInitializing && !isScanning && (
          <div className="absolute inset-0 flex items-center justify-center pt-20">
            <EmptyLibrary onAddMusic={() => navigate('/settings')} />
          </div>
        )}
        
        {songs.length > 0 && sortedAndFilteredSongs.length === 0 && filter && (
          <div className="p-12 text-center text-text-subtle absolute top-40 w-full pointer-events-none">
            <p>No songs match "{filter}"</p>
          </div>
        )}
    </div>
  );
};