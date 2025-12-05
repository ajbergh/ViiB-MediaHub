import React from 'react';
import { Play, SkipForward, ListPlus, Save, RefreshCw, Sparkles } from 'lucide-react';
import { useStore } from '../../store';
import { MenuItem } from './MenuShared';
import { SmartMix } from '../../types';

export const SmartMixMenu: React.FC<{ mix: SmartMix; onClose: () => void }> = ({ mix, onClose }) => {
    const { songs, playSong, playNext, addToQueue, saveSmartMixAsPlaylist, refreshSmartMixes } = useStore();
    const mixSongs = mix.songIds.map((id: string) => songs.find(s => s.id === id)).filter(Boolean) as any[];

    const handleAction = (action: () => void) => {
        action();
        onClose();
    };

    return (
        <>
            <div className="px-3 py-2 border-b border-[#333] mb-1">
                <div className="font-bold text-white truncate text-sm flex items-center gap-1">
                    <Sparkles size={12} className="text-yellow-500" />
                    {mix.name}
                </div>
                <div className="text-xs text-gray-400 truncate">{mix.songIds.length} tracks</div>
            </div>

            <MenuItem icon={Play} label="Play Mix" onClick={() => handleAction(() => {
                if (mixSongs.length > 0) playSong(mixSongs[0], mixSongs);
            })} />
            <MenuItem icon={SkipForward} label="Play Next" onClick={() => handleAction(() => playNext(mixSongs))} />
            <MenuItem icon={ListPlus} label="Add to Queue" onClick={() => handleAction(() => addToQueue(mixSongs))} />
            
            <div className="border-t border-[#333] my-1"></div>
            
            <MenuItem icon={Save} label="Save as Playlist" onClick={() => handleAction(() => {
                    saveSmartMixAsPlaylist(mix.id);
                    alert(`Saved "${mix.name}" to your playlists.`);
            })} />
            
            <MenuItem icon={RefreshCw} label="Refresh Mix" onClick={() => handleAction(() => refreshSmartMixes())} />
        </>
    );
};
