/**
 * Shared Context Menu Utilities
 *
 * Common elements used across multiple context menus such as MenuItem and PlaylistsSubmenu.
 */
import React from 'react';
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
    const { playlists, addToPlaylist, createPlaylist } = useStore();

    const handleAddToPlaylist = (playlistId: string) => {
        addToPlaylist(playlistId, songId);
        onClose();
    };

    const handleCreatePlaylist = () => {
        const name = prompt("New Playlist Name:");
        if (name) {
            createPlaylist(name);
        }
        onClose();
    };

    return (
        <div
            role="menu"
            aria-label="Playlists"
            data-viib-submenu="playlists"
            className="absolute left-full top-0 ml-1 w-56 bg-surface-2 ring-1 ring-surface-3 rounded-xl shadow-xl shadow-black/30 py-1 overflow-hidden z-50"
            onKeyDown={(e) => {
                if (e.key === 'ArrowLeft' && onBack) {
                    e.preventDefault();
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