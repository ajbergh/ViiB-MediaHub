/**
 * ViiB MediaHub - Main Application Component
 * 
 * Entry point for the React frontend. Handles:
 * - Route configuration using react-router-dom
 * - Global state initialization (library, Spotify credentials sync)
 * - Error boundary for graceful component failure handling
 * - Background metadata enrichment startup
 * - Global UI overlays (ConfirmDialog, DownloadManager)
 * 
 * @module App
 */

import React, { useEffect, Component, ReactNode } from 'react';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { Layout } from './components/Layout';
import { Home } from './pages/Home';
import { Songs } from './pages/Songs';
import { Albums } from './pages/Albums';
import { AlbumDetail } from './pages/AlbumDetail';
import { Artists } from './pages/Artists';
import { ArtistDetail } from './pages/ArtistDetail';
import { Genres } from './pages/Genres';
import { GenreDetail } from './pages/GenreDetail';
import { SmartPlaylists } from './pages/SmartPlaylists';
import { Playlists } from './pages/Playlists';
import { LikedSongs } from './pages/LikedSongs';
import { LikedAlbums } from './pages/LikedAlbums';
import { Spotify } from './pages/Spotify';
import { SpotifyCallback } from './pages/SpotifyCallback';
import { SpotifyAlbumDetail } from './pages/SpotifyAlbumDetail';
import { SpotifyPlaylistDetail } from './pages/SpotifyPlaylistDetail';
import { Downloads } from './pages/Downloads';
import { Search } from './pages/Search';
import { Settings } from './pages/Settings';
import { Stats } from './pages/Stats';
import { SmartMixDetail } from './pages/SmartMixDetail';
import { useStore } from './store';
import { api } from './services/api';
import DownloadManager from './components/DownloadManager';
import LibraryEventListener from './components/LibraryEventListener';
import ConfirmDialog from './components/ConfirmDialog';
import FirstLaunchDialog from './components/FirstLaunchDialog';
import { useBackgroundEnrichment } from './hooks/useBackgroundEnrichment';

class ErrorBoundary extends Component<{ children: ReactNode }, { hasError: boolean }> {
  constructor(props: { children: ReactNode }) {
    super(props);
    this.state = { hasError: false };
  }

  static getDerivedStateFromError() {
    return { hasError: true };
  }

  componentDidCatch(error: Error, errorInfo: React.ErrorInfo) {
    console.error('ErrorBoundary caught an error:', error, errorInfo);
  }

  render() {
    if (this.state.hasError) {
      return null; // Silently fail for DownloadManager
    }
    return this.props.children;
  }
}

const App: React.FC = () => {
  const initLibrary = useStore(state => state.initLibrary);
  const { spotifyAccessToken, setSpotifyTokens, setSpotifyCredentials } = useStore();
  const confirmDialog = useStore(state => state.confirmDialog);
  const closeConfirmDialog = useStore(state => state.closeConfirmDialog);
  const hasCompletedSetup = useStore(state => state.hasCompletedSetup);
  const setHasCompletedSetup = useStore(state => state.setHasCompletedSetup);
  const backendAvailable = useStore(state => state.backendAvailable);

  // Background metadata enrichment for unchecked albums
  useBackgroundEnrichment();

  useEffect(() => {
    initLibrary();
  }, [initLibrary]);

  // Check for existing configuration and auto-complete setup if data exists
  useEffect(() => {
      const checkExistingConfig = async () => {
          if (!backendAvailable || hasCompletedSetup) {
              return;
          }

          try {
              // Check if database has existing configuration data
              const [folders, creds, songs] = await Promise.all([
                  api.getFolders().catch(() => []),
                  api.getSpotifyCredentials().catch(() => null),
                  api.getSongs().catch(() => [])
              ]);

              // If we have scan folders or songs or Spotify credentials, mark setup as complete
              const hasExistingData = folders.length > 0 || songs.length > 0 || 
                                     (creds && creds.clientId && creds.clientSecret);
              
              if (hasExistingData) {
                  console.log("Existing configuration detected, skipping first launch dialog");
                  setHasCompletedSetup(true);
              }
          } catch (e) {
              console.error("Failed to check existing configuration", e);
          }
      };

      checkExistingConfig();
  }, [backendAvailable, hasCompletedSetup, setHasCompletedSetup]);

  // Sync Spotify credentials from backend on startup
  useEffect(() => {
      const syncSpotify = async () => {
          // Only fetch if we don't have tokens (or maybe always to ensure sync?)
          // If we have tokens in localStorage (via persist), we might be fine.
          // But if backend has newer tokens (refreshed by background task?), we should take them.
          // For now, let's just fetch if missing or expired?
          // Simple check: if no access token, try to get from backend.
          if (!spotifyAccessToken) {
              try {
                  const creds = await api.getSpotifyCredentials();
                  if (creds && creds.accessToken) {
                      setSpotifyCredentials(creds.clientId, creds.clientSecret);
                      setSpotifyTokens(creds.accessToken, creds.refreshToken, creds.expiry);
                      console.log("Synced Spotify credentials from backend");
                  }
              } catch (e) {
                  // Ignore error, maybe not set up yet
              }
          }
      };
      syncSpotify();
  }, [spotifyAccessToken, setSpotifyCredentials, setSpotifyTokens]);

  // Load audio settings from backend on startup
  const loadAudioSettings = useStore(state => state.loadAudioSettings);
  useEffect(() => {
      if (backendAvailable) {
          loadAudioSettings();
      }
  }, [backendAvailable, loadAudioSettings]);

  return (
    <BrowserRouter>
      <Layout>
        <Routes>
          <Route path="/" element={<Home />} />
          <Route path="/songs" element={<Songs />} />
          <Route path="/albums" element={<Albums />} />
          <Route path="/album/:albumName" element={<AlbumDetail />} />
          <Route path="/artists" element={<Artists />} />
          <Route path="/artist/:artistName" element={<ArtistDetail />} />
          <Route path="/genres" element={<Genres />} />
          <Route path="/genres/:genreId" element={<GenreDetail />} />
          <Route path="/smart-playlists" element={<SmartPlaylists />} />
          <Route path="/playlists" element={<Playlists />} />
          <Route path="/liked" element={<LikedSongs />} />
          <Route path="/liked-albums" element={<LikedAlbums />} />
          <Route path="/smart-mix/:mixId" element={<SmartMixDetail />} />
          <Route path="/spotify" element={<Spotify />} />
          <Route path="/spotify/album/:id" element={<SpotifyAlbumDetail />} />
          <Route path="/spotify/playlist/:id" element={<SpotifyPlaylistDetail />} />
          <Route path="/callback" element={<SpotifyCallback />} />
          <Route path="/downloads" element={<Downloads />} />
          <Route path="/search" element={<Search />} />
          <Route path="/stats" element={<Stats />} />
          <Route path="/settings" element={<Settings />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
        <ErrorBoundary>
          <DownloadManager />
        </ErrorBoundary>
        <LibraryEventListener />
      </Layout>
      {/* Global Confirm Dialog */}
      <ConfirmDialog
        isOpen={!!confirmDialog}
        title={confirmDialog?.title || ''}
        message={confirmDialog?.message || ''}
        confirmLabel={confirmDialog?.confirmLabel}
        cancelLabel={confirmDialog?.cancelLabel}
        variant={confirmDialog?.variant}
        onConfirm={() => confirmDialog?.onConfirm()}
        onCancel={closeConfirmDialog}
      />
      {/* First Launch Setup Dialog */}
      <FirstLaunchDialog
        isOpen={backendAvailable && !hasCompletedSetup}
        onComplete={() => setHasCompletedSetup(true)}
      />
    </BrowserRouter>
  );
};

export default App;