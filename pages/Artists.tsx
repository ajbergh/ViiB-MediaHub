/**
 * ViiB MediaHub - Artists Page
 * 
 * Displays all artists in the library with virtualized grid layout.
 * 
 * Features:
 * - Virtualized grid using react-virtuoso for large libraries
 * - Artist images from catalog/enrichment data, including Plex-aware mappings, or gradient fallback
 * - Shows song and album count per artist
 * - Click to navigate to artist detail page
 * - Context menu support for each artist
 * 
 * @module Artists
 */

import React, { useEffect, useState, forwardRef, useCallback, useMemo } from 'react';
import { useNavigate } from 'react-router';
import { useArtists, useStore } from '../store';
import { generateGradient, cssUrl } from '../utils';
import { ContextMenuType } from '../types';
import { VirtuosoGrid } from 'react-virtuoso';
import { Search } from 'lucide-react';
import { EmptyArtists } from '../components/EmptyState';
import { Page, PageHeader } from '../components/ui/Page';
import { CardSizeSlider, MAX_CARD_COLUMNS } from '../components/ui/CardSizeSlider';
import { LibraryListHeader, LibraryListRow } from '../components/ui/LibraryList';
import { TextInput } from '../components/ui/TextInput';

// Define Grid Components
// gridTemplateColumns driven by --card-cols CSS variable on the parent wrapper.
const ListContainer = forwardRef<HTMLDivElement, any>(({ style, children, ...props }, ref) => (
  <div
    ref={ref}
    {...props}
    style={{
      ...style,
      display: 'grid',
      gridTemplateColumns: 'repeat(var(--card-cols, 5), minmax(0, 1fr))',
      gap: 'var(--card-gap, 1.5rem)',
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

export const Artists: React.FC = () => {
  const navigate = useNavigate();
  const artists = useArtists();
  const { openContextMenu, artistMetadata } = useStore();
  const [scrollParent, setScrollParent] = useState<HTMLElement | null>(null);
  const [filter, setFilter] = useState('');
  const [cardCols, setCardCols] = useState(() => Number(localStorage.getItem('artists-card-cols') ?? 5));
  const listView = cardCols === MAX_CARD_COLUMNS;
  const handleCardColsChange = (v: number) => { setCardCols(v); localStorage.setItem('artists-card-cols', String(v)); };

  // Debug: Log artistMetadata on first render and when it changes
  useEffect(() => {
    console.log(`🎨 Artists page: artistMetadata has ${Object.keys(artistMetadata).length} entries`);
    if (Object.keys(artistMetadata).length > 0) {
      const sample = Object.entries(artistMetadata).slice(0, 3);
      sample.forEach(([name, meta]) => {
        console.log(`   📷 "${name}": imageUrl = ${meta.imageUrl?.substring(0, 80)}...`);
      });
    }
  }, [artistMetadata]);

  useEffect(() => {
    setScrollParent(document.querySelector('main'));
  }, []);

  const handleArtistClick = useCallback((artistName: string) => {
      navigate(`/artist/${encodeURIComponent(artistName)}`);
  }, [navigate]);

  const filteredArtists = useMemo(() => {
    if (!filter.trim()) return artists;
    const query = filter.toLowerCase();
    return artists.filter(artist => artist.name.toLowerCase().includes(query));
  }, [artists, filter]);

  return (
    <Page withPlayerPadding={false}>
        <PageHeader heading="Artists" subtitle={`${filteredArtists.length}${filter ? ` of ${artists.length}` : ''} artists`}
          actions={
            <div className="flex items-center gap-3">
              <CardSizeSlider listAtMinimum value={cardCols} onChange={handleCardColsChange} />
              {/* Search Input */}
              <div className="w-full md:w-72">
                <TextInput
                  leftIcon={<Search size={18} className="text-text-secondary" aria-hidden="true" />}
                  type="text"
                  placeholder="Search artists…"
                  aria-label="Search artists"
                  value={filter}
                  onChange={(e) => setFilter(e.target.value)}
                  className="rounded-full"
                />
              </div>
            </div>
          }
        />

        {artists.length === 0 ? (
            <EmptyArtists />
        ) : (
             <div style={{ '--card-cols': listView ? 1 : cardCols, '--card-gap': listView ? '0px' : '0.5rem' } as React.CSSProperties}>
             {listView && <LibraryListHeader title="Artist" detail="Albums" stats="Tracks" />}
             <VirtuosoGrid
                key={listView ? 'list' : 'tiles'}
                useWindowScroll={false}
                customScrollParent={scrollParent}
                data={filteredArtists}
                components={{
                    List: ListContainer,
                    Item: ItemContainer
                }}
                itemContent={(index, artist) => {
                    const metadata = artistMetadata[artist.name];
                    const displayImage = metadata?.imageUrl || artist.imageUrl;

                    if (listView) return <LibraryListRow index={index} name={artist.name}
                      artwork={<div className="w-full h-full rounded-full flex items-center justify-center" style={{ background: displayImage ? `${cssUrl(displayImage)} center/cover no-repeat` : generateGradient(artist.name) }}>{!displayImage && artist.name.charAt(0)}</div>}
                      detail={`${artist.albumCount} albums`} stats={String(artist.songCount)}
                      onOpen={() => handleArtistClick(artist.name)} onContextMenu={e => openContextMenu(e, ContextMenuType.ARTIST, artist)} />;

                    return (
                        <div 
                            className="bg-transparent p-3 rounded-lg hover:bg-surface-hover transition-colors group cursor-pointer flex flex-col items-start text-left relative overflow-hidden h-full"
                            onClick={() => handleArtistClick(artist.name)}
                            onContextMenu={(e) => openContextMenu(e, ContextMenuType.ARTIST, artist)}
                        >
                            <div className="relative w-full aspect-square mb-3">
                                <div 
                                  className="w-full h-full rounded-full shadow-xl flex items-center justify-center text-display font-bold text-text-main/30 overflow-hidden bg-surface-3"
                                    style={{ 
                                        background: displayImage
                                            ? `${cssUrl(displayImage)} center/cover no-repeat` 
                                            : generateGradient(artist.name)
                                    }}
                                >
                                    {!displayImage && artist.name.charAt(0)}
                                </div>
                                <div className="absolute bottom-1 right-1 w-11 h-11 rounded-full bg-brand text-surface-0 flex items-center justify-center shadow-xl translate-y-2 opacity-0 group-hover:translate-y-0 group-hover:opacity-100 transition-all duration-200">
                                    <svg className="w-5 h-5 fill-current ml-0.5" viewBox="0 0 24 24" aria-hidden="true"><path d="M8 5v14l11-7z"/></svg>
                                </div>
                            </div>
                            <h4 className="font-semibold text-sm truncate text-text-main mb-1 w-full">{artist.name}</h4>
                            <p className="text-xs text-text-secondary">Artist • {artist.songCount} songs</p>
                        </div>
                    );
                }}
             />
             </div>
        )}
    </Page>
  );
};
