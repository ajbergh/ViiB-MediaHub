/**
 * ViiB MediaHub - Liked Albums Page
 * 
 * Displays all albums that the user has liked (favorited).
 * Albums are sorted by when they were liked (newest first) by default.
 * 
 * @module LikedAlbums
 */

import React, { useMemo, useState, useEffect, forwardRef } from 'react';
import { useStore, useAlbums } from '../store';
import { Heart, ChevronDown, ArrowUpDown } from 'lucide-react';
import { coverBackground } from '../utils';
import { ContextMenuType } from '../types';
import { VirtuosoGrid } from 'react-virtuoso';
import { useNavigate } from 'react-router';
import { AlbumLikeButton } from '../components/AlbumLikeButton';
import { Page } from '../components/ui/Page';
import { EmptyState } from '../components/EmptyState';
import { EMPTY_STATE } from '../lib/emptyStateCopy';
import { CardSizeSlider } from '../components/ui/CardSizeSlider';
import { resolveAlbumArtwork } from '../lib/artwork';

type LikedAlbumSortOption = 'recent' | 'name-asc' | 'name-desc' | 'artist-asc' | 'artist-desc';

const sortLabels: Record<LikedAlbumSortOption, string> = {
    'recent': 'Recently Liked',
    'name-asc': 'Name (A-Z)',
    'name-desc': 'Name (Z-A)',
    'artist-asc': 'Artist (A-Z)',
    'artist-desc': 'Artist (Z-A)',
};

const ListContainer = forwardRef<HTMLDivElement, any>(({ style, children, ...props }, ref) => (
    <div
        ref={ref}
        {...props}
        style={{
          ...style,
          display: 'grid',
          gridTemplateColumns: 'repeat(var(--card-cols, 4), minmax(0, 1fr))',
          gap: '0.5rem',
          paddingBottom: '8rem',
        }}
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
    const [cardCols, setCardCols] = useState(() => Number(localStorage.getItem('liked-albums-card-cols') ?? 4));
    const handleCardColsChange = (v: number) => { setCardCols(v); localStorage.setItem('liked-albums-card-cols', String(v)); };

    useEffect(() => {
        setScrollParent(document.querySelector('main'));
    }, []);

    const likedAlbums = useMemo(() => {
        return albums.filter(album => {
            const albumKey = `${album.name}::${album.artist}`;
            return likedAlbumKeys.has(albumKey);
        });
    }, [albums, likedAlbumKeys]);

    const sortedLikedAlbums = useMemo(() => {
        const sorted = [...likedAlbums];
        switch (sortBy) {
            case 'recent':
                return sorted.sort((a, b) => (b.addedAt || 0) - (a.addedAt || 0));
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
    }, [likedAlbums, sortBy]);

    const likedAlbumsCopy = EMPTY_STATE.likedAlbums;
    if (sortedLikedAlbums.length === 0) {
        return (
            <Page withPlayerPadding={false}>
                <div className="flex flex-col sm:flex-row items-start sm:items-end gap-6 mb-8">
                    <div className="w-40 h-40 sm:w-48 sm:h-48 rounded-md bg-gradient-to-br from-brand/60 via-surface-3 to-surface-2 flex items-center justify-center shadow-2xl flex-shrink-0">
                        <Heart size={72} className="text-text-main/70" />
                    </div>
                    <div className="flex-1 min-w-0">
                        <p className="text-xs uppercase tracking-widest text-text-secondary mb-1.5 font-bold">Collection</p>
                        <h1 className="text-display font-extrabold tracking-tight text-text-main mb-3">Liked Albums</h1>
                        <p className="text-text-secondary text-sm">No liked albums yet</p>
                    </div>
                </div>
                <div className="flex items-center justify-center py-12">
                    <EmptyState
                        icon={<likedAlbumsCopy.icon size={48} />}
                        title={likedAlbumsCopy.title}
                        description={likedAlbumsCopy.description}
                        action={likedAlbumsCopy.primaryAction ? {
                            label: likedAlbumsCopy.primaryAction,
                            onClick: () => navigate('/albums')
                        } : undefined}
                        secondaryAction={likedAlbumsCopy.secondaryAction ? {
                            label: likedAlbumsCopy.secondaryAction,
                            onClick: () => navigate('/artists')
                        } : undefined}
                    />
                </div>
            </Page>
        );
    }

    return (
        <Page withPlayerPadding={false}>
            <div className="flex flex-col sm:flex-row items-start sm:items-end gap-6 mb-6">
                <div className="w-40 h-40 sm:w-48 sm:h-48 rounded-md bg-gradient-to-br from-brand via-accent-green to-surface-2 flex items-center justify-center shadow-2xl flex-shrink-0">
                    <Heart size={72} className="text-text-main fill-current drop-shadow-md" />
                </div>
                <div className="flex-1 min-w-0">
                    <p className="text-xs uppercase tracking-widest text-text-secondary mb-1.5 font-bold">Collection</p>
                    <h1 className="text-display font-extrabold tracking-tight text-text-main mb-3">Liked Albums</h1>
                    <p className="text-text-secondary text-sm font-medium">
                        {sortedLikedAlbums.length} album{sortedLikedAlbums.length !== 1 ? 's' : ''}
                    </p>
                </div>
            </div>

            <div className="flex items-center justify-end gap-3 mb-6">
                <CardSizeSlider value={cardCols} onChange={handleCardColsChange} />
                <div className="relative">
                    <button
                        type="button"
                        onClick={() => setShowSortMenu(!showSortMenu)}
                        className="flex items-center gap-2 px-4 py-2 bg-surface-3 hover:bg-surface-highlight rounded-full text-sm text-text-main transition-colors"
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
                            <div className="absolute right-0 top-full mt-2 bg-surface-2 border border-surface-border rounded-md shadow-xl z-50 py-1 min-w-[180px]">
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

            <div style={{ '--card-cols': cardCols } as React.CSSProperties}>
              <VirtuosoGrid
                useWindowScroll={false}
                customScrollParent={scrollParent}
                data={sortedLikedAlbums}
                components={{ List: ListContainer, Item: ItemContainer }}
                itemContent={(index, album) => {
                    const metadataKey = `${album.name}::${album.artist}`;
                    const metadata = albumMetadata[metadataKey];
                    const coverUrl = resolveAlbumArtwork(album.coverUrl, metadata?.coverUrl, album.plexBacked);

                    return (
                        <div
                            className="bg-transparent p-3 rounded-lg hover:bg-surface-hover transition-colors group cursor-pointer h-full flex flex-col relative"
                            onClick={() => navigate(`/album/${encodeURIComponent(album.name)}/${encodeURIComponent(album.artist)}`)}
                            onContextMenu={(e) => openContextMenu(e, ContextMenuType.ALBUM, album)}
                            role="button"
                            tabIndex={0}
                            aria-label={`Open album ${album.name}`}
                            onKeyDown={(e) => {
                                if (e.key === 'Enter' || e.key === ' ') {
                                    e.preventDefault();
                                    navigate(`/album/${encodeURIComponent(album.name)}/${encodeURIComponent(album.artist)}`);
                                }
                            }}
                        >
                            <div className="absolute top-5 right-5 z-10 opacity-0 group-hover:opacity-100 transition-opacity">
                                <AlbumLikeButton albumKey={metadataKey} size={20} />
                            </div>
                            <div
                                className="w-full aspect-square rounded-md mb-3 shadow-xl flex items-center justify-center text-display font-bold text-text-main/20 relative overflow-hidden bg-surface-3"
                                style={{ background: coverBackground(coverUrl, album.name) }}
                            >
                                {!coverUrl && <span className="z-10">{album.name.charAt(0)}</span>}
                                <div className="absolute bottom-2 right-2 w-11 h-11 bg-brand rounded-full flex items-center justify-center shadow-xl translate-y-2 opacity-0 group-hover:translate-y-0 group-hover:opacity-100 transition-all duration-200">
                                    <svg className="w-5 h-5 text-surface-0 fill-current ml-0.5" viewBox="0 0 24 24"><path d="M8 5v14l11-7z"/></svg>
                                </div>
                            </div>
                            <h4 className="font-semibold text-sm truncate text-text-main mb-1">{album.name}</h4>
                            <div className="flex justify-between items-center mt-auto">
                                <p className="text-xs text-text-secondary truncate max-w-[75%]">{album.artist}</p>
                                {metadata?.releaseDate && (
                                    <span className="text-[11px] text-text-subtle">{new Date(metadata.releaseDate).getFullYear()}</span>
                                )}
                            </div>
                        </div>
                    );
                }}
              />
            </div>
        </Page>
    );
};

export default LikedAlbums;
