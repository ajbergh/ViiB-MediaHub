import React, { useState, useRef } from 'react';
import { useStore, useAlbumCovers } from '../store';
import { X, Trash2, GripVertical, Play, ListMusic } from 'lucide-react';
import { generateGradient, formatTime } from '../utils';
import { ContextMenuType } from '../types';

export const Queue: React.FC = () => {
  const { 
    queue, 
    currentSongIndex, 
    isQueueOpen, 
    setQueueOpen, 
    removeFromQueue, 
    reorderQueue, 
    playQueueItem,
    clearQueue,
    openContextMenu
  } = useStore();
  
  const albumCovers = useAlbumCovers();
  const [draggedItemIndex, setDraggedItemIndex] = useState<number | null>(null);
  const [isResizing, setIsResizing] = useState(false);
  const [height, setHeight] = useState(500); // Initial height in px

  if (!isQueueOpen) return null;

  // Drag and Drop Handlers
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

  // Resize Handler
  const handleMouseDown = (e: React.MouseEvent) => {
      e.preventDefault();
      setIsResizing(true);
      
      const startY = e.clientY;
      const startHeight = height;
      
      const handleMouseMove = (moveEvent: MouseEvent) => {
          const deltaY = startY - moveEvent.clientY; // Dragging up increases height
          const newHeight = Math.min(Math.max(300, startHeight + deltaY), window.innerHeight - 150);
          setHeight(newHeight);
      };
      
      const handleMouseUp = () => {
          setIsResizing(false);
          document.removeEventListener('mousemove', handleMouseMove);
          document.removeEventListener('mouseup', handleMouseUp);
      };
      
      document.addEventListener('mousemove', handleMouseMove);
      document.addEventListener('mouseup', handleMouseUp);
  };

  return (
    <div 
        className="fixed bottom-24 right-4 w-full max-w-[400px] bg-surface-2 border border-surface-3 rounded-xl shadow-2xl z-40 flex flex-col overflow-hidden transition-all duration-200"
        style={{ height: `${height}px` }}
    >
        {/* Resize Handle */}
        <div 
            className="h-4 w-full bg-surface-highlight cursor-ns-resize flex items-center justify-center hover:bg-surface-hover"
            onMouseDown={handleMouseDown}
        >
            <div className="w-12 h-1 bg-surface-slider rounded-full"></div>
        </div>

        {/* Header */}
        <div className="p-4 border-b border-surface-3 flex items-center justify-between bg-surface-2">
            <div className="flex items-center gap-2">
                <h2 className="font-bold text-text-main">Queue</h2>
                <span className="text-xs text-text-secondary bg-surface-hover px-2 py-0.5 rounded-full">{queue.length} tracks</span>
            </div>
            <div className="flex items-center gap-2">
                {queue.length > 0 && (
                     <button 
                        onClick={clearQueue}
                        className="text-xs text-text-secondary hover:text-text-main hover:underline mr-2"
                    >
                        Clear
                    </button>
                )}
                <button 
                    onClick={() => setQueueOpen(false)}
                    className="p-1 hover:bg-surface-hover rounded-full text-text-secondary hover:text-text-main"
                >
                    <X size={18} />
                </button>
            </div>
        </div>

        {/* List */}
        <div className="flex-1 overflow-y-auto overflow-x-hidden p-2 space-y-1 bg-surface-1">
            {queue.length === 0 ? (
                <div className="flex flex-col items-center justify-center h-full text-text-subtle text-center p-6">
                    <ListMusic size={48} className="mb-4 opacity-30" />
                    <h3 className="font-bold mb-1">Your queue is empty</h3>
                    <p className="text-sm">Add songs or albums to start listening</p>
                </div>
            ) : (
                queue.map((song, idx) => {
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
                            className={`group flex items-center gap-3 p-2 rounded-md border border-transparent ${
                                isCurrent ? 'bg-surface-hover border-l-brand border-l-4' : 'hover:bg-surface-highlight border-l-transparent border-l-4'
                            } ${draggedItemIndex === idx ? 'opacity-50' : ''}`}
                        >
                            {/* Drag Handle */}
                            <div className="cursor-grab active:cursor-grabbing text-surface-slider group-hover:text-text-subtle">
                                <GripVertical size={16} />
                            </div>

                            {/* Cover */}
                            <div 
                                className="w-10 h-10 rounded bg-surface-3 bg-center bg-cover flex-shrink-0 relative"
                                style={{ background: displayCover ? `url(${displayCover})` : generateGradient(song.album) }}
                            >
                                {/* Play overlay on hover */}
                                <div 
                                    className="absolute inset-0 bg-black/40 hidden group-hover:flex items-center justify-center cursor-pointer"
                                    onClick={() => playQueueItem(idx)}
                                >
                                    <Play size={16} className="text-white fill-current" />
                                </div>
                                {isCurrent && (
                                    <div className="absolute inset-0 bg-black/40 flex items-center justify-center">
                                        <div className="w-3 h-3 bg-brand rounded-full animate-pulse shadow-[0_0_8px_#1db954]"></div>
                                    </div>
                                )}
                            </div>

                            {/* Info */}
                            <div className="flex-1 min-w-0 flex flex-col justify-center">
                                <span className={`text-sm font-medium truncate ${isCurrent ? 'text-brand' : 'text-text-main'}`}>
                                    {song.title}
                                </span>
                                <span className="text-xs text-text-secondary truncate">
                                    {song.artist}
                                </span>
                            </div>

                            {/* Time */}
                            <span className="text-xs text-text-subtle font-mono">
                                {formatTime(song.duration)}
                            </span>

                            {/* Remove */}
                            <button 
                                onClick={(e) => { e.stopPropagation(); removeFromQueue(idx); }}
                                className="p-1.5 rounded hover:bg-surface-border text-surface-slider group-hover:text-text-secondary hover:text-red-400 transition-colors"
                                title="Remove from Queue"
                            >
                                <Trash2 size={14} />
                            </button>
                        </div>
                    );
                })
            )}
        </div>
    </div>
  );
};