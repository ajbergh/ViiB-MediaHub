import React, { useEffect, useRef, useState } from 'react';
import { useStore } from '../store';
import { ContextMenuType } from '../types';
import { SongMenu } from './context-menus/SongMenu';
import { AlbumMenu } from './context-menus/AlbumMenu';
import { ArtistMenu } from './context-menus/ArtistMenu';
import { PlaylistMenu } from './context-menus/PlaylistMenu';
import { SmartMixMenu } from './context-menus/SmartMixMenu';
import { QueueItemMenu } from './context-menus/QueueItemMenu';

export const ContextMenu: React.FC = () => {
    const { contextMenu, closeContextMenu } = useStore();
    const { isOpen, x, y, type, data } = contextMenu;
    const menuRef = useRef<HTMLDivElement>(null);

    // Calculate position to prevent overflow
    const [adjustedPos, setAdjustedPos] = useState({ x: 0, y: 0 });

    useEffect(() => {
        if (isOpen && menuRef.current) {
            const rect = menuRef.current.getBoundingClientRect();
            let newX = x;
            let newY = y;

            if (x + rect.width > window.innerWidth) {
                newX = window.innerWidth - rect.width - 10;
            }
            if (y + rect.height > window.innerHeight) {
                newY = window.innerHeight - rect.height - 10;
            }
            setAdjustedPos({ x: newX, y: newY });
        }
    }, [isOpen, x, y]);

    // Close on outside click
    useEffect(() => {
        const handleClickOutside = (event: MouseEvent) => {
            if (menuRef.current && !menuRef.current.contains(event.target as Node)) {
                closeContextMenu();
            }
        };
        
        const handleKeyDown = (event: KeyboardEvent) => {
            if (event.key === 'Escape') closeContextMenu();
        };

        if (isOpen) {
            document.addEventListener('mousedown', handleClickOutside);
            document.addEventListener('keydown', handleKeyDown);
        }
        return () => {
            document.removeEventListener('mousedown', handleClickOutside);
            document.removeEventListener('keydown', handleKeyDown);
        };
    }, [isOpen, closeContextMenu]);

    if (!isOpen) return null;

    return (
        <div 
            ref={menuRef}
            className="fixed z-[9999] w-56 bg-surface-3 border border-surface-border rounded-lg shadow-2xl py-1 text-gray-200 animate-in fade-in duration-100"
            style={{ top: adjustedPos.y, left: adjustedPos.x }}
        >
            {type === ContextMenuType.SONG && <SongMenu song={data} onClose={closeContextMenu} />}
            {type === ContextMenuType.ALBUM && <AlbumMenu album={data} onClose={closeContextMenu} />}
            {type === ContextMenuType.ARTIST && <ArtistMenu artist={data} onClose={closeContextMenu} />}
            {type === ContextMenuType.PLAYLIST && <PlaylistMenu playlist={data} onClose={closeContextMenu} />}
            {type === ContextMenuType.SMART_MIX && <SmartMixMenu mix={data} onClose={closeContextMenu} />}
            {type === ContextMenuType.QUEUE_ITEM && <QueueItemMenu song={data.song} index={data.index} onClose={closeContextMenu} />}
        </div>
    );
};