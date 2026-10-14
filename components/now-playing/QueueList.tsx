/** Displays the upcoming playback queue within Now Playing. */

import React, { useCallback, useEffect, useState } from 'react';
import { Virtuoso } from 'react-virtuoso';
import { Play, GripVertical, Trash2, Download, CheckCircle, Loader2 } from 'lucide-react';
import { useStore, useAlbumCovers } from '../../store';
import { formatTime, generateGradient, coverBackground } from '../../utils';
import { ContextMenuType, Song } from '../../types';
import { api } from '../../services/api';
import { Button } from '../ui/Button';

interface Props {
    queue: Song[];
    currentSongIndex: number;
}

interface QueueListRowProps {
    song: Song;
    index: number;
    isCurrent: boolean;
    isDragging: boolean;
    isDownloading: boolean;
    displayCover: string | undefined;
    onDragStart: (e: React.DragEvent, index: number) => void;
    onDragOver: (e: React.DragEvent, index: number) => void;
    onDrop: (e: React.DragEvent, index: number) => void;
    onPlay: (index: number) => void;
    onRemove: (index: number) => void;
    onDownload: (song: Song, e: React.MouseEvent) => void;
    onContextMenu: (e: React.MouseEvent, song: Song, index: number) => void;
}

const QueueListRow = React.memo<QueueListRowProps>(({
    song,
    index,
    isCurrent,
    isDragging,
    isDownloading,
    displayCover,
    onDragStart,
    onDragOver,
    onDrop,
    onPlay,
    onRemove,
    onDownload,
    onContextMenu,
}) => {
    const [imgError, setImgError] = useState(false);

    useEffect(() => {
        setImgError(false);
    }, [displayCover]);

    const hasValidCover = Boolean(displayCover && !imgError);
    const fallbackSeed = song.album || song.title;

    return (
        <div
            draggable
            onDragStart={(e) => onDragStart(e, index)}
            onDragOver={(e) => onDragOver(e, index)}
            onDrop={(e) => onDrop(e, index)}
            onContextMenu={(e) => onContextMenu(e, song, index)}
            className={`group flex items-center gap-4 p-3 mb-1 rounded-lg border border-transparent transition-all ${
                isCurrent
                    ? 'bg-surface-2/60 border-accent-green/40'
                    : 'hover:bg-surface-2/30 border-transparent'
            } ${isDragging ? 'opacity-40' : ''}`}
        >
            <div className="cursor-grab active:cursor-grabbing text-text-subtle/70 group-hover:text-text-secondary">
                <GripVertical size={20} />
            </div>

            <div
                className="w-12 h-12 rounded bg-surface-3 bg-center bg-cover relative flex-shrink-0 overflow-hidden"
                style={{ background: hasValidCover ? coverBackground(displayCover, fallbackSeed) : generateGradient(fallbackSeed) }}
            >
                {hasValidCover && (
                    <img
                        src={displayCover}
                        loading="lazy"
                        onError={() => setImgError(true)}
                        className="w-full h-full object-cover block"
                        alt=""
                    />
                )}
                <div
                    className="absolute inset-0 bg-surface-0/60 hidden group-hover:flex items-center justify-center cursor-pointer"
                    onClick={() => onPlay(index)}
                >
                    <Play size={20} className="text-text-main fill-current" />
                </div>
            </div>

            <div className="flex-1 min-w-0">
                <div className={`font-bold truncate ${isCurrent ? 'text-accent-green' : 'text-text-main'}`}>{song.title}</div>
                <div className="text-sm text-text-secondary truncate">{song.artist}</div>
            </div>

            <div className="text-sm font-mono text-text-subtle/70">{formatTime(song.duration)}</div>

            {song.spotifyId && song.isStreaming && (
                <Button
                    onClick={(e) => onDownload(song, e)}
                    disabled={isDownloading}
                    variant="ghost"
                    className="p-2 text-text-subtle/70 hover:text-brand opacity-0 group-hover:opacity-100 transition-opacity disabled:opacity-50"
                    title="Download for offline"
                    aria-label="Download for offline"
                >
                    {isDownloading ? (
                        <Loader2 size={18} className="animate-spin" />
                    ) : (
                        <Download size={18} />
                    )}
                </Button>
            )}
            {song.spotifyId && !song.isStreaming && (
                <div className="p-2 text-brand" title="Downloaded">
                    <CheckCircle size={18} />
                </div>
            )}

            <Button
                onClick={(e) => { e.stopPropagation(); onRemove(index); }}
                variant="ghost"
                className="p-2 text-text-subtle/70 hover:text-error opacity-0 group-hover:opacity-100 transition-opacity"
                aria-label="Remove from queue"
                title="Remove from queue"
            >
                <Trash2 size={18} />
            </Button>
        </div>
    );
});
QueueListRow.displayName = 'QueueListRow';

export const QueueList: React.FC<Props> = ({ queue, currentSongIndex }) => {
    const playQueueItem = useStore((state) => state.playQueueItem);
    const removeFromQueue = useStore((state) => state.removeFromQueue);
    const reorderQueue = useStore((state) => state.reorderQueue);
    const openContextMenu = useStore((state) => state.openContextMenu);
    const showToast = useStore((state) => state.showToast);
    const albumCovers = useAlbumCovers();
    const [draggedItemIndex, setDraggedItemIndex] = useState<number | null>(null);
    const [downloadingIds, setDownloadingIds] = useState<Set<string>>(new Set());

    const handleDownloadTrack = useCallback(async (song: Song, e: React.MouseEvent) => {
        e.stopPropagation();
        if (!song.spotifyId || downloadingIds.has(song.spotifyId)) return;

        const spotifyId = song.spotifyId;
        setDownloadingIds(prev => new Set(prev).add(spotifyId));
        try {
            await api.downloadTrack(
                spotifyId,
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
                newSet.delete(spotifyId);
                return newSet;
            });
        }
    }, [downloadingIds, showToast]);

    const handleDragStart = useCallback((e: React.DragEvent, index: number) => {
        setDraggedItemIndex(index);
        e.dataTransfer.effectAllowed = "move";
    }, []);

    const handleDragOver = useCallback((e: React.DragEvent, _index: number) => {
        e.preventDefault();
    }, []);

    const handleDrop = useCallback((e: React.DragEvent, index: number) => {
        e.preventDefault();
        setDraggedItemIndex((dragged) => {
            if (dragged !== null && dragged !== index) {
                reorderQueue(dragged, index);
            }
            return null;
        });
    }, [reorderQueue]);

    const handleContextMenu = useCallback((e: React.MouseEvent, song: Song, index: number) => {
        openContextMenu(e, ContextMenuType.QUEUE_ITEM, { song, index });
    }, [openContextMenu]);

    if (queue.length === 0) {
        return <div className="py-12 text-center text-sm text-text-secondary">Queue is empty</div>;
    }

    return (
        <div className="h-full min-h-[320px]">
            <Virtuoso
                data={queue}
                className="h-full overflow-x-hidden"
                style={{ height: '100%' }}
                computeItemKey={(idx, song) => `${song.id}-${idx}`}
                itemContent={(idx, song) => (
                    <QueueListRow
                        song={song}
                        index={idx}
                        isCurrent={idx === currentSongIndex}
                        isDragging={draggedItemIndex === idx}
                        isDownloading={Boolean(song.spotifyId && downloadingIds.has(song.spotifyId))}
                        displayCover={song.coverUrl || albumCovers[song.album]}
                        onDragStart={handleDragStart}
                        onDragOver={handleDragOver}
                        onDrop={handleDrop}
                        onPlay={playQueueItem}
                        onRemove={removeFromQueue}
                        onDownload={handleDownloadTrack}
                        onContextMenu={handleContextMenu}
                    />
                )}
            />
        </div>
    );
};

