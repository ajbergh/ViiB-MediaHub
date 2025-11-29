import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Wifi, LogOut, ExternalLink, CheckCircle, Search as SearchIcon, Loader2, Play, MoreHorizontal, User, Music } from 'lucide-react';
import { formatTime } from '../utils';
import { useStore } from '../store';
import { SpotifyService } from '../services/spotifyService';
import { SpotifyAuthError, SpotifyRateLimitError, SpotifyApiError } from '../lib/spotifyErrors';
import { api } from '../services/api';

export const Spotify: React.FC = () => {
  const navigate = useNavigate();
  const { 
      spotifyClientId, spotifyClientSecret, spotifyUser, 
      spotifyAccessToken, spotifyRefreshToken, spotifyTokenExpiry,
      logoutSpotify, setSpotifyTokens, setSpotifyUser, addLog 
  } = useStore();

  // Session restoration state
  const [isRestoringSession, setIsRestoringSession] = useState(false);

  // Tab State
  const [activeTab, setActiveTab] = useState<'search' | 'recent' | 'albums' | 'playlists'>('search');

  // Search State
  const [inputValue, setInputValue] = useState('');
  const [debouncedQuery, setDebouncedQuery] = useState('');
  const [spotifyResults, setSpotifyResults] = useState<any>(null);
  const [isSearching, setIsSearching] = useState(false);
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  const [hasMore, setHasMore] = useState(false);

  // Library State
  const [recentlyPlayed, setRecentlyPlayed] = useState<any>(null);
  const [savedAlbums, setSavedAlbums] = useState<any>(null);
  const [savedPlaylists, setSavedPlaylists] = useState<any>(null);
  const [isLoadingLibrary, setIsLoadingLibrary] = useState(false);

  // Download State - track which items are currently being queued
  const [downloadingTracks, setDownloadingTracks] = useState<Set<string>>(new Set());
  const [downloadingAlbums, setDownloadingAlbums] = useState<Set<string>>(new Set());
  const [downloadingPlaylists, setDownloadingPlaylists] = useState<Set<string>>(new Set());

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
      }, 500); // 500ms delay for API calls

      return () => {
          clearTimeout(handler);
      };
  }, [inputValue]);

  // Search Effect
  useEffect(() => {
      if (debouncedQuery && spotifyUser) {
          const searchSpotify = async () => {
              setIsSearching(true);
              try {
                  const results = await SpotifyService.search(debouncedQuery, ['album', 'playlist', 'track'], 20, 0);
                  setSpotifyResults(results);
                  
                  // Check if there are more results
                  const hasMoreAlbums = results.albums?.next !== null;
                  const hasMorePlaylists = results.playlists?.next !== null;
                  const hasMoreTracks = results.tracks?.next !== null;
                  setHasMore(hasMoreAlbums || hasMorePlaylists || hasMoreTracks);
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
                               spotifyResults.tracks?.items?.length || 0;
          
          const moreResults = await SpotifyService.search(debouncedQuery, ['album', 'playlist', 'track'], 20, currentOffset);
          
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
              } : prev.tracks || moreResults.tracks
          }));
          
          // Update hasMore
          const hasMoreAlbums = moreResults.albums?.next !== null;
          const hasMorePlaylists = moreResults.playlists?.next !== null;
          const hasMoreTracks = moreResults.tracks?.next !== null;
          setHasMore(hasMoreAlbums || hasMorePlaylists || hasMoreTracks);
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

  const handleLogin = async () => {
    if (!spotifyClientId || !spotifyClientSecret) {
        alert("Please configure your Client ID and Client Secret in Settings first.");
        return;
    }
    
    // Ensure we use 127.0.0.1 instead of localhost for Spotify compliance
    let origin = window.location.origin;
    if (window.location.hostname === 'localhost') {
        origin = origin.replace('localhost', '127.0.0.1');
        if (window.location.origin !== origin) {
             // If we are on localhost, we should probably redirect the user to 127.0.0.1 first
             // or just use 127.0.0.1 for the callback and hope the popup handles it.
             // But the popup needs to communicate back to the opener.
             // If opener is localhost and popup is 127.0.0.1, cross-origin restrictions might apply.
             // So it's better to warn the user to use 127.0.0.1.
             alert("Please access this app via http://127.0.0.1:3000 instead of localhost to comply with Spotify's new security requirements.");
             window.location.href = window.location.href.replace('localhost', '127.0.0.1');
             return;
        }
    }

    const redirectUri = `${origin}/callback`;
    
    addLog('info', 'Initiating Spotify Login', { redirectUri, clientId: spotifyClientId });
    
    const { url, codeVerifier } = await SpotifyService.generateAuthUrl(spotifyClientId, redirectUri);
    
    // Store verifier for the callback
    localStorage.setItem('spotify_code_verifier', codeVerifier);
    
    addLog('info', 'Opening Spotify Auth Popup', { url });

    // Open popup
    const width = 600;
    const height = 800;
    const left = window.screen.width / 2 - width / 2;
    const top = window.screen.height / 2 - height / 2;
    window.open(url, 'Spotify Auth', `width=${width},height=${height},left=${left},top=${top}`);
  };

  const handleLogout = () => {
      logoutSpotify();
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

  // Show loading screen while restoring session
  if (isRestoringSession) {
    return (
        <div className="h-full flex flex-col items-center justify-center p-8 text-center">
            <div className="w-24 h-24 bg-[#1db954] rounded-full flex items-center justify-center mb-6 shadow-lg shadow-[#1db954]/20">
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
            <div className="w-24 h-24 bg-[#1db954] rounded-full flex items-center justify-center mb-6 shadow-lg shadow-[#1db954]/20">
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
                className="bg-[#1db954] hover:bg-[#1ed760] disabled:opacity-50 disabled:cursor-not-allowed text-black font-bold py-3 px-8 rounded-full transition-all transform hover:scale-105 shadow-lg flex items-center gap-2"
            >
                <Wifi size={20} /> Connect Spotify
            </button>

            {(!spotifyClientId || !spotifyClientSecret) && (
                <div className="mt-8 w-full max-w-lg bg-[#181818] border border-[#ffaa00]/30 rounded-xl p-6 relative overflow-hidden text-left">
                    <div className="absolute top-0 left-0 w-1 h-full bg-[#ffaa00]"></div>
                    <h4 className="text-[#ffaa00] font-bold text-sm uppercase tracking-wide mb-3 flex items-center gap-2">
                        Configuration Required
                    </h4>
                    <div className="text-sm text-[#b3b8c1] space-y-3">
                        <p>To enable integration:</p>
                        <ol className="list-decimal list-inside space-y-2 ml-1">
                            <li>Create a Spotify App at <a href="https://developer.spotify.com/dashboard" target="_blank" rel="noreferrer" className="text-green-500 hover:underline">developer.spotify.com</a></li>
                            <li>
                                Go to <span className="text-white font-bold">Settings</span> in this app and enter both your 
                                <span className="text-white font-mono bg-[#333] px-1 rounded mx-1">Client ID</span> 
                                and 
                                <span className="text-white font-mono bg-[#333] px-1 rounded mx-1">Client Secret</span>.
                            </li>
                            <li>Add this Redirect URI in your Spotify Dashboard:</li>
                        </ol>
                        <div className="mt-2 bg-[#121212] p-3 rounded font-mono text-xs text-gray-400 break-all select-all border border-[#333]">
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
                <Music className="text-[#1db954]" size={32} />
                Spotify
            </h1>
            <button 
                onClick={handleLogout}
                className="flex items-center gap-2 px-4 py-2 bg-surface-2 hover:bg-surface-3 rounded-full text-sm font-bold transition-colors"
            >
                <LogOut size={16} /> Disconnect
            </button>
        </div>

        <div className="bg-gradient-to-br from-[#1db954]/20 to-surface-1 p-6 rounded-2xl border border-[#1db954]/30 mb-8">
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
                        className="inline-flex items-center gap-1 text-[#1db954] hover:text-[#1ed760] font-bold text-sm"
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
                className={`px-6 py-3 font-bold transition-all border-b-2 ${
                    activeTab === 'search'
                        ? 'border-[#1db954] text-[#1db954]'
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
                className={`px-6 py-3 font-bold transition-all border-b-2 ${
                    activeTab === 'recent'
                        ? 'border-[#1db954] text-[#1db954]'
                        : 'border-transparent text-text-secondary hover:text-text-main'
                }`}
            >
                Recently Played
            </button>
            <button
                onClick={() => setActiveTab('albums')}
                className={`px-6 py-3 font-bold transition-all border-b-2 ${
                    activeTab === 'albums'
                        ? 'border-[#1db954] text-[#1db954]'
                        : 'border-transparent text-text-secondary hover:text-text-main'
                }`}
            >
                Saved Albums
            </button>
            <button
                onClick={() => setActiveTab('playlists')}
                className={`px-6 py-3 font-bold transition-all border-b-2 ${
                    activeTab === 'playlists'
                        ? 'border-[#1db954] text-[#1db954]'
                        : 'border-transparent text-text-secondary hover:text-text-main'
                }`}
            >
                Saved Playlists
            </button>
        </div>

        {/* Search Section */}
        {activeTab === 'search' && (
        <>
        <div className="mb-8">
            <div className="relative w-full max-w-3xl">
                <SearchIcon className="absolute left-5 top-1/2 -translate-y-1/2 text-text-secondary" size={22} />
                <input 
                    type="text" 
                    placeholder="Search Spotify for songs, albums, or playlists..."
                    className="w-full bg-surface-highlight hover:bg-surface-hover focus:bg-surface-hover border border-transparent focus:border-[#1db954] rounded-full py-4 pl-14 pr-6 text-text-main outline-none transition-all placeholder-text-subtle text-lg shadow-lg"
                    value={inputValue}
                    onChange={(e) => setInputValue(e.target.value)}
                />
                {isSearching && (
                    <div className="absolute right-5 top-1/2 -translate-y-1/2">
                        <Loader2 className="animate-spin text-[#1db954]" size={20} />
                    </div>
                )}
            </div>
        </div>

        {/* Results */}
        {spotifyResults ? (
            <div className="space-y-10 pb-20">
                {/* Albums */}
                {spotifyResults.albums?.items && Array.isArray(spotifyResults.albums.items) && spotifyResults.albums.items.length > 0 && (
                    <section>
                        <h2 className="text-xl font-bold mb-4">Albums</h2>
                        <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-4">
                            {spotifyResults.albums.items.filter((a: any) => a).map((album: any) => (
                                <div key={album.id} className="bg-surface-1 hover:bg-surface-2 p-4 rounded-lg transition-colors group relative">
                                    <div onClick={() => navigate(`/spotify/album/${album.id}`)} className="cursor-pointer">
                                        <div className="aspect-square mb-4 relative shadow-lg rounded-md overflow-hidden">
                                            <img src={album.images?.[0]?.url} alt={album.name} className="w-full h-full object-cover" />
                                            <button className="absolute right-2 bottom-2 w-10 h-10 bg-[#1db954] rounded-full flex items-center justify-center shadow-xl translate-y-4 opacity-0 group-hover:translate-y-0 group-hover:opacity-100 transition-all hover:scale-105 text-black">
                                                <Play size={20} fill="black" />
                                            </button>
                                        </div>
                                        <h3 className="font-bold truncate text-text-main">{album.name}</h3>
                                        <p className="text-sm text-text-secondary truncate">{album.artists?.map((a: any) => a.name).join(', ')}</p>
                                        <p className="text-xs text-text-subtle mt-1">{album.release_date?.split('-')[0]} • Album</p>
                                    </div>
                                    <button
                                        onClick={(e) => { e.stopPropagation(); handleDownloadAlbum(album); }}
                                        disabled={downloadingAlbums.has(album.id)}
                                        className="absolute top-2 right-2 p-2 bg-surface-3 hover:bg-brand rounded-full opacity-0 group-hover:opacity-100 transition-all disabled:opacity-50 disabled:cursor-not-allowed"
                                        title={downloadingAlbums.has(album.id) ? "Queueing..." : "Download album"}
                                    >
                                        {downloadingAlbums.has(album.id) ? (
                                            <Loader2 className="w-4 h-4 text-white animate-spin" />
                                        ) : (
                                            <svg className="w-4 h-4 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M9 19l3 3m0 0l3-3m-3 3V10" />
                                            </svg>
                                        )}
                                    </button>
                                </div>
                            ))}
                        </div>
                    </section>
                )}

                {/* Playlists */}
                {spotifyResults.playlists?.items && Array.isArray(spotifyResults.playlists.items) && spotifyResults.playlists.items.length > 0 && (
                    <section>
                        <h2 className="text-xl font-bold mb-4">Playlists</h2>
                        <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-4">
                            {spotifyResults.playlists.items.filter((p: any) => p).map((playlist: any) => (
                                <div key={playlist.id} className="bg-surface-1 hover:bg-surface-2 p-4 rounded-lg transition-colors group relative">
                                    <div onClick={() => navigate(`/spotify/playlist/${playlist.id}`)} className="cursor-pointer">
                                        <div className="aspect-square mb-4 relative shadow-lg rounded-md overflow-hidden">
                                            <img src={playlist.images?.[0]?.url} alt={playlist.name} className="w-full h-full object-cover" />
                                            <button className="absolute right-2 bottom-2 w-10 h-10 bg-[#1db954] rounded-full flex items-center justify-center shadow-xl translate-y-4 opacity-0 group-hover:translate-y-0 group-hover:opacity-100 transition-all hover:scale-105 text-black">
                                                <Play size={20} fill="black" />
                                            </button>
                                        </div>
                                        <h3 className="font-bold truncate text-text-main">{playlist.name}</h3>
                                        <p className="text-sm text-text-secondary truncate">By {playlist.owner?.display_name}</p>
                                    </div>
                                    <button
                                        onClick={(e) => { e.stopPropagation(); handleDownloadPlaylist(playlist); }}
                                        disabled={downloadingPlaylists.has(playlist.id)}
                                        className="absolute top-2 right-2 p-2 bg-surface-3 hover:bg-brand rounded-full opacity-0 group-hover:opacity-100 transition-all disabled:opacity-50 disabled:cursor-not-allowed"
                                        title={downloadingPlaylists.has(playlist.id) ? "Queueing..." : "Download playlist"}
                                    >
                                        {downloadingPlaylists.has(playlist.id) ? (
                                            <Loader2 className="w-4 h-4 text-white animate-spin" />
                                        ) : (
                                            <svg className="w-4 h-4 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M9 19l3 3m0 0l3-3m-3 3V10" />
                                            </svg>
                                        )}
                                    </button>
                                </div>
                            ))}
                        </div>
                    </section>
                )}

                {/* Tracks */}
                {spotifyResults.tracks?.items && Array.isArray(spotifyResults.tracks.items) && spotifyResults.tracks.items.length > 0 && (
                    <section>
                        <h2 className="text-xl font-bold mb-4">Songs</h2>
                        <div className="bg-surface-1 rounded-xl overflow-hidden">
                            {spotifyResults.tracks.items.filter((t: any) => t).map((track: any, idx: number) => (
                                <div key={track.id} className="flex items-center gap-4 p-3 hover:bg-surface-hover group transition-colors border-b border-surface-border last:border-0">
                                    <div className="w-8 text-center text-text-subtle text-sm">{idx + 1}</div>
                                    <div className="w-10 h-10 rounded overflow-hidden flex-shrink-0">
                                        <img src={track.album?.images?.[2]?.url || track.album?.images?.[0]?.url} alt={track.name} className="w-full h-full object-cover" />
                                    </div>
                                    <div className="flex-1 min-w-0">
                                        <div className="font-medium text-text-main truncate group-hover:text-[#1db954] transition-colors">{track.name}</div>
                                        <div className="text-sm text-text-secondary truncate">{track.artists?.map((a: any) => a.name).join(', ')}</div>
                                    </div>
                                    <div className="text-sm text-text-subtle font-mono">{formatTime(track.duration_ms / 1000)}</div>
                                    <button 
                                        onClick={() => handleDownloadTrack(track)}
                                        disabled={downloadingTracks.has(track.id)}
                                        className="p-2 text-text-subtle hover:text-white opacity-0 group-hover:opacity-100 transition-opacity disabled:opacity-50 disabled:cursor-not-allowed"
                                        title={downloadingTracks.has(track.id) ? "Queueing..." : "Download track"}
                                    >
                                        {downloadingTracks.has(track.id) ? (
                                            <Loader2 className="w-4 h-4 animate-spin" />
                                        ) : (
                                            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M9 19l3 3m0 0l3-3m-3 3V10" />
                                            </svg>
                                        )}
                                    </button>
                                </div>
                            ))}
                        </div>
                    </section>
                )}
                
                {(!spotifyResults.albums?.items?.length && !spotifyResults.playlists?.items?.length && !spotifyResults.tracks?.items?.length) && (
                    <div className="text-center p-10 text-text-subtle">
                        No results found on Spotify for "{debouncedQuery}"
                    </div>
                )}
                
                {/* Load More Button */}
                {hasMore && (spotifyResults.albums?.items?.length || spotifyResults.playlists?.items?.length || spotifyResults.tracks?.items?.length) && (
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
                        <Loader2 className="animate-spin text-[#1db954]" size={48} />
                    </div>
                ) : recentlyPlayed?.items && recentlyPlayed.items.length > 0 ? (
                    <div className="bg-surface-1 rounded-xl overflow-hidden">
                        {recentlyPlayed.items.map((item: any, idx: number) => (
                            <div key={`${item.track.id}-${idx}`} className="flex items-center gap-4 p-3 hover:bg-surface-hover group transition-colors border-b border-surface-border last:border-0">
                                <div className="w-10 h-10 rounded overflow-hidden flex-shrink-0">
                                    <img src={item.track.album?.images?.[2]?.url || item.track.album?.images?.[0]?.url} alt={item.track.name} className="w-full h-full object-cover" />
                                </div>
                                <div className="flex-1 min-w-0">
                                    <div className="font-medium text-text-main truncate group-hover:text-[#1db954] transition-colors">
                                        {item.track.name}
                                    </div>
                                    <div className="text-sm text-text-secondary truncate">{item.track.artists?.map((a: any) => a.name).join(', ')}</div>
                                </div>
                                <div className="text-sm text-text-subtle">{item.track.album.name}</div>
                                <div className="text-sm text-text-subtle font-mono">{formatTime(item.track.duration_ms / 1000)}</div>
                                <button className="p-2 text-text-subtle hover:text-white opacity-0 group-hover:opacity-100 transition-opacity">
                                    <MoreHorizontal size={18} />
                                </button>
                            </div>
                        ))}
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
                        <Loader2 className="animate-spin text-[#1db954]" size={48} />
                    </div>
                ) : savedAlbums?.items && savedAlbums.items.length > 0 ? (
                    <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-4">
                        {savedAlbums.items.map((item: any) => (
                            <div key={item.album.id} onClick={() => navigate(`/spotify/album/${item.album.id}`)} className="bg-surface-1 hover:bg-surface-2 p-4 rounded-lg transition-colors group cursor-pointer">
                                <div className="aspect-square mb-4 relative shadow-lg rounded-md overflow-hidden">
                                    <img src={item.album.images?.[0]?.url} alt={item.album.name} className="w-full h-full object-cover" />
                                    <button className="absolute right-2 bottom-2 w-10 h-10 bg-[#1db954] rounded-full flex items-center justify-center shadow-xl translate-y-4 opacity-0 group-hover:translate-y-0 group-hover:opacity-100 transition-all hover:scale-105 text-black">
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
                        <Loader2 className="animate-spin text-[#1db954]" size={48} />
                    </div>
                ) : savedPlaylists?.items && savedPlaylists.items.length > 0 ? (
                    <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-4">
                        {savedPlaylists.items.map((playlist: any) => (
                            <div key={playlist.id} onClick={() => navigate(`/spotify/playlist/${playlist.id}`)} className="bg-surface-1 hover:bg-surface-2 p-4 rounded-lg transition-colors group cursor-pointer">
                                <div className="aspect-square mb-4 relative shadow-lg rounded-md overflow-hidden">
                                    <img src={playlist.images?.[0]?.url} alt={playlist.name} className="w-full h-full object-cover" />
                                    <button className="absolute right-2 bottom-2 w-10 h-10 bg-[#1db954] rounded-full flex items-center justify-center shadow-xl translate-y-4 opacity-0 group-hover:translate-y-0 group-hover:opacity-100 transition-all hover:scale-105 text-black">
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