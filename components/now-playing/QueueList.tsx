import React, { useState } from 'react';
import { Play, GripVertical, Trash2 } from 'lucide-react';
import { useStore, useAlbumCovers } from '../../store';
import { formatTime, generateGradient } from '../../utils';
import { ContextMenuType } from '../../types';

interface Props {
    queue: any[];
    currentSongIndex: number;
}

export const QueueList: React.FC<Props> = ({ queue, currentSongIndex }) => {
    const { playQueueItem, removeFromQueue, reorderQueue, openContextMenu } = useStore();
    const albumCovers = useAlbumCovers();
    const [draggedItemIndex, setDraggedItemIndex] = useState<number | null>(null);

    const handleDragStart = (e: React.DragEvent, index: number) => {
        setDraggedItemIndex(index);
        e.dataTransfer.effectAllowed = "move";
    };

    const handleDragOver = (e: React.DragEvent, index: number) => {
        e.preventDefault();
        if (draggedItemIndex === null || draggedItemIndex === index) return;
    };

    const handleDrop = (e: React.DragEvent, index: number) => {
        e.preventDefault();
        if (draggedItemIndex !== null && draggedItemIndex !== index) {
            reorderQueue(draggedItemIndex, index);
        }
        setDraggedItemIndex(null);
    };

    return (
        <div className="space-y-1">
            {queue.map((song, idx) => {
                const isCurrent = idx === currentSongIndex;
                const displayCover = song.coverUrl || albumCovers[song.album];
                
                return (
                    <div 
                        key={`${song.id}-${idx}`}
                        draggable
                        onDragStart={(e) => handleDragStart(e, idx)}
                        onDragOver={(e) => handleDragOver(e, idx)}
                        onDrop={(e) => handleDrop(e, idx)}
                        onContextMenu={(e) => openContextMenu(e, ContextMenuType.QUEUE_ITEM, { song, index: idx })}
                        className={`group flex items-center gap-4 p-3 rounded-lg border border-transparent transition-all ${
                            isCurrent 
                                ? 'bg-white/10 border-green-500/50' 
                                : 'hover:bg-white/5 border-transparent'
                        } ${draggedItemIndex === idx ? 'opacity-40' : ''}`}
                    >
                        <div className="cursor-grab active:cursor-grabbing text-white/30 group-hover:text-white/60">
                            <GripVertical size={20} />
                        </div>
                        
                        <div className="w-12 h-12 rounded bg-[#282828] relative flex-shrink-0 overflow-hidden">
                                <img 
                                src={displayCover || ''} 
                                className="w-full h-full object-cover"
                                style={{ background: !displayCover ? generateGradient(song.album) : undefined }}
                                alt="" 
                                />
                                <div 
                                className="absolute inset-0 bg-black/40 hidden group-hover:flex items-center justify-center cursor-pointer"
                                onClick={() => playQueueItem(idx)}
                                >
                                <Play size={20} className="fill-white" />
                                </div>
                        </div>

                        <div className="flex-1 min-w-0">
                            <div className={`font-bold truncate ${isCurrent ? 'text-green-400' : 'text-white'}`}>{song.title}</div>
                            <div className="text-sm text-white/50 truncate">{song.artist}</div>
                        </div>
                        
                        <div className="text-sm font-mono text-white/30">{formatTime(song.duration)}</div>

                        <button 
                            onClick={(e) => { e.stopPropagation(); removeFromQueue(idx); }}
                            className="p-2 text-white/30 hover:text-red-400 opacity-0 group-hover:opacity-100 transition-opacity"
                        >
                            <Trash2 size={18} />
                        </button>
                    </div>
                )
            })}
        </div>
    );
};
