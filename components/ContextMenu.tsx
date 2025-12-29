/**
 * ViiB MediaHub - Context Menu Component
 * 
 * Global context menu layer rendering appropriate menus for different item types.
 * 
 * Supported Types:
 * - SONG: Play, add to queue/playlist, go to album/artist
 * - ALBUM: Play, add to queue, go to artist
 * - ARTIST: Play all, shuffle all
 * - PLAYLIST: Play, rename, delete
 * - SMART_MIX: Play, shuffle, save as playlist
 * - QUEUE_ITEM: Remove from queue, play next
 * 
 * Handles click-outside and escape key dismissal.
 * Position auto-adjusts to stay within viewport.
 * 
 * Phase 5 TODO: implement arrow-key roving focus and optional typeahead
 * to meet ARIA menu interaction baseline for keyboard-only users.
 * 
 * @module ContextMenu
 */

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

    const typeaheadRef = useRef<{ buffer: string; lastAt: number }>({ buffer: '', lastAt: 0 });

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

    const getRootMenuEl = (): HTMLElement | null => menuRef.current;

    const isVisible = (el: HTMLElement): boolean => {
        // offsetParent is null for display:none and most hidden/positioned-offscreen cases.
        return el.offsetParent !== null;
    };

    const getActiveMenuEl = (): HTMLElement | null => {
        const root = getRootMenuEl();
        if (!root) return null;
        const active = document.activeElement as HTMLElement | null;
        if (!active) return root;
        const menu = active.closest('[role="menu"]') as HTMLElement | null;
        return menu && root.contains(menu) ? menu : root;
    };

    const getMenuItems = (menuEl: HTMLElement): HTMLElement[] => {
        const items = Array.from(menuEl.querySelectorAll('[role="menuitem"]')) as HTMLElement[];
        return items
            .filter((el) => isVisible(el))
            .filter((el) => el.closest('[role="menu"]') === menuEl);
    };

    const focusItemAt = (menuEl: HTMLElement, index: number) => {
        const items = getMenuItems(menuEl);
        if (items.length === 0) return;
        const clamped = ((index % items.length) + items.length) % items.length;
        items[clamped]?.focus();
    };

    const focusFirstItem = () => {
        const root = getRootMenuEl();
        if (!root) return;
        const items = getMenuItems(root);
        items[0]?.focus();
    };

    const focusNextByPrefix = (menuEl: HTMLElement, prefix: string) => {
        const items = getMenuItems(menuEl);
        if (items.length === 0) return;
        const active = document.activeElement as HTMLElement | null;
        const start = Math.max(0, items.findIndex((i) => i === active));
        const norm = prefix.toLowerCase();

        for (let offset = 1; offset <= items.length; offset++) {
            const idx = (start + offset) % items.length;
            const el = items[idx];
            const label = (el.getAttribute('data-viib-label') || el.textContent || '').trim().toLowerCase();
            if (label.startsWith(norm)) {
                el.focus();
                return;
            }
        }
    };

    // Close on outside click
    useEffect(() => {
        const handleClickOutside = (event: MouseEvent) => {
            if (menuRef.current && !menuRef.current.contains(event.target as Node)) {
                closeContextMenu();
            }
        };

        const handleKeyDown = (event: KeyboardEvent) => {
            const root = getRootMenuEl();
            if (!root || !isOpen) return;

            const active = document.activeElement as HTMLElement | null;
            if (active && !root.contains(active)) return;

            if (event.key === 'Escape') {
                event.preventDefault();
                closeContextMenu();
                return;
            }

            const menuEl = getActiveMenuEl();
            if (!menuEl) return;

            const items = getMenuItems(menuEl);
            if (items.length === 0) return;

            const currentIndex = Math.max(0, items.findIndex((i) => i === document.activeElement));

            switch (event.key) {
                case 'ArrowDown':
                    event.preventDefault();
                    focusItemAt(menuEl, currentIndex + 1);
                    return;
                case 'ArrowUp':
                    event.preventDefault();
                    focusItemAt(menuEl, currentIndex - 1);
                    return;
                case 'Home':
                    event.preventDefault();
                    focusItemAt(menuEl, 0);
                    return;
                case 'End':
                    event.preventDefault();
                    focusItemAt(menuEl, items.length - 1);
                    return;
                case 'Enter':
                case ' ': {
                    const el = document.activeElement as HTMLElement | null;
                    if (el && el.getAttribute('role') === 'menuitem') {
                        event.preventDefault();
                        (el as HTMLButtonElement).click();
                    }
                    return;
                }
                case 'ArrowRight': {
                    const el = document.activeElement as HTMLElement | null;
                    if (!el) return;
                    if (el.getAttribute('aria-haspopup') === 'menu') {
                        event.preventDefault();
                        (el as HTMLButtonElement).click();
                        requestAnimationFrame(() => {
                            const container = el.parentElement;
                            if (!container) return;
                            const submenu = container.querySelector('[role="menu"]') as HTMLElement | null;
                            if (!submenu || !isVisible(submenu)) return;
                            const subItems = getMenuItems(submenu);
                            subItems[0]?.focus();
                        });
                    }
                    return;
                }
                case 'ArrowLeft': {
                    if (menuEl === root) return;
                    event.preventDefault();
                    // Find the nearest trigger in the root menu that owns this submenu.
                    const triggers = Array.from(root.querySelectorAll('[aria-haspopup="menu"]')) as HTMLElement[];
                    const trigger = triggers.find((t) => {
                        const container = t.parentElement;
                        return !!container && container.contains(menuEl);
                    });
                    if (trigger) {
                        (trigger as HTMLButtonElement).click();
                        trigger.focus();
                    }
                    return;
                }
                default: {
                    // Typeahead
                    if (event.key.length === 1 && !event.ctrlKey && !event.metaKey && !event.altKey) {
                        const now = Date.now();
                        const state = typeaheadRef.current;
                        const isStale = now - state.lastAt > 600;
                        state.buffer = (isStale ? '' : state.buffer) + event.key;
                        state.lastAt = now;
                        focusNextByPrefix(menuEl, state.buffer);
                    }
                }
            }
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

    // Focus first menu item when opened
    useEffect(() => {
        if (!isOpen) return;
        // Wait a tick for the menu content to render.
        requestAnimationFrame(() => focusFirstItem());
    }, [isOpen, type]);

    if (!isOpen) return null;

    return (
        <div 
            ref={menuRef}
            role="menu"
            aria-label="Context menu"
            className="fixed z-[9999] w-56 bg-surface-2 ring-1 ring-surface-3 rounded-xl shadow-xl shadow-black/30 py-1 overflow-hidden text-text-main animate-in fade-in duration-150 motion-reduce:transition-none"
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