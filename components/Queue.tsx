/**
 * ViiB MediaHub - Queue Panel Component
 *
 * Floating side panel displaying the current playback queue.
 *
 * Features:
 * - Virtualized list (react-virtuoso) so a 2,000-track queue stays performant
 * - Drag-and-drop reordering via drag handles
 * - Remove individual items
 * - Clear entire queue
 * - Click to play specific track
 * - Current song highlight + "Jump to current" affordance
 * - Smooth slide-in animation from right edge
 * - Empty state via shared EmptyQueue component
 *
 * @module Queue
 */

import React, { useCallback, useEffect, useRef, useState } from 'react';
import { Virtuoso, VirtuosoHandle } from 'react-virtuoso';
import { useStore, useAlbumCovers } from '../store';
import { X, Trash2, GripVertical, Play, LocateFixed } from 'lucide-react';
import { generateGradient, formatTime, coverBackground } from '../utils';
import { ContextMenuType, Song } from '../types';
import { EmptyQueue } from './EmptyState';

interface QueueRowProps {
  song: Song;
  index: number;
  isCurrent: boolean;
  isDragging: boolean;
  cover: string | undefined;
  onDragEnd: () => void;
  onDragStart: (e: React.DragEvent, index: number) => void;
  onDragOver: (e: React.DragEvent, index: number) => void;
  onDrop: (e: React.DragEvent, index: number) => void;
  onPlay: (index: number) => void;
  onRemove: (index: number) => void;
  onContextMenu: (e: React.MouseEvent, song: Song, index: number) => void;
}

const QueueRow = React.memo<QueueRowProps>(({
  song, index, isCurrent, isDragging, cover,
  onDragStart, onDragEnd, onDragOver, onDrop, onPlay, onRemove, onContextMenu,
}) => {
  const [imgError, setImgError] = useState(false);

  useEffect(() => {
    setImgError(false);
  }, [cover]);

  const hasValidCover = Boolean(cover && !imgError);
  const fallbackSeed = song.album || song.title;

  return (
    <div
      draggable
      onDragStart={(e) => onDragStart(e, index)}
      onDragEnd={onDragEnd}
      onDragOver={(e) => onDragOver(e, index)}
      onDrop={(e) => onDrop(e, index)}
      onContextMenu={(e) => onContextMenu(e, song, index)}
      className={`group flex items-center gap-3 p-2 mx-2 mb-0.5 rounded-md transition-colors ${
        isCurrent ? 'bg-surface-highlight' : 'hover:bg-surface-hover'
      } ${isDragging ? 'opacity-50' : ''}`}
    >
      <div className="cursor-grab active:cursor-grabbing text-text-subtle opacity-0 group-hover:opacity-100 transition-opacity" aria-hidden="true">
        <GripVertical size={15} />
      </div>

      <div
        className="w-10 h-10 rounded bg-surface-3 bg-center bg-cover flex-shrink-0 relative overflow-hidden"
        style={{ background: hasValidCover ? coverBackground(cover, fallbackSeed) : generateGradient(fallbackSeed) }}
      >
        {hasValidCover && (
          <img
            src={cover}
            alt=""
            loading="lazy"
            onError={() => setImgError(true)}
            className="w-full h-full object-cover block"
          />
        )}
        <button
          type="button"
          onClick={() => onPlay(index)}
          aria-label={`Play ${song.title}`}
          className="absolute inset-0 bg-surface-0/60 hidden group-hover:flex items-center justify-center cursor-pointer"
        >
          <Play size={16} aria-hidden="true" className="text-text-main fill-current" />
        </button>
        {isCurrent && (
          <div className="absolute inset-0 bg-surface-0/50 flex items-center justify-center" aria-hidden="true">
            <div className="w-2.5 h-2.5 bg-brand rounded-full animate-pulse"></div>
          </div>
        )}
      </div>

      <div className="flex-1 min-w-0 flex flex-col justify-center">
        <span className={`text-sm font-medium truncate ${isCurrent ? 'text-brand' : 'text-text-main'}`}>
          {song.title}
        </span>
        <span className="text-xs text-text-secondary truncate">
          {song.artist}
        </span>
      </div>

      <span className="text-xs text-text-subtle font-mono">
        {formatTime(song.duration)}
      </span>

      <button
        onClick={(e) => { e.stopPropagation(); onRemove(index); }}
        className="p-1.5 rounded-md hover:bg-surface-3 text-text-subtle opacity-0 group-hover:opacity-100 hover:text-error transition-all"
        title="Remove from Queue"
        aria-label={`Remove ${song.title} from queue`}
      >
        <Trash2 size={14} aria-hidden="true" />
      </button>
    </div>
  );
});
QueueRow.displayName = 'QueueRow';

export const Queue: React.FC = () => {
  const isQueueOpen = useStore((state) => state.isQueueOpen);
  const queue = useStore((state) => state.queue);
  const currentSongIndex = useStore((state) => state.currentSongIndex);
  const setQueueOpen = useStore((state) => state.setQueueOpen);
  const removeFromQueue = useStore((state) => state.removeFromQueue);
  const reorderQueue = useStore((state) => state.reorderQueue);
  const playQueueItem = useStore((state) => state.playQueueItem);
  const clearQueue = useStore((state) => state.clearQueue);
  const openContextMenu = useStore((state) => state.openContextMenu);

  const albumCovers = useAlbumCovers();
  const [draggedItemIndex, setDraggedItemIndex] = useState<number | null>(null);
  const draggedIndex = useRef<number | null>(null);
  const [height, setHeight] = useState(520);
  const virtuosoRef = useRef<VirtuosoHandle>(null);

  const handleDragStart = useCallback((e: React.DragEvent, index: number) => {
    draggedIndex.current = index;
    setDraggedItemIndex(index);
    e.dataTransfer.effectAllowed = 'move';
  }, []);

  const handleDragOver = useCallback((e: React.DragEvent, _index: number) => {
    e.preventDefault();
  }, []);

  const handleDrop = useCallback((e: React.DragEvent, index: number) => {
    e.preventDefault();
    const dragged = draggedIndex.current;
    draggedIndex.current = null;
    setDraggedItemIndex(null);
    if (dragged !== null && dragged !== index) reorderQueue(dragged, index);
  }, [reorderQueue]);

  const handleDragEnd = useCallback(() => {
    draggedIndex.current = null;
    setDraggedItemIndex(null);
  }, []);

  const handleContextMenu = useCallback((e: React.MouseEvent, song: Song, index: number) => {
    openContextMenu(e, ContextMenuType.QUEUE_ITEM, { song, index });
  }, [openContextMenu]);

  const handleMouseDown = (e: React.MouseEvent) => {
    e.preventDefault();
    const startY = e.clientY;
    const startHeight = height;

    const handleMouseMove = (moveEvent: MouseEvent) => {
      const deltaY = startY - moveEvent.clientY;
      const newHeight = Math.min(Math.max(300, startHeight + deltaY), window.innerHeight - 150);
      setHeight(newHeight);
    };

    const handleMouseUp = () => {
      document.removeEventListener('mousemove', handleMouseMove);
      document.removeEventListener('mouseup', handleMouseUp);
    };

    document.addEventListener('mousemove', handleMouseMove);
    document.addEventListener('mouseup', handleMouseUp);
  };

  const jumpToCurrent = () => {
    if (currentSongIndex >= 0 && virtuosoRef.current) {
      virtuosoRef.current.scrollToIndex({ index: currentSongIndex, align: 'center', behavior: 'smooth' });
    }
  };

  if (!isQueueOpen) return null;

  return (
    <div
      className="fixed bottom-24 right-4 w-full max-w-[380px] bg-surface-2 border border-surface-border rounded-lg shadow-2xl z-40 flex flex-col overflow-hidden transition-all duration-200"
      style={{ height: `${height}px` }}
      role="region"
      aria-label="Playback queue"
    >
      {/* Resize Handle */}
      <div
        className="h-3 w-full bg-surface-2 cursor-ns-resize flex items-center justify-center hover:bg-surface-hover"
        onMouseDown={handleMouseDown}
        aria-hidden="true"
      >
        <div className="w-10 h-1 bg-surface-3 rounded-full"></div>
      </div>

      {/* Header */}
      <div className="px-4 py-3 border-b border-surface-border flex items-center justify-between bg-surface-2">
        <div className="flex items-center gap-2.5">
          <h2 className="text-base font-bold text-text-main tracking-tight">Queue</h2>
          <span className="text-xs font-medium text-text-secondary bg-surface-3 px-2 py-0.5 rounded-full">
            {queue.length} {queue.length === 1 ? 'track' : 'tracks'}
          </span>
        </div>
        <div className="flex items-center gap-1.5">
          {currentSongIndex >= 0 && queue.length > 0 && (
            <button
              onClick={jumpToCurrent}
              aria-label="Jump to current track"
              title="Jump to current track"
              className="p-1.5 rounded-full text-text-secondary hover:bg-surface-hover hover:text-text-main transition-colors"
            >
              <LocateFixed size={16} aria-hidden="true" />
            </button>
          )}
          {queue.length > 0 && (
            <button
              onClick={clearQueue}
              className="text-xs font-semibold text-text-secondary hover:text-text-main px-2 py-1 rounded hover:bg-surface-hover transition-colors"
            >
              Clear
            </button>
          )}
          <button
            onClick={() => setQueueOpen(false)}
            aria-label="Close queue"
            className="p-1.5 hover:bg-surface-hover rounded-full text-text-secondary hover:text-text-main transition-colors"
          >
            <X size={18} aria-hidden="true" />
          </button>
        </div>
      </div>

      {/* Virtualized List */}
      <div className="flex-1 overflow-hidden bg-surface-2">
        {queue.length === 0 ? (
          <EmptyQueue />
        ) : (
          <Virtuoso
            ref={virtuosoRef}
            data={queue}
            className="h-full overflow-x-hidden"
            components={{
              Header: () => <div className="h-2" />,
              Footer: () => <div className="h-2" />,
            }}
            computeItemKey={(idx, song) => `${song.id}-${idx}`}
            itemContent={(idx, song) => (
              <QueueRow
                song={song}
                index={idx}
                isCurrent={idx === currentSongIndex}
                isDragging={draggedItemIndex === idx}
                cover={song.coverUrl || albumCovers[song.album]}
                onDragStart={handleDragStart}
                onDragEnd={handleDragEnd}
                onDragOver={handleDragOver}
                onDrop={handleDrop}
                onPlay={playQueueItem}
                onRemove={removeFromQueue}
                onContextMenu={handleContextMenu}
              />
            )}
          />
        )}
      </div>
    </div>
  );
};
