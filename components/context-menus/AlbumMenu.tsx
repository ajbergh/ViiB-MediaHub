/**
 * Album Context Menu
 *
 * Provides contextual actions for an album such as Play, Play Next, and Add to Queue.
 * Integrated into the right-click context menu system.
 */
import React from 'react';
import { Play, SkipForward, ListPlus, Mic2 } from 'lucide-react';
import { useStore } from '../../store';
import { MenuItem } from './MenuShared';
import { useNavigate } from 'react-router-dom';
import { Album } from '../../types';

/**
 * AlbumMenu props:
 *  - album: Album object for which to show the menu
 *  - onClose: Callback invoked when the menu closes
 */
export const AlbumMenu: React.FC<{ album: Album; onClose: () => void }> = ({ album, onClose }) => {
    const { songs, playSong, playNext, addToQueue } = useStore();
    const navigate = useNavigate();
    const albumSongs = songs.filter(s => s.album === album.name);

    const handleAction = (action: () => void) => {
        action();
        onClose();
    };

    return (
        <>
            <div className="px-3 py-2 border-b border-surface-border mb-1">
                <div className="font-bold text-text-main truncate text-sm">{album.name}</div>
                <div className="text-xs text-text-secondary truncate">{album.artist}</div>
            </div>

            <MenuItem icon={Play} label="Play Album" onClick={() => handleAction(() => {
                if (albumSongs.length > 0) playSong(albumSongs[0], albumSongs);
            })} />
            <MenuItem icon={SkipForward} label="Play Next" onClick={() => handleAction(() => playNext(albumSongs))} />
            <MenuItem icon={ListPlus} label="Add to Queue" onClick={() => handleAction(() => addToQueue(albumSongs))} />
            
            <div className="border-t border-surface-border my-1"></div>
            
            <MenuItem icon={Mic2} label="Go to Artist" onClick={() => {
                navigate('/artists');
                onClose();
            }} />
        </>
    );
};
