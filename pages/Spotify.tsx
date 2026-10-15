import { SpotifyAccountNotice } from '../components/SpotifyAccountNotice';
import { savedLibraryDownloadOrigins, savedLibraryDownloadState } from '../lib/spotifyDownloadOrigins';
/**
 * ViiB MediaHub - Spotify Page
 * 
 * Spotify integration hub for browsing, streaming, and downloading from Spotify catalog.
 * 
 * Features:
 * - Backend cookie session connection
 * - Search Spotify catalog (tracks, albums, artists, playlists)
 * - Browse user's saved albums and playlists
 * - View recently played tracks
 * - Queue downloads for tracks, albums, and playlists
 * - Redacted backend session restoration
 * 
 * Requires Spotify Premium for streaming and download functionality.
 * Uses backend Web Player catalog adapters for search/browse and librespot for audio.
 * Recent-history availability depends on the provider capabilities exposed to the session.
 * 
 * @module Spotify
 */

import { appendSpotifyLibraryPage } from '../lib/spotifyLibraryPaging';
import {fetchSpotifyPlaylist} from '../services/spotifyPlaylist';
import {fetchSpotifyAlbum} from '../services/spotifyAlbum';
import { nextSpotifySearchOffset } from '../lib/spotifySearchPaging';
import { SpotifySessionConnect } from '../components/SpotifySessionConnect';
import React, { useEffect, useMemo, useRef, useState } from 'react';
import { SpotifyArtistLinks } from '../components/SpotifyArtistLinks';
import { Link, useNavigate } from 'react-router';
import { LogOut, ExternalLink, CheckCircle, Search as SearchIcon, Loader2, Play, MoreHorizontal, User, Music, Shuffle, ListPlus, Download, Mic2, Disc, ListMusic } from 'lucide-react';
import { formatTime } from '../utils';
import { useStore } from '../store';
import { SpotifyService } from '../services/spotifyService';
import { SpotifyAuthError, SpotifyRateLimitError, SpotifyApiError } from '../lib/spotifyErrors';
import { api, type SpotifyDownloadOrigin } from '../services/api';
import { spotifyTrackToSong, spotifyTracksToSongs, spotifyAlbumToSongs } from '../lib/spotifyHelpers';
import { ContextMenuType } from '../types';
import { Button } from '../components/ui/Button';
import { TextInput } from '../components/ui/TextInput';
import { CardSizeSlider } from '../components/ui/CardSizeSlider';

const getSpotifyProfileWithTimeout = async () => {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 15000);
    try {
        return await SpotifyService.getUserProfile(controller.signal);
    } finally {
        clearTimeout(timeout);
    }
};

export const Spotify: React.FC = () => {
    const accountProduct = useStore(s => s.spotifyAccountProduct);
    const mediaDisabled = useStore(s => !s.spotifyConnected || s.spotifyAccountProduct !== 'premium');
    const navigate = useNavigate();
    const spotifyConnected = useStore(s => s.spotifyConnected);
    const spotifySessionGeneration = useStore(s => s.spotifySessionGeneration);
    const setSpotifyConnected = useStore(s => s.setSpotifyConnected);
    const spotifyUser = useStore(s => s.spotifyUser);
    const logoutSpotify = useStore(s => s.logoutSpotify);
    const setSpotifyUser = useStore(s => s.setSpotifyUser);
    const addLog = useStore(s => s.addLog);
    const playSong = useStore(s => s.playSong);
    const addToQueue = useStore(s => s.addToQueue);
    const showToast = useStore(s => s.showToast);
    const openContextMenu = useStore(s => s.openContextMenu);
    const songs = useStore(s => s.songs);

    // Search & library persistence from store
    const spotifySearchQuery = useStore(s => s.spotifySearchQuery);
    const spotifySearchResults = useStore(s => s.spotifySearchResults);
    const spotifyActiveTab = useStore(s => s.spotifyActiveTab);
    const recentlyPlayed = useStore(s => s.spotifyRecentlyPlayed);
    const savedAlbums = useStore(s => s.spotifySavedAlbums);
    const savedPlaylists = useStore(s => s.spotifySavedPlaylists);
    const setSpotifySearchQuery = useStore(s => s.setSpotifySearchQuery);
    const setSpotifyLastSearchedQuery = useStore(s => s.setSpotifyLastSearchedQuery);
    const setSpotifySearchResults = useStore(s => s.setSpotifySearchResults);
    const setSpotifyActiveTab = useStore(s => s.setSpotifyActiveTab);
    const setRecentlyPlayed = useStore(s => s.setSpotifyRecentlyPlayed);
    const setSavedAlbums = useStore(s => s.setSpotifySavedAlbums);
    const setSavedPlaylists = useStore(s => s.setSpotifySavedPlaylists);

    // Session restoration state - only block the UI if not already connected
    const [isRestoringSession, setIsRestoringSession] = useState(() => !useStore.getState().spotifyConnected);

    // Tab State - initialize from persisted store
    const [activeTab, setActiveTabLocal] = useState<'search' | 'recent' | 'albums' | 'playlists'>(spotifyActiveTab);

    // Search State - initialize from persisted store
    const [inputValue, setInputValue] = useState(spotifySearchQuery);
    const [debouncedQuery, setDebouncedQuery] = useState(spotifySearchQuery);
    const [spotifyResults, setSpotifyResultsLocal] = useState<any>(spotifySearchResults);
    const [isSearching, setIsSearching] = useState(false);
    const [searchError, setSearchError] = useState('');
    const [searchRetry, setSearchRetry] = useState(0);
    const [isLoadingMore, setIsLoadingMore] = useState(false);
    const [hasMore, setHasMore] = useState(() =>
        spotifySearchResults
            ? ['albums', 'playlists', 'tracks', 'artists'].some(category => Boolean(spotifySearchResults[category]?.next))
            : false
    );
    
    // Search result category filter (Tracks, Albums, Artists, Playlists)
    const [searchResultTab, setSearchResultTab] = useState<'tracks' | 'albums' | 'artists' | 'playlists'>('tracks');
    
    // Wrapper to persist tab changes
    const setActiveTab = (tab: 'search' | 'recent' | 'albums' | 'playlists') => {
        setActiveTabLocal(tab);
        setSpotifyActiveTab(tab);
    };
    
    // Wrapper to persist search results
    const setSpotifyResults = (results: any) => {
        const nextResults = typeof results === "function" ? results(useStore.getState().spotifySearchResults) : results;
        setSpotifyResultsLocal(nextResults);
        setSpotifySearchResults(nextResults);
    };

    // Library loading UI state
    const [isLoadingLibrary, setIsLoadingLibrary] = useState(false);
    const [libraryLoadError, setLibraryLoadError] = useState('');
    const [libraryReload, setLibraryReload] = useState(0);

    const [loadingMoreLibrary, setLoadingMoreLibrary] = useState(false);
    const [libraryPageError, setLibraryPageError] = useState('');
    const libraryRequest = useRef(0);
    const libraryBusy = useRef(false);
    useEffect(() => {
        libraryRequest.current++;
        libraryBusy.current = false;
        setLoadingMoreLibrary(false);
        setLibraryPageError('');
        setLibraryLoadError('');
        return () => { libraryRequest.current++; libraryBusy.current = false; };
    }, [activeTab, spotifySessionGeneration]);

    const loadMoreLibrary = async () => {
        if (libraryBusy.current || (activeTab !== 'albums' && activeTab !== 'playlists')) return;
        const kind = activeTab;
        const previous = kind === 'albums' ? savedAlbums : savedPlaylists;
        if (!previous?.next) return;
        const request = ++libraryRequest.current;
        const generation = spotifySessionGeneration;
        const current = () => request === libraryRequest.current && generation === useStore.getState().spotifySessionGeneration && kind === useStore.getState().spotifyActiveTab;
        libraryBusy.current = true;
        setLoadingMoreLibrary(true);
        setLibraryPageError('');
        try {
            const offset = previous.offset + previous.items.length;
            const page = kind === 'albums'
                ? await SpotifyService.getSavedAlbums(20, offset)
                : await SpotifyService.getSavedPlaylists(20, offset);
            if (!current()) return;
            const combined = appendSpotifyLibraryPage(previous, page);
            if (kind === 'albums') setSavedAlbums(combined); else setSavedPlaylists(combined);
        } catch {
            if (current()) setLibraryPageError('Could not load more. Retry, or refresh if your library changed.');
        } finally {
            if (current()) { libraryBusy.current = false; setLoadingMoreLibrary(false); }
        }
    };
    const libraryPagingControls = (page: any) => (
        <div className="flex flex-col items-center gap-3 py-6">
            {libraryPageError && <p role="alert">{libraryPageError}</p>}
            {page?.next && <Button disabled={loadingMoreLibrary} onClick={loadMoreLibrary}>
                {loadingMoreLibrary ? 'Loading more…' : 'Load more'}
            </Button>}
            {libraryPageError && <Button onClick={() => {
                libraryRequest.current++; libraryBusy.current = false;
                setLoadingMoreLibrary(false); setLibraryPageError('');
                if (activeTab === 'albums') setSavedAlbums(null); else setSavedPlaylists(null);
            }}>Refresh library</Button>}
        </div>
    );

    // Download State - track which items are currently being queued
    const [cardCols, setCardCols] = useState(() => Number(localStorage.getItem('spotify-card-cols') ?? 5));
    const handleCardColsChange = (v: number) => { setCardCols(v); localStorage.setItem('spotify-card-cols', String(v)); };
    const [downloadingTracks, setDownloadingTracks] = useState<Set<string>>(new Set());
    const [downloadingAlbums, setDownloadingAlbums] = useState<Set<string>>(new Set());
    const [downloadingPlaylists, setDownloadingPlaylists] = useState<Set<string>>(new Set());
    
    // Downloaded tracks - derived in-memory from Zustand songs without IndexedDB dump
    const downloadedSpotifyIds = useMemo(() => {
        const ids = new Set<string>();
        for (let i = 0; i < songs.length; i++) {
            const sid = songs[i].spotifyId;
            if (sid) ids.add(sid);
        }
        return ids;
    }, [songs]);

    const prevSessionGenerationRef = useRef(spotifySessionGeneration);
    const prevConnectedRef = useRef(spotifyConnected);
    useEffect(() => {
        const genChanged = prevSessionGenerationRef.current !== spotifySessionGeneration;
        const wasConnected = prevConnectedRef.current;
        prevSessionGenerationRef.current = spotifySessionGeneration;
        prevConnectedRef.current = spotifyConnected;
        // Clear cached results when disconnecting or switching accounts, not on route remount
        if (genChanged && (!spotifyConnected || wasConnected)) {
            setRecentlyPlayed(null);
            setSavedAlbums(null);
            setSavedPlaylists(null);
            setSpotifyResultsLocal(null);
            setSpotifySearchResults(null);
            setSpotifyLastSearchedQuery('');
            setIsSearching(false);
            setIsLoadingMore(false);
            setIsLoadingLibrary(false);
            setHasMore(false);
        }
    }, [spotifySessionGeneration, spotifyConnected, setRecentlyPlayed, setSavedAlbums, setSavedPlaylists, setSpotifySearchResults, setSpotifyLastSearchedQuery]);

    useEffect(() => {
        let active = true;
        const generation = useStore.getState().spotifySessionGeneration;
        if (!useStore.getState().spotifyConnected) {
            setIsRestoringSession(true);
        }
        void api.getSpotifyAuthStatus().then(status => {
            if (!active || generation !== useStore.getState().spotifySessionGeneration) return;
            const connected = status.connected && !status.authRequired;
            setSpotifyConnected(connected);
            useStore.getState().setSpotifyAccountProduct(connected ? status.product : undefined);
        }).catch(() => {if(active && generation === useStore.getState().spotifySessionGeneration) setSpotifyConnected(false);}).finally(() => {if(active) setIsRestoringSession(false);});
        return () => {active=false;};
    }, [setSpotifyConnected]);

    useEffect(() => {
        if (!spotifyConnected) return;
        const state = useStore.getState();
        if (state.spotifyUser && state.spotifyCacheGeneration === spotifySessionGeneration) return;
        let active = true;
        const current = () => active && useStore.getState().spotifySessionGeneration === spotifySessionGeneration;
        void getSpotifyProfileWithTimeout().then(profile => {if(current()) setSpotifyUser(profile);}).catch(error => {
            if(current()) addLog('warn', 'Spotify profile unavailable', {status: error?.statusCode});
        });
        return () => {active=false;};
    }, [spotifyConnected, spotifySessionGeneration, setSpotifyUser, addLog]);

    // Debounce Logic
    useEffect(() => {
        const handler = setTimeout(() => {
            setDebouncedQuery(inputValue);
            setSpotifySearchQuery(inputValue); // Persist the search query
        }, 150); // Short typing debounce; Enter submits immediately.

        return () => {
            clearTimeout(handler);
        };
    }, [inputValue, setSpotifySearchQuery]);

    const lastSearchRetryRef = useRef(0);
    // Search Effect
    useEffect(() => {
        let active = true;
        const controller = new AbortController();
        const current = () => active && useStore.getState().spotifySessionGeneration === spotifySessionGeneration;
        const trimmedQuery = debouncedQuery.trim();
        const retryChanged = searchRetry !== lastSearchRetryRef.current;
        lastSearchRetryRef.current = searchRetry;

        if (trimmedQuery && debouncedQuery === inputValue && spotifyConnected && activeTab === 'search') {
            const state = useStore.getState();
            if (
                !retryChanged &&
                state.spotifySearchResults &&
                state.spotifyLastSearchedQuery === trimmedQuery &&
                state.spotifyCacheGeneration === spotifySessionGeneration
            ) {
                setSpotifyResultsLocal(state.spotifySearchResults);
                setHasMore(['albums', 'playlists', 'tracks', 'artists'].some(category => Boolean(state.spotifySearchResults[category]?.next)));
                setIsSearching(false);
                return () => { active = false; controller.abort(); };
            }

            const searchSpotify = async () => {
                setIsSearching(true);
                setSearchError('');
                setSpotifyResults(null);
                setSpotifyLastSearchedQuery('');
                const publishResults = (results: any) => {
                    if (!current()) return;
                    setSpotifyResults(results);
                    setSpotifyLastSearchedQuery(trimmedQuery);
                    setHasMore(['albums', 'playlists', 'tracks', 'artists'].some(category => Boolean(results[category]?.next)));
                    setIsSearching(false);
                };
                try {
                    const results = await SpotifyService.search(debouncedQuery, ['album', 'playlist', 'track', 'artist'], 20, 0, {
                        signal: controller.signal,
                        onCatalogResults: publishResults,
                    });
                    publishResults(results);
                } catch (error) {
                    if (!current()) return;
                    setSearchError(error instanceof SpotifyRateLimitError
                        ? `Spotify is rate limiting search. Try again in ${error.retryAfter} seconds.`
                        : error instanceof SpotifyAuthError
                            ? 'Please reconnect to Spotify to search.'
                            : 'Spotify search failed. Try again.');
                    if (error instanceof SpotifyRateLimitError) {
                        addLog('warn', `Rate limited. Try again in ${error.retryAfter} seconds`);
                    } else if (error instanceof SpotifyAuthError) {
                        addLog('error', 'Authentication failed. Please reconnect to Spotify.');
                    } else if (error instanceof SpotifyApiError) {
                        addLog('error', `Spotify API Error: ${error.message}`);
                    } else {
                        addLog('error', 'Search failed. Please try again.');
                        console.error("Spotify search failed", error);
                    }
                }
                if (current()) setIsSearching(false);
            };
            searchSpotify();
        } else if (!inputValue.trim()) {
            setSpotifyResults(null);
            setSpotifyLastSearchedQuery('');
            setSearchError('');
            setHasMore(false);
            setIsSearching(false);
        } else {
            setIsSearching(spotifyConnected && activeTab === 'search');
        }
        return () => { active = false; controller.abort(); };
    }, [debouncedQuery, inputValue, activeTab, spotifyConnected, spotifySessionGeneration, searchRetry, addLog, logoutSpotify]);

    const handleLoadMore = async () => {
        if (!spotifyConnected || !debouncedQuery || !spotifyResults || isLoadingMore) return;
        const generation = spotifySessionGeneration;

        setIsLoadingMore(true);
        try {
            const currentOffset = nextSpotifySearchOffset(spotifyResults);
            if (currentOffset === undefined) {setHasMore(false);setIsLoadingMore(false);return;}

            const moreResults = await SpotifyService.search(debouncedQuery, ['album', 'playlist', 'track', 'artist'], 20, currentOffset);

            // Merge results
            if (generation !== useStore.getState().spotifySessionGeneration || debouncedQuery !== useStore.getState().spotifySearchQuery) return;
            setSpotifyResults((prev: any) => ({
                albums: prev.albums && moreResults.albums ? {
                    ...moreResults.albums,
                    items: [...prev.albums.items, ...moreResults.albums.items]
                } : prev.albums || moreResults.albums,
                playlists: prev.playlists && moreResults.playlists ? {
                    ...moreResults.playlists,
                    items: [...prev.playlists.items, ...moreResults.playlists.items]
                } : prev.playlists || moreResults.playlists,
                tracks: prev.tracks && moreResults.tracks ? {
                    ...moreResults.tracks,
                    items: [...prev.tracks.items, ...moreResults.tracks.items]
                } : prev.tracks || moreResults.tracks,
                artists: prev.artists && moreResults.artists ? {
                    ...moreResults.artists,
                    items: [...prev.artists.items, ...moreResults.artists.items]
                } : prev.artists || moreResults.artists
            }));

            // Update hasMore
            const hasMoreAlbums = Boolean(moreResults.albums?.next);
            const hasMorePlaylists = Boolean(moreResults.playlists?.next);
            const hasMoreTracks = Boolean(moreResults.tracks?.next);
            const hasMoreArtists = Boolean(moreResults.artists?.next);
            setHasMore(hasMoreAlbums || hasMorePlaylists || hasMoreTracks || hasMoreArtists);
        } catch (error) {
            addLog('error', 'Failed to load more results');
            console.error("Load more failed", error);
        }
        if (generation === useStore.getState().spotifySessionGeneration) setIsLoadingMore(false);
    };

    // Load library data when tabs change
    useEffect(() => {
        if (!spotifyConnected || activeTab === 'search') return;
        const alreadyLoaded =
            (activeTab === 'recent' && Boolean(recentlyPlayed)) ||
            (activeTab === 'albums' && Boolean(savedAlbums)) ||
            (activeTab === 'playlists' && Boolean(savedPlaylists));
        if (alreadyLoaded) {
            setIsLoadingLibrary(false);
            return;
        }

        let active = true;
        const current = () => active && useStore.getState().spotifySessionGeneration === spotifySessionGeneration;

        const loadLibraryData = async () => {
            setIsLoadingLibrary(true);
            setLibraryLoadError('');
            try {
                if (activeTab === 'recent' && !recentlyPlayed) {
                    const data = await SpotifyService.getRecentlyPlayed(50);
                    if (current()) setRecentlyPlayed(data);
                } else if (activeTab === 'albums' && !savedAlbums) {
                    const data = await SpotifyService.getSavedAlbums(20, 0);
                    if (current()) setSavedAlbums(data);
                } else if (activeTab === 'playlists' && !savedPlaylists) {
                    const data = await SpotifyService.getSavedPlaylists(20, 0);
                    if (current()) setSavedPlaylists(data);
                }
            } catch (error) {
                if (!current()) return;
                setLibraryLoadError(error instanceof SpotifyRateLimitError
                    ? `Spotify is rate limiting this request. Try again in ${error.retryAfter} seconds.`
                    : error instanceof SpotifyAuthError
                        ? 'Please reconnect to Spotify to load this view.'
                        : 'Spotify could not load this view. Try again later.');
                if (error instanceof SpotifyRateLimitError) {
                    addLog('warn', `Rate limited. Try again in ${error.retryAfter} seconds`);
                } else if (error instanceof SpotifyAuthError) {
                    addLog('error', 'Authentication failed. Please reconnect to Spotify.');
                } else if (error instanceof SpotifyApiError) {
                    addLog('error', `Spotify API Error: ${error.message}`);
                } else {
                    addLog('error', 'Failed to load library data');
                    console.error('Library data error:', error);
                }
            }
            if (current()) setIsLoadingLibrary(false);
        };

        loadLibraryData();
        return () => { active = false; };
    }, [activeTab, spotifyConnected, spotifySessionGeneration, recentlyPlayed, savedAlbums, savedPlaylists, libraryReload, addLog]);

    const searchTracks = useMemo(
        () => spotifyResults?.tracks?.items?.filter((t: any) => t) ?? [],
        [spotifyResults]
    );
    const recentTracks = useMemo(
        () => recentlyPlayed?.items?.map((i: any) => i?.track).filter(Boolean) ?? [],
        [recentlyPlayed]
    );

    // Backend session logout
    const handleLogout = async () => {
        try {
            await api.disconnectSpotifySession();
            logoutSpotify();
            setSpotifyResults(null);
            setInputValue('');
        } catch {
            showToast({type:'error', message:'Could not disconnect from Spotify. Try again.'});
        }
    };

    const handleDownloadTrack = async (track: any) => {
        if (mediaDisabled) return;
        if (downloadingTracks.has(track.id)) return; // Already downloading

        setDownloadingTracks(prev => new Set(prev).add(track.id));
        try {
            await api.downloadTrack(
                track.id,
                track.name,
                track.artists?.map((a: any) => a.name).join(', ') || 'Unknown Artist',
                track.album?.name || 'Unknown Album',
                Math.floor(track.duration_ms / 1000)
            );
            addLog('success', `Download queued: ${track.name}`);
        } catch (error) {
            addLog('error', `Failed to queue download: ${error instanceof Error ? error.message : 'Unknown error'}`);
        } finally {
            setDownloadingTracks(prev => {
                const newSet = new Set(prev);
                newSet.delete(track.id);
                return newSet;
            });
        }
    };

    const handleDownloadAlbum = async (album: any) => {
        if (mediaDisabled) return;
        if (downloadingAlbums.has(album.id)) return; // Already downloading

        setDownloadingAlbums(prev => new Set(prev).add(album.id));
        try {
            await api.downloadAlbum(
                album.id,
                album.name,
                album.artists?.[0]?.name || 'Unknown Artist'
            );
            addLog('success', `Album download queued: ${album.name}`);
        } catch (error) {
            addLog('error', `Failed to queue album download: ${error instanceof Error ? error.message : 'Unknown error'}`);
        } finally {
            setDownloadingAlbums(prev => {
                const newSet = new Set(prev);
                newSet.delete(album.id);
                return newSet;
            });
        }
    };

    const handleDownloadPlaylist = async (playlist: any) => {
        if (mediaDisabled) return;
        if (downloadingPlaylists.has(playlist.id)) return; // Already downloading

        setDownloadingPlaylists(prev => new Set(prev).add(playlist.id));
        try {
            await api.downloadPlaylist(
                playlist.id,
                playlist.name,
                playlist.owner?.display_name || 'Unknown Owner'
            );
            addLog('success', `Playlist download queued: ${playlist.name}`);
        } catch (error) {
            addLog('error', `Failed to queue playlist download: ${error instanceof Error ? error.message : 'Unknown error'}`);
        } finally {
            setDownloadingPlaylists(prev => {
                const newSet = new Set(prev);
                newSet.delete(playlist.id);
                return newSet;
            });
        }
    };


    // Play a single Spotify track via streaming
    const handlePlayTrack = (track: any, allTracks?: any[]) => {
        if (mediaDisabled) return;
        const song = spotifyTrackToSong(track);
        const context = allTracks ? spotifyTracksToSongs(allTracks) : undefined;
        playSong(song, context);
        addLog('info', ` Streaming: ${track.name}`);
    };

    // Play all search result tracks  
    const handlePlayAllTracks = () => {
        if (mediaDisabled) return;
        if (searchTracks.length > 0) {
            handlePlayTrack(searchTracks[0], searchTracks);
        }
    };

    // Play a Spotify album - fetches full album data then plays all tracks
    const handlePlayAlbum = async (album: any) => {
        if (mediaDisabled) return;
        try {
            addLog('info', `Loading album: ${album.name}...`);

            const fullAlbum = await fetchSpotifyAlbum(album.id);
            const songs = spotifyAlbumToSongs(fullAlbum);

            if (songs.length > 0) {
                playSong(songs[0], songs);
                addLog('info', `▶ Playing album: ${album.name} (${songs.length} tracks)`);
            } else {
                addLog('warn', 'Album has no playable tracks');
            }
        } catch (error) {
            console.error('Failed to play album:', error);
            addLog('error', `Failed to play album: ${error instanceof Error ? error.message : 'Unknown error'}`);
        }
    };

    // Play a Spotify playlist - fetches playlist tracks then plays all
    const handlePlayPlaylist = async (playlist: any) => {
        if (mediaDisabled) return;
        try {
            addLog('info', `Loading playlist: ${playlist.name}...`);

            const fullPlaylist = await fetchSpotifyPlaylist(playlist.id);
            const data = fullPlaylist.tracks;
            const tracks = data.items
                ?.filter((item: any) => item?.track && item.track.id)
                .map((item: any) => item.track);

            if (tracks && tracks.length > 0) {
                const songs = spotifyTracksToSongs(tracks);
                playSong(songs[0], songs);
                addLog('info', `▶ Playing playlist: ${playlist.name} (${songs.length} tracks)`);
            } else {
                addLog('warn', 'Playlist has no playable tracks');
            }
        } catch (error) {
            console.error('Failed to play playlist:', error);
            addLog('error', `Failed to play playlist: ${error instanceof Error ? error.message : 'Unknown error'}`);
        }
    };

    // Shuffle play album
    const handleShuffleAlbum = async (album: any) => {
        if (mediaDisabled) return;
        try {
            addLog('info', `Loading album for shuffle: ${album.name}...`);

            const fullAlbum = await fetchSpotifyAlbum(album.id);
            const songs = spotifyAlbumToSongs(fullAlbum);

            if (songs.length > 0) {
                // Shuffle the songs array
                const shuffled = [...songs].sort(() => Math.random() - 0.5);
                playSong(shuffled[0], shuffled);
                addLog('info', `🔀 Shuffling album: ${album.name} (${songs.length} tracks)`);
            } else {
                addLog('warn', 'Album has no playable tracks');
            }
        } catch (error) {
            console.error('Failed to shuffle album:', error);
            addLog('error', `Failed to shuffle album: ${error instanceof Error ? error.message : 'Unknown error'}`);
        }
    };

    // Shuffle play playlist
    const handleShufflePlaylist = async (playlist: any) => {
        if (mediaDisabled) return;
        try {
            addLog('info', `Loading playlist for shuffle: ${playlist.name}...`);

            const fullPlaylist = await fetchSpotifyPlaylist(playlist.id);
            const data = fullPlaylist.tracks;
            const tracks = data.items
                ?.filter((item: any) => item?.track && item.track.id)
                .map((item: any) => item.track);

            if (tracks && tracks.length > 0) {
                const songs = spotifyTracksToSongs(tracks);
                // Shuffle the songs array
                const shuffled = [...songs].sort(() => Math.random() - 0.5);
                playSong(shuffled[0], shuffled);
                addLog('info', `🔀 Shuffling playlist: ${playlist.name} (${songs.length} tracks)`);
            } else {
                addLog('warn', 'Playlist has no playable tracks');
            }
        } catch (error) {
            console.error('Failed to shuffle playlist:', error);
            addLog('error', `Failed to shuffle playlist: ${error instanceof Error ? error.message : 'Unknown error'}`);
        }
    };

    // Add track to queue
    const handleAddTrackToQueue = (track: any) => {
        if (mediaDisabled) return;
        const song = spotifyTrackToSong(track);
        addToQueue(song);
        showToast({ type: 'success', message: `Added "${track.name}" to queue` });
    };

    // Add album to queue
    const handleAddAlbumToQueue = async (album: any) => {
        if (mediaDisabled) return;
        try {
            addLog('info', `Adding album to queue: ${album.name}...`);

            const fullAlbum = await fetchSpotifyAlbum(album.id);
            const songs = spotifyAlbumToSongs(fullAlbum);

            if (songs.length > 0) {
                addToQueue(songs);
                showToast({ type: 'success', message: `Added ${songs.length} tracks from "${album.name}" to queue` });
            } else {
                addLog('warn', 'Album has no playable tracks');
            }
        } catch (error) {
            console.error('Failed to add album to queue:', error);
            addLog('error', `Failed to add album to queue: ${error instanceof Error ? error.message : 'Unknown error'}`);
        }
    };

    // Add playlist to queue
    const handleAddPlaylistToQueue = async (playlist: any) => {
        if (mediaDisabled) return;
        try {
            addLog('info', `Adding playlist to queue: ${playlist.name}...`);

            const fullPlaylist = await fetchSpotifyPlaylist(playlist.id);
            const data = fullPlaylist.tracks;
            const tracks = data.items
                ?.filter((item: any) => item?.track && item.track.id)
                .map((item: any) => item.track);

            if (tracks && tracks.length > 0) {
                const songs = spotifyTracksToSongs(tracks);
                addToQueue(songs);
                showToast({ type: 'success', message: `Added ${songs.length} tracks from "${playlist.name}" to queue` });
            } else {
                addLog('warn', 'Playlist has no playable tracks');
            }
        } catch (error) {
            console.error('Failed to add playlist to queue:', error);
            addLog('error', `Failed to add playlist to queue: ${error instanceof Error ? error.message : 'Unknown error'}`);
        }
    };

    // Play artist top tracks
    const handlePlayArtistTopTracks = async (artist: any) => {
        if (mediaDisabled) return;
        try {
            addLog('info', `Loading top tracks for: ${artist.name}...`);
            const topTracksData = await SpotifyService.getArtistTopTracks(artist.id);
            
            if (topTracksData?.tracks && topTracksData.tracks.length > 0) {
                const songs = spotifyTracksToSongs(topTracksData.tracks);
                playSong(songs[0], songs);
                addLog('info', `▶ Playing ${artist.name}'s top tracks (${songs.length} tracks)`);
            } else {
                addLog('warn', 'Artist has no top tracks available');
                showToast({ type: 'warning', message: 'No top tracks available for this artist' });
            }
        } catch (error) {
            console.error('Failed to play artist top tracks:', error);
            addLog('error', `Failed to play artist top tracks: ${error instanceof Error ? error.message : 'Unknown error'}`);
        }
    };

    // Shuffle artist top tracks
    const handleShuffleArtistTopTracks = async (artist: any) => {
        if (mediaDisabled) return;
        try {
            addLog('info', `Loading top tracks for shuffle: ${artist.name}...`);
            const topTracksData = await SpotifyService.getArtistTopTracks(artist.id);
            
            if (topTracksData?.tracks && topTracksData.tracks.length > 0) {
                const songs = spotifyTracksToSongs(topTracksData.tracks);
                const shuffled = [...songs].sort(() => Math.random() - 0.5);
                playSong(shuffled[0], shuffled);
                addLog('info', `🔀 Shuffling ${artist.name}'s top tracks (${songs.length} tracks)`);
            } else {
                addLog('warn', 'Artist has no top tracks available');
                showToast({ type: 'warning', message: 'No top tracks available for this artist' });
            }
        } catch (error) {
            console.error('Failed to shuffle artist top tracks:', error);
            addLog('error', `Failed to shuffle artist top tracks: ${error instanceof Error ? error.message : 'Unknown error'}`);
        }
    };

    // Add artist top tracks to queue
    const handleAddArtistToQueue = async (artist: any) => {
        if (mediaDisabled) return;
        try {
            addLog('info', `Adding ${artist.name}'s top tracks to queue...`);
            const topTracksData = await SpotifyService.getArtistTopTracks(artist.id);
            
            if (topTracksData?.tracks && topTracksData.tracks.length > 0) {
                const songs = spotifyTracksToSongs(topTracksData.tracks);
                addToQueue(songs);
                showToast({ type: 'success', message: `Added ${songs.length} top tracks from "${artist.name}" to queue` });
            } else {
                addLog('warn', 'Artist has no top tracks available');
                showToast({ type: 'warning', message: 'No top tracks available for this artist' });
            }
        } catch (error) {
            console.error('Failed to add artist to queue:', error);
            addLog('error', `Failed to add artist to queue: ${error instanceof Error ? error.message : 'Unknown error'}`);
        }
    };

    // Show loading screen while restoring session
    if (isRestoringSession) {
        return (
            <div className="h-full flex flex-col items-center justify-center p-8 text-center">
                <div className="w-24 h-24 bg-brand rounded-full flex items-center justify-center mb-6 shadow-lg shadow-brand/20">
                    <Loader2 size={48} className="text-surface-0 animate-spin" />
                </div>
                <h1 className="text-section font-semibold mb-2">Restoring Session</h1>
                <p className="text-text-secondary">Connecting to Spotify...</p>
            </div>
        );
    }

    if (!spotifyConnected) {
        return (
            <div className="h-full flex flex-col items-center justify-center p-8 text-center">
                <div className="w-24 h-24 bg-brand rounded-full flex items-center justify-center mb-6 shadow-lg shadow-brand/20">
                    <Music size={48} className="text-surface-0" />
                </div>
                <h1 className="text-display mb-4">Connect to Spotify</h1>
                <p className="text-text-secondary max-w-md mb-8">
                    Link your Spotify account to search and play music directly from ViiB MediaHub.
                    Requires a Spotify Premium account for full playback.
                </p>
                <SpotifySessionConnect />
            </div>
        );
    }

    const showCardSizeSlider =
        activeTab === 'albums' ||
        activeTab === 'playlists' ||
        (activeTab === 'search' && Boolean(spotifyResults) && searchResultTab !== 'tracks');

    return (
        <div className="p-8 h-full overflow-y-auto">
            <SpotifyAccountNotice />
            <div className="flex items-center justify-between mb-8">
                <h1 className="text-display flex items-center gap-3">
                    <Music className="text-brand" size={32} />
                    Spotify
                </h1>
                <button
                    onClick={handleLogout}
                    className="flex items-center gap-2 px-4 py-2 bg-surface-2 hover:bg-surface-3 rounded-full text-sm font-bold transition-colors"
                >
                    <LogOut size={16} /> Disconnect
                </button>
            </div>

            <div className="bg-gradient-to-br from-brand/20 to-surface-1 p-6 rounded-2xl border border-brand/30 mb-8">
                <div className="flex items-center gap-6">
                    {spotifyUser?.images && spotifyUser?.images.length > 0 ? (
                        <img
                            src={spotifyUser?.images[0].url}
                            alt={spotifyUser?.display_name}
                            className="w-24 h-24 rounded-full shadow-xl border-4 border-surface-1"
                        />
                    ) : (
                        <div className="w-24 h-24 rounded-full bg-surface-3 flex items-center justify-center border-4 border-surface-1">
                            <User size={40} className="text-text-secondary" />
                        </div>
                    )}

                    <div>
                        <h2 className="text-section font-bold mb-1">{spotifyUser?.display_name || "Spotify account"}</h2>
                        <div className="flex items-center gap-4 text-text-secondary text-sm mb-3">
                            {spotifyUser?.followers && <span>{spotifyUser.followers.total.toLocaleString()} followers</span>}
                            {accountProduct !== 'unknown' && <span className="uppercase">{accountProduct} Plan</span>}
                            {spotifyUser?.country && <span>{spotifyUser.country}</span>}
                        </div>
                        <a
                            href={spotifyUser?.external_urls?.spotify}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="inline-flex items-center gap-1 text-brand hover:text-brand-hover font-bold text-sm transition-all duration-200"
                        >
                            Open in Spotify <ExternalLink size={14} />
                        </a>
                    </div>
                </div>
            </div>

            {/* Tabs */}
            <div className="flex items-center gap-2 mb-8 border-b border-surface-border">
                <div className="flex gap-2 flex-1">
                <Button
                    variant="ghost"
                    onClick={() => setActiveTab('search')}
                    className={`rounded-none px-6 py-3 font-bold text-base transition-all duration-200 border-b-2 hover:bg-transparent ${
                        activeTab === 'search'
                            ? 'border-brand text-brand'
                            : 'border-transparent text-text-secondary hover:text-text-main'
                    }`}
                >
                    <div className="flex items-center gap-2">
                        <SearchIcon size={18} />
                        Search
                    </div>
                </Button>
                <Button
                    variant="ghost"
                    onClick={() => setActiveTab('recent')}
                    className={`rounded-none px-6 py-3 font-bold text-base transition-all duration-200 border-b-2 hover:bg-transparent ${
                        activeTab === 'recent'
                            ? 'border-brand text-brand'
                            : 'border-transparent text-text-secondary hover:text-text-main'
                    }`}
                >
                    Recently Played
                </Button>
                <Button
                    variant="ghost"
                    onClick={() => setActiveTab('albums')}
                    className={`rounded-none px-6 py-3 font-bold text-base transition-all duration-200 border-b-2 hover:bg-transparent ${
                        activeTab === 'albums'
                            ? 'border-brand text-brand'
                            : 'border-transparent text-text-secondary hover:text-text-main'
                    }`}
                >
                    Saved Albums
                </Button>
                <Button
                    variant="ghost"
                    onClick={() => setActiveTab('playlists')}
                    className={`rounded-none px-6 py-3 font-bold text-base transition-all duration-200 border-b-2 hover:bg-transparent ${
                        activeTab === 'playlists'
                            ? 'border-brand text-brand'
                            : 'border-transparent text-text-secondary hover:text-text-main'
                    }`}
                >
                    Saved Playlists
                </Button>
                </div>
                {showCardSizeSlider && <CardSizeSlider value={cardCols} onChange={handleCardColsChange} />}
            </div>

            {/* Search Section */}
            {activeTab === 'search' && (
                <>
                    <div className="mb-6">
                        <div className="w-full max-w-3xl">
                            <TextInput
                                type="text"
                                placeholder="Search Spotify for songs, albums, or playlists..."
                                value={inputValue}
                                onChange={(e) => setInputValue(e.target.value)}
                                onKeyDown={(e) => {
                                    if (e.key === 'Enter') {
                                        setDebouncedQuery(inputValue);
                                        setSpotifySearchQuery(inputValue);
                                    }
                                }}
                                leftIcon={<SearchIcon className="text-text-secondary" size={22} />}
                                rightIcon={
                                    isSearching ? <Loader2 className="animate-spin text-brand" size={20} /> : null
                                }
                                className="w-full rounded-full py-4 px-6 bg-surface-highlight hover:bg-surface-hover focus-within:bg-surface-hover shadow-lg"
                                inputClassName="text-lg placeholder-text-subtle"
                            />
                        </div>
                    </div>

                    {searchError && (
                        <div className="flex flex-col items-center gap-3 py-4">
                            <p role="alert">{searchError}</p>
                            <Button onClick={() => setSearchRetry(value => value + 1)}>Retry search</Button>
                        </div>
                    )}
                    {/* Search Result Category Tabs */}
                    {spotifyResults && (
                        <div className="flex gap-2 mb-6 flex-wrap">
                            <Button
                                onClick={() => setSearchResultTab('tracks')}
                                variant={searchResultTab === 'tracks' ? 'primary' : 'secondary'}
                                className="rounded-full px-4 py-2 text-sm font-medium"
                            >
                                Tracks {spotifyResults.tracks?.items?.length > 0 && `(${spotifyResults.tracks.items.length})`}
                            </Button>
                            <Button
                                onClick={() => setSearchResultTab('albums')}
                                variant={searchResultTab === 'albums' ? 'primary' : 'secondary'}
                                className="rounded-full px-4 py-2 text-sm font-medium"
                            >
                                Albums {spotifyResults.albums?.items?.length > 0 && `(${spotifyResults.albums.items.length})`}
                            </Button>
                            <Button
                                onClick={() => setSearchResultTab('artists')}
                                variant={searchResultTab === 'artists' ? 'primary' : 'secondary'}
                                className="rounded-full px-4 py-2 text-sm font-medium"
                            >
                                Artists {spotifyResults.artists?.items?.length > 0 && `(${spotifyResults.artists.items.length})`}
                            </Button>
                            <Button
                                onClick={() => setSearchResultTab('playlists')}
                                variant={searchResultTab === 'playlists' ? 'primary' : 'secondary'}
                                className="rounded-full px-4 py-2 text-sm font-medium"
                            >
                                Playlists {spotifyResults.playlists?.items?.length > 0 && `(${spotifyResults.playlists.items.length})`}
                            </Button>
                        </div>
                    )}

                    {/* Results */}
                    {spotifyResults ? (
                        <div className="pb-32">
                            {/* Albums - only show when albums tab selected */}
                            {searchResultTab === 'albums' && spotifyResults.albums?.items && Array.isArray(spotifyResults.albums.items) && spotifyResults.albums.items.length > 0 && (
                                <section>
                                    <div className="grid gap-4" style={{ gridTemplateColumns: `repeat(${cardCols}, minmax(0, 1fr))` }}>
                                        {spotifyResults.albums.items.filter((a: any) => a).map((album: any) => (
                                            <div
                                                key={album.id}
                                                className="bg-surface-1 hover:bg-surface-2 p-4 rounded-lg transition-colors group relative cursor-pointer"
                                                onClick={() => navigate(`/spotify/album/${album.id}`)}
                                                role="link"
                                                tabIndex={0}
                                                aria-label={`Open Spotify album ${album.name}`}
                                                onKeyDown={(e) => {
                                                    if (e.key === 'Enter' || e.key === ' ') {
                                                        e.preventDefault();
                                                        navigate(`/spotify/album/${album.id}`);
                                                    }
                                                }}
                                            >
                                                    <div className="aspect-square mb-4 relative shadow-lg rounded-md overflow-hidden">
                                                        {album.images?.[0]?.url ? (
                                                            <img src={album.images[0].url} alt={album.name} className="w-full h-full object-cover" />
                                                        ) : (
                                                            <div className="w-full h-full bg-surface-3 flex items-center justify-center">
                                                                <Music size={40} className="text-text-subtle" />
                                                            </div>
                                                        )}
                                                        <div className="absolute right-2 bottom-2 flex gap-1 translate-y-4 opacity-0 group-hover:translate-y-0 group-hover:opacity-100 transition-all duration-200">
                                                            <button 
                                                                onClick={(e) => { e.stopPropagation(); handleShuffleAlbum(album); }}
                                                                className="w-8 h-8 bg-surface-3 hover:bg-surface-hover rounded-full flex items-center justify-center shadow-lg hover:scale-105 text-text-main disabled:opacity-50 disabled:cursor-not-allowed"
                                                                aria-label="Shuffle album"
                                                                title="Shuffle"
                                                                disabled={mediaDisabled}
                                                            >
                                                                <Shuffle size={14} />
                                                            </button>
                                                            <button 
                                                                onClick={(e) => { e.stopPropagation(); handleAddAlbumToQueue(album); }}
                                                                className="w-8 h-8 bg-surface-3 hover:bg-surface-hover rounded-full flex items-center justify-center shadow-lg hover:scale-105 text-text-main disabled:opacity-50 disabled:cursor-not-allowed"
                                                                aria-label="Add to queue"
                                                                title="Add to queue"
                                                                disabled={mediaDisabled}
                                                            >
                                                                <ListPlus size={14} />
                                                            </button>
                                                            <button 
                                                                onClick={(e) => { e.stopPropagation(); handlePlayAlbum(album); }}
                                                                className="w-10 h-10 bg-brand rounded-full flex items-center justify-center shadow-xl hover:scale-105 text-surface-0 disabled:opacity-50 disabled:cursor-not-allowed"
                                                                aria-label="Play album"
                                                                disabled={mediaDisabled}
                                                            >
                                                                <Play size={20} fill="currentColor" />
                                                            </button>
                                                        </div>
                                                    </div>
                                                    <h3 className="font-bold truncate text-text-main">{album.name}</h3>
                                                    <p className="text-sm text-text-secondary truncate"><SpotifyArtistLinks artists={album.artists || []} /></p>
                                                    <p className="text-xs text-text-subtle mt-1">{album.release_date?.split('-')[0]} • Album</p>
                                                <button
                                                    onClick={(e) => { e.stopPropagation(); handleDownloadAlbum(album); }}
                                                    disabled={mediaDisabled || downloadingAlbums.has(album.id)}
                                                    className="absolute top-2 right-2 p-2 bg-surface-3 hover:bg-brand rounded-full opacity-0 group-hover:opacity-100 transition-all duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
                                                    title={downloadingAlbums.has(album.id) ? "Queueing..." : "Download album"}
                                                    aria-label="Download album"
                                                >
                                                    {downloadingAlbums.has(album.id) ? (
                                                        <Loader2 className="w-4 h-4 text-text-main animate-spin" />
                                                    ) : (
                                                        <Download className="w-4 h-4 text-text-main" />
                                                    )}
                                                </button>
                                            </div>
                                        ))}
                                    </div>
                                </section>
                            )}

                            {/* Artists - only show when artists tab selected */}
                            {searchResultTab === 'artists' && spotifyResults.artists?.items && Array.isArray(spotifyResults.artists.items) && spotifyResults.artists.items.length > 0 && (
                                <section>
                                    <div className="grid gap-4" style={{ gridTemplateColumns: `repeat(${cardCols}, minmax(0, 1fr))` }}>
                                        {spotifyResults.artists.items.filter((a: any) => a).map((artist: any) => (
                                            <div key={artist.id} className="relative bg-surface-1 hover:bg-surface-2 p-4 rounded-lg transition-colors group">
                                                <Link to={`/spotify/artist/${encodeURIComponent(artist.id)}`} aria-label={`View artist ${artist.name}`} className="absolute inset-0 z-10 rounded-lg focus-visible:outline focus-visible:outline-brand" />
                                                <div className="aspect-square mb-4 relative shadow-lg rounded-full overflow-hidden">
                                                    {artist.images?.[0]?.url ? (
                                                        <img src={artist.images[0].url} alt={artist.name} className="w-full h-full object-cover" />
                                                    ) : (
                                                        <div className="w-full h-full bg-surface-3 flex items-center justify-center">
                                                            <Mic2 size={40} className="text-text-subtle" />
                                                        </div>
                                                    )}
                                                    <div className="absolute z-20 right-2 bottom-2 flex gap-1 translate-y-4 opacity-0 group-hover:translate-y-0 group-hover:opacity-100 group-focus-within:translate-y-0 group-focus-within:opacity-100 transition-all duration-200">
                                                        <button 
                                                            onClick={(e) => { e.stopPropagation(); handleShuffleArtistTopTracks(artist); }}
                                                            className="w-8 h-8 bg-surface-3 hover:bg-surface-hover rounded-full flex items-center justify-center shadow-lg hover:scale-105 text-text-main disabled:opacity-50 disabled:cursor-not-allowed"
                                                            aria-label="Shuffle artist's top tracks"
                                                            title="Shuffle top tracks"
                                                            disabled={mediaDisabled}
                                                        >
                                                            <Shuffle size={14} />
                                                        </button>
                                                        <button 
                                                            onClick={(e) => { e.stopPropagation(); handleAddArtistToQueue(artist); }}
                                                            className="w-8 h-8 bg-surface-3 hover:bg-surface-hover rounded-full flex items-center justify-center shadow-lg hover:scale-105 text-text-main disabled:opacity-50 disabled:cursor-not-allowed"
                                                            aria-label="Add artist's top tracks to queue"
                                                            title="Add to queue"
                                                            disabled={mediaDisabled}
                                                        >
                                                            <ListPlus size={14} />
                                                        </button>
                                                        <button 
                                                            onClick={(e) => { e.stopPropagation(); handlePlayArtistTopTracks(artist); }}
                                                            className="w-10 h-10 bg-brand rounded-full flex items-center justify-center shadow-xl hover:scale-105 text-surface-0 disabled:opacity-50 disabled:cursor-not-allowed"
                                                            aria-label="Play artist's top tracks"
                                                            title="Play top tracks"
                                                            disabled={mediaDisabled}
                                                        >
                                                            <Play size={20} fill="currentColor" />
                                                        </button>
                                                    </div>
                                                </div>
                                                <h3 className="font-bold truncate text-text-main text-center">{artist.name}</h3>
                                                <p className="text-sm text-text-secondary truncate text-center mt-1">
                                                    {artist.followers && `${artist.followers.total.toLocaleString()} followers`}
                                                </p>
                                            </div>
                                        ))}
                                    </div>
                                </section>
                            )}

                            {/* Playlists - only show when playlists tab selected */}
                            {searchResultTab === 'playlists' && spotifyResults.playlists?.items && Array.isArray(spotifyResults.playlists.items) && spotifyResults.playlists.items.length > 0 && (
                                <section>
                                    <div className="grid gap-4" style={{ gridTemplateColumns: `repeat(${cardCols}, minmax(0, 1fr))` }}>
                                        {spotifyResults.playlists.items.filter((p: any) => p).map((playlist: any) => (
                                            <div
                                                key={playlist.id}
                                                className="bg-surface-1 hover:bg-surface-2 p-4 rounded-lg transition-colors group relative cursor-pointer"
                                                onClick={() => navigate(`/spotify/playlist/${playlist.id}`)}
                                                role="link"
                                                tabIndex={0}
                                                aria-label={`Open Spotify playlist ${playlist.name || 'Untitled playlist'}`}
                                                onKeyDown={(e) => {
                                                    if (e.key === 'Enter' || e.key === ' ') {
                                                        e.preventDefault();
                                                        navigate(`/spotify/playlist/${playlist.id}`);
                                                    }
                                                }}
                                            >
                                                    <div className="aspect-square mb-4 relative shadow-lg rounded-md overflow-hidden">
                                                        {playlist.images?.[0]?.url ? (
                                                            <img src={playlist.images[0].url} alt={playlist.name} className="w-full h-full object-cover" />
                                                        ) : (
                                                            <div className="w-full h-full bg-surface-3 flex items-center justify-center">
                                                                <Music size={40} className="text-text-subtle" />
                                                            </div>
                                                        )}
                                                        <div className="absolute right-2 bottom-2 flex gap-1 translate-y-4 opacity-0 group-hover:translate-y-0 group-hover:opacity-100 transition-all duration-200">
                                                            <button 
                                                                onClick={(e) => { e.stopPropagation(); handleShufflePlaylist(playlist); }}
                                                                className="w-8 h-8 bg-surface-3 hover:bg-surface-hover rounded-full flex items-center justify-center shadow-lg hover:scale-105 text-text-main disabled:opacity-50 disabled:cursor-not-allowed"
                                                                aria-label="Shuffle playlist"
                                                                title="Shuffle"
                                                                disabled={mediaDisabled}
                                                            >
                                                                <Shuffle size={14} />
                                                            </button>
                                                            <button 
                                                                onClick={(e) => { e.stopPropagation(); handleAddPlaylistToQueue(playlist); }}
                                                                className="w-8 h-8 bg-surface-3 hover:bg-surface-hover rounded-full flex items-center justify-center shadow-lg hover:scale-105 text-text-main disabled:opacity-50 disabled:cursor-not-allowed"
                                                                aria-label="Add to queue"
                                                                title="Add to queue"
                                                                disabled={mediaDisabled}
                                                            >
                                                                <ListPlus size={14} />
                                                            </button>
                                                            <button 
                                                                onClick={(e) => { e.stopPropagation(); handlePlayPlaylist(playlist); }}
                                                                className="w-10 h-10 bg-brand rounded-full flex items-center justify-center shadow-xl hover:scale-105 text-surface-0 disabled:opacity-50 disabled:cursor-not-allowed"
                                                                aria-label="Play playlist"
                                                                disabled={mediaDisabled}
                                                            >
                                                                <Play size={20} fill="currentColor" />
                                                            </button>
                                                        </div>
                                                    </div>
                                                    <h3 className="font-bold truncate text-text-main">{playlist.name || 'Untitled playlist'}</h3>
                                                    <p className="text-sm text-text-secondary truncate">By {playlist.owner?.display_name}</p>
                                                <button
                                                    onClick={(e) => { e.stopPropagation(); handleDownloadPlaylist(playlist); }}
                                                    disabled={mediaDisabled || downloadingPlaylists.has(playlist.id)}
                                                    className="absolute top-2 right-2 p-2 bg-surface-3 hover:bg-brand rounded-full opacity-0 group-hover:opacity-100 transition-all duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
                                                    title={downloadingPlaylists.has(playlist.id) ? "Queueing..." : "Download playlist"}
                                                    aria-label="Download playlist"
                                                >
                                                    {downloadingPlaylists.has(playlist.id) ? (
                                                        <Loader2 className="w-4 h-4 text-text-main animate-spin" />
                                                    ) : (
                                                        <Download className="w-4 h-4 text-text-main" />
                                                    )}
                                                </button>
                                            </div>
                                        ))}
                                    </div>
                                </section>
                            )}

                            {/* Tracks - only show when tracks tab selected */}
                            {searchResultTab === 'tracks' && searchTracks.length > 0 && (
                                <section>
                                    <div className="bg-surface-1 rounded-xl overflow-hidden">
                                        {searchTracks.map((track: any, idx: number) => {
                                            const isDownloaded = downloadedSpotifyIds.has(track.id);
                                            const song = spotifyTrackToSong(track);
                                            return (
                                            <div 
                                                key={track.id} 
                                                className="flex items-center gap-4 p-3 hover:bg-surface-hover group transition-colors border-b border-surface-border last:border-0 cursor-pointer" 
                                                aria-disabled={mediaDisabled}
                                                onClick={mediaDisabled ? undefined : () => handlePlayTrack(track, searchTracks)}
                                                onContextMenu={(e) => {
                                                    e.preventDefault();
                                                    openContextMenu(e, ContextMenuType.SONG, song);
                                                }}
                                            >
                                                <div className="w-8 text-center text-text-subtle text-sm relative"><span className="group-hover:hidden">{idx + 1}</span><Play size={14} className="hidden group-hover:block absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 text-brand fill-current" /></div>
                                                <div 
                                                    className="w-10 h-10 rounded overflow-hidden flex-shrink-0 relative cursor-pointer"
                                                    onClick={(e) => {
                                                        e.stopPropagation();
                                                        if (track.album?.id) {
                                                            navigate(`/spotify/album/${track.album.id}`);
                                                        }
                                                    }}
                                                    title="Go to album"
                                                >
                                                    <img src={track.album?.images?.[2]?.url || track.album?.images?.[0]?.url} alt={track.name} className="w-full h-full object-cover" />
                                                    {isDownloaded && (
                                                        <div className="absolute -bottom-1 -right-1 bg-brand rounded-full p-0.5" title="Downloaded">
                                                            <CheckCircle size={12} className="text-surface-0" />
                                                        </div>
                                                    )}
                                                </div>
                                                <div className="flex-1 min-w-0">
                                                    <div className="font-medium text-text-main truncate group-hover:text-brand transition-all duration-200">{track.name}</div>
                                                    <div className="text-sm text-text-secondary truncate">
                                                        {track.artists?.map((artist: any, i: number) => (
                                                            <span key={artist.id}>
                                                                <Link
                                                                    to={`/spotify/artist/${encodeURIComponent(artist.id)}`}
                                                                    className="hover:underline"
                                                                    onClick={(e) => e.stopPropagation()}
                                                                >
                                                                    {artist.name}
                                                                </Link>
                                                                {i < track.artists.length - 1 && ', '}
                                                            </span>
                                                        ))}
                                                    </div>
                                                </div>
                                                <div className="text-sm text-text-subtle font-mono">{formatTime(track.duration_ms / 1000)}</div>
                                                
                                                {/* Add to Queue button */}
                                                <button
                                                    onClick={(e) => { e.stopPropagation(); handleAddTrackToQueue(track); }}
                                                    className="p-2 text-text-subtle hover:text-text-main opacity-0 group-hover:opacity-100 transition-opacity disabled:opacity-50 disabled:cursor-not-allowed"
                                                    title="Add to queue"
                                                    disabled={mediaDisabled}
                                                >
                                                    <ListPlus size={16} />
                                                </button>
                                                
                                                {/* Download button */}
                                                {isDownloaded ? (
                                                    <div className="p-2 text-brand opacity-0 group-hover:opacity-100 transition-opacity" title="Downloaded - will play locally">
                                                        <CheckCircle size={16} />
                                                    </div>
                                                ) : (
                                                <button
                                                    onClick={(e) => { e.stopPropagation(); handleDownloadTrack(track); }}
                                                    disabled={mediaDisabled || downloadingTracks.has(track.id)}
                                                    className="p-2 text-text-subtle hover:text-text-main opacity-0 group-hover:opacity-100 transition-opacity disabled:opacity-50 disabled:cursor-not-allowed"
                                                    title={downloadingTracks.has(track.id) ? "Queueing..." : "Download for offline"}
                                                >
                                                    {downloadingTracks.has(track.id) ? (
                                                        <Loader2 className="w-4 h-4 animate-spin" />
                                                    ) : (
                                                        <Download size={16} />
                                                    )}
                                                </button>
                                                )}
                                            </div>
                                        );
                                        })}
                                    </div>
                                </section>
                            )}

                            {/* Empty state for selected tab */}
                            {searchResultTab === 'tracks' && !spotifyResults.tracks?.items?.length && (
                                <div className="text-center p-10 text-text-subtle">
                                    No tracks found for "{debouncedQuery}"
                                </div>
                            )}
                            {searchResultTab === 'albums' && !spotifyResults.albums?.items?.length && (
                                <div className="text-center p-10 text-text-subtle">
                                    No albums found for "{debouncedQuery}"
                                </div>
                            )}
                            {searchResultTab === 'artists' && !spotifyResults.artists?.items?.length && (
                                <div className="text-center p-10 text-text-subtle">
                                    No artists found for "{debouncedQuery}"
                                </div>
                            )}
                            {searchResultTab === 'playlists' && !spotifyResults.playlists?.items?.length && (
                                <div className="text-center p-10 text-text-subtle">
                                    No playlists found for "{debouncedQuery}"
                                </div>
                            )}

                            {/* Load More Button */}
                            {hasMore && (spotifyResults.albums?.items?.length || spotifyResults.playlists?.items?.length || spotifyResults.tracks?.items?.length || spotifyResults.artists?.items?.length) && (
                                <div className="flex justify-center mt-8">
                                    <button
                                        onClick={handleLoadMore}
                                        disabled={isLoadingMore}
                                        className="bg-surface-2 hover:bg-surface-3 disabled:opacity-50 disabled:cursor-not-allowed text-text-main font-bold py-3 px-8 rounded-full transition-all flex items-center gap-2"
                                    >
                                        {isLoadingMore ? (
                                            <>
                                                <Loader2 className="animate-spin" size={20} />
                                                Loading...
                                            </>
                                        ) : (
                                            'Load More'
                                        )}
                                    </button>
                                </div>
                            )}
                        </div>
                    ) : (
                        <div className="flex flex-col items-center justify-center py-20 opacity-50">
                            <SearchIcon size={64} className="mb-4 text-text-subtle" />
                            <h3 className="text-xl font-bold text-text-secondary">Search Spotify</h3>
                            <p className="text-text-subtle mt-2">Find your favorite music on Spotify</p>
                        </div>
                    )}
                </>
            )}

            {activeTab !== 'search' && libraryLoadError && (
                <div className="py-4 flex flex-col items-center gap-3">
                    <p role="alert">{libraryLoadError}</p>
                    <Button onClick={() => setLibraryReload(value => value + 1)}>Retry library request</Button>
                </div>
            )}

            {/* Recently Played Tab */}
            {activeTab === 'recent' && (
                <div>
                    {libraryLoadError ? null : isLoadingLibrary ? (
                        <div className="flex items-center justify-center py-20">
                            <Loader2 className="animate-spin text-brand" size={48} />
                        </div>
                    ) : recentlyPlayed?.items && recentlyPlayed.items.length > 0 ? (
                        <div className="bg-surface-1 rounded-xl overflow-hidden">
                            {recentlyPlayed.items.map((item: any, idx: number) => {
                                const isDownloaded = downloadedSpotifyIds.has(item.track.id);
                                return (
                                <div 
                                    key={`${item.track.id}-${idx}`} 
                                    className="flex items-center gap-4 p-3 hover:bg-surface-hover group transition-all duration-200 border-b border-surface-border last:border-0 cursor-pointer"
                                    aria-disabled={mediaDisabled}
                                    onClick={mediaDisabled ? undefined : () => handlePlayTrack(item.track, recentTracks)}
                                    onContextMenu={(e) => {
                                        e.preventDefault();
                                        const song = spotifyTrackToSong(item.track);
                                        openContextMenu(e, ContextMenuType.SONG, song);
                                    }}
                                >
                                    <div className="w-8 text-center text-text-subtle text-sm relative">
                                        <span className="group-hover:hidden">{idx + 1}</span>
                                        <Play size={14} className="hidden group-hover:block absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 text-brand fill-current" />
                                    </div>
                                    <div className="w-10 h-10 rounded overflow-hidden flex-shrink-0 relative">
                                        <img src={item.track.album?.images?.[2]?.url || item.track.album?.images?.[0]?.url} alt={item.track.name} className="w-full h-full object-cover" />
                                        {isDownloaded && (
                                            <div className="absolute -bottom-1 -right-1 bg-brand rounded-full p-0.5" title="Downloaded">
                                                <CheckCircle size={12} className="text-surface-0" />
                                            </div>
                                        )}
                                    </div>
                                    <div className="flex-1 min-w-0">
                                        <div className="font-medium text-text-main truncate group-hover:text-brand transition-all duration-200">
                                            {item.track.name}
                                        </div>
                                        <div className="text-sm text-text-secondary truncate"><SpotifyArtistLinks artists={item.track.artists || []} /></div>
                                    </div>
                                    <div 
                                        className="text-sm text-text-subtle hover:text-brand hover:underline cursor-pointer"
                                        onClick={(e) => { e.stopPropagation(); navigate(`/spotify/album/${item.track.album.id}`); }}
                                    >
                                        {item.track.album.name}
                                    </div>
                                    <div className="text-sm text-text-subtle font-mono">{formatTime(item.track.duration_ms / 1000)}</div>
                                    
                                    {/* Add to Queue button */}
                                    <button
                                        onClick={(e) => { e.stopPropagation(); handleAddTrackToQueue(item.track); }}
                                        className="p-2 text-text-subtle hover:text-text-main opacity-0 group-hover:opacity-100 transition-opacity disabled:opacity-50 disabled:cursor-not-allowed"
                                        title="Add to queue"
                                        disabled={mediaDisabled}
                                    >
                                        <ListPlus size={16} />
                                    </button>
                                    
                                    {/* Download button */}
                                    {isDownloaded ? (
                                        <div className="p-2 text-brand opacity-0 group-hover:opacity-100 transition-opacity" title="Downloaded">
                                            <CheckCircle size={16} />
                                        </div>
                                    ) : (
                                        <button
                                            onClick={(e) => { e.stopPropagation(); handleDownloadTrack(item.track); }}
                                            disabled={mediaDisabled || downloadingTracks.has(item.track.id)}
                                            className="p-2 text-text-subtle hover:text-text-main opacity-0 group-hover:opacity-100 transition-opacity disabled:opacity-50"
                                            title="Download for offline"
                                        >
                                            {downloadingTracks.has(item.track.id) ? (
                                                <Loader2 className="w-4 h-4 animate-spin" />
                                            ) : (
                                                <Download size={16} />
                                            )}
                                        </button>
                                    )}
                                    
                                    <button 
                                        onClick={(e) => { 
                                            e.stopPropagation();
                                            const song = spotifyTrackToSong(item.track);
                                            openContextMenu(e, ContextMenuType.SONG, song);
                                        }}
                                        className="p-2 text-text-subtle hover:text-text-main opacity-0 group-hover:opacity-100 transition-opacity"
                                    >
                                        <MoreHorizontal size={18} />
                                    </button>
                                </div>
                            );
                            })}
                        </div>
                    ) : (
                        <div className="text-center py-20 text-text-subtle">
                            <Music size={64} className="mx-auto mb-4 opacity-50" />
                            <p>No recently played tracks</p>
                        </div>
                    )}
                </div>
            )}

            {/* Saved Albums Tab */}
            {activeTab === 'albums' && (
                <div>
                    {libraryLoadError ? null : isLoadingLibrary ? (
                        <div className="flex items-center justify-center py-20">
                            <Loader2 className="animate-spin text-brand" size={48} />
                        </div>
                    ) : savedAlbums?.items && savedAlbums.items.length > 0 ? (
                        <div className="grid gap-4" style={{ gridTemplateColumns: `repeat(${cardCols}, minmax(0, 1fr))` }}>
                            {savedAlbums.items.filter((item: any) => item?.album?.id).map((item: any, index: number) => (
                                <div
                                    key={`${item.album.id}:${index}`}
                                    onClick={() => navigate(`/spotify/album/${item.album.id}`, { state: savedLibraryDownloadState('saved_albums', item.album.id, spotifySessionGeneration) })}
                                    className="bg-surface-1 hover:bg-surface-2 p-4 rounded-lg transition-all duration-200 group relative cursor-pointer"
                                    role="link"
                                    tabIndex={0}
                                    aria-label={`Open Spotify album ${item.album.name}`}
                                    onKeyDown={(e) => {
                                        if (e.key === 'Enter' || e.key === ' ') {
                                            e.preventDefault();
                                            navigate(`/spotify/album/${item.album.id}`, { state: savedLibraryDownloadState('saved_albums', item.album.id, spotifySessionGeneration) });
                                        }
                                    }}
                                >
                                    <div className="aspect-square mb-4 relative shadow-lg rounded-md overflow-hidden">
                                        {item.album.images?.[0]?.url ? (
                                            <img src={item.album.images[0].url} alt={item.album.name} className="w-full h-full object-cover" />
                                        ) : (
                                            <div className="w-full h-full bg-surface-3 flex items-center justify-center">
                                                <Music size={40} className="text-text-subtle" />
                                            </div>
                                        )}
                                        <div className="absolute right-2 bottom-2 flex gap-1 translate-y-4 opacity-0 group-hover:translate-y-0 group-hover:opacity-100 transition-all duration-200">
                                            <button
                                                onClick={(e) => { e.stopPropagation(); handleShuffleAlbum(item.album); }}
                                                className="w-8 h-8 bg-surface-3 hover:bg-surface-hover rounded-full flex items-center justify-center shadow-lg hover:scale-105 text-text-main disabled:opacity-50 disabled:cursor-not-allowed"
                                                aria-label="Shuffle album"
                                                title="Shuffle"
                                                disabled={mediaDisabled}
                                            >
                                                <Shuffle size={14} />
                                            </button>
                                            <button
                                                onClick={(e) => { e.stopPropagation(); handleAddAlbumToQueue(item.album); }}
                                                className="w-8 h-8 bg-surface-3 hover:bg-surface-hover rounded-full flex items-center justify-center shadow-lg hover:scale-105 text-text-main disabled:opacity-50 disabled:cursor-not-allowed"
                                                aria-label="Add to queue"
                                                title="Add to queue"
                                                disabled={mediaDisabled}
                                            >
                                                <ListPlus size={14} />
                                            </button>
                                            <button
                                                onClick={(e) => { e.stopPropagation(); handlePlayAlbum(item.album); }}
                                                className="w-10 h-10 bg-brand rounded-full flex items-center justify-center shadow-xl hover:scale-105 text-surface-0 disabled:opacity-50 disabled:cursor-not-allowed"
                                                aria-label="Play album"
                                                disabled={mediaDisabled}
                                            >
                                                <Play size={20} fill="currentColor" />
                                            </button>
                                        </div>
                                    </div>
                                    <h3 className="font-bold truncate text-text-main">{item.album.name}</h3>
                                    <p className="text-sm text-text-secondary truncate"><SpotifyArtistLinks artists={item.album.artists || []} /></p>
                                    <p className="text-xs text-text-subtle mt-1">{item.album.release_date?.split('-')[0]} • {item.album.total_tracks} tracks</p>
                                    <button
                                        onClick={(e) => { e.stopPropagation(); handleDownloadAlbum(item.album); }}
                                        disabled={mediaDisabled || downloadingAlbums.has(item.album.id)}
                                        className="absolute top-2 right-2 p-2 bg-surface-3 hover:bg-brand rounded-full opacity-0 group-hover:opacity-100 transition-all duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
                                        title={downloadingAlbums.has(item.album.id) ? "Queueing..." : "Download album"}
                                        aria-label="Download album"
                                    >
                                        {downloadingAlbums.has(item.album.id) ? (
                                            <Loader2 className="w-4 h-4 text-text-main animate-spin" />
                                        ) : (
                                            <Download className="w-4 h-4 text-text-main" />
                                        )}
                                    </button>
                                </div>
                            ))}
                        </div>
                    ) : (
                        <div className="text-center py-20 text-text-subtle">
                            <Music size={64} className="mx-auto mb-4 opacity-50" />
                            <p>No saved albums</p>
                        </div>
                    )}
                    {libraryPagingControls(savedAlbums)}
                </div>
            )}

            {/* Saved Playlists Tab */}
            {activeTab === 'playlists' && (
                <div>
                    {libraryLoadError ? null : isLoadingLibrary ? (
                        <div className="flex items-center justify-center py-20">
                            <Loader2 className="animate-spin text-brand" size={48} />
                        </div>
                    ) : savedPlaylists?.items && savedPlaylists.items.length > 0 ? (
                        <div className="grid gap-4" style={{ gridTemplateColumns: `repeat(${cardCols}, minmax(0, 1fr))` }}>
                            {savedPlaylists.items.filter((playlist: any) => playlist?.id).map((playlist: any, index: number) => (
                                <div
                                    key={`${playlist.id}:${index}`}
                                    onClick={() => navigate(`/spotify/playlist/${playlist.id}`, { state: savedLibraryDownloadState('saved_playlists', playlist.id, spotifySessionGeneration) })}
                                    className="bg-surface-1 hover:bg-surface-2 p-4 rounded-lg transition-all duration-200 group relative cursor-pointer"
                                    role="link"
                                    tabIndex={0}
                                    aria-label={`Open Spotify playlist ${playlist.name || 'Untitled playlist'}`}
                                    onKeyDown={(e) => {
                                        if (e.key === 'Enter' || e.key === ' ') {
                                            e.preventDefault();
                                            navigate(`/spotify/playlist/${playlist.id}`, { state: savedLibraryDownloadState('saved_playlists', playlist.id, spotifySessionGeneration) });
                                        }
                                    }}
                                >
                                    <div className="aspect-square mb-4 relative shadow-lg rounded-md overflow-hidden">
                                        {playlist.images?.[0]?.url ? (
                                            <img src={playlist.images[0].url} alt={playlist.name} className="w-full h-full object-cover" />
                                        ) : (
                                            <div className="w-full h-full bg-surface-3 flex items-center justify-center">
                                                <Music size={40} className="text-text-subtle" />
                                            </div>
                                        )}
                                        <div className="absolute right-2 bottom-2 flex gap-1 translate-y-4 opacity-0 group-hover:translate-y-0 group-hover:opacity-100 transition-all duration-200">
                                            <button
                                                onClick={(e) => { e.stopPropagation(); handleShufflePlaylist(playlist); }}
                                                className="w-8 h-8 bg-surface-3 hover:bg-surface-hover rounded-full flex items-center justify-center shadow-lg hover:scale-105 text-text-main disabled:opacity-50 disabled:cursor-not-allowed"
                                                aria-label="Shuffle playlist"
                                                title="Shuffle"
                                                disabled={mediaDisabled}
                                            >
                                                <Shuffle size={14} />
                                            </button>
                                            <button
                                                onClick={(e) => { e.stopPropagation(); handleAddPlaylistToQueue(playlist); }}
                                                className="w-8 h-8 bg-surface-3 hover:bg-surface-hover rounded-full flex items-center justify-center shadow-lg hover:scale-105 text-text-main disabled:opacity-50 disabled:cursor-not-allowed"
                                                aria-label="Add to queue"
                                                title="Add to queue"
                                                disabled={mediaDisabled}
                                            >
                                                <ListPlus size={14} />
                                            </button>
                                            <button
                                                onClick={(e) => { e.stopPropagation(); handlePlayPlaylist(playlist); }}
                                                className="w-10 h-10 bg-brand rounded-full flex items-center justify-center shadow-xl hover:scale-105 text-surface-0 disabled:opacity-50 disabled:cursor-not-allowed"
                                                aria-label="Play playlist"
                                                disabled={mediaDisabled}
                                            >
                                                <Play size={20} fill="currentColor" />
                                            </button>
                                        </div>
                                    </div>
                                    <h3 className="font-bold truncate text-text-main">{playlist.name || 'Untitled playlist'}</h3>
                                    <p className="text-sm text-text-secondary truncate">By {playlist.owner?.display_name}</p>
                                    <p className="text-xs text-text-subtle mt-1">{playlist.tracks.total} tracks</p>
                                    <button
                                        onClick={(e) => { e.stopPropagation(); handleDownloadPlaylist(playlist); }}
                                        disabled={mediaDisabled || downloadingPlaylists.has(playlist.id)}
                                        className="absolute top-2 right-2 p-2 bg-surface-3 hover:bg-brand rounded-full opacity-0 group-hover:opacity-100 transition-all duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
                                        title={downloadingPlaylists.has(playlist.id) ? "Queueing..." : "Download playlist"}
                                        aria-label="Download playlist"
                                    >
                                        {downloadingPlaylists.has(playlist.id) ? (
                                            <Loader2 className="w-4 h-4 text-text-main animate-spin" />
                                        ) : (
                                            <Download className="w-4 h-4 text-text-main" />
                                        )}
                                    </button>
                                </div>
                            ))}
                        </div>
                    ) : (
                        <div className="text-center py-20 text-text-subtle">
                            <Music size={64} className="mx-auto mb-4 opacity-50" />
                            <p>No saved playlists</p>
                        </div>
                    )}
                    {libraryPagingControls(savedPlaylists)}
                </div>
            )}
        </div>
    );
};
