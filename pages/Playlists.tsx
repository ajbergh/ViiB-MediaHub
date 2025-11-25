import React, { useState } from 'react';
import { useStore } from '../store';
import { ListMusic, Plus } from 'lucide-react';
import { ContextMenuType } from '../types';

export const Playlists: React.FC = () => {
  const { playlists, createPlaylist, openContextMenu } = useStore();
  const [showInput, setShowInput] = useState(false);
  const [newPlaylistName, setNewPlaylistName] = useState('');

  const handleCreate = () => {
    if (newPlaylistName.trim()) {
      createPlaylist(newPlaylistName);
      setNewPlaylistName('');
      setShowInput(false);
    }
  };

  return (
    <div className="p-8 pb-32">
      <div className="flex justify-between items-center mb-8">
        <h1 className="text-3xl font-bold">Playlists</h1>
        <button 
            onClick={() => setShowInput(true)}
            className="flex items-center gap-2 bg-surface-hover hover:bg-surface-border text-text-main px-4 py-2 rounded-full font-medium transition-colors text-sm"
        >
            <Plus size={16} /> Create Playlist
        </button>
      </div>

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
        <div className="flex flex-col items-center justify-center mt-20 text-center">
            <ListMusic size={64} className="text-surface-border mb-4" />
            <h3 className="text-xl font-bold text-text-main mb-2">No playlists yet</h3>
            <p className="text-text-subtle mb-6">Create your first playlist to organize your collection.</p>
            <button 
                onClick={() => setShowInput(true)}
                className="bg-brand hover:bg-brand-hover text-black font-bold py-3 px-8 rounded-full transition-all"
            >
                Create Playlist
            </button>
        </div>
      ) : (
        <div className="grid grid-cols-2 md:grid-cols-4 lg:grid-cols-5 gap-6">
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
    </div>
  );
};