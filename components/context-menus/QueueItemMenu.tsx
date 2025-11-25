import React from 'react';
import { Play, ArrowUp, Trash2, Disc } from 'lucide-react';
import { useStore } from '../../store';
import { MenuItem } from './MenuShared';
import { useNavigate } from 'react-router-dom';
import { Song } from '../../types';

export const QueueItemMenu: React.FC<{ song: Song; index: number; onClose: () => void }> = ({ song, index, onClose }) => {
    const { playQueueItem, reorderQueue, removeFromQueue } = useStore();
    const navigate = useNavigate();

    const handleAction = (action: () => void) => {
        action();
        onClose();
    };

    return (
        <>
            <div className="px-3 py-2 border-b border-[#333] mb-1">
                <div className="font-bold text-white truncate text-sm">{song.title}</div>
                <div className="text-xs text-gray-400 truncate">Queue Position: {index + 1}</div>
            </div>

            <MenuItem icon={Play} label="Play This Song" onClick={() => handleAction(() => {
                    playQueueItem(index);
            })} />
            
            <MenuItem icon={ArrowUp} label="Move to Top" onClick={() => handleAction(() => reorderQueue(index, 0))} />
            <MenuItem icon={Trash2} label="Remove from Queue" onClick={() => handleAction(() => removeFromQueue(index))} />

            <div className="border-t border-[#333] my-1"></div>
            
            <MenuItem icon={Disc} label="Go to Album" onClick={() => {
                navigate(`/album/${encodeURIComponent(song.album)}`);
                onClose();
            }} />
        </>
    );
};
