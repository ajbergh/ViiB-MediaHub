/**
 * ViiB MediaHub - Playlists Page
 * 
 * Manages user-created playlists with creation and editing capabilities.
 * 
 * Features:
 * - Create new playlists with custom names
 * - Display playlist grid with cover art
 * - Click to navigate to playlist detail
 * - Context menu for rename/delete operations
 * - Empty state for new users
 * 
 * @module Playlists
 */

import React, { useState } from 'react';
import { useStore } from '../store';
import { ListMusic, Plus } from 'lucide-react';
import { ContextMenuType } from '../types';
import { EmptyPlaylists } from '../components/EmptyState';
import { Page, PageHeader } from '../components/ui/Page';
import { CardSizeSlider } from '../components/ui/CardSizeSlider';

export const Playlists: React.FC = () => {
  const { playlists, createPlaylist, openContextMenu } = useStore();
  const [showInput, setShowInput] = useState(false);
  const [newPlaylistName, setNewPlaylistName] = useState('');
  const [cardCols, setCardCols] = useState(() => Number(localStorage.getItem('playlists-card-cols') ?? 5));
  const handleCardColsChange = (v: number) => { setCardCols(v); localStorage.setItem('playlists-card-cols', String(v)); };

  const handleCreate = () => {
    if (newPlaylistName.trim()) {
      createPlaylist(newPlaylistName);
      setNewPlaylistName('');
      setShowInput(false);
    }
  };

  return (
    <Page>
      <PageHeader
        heading="Playlists"
        actions={
          <div className="flex items-center gap-3">
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
                    <div className="w-full aspect-square bg-surface-1 rounded-md mb-4 flex items-center justify-center shadow-lg relative overflow-hidden">
                        <ListMusic size={40} className="text-surface-border" />
                    </div>
                    <h4 className="font-bold truncate text-text-main mb-1">{pl.name}</h4>
                    <p className="text-sm text-text-secondary">{pl.songIds.length} songs</p>
                </div>
            ))}
        </div>
      )}
    </Page>
  );
};