import React, { useEffect } from 'react';
import { MemoryRouter, Routes, Route, Navigate } from 'react-router-dom';
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

const App: React.FC = () => {
  const initLibrary = useStore(state => state.initLibrary);

  useEffect(() => {
    initLibrary();
  }, [initLibrary]);

  return (
    <MemoryRouter>
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
    </MemoryRouter>
  );
};

export default App;