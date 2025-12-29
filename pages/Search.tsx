/**
 * ViiB MediaHub - Search Page
 * 
 * Global search across the local music library.
 * 
 * Features:
 * - Real-time search with debounced input
 * - Categorized results: Artists, Albums, Tracks, Playlists
 * - Tab navigation between result categories
 * - Virtualized results list for performance
 * - Play directly from search results
 * - Context menu support for each result
 * - Receives initial query from Home page search bar
 * 
 * @module Search
 */

import React, { useState, useEffect, useMemo } from 'react';
import { Search as SearchIcon, MoreHorizontal, Play, Shuffle, ListPlus, Music } from 'lucide-react';
import { useStore, useAlbumCovers } from '../store';
import { useLocation, useNavigate } from 'react-router-dom';
import { generateGradient, formatTime } from '../utils';
import { ContextMenuType } from '../types';
import { EmptySearchResults } from '../components/EmptyState';
import { TextInput } from '../components/ui/TextInput';
import { Chip } from '../components/ui/Chip';
import { Page, PageHeader } from '../components/ui/Page';

// Search result category tab type
type SearchResultTab = 'all' | 'tracks' | 'albums' | 'artists' | 'playlists';

export const Search: React.FC = () => {
    const location = useLocation();
    const navigate = useNavigate();
    const { 
        songs, playlists, playSong, addToQueue, currentSong, isPlaying, openContextMenu, showToast,
        artistMetadata, fetchArtistMetadata,
        // Persisted search state
        localSearchQuery, localSearchTab, setLocalSearchQuery, setLocalSearchTab
    } = useStore();
    const albumCovers = useAlbumCovers();
    
    // Initialize from persisted state, with location override
    const [inputValue, setInputValue] = useState(localSearchQuery);
    const [debouncedQuery, setDebouncedQuery] = useState(localSearchQuery);
    const [searchResultTab, setSearchResultTabLocal] = useState<SearchResultTab>(localSearchTab);

    // Wrapper to persist tab changes
    const setSearchResultTab = (tab: SearchResultTab) => {
        setSearchResultTabLocal(tab);
        setLocalSearchTab(tab);
    };

    // Album playback handlers
    const handlePlayAlbum = (albumName: string, artistName: string) => {
        const albumSongs = songs.filter(s => s.album === albumName && s.artist === artistName)
            .sort((a, b) => (a.trackNumber || 0) - (b.trackNumber || 0));
        if (albumSongs.length > 0) {
            // playSong with context sets the queue
            playSong(albumSongs[0], albumSongs);
            showToast({ type: 'success', message: `Playing ${albumName}` });
        }
    };

    const handleShuffleAlbum = (albumName: string, artistName: string) => {
        const albumSongs = songs.filter(s => s.album === albumName && s.artist === artistName);
        if (albumSongs.length > 0) {
            const shuffled = [...albumSongs].sort(() => Math.random() - 0.5);
            playSong(shuffled[0], shuffled);
            showToast({ type: 'success', message: `Shuffling ${albumName}` });
        }
    };

    const handleAddAlbumToQueue = (albumName: string, artistName: string) => {
        const albumSongs = songs.filter(s => s.album === albumName && s.artist === artistName)
            .sort((a, b) => (a.trackNumber || 0) - (b.trackNumber || 0));
        albumSongs.forEach(song => addToQueue(song));
        showToast({ type: 'success', message: `Added ${albumSongs.length} tracks to queue` });
    };

    // Handle initial navigation from Home (overrides persisted state)
    useEffect(() => {
        if (location.state && location.state.query) {
            setInputValue(location.state.query);
            setDebouncedQuery(location.state.query);
            setLocalSearchQuery(location.state.query);
        }
    }, [location.state, setLocalSearchQuery]);

    // Debounce Logic + persist query
    useEffect(() => {
        const handler = setTimeout(() => {
            setDebouncedQuery(inputValue);
            setLocalSearchQuery(inputValue);
        }, 300); // 300ms delay

        return () => {
            clearTimeout(handler);
        };
    }, [inputValue, setLocalSearchQuery]);

    // Compute all search results categorized
    const searchResults = useMemo(() => {
        if (!debouncedQuery) {
            return { tracks: [], albums: [], artists: [], playlists: [] };
        }
        
        const lowerQuery = debouncedQuery.toLowerCase();
        
        // Filter tracks
        const tracks = songs.filter(
            s => s.title.toLowerCase().includes(lowerQuery) || 
                 s.artist.toLowerCase().includes(lowerQuery) ||
                 s.album.toLowerCase().includes(lowerQuery)
        );
        
        // Aggregate albums from songs
        const albumMap = new Map<string, { name: string; artist: string; coverUrl?: string; songCount: number }>();
        songs.forEach(song => {
            const key = `${song.album}||${song.artist}`;
            if (song.album.toLowerCase().includes(lowerQuery) || 
                song.artist.toLowerCase().includes(lowerQuery)) {
                if (!albumMap.has(key)) {
                    albumMap.set(key, {
                        name: song.album,
                        artist: song.artist,
                        coverUrl: song.coverUrl || albumCovers[song.album],
                        songCount: 1
                    });
                } else {
                    const existing = albumMap.get(key)!;
                    existing.songCount++;
                    if (!existing.coverUrl && (song.coverUrl || albumCovers[song.album])) {
                        existing.coverUrl = song.coverUrl || albumCovers[song.album];
                    }
                }
            }
        });
        // Filter albums by query match in name
        const albums = Array.from(albumMap.values()).filter(
            a => a.name.toLowerCase().includes(lowerQuery)
        );
        
        // Aggregate artists from songs
        const artistMap = new Map<string, { name: string; songCount: number; albumCount: number }>();
        const artistAlbums = new Map<string, Set<string>>();
        songs.forEach(song => {
            const artistName = song.artist;
            if (artistName.toLowerCase().includes(lowerQuery)) {
                if (!artistMap.has(artistName)) {
                    artistMap.set(artistName, { name: artistName, songCount: 1, albumCount: 0 });
                    artistAlbums.set(artistName, new Set([song.album]));
                } else {
                    const existing = artistMap.get(artistName)!;
                    existing.songCount++;
                    artistAlbums.get(artistName)!.add(song.album);
                }
            }
        });
        // Update album counts
        artistMap.forEach((artist, name) => {
            artist.albumCount = artistAlbums.get(name)?.size || 0;
        });
        const artists = Array.from(artistMap.values());
        
        // Filter playlists
        const filteredPlaylists = playlists.filter(
            p => p.name.toLowerCase().includes(lowerQuery)
        );
        
        return { tracks, albums, artists, playlists: filteredPlaylists };
    }, [songs, playlists, debouncedQuery, albumCovers]);

    // Fetch artist metadata for search results
    useEffect(() => {
        searchResults.artists.forEach(artist => {
            if (!artistMetadata[artist.name]) {
                fetchArtistMetadata(artist.name);
            }
        });
    }, [searchResults.artists, artistMetadata, fetchArtistMetadata]);

    const hasResults = debouncedQuery.length > 0;
    const totalResults = searchResults.tracks.length + searchResults.albums.length + 
                         searchResults.artists.length + searchResults.playlists.length;

    // Get count for each category
    const getCategoryCount = (category: SearchResultTab): number => {
        switch (category) {
            case 'tracks': return searchResults.tracks.length;
            case 'albums': return searchResults.albums.length;
            case 'artists': return searchResults.artists.length;
            case 'playlists': return searchResults.playlists.length;
            case 'all': return totalResults;
            default: return 0;
        }
    };

        return (
        <Page withPlayerPadding={false} className="flex flex-col">
            <PageHeader
              heading="Search"
              className="mb-6 flex-shrink-0"
              titleClassName="text-section mb-0"
            />
            
            <div className="w-full max-w-3xl mb-6 flex-shrink-0">
                <TextInput
                    leftIcon={<SearchIcon size={18} className="text-text-secondary" aria-hidden="true" />}
                    type="text"
                    placeholder="What do you want to feel?"
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
                    {/* Category Tabs */}
                    <div className="flex gap-2 mb-6 flex-wrap flex-shrink-0">
                        {(['all', 'tracks', 'albums', 'artists', 'playlists'] as SearchResultTab[]).map((tab) => (
                            <Chip
                                key={tab}
                                onClick={() => setSearchResultTab(tab)}
                                selected={searchResultTab === tab}
                                accent="brand"
                            >
                                {tab.charAt(0).toUpperCase() + tab.slice(1)}
                                {getCategoryCount(tab) > 0 && ` (${getCategoryCount(tab)})`}
                            </Chip>
                        ))}
                    </div>

                    {totalResults === 0 ? (
                        <EmptySearchResults query={debouncedQuery} />
                    ) : (
                        <div className="flex-1 overflow-y-auto pb-32">
                            {/* Artists Section */}
                            {(searchResultTab === 'all' || searchResultTab === 'artists') && searchResults.artists.length > 0 && (
                                <section className="mb-8">
                                    {searchResultTab === 'all' && <h2 className="text-xl font-bold mb-4">Artists</h2>}
                                    <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6 gap-4">
                                        {(searchResultTab === 'all' ? searchResults.artists.slice(0, 6) : searchResults.artists).map((artist) => {
                                            const metadata = artistMetadata[artist.name];
                                            const imageUrl = metadata?.imageUrl;
                                            
                                            return (
                                                <div 
                                                    key={artist.name}
                                                    onClick={() => navigate(`/artist/${encodeURIComponent(artist.name)}`)}
                                                    className="bg-surface-1 hover:bg-surface-2 p-4 rounded-lg transition-colors group cursor-pointer"
                                                >
                                                    <div className="aspect-square mb-4 rounded-full overflow-hidden bg-surface-3">
                                                        {imageUrl ? (
                                                            <img 
                                                                src={imageUrl} 
                                                                alt={artist.name} 
                                                                className="w-full h-full object-cover"
                                                            />
                                                        ) : (
                                                            <div 
                                                                className="w-full h-full flex items-center justify-center"
                                                                style={{ background: generateGradient(artist.name) }}
                                                            >
                                                                <Music size={40} className="text-white/60" />
                                                            </div>
                                                        )}
                                                    </div>
                                                    <h3 className="font-semibold truncate text-text-main">{artist.name}</h3>
                                                    <p className="text-text-secondary text-sm">
                                                        {artist.songCount} {artist.songCount === 1 ? 'song' : 'songs'} • {artist.albumCount} {artist.albumCount === 1 ? 'album' : 'albums'}
                                                    </p>
                                                </div>
                                            );
                                        })}
                                    </div>
                                    {searchResultTab === 'all' && searchResults.artists.length > 6 && (
                                        <button 
                                            onClick={() => setSearchResultTab('artists')}
                                            className="mt-4 text-sm text-text-secondary hover:text-text-main transition-colors"
                                        >
                                            Show all {searchResults.artists.length} artists →
                                        </button>
                                    )}
                                </section>
                            )}

                            {/* Albums Section */}
                            {(searchResultTab === 'all' || searchResultTab === 'albums') && searchResults.albums.length > 0 && (
                                <section className="mb-8">
                                    {searchResultTab === 'all' && <h2 className="text-xl font-bold mb-4">Albums</h2>}
                                    <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-4">
                                        {(searchResultTab === 'all' ? searchResults.albums.slice(0, 5) : searchResults.albums).map((album) => (
                                            <div 
                                                key={`${album.name}-${album.artist}`}
                                                className="bg-surface-1 hover:bg-surface-2 p-4 rounded-lg transition-colors group relative"
                                            >
                                                <div 
                                                    onClick={() => navigate(`/album/${encodeURIComponent(album.name)}/${encodeURIComponent(album.artist)}`)}
                                                    className="cursor-pointer"
                                                >
                                                    <div className="aspect-square mb-4 relative shadow-lg rounded-md overflow-hidden">
                                                        {album.coverUrl ? (
                                                            <img src={album.coverUrl} alt={album.name} className="w-full h-full object-cover" />
                                                        ) : (
                                                            <div className="w-full h-full" style={{ background: generateGradient(album.name) }}></div>
                                                        )}
                                                        <div className="absolute right-2 bottom-2 flex gap-1 translate-y-4 opacity-0 group-hover:translate-y-0 group-hover:opacity-100 transition-all duration-200">
                                                            <button 
                                                                onClick={(e) => { e.stopPropagation(); handleShuffleAlbum(album.name, album.artist); }}
                                                                className="w-8 h-8 bg-surface-3 hover:bg-surface-hover rounded-full flex items-center justify-center shadow-lg hover:scale-105 text-white" 
                                                                aria-label="Shuffle album"
                                                                title="Shuffle"
                                                            >
                                                                <Shuffle size={14} />
                                                            </button>
                                                            <button 
                                                                onClick={(e) => { e.stopPropagation(); handleAddAlbumToQueue(album.name, album.artist); }}
                                                                className="w-8 h-8 bg-surface-3 hover:bg-surface-hover rounded-full flex items-center justify-center shadow-lg hover:scale-105 text-white" 
                                                                aria-label="Add to queue"
                                                                title="Add to queue"
                                                            >
                                                                <ListPlus size={14} />
                                                            </button>
                                                            <button 
                                                                onClick={(e) => { e.stopPropagation(); handlePlayAlbum(album.name, album.artist); }}
                                                                className="w-10 h-10 bg-brand hover:bg-brand-hover rounded-full flex items-center justify-center shadow-lg hover:scale-105" 
                                                                aria-label="Play album"
                                                            >
                                                                <Play size={16} className="text-black ml-0.5" fill="black" />
                                                            </button>
                                                        </div>
                                                    </div>
                                                    <h3 className="font-semibold truncate text-text-main">{album.name}</h3>
                                                    <p className="text-text-secondary text-sm truncate">{album.artist}</p>
                                                </div>
                                            </div>
                                        ))}
                                    </div>
                                    {searchResultTab === 'all' && searchResults.albums.length > 5 && (
                                        <button 
                                            onClick={() => setSearchResultTab('albums')}
                                            className="mt-4 text-sm text-text-secondary hover:text-text-main transition-colors"
                                        >
                                            Show all {searchResults.albums.length} albums →
                                        </button>
                                    )}
                                </section>
                            )}

                            {/* Playlists Section */}
                            {(searchResultTab === 'all' || searchResultTab === 'playlists') && searchResults.playlists.length > 0 && (
                                <section className="mb-8">
                                    {searchResultTab === 'all' && <h2 className="text-xl font-bold mb-4">Playlists</h2>}
                                    <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-4">
                                        {(searchResultTab === 'all' ? searchResults.playlists.slice(0, 5) : searchResults.playlists).map((playlist) => (
                                            <div 
                                                key={playlist.id}
                                                onClick={() => navigate(`/playlist/${playlist.id}`)}
                                                className="bg-surface-1 hover:bg-surface-2 p-4 rounded-lg transition-colors group cursor-pointer"
                                            >
                                                <div className="aspect-square mb-4 relative shadow-lg rounded-md overflow-hidden">
                                                    {playlist.coverUrl ? (
                                                        <img src={playlist.coverUrl} alt={playlist.name} className="w-full h-full object-cover" />
                                                    ) : (
                                                        <div 
                                                            className="w-full h-full flex items-center justify-center"
                                                            style={{ background: generateGradient(playlist.name) }}
                                                        >
                                                            <ListPlus size={40} className="text-white/60" />
                                                        </div>
                                                    )}
                                                </div>
                                                <h3 className="font-semibold truncate text-text-main">{playlist.name}</h3>
                                                <p className="text-text-secondary text-sm">
                                                    {playlist.songIds.length} {playlist.songIds.length === 1 ? 'track' : 'tracks'}
                                                </p>
                                            </div>
                                        ))}
                                    </div>
                                    {searchResultTab === 'all' && searchResults.playlists.length > 5 && (
                                        <button 
                                            onClick={() => setSearchResultTab('playlists')}
                                            className="mt-4 text-sm text-text-secondary hover:text-text-main transition-colors"
                                        >
                                            Show all {searchResults.playlists.length} playlists →
                                        </button>
                                    )}
                                </section>
                            )}

                            {/* Tracks Section */}
                            {(searchResultTab === 'all' || searchResultTab === 'tracks') && searchResults.tracks.length > 0 && (
                                <section className="mb-8">
                                    {searchResultTab === 'all' && <h2 className="text-xl font-bold mb-4">Tracks</h2>}
                                    <div className="bg-surface-1 rounded-lg overflow-hidden">
                                        {(searchResultTab === 'all' ? searchResults.tracks.slice(0, 8) : searchResults.tracks).map((song, index) => {
                                            const isCurrent = currentSong?.id === song.id;
                                            const displayCover = song.coverUrl || albumCovers[song.album];
                                            
                                            return (
                                                <div 
                                                    key={song.id}
                                                    className={`flex items-center gap-4 px-4 py-3 hover:bg-surface-hover group transition-colors cursor-pointer ${
                                                        isCurrent ? 'bg-surface-hover' : ''
                                                    } ${index > 0 ? 'border-t border-surface-2' : ''}`}
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
                                                        onClick={(e) => { e.stopPropagation(); openContextMenu(e, ContextMenuType.SONG, song); }}
                                                        className="text-text-subtle hover:text-text-main transition-opacity opacity-0 group-hover:opacity-100 p-2"
                                                    >
                                                        <MoreHorizontal size={20} />
                                                    </button>
                                                </div>
                                            );
                                        })}
                                    </div>
                                    {searchResultTab === 'all' && searchResults.tracks.length > 8 && (
                                        <button 
                                            onClick={() => setSearchResultTab('tracks')}
                                            className="mt-4 text-sm text-text-secondary hover:text-text-main transition-colors"
                                        >
                                            Show all {searchResults.tracks.length} tracks →
                                        </button>
                                    )}
                                </section>
                            )}
                        </div>
                    )}
                </div>
            )}
        </Page>
    );
};
