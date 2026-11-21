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
import { Button } from '../components/ui/Button';
import { CardSizeSlider, MAX_CARD_COLUMNS } from '../components/ui/CardSizeSlider';
import { LibraryListHeader, LibraryListRow } from '../components/ui/LibraryList';
import { api } from '../services/api';
import { PlaylistArtwork } from '../components/PlaylistArtwork';
import { getPlaylistArtwork } from '../lib/playlistArtwork';
import { indexPlaylistSongs } from '../lib/playlistContents';

export const Playlists: React.FC = () => {
  const navigate = useNavigate();
  const playlists = useStore(state => state.playlists);
  const createPlaylist = useStore(state => state.createPlaylist);
  const openContextMenu = useStore(state => state.openContextMenu);
  const refreshLibrary = useStore(state => state.refreshLibrary);
  const showToast = useStore(state => state.showToast);
  const songs = useStore(state => state.songs);
  const albumCovers = useAlbumCovers();
  const songsById = indexPlaylistSongs(songs);
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
  const listView = cardCols === MAX_CARD_COLUMNS;
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
        subtitle={`${playlists.length} ${playlists.length === 1 ? 'playlist' : 'playlists'}`}
        actions={
          <div className="flex flex-wrap items-center gap-2.5">
          <input
            ref={importInputRef}
            type="file"
            accept=".m3u,.m3u8,audio/x-mpegurl"
            className="hidden"
            onChange={(event) => void handleImport(event.target.files?.[0])}
          />
          <Button
            variant="secondary"
            size="sm"
            leftIcon={<FileUp size={15} />}
            onClick={() => importInputRef.current?.click()}
          >
            Import M3U
          </Button>
          <Button
            variant="primary"
            size="sm"
            leftIcon={<Plus size={15} />}
            onClick={() => setShowInput(true)}
          >
            Create Playlist
          </Button>
          <CardSizeSlider listAtMinimum value={cardCols} onChange={handleCardColsChange} />
          </div>
        }
      />

      {showInput && (
        <div className="mb-6 p-4 bg-surface-2 rounded-lg border border-surface-border flex gap-3 items-center">
          <input 
            type="text" 
            placeholder="My Playlist #1"
            aria-label="Playlist name"
            className="flex-1 bg-surface-3 border border-transparent focus:border-surface-slider rounded-full px-4 py-2 text-sm text-text-main outline-none"
            value={newPlaylistName}
            onChange={(e) => setNewPlaylistName(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleCreate()}
            autoFocus
          />
          <Button variant="primary" size="sm" onClick={handleCreate}>Save</Button>
          <Button variant="ghost" size="sm" onClick={() => setShowInput(false)}>Cancel</Button>
        </div>
      )}

      {playlists.length === 0 ? (
        <EmptyPlaylists onCreate={() => setShowInput(true)} />
      ) : (
        <div style={{ '--card-cols': listView ? 1 : cardCols } as React.CSSProperties}>
        {listView && <LibraryListHeader title="Playlist" stats="Tracks" />}
        <div
          className={listView ? 'grid grid-cols-1 gap-0' : 'grid grid-cols-2 sm:grid-cols-3 md:grid-cols-[repeat(var(--card-cols,4),minmax(0,1fr))] gap-2'}
        >
            {playlists.map((pl, index) => listView ? (
                <LibraryListRow key={pl.id} index={index} name={pl.name}
                  artwork={<PlaylistArtwork name={pl.name} coverUrl={pl.coverUrl} covers={thumbnails.get(pl.id) || []} />}
                  stats={String(pl.songIds.length)} onOpen={() => navigate(`/playlist/${encodeURIComponent(pl.id)}`)}
                  onContextMenu={e => openContextMenu(e, ContextMenuType.PLAYLIST, pl)}
                  actions={<button type="button" aria-label={`Export ${pl.name} as M3U`} className="min-w-[32px] min-h-[32px] inline-flex items-center justify-center p-2 text-text-secondary hover:text-brand" onClick={event => { event.stopPropagation(); void handleExport(pl.id, pl.name); }}><Download size={16} /></button>} />
            ) : (
                <div 
                    key={pl.id} 
                    className="bg-transparent p-3 rounded-lg hover:bg-surface-hover transition-colors group cursor-pointer"
                    onContextMenu={(e) => openContextMenu(e, ContextMenuType.PLAYLIST, pl)}
                >
                    <button type="button" className="w-full text-left rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand" aria-label={`Open playlist ${pl.name}`} onClick={() => navigate(`/playlist/${encodeURIComponent(pl.id)}`)}>
                    <PlaylistArtwork
                      name={pl.name}
                      coverUrl={pl.coverUrl}
                      covers={thumbnails.get(pl.id) || []}
                      className="mb-3 shadow-xl"
                    />
                    <h4 className="font-semibold text-sm truncate text-text-main mb-1">{pl.name}</h4>
                    <p className="text-xs text-text-secondary">{pl.songIds.length} songs</p>
                    </button>
                    <button
                      type="button"
                      className="mt-2 min-h-[32px] px-2 inline-flex items-center gap-1.5 text-xs font-medium text-text-subtle hover:text-text-main transition-colors rounded-md"
                      onClick={(event) => { event.stopPropagation(); void handleExport(pl.id, pl.name); }}
                    >
                      <Download size={13} /> Export M3U
                    </button>
                </div>
            ))}
        </div>
        </div>
      )}
    </Page>
  );
};
