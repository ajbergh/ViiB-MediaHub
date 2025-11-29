/**
 * ViiB MediaHub - Albums Page
 * 
 * Displays the album library in a responsive grid with virtualized scrolling.
 * 
 * Features:
 * - Virtualized grid using react-virtuoso for large libraries
 * - Album covers with gradient fallbacks
 * - Multiple sort options (name, artist, song count, recently added)
 * - Click to navigate to album detail page
 * - Context menu support for each album
 * 
 * @module Albums
 */

import React, { useEffect, useState, forwardRef, useMemo } from 'react';
import { useAlbums, useStore } from '../store';
import { generateGradient, coverBackground } from '../utils';
import { useNavigate } from 'react-router-dom';
import { ContextMenuType, Album } from '../types';
import { VirtuosoGrid } from 'react-virtuoso';
import { ChevronDown, ArrowUpDown } from 'lucide-react';

type AlbumSortOption = 'recent' | 'name-asc' | 'name-desc' | 'artist-asc' | 'artist-desc' | 'songs-desc' | 'songs-asc';

const sortLabels: Record<AlbumSortOption, string> = {
  'recent': 'Recently Added',
  'name-asc': 'Name (A-Z)',
  'name-desc': 'Name (Z-A)',
  'artist-asc': 'Artist (A-Z)',
  'artist-desc': 'Artist (Z-A)',
  'songs-desc': 'Most Songs',
  'songs-asc': 'Fewest Songs',
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

export const Albums: React.FC = () => {
  const albums = useAlbums();
  const { openContextMenu, fetchAlbumMetadata, albumMetadata } = useStore();
  const navigate = useNavigate();
  const [scrollParent, setScrollParent] = useState<HTMLElement | null>(null);
  const [sortBy, setSortBy] = useState<AlbumSortOption>('recent');
  const [showSortMenu, setShowSortMenu] = useState(false);

  useEffect(() => {
    setScrollParent(document.querySelector('main'));
  }, []);

  useEffect(() => {
    // Only fetch metadata for albums that are MISSING artwork
    // This prevents unnecessary Spotify API calls for albums that already have covers
    const albumsMissingArt = albums.filter(album => {
        const metadataKey = `${album.name}::${album.artist}`;
        // Skip if we already have metadata OR if album has embedded cover
        const hasMetadata = !!albumMetadata[metadataKey];
        const hasEmbeddedCover = !!album.coverUrl;
        return !hasMetadata && !hasEmbeddedCover;
    });

    // Limit to first 30 to avoid rate limiting, with staggered requests
    albumsMissingArt.slice(0, 30).forEach((album, idx) => {
        setTimeout(() => {
            fetchAlbumMetadata(album.name, album.artist);
        }, idx * 200); // 200ms between requests to be gentle on Spotify API
    });
  }, [albums.length, albumMetadata]);

  const sortedAlbums = useMemo(() => {
    const sorted = [...albums];
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
      case 'songs-desc':
        return sorted.sort((a, b) => b.songCount - a.songCount);
      case 'songs-asc':
        return sorted.sort((a, b) => a.songCount - b.songCount);
      default:
        return sorted;
    }
  }, [albums, sortBy]);

  return (
    <div className="p-8 h-full">
        <div className="mb-8 flex flex-col md:flex-row md:items-center md:justify-between gap-4">
            <div>
                <h1 className="text-3xl font-bold mb-2">Albums</h1>
                <p className="text-text-secondary">{albums.length} albums</p>
            </div>
            
            {/* Sort Dropdown */}
            <div className="relative">
                <button
                    onClick={() => setShowSortMenu(!showSortMenu)}
                    className="flex items-center gap-2 px-4 py-2 bg-surface-highlight hover:bg-surface-hover rounded-full text-sm text-text-main transition-colors border border-transparent hover:border-surface-slider"
                >
                    <ArrowUpDown size={16} className="text-text-secondary" />
                    <span>{sortLabels[sortBy]}</span>
                    <ChevronDown size={16} className={`text-text-secondary transition-transform ${showSortMenu ? 'rotate-180' : ''}`} />
                </button>
                
                {showSortMenu && (
                    <>
                        <div className="fixed inset-0 z-40" onClick={() => setShowSortMenu(false)} />
                        <div className="absolute right-0 top-full mt-2 bg-surface-2 border border-surface-3 rounded-lg shadow-xl z-50 py-1 min-w-[180px]">
                            {(Object.keys(sortLabels) as AlbumSortOption[]).map((option) => (
                                <button
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

        {sortedAlbums.length === 0 ? (
             <div className="flex flex-col items-center justify-center py-20 text-text-subtle">
                <p>No albums found.</p>
            </div>
        ) : (
            <VirtuosoGrid
                useWindowScroll={false}
                customScrollParent={scrollParent}
                data={sortedAlbums}
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
                            className="bg-surface-2 p-4 rounded-lg hover:bg-surface-3 transition-all group cursor-pointer border border-transparent hover:border-surface-border h-full flex flex-col"
                            onClick={() => navigate(`/album/${encodeURIComponent(album.name)}`)}
                            onContextMenu={(e) => openContextMenu(e, ContextMenuType.ALBUM, album)}
                        >
                            <div 
                                className="w-full aspect-square rounded-md mb-4 shadow-lg flex items-center justify-center text-5xl font-bold text-white/20 relative overflow-hidden bg-surface-3"
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
                                    <span className="text-[10px] text-[#555] font-mono">{new Date(metadata.releaseDate).getFullYear()}</span>
                                )}
                            </div>
                        </div>
                    );
                }}
            />
        )}
    </div>
  );
};
