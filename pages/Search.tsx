import React, { useEffect, useMemo, useState } from 'react';
import { ListPlus, Music, Play, Search as SearchIcon } from 'lucide-react';
import { useLocation, useNavigate } from 'react-router-dom';
import { useStore } from '../store';
import { Song } from '../types';
import { libraryV2, ClientLibrarySearchResult } from '../services/libraryV2';
import { generateGradient, formatTime } from '../utils';
import { EmptySearchResults } from '../components/EmptyState';
import { TextInput } from '../components/ui/TextInput';
import { Page, PageHeader } from '../components/ui/Page';

type SearchTab = 'all' | 'tracks' | 'albums' | 'artists' | 'playlists';

const emptyResults = (query = ''): ClientLibrarySearchResult => ({
  query,
  tracks: [],
  albums: [],
  artists: [],
  playlists: [],
});

function localSearch(query: string, songs: Song[], playlists: ReturnType<typeof useStore.getState>['playlists']): ClientLibrarySearchResult {
  const normalized = query.trim().toLocaleLowerCase();
  if (!normalized) return emptyResults(query);

  const tracks = songs.filter(song =>
    song.title.toLocaleLowerCase().includes(normalized) ||
    song.artist.toLocaleLowerCase().includes(normalized) ||
    song.album.toLocaleLowerCase().includes(normalized) ||
    song.genre?.some(genre => genre.toLocaleLowerCase().includes(normalized)),
  ).slice(0, 100);

  const albums = new Map<string, ClientLibrarySearchResult['albums'][number]>();
  const artists = new Map<string, { name: string; songCount: number; albumNames: Set<string> }>();
  for (const song of songs) {
    const albumArtist = song.albumArtist || song.artist;
    if (song.album.toLocaleLowerCase().includes(normalized) || albumArtist.toLocaleLowerCase().includes(normalized)) {
      const key = `${song.album}\u0000${albumArtist}`;
      const existing = albums.get(key);
      if (existing) existing.songCount += 1;
      else albums.set(key, { name: song.album, artist: albumArtist, songCount: 1, coverPath: song.coverUrl });
    }
    if (song.artist.toLocaleLowerCase().includes(normalized)) {
      const existing = artists.get(song.artist) || { name: song.artist, songCount: 0, albumNames: new Set<string>() };
      existing.songCount += 1;
      existing.albumNames.add(song.album);
      artists.set(song.artist, existing);
    }
  }

  return {
    query,
    tracks,
    albums: [...albums.values()].slice(0, 100),
    artists: [...artists.values()].slice(0, 100).map(artist => ({
      name: artist.name,
      songCount: artist.songCount,
      albumCount: artist.albumNames.size,
    })),
    playlists: playlists
      .filter(playlist => playlist.name.toLocaleLowerCase().includes(normalized))
      .slice(0, 100)
      .map(playlist => ({ id: playlist.id, name: playlist.name, songCount: playlist.songIds.length })),
  };
}

export const Search: React.FC = () => {
  const location = useLocation();
  const navigate = useNavigate();
  const songs = useStore(state => state.songs);
  const playlists = useStore(state => state.playlists);
  const backendAvailable = useStore(state => state.backendAvailable);
  const playSong = useStore(state => state.playSong);
  const addToQueue = useStore(state => state.addToQueue);
  const showToast = useStore(state => state.showToast);
  const localSearchQuery = useStore(state => state.localSearchQuery);
  const localSearchTab = useStore(state => state.localSearchTab) as SearchTab;
  const setLocalSearchQuery = useStore(state => state.setLocalSearchQuery);
  const setLocalSearchTab = useStore(state => state.setLocalSearchTab);

  const locationQuery = (location.state as { query?: string } | null)?.query;
  const [input, setInput] = useState(locationQuery || localSearchQuery || '');
  const [query, setQuery] = useState(locationQuery || localSearchQuery || '');
  const [tab, setTab] = useState<SearchTab>(localSearchTab || 'all');
  const [results, setResults] = useState<ClientLibrarySearchResult>(emptyResults());
  const [loading, setLoading] = useState(false);
  const [searchError, setSearchError] = useState<string | null>(null);

  useEffect(() => {
    if (!locationQuery) return;
    setInput(locationQuery);
    setQuery(locationQuery);
    setLocalSearchQuery(locationQuery);
  }, [locationQuery, setLocalSearchQuery]);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setQuery(input.trim());
      setLocalSearchQuery(input.trim());
    }, 250);
    return () => window.clearTimeout(timer);
  }, [input, setLocalSearchQuery]);

  useEffect(() => {
    const controller = new AbortController();
    if (!query) {
      setResults(emptyResults());
      setLoading(false);
      setSearchError(null);
      return () => controller.abort();
    }

    const run = async () => {
      setLoading(true);
      setSearchError(null);
      try {
        const next = backendAvailable
          ? await libraryV2.search(query, 100, controller.signal)
          : localSearch(query, songs, playlists);
        setResults(next);
      } catch (error) {
        if (controller.signal.aborted) return;
        console.warn('Server search failed; using local fallback:', error);
        setResults(localSearch(query, songs, playlists));
        setSearchError('Server search was unavailable; showing local results.');
      } finally {
        if (!controller.signal.aborted) setLoading(false);
      }
    };
    void run();
    return () => controller.abort();
  }, [backendAvailable, playlists, query, songs]);

  const totalResults = results.tracks.length + results.albums.length + results.artists.length + results.playlists.length;
  const counts = useMemo(() => ({
    all: totalResults,
    tracks: results.tracks.length,
    albums: results.albums.length,
    artists: results.artists.length,
    playlists: results.playlists.length,
  }), [results, totalResults]);

  const selectTab = (next: SearchTab) => {
    setTab(next);
    setLocalSearchTab(next);
  };

  const playAlbum = (album: string, artist: string) => {
    const albumSongs = songs
      .filter(song => song.album === album && (song.albumArtist || song.artist) === artist)
      .sort((a, b) => (a.discNumber || 0) - (b.discNumber || 0) || (a.trackNumber || 0) - (b.trackNumber || 0));
    if (albumSongs[0]) playSong(albumSongs[0], albumSongs);
  };

  const addAlbum = (album: string, artist: string) => {
    const albumSongs = songs.filter(song => song.album === album && (song.albumArtist || song.artist) === artist);
    if (albumSongs.length) {
      addToQueue(albumSongs);
      showToast({ type: 'success', message: `Added ${albumSongs.length} tracks to queue` });
    }
  };

  return (
    <Page withPlayerPadding={false} className="flex flex-col">
      <PageHeader heading="Search" className="mb-6 flex-shrink-0" titleClassName="text-section mb-0" />

      <div className="w-full max-w-3xl mb-4 flex-shrink-0">
        <TextInput
          className="w-full"
          leftIcon={<SearchIcon size={18} className="text-text-secondary" aria-hidden="true" />}
          type="search"
          placeholder="Search tracks, albums, artists, genres, paths, and playlists"
          aria-label="Search library"
          value={input}
          onChange={event => setInput(event.target.value)}
          autoFocus
        />
      </div>

      {searchError && <p className="text-sm text-warning mb-3" role="status">{searchError}</p>}
      {query && (
        <div className="flex gap-2 mb-6 flex-wrap" role="tablist" aria-label="Search result categories">
          {(['all', 'tracks', 'albums', 'artists', 'playlists'] as SearchTab[]).map(item => (
            <button
              key={item}
              role="tab"
              aria-selected={tab === item}
              onClick={() => selectTab(item)}
              className={`px-3 py-1.5 rounded-full text-sm transition-colors ${tab === item ? 'bg-brand text-black' : 'bg-surface-2 text-text-secondary hover:text-text-main'}`}
            >
              {item[0].toUpperCase() + item.slice(1)} ({counts[item]})
            </button>
          ))}
        </div>
      )}

      {!query ? (
        <div className="flex flex-col items-center justify-center mt-12 opacity-60">
          <SearchIcon size={72} className="mb-4" />
          <h2 className="text-xl font-bold">Search your library</h2>
          <p className="text-sm">Backend mode uses the indexed, paginated search API.</p>
        </div>
      ) : loading && totalResults === 0 ? (
        <div className="text-text-secondary py-10" role="status">Searching…</div>
      ) : totalResults === 0 ? (
        <EmptySearchResults query={query} />
      ) : (
        <div className="flex-1 overflow-y-auto pb-32 space-y-8">
          {(tab === 'all' || tab === 'artists') && results.artists.length > 0 && (
            <section>
              <h2 className="text-xl font-bold mb-4">Artists</h2>
              <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-5 gap-3">
                {(tab === 'all' ? results.artists.slice(0, 5) : results.artists).map(artist => (
                  <button
                    key={artist.name}
                    onClick={() => navigate(`/artist/${encodeURIComponent(artist.name)}`)}
                    className="text-left bg-surface-1 hover:bg-surface-2 rounded-lg p-4 transition-colors"
                  >
                    <div className="aspect-square rounded-full mb-3 flex items-center justify-center" style={{ background: generateGradient(artist.name) }}>
                      <Music size={36} className="text-white/70" />
                    </div>
                    <div className="font-semibold truncate">{artist.name}</div>
                    <div className="text-sm text-text-secondary">{artist.songCount} tracks · {artist.albumCount} albums</div>
                  </button>
                ))}
              </div>
            </section>
          )}

          {(tab === 'all' || tab === 'albums') && results.albums.length > 0 && (
            <section>
              <h2 className="text-xl font-bold mb-4">Albums</h2>
              <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-5 gap-3">
                {(tab === 'all' ? results.albums.slice(0, 5) : results.albums).map(album => (
                  <div key={`${album.name}\u0000${album.artist}`} className="bg-surface-1 rounded-lg p-3 group">
                    <button className="w-full text-left" onClick={() => navigate(`/album/${encodeURIComponent(album.name)}/${encodeURIComponent(album.artist)}`)}>
                      <div className="aspect-square rounded-md overflow-hidden mb-3" style={{ background: generateGradient(album.name) }}>
                        {album.coverPath && <img src={album.coverPath} alt="" className="w-full h-full object-cover" />}
                      </div>
                      <div className="font-semibold truncate">{album.name}</div>
                      <div className="text-sm text-text-secondary truncate">{album.artist} · {album.songCount}</div>
                    </button>
                    <div className="flex gap-2 mt-3">
                      <button onClick={() => playAlbum(album.name, album.artist)} className="p-2 rounded-full bg-brand text-black" aria-label={`Play ${album.name}`}><Play size={15} fill="currentColor" /></button>
                      <button onClick={() => addAlbum(album.name, album.artist)} className="p-2 rounded-full bg-surface-3" aria-label={`Add ${album.name} to queue`}><ListPlus size={15} /></button>
                    </div>
                  </div>
                ))}
              </div>
            </section>
          )}

          {(tab === 'all' || tab === 'tracks') && results.tracks.length > 0 && (
            <section>
              <h2 className="text-xl font-bold mb-3">Tracks</h2>
              <div className="divide-y divide-border-subtle rounded-lg overflow-hidden bg-surface-1">
                {(tab === 'all' ? results.tracks.slice(0, 12) : results.tracks).map(track => (
                  <div key={track.id} className="flex items-center gap-3 px-4 py-3 hover:bg-surface-2">
                    <button onClick={() => playSong(track, results.tracks)} className="p-2 rounded-full hover:bg-surface-3" aria-label={`Play ${track.title}`}><Play size={16} fill="currentColor" /></button>
                    <div className="min-w-0 flex-1">
                      <div className="font-medium truncate">{track.title}</div>
                      <div className="text-sm text-text-secondary truncate">{track.artist} — {track.album}</div>
                    </div>
                    <div className="text-sm text-text-secondary tabular-nums">{formatTime(track.duration)}</div>
                    <button onClick={() => addToQueue(track)} className="p-2 rounded-full hover:bg-surface-3" aria-label={`Add ${track.title} to queue`}><ListPlus size={16} /></button>
                  </div>
                ))}
              </div>
            </section>
          )}

          {(tab === 'all' || tab === 'playlists') && results.playlists.length > 0 && (
            <section>
              <h2 className="text-xl font-bold mb-3">Playlists</h2>
              <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
                {results.playlists.map(playlist => (
                  <button key={playlist.id} onClick={() => navigate(`/playlist/${playlist.id}`)} className="text-left bg-surface-1 hover:bg-surface-2 rounded-lg p-4">
                    <div className="font-semibold truncate">{playlist.name}</div>
                    <div className="text-sm text-text-secondary">{playlist.songCount} tracks</div>
                  </button>
                ))}
              </div>
            </section>
          )}
        </div>
      )}
    </Page>
  );
};
