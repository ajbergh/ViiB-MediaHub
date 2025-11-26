import React, { useEffect } from 'react';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { Layout } from './components/Layout';
import { Home } from './pages/Home';
import { Songs } from './pages/Songs';
import { Albums } from './pages/Albums';
import { AlbumDetail } from './pages/AlbumDetail';
import { Artists } from './pages/Artists';
import { Playlists } from './pages/Playlists';
import { Spotify } from './pages/Spotify';
import { SpotifyCallback } from './pages/SpotifyCallback';
import { Downloads } from './pages/Downloads';
import { Search } from './pages/Search';
import { Settings } from './pages/Settings';
import { SmartMixDetail } from './pages/SmartMixDetail';
import { useStore } from './store';
import { api } from './services/api';

const App: React.FC = () => {
  const initLibrary = useStore(state => state.initLibrary);
  const { spotifyAccessToken, setSpotifyTokens, setSpotifyCredentials } = useStore();

  useEffect(() => {
    initLibrary();
  }, [initLibrary]);

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

  return (
    <BrowserRouter>
      <Layout>
        <Routes>
          <Route path="/" element={<Home />} />
          <Route path="/songs" element={<Songs />} />
          <Route path="/albums" element={<Albums />} />
          <Route path="/album/:albumName" element={<AlbumDetail />} />
          <Route path="/artists" element={<Artists />} />
          <Route path="/playlists" element={<Playlists />} />
          <Route path="/smart-mix/:mixId" element={<SmartMixDetail />} />
          <Route path="/spotify" element={<Spotify />} />
          <Route path="/callback" element={<SpotifyCallback />} />
          <Route path="/downloads" element={<Downloads />} />
          <Route path="/search" element={<Search />} />
          <Route path="/settings" element={<Settings />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </Layout>
    </BrowserRouter>
  );
};

export default App;