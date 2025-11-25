import React from 'react';
import { Plus, ListMusic } from 'lucide-react';
import { useStore } from '../../store';

export const MenuItem: React.FC<{ icon: any; label: string; onClick: () => void }> = ({ icon: Icon, label, onClick }) => (
    <button 
        onClick={(e) => { e.stopPropagation(); onClick(); }}
        className="w-full text-left px-4 py-2 text-sm hover:bg-surface-border hover:text-white flex items-center gap-3 transition-colors group"
    >
        <Icon size={16} className="text-gray-400 group-hover:text-white" />
        <span>{label}</span>
    </button>
);

export const PlaylistsSubmenu: React.FC<{ songId: string; onClose: () => void }> = ({ songId, onClose }) => {
    const { playlists, addToPlaylist, createPlaylist } = useStore();

    const handleAddToPlaylist = (playlistId: string) => {
        addToPlaylist(playlistId, songId);
        onClose();
    };

    const handleCreatePlaylist = () => {
        const name = prompt("New Playlist Name:");
        if (name) {
            createPlaylist(name);
        }
        onClose();
    };

    return (
        <div className="absolute left-full top-0 ml-1 w-48 bg-surface-3 rounded-md shadow-xl border border-surface-border z-50 overflow-hidden py-1">
            <button 
                className="w-full text-left px-4 py-2 text-sm text-white hover:bg-surface-slider flex items-center gap-2"
                onClick={handleCreatePlaylist}
            >
                <Plus size={14} /> New Playlist
            </button>
            <div className="border-t border-surface-slider my-1"></div>
            {playlists.length === 0 ? (
                <div className="px-4 py-2 text-xs text-gray-500 italic">No playlists</div>
            ) : (
                <div className="max-h-48 overflow-y-auto">
                    {playlists.map(pl => (
                        <button
                            key={pl.id}
                            className="w-full text-left px-4 py-2 text-sm text-white hover:bg-surface-slider truncate"
                            onClick={() => handleAddToPlaylist(pl.id)}
                        >
                            {pl.name}
                        </button>
                    ))}
                </div>
            )}
        </div>
    );
};