/**
 * ViiB MediaHub - Spotify Page
 * 
 * Spotify integration hub for browsing, streaming, and downloading from Spotify catalog.
 * 
 * Features:
 * - OAuth login with PKCE flow
 * - Search Spotify catalog (tracks, albums, artists, playlists)
 * - Browse user's saved albums and playlists
 * - View recently played tracks
 * - Queue downloads for tracks, albums, and playlists
 * - Session restoration from cached tokens
 * 
 * Requires Spotify Premium for streaming and download functionality.
 * Uses Web API for search/browse, librespot for streaming and downloads.
 * 
 * @module Spotify
 */

import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Wifi, LogOut, ExternalLink, CheckCircle, Search as SearchIcon, Loader2, Play, MoreHorizontal, User, Music, Shuffle, ListPlus, Download, Mic2 } from 'lucide-react';
import { formatTime, getOAuthCallbackUrl, isWailsEnvironment } from '../utils';
import { useStore } from '../store';
import { SpotifyService } from '../services/spotifyService';
import { SpotifyAuthError, SpotifyRateLimitError, SpotifyApiError } from '../lib/spotifyErrors';
import { api } from '../services/api';
import { libraryService } from '../services/libraryService';
import { spotifyTrackToSong, spotifyTracksToSongs, spotifyAlbumToSongs } from '../lib/spotifyHelpers';
import { ContextMenuType } from '../types';

export const Spotify: React.FC = () => {
    const navigate = useNavigate();
    const {
        spotifyClientId, spotifyClientSecret, spotifyUser,
        spotifyAccessToken, spotifyRefreshToken, spotifyTokenExpiry,
        logoutSpotify, setSpotifyTokens, setSpotifyUser, addLog,
        playSong, addToQueue, showToast, openContextMenu,
        // Search persistence from store
        spotifySearchQuery, spotifySearchResults, spotifyActiveTab,
        setSpotifySearchQuery, setSpotifySearchResults, setSpotifyActiveTab
    } = useStore();

    // Session restoration state
    const [isRestoringSession, setIsRestoringSession] = useState(false);

    // Tab State - initialize from persisted store
    const [activeTab, setActiveTabLocal] = useState<'search' | 'recent' | 'albums' | 'playlists'>(spotifyActiveTab);

    // Search State - initialize from persisted store
    const [inputValue, setInputValue] = useState(spotifySearchQuery);
    const [debouncedQuery, setDebouncedQuery] = useState(spotifySearchQuery);
    const [spotifyResults, setSpotifyResultsLocal] = useState<any>(spotifySearchResults);
    const [isSearching, setIsSearching] = useState(false);
    const [isLoadingMore, setIsLoadingMore] = useState(false);
    const [hasMore, setHasMore] = useState(false);
    
    // Search result category filter (Tracks, Albums, Artists, Playlists)
    const [searchResultTab, setSearchResultTab] = useState<'tracks' | 'albums' | 'artists' | 'playlists'>('tracks');
    
    // Wrapper to persist tab changes
    const setActiveTab = (tab: 'search' | 'recent' | 'albums' | 'playlists') => {
        setActiveTabLocal(tab);
        setSpotifyActiveTab(tab);
    };
    
    // Wrapper to persist search results
    const setSpotifyResults = (results: any) => {
        setSpotifyResultsLocal(results);
        setSpotifySearchResults(results);
    };

    // Library State
    const [recentlyPlayed, setRecentlyPlayed] = useState<any>(null);
    const [savedAlbums, setSavedAlbums] = useState<any>(null);
    const [savedPlaylists, setSavedPlaylists] = useState<any>(null);
    const [isLoadingLibrary, setIsLoadingLibrary] = useState(false);

    // Download State - track which items are currently being queued
    const [downloadingTracks, setDownloadingTracks] = useState<Set<string>>(new Set());
    const [downloadingAlbums, setDownloadingAlbums] = useState<Set<string>>(new Set());
    const [downloadingPlaylists, setDownloadingPlaylists] = useState<Set<string>>(new Set());
    
    // Downloaded tracks - Set of Spotify IDs that have been downloaded locally
    const [downloadedSpotifyIds, setDownloadedSpotifyIds] = useState<Set<string>>(new Set());

    // Load downloaded Spotify track IDs on mount
    useEffect(() => {
        const loadDownloadedIds = async () => {
            try {
                const allSongs = await libraryService.getAllSongs();
                const spotifyIds = allSongs
                    .filter(song => song.spotifyId)
                    .map(song => song.spotifyId!);
                setDownloadedSpotifyIds(new Set(spotifyIds));
                console.log('[Spotify] Loaded downloaded track IDs:', spotifyIds.length);
            } catch (error) {
                console.error('[Spotify] Failed to load downloaded track IDs:', error);
            }
        };
        loadDownloadedIds();
    }, []);

    useEffect(() => {
        const handleMessage = (event: MessageEvent) => {
            if (event.origin !== window.location.origin) return;

            if (event.data?.type === 'SPOTIFY_AUTH_SUCCESS') {
                const { accessToken, refreshToken, expiry, user } = event.data;
                setSpotifyTokens(accessToken, refreshToken, expiry);
                setSpotifyUser(user);
                addLog('success', `Logged in as ${user.display_name}`);
            }
        };

        window.addEventListener('message', handleMessage);
        return () => window.removeEventListener('message', handleMessage);
    }, [setSpotifyTokens, setSpotifyUser, addLog]);

    // Session restoration effect - runs once on mount to restore cached session
    useEffect(() => {
        const restoreSession = async () => {
            // Check if we have cached tokens to validate
            if (!spotifyAccessToken || !spotifyRefreshToken) {
                console.log('[Spotify] No cached tokens found');
                // If we have a stale user without tokens, clear it
                if (spotifyUser) {
                    console.log('[Spotify] Clearing stale user without tokens');
                    logoutSpotify();
                }
                return;
            }

            // If we already have a user and token is not expired, assume valid for now
            // This provides a faster initial load - token will be validated on first API call anyway
            if (spotifyUser && spotifyTokenExpiry && Date.now() < spotifyTokenExpiry) {
                console.log('[Spotify] Session appears valid (token not expired), skipping validation');
                return;
            }

            console.log('[Spotify] Validating cached session...');
            setIsRestoringSession(true);

            try {
                // Try to get a valid access token (will refresh if expired)
                const accessToken = await SpotifyService.getAccessToken();

                if (!accessToken) {
                    console.log('[Spotify] Could not get valid access token');
                    logoutSpotify();
                    setIsRestoringSession(false);
                    return;
                }

                // Fetch user profile to validate token and restore/update user
                const userProfile = await SpotifyService.getUserProfile();

                if (userProfile) {
                    console.log('[Spotify] Session validated successfully for:', userProfile.display_name);
                    setSpotifyUser(userProfile);
                    addLog('success', `Session restored for ${userProfile.display_name}`);

                    // Sync refreshed tokens to backend
                    try {
                        const currentState = useStore.getState();
                        await api.saveSpotifyCredentials({
                            clientId: currentState.spotifyClientId || '',
                            clientSecret: currentState.spotifyClientSecret || '',
                            accessToken: currentState.spotifyAccessToken || '',
                            refreshToken: currentState.spotifyRefreshToken || '',
                            expiry: currentState.spotifyTokenExpiry || 0,
                        });
                    } catch (e) {
                        console.warn('[Spotify] Failed to sync tokens to backend:', e);
                    }
                } else {
                    console.log('[Spotify] Failed to fetch user profile');
                    logoutSpotify();
                }
            } catch (error) {
                console.error('[Spotify] Session restoration failed:', error);
                // Clear invalid cached credentials
                logoutSpotify();
            } finally {
                setIsRestoringSession(false);
            }
        };

        restoreSession();
    }, []); // Empty dependency array - runs once on mount

    // Debounce Logic
    useEffect(() => {
        const handler = setTimeout(() => {
            setDebouncedQuery(inputValue);
            setSpotifySearchQuery(inputValue); // Persist the search query
        }, 500); // 500ms delay for API calls

        return () => {
            clearTimeout(handler);
        };
    }, [inputValue, setSpotifySearchQuery]);

    // Search Effect
    useEffect(() => {
        if (debouncedQuery && spotifyUser) {
            const searchSpotify = async () => {
                setIsSearching(true);
                try {
                    const results = await SpotifyService.search(debouncedQuery, ['album', 'playlist', 'track', 'artist'], 20, 0);
                    setSpotifyResults(results);

                    // Check if there are more results
                    const hasMoreAlbums = results.albums?.next !== null;
                    const hasMorePlaylists = results.playlists?.next !== null;
                    const hasMoreTracks = results.tracks?.next !== null;
                    const hasMoreArtists = results.artists?.next !== null;
                    setHasMore(hasMoreAlbums || hasMorePlaylists || hasMoreTracks || hasMoreArtists);
                } catch (error) {
                    if (error instanceof SpotifyRateLimitError) {
                        addLog('warn', `Rate limited. Try again in ${error.retryAfter} seconds`);
                    } else if (error instanceof SpotifyAuthError) {
                        addLog('error', 'Authentication failed. Please reconnect to Spotify.');
                        logoutSpotify();
                    } else if (error instanceof SpotifyApiError) {
                        addLog('error', `Spotify API Error: ${error.message}`);
                    } else {
                        addLog('error', 'Search failed. Please try again.');
                        console.error("Spotify search failed", error);
                    }
                }
                setIsSearching(false);
            };
            searchSpotify();
        } else if (!debouncedQuery) {
            setSpotifyResults(null);
            setHasMore(false);
        }
    }, [debouncedQuery, spotifyUser, addLog, logoutSpotify]);

    const handleLoadMore = async () => {
        if (!debouncedQuery || !spotifyResults || isLoadingMore) return;

        setIsLoadingMore(true);
        try {
            const currentOffset = spotifyResults.albums?.items?.length ||
                spotifyResults.playlists?.items?.length ||
                spotifyResults.tracks?.items?.length ||
                spotifyResults.artists?.items?.length || 0;

            const moreResults = await SpotifyService.search(debouncedQuery, ['album', 'playlist', 'track', 'artist'], 20, currentOffset);

            // Merge results
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
            const hasMoreAlbums = moreResults.albums?.next !== null;
            const hasMorePlaylists = moreResults.playlists?.next !== null;
            const hasMoreTracks = moreResults.tracks?.next !== null;
            const hasMoreArtists = moreResults.artists?.next !== null;
            setHasMore(hasMoreAlbums || hasMorePlaylists || hasMoreTracks || hasMoreArtists);
        } catch (error) {
            addLog('error', 'Failed to load more results');
            console.error("Load more failed", error);
        }
        setIsLoadingMore(false);
    };

    // Load library data when tabs change
    useEffect(() => {
        if (!spotifyUser) return;

        const loadLibraryData = async () => {
            setIsLoadingLibrary(true);
            try {
                if (activeTab === 'recent' && !recentlyPlayed) {
                    const data = await SpotifyService.getRecentlyPlayed(50);
                    setRecentlyPlayed(data);
                } else if (activeTab === 'albums' && !savedAlbums) {
                    const data = await SpotifyService.getSavedAlbums(20, 0);
                    setSavedAlbums(data);
                } else if (activeTab === 'playlists' && !savedPlaylists) {
                    const data = await SpotifyService.getSavedPlaylists(20, 0);
                    setSavedPlaylists(data);
                }
            } catch (error) {
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
            setIsLoadingLibrary(false);
        };

        if (activeTab !== 'search') {
            loadLibraryData();
        }
    }, [activeTab, spotifyUser, recentlyPlayed, savedAlbums, savedPlaylists, addLog]);

    // Polling state for Wails cross-origin auth
    const [isWaitingForAuth, setIsWaitingForAuth] = useState(false);
    const pollIntervalRef = React.useRef<NodeJS.Timeout | null>(null);

    // Poll backend for auth completion (for Wails cross-origin popup)
    useEffect(() => {
        if (!isWaitingForAuth) return;
        
        const pollForAuth = async () => {
            try {
                const creds = await api.getSpotifyCredentials();
                if (creds && creds.accessToken && creds.refreshToken) {
                    console.log('[Spotify] Auth detected via backend polling');
                    
                    // Update Zustand with tokens from backend
                    setSpotifyTokens(creds.accessToken, creds.refreshToken, creds.expiry || Date.now() + 3600000);
                    
                    // Fetch user profile
                    try {
                        const profile = await SpotifyService.getUserProfile();
                        if (profile) {
                            setSpotifyUser(profile);
                            addLog('success', `Logged in as ${profile.display_name}`);
                            showToast({ type: 'success', message: `Connected as ${profile.display_name}` });
                        }
                    } catch (e) {
                        console.error('[Spotify] Failed to fetch profile after auth:', e);
                    }
                    
                    setIsWaitingForAuth(false);
                }
            } catch (e) {
                console.error('[Spotify] Poll error:', e);
            }
        };
        
        // Poll every 2 seconds
        pollIntervalRef.current = setInterval(pollForAuth, 2000);
        
        return () => {
            if (pollIntervalRef.current) {
                clearInterval(pollIntervalRef.current);
                pollIntervalRef.current = null;
            }
        };
    }, [isWaitingForAuth, setSpotifyTokens, setSpotifyUser, addLog, showToast]);

    const handleLogin = async () => {
        if (!spotifyClientId || !spotifyClientSecret) {
            alert("Please configure your Client ID and Client Secret in Settings first.");
            return;
        }

        // Get the proper callback URL (handles Wails environment)
        const redirectUri = await getOAuthCallbackUrl();
        
        // For standard web builds on localhost, prompt user to use 127.0.0.1
        if (!isWailsEnvironment() && window.location.hostname === 'localhost') {
            alert("Please access this app via http://127.0.0.1:3000 instead of localhost to comply with Spotify's new security requirements.");
            window.location.href = window.location.href.replace('localhost', '127.0.0.1');
            return;
        }

        addLog('info', 'Initiating Spotify Login', { redirectUri, clientId: spotifyClientId });

        const { url, codeVerifier } = await SpotifyService.generateAuthUrl(spotifyClientId, redirectUri);

        // For Wails builds, save credentials to backend BEFORE opening popup
        // This allows the cross-origin popup to fetch them
        if (isWailsEnvironment()) {
            try {
                const preSaveData = {
                    clientId: spotifyClientId,
                    clientSecret: spotifyClientSecret,
                    accessToken: '',
                    refreshToken: '',
                    expiry: 0,
                    codeVerifier: codeVerifier // Save verifier for cross-origin callback
                };
                console.log('[Spotify] Pre-saving credentials to backend:', JSON.stringify(preSaveData));
                await api.saveSpotifyCredentials(preSaveData);
                console.log('[Spotify] Pre-saved credentials and verifier to backend for popup');
            } catch (e) {
                console.error('[Spotify] Failed to pre-save credentials:', e);
                addLog('error', 'Failed to save credentials');
                return;
            }
        }

        // Store verifier for the callback
        localStorage.setItem('spotify_code_verifier', codeVerifier);
        // Also store the redirect URI for the callback to use
        localStorage.setItem('spotify_redirect_uri', redirectUri);

        addLog('info', 'Opening Spotify Auth Popup', { url });

        // Start polling for auth completion (for Wails)
        if (isWailsEnvironment()) {
            setIsWaitingForAuth(true);
        }

        // Open popup
        const width = 600;
        const height = 800;
        const left = window.screen.width / 2 - width / 2;
        const top = window.screen.height / 2 - height / 2;
        window.open(url, 'Spotify Auth', `width=${width},height=${height},left=${left},top=${top}`);
    };

    const handleLogout = () => {
        logoutSpotify();
        setIsWaitingForAuth(false);
        setSpotifyResults(null);
        setInputValue('');
    };

    const handleDownloadTrack = async (track: any) => {
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
        const song = spotifyTrackToSong(track);
        const context = allTracks ? spotifyTracksToSongs(allTracks) : undefined;
        playSong(song, context);
        addLog('info', ` Streaming: ${track.name}`);
    };

    // Play all search result tracks  
    const handlePlayAllTracks = () => {
        const tracks = spotifyResults?.tracks?.items?.filter((t: any) => t);
        if (tracks && tracks.length > 0) {
            handlePlayTrack(tracks[0], tracks);
        }
    };

    // Play a Spotify album - fetches full album data then plays all tracks
    const handlePlayAlbum = async (album: any) => {
        try {
            addLog('info', `Loading album: ${album.name}...`);
            const token = await SpotifyService.getAccessToken();
            if (!token) {
                addLog('error', 'Not authenticated with Spotify');
                return;
            }

            const response = await fetch(`https://api.spotify.com/v1/albums/${album.id}`, {
                headers: { 'Authorization': `Bearer ${token}` }
            });

            if (!response.ok) {
                throw new Error(`Failed to fetch album: ${response.statusText}`);
            }

            const fullAlbum = await response.json();
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
        try {
            addLog('info', `Loading playlist: ${playlist.name}...`);
            const token = await SpotifyService.getAccessToken();
            if (!token) {
                addLog('error', 'Not authenticated with Spotify');
                return;
            }

            const response = await fetch(`https://api.spotify.com/v1/playlists/${playlist.id}/tracks?limit=100`, {
                headers: { 'Authorization': `Bearer ${token}` }
            });

            if (!response.ok) {
                throw new Error(`Failed to fetch playlist: ${response.statusText}`);
            }

            const data = await response.json();
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
        try {
            addLog('info', `Loading album for shuffle: ${album.name}...`);
            const token = await SpotifyService.getAccessToken();
            if (!token) {
                addLog('error', 'Not authenticated with Spotify');
                return;
            }

            const response = await fetch(`https://api.spotify.com/v1/albums/${album.id}`, {
                headers: { 'Authorization': `Bearer ${token}` }
            });

            if (!response.ok) {
                throw new Error(`Failed to fetch album: ${response.statusText}`);
            }

            const fullAlbum = await response.json();
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
        try {
            addLog('info', `Loading playlist for shuffle: ${playlist.name}...`);
            const token = await SpotifyService.getAccessToken();
            if (!token) {
                addLog('error', 'Not authenticated with Spotify');
                return;
            }

            const response = await fetch(`https://api.spotify.com/v1/playlists/${playlist.id}/tracks?limit=100`, {
                headers: { 'Authorization': `Bearer ${token}` }
            });

            if (!response.ok) {
                throw new Error(`Failed to fetch playlist: ${response.statusText}`);
            }

            const data = await response.json();
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
        const song = spotifyTrackToSong(track);
        addToQueue(song);
        showToast({ type: 'success', message: `Added "${track.name}" to queue` });
    };

    // Add album to queue
    const handleAddAlbumToQueue = async (album: any) => {
        try {
            addLog('info', `Adding album to queue: ${album.name}...`);
            const token = await SpotifyService.getAccessToken();
            if (!token) {
                addLog('error', 'Not authenticated with Spotify');
                return;
            }

            const response = await fetch(`https://api.spotify.com/v1/albums/${album.id}`, {
                headers: { 'Authorization': `Bearer ${token}` }
            });

            if (!response.ok) {
                throw new Error(`Failed to fetch album: ${response.statusText}`);
            }

            const fullAlbum = await response.json();
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
        try {
            addLog('info', `Adding playlist to queue: ${playlist.name}...`);
            const token = await SpotifyService.getAccessToken();
            if (!token) {
                addLog('error', 'Not authenticated with Spotify');
                return;
            }

            const response = await fetch(`https://api.spotify.com/v1/playlists/${playlist.id}/tracks?limit=100`, {
                headers: { 'Authorization': `Bearer ${token}` }
            });

            if (!response.ok) {
                throw new Error(`Failed to fetch playlist: ${response.statusText}`);
            }

            const data = await response.json();
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
                    <Loader2 size={48} className="text-black animate-spin" />
                </div>
                <h1 className="text-2xl font-bold mb-2">Restoring Session</h1>
                <p className="text-text-secondary">Connecting to Spotify...</p>
            </div>
        );
    }

    if (!spotifyUser) {
        return (
            <div className="h-full flex flex-col items-center justify-center p-8 text-center">
                <div className="w-24 h-24 bg-brand rounded-full flex items-center justify-center mb-6 shadow-lg shadow-brand/20">
                    <Music size={48} className="text-black" />
                </div>
                <h1 className="text-3xl font-bold mb-4">Connect to Spotify</h1>
                <p className="text-text-secondary max-w-md mb-8">
                    Link your Spotify account to search and play music directly from ViiB MediaHub.
                    Requires a Spotify Premium account for full playback.
                </p>
                <button
                    onClick={handleLogin}
                    disabled={!spotifyClientId || !spotifyClientSecret}
                    className="bg-brand hover:bg-brand-hover disabled:opacity-50 disabled:cursor-not-allowed text-black font-bold py-3 px-8 rounded-full transition-all duration-200 transform hover:scale-105 shadow-lg flex items-center gap-2"
                >
                    <Wifi size={20} /> Connect Spotify
                </button>

                {(!spotifyClientId || !spotifyClientSecret) && (
                    <div className="mt-8 w-full max-w-lg bg-surface-2 border border-warning/30 rounded-xl p-6 relative overflow-hidden text-left">
                        <div className="absolute top-0 left-0 w-1 h-full bg-warning"></div>
                        <h4 className="text-warning font-bold text-sm uppercase tracking-wide mb-3 flex items-center gap-2">
                            Configuration Required
                        </h4>
                        <div className="text-sm text-text-secondary space-y-3">
                            <p>To enable integration:</p>
                            <ol className="list-decimal list-inside space-y-2 ml-1">
                                <li>Create a Spotify App at <a href="https://developer.spotify.com/dashboard" target="_blank" rel="noreferrer" className="text-green-500 hover:underline">developer.spotify.com</a></li>
                                <li>
                                    Go to <span className="text-white font-bold">Settings</span> in this app and enter both your
                                    <span className="text-white font-mono bg-surface-3 px-1 rounded mx-1">Client ID</span>
                                    and
                                    <span className="text-white font-mono bg-surface-3 px-1 rounded mx-1">Client Secret</span>.
                                </li>
                                <li>Add this Redirect URI in your Spotify Dashboard:</li>
                            </ol>
                            <div className="mt-2 bg-surface-1 p-3 rounded font-mono text-xs text-gray-400 break-all select-all border border-surface-3">
                                {window.location.origin}/callback
                            </div>
                        </div>
                    </div>
                )}
            </div>
        );
    }

    return (
        <div className="p-8 h-full overflow-y-auto">
            <div className="flex items-center justify-between mb-8">
                <h1 className="text-3xl font-bold flex items-center gap-3">
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
                    {spotifyUser.images && spotifyUser.images.length > 0 ? (
                        <img
                            src={spotifyUser.images[0].url}
                            alt={spotifyUser.display_name}
                            className="w-24 h-24 rounded-full shadow-xl border-4 border-surface-1"
                        />
                    ) : (
                        <div className="w-24 h-24 rounded-full bg-surface-3 flex items-center justify-center border-4 border-surface-1">
                            <User size={40} className="text-text-secondary" />
                        </div>
                    )}

                    <div>
                        <h2 className="text-2xl font-bold mb-1">{spotifyUser.display_name}</h2>
                        <div className="flex items-center gap-4 text-text-secondary text-sm mb-3">
                            <span>{spotifyUser.followers?.total.toLocaleString()} followers</span>
                            <span>•</span>
                            <span className="uppercase">{spotifyUser.product} Plan</span>
                            <span>•</span>
                            <span>{spotifyUser.country}</span>
                        </div>
                        <a
                            href={spotifyUser.external_urls.spotify}
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
            <div className="flex gap-2 mb-8 border-b border-surface-border">
                <button
                    onClick={() => setActiveTab('search')}
                    className={`px-6 py-3 font-bold transition-all duration-200 border-b-2 ${activeTab === 'search'
                            ? 'border-brand text-brand'
                            : 'border-transparent text-text-secondary hover:text-text-main'
                        }`}
                >
                    <div className="flex items-center gap-2">
                        <SearchIcon size={18} />
                        Search
                    </div>
                </button>
                <button
                    onClick={() => setActiveTab('recent')}
                    className={`px-6 py-3 font-bold transition-all duration-200 border-b-2 ${activeTab === 'recent'
                            ? 'border-brand text-brand'
                            : 'border-transparent text-text-secondary hover:text-text-main'
                        }`}
                >
                    Recently Played
                </button>
                <button
                    onClick={() => setActiveTab('albums')}
                    className={`px-6 py-3 font-bold transition-all duration-200 border-b-2 ${activeTab === 'albums'
                            ? 'border-brand text-brand'
                            : 'border-transparent text-text-secondary hover:text-text-main'
                        }`}
                >
                    Saved Albums
                </button>
                <button
                    onClick={() => setActiveTab('playlists')}
                    className={`px-6 py-3 font-bold transition-all duration-200 border-b-2 ${activeTab === 'playlists'
                            ? 'border-brand text-brand'
                            : 'border-transparent text-text-secondary hover:text-text-main'
                        }`}
                >
                    Saved Playlists
                </button>
            </div>

            {/* Search Section */}
            {activeTab === 'search' && (
                <>
                    <div className="mb-6">
                        <div className="relative w-full max-w-3xl">
                            <SearchIcon className="absolute left-5 top-1/2 -translate-y-1/2 text-text-secondary" size={22} />
                            <input
                                type="text"
                                placeholder="Search Spotify for songs, albums, or playlists..."
                                className="w-full bg-surface-highlight hover:bg-surface-hover focus:bg-surface-hover border border-transparent focus:border-brand rounded-full py-4 pl-14 pr-6 text-text-main outline-none transition-all duration-200 placeholder-text-subtle text-lg shadow-lg"
                                value={inputValue}
                                onChange={(e) => setInputValue(e.target.value)}
                            />
                            {isSearching && (
                                <div className="absolute right-5 top-1/2 -translate-y-1/2">
                                    <Loader2 className="animate-spin text-brand" size={20} />
                                </div>
                            )}
                        </div>
                    </div>

                    {/* Search Result Category Tabs */}
                    {spotifyResults && (
                        <div className="flex gap-2 mb-6 flex-wrap">
                            <button
                                onClick={() => setSearchResultTab('tracks')}
                                className={`px-4 py-2 rounded-full font-medium text-sm transition-all ${
                                    searchResultTab === 'tracks'
                                        ? 'bg-brand text-black'
                                        : 'bg-surface-2 text-text-secondary hover:bg-surface-3 hover:text-text-main'
                                }`}
                            >
                                Tracks {spotifyResults.tracks?.items?.length > 0 && `(${spotifyResults.tracks.items.length})`}
                            </button>
                            <button
                                onClick={() => setSearchResultTab('albums')}
                                className={`px-4 py-2 rounded-full font-medium text-sm transition-all ${
                                    searchResultTab === 'albums'
                                        ? 'bg-brand text-black'
                                        : 'bg-surface-2 text-text-secondary hover:bg-surface-3 hover:text-text-main'
                                }`}
                            >
                                Albums {spotifyResults.albums?.items?.length > 0 && `(${spotifyResults.albums.items.length})`}
                            </button>
                            <button
                                onClick={() => setSearchResultTab('artists')}
                                className={`px-4 py-2 rounded-full font-medium text-sm transition-all ${
                                    searchResultTab === 'artists'
                                        ? 'bg-brand text-black'
                                        : 'bg-surface-2 text-text-secondary hover:bg-surface-3 hover:text-text-main'
                                }`}
                            >
                                Artists {spotifyResults.artists?.items?.length > 0 && `(${spotifyResults.artists.items.length})`}
                            </button>
                            <button
                                onClick={() => setSearchResultTab('playlists')}
                                className={`px-4 py-2 rounded-full font-medium text-sm transition-all ${
                                    searchResultTab === 'playlists'
                                        ? 'bg-brand text-black'
                                        : 'bg-surface-2 text-text-secondary hover:bg-surface-3 hover:text-text-main'
                                }`}
                            >
                                Playlists {spotifyResults.playlists?.items?.length > 0 && `(${spotifyResults.playlists.items.length})`}
                            </button>
                        </div>
                    )}

                    {/* Results */}
                    {spotifyResults ? (
                        <div className="pb-32">
                            {/* Albums - only show when albums tab selected */}
                            {searchResultTab === 'albums' && spotifyResults.albums?.items && Array.isArray(spotifyResults.albums.items) && spotifyResults.albums.items.length > 0 && (
                                <section>
                                    <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-4">
                                        {spotifyResults.albums.items.filter((a: any) => a).map((album: any) => (
                                            <div key={album.id} className="bg-surface-1 hover:bg-surface-2 p-4 rounded-lg transition-colors group relative">
                                                <div onClick={() => navigate(`/spotify/album/${album.id}`)} className="cursor-pointer">
                                                    <div className="aspect-square mb-4 relative shadow-lg rounded-md overflow-hidden">
                                                        <img src={album.images?.[0]?.url} alt={album.name} className="w-full h-full object-cover" />
                                                        <div className="absolute right-2 bottom-2 flex gap-1 translate-y-4 opacity-0 group-hover:translate-y-0 group-hover:opacity-100 transition-all duration-200">
                                                            <button 
                                                                onClick={(e) => { e.stopPropagation(); handleShuffleAlbum(album); }}
                                                                className="w-8 h-8 bg-surface-3 hover:bg-surface-hover rounded-full flex items-center justify-center shadow-lg hover:scale-105 text-white" 
                                                                aria-label="Shuffle album"
                                                                title="Shuffle"
                                                            >
                                                                <Shuffle size={14} />
                                                            </button>
                                                            <button 
                                                                onClick={(e) => { e.stopPropagation(); handleAddAlbumToQueue(album); }}
                                                                className="w-8 h-8 bg-surface-3 hover:bg-surface-hover rounded-full flex items-center justify-center shadow-lg hover:scale-105 text-white" 
                                                                aria-label="Add to queue"
                                                                title="Add to queue"
                                                            >
                                                                <ListPlus size={14} />
                                                            </button>
                                                            <button 
                                                                onClick={(e) => { e.stopPropagation(); handlePlayAlbum(album); }}
                                                                className="w-10 h-10 bg-brand rounded-full flex items-center justify-center shadow-xl hover:scale-105 text-black" 
                                                                aria-label="Play album"
                                                            >
                                                                <Play size={20} fill="black" />
                                                            </button>
                                                        </div>
                                                    </div>
                                                    <h3 className="font-bold truncate text-text-main">{album.name}</h3>
                                                    <p className="text-sm text-text-secondary truncate">{album.artists?.map((a: any) => a.name).join(', ')}</p>
                                                    <p className="text-xs text-text-subtle mt-1">{album.release_date?.split('-')[0]} • Album</p>
                                                </div>
                                                <button
                                                    onClick={(e) => { e.stopPropagation(); handleDownloadAlbum(album); }}
                                                    disabled={downloadingAlbums.has(album.id)}
                                                    className="absolute top-2 right-2 p-2 bg-surface-3 hover:bg-brand rounded-full opacity-0 group-hover:opacity-100 transition-all duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
                                                    title={downloadingAlbums.has(album.id) ? "Queueing..." : "Download album"}
                                                    aria-label="Download album"
                                                >
                                                    {downloadingAlbums.has(album.id) ? (
                                                        <Loader2 className="w-4 h-4 text-white animate-spin" />
                                                    ) : (
                                                        <Download className="w-4 h-4 text-white" />
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
                                    <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-4">
                                        {spotifyResults.artists.items.filter((a: any) => a).map((artist: any) => (
                                            <div key={artist.id} className="bg-surface-1 hover:bg-surface-2 p-4 rounded-lg transition-colors group">
                                                <div className="aspect-square mb-4 relative shadow-lg rounded-full overflow-hidden">
                                                    {artist.images?.[0]?.url ? (
                                                        <img src={artist.images[0].url} alt={artist.name} className="w-full h-full object-cover" />
                                                    ) : (
                                                        <div className="w-full h-full bg-surface-3 flex items-center justify-center">
                                                            <Mic2 size={40} className="text-text-subtle" />
                                                        </div>
                                                    )}
                                                    <div className="absolute right-2 bottom-2 flex gap-1 translate-y-4 opacity-0 group-hover:translate-y-0 group-hover:opacity-100 transition-all duration-200">
                                                        <button 
                                                            onClick={(e) => { e.stopPropagation(); handleShuffleArtistTopTracks(artist); }}
                                                            className="w-8 h-8 bg-surface-3 hover:bg-surface-hover rounded-full flex items-center justify-center shadow-lg hover:scale-105 text-white" 
                                                            aria-label="Shuffle artist's top tracks"
                                                            title="Shuffle top tracks"
                                                        >
                                                            <Shuffle size={14} />
                                                        </button>
                                                        <button 
                                                            onClick={(e) => { e.stopPropagation(); handleAddArtistToQueue(artist); }}
                                                            className="w-8 h-8 bg-surface-3 hover:bg-surface-hover rounded-full flex items-center justify-center shadow-lg hover:scale-105 text-white" 
                                                            aria-label="Add artist's top tracks to queue"
                                                            title="Add to queue"
                                                        >
                                                            <ListPlus size={14} />
                                                        </button>
                                                        <button 
                                                            onClick={(e) => { e.stopPropagation(); handlePlayArtistTopTracks(artist); }}
                                                            className="w-10 h-10 bg-brand rounded-full flex items-center justify-center shadow-xl hover:scale-105 text-black" 
                                                            aria-label="Play artist's top tracks"
                                                            title="Play top tracks"
                                                        >
                                                            <Play size={20} fill="black" />
                                                        </button>
                                                    </div>
                                                </div>
                                                <h3 className="font-bold truncate text-text-main text-center">{artist.name}</h3>
                                                <p className="text-sm text-text-secondary truncate text-center mt-1">
                                                    {artist.followers?.total?.toLocaleString()} followers
                                                </p>
                                            </div>
                                        ))}
                                    </div>
                                </section>
                            )}

                            {/* Playlists - only show when playlists tab selected */}
                            {searchResultTab === 'playlists' && spotifyResults.playlists?.items && Array.isArray(spotifyResults.playlists.items) && spotifyResults.playlists.items.length > 0 && (
                                <section>
                                    <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-4">
                                        {spotifyResults.playlists.items.filter((p: any) => p).map((playlist: any) => (
                                            <div key={playlist.id} className="bg-surface-1 hover:bg-surface-2 p-4 rounded-lg transition-colors group relative">
                                                <div onClick={() => navigate(`/spotify/playlist/${playlist.id}`)} className="cursor-pointer">
                                                    <div className="aspect-square mb-4 relative shadow-lg rounded-md overflow-hidden">
                                                        <img src={playlist.images?.[0]?.url} alt={playlist.name} className="w-full h-full object-cover" />
                                                        <div className="absolute right-2 bottom-2 flex gap-1 translate-y-4 opacity-0 group-hover:translate-y-0 group-hover:opacity-100 transition-all duration-200">
                                                            <button 
                                                                onClick={(e) => { e.stopPropagation(); handleShufflePlaylist(playlist); }}
                                                                className="w-8 h-8 bg-surface-3 hover:bg-surface-hover rounded-full flex items-center justify-center shadow-lg hover:scale-105 text-white" 
                                                                aria-label="Shuffle playlist"
                                                                title="Shuffle"
                                                            >
                                                                <Shuffle size={14} />
                                                            </button>
                                                            <button 
                                                                onClick={(e) => { e.stopPropagation(); handleAddPlaylistToQueue(playlist); }}
                                                                className="w-8 h-8 bg-surface-3 hover:bg-surface-hover rounded-full flex items-center justify-center shadow-lg hover:scale-105 text-white" 
                                                                aria-label="Add to queue"
                                                                title="Add to queue"
                                                            >
                                                                <ListPlus size={14} />
                                                            </button>
                                                            <button 
                                                                onClick={(e) => { e.stopPropagation(); handlePlayPlaylist(playlist); }}
                                                                className="w-10 h-10 bg-brand rounded-full flex items-center justify-center shadow-xl hover:scale-105 text-black" 
                                                                aria-label="Play playlist"
                                                            >
                                                                <Play size={20} fill="black" />
                                                            </button>
                                                        </div>
                                                    </div>
                                                    <h3 className="font-bold truncate text-text-main">{playlist.name}</h3>
                                                    <p className="text-sm text-text-secondary truncate">By {playlist.owner?.display_name}</p>
                                                </div>
                                                <button
                                                    onClick={(e) => { e.stopPropagation(); handleDownloadPlaylist(playlist); }}
                                                    disabled={downloadingPlaylists.has(playlist.id)}
                                                    className="absolute top-2 right-2 p-2 bg-surface-3 hover:bg-brand rounded-full opacity-0 group-hover:opacity-100 transition-all duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
                                                    title={downloadingPlaylists.has(playlist.id) ? "Queueing..." : "Download playlist"}
                                                    aria-label="Download playlist"
                                                >
                                                    {downloadingPlaylists.has(playlist.id) ? (
                                                        <Loader2 className="w-4 h-4 text-white animate-spin" />
                                                    ) : (
                                                        <Download className="w-4 h-4 text-white" />
                                                    )}
                                                </button>
                                            </div>
                                        ))}
                                    </div>
                                </section>
                            )}

                            {/* Tracks - only show when tracks tab selected */}
                            {searchResultTab === 'tracks' && spotifyResults.tracks?.items && Array.isArray(spotifyResults.tracks.items) && spotifyResults.tracks.items.length > 0 && (
                                <section>
                                    <div className="bg-surface-1 rounded-xl overflow-hidden">
                                        {spotifyResults.tracks.items.filter((t: any) => t).map((track: any, idx: number) => {
                                            const isDownloaded = downloadedSpotifyIds.has(track.id);
                                            const song = spotifyTrackToSong(track);
                                            return (
                                            <div 
                                                key={track.id} 
                                                className="flex items-center gap-4 p-3 hover:bg-surface-hover group transition-colors border-b border-surface-border last:border-0 cursor-pointer" 
                                                onClick={() => handlePlayTrack(track, spotifyResults.tracks.items.filter((t: any) => t))}
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
                                                            <CheckCircle size={12} className="text-black" />
                                                        </div>
                                                    )}
                                                </div>
                                                <div className="flex-1 min-w-0">
                                                    <div className="font-medium text-text-main truncate group-hover:text-brand transition-all duration-200">{track.name}</div>
                                                    <div className="text-sm text-text-secondary truncate">
                                                        {track.artists?.map((artist: any, i: number) => (
                                                            <span key={artist.id}>
                                                                <span 
                                                                    className="hover:underline cursor-pointer"
                                                                    onClick={(e) => { e.stopPropagation(); navigate(`/spotify?artist=${artist.id}`); }}
                                                                >
                                                                    {artist.name}
                                                                </span>
                                                                {i < track.artists.length - 1 && ', '}
                                                            </span>
                                                        ))}
                                                    </div>
                                                </div>
                                                <div className="text-sm text-text-subtle font-mono">{formatTime(track.duration_ms / 1000)}</div>
                                                
                                                {/* Add to Queue button */}
                                                <button
                                                    onClick={(e) => { e.stopPropagation(); handleAddTrackToQueue(track); }}
                                                    className="p-2 text-text-subtle hover:text-white opacity-0 group-hover:opacity-100 transition-opacity"
                                                    title="Add to queue"
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
                                                    disabled={downloadingTracks.has(track.id)}
                                                    className="p-2 text-text-subtle hover:text-white opacity-0 group-hover:opacity-100 transition-opacity disabled:opacity-50 disabled:cursor-not-allowed"
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

            {/* Recently Played Tab */}
            {activeTab === 'recent' && (
                <div>
                    {isLoadingLibrary ? (
                        <div className="flex items-center justify-center py-20">
                            <Loader2 className="animate-spin text-brand" size={48} />
                        </div>
                    ) : recentlyPlayed?.items && recentlyPlayed.items.length > 0 ? (
                        <div className="bg-surface-1 rounded-xl overflow-hidden">
                            {recentlyPlayed.items.map((item: any, idx: number) => {
                                const allTracks = recentlyPlayed.items.map((i: any) => i.track);
                                const isDownloaded = downloadedSpotifyIds.has(item.track.id);
                                return (
                                <div 
                                    key={`${item.track.id}-${idx}`} 
                                    className="flex items-center gap-4 p-3 hover:bg-surface-hover group transition-all duration-200 border-b border-surface-border last:border-0 cursor-pointer"
                                    onClick={() => handlePlayTrack(item.track, allTracks)}
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
                                                <CheckCircle size={12} className="text-black" />
                                            </div>
                                        )}
                                    </div>
                                    <div className="flex-1 min-w-0">
                                        <div className="font-medium text-text-main truncate group-hover:text-brand transition-all duration-200">
                                            {item.track.name}
                                        </div>
                                        <div className="text-sm text-text-secondary truncate">{item.track.artists?.map((a: any) => a.name).join(', ')}</div>
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
                                        className="p-2 text-text-subtle hover:text-white opacity-0 group-hover:opacity-100 transition-opacity"
                                        title="Add to queue"
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
                                            disabled={downloadingTracks.has(item.track.id)}
                                            className="p-2 text-text-subtle hover:text-white opacity-0 group-hover:opacity-100 transition-opacity disabled:opacity-50"
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
                                        className="p-2 text-text-subtle hover:text-white opacity-0 group-hover:opacity-100 transition-opacity"
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
                    {isLoadingLibrary ? (
                        <div className="flex items-center justify-center py-20">
                            <Loader2 className="animate-spin text-brand" size={48} />
                        </div>
                    ) : savedAlbums?.items && savedAlbums.items.length > 0 ? (
                        <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-4">
                            {savedAlbums.items.map((item: any) => (
                                <div key={item.album.id} onClick={() => navigate(`/spotify/album/${item.album.id}`)} className="bg-surface-1 hover:bg-surface-2 p-4 rounded-lg transition-all duration-200 group cursor-pointer">
                                    <div className="aspect-square mb-4 relative shadow-lg rounded-md overflow-hidden">
                                        <img src={item.album.images?.[0]?.url} alt={item.album.name} className="w-full h-full object-cover" />
                                        <button className="absolute right-2 bottom-2 w-10 h-10 bg-brand rounded-full flex items-center justify-center shadow-xl translate-y-4 opacity-0 group-hover:translate-y-0 group-hover:opacity-100 transition-all duration-200 hover:scale-105 text-black" aria-label="Play album">
                                            <Play size={20} fill="black" />
                                        </button>
                                    </div>
                                    <h3 className="font-bold truncate text-text-main">{item.album.name}</h3>
                                    <p className="text-sm text-text-secondary truncate">{item.album.artists?.map((a: any) => a.name).join(', ')}</p>
                                    <p className="text-xs text-text-subtle mt-1">{item.album.release_date?.split('-')[0]} • {item.album.total_tracks} tracks</p>
                                </div>
                            ))}
                        </div>
                    ) : (
                        <div className="text-center py-20 text-text-subtle">
                            <Music size={64} className="mx-auto mb-4 opacity-50" />
                            <p>No saved albums</p>
                        </div>
                    )}
                </div>
            )}

            {/* Saved Playlists Tab */}
            {activeTab === 'playlists' && (
                <div>
                    {isLoadingLibrary ? (
                        <div className="flex items-center justify-center py-20">
                            <Loader2 className="animate-spin text-brand" size={48} />
                        </div>
                    ) : savedPlaylists?.items && savedPlaylists.items.length > 0 ? (
                        <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-4">
                            {savedPlaylists.items.map((playlist: any) => (
                                <div key={playlist.id} onClick={() => navigate(`/spotify/playlist/${playlist.id}`)} className="bg-surface-1 hover:bg-surface-2 p-4 rounded-lg transition-all duration-200 group cursor-pointer">
                                    <div className="aspect-square mb-4 relative shadow-lg rounded-md overflow-hidden">
                                        <img src={playlist.images?.[0]?.url} alt={playlist.name} className="w-full h-full object-cover" />
                                        <button className="absolute right-2 bottom-2 w-10 h-10 bg-brand rounded-full flex items-center justify-center shadow-xl translate-y-4 opacity-0 group-hover:translate-y-0 group-hover:opacity-100 transition-all duration-200 hover:scale-105 text-black" aria-label="Play playlist">
                                            <Play size={20} fill="black" />
                                        </button>
                                    </div>
                                    <h3 className="font-bold truncate text-text-main">{playlist.name}</h3>
                                    <p className="text-sm text-text-secondary truncate">By {playlist.owner?.display_name}</p>
                                    <p className="text-xs text-text-subtle mt-1">{playlist.tracks.total} tracks</p>
                                </div>
                            ))}
                        </div>
                    ) : (
                        <div className="text-center py-20 text-text-subtle">
                            <Music size={64} className="mx-auto mb-4 opacity-50" />
                            <p>No saved playlists</p>
                        </div>
                    )}
                </div>
            )}
        </div>
    );
};