/**
 * ViiB MediaHub - Main Application Component
 * 
 * Entry point for the React frontend. Handles:
 * - Route configuration using react-router
 * - Global state initialization (library, Spotify credentials sync)
 * - Error boundary for graceful component failure handling
 * - Background metadata enrichment startup
 * - Global UI overlays (ConfirmDialog, DownloadManager)
 * 
 * @module App
 */
import React, { useEffect, Component, ReactNode } from 'react';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router';
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
import { Duplicates } from './pages/Duplicates';
import { Stats } from './pages/Stats';
import { SmartMixDetail } from './pages/SmartMixDetail';
import { DJModeV2 } from './pages/DJModeV2';
import { useStore } from './store';
import { api } from './services/api';
import { plexService } from './services/plex';
import DownloadManager from './components/DownloadManager';
import LibraryEventListener from './components/LibraryEventListener';
import ConfirmDialog from './components/ConfirmDialog';
import FirstLaunchDialog from './components/FirstLaunchDialog';
import { SongInfoDialog } from './components/SongInfoDialog';
import { useBackgroundEnrichment } from './hooks/useBackgroundEnrichment';
import { setupGlobalErrorHandlers, createLogger } from './services/loggerService';

setupGlobalErrorHandlers();
const appLogger = createLogger('App');

class ErrorBoundary extends Component<{ children: ReactNode }, { hasError: boolean }> {
  constructor(props: { children: ReactNode }) { super(props); this.state = { hasError: false }; }
  static getDerivedStateFromError() { return { hasError: true }; }
  componentDidCatch(error: Error, errorInfo: React.ErrorInfo) { appLogger.logError(error, `ErrorBoundary caught an error in ${errorInfo.componentStack}`); }
  render() {
    if (this.state.hasError) {
      return (
        <div className="flex min-h-screen items-center justify-center bg-surface-0 p-8">
          <div className="max-w-lg rounded-xl border border-error/30 bg-surface-2 p-8 text-center">
            <h1 className="text-xl font-semibold text-error">ViiB MediaHub encountered an interface error</h1>
            <p className="mt-3 text-sm text-text-secondary">Your library data remains stored locally. Refresh the application to recover the interface.</p>
            <button type="button" className="mt-5 rounded-full bg-brand px-5 py-2 font-semibold text-surface-0 hover:bg-brand-hover" onClick={() => window.location.reload()}>Refresh application</button>
          </div>
        </div>
      );
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

  useBackgroundEnrichment();
  useEffect(() => { initLibrary(); }, [initLibrary]);

  useEffect(() => {
    const checkExistingConfig = async () => {
      if (!backendAvailable || hasCompletedSetup) return;
      try {
        const [folders, creds, songs, plexConfig] = await Promise.all([
          api.getFolders().catch(() => []),
          api.getSpotifyCredentials().catch(() => null),
          api.getSongs().catch(() => []),
          plexService.getConfig().catch(() => null),
        ]);
        if (folders.length > 0 || songs.length > 0 || Boolean(creds?.clientId) || Boolean(plexConfig?.source?.libraryId)) {
          setHasCompletedSetup(true);
        }
      } catch (error) { appLogger.warn('Failed to check existing configuration', error); }
    };
    void checkExistingConfig();
  }, [backendAvailable, hasCompletedSetup, setHasCompletedSetup]);

  useEffect(() => {
    const syncSpotify = async () => {
      if (spotifyAccessToken) return;
      try {
        const creds = await api.getSpotifyCredentials();
        // The public client ID is needed to begin a PKCE login even when the
        // user has not authenticated yet. The backend intentionally does not
        // return the stored client secret to the renderer.
        if (creds?.clientId) {
          setSpotifyCredentials(creds.clientId, '');
        }
        if (creds?.accessToken) {
          setSpotifyTokens(creds.accessToken, creds.refreshToken, creds.expiry);
        }
      } catch { /* Spotify is optional. */ }
    };
    void syncSpotify();
  }, [spotifyAccessToken, setSpotifyCredentials, setSpotifyTokens]);

  const loadAudioSettings = useStore(state => state.loadAudioSettings);
  useEffect(() => { if (backendAvailable) void loadAudioSettings(); }, [backendAvailable, loadAudioSettings]);

  return (
    <ErrorBoundary>
      <BrowserRouter>
        <Layout>
          <Routes>
            <Route path="/" element={<Home />} />
            <Route path="/songs" element={<Songs />} />
            <Route path="/albums" element={<Albums />} />
            <Route path="/album/:albumName/:artistName?" element={<AlbumDetail />} />
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
            <Route path="/duplicates" element={<Duplicates />} />
            <Route path="/library-operations" element={<Navigate to="/settings" state={{ tab: 'health' }} replace />} />
            <Route path="/dj" element={<DJModeV2 />} />
            <Route path="/dj-v2" element={<Navigate to="/dj" replace />} />
            <Route path="/settings" element={<Settings />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
          <DownloadManager />
          <LibraryEventListener />
        </Layout>
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
        <FirstLaunchDialog isOpen={backendAvailable && !hasCompletedSetup} onComplete={() => setHasCompletedSetup(true)} />
        <SongInfoDialog />
      </BrowserRouter>
    </ErrorBoundary>
  );
};

export default App;