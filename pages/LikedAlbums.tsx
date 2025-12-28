/**
 * ViiB MediaHub - Liked Albums Page
 * 
 * Displays all albums that the user has liked (favorited).
 * Albums are sorted by when they were liked (newest first).
 * 
 * Features:
 * - Grid view using virtualized scrolling for performance
 * - Unlike albums directly from the grid
 * - Empty state for users with no liked albums
 * - Click to navigate to album detail
 * 
 * @module LikedAlbums
 */

import React, { useMemo, useState, useEffect, forwardRef } from 'react';
import { useStore, useAlbums } from '../store';
import { Heart, ChevronDown, ArrowUpDown } from 'lucide-react';
import { coverBackground } from '../utils';
import { ContextMenuType, Album } from '../types';
import { VirtuosoGrid } from 'react-virtuoso';
import { useNavigate } from 'react-router-dom';
import { AlbumLikeButton } from '../components/AlbumLikeButton';
import { Page } from '../components/ui/Page';

type LikedAlbumSortOption = 'recent' | 'name-asc' | 'name-desc' | 'artist-asc' | 'artist-desc';

const sortLabels: Record<LikedAlbumSortOption, string> = {
    'recent': 'Recently Liked',
    'name-asc': 'Name (A-Z)',
    'name-desc': 'Name (Z-A)',
    'artist-asc': 'Artist (A-Z)',
    'artist-desc': 'Artist (Z-A)',
};

// Define Grid Components outside to prevent re-renders
const ListContainer = forwardRef<HTMLDivElement, any>(({ style, children, ...props }, ref) => (
    <div
        ref={ref}
        {...props}
        style={style}
        className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6 gap-6 pb-32"
    >
        {children}
    </div>
));

const ItemContainer = forwardRef<HTMLDivElement, any>(({ children, ...props }, ref) => (
    <div {...props} ref={ref} className="w-full h-full">
        {children}
    </div>
));

export const LikedAlbums: React.FC = () => {
    const albums = useAlbums();
    const { likedAlbumKeys, albumMetadata, openContextMenu } = useStore();
    const navigate = useNavigate();
    const [scrollParent, setScrollParent] = useState<HTMLElement | null>(null);
    const [sortBy, setSortBy] = useState<LikedAlbumSortOption>('recent');
    const [showSortMenu, setShowSortMenu] = useState(false);

    useEffect(() => {
        setScrollParent(document.querySelector('main'));
    }, []);

    // Get liked albums with their metadata
    const likedAlbums = useMemo(() => {
        return albums.filter(album => {
            const albumKey = `${album.name}::${album.artist}`;
            return likedAlbumKeys.has(albumKey);
        });
    }, [albums, likedAlbumKeys]);

    // Sort the albums
    const sortedLikedAlbums = useMemo(() => {
        const sorted = [...likedAlbums];
        switch (sortBy) {
            case 'recent':
                // For recent, we'd need likedAt from metadata - fallback to addedAt for now
                return sorted.sort((a, b) => {
                    const keyA = `${a.name}::${a.artist}`;
                    const keyB = `${b.name}::${b.artist}`;
                    const metaA = albumMetadata[keyA];
                    const metaB = albumMetadata[keyB];
                    // Use addedAt as fallback if likedAt not available
                    return (b.addedAt || 0) - (a.addedAt || 0);
                });
            case 'name-asc':
                return sorted.sort((a, b) => a.name.localeCompare(b.name));
            case 'name-desc':
                return sorted.sort((a, b) => b.name.localeCompare(a.name));
            case 'artist-asc':
                return sorted.sort((a, b) => a.artist.localeCompare(b.artist));
            case 'artist-desc':
                return sorted.sort((a, b) => b.artist.localeCompare(a.artist));
            default:
                return sorted;
        }
    }, [likedAlbums, sortBy, albumMetadata]);

    // Empty state
    if (sortedLikedAlbums.length === 0) {
        return (
            <Page withPlayerPadding={false}>
                {/* Header Section */}
                <div className="flex items-end gap-6 mb-8">
                    <div className="w-48 h-48 rounded-lg bg-gradient-to-br from-brand/40 to-surface-1 flex items-center justify-center shadow-2xl">
                        <Heart size={80} className="text-white/50" />
                    </div>
                    
                    <div className="flex-1">
                        <p className="text-xs uppercase tracking-widest text-text-secondary mb-2 font-semibold">Collection</p>
                        <h1 className="text-display font-bold text-text-main mb-4">Liked Albums</h1>
                        <p className="text-text-secondary text-sm">No liked albums yet</p>
                    </div>
                </div>
                
                <div className="flex flex-col items-center justify-center p-12 text-center">
                    <Heart size={64} className="text-text-subtle mb-4" />
                    <h2 className="text-section font-semibold text-text-main mb-2">Albums you like will appear here</h2>
                    <p className="text-text-secondary max-w-md">
                        Click the heart icon on any album to add it to your Liked Albums collection.
                    </p>
                </div>
            </Page>
        );
    }

    return (
        <Page withPlayerPadding={false}>
            {/* Header Section */}
            <div className="flex items-end gap-6 mb-8">
                <div className="w-48 h-48 rounded-lg bg-gradient-to-br from-brand to-surface-1 flex items-center justify-center shadow-2xl shadow-brand/30">
                    <Heart size={80} className="text-white fill-current" />
                </div>
                
                <div className="flex-1">
                    <p className="text-xs uppercase tracking-widest text-text-secondary mb-2 font-semibold">Collection</p>
                    <h1 className="text-display font-bold text-text-main mb-4">Liked Albums</h1>
                    <p className="text-text-secondary text-sm">
                        {sortedLikedAlbums.length} album{sortedLikedAlbums.length !== 1 ? 's' : ''}
                    </p>
                </div>
            </div>

            {/* Sort Dropdown */}
            <div className="flex justify-end mb-6">
                <div className="relative">
                    <button
                        type="button"
                        onClick={() => setShowSortMenu(!showSortMenu)}
                        className="flex items-center gap-2 px-4 py-2 bg-surface-highlight hover:bg-surface-hover rounded-full text-sm text-text-main transition-colors border border-transparent hover:border-surface-slider"
                        aria-haspopup="menu"
                        aria-expanded={showSortMenu}
                    >
                        <ArrowUpDown size={16} className="text-text-secondary" aria-hidden="true" />
                        <span>{sortLabels[sortBy]}</span>
                        <ChevronDown size={16} className={`text-text-secondary transition-transform ${showSortMenu ? 'rotate-180' : ''}`} aria-hidden="true" />
                    </button>
                    
                    {showSortMenu && (
                        <>
                            <button
                              type="button"
                              tabIndex={-1}
                              aria-label="Close sort menu"
                              className="fixed inset-0 z-40"
                              onClick={() => setShowSortMenu(false)}
                            />
                            <div className="absolute right-0 top-full mt-2 bg-surface-2 border border-surface-3 rounded-lg shadow-xl z-50 py-1 min-w-[180px]">
                                {(Object.keys(sortLabels) as LikedAlbumSortOption[]).map((option) => (
                                    <button
                                        type="button"
                                        key={option}
                                        onClick={() => { setSortBy(option); setShowSortMenu(false); }}
                                        className={`w-full text-left px-4 py-2 text-sm hover:bg-surface-hover transition-colors ${sortBy === option ? 'text-brand font-medium' : 'text-text-main'}`}
                                    >
                                        {sortLabels[option]}
                                    </button>
                                ))}
                            </div>
                        </>
                    )}
                </div>
            </div>

            {/* Album Grid */}
            <VirtuosoGrid
                useWindowScroll={false}
                customScrollParent={scrollParent}
                data={sortedLikedAlbums}
                components={{
                    List: ListContainer,
                    Item: ItemContainer
                }}
                itemContent={(index, album) => {
                    const metadataKey = `${album.name}::${album.artist}`;
                    const metadata = albumMetadata[metadataKey];
                    const coverUrl = metadata?.coverUrl || album.coverUrl;

                    return (
                                                <div 
                            className="bg-surface-2 p-4 rounded-lg hover:bg-surface-3 transition-all group cursor-pointer border border-transparent hover:border-surface-border h-full flex flex-col relative"
                            onClick={() => navigate(`/album/${encodeURIComponent(album.name)}`)}
                            onContextMenu={(e) => openContextMenu(e, ContextMenuType.ALBUM, album)}
                                                        role="button"
                                                        tabIndex={0}
                                                        aria-label={`Open album ${album.name}`}
                                                        onKeyDown={(e) => {
                                                            if (e.key === 'Enter' || e.key === ' ') {
                                                                e.preventDefault();
                                                                navigate(`/album/${encodeURIComponent(album.name)}`);
                                                            }
                                                        }}
                        >
                            {/* Like Button (top-right corner) */}
                            <div className="absolute top-2 right-2 z-10 opacity-0 group-hover:opacity-100 transition-opacity">
                                <AlbumLikeButton albumKey={metadataKey} size={20} />
                            </div>
                            
                            <div 
                                className="w-full aspect-square rounded-md mb-4 shadow-lg flex items-center justify-center text-display font-bold text-white/20 relative overflow-hidden bg-surface-3"
                                style={{ background: coverBackground(coverUrl, album.name) }}
                            >
                                {!coverUrl && <span className="z-10">{album.name.charAt(0)}</span>}
                                
                                {/* Hover Overlay */}
                                <div className="absolute inset-0 bg-black/40 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center">
                                    <div className="w-12 h-12 bg-brand rounded-full flex items-center justify-center shadow-lg transform translate-y-4 group-hover:translate-y-0 transition-transform duration-300">
                                        <svg className="w-6 h-6 text-black fill-current ml-1" viewBox="0 0 24 24"><path d="M8 5v14l11-7z"/></svg>
                                    </div>
                                </div>
                            </div>
                            <h4 className="font-bold truncate text-text-main mb-1">{album.name}</h4>
                            <div className="flex justify-between items-center mt-auto">
                                <p className="text-sm text-text-secondary truncate max-w-[70%]">{album.artist}</p>
                                {metadata?.releaseDate && (
                                    <span className="text-[10px] text-text-subtle font-mono">{new Date(metadata.releaseDate).getFullYear()}</span>
                                )}
                            </div>
                        </div>
                    );
                }}
            />
        </Page>
    );
};

export default LikedAlbums;
