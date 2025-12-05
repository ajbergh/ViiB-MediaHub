/**
 * Playlist Context Menu
 *
 * Provides contextual actions for playlists such as Play, Play Next, Add to Queue and Delete.
 */
import React from 'react';
import { Play, SkipForward, ListPlus, Trash2 } from 'lucide-react';
import { useStore } from '../../store';
import { MenuItem } from './MenuShared';
import { Playlist } from '../../types';

/**
 * PlaylistMenu props:
 *  - playlist: Playlist object for menu actions
 *  - onClose: Callback invoked when the menu closes
 */
export const PlaylistMenu: React.FC<{ playlist: Playlist; onClose: () => void }> = ({ playlist, onClose }) => {
    const { songs, playSong, playNext, addToQueue, deletePlaylist, showConfirmDialog, closeConfirmDialog } = useStore();
    const playlistSongs = songs.filter(s => playlist.songIds.includes(s.id));

    const handleAction = (action: () => void) => {
        action();
        onClose();
    };

    const handleDeletePlaylist = () => {
        onClose();
        showConfirmDialog({
            title: 'Delete Playlist?',
            message: `Are you sure you want to delete "${playlist.name}"?`,
            confirmLabel: 'Delete',
            variant: 'danger',
            onConfirm: () => {
                deletePlaylist(playlist.id);
                closeConfirmDialog();
            },
        });
    };

    return (
        <>
            <div className="px-3 py-2 border-b border-[#333] mb-1">
                <div className="font-bold text-white truncate text-sm">{playlist.name}</div>
                <div className="text-xs text-gray-400 truncate">{playlist.songIds.length} songs</div>
            </div>

            <MenuItem icon={Play} label="Play Playlist" onClick={() => handleAction(() => {
                if (playlistSongs.length > 0) playSong(playlistSongs[0], playlistSongs);
            })} />
            <MenuItem icon={SkipForward} label="Play Next" onClick={() => handleAction(() => playNext(playlistSongs))} />
            <MenuItem icon={ListPlus} label="Add to Queue" onClick={() => handleAction(() => addToQueue(playlistSongs))} />
            
            <div className="border-t border-[#333] my-1"></div>
            
            <MenuItem icon={Trash2} label="Delete Playlist" onClick={handleDeletePlaylist} />
        </>
    );
};
