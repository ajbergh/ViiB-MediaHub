import React, { useMemo, useState } from 'react';
import { Link, useParams } from 'react-router';
import { ArrowDown, ArrowUp, Play, Plus, Search, Trash2 } from 'lucide-react';
import { useAlbumCovers, useStore } from '../store';
import { Playlist, Song } from '../types';
import { Page, PageHeader } from '../components/ui/Page';
import { TextInput } from '../components/ui/TextInput';
import { PlaylistArtwork } from '../components/PlaylistArtwork';
import { getPlaylistArtwork } from '../lib/playlistArtwork';
import { resolvePlaylistSongs } from '../lib/playlistContents';
import { formatTime } from '../utils';

const buttonClass = 'rounded-lg px-3 py-2 text-sm font-medium bg-surface-2 hover:bg-surface-3 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand disabled:opacity-40 disabled:cursor-not-allowed';

const PlaylistEditor: React.FC<{ playlist: Playlist }> = ({ playlist }) => {
  const songs = useStore(state => state.songs);
  const updatePlaylistContents = useStore(state => state.updatePlaylistContents);
  const playSong = useStore(state => state.playSong);
  const albumCovers = useAlbumCovers();
  const [draft, setDraft] = useState<string[] | null>(null);
  const [baseIds, setBaseIds] = useState<string[] | null>(null);
  const [query, setQuery] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [saved, setSaved] = useState(false);
  const songIds = draft ?? playlist.songIds;
  const dirty = draft !== null && (draft.length !== playlist.songIds.length || draft.some((id, i) => id !== playlist.songIds[i]));
  const songsById = useMemo(() => new Map(songs.map(song => [song.id, song])), [songs]);
  const covers = useMemo(() => getPlaylistArtwork(songIds, songsById, albumCovers), [songIds, songsById, albumCovers]);
  const matches = useMemo(() => {
    const normalized = query.trim().toLocaleLowerCase();
    const result: Song[] = [];
    for (const song of songs) {
      if (!normalized || [song.title, song.artist, song.album].some(value => value.toLocaleLowerCase().includes(normalized))) result.push(song);
      if (result.length === 50) break;
    }
    return result;
  }, [query, songs]);
  const edit = (ids: string[]) => {
    if (baseIds === null) setBaseIds([...playlist.songIds]);
    setDraft(ids); setError(''); setSaved(false);
  };
  const move = (from: number, to: number) => {
    const ids = [...songIds];
    const [id] = ids.splice(from, 1); ids.splice(to, 0, id); edit(ids);
  };
  const save = async () => {
    setSaving(true); setError('');
    try {
      await updatePlaylistContents(playlist.id, songIds, baseIds ?? playlist.songIds);
      setDraft(null); setBaseIds(null); setSaved(true);
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : 'Unable to save playlist. Please try again.');
    } finally { setSaving(false); }
  };
  const availableSongs = resolvePlaylistSongs(songIds, songsById);

  return (
    <Page>
      <Link to="/playlists" className="text-sm text-text-secondary hover:text-text-main">← Playlists</Link>
      <div className="mt-6 flex items-center gap-6">
        <div className="w-28 sm:w-36 shrink-0"><PlaylistArtwork name={playlist.name} coverUrl={playlist.coverUrl} covers={covers} /></div>
        <div className="min-w-0 flex-1">
          <PageHeader heading={playlist.name} subtitle={`${songIds.length} tracks`} />
          <button type="button" className={buttonClass} disabled={availableSongs.length === 0 || saving} onClick={() => void playSong(availableSongs[0], availableSongs)}><Play size={14} className="inline mr-2" />Play playlist</button>
        </div>
      </div>
      <div className="mt-6 mb-4 flex items-center gap-3">
        <button type="button" className={`${buttonClass.replace('bg-surface-2 hover:bg-surface-3', 'bg-brand hover:bg-brand-hover')} text-black`} disabled={!dirty || saving} onClick={() => void save()}>{saving ? 'Saving…' : 'Save changes'}</button>
        <button type="button" className={buttonClass} disabled={draft === null || saving} onClick={() => { setDraft(null); setBaseIds(null); setError(''); setSaved(false); }}>Cancel</button>
        {dirty && <span className="text-sm text-text-secondary">Unsaved changes</span>}
        {saved && <span role="status" className="text-sm text-text-secondary">Playlist saved</span>}
      </div>
      {error && <p role="alert" className="mb-4 text-error">{error}</p>}
      <fieldset disabled={saving} className="min-w-0">
        <legend className="text-lg font-semibold mb-3">Playlist contents</legend>
        <p className="mb-3 text-sm text-text-secondary">Use the arrows to reorder tracks. Removing a track only removes it from this playlist.</p>
        {songIds.length === 0 ? <p className="py-6 text-text-secondary">No tracks yet. Add tracks from your library below.</p> : (
          <ol className="max-h-[50vh] overflow-y-auto rounded-lg border border-surface-border divide-y divide-surface-border" aria-label="Playlist tracks">
            {songIds.map((id, index) => {
              const song = songsById.get(id);
              const title = song?.title || 'Unavailable track';
              return <li key={`${index}:${id}`} className="flex items-center gap-3 p-3 bg-surface-1">
                <span className="w-6 shrink-0 text-sm text-text-subtle">{index + 1}</span>
                <div className="min-w-0 flex-1"><p className="truncate font-medium">{title}</p><p className="truncate text-sm text-text-secondary">{song ? `${song.artist} · ${song.album}` : 'This track is not currently in your library.'}</p></div>
                {song && <span className="hidden sm:block text-sm text-text-secondary">{formatTime(song.duration)}</span>}
                <button type="button" className={buttonClass} aria-label={`Move track ${index + 1} up`} disabled={index === 0} onClick={() => move(index, index - 1)}><ArrowUp size={16} /></button>
                <button type="button" className={buttonClass} aria-label={`Move track ${index + 1} down`} disabled={index === songIds.length - 1} onClick={() => move(index, index + 1)}><ArrowDown size={16} /></button>
                <button type="button" className={`${buttonClass} text-error`} aria-label={`Remove track ${index + 1}: ${title}`} onClick={() => edit(songIds.filter((_, i) => i !== index))}><Trash2 size={16} /></button>
              </li>;
            })}
          </ol>
        )}
        <h2 className="mt-8 mb-3 text-lg font-semibold">Add tracks</h2>
        <TextInput aria-label="Find tracks to add" placeholder="Search by title, artist, or album" value={query} onChange={event => setQuery(event.target.value)} leftIcon={<Search size={16} />} />
        <p className="my-3 text-sm text-text-secondary">Showing up to 50 matches. Search to narrow the list. You can add the same track more than once.</p>
        {matches.length === 0 ? <p className="text-text-secondary">No matching tracks.</p> : <ul className="max-h-80 overflow-y-auto divide-y divide-surface-border">
          {matches.map(song => <li key={song.id} className="flex items-center gap-3 py-3">
            <div className="min-w-0 flex-1"><p className="truncate font-medium">{song.title}</p><p className="truncate text-sm text-text-secondary">{song.artist} · {song.album}</p></div>
            <button type="button" className={buttonClass} aria-label={`Add ${song.title} to playlist`} onClick={() => edit([...songIds, song.id])}><Plus size={16} className="inline mr-1" />Add</button>
          </li>)}
        </ul>}
      </fieldset>
    </Page>
  );
};

export const PlaylistDetail: React.FC = () => {
  const { playlistId } = useParams<{ playlistId: string }>();
  const playlist = useStore(state => state.playlists.find(item => item.id === playlistId));
  const initializing = useStore(state => state.isLibraryInitializing);
  if (!playlist) return <Page><PageHeader heading={initializing ? 'Loading playlist…' : 'Playlist not found'} /><Link to="/playlists">Back to Playlists</Link></Page>;
  return <PlaylistEditor key={playlist.id} playlist={playlist} />;
};
