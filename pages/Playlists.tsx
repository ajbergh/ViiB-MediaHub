/**
 * ViiB MediaHub - Playlists Page
 * 
 * Manages user-created playlists with creation, import/export, and playback actions.
 * 
 * Features:
 * - Create new playlists with custom names
 * - Display playlist grid with cover art
 * - Context menu for playback, queue, and delete operations
 * - Empty state for new users
 * 
 * @module Playlists
 */

import { useNavigate } from 'react-router';
import React, { useMemo, useRef, useState } from 'react';
import { useAlbumCovers, useStore } from '../store';
import { Download, FileUp, Plus } from 'lucide-react';
import { ContextMenuType } from '../types';
import { EmptyPlaylists } from '../components/EmptyState';
import { Page, PageHeader } from '../components/ui/Page';
import { CardSizeSlider } from '../components/ui/CardSizeSlider';
import { api } from '../services/api';
import { PlaylistArtwork } from '../components/PlaylistArtwork';
import { getPlaylistArtwork } from '../lib/playlistArtwork';

export const Playlists: React.FC = () => {
  const navigate = useNavigate();
  const { playlists, createPlaylist, openContextMenu, refreshLibrary, showToast } = useStore();
  const songs = useStore(state => state.songs);
  const albumCovers = useAlbumCovers();
  const songsById = useMemo(() => new Map(songs.map(song => [song.id, song])), [songs]);
  const thumbnails = useMemo(() => new Map(playlists.map(playlist => [
    playlist.id, getPlaylistArtwork(playlist.songIds, songsById, albumCovers),
  ])), [playlists, songsById, albumCovers]);
  const [showInput, setShowInput] = useState(false);
  const [newPlaylistName, setNewPlaylistName] = useState('');
  const [cardCols, setCardCols] = useState(() => Number(localStorage.getItem('playlists-card-cols') ?? 5));
  const importInputRef = useRef<HTMLInputElement>(null);

  const handleImport = async (file?: File) => {
    if (!file) return;
    const content = await file.text();
    const name = file.name.replace(/\.(m3u8?|txt)$/i, '') || 'Imported Playlist';
    await api.importPlaylistM3U(name, content);
    await refreshLibrary();
    if (importInputRef.current) importInputRef.current.value = '';
  };

  const handleExport = async (id: string, name: string) => {
    const blob = await api.exportPlaylistM3U(id);
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = `${name.replace(/[^a-z0-9_-]+/gi, '-')}.m3u8`;
    anchor.click();
    URL.revokeObjectURL(url);
  };
  const handleCardColsChange = (v: number) => { setCardCols(v); localStorage.setItem('playlists-card-cols', String(v)); };

  const handleCreate = async () => {
    if (newPlaylistName.trim()) {
      try {
        await createPlaylist(newPlaylistName);
        setNewPlaylistName('');
        setShowInput(false);
      } catch { showToast({ type: 'error', message: 'Unable to create playlist. Please try again.' }); }
    }
  };

  return (
    <Page>
      <PageHeader
        heading="Playlists"
        actions={
          <div className="flex items-center gap-3">
          <input
            ref={importInputRef}
            type="file"
            accept=".m3u,.m3u8,audio/x-mpegurl"
            className="hidden"
            onChange={(event) => void handleImport(event.target.files?.[0])}
          />
          <button
            onClick={() => importInputRef.current?.click()}
            className="flex items-center gap-2 bg-surface-hover hover:bg-surface-border text-text-main px-4 py-2 rounded-full font-medium transition-colors text-sm"
          >
            <FileUp size={16} /> Import M3U
          </button>
          <button
            onClick={() => setShowInput(true)}
            className="flex items-center gap-2 bg-surface-hover hover:bg-surface-border text-text-main px-4 py-2 rounded-full font-medium transition-colors text-sm"
          >
            <Plus size={16} /> Create Playlist
          </button>
          <CardSizeSlider value={cardCols} onChange={handleCardColsChange} />
          </div>
        }
      />

      {showInput && (
        <div className="mb-8 p-6 bg-surface-2 rounded-xl border border-surface-border flex gap-4 items-center">
          <input 
            type="text" 
            placeholder="My Cool Playlist" 
            className="flex-1 bg-black border border-surface-border rounded px-4 py-2 text-white focus:border-brand outline-none"
            value={newPlaylistName}
            onChange={(e) => setNewPlaylistName(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleCreate()}
            autoFocus
          />
          <button onClick={handleCreate} className="bg-brand text-black font-bold px-4 py-2 rounded hover:bg-brand-hover">Save</button>
          <button onClick={() => setShowInput(false)} className="text-text-secondary hover:text-text-main">Cancel</button>
        </div>
      )}

      {playlists.length === 0 ? (
        <EmptyPlaylists onCreate={() => setShowInput(true)} />
      ) : (
        <div
          className="grid gap-6"
          style={{ gridTemplateColumns: `repeat(${cardCols}, minmax(0, 1fr))` }}
        >
            {playlists.map((pl) => (
                <div 
                    key={pl.id} 
                    className="bg-surface-2 p-4 rounded-lg hover:bg-surface-3 transition-all group cursor-pointer"
                    onContextMenu={(e) => openContextMenu(e, ContextMenuType.PLAYLIST, pl)}
                >
                    <button type="button" className="w-full text-left rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand" aria-label={`Open playlist ${pl.name}`} onClick={() => navigate(`/playlist/${encodeURIComponent(pl.id)}`)}>
                    <PlaylistArtwork
                      name={pl.name}
                      coverUrl={pl.coverUrl}
                      covers={thumbnails.get(pl.id) || []}
                      className="mb-4"
                    />
                    <h4 className="font-bold truncate text-text-main mb-1">{pl.name}</h4>
                    <p className="text-sm text-text-secondary">{pl.songIds.length} songs</p>
                    </button>
                    <button
                      type="button"
                      className="mt-3 inline-flex items-center gap-2 text-xs font-medium text-text-secondary hover:text-brand"
                      onClick={(event) => { event.stopPropagation(); void handleExport(pl.id, pl.name); }}
                    >
                      <Download size={14} /> Export M3U
                    </button>
                </div>
            ))}
        </div>
      )}
    </Page>
  );
};
