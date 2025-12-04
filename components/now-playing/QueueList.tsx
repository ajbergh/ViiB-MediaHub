import React, { useState } from 'react';
import { Play, GripVertical, Trash2, Download, CheckCircle, Loader2 } from 'lucide-react';
import { useStore, useAlbumCovers } from '../../store';
import { formatTime, generateGradient } from '../../utils';
import { ContextMenuType } from '../../types';
import { api } from '../../services/api';

interface Props {
    queue: any[];
    currentSongIndex: number;
}

export const QueueList: React.FC<Props> = ({ queue, currentSongIndex }) => {
    const { playQueueItem, removeFromQueue, reorderQueue, openContextMenu, showToast } = useStore();
    const albumCovers = useAlbumCovers();
    const [draggedItemIndex, setDraggedItemIndex] = useState<number | null>(null);
    const [downloadingIds, setDownloadingIds] = useState<Set<string>>(new Set());
    
    const handleDownloadTrack = async (song: any, e: React.MouseEvent) => {
        e.stopPropagation();
        if (!song.spotifyId || downloadingIds.has(song.spotifyId)) return;
        
        setDownloadingIds(prev => new Set(prev).add(song.spotifyId));
        try {
            await api.downloadTrack(
                song.spotifyId,
                song.title,
                song.artist,
                song.album,
                song.duration
            );
            showToast({ type: 'success', message: `Queued: ${song.title}` });
        } catch (error) {
            console.error('Failed to queue download:', error);
            showToast({ type: 'error', message: 'Failed to queue download' });
        } finally {
            setDownloadingIds(prev => {
                const newSet = new Set(prev);
                newSet.delete(song.spotifyId);
                return newSet;
            });
        }
    };

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
                        
                        <div className="w-12 h-12 rounded bg-surface-3 relative flex-shrink-0 overflow-hidden">
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

                        {/* Download button for streaming Spotify tracks */}
                        {song.spotifyId && song.isStreaming && (
                            <button 
                                onClick={(e) => handleDownloadTrack(song, e)}
                                disabled={downloadingIds.has(song.spotifyId)}
                                className="p-2 text-white/30 hover:text-brand opacity-0 group-hover:opacity-100 transition-opacity disabled:opacity-50"
                                title="Download for offline"
                            >
                                {downloadingIds.has(song.spotifyId) ? (
                                    <Loader2 size={18} className="animate-spin" />
                                ) : (
                                    <Download size={18} />
                                )}
                            </button>
                        )}
                        {/* Show downloaded indicator */}
                        {song.spotifyId && !song.isStreaming && (
                            <div className="p-2 text-brand" title="Downloaded">
                                <CheckCircle size={18} />
                            </div>
                        )}

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
