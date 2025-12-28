/**
 * SmartMix Context Menu
 *
 * Actions available for Smart Mixes such as Play, Play Next, Add to Queue, Save as Playlist, and Refresh.
 */
import React from 'react';
import { Play, SkipForward, ListPlus, Save, RefreshCw, Sparkles } from 'lucide-react';
import { useStore } from '../../store';
import { MenuItem } from './MenuShared';
import { SmartMix } from '../../types';

/**
 * SmartMixMenu props:
 *  - mix: SmartMix object for menu actions
 *  - onClose: Callback invoked when the menu closes
 */
export const SmartMixMenu: React.FC<{ mix: SmartMix; onClose: () => void }> = ({ mix, onClose }) => {
    const { songs, playSong, playNext, addToQueue, saveSmartMixAsPlaylist, refreshSmartMixes } = useStore();
    const mixSongs = mix.songIds.map((id: string) => songs.find(s => s.id === id)).filter(Boolean) as any[];

    const handleAction = (action: () => void) => {
        action();
        onClose();
    };

    return (
        <>
            <div className="px-3 py-2 border-b border-surface-border mb-1">
                <div className="font-bold text-text-main truncate text-sm flex items-center gap-1">
                    <Sparkles size={12} className="text-brand" />
                    {mix.name}
                </div>
                <div className="text-xs text-text-secondary truncate">{mix.songIds.length} tracks</div>
            </div>

            <MenuItem icon={Play} label="Play Mix" onClick={() => handleAction(() => {
                if (mixSongs.length > 0) playSong(mixSongs[0], mixSongs);
            })} />
            <MenuItem icon={SkipForward} label="Play Next" onClick={() => handleAction(() => playNext(mixSongs))} />
            <MenuItem icon={ListPlus} label="Add to Queue" onClick={() => handleAction(() => addToQueue(mixSongs))} />
            
            <div className="border-t border-surface-border my-1"></div>
            
            <MenuItem icon={Save} label="Save as Playlist" onClick={() => handleAction(() => {
                    saveSmartMixAsPlaylist(mix.id);
                    alert(`Saved "${mix.name}" to your playlists.`);
            })} />
            
            <MenuItem icon={RefreshCw} label="Refresh Mix" onClick={() => handleAction(() => refreshSmartMixes())} />
        </>
    );
};
