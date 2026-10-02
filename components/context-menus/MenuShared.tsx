/**
 * Shared Context Menu Utilities
 *
 * Common elements used across multiple context menus such as MenuItem and PlaylistsSubmenu.
 */
import React, { useLayoutEffect, useRef, useState } from 'react';
import type { LucideIcon } from 'lucide-react';
import { Plus } from 'lucide-react';
import { useStore } from '../../store';

/**
 * MenuItem - Button element used within context menus.
 * Props:
 *  - icon: Icon component to display on the left
 *  - label: Text label for the menu item
 *  - onClick: Handler for activation
 */
type MenuItemProps = {
    icon: LucideIcon;
    label: string;
    onClick: () => void;
    disabled?: boolean;
    destructive?: boolean;
};

export const MenuItem: React.FC<MenuItemProps> = ({ icon: Icon, label, onClick, disabled, destructive }) => (
    <button
        onClick={(e) => {
            e.stopPropagation();
            if (!disabled) onClick();
        }}
        disabled={disabled}
        role="menuitem"
        tabIndex={-1}
        data-viib-label={label}
        aria-label={label}
        className={
            'group w-full text-left px-4 py-2 text-sm flex items-center gap-3 justify-between ' +
            'hover:bg-surface-1/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-0 ' +
            'transition-colors duration-150 motion-reduce:transition-none ' +
            (disabled ? 'opacity-50 cursor-not-allowed ' : 'cursor-pointer ') +
            (destructive ? 'text-error ' : 'text-text-main ')
        }
    >
        <div className="flex items-center gap-3 min-w-0">
            <Icon size={16} className="text-text-subtle group-hover:text-text-main" />
            <span className="truncate">{label}</span>
        </div>
    </button>
);

/**
 * PlaylistsSubmenu - Submenu used to add a song to an existing playlist or create a new playlist.
 * Props:
 *  - songId: ID of the song being added
 *  - onClose: Callback when the submenu closes
 */
export const PlaylistsSubmenu: React.FC<{ songId: string; onClose: () => void; onBack?: () => void }> = ({ songId, onClose, onBack }) => {
    const { playlists, addToPlaylist, createPlaylist, showToast } = useStore();
    const submenuRef = useRef<HTMLDivElement>(null);
    const [position, setPosition] = useState<React.CSSProperties>({});

    useLayoutEffect(() => {
        const placeSubmenu = () => {
            const submenu = submenuRef.current;
            const anchor = submenu?.parentElement;
            if (!submenu || !anchor) return;
            const rect = anchor.getBoundingClientRect();
            const margin = 8;
            const width = Math.min(224, Math.max(0, window.innerWidth - margin * 2));
            const height = Math.min(submenu.getBoundingClientRect().height, window.innerHeight - margin * 2);
            const preferredLeft = rect.right + width <= window.innerWidth - margin
                ? rect.right : rect.left - width;
            const left = Math.max(margin, Math.min(preferredLeft, window.innerWidth - width - margin));
            const top = Math.max(margin, Math.min(rect.top, window.innerHeight - height - margin));
            // Keep this in the menu DOM for focus and click-outside handling.
            // Adjacent edges also let the pointer enter without crossing a gap.
            setPosition({ left: left - rect.left, top: top - rect.top, width, maxHeight: window.innerHeight - margin * 2 });
        };
        placeSubmenu();
        window.addEventListener('resize', placeSubmenu);
        return () => window.removeEventListener('resize', placeSubmenu);
    }, [playlists.length]);


    const handleAddToPlaylist = (playlistId: string) => {
        addToPlaylist(playlistId, songId);
        onClose();
    };

    const handleCreatePlaylist = async () => {
        const name = prompt("New Playlist Name:");
        if (name) {
            try { await createPlaylist(name, [songId]); }
            catch { showToast({ type: 'error', message: 'Unable to create playlist. Please try again.' }); return; }
        }
        onClose();
    };

    return (
        <div
            role="menu"
            aria-label="Playlists"
            data-viib-submenu="playlists"
            ref={submenuRef}
            style={position}
            className="absolute left-full top-0 w-56 bg-surface-2 ring-1 ring-surface-3 rounded-xl shadow-xl shadow-black/30 py-1 overflow-y-auto z-50"
            onKeyDown={(e) => {
                if (e.key === 'ArrowLeft' && onBack) {
                    e.preventDefault();
                    e.stopPropagation();
                    onBack();
                }
            }}
        >
            <button
                role="menuitem"
                tabIndex={-1}
                data-viib-label="New Playlist"
                aria-label="New Playlist"
                className="w-full text-left px-4 py-2 text-sm text-text-main hover:bg-surface-1/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-0 flex items-center gap-2 transition-colors duration-150 motion-reduce:transition-none"
                onClick={handleCreatePlaylist}
            >
                <Plus size={14} /> New Playlist
            </button>
            <div role="separator" className="border-t border-surface-3 my-1"></div>
            {playlists.length === 0 ? (
                <div className="px-4 py-2 text-xs text-text-subtle italic">No playlists</div>
            ) : (
                <div className="max-h-48 overflow-y-auto">
                    {playlists.map((pl) => (
                        <button
                            key={pl.id}
                            role="menuitem"
                            tabIndex={-1}
                            data-viib-label={pl.name}
                            aria-label={pl.name}
                            className="w-full text-left px-4 py-2 text-sm text-text-main hover:bg-surface-1/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-0 truncate transition-colors duration-150 motion-reduce:transition-none"
                            onClick={() => handleAddToPlaylist(pl.id)}
                        >
                            {pl.name}
                        </button>
                    ))}
                </div>
            )}
        </div>
    );
};