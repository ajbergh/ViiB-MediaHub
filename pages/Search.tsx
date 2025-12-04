/**
 * ViiB MediaHub - Search Page
 * 
 * Global search across the local music library.
 * 
 * Features:
 * - Real-time search with debounced input
 * - Searches across song title, artist, and album
 * - Virtualized results list for performance
 * - Play directly from search results
 * - Context menu support for each result
 * - Receives initial query from Home page search bar
 * 
 * @module Search
 */

import React, { useState, useEffect, useMemo } from 'react';
import { Search as SearchIcon, MoreHorizontal } from 'lucide-react';
import { useStore, useAlbumCovers } from '../store';
import { useLocation } from 'react-router-dom';
import { generateGradient, formatTime } from '../utils';
import { ContextMenuType } from '../types';
import { Virtuoso } from 'react-virtuoso';
import { EmptySearchResults } from '../components/EmptyState';

export const Search: React.FC = () => {
    const location = useLocation();
    const { songs, playSong, currentSong, isPlaying, openContextMenu } = useStore();
    const albumCovers = useAlbumCovers();
    
    // Input state vs Debounced state
    const [inputValue, setInputValue] = useState('');
    const [debouncedQuery, setDebouncedQuery] = useState('');
    const [scrollParent, setScrollParent] = useState<HTMLElement | null>(null);

    useEffect(() => {
        setScrollParent(document.querySelector('main'));
    }, []);

    // Handle initial navigation from Home
    useEffect(() => {
        if (location.state && location.state.query) {
            setInputValue(location.state.query);
            setDebouncedQuery(location.state.query);
        }
    }, [location.state]);

    // Debounce Logic
    useEffect(() => {
        const handler = setTimeout(() => {
            setDebouncedQuery(inputValue);
        }, 300); // 300ms delay

        return () => {
            clearTimeout(handler);
        };
    }, [inputValue]);

    // Memoize the filtering operation
    const filteredSongs = useMemo(() => {
        if (!debouncedQuery) return [];
        const lowerQuery = debouncedQuery.toLowerCase();
        return songs.filter(
            s => s.title.toLowerCase().includes(lowerQuery) || 
                 s.artist.toLowerCase().includes(lowerQuery) ||
                 s.album.toLowerCase().includes(lowerQuery)
        );
    }, [songs, debouncedQuery]);

    const hasResults = debouncedQuery.length > 0;

    return (
        <div className="p-8 h-full flex flex-col animate-fade-in">
            <h1 className="text-3xl font-bold mb-6 flex-shrink-0">Search</h1>
            
            <div className="relative w-full max-w-3xl mb-8 flex-shrink-0">
                <SearchIcon className="absolute left-5 top-1/2 -translate-y-1/2 text-text-secondary" size={22} />
                <input 
                    type="text" 
                    placeholder="What do you want to listen to?"
                    className="w-full bg-surface-highlight hover:bg-surface-hover focus:bg-surface-hover border border-transparent focus:border-surface-slider rounded-full py-4 pl-14 pr-6 text-text-main outline-none transition-all placeholder-text-subtle text-lg shadow-lg"
                    value={inputValue}
                    onChange={(e) => setInputValue(e.target.value)}
                    autoFocus
                />
            </div>

            {!hasResults ? (
                <div className="flex flex-col items-center justify-center mt-12 opacity-50">
                     <SearchIcon size={80} className="mb-4" />
                     <h2 className="text-xl font-bold">Search for music</h2>
                     <p className="text-sm">Find your favorite songs, albums, and artists</p>
                </div>
            ) : (
                <div className="flex-1 flex flex-col min-h-0">
                     <div className="text-text-secondary mb-4 font-medium flex-shrink-0">Found {filteredSongs.length} tracks</div>
                     
                     {filteredSongs.length === 0 ? (
                        <EmptySearchResults query={debouncedQuery} />
                     ) : (
                        <div className="flex-1">
                             <Virtuoso
                                useWindowScroll={false}
                                customScrollParent={scrollParent}
                                data={filteredSongs}
                                totalCount={filteredSongs.length}
                                itemContent={(index, song) => {
                                    const isCurrent = currentSong?.id === song.id;
                                    const displayCover = song.coverUrl || albumCovers[song.album];
                                    
                                    return (
                                        <div 
                                            key={song.id}
                                            className={`flex items-center gap-4 px-4 py-3 rounded-md hover:bg-surface-hover group transition-colors cursor-pointer border-b border-transparent ${isCurrent ? 'bg-surface-hover' : ''}`}
                                            onClick={() => playSong(song)}
                                            onContextMenu={(e) => openContextMenu(e, ContextMenuType.SONG, song)}
                                        >
                                            <div className="w-10 h-10 flex-shrink-0 rounded bg-surface-3 overflow-hidden relative">
                                                {displayCover ? (
                                                    <img src={displayCover} alt={song.album} className="w-full h-full object-cover" />
                                                ) : (
                                                    <div className="w-full h-full" style={{ background: generateGradient(song.album) }}></div>
                                                )}
                                                {isCurrent && isPlaying && (
                                                    <div className="absolute inset-0 bg-black/40 flex items-center justify-center">
                                                        <div className="w-3 h-3 bg-brand rounded-full animate-pulse shadow-[0_0_8px_rgb(29,185,84)]"></div>
                                                    </div>
                                                )}
                                            </div>
                                            
                                            <div className="flex flex-col flex-1 min-w-0">
                                                <span className={`font-medium truncate ${isCurrent ? 'text-brand' : 'text-text-main'}`}>{song.title}</span>
                                                <span className="text-sm text-text-secondary truncate">{song.artist} • {song.album}</span>
                                            </div>
                                            
                                            <div className="text-text-secondary text-sm font-mono">{formatTime(song.duration)}</div>
                                            
                                            <button 
                                                onClick={(e) => openContextMenu(e, ContextMenuType.SONG, song)}
                                                className={`text-text-subtle hover:text-text-main transition-opacity opacity-0 group-hover:opacity-100 p-2`}
                                            >
                                                <MoreHorizontal size={20} />
                                            </button>
                                        </div>
                                    );
                                }}
                                style={{ height: '100%' }}
                                components={{
                                    Footer: () => <div className="h-32" /> // Bottom padding for player
                                }}
                             />
                        </div>
                     )}
                </div>
            )}
        </div>
    );
};
