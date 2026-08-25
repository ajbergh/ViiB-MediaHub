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
import { coverBackground } from '../utils';
import { useNavigate } from 'react-router';
import { ContextMenuType } from '../types';
import { VirtuosoGrid } from 'react-virtuoso';
import { ChevronDown, ArrowUpDown } from 'lucide-react';
import { EmptyAlbums } from '../components/EmptyState';
import { Button } from '../components/ui/Button';
import { Card } from '../components/ui/Card';
import { Menu, MenuItem } from '../components/ui/Menu';
import { Page, PageHeader } from '../components/ui/Page';
import { CardSizeSlider } from '../components/ui/CardSizeSlider';
import { resolveAlbumArtwork } from '../lib/artwork';

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

const ListContainer = forwardRef<HTMLDivElement, any>(({ style, children, ...props }, ref) => (
  <div
    ref={ref}
    {...props}
    style={{
      ...style,
      display: 'grid',
      gridTemplateColumns: 'repeat(var(--card-cols, 4), minmax(0, 1fr))',
      gap: '1.5rem',
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

export const Albums: React.FC = () => {
  const albums = useAlbums();
  const { openContextMenu, fetchAlbumMetadata, albumMetadata } = useStore();
  const navigate = useNavigate();
  const [scrollParent, setScrollParent] = useState<HTMLElement | null>(null);
  const [sortBy, setSortBy] = useState<AlbumSortOption>('recent');
  const [showSortMenu, setShowSortMenu] = useState(false);
  const [cardCols, setCardCols] = useState(() => Number(localStorage.getItem('albums-card-cols') ?? 4));
  const handleCardColsChange = (v: number) => { setCardCols(v); localStorage.setItem('albums-card-cols', String(v)); };

  useEffect(() => {
    setScrollParent(document.querySelector('main'));
  }, []);

  useEffect(() => {
    // Only fetch metadata for albums that are MISSING artwork.
    // Plex artwork is supplied by PMS and must not be replaced by enrichment.
    const albumsMissingArt = albums.filter(album => {
        const metadataKey = `${album.name}::${album.artist}`;
        const hasMetadata = !!albumMetadata[metadataKey];
        const hasCatalogCover = !!album.coverUrl;
        return !hasMetadata && !hasCatalogCover;
    });

    albumsMissingArt.slice(0, 30).forEach((album, idx) => {
        setTimeout(() => {
            fetchAlbumMetadata(album.name, album.artist);
        }, idx * 200);
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
    <Page withPlayerPadding={false}>
        <PageHeader
          heading="Albums"
          subtitle={`${albums.length} albums`}
          actions={
            <div className="flex items-center gap-3">
              <CardSizeSlider value={cardCols} onChange={handleCardColsChange} />
              <div className="relative">
                <Button
                  variant="secondary"
                  onClick={() => setShowSortMenu(!showSortMenu)}
                  className="rounded-full"
                  leftIcon={<ArrowUpDown size={16} className="text-text-secondary" aria-hidden="true" />}
                  rightIcon={<ChevronDown size={16} className={`text-text-secondary transition-transform ${showSortMenu ? 'rotate-180' : ''}`} aria-hidden="true" />}
                  aria-haspopup="menu"
                  aria-expanded={showSortMenu}
                >
                  {sortLabels[sortBy]}
                </Button>
                {showSortMenu && (
                  <>
                    <button
                      type="button"
                      tabIndex={-1}
                      aria-label="Close sort menu"
                      className="fixed inset-0 z-40"
                      onClick={() => setShowSortMenu(false)}
                    />
                    <Menu
                      className="absolute right-0 top-full mt-2 z-50 min-w-[180px]"
                      onRequestClose={() => setShowSortMenu(false)}
                    >
                      {(Object.keys(sortLabels) as AlbumSortOption[]).map((option) => (
                        <MenuItem
                          key={option}
                          onClick={() => { setSortBy(option); setShowSortMenu(false); }}
                          active={sortBy === option}
                        >
                          {sortLabels[option]}
                        </MenuItem>
                      ))}
                    </Menu>
                  </>
                )}
              </div>
            </div>
          }
        />

        {sortedAlbums.length === 0 ? (
          <EmptyAlbums />
        ) : (
          <div style={{ '--card-cols': cardCols } as React.CSSProperties}>
            <VirtuosoGrid
              useWindowScroll={false}
              customScrollParent={scrollParent}
              data={sortedAlbums}
              components={{ List: ListContainer, Item: ItemContainer }}
              itemContent={(index, album) => {
                const metadataKey = `${album.name}::${album.artist}`;
                const metadata = albumMetadata[metadataKey];
                const coverUrl = resolveAlbumArtwork(album.coverUrl, metadata?.coverUrl);

                return (
                  <Card
                    interactive
                    className="p-4 h-full flex flex-col cursor-pointer"
                    onClick={() => navigate(`/album/${encodeURIComponent(album.name)}/${encodeURIComponent(album.artist)}`)}
                    onContextMenu={(e) => openContextMenu(e, ContextMenuType.ALBUM, album)}
                    role="button"
                    tabIndex={0}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault();
                        navigate(`/album/${encodeURIComponent(album.name)}/${encodeURIComponent(album.artist)}`);
                      }
                    }}
                  >
                    <div
                      className="w-full aspect-square rounded-md mb-4 shadow-lg flex items-center justify-center text-display font-bold text-white/20 relative overflow-hidden bg-surface-3"
                      style={{ background: coverBackground(coverUrl, album.name) }}
                    >
                      {!coverUrl && <span className="z-10">{album.name.charAt(0)}</span>}
                      <div className="absolute inset-0 bg-black/40 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center">
                        <Button
                          variant="primary"
                          className="w-12 h-12 rounded-full p-0 translate-y-4 group-hover:translate-y-0 transition-transform duration-200"
                          aria-label="Open album"
                        >
                          <svg className="w-6 h-6 text-black fill-current ml-0.5" viewBox="0 0 24 24" aria-hidden="true"><path d="M8 5v14l11-7z"/></svg>
                        </Button>
                      </div>
                    </div>
                    <h4 className="font-bold truncate text-text-main mb-1">{album.name}</h4>
                    <div className="flex justify-between items-center mt-auto">
                      <p className="text-sm text-text-secondary truncate max-w-[70%]">{album.artist}</p>
                      {metadata?.releaseDate && (
                        <span className="text-[10px] text-text-subtle font-mono">{new Date(metadata.releaseDate).getFullYear()}</span>
                      )}
                    </div>
                  </Card>
                );
              }}
            />
          </div>
        )}
    </Page>
  );
};
