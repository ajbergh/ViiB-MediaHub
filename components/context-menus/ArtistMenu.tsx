import React from 'react';
import { Play, ListPlus } from 'lucide-react';
import { useStore } from '../../store';
import { MenuItem } from './MenuShared';
import { Artist } from '../../types';

export const ArtistMenu: React.FC<{ artist: Artist; onClose: () => void }> = ({ artist, onClose }) => {
    const { songs, playSong, addToQueue } = useStore();
    const artistSongs = songs.filter(s => s.artist === artist.name);

    const handleAction = (action: () => void) => {
        action();
        onClose();
    };

    return (
        <>
            <div className="px-3 py-2 border-b border-[#333] mb-1">
                <div className="font-bold text-white truncate text-sm">{artist.name}</div>
                <div className="text-xs text-gray-400 truncate">{artist.songCount} songs</div>
            </div>

            <MenuItem icon={Play} label="Play Top Songs" onClick={() => handleAction(() => {
                if (artistSongs.length > 0) playSong(artistSongs[0], artistSongs);
            })} />
            <MenuItem icon={ListPlus} label="Add All to Queue" onClick={() => handleAction(() => addToQueue(artistSongs))} />
        </>
    );
};
