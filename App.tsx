/**
 * ViiB MediaHub - Main Application Component
 * 
 * Entry point for the React frontend. Handles:
 * - Route configuration using react-router
 * - Global state initialization (library, redacted Spotify session status)
 * - Error boundary for graceful component failure handling
 * - Background metadata enrichment startup
 * - Global UI overlays (ConfirmDialog, DownloadManager)
 * 
 * @module App
 */
import React, { useEffect, Component, ReactNode, lazy, Suspense } from 'react';
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
import { Playlists } from './pages/Playlists';
import { PlaylistDetail } from './pages/PlaylistDetail';
import { LikedSongs } from './pages/LikedSongs';
import { LikedAlbums } from './pages/LikedAlbums';
import { SpotifyCallback } from './pages/SpotifyCallback';
import { Search } from './pages/Search';
import { SmartMixDetail } from './pages/SmartMixDetail';
import { useStore } from './store';
import { api } from './services/api';
import { plexService } from './services/plex';
import DownloadManager from './components/DownloadManager';
import LibraryEventListener from './components/LibraryEventListener';
import ConfirmDialog from './components/ConfirmDialog';
import { useBackgroundEnrichment } from './hooks/useBackgroundEnrichment';
import { setupGlobalErrorHandlers, createLogger } from './services/loggerService';
import { openExternalURL, shouldUseSystemBrowser } from './services/externalNavigation';

const Spotify = lazy(() => import('./pages/Spotify').then(module => ({ default: module.Spotify })));
const SpotifyArtistDetail = lazy(() => import('./pages/SpotifyArtistDetail').then(module => ({ default: module.SpotifyArtistDetail })));
const SpotifyAlbumDetail = lazy(() => import('./pages/SpotifyAlbumDetail').then(module => ({ default: module.SpotifyAlbumDetail })));
const SpotifyPlaylistDetail = lazy(() => import('./pages/SpotifyPlaylistDetail').then(module => ({ default: module.SpotifyPlaylistDetail })));
const Downloads = lazy(() => import('./pages/Downloads').then(module => ({ default: module.Downloads })));
const SmartPlaylists = lazy(() => import('./pages/SmartPlaylists').then(module => ({ default: module.SmartPlaylists })));
const Stats = lazy(() => import('./pages/Stats').then(module => ({ default: module.Stats })));
const Duplicates = lazy(() => import('./pages/Duplicates').then(module => ({ default: module.Duplicates })));
const Settings = lazy(() => import('./pages/Settings').then(module => ({ default: module.Settings })));
const DJModeV2 = lazy(() => import('./pages/DJModeV2').then(module => ({ default: module.DJModeV2 })));
const FirstLaunchDialog = lazy(() => import('./components/FirstLaunchDialog').then(module => ({ default: module.FirstLaunchDialog })));
const SongInfoDialog = lazy(() => import('./components/SongInfoDialog').then(module => ({ default: module.SongInfoDialog })));

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
  const setSpotifyConnected = useStore(state => state.setSpotifyConnected);
  const hasLocalLibrary = useStore(state => state.songs.length > 0 || state.scanFolders.length > 0);
  const hasSongInfo = useStore(state => Boolean(state.songInfoModalSong));
  const confirmDialog = useStore(state => state.confirmDialog);
  const closeConfirmDialog = useStore(state => state.closeConfirmDialog);
  const hasCompletedSetup = useStore(state => state.hasCompletedSetup);
  const setHasCompletedSetup = useStore(state => state.setHasCompletedSetup);
  const isLibraryInitializing = useStore(state => state.isLibraryInitializing);
  const backendAvailable = useStore(state => state.backendAvailable);

  useBackgroundEnrichment();
  useEffect(() => { initLibrary(); }, [initLibrary]);

  useEffect(() => {
    const checkExistingConfig = async () => {
      if (!backendAvailable || hasCompletedSetup || isLibraryInitializing) return;
      if (hasLocalLibrary) {
        setHasCompletedSetup(true);
        return;
      }
      try {
        const [creds, plexConfig] = await Promise.all([
          api.getSpotifyAuthStatus().catch(() => null),
          plexService.getConfig().catch(() => null),
        ]);
        if (Boolean(creds?.connected) || Boolean(plexConfig?.source?.libraryId)) {
          setHasCompletedSetup(true);
        }
      } catch (error) { appLogger.warn('Failed to check existing configuration', error); }
    };
    void checkExistingConfig();
  }, [backendAvailable, hasCompletedSetup, hasLocalLibrary, isLibraryInitializing, setHasCompletedSetup]);

  useEffect(() => {
    if (!backendAvailable) return;
    let active = true;
    const generation = useStore.getState().spotifySessionGeneration;
    void api.getSpotifyAuthStatus().then(status => {
      if (!active || generation !== useStore.getState().spotifySessionGeneration) return;
      const connected = status.connected && !status.authRequired;
      setSpotifyConnected(connected);
      useStore.getState().setSpotifyAccountProduct(connected ? status.product : undefined);
    }).catch(() => {
      if (active && generation === useStore.getState().spotifySessionGeneration) setSpotifyConnected(false);
    });
    return () => {active=false;};
  }, [backendAvailable, setSpotifyConnected]);

  const loadAudioSettings = useStore(state => state.loadAudioSettings);
  useEffect(() => { if (backendAvailable) void loadAudioSettings(); }, [backendAvailable, loadAudioSettings]);

  // Wails' system-browser API keeps external links out of the WKWebView. This
  // avoids transient child windows and retains the application's origin.
  useEffect(() => {
    if (!shouldUseSystemBrowser()) return;

    const openLinkExternally = (event: MouseEvent) => {
      if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
      const target = event.target;
      if (!(target instanceof Element)) return;
      const link = target.closest('a[target="_blank"]') as HTMLAnchorElement | null;
      if (!link || !/^https?:$/i.test(link.protocol)) return;

      event.preventDefault();
      void openExternalURL(link.href);
    };

    document.addEventListener('click', openLinkExternally);
    return () => document.removeEventListener('click', openLinkExternally);
  }, []);

  return (
    <ErrorBoundary>
      <BrowserRouter>
        <Layout>
          <Suspense fallback={<p role="status" className="p-6 text-text-secondary">Loading page?</p>}>
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
            <Route path="/playlist/:playlistId" element={<PlaylistDetail />} />
            <Route path="/liked" element={<LikedSongs />} />
            <Route path="/liked-albums" element={<LikedAlbums />} />
            <Route path="/smart-mix/:mixId" element={<SmartMixDetail />} />
            <Route path="/spotify" element={<Spotify />} />
            <Route path="/spotify/artist/:id" element={<SpotifyArtistDetail />} />
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
          </Suspense>
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
        <Suspense fallback={null}>
          {backendAvailable && !isLibraryInitializing && !hasCompletedSetup && !hasLocalLibrary && (
            <FirstLaunchDialog isOpen onComplete={() => setHasCompletedSetup(true)} />
          )}
          {hasSongInfo && <SongInfoDialog />}
        </Suspense>
      </BrowserRouter>
    </ErrorBoundary>
  );
};

export default App;
