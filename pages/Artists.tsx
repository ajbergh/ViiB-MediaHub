/**
 * ViiB MediaHub - Artists Page
 * 
 * Displays all artists in the library with virtualized grid layout.
 * 
 * Features:
 * - Virtualized grid using react-virtuoso for large libraries
 * - Artist images from Spotify (via background enrichment) or gradient fallback
 * - Shows song and album count per artist
 * - Click to navigate to artist detail page
 * - Context menu support for each artist
 * 
 * @module Artists
 */

import React, { useEffect, useState, forwardRef, useRef, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import { useArtists, useStore } from '../store';
import { generateGradient, cssUrl } from '../utils';
import { ContextMenuType } from '../types';
import { VirtuosoGrid } from 'react-virtuoso';

// Define Grid Components
const ListContainer = forwardRef<HTMLDivElement, any>(({ style, children, ...props }, ref) => (
  <div
    ref={ref}
    {...props}
    style={style}
    className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-5 gap-6 pb-32"
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
  const { openContextMenu, fetchArtistMetadata, artistMetadata } = useStore();
  const [scrollParent, setScrollParent] = useState<HTMLElement | null>(null);
  const fetchedArtistsRef = useRef<Set<string>>(new Set());
  const batchTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

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

  // Background fetching for ALL artists (slowly, with rate limiting)
  useEffect(() => {
      // Clear any pending timer
      if (batchTimerRef.current) {
          clearTimeout(batchTimerRef.current);
      }

      // Get artists that haven't been fetched yet
      const artistsToFetch = artists.filter(a => 
          !fetchedArtistsRef.current.has(a.name) && 
          !artistMetadata[a.name]
      );

      if (artistsToFetch.length === 0) return;

      let currentIndex = 0;

      const fetchNext = () => {
          if (currentIndex >= artistsToFetch.length) return;

          const artist = artistsToFetch[currentIndex];
          fetchedArtistsRef.current.add(artist.name);
          fetchArtistMetadata(artist.name);
          currentIndex++;

          // Slower rate: 500ms between each fetch to be gentle on Spotify API
          batchTimerRef.current = setTimeout(fetchNext, 500);
      };

      // Start fetching after 2 seconds to let UI settle
      batchTimerRef.current = setTimeout(fetchNext, 2000);

      return () => {
          if (batchTimerRef.current) {
              clearTimeout(batchTimerRef.current);
          }
      };
  }, [artists.length, artistMetadata]); 

  const handleArtistClick = useCallback((artistName: string) => {
      navigate(`/artist/${encodeURIComponent(artistName)}`);
  }, [navigate]);

  return (
    <div className="p-8 h-full">
        <div className="mb-8">
            <h1 className="text-3xl font-bold mb-2">Artists</h1>
            <p className="text-text-secondary">{artists.length} artists</p>
        </div>

        {artists.length === 0 ? (
             <div className="flex flex-col items-center justify-center py-20 text-text-subtle">
                <p>No artists found.</p>
            </div>
        ) : (
             <VirtuosoGrid
                useWindowScroll={false}
                customScrollParent={scrollParent}
                data={artists}
                components={{
                    List: ListContainer,
                    Item: ItemContainer
                }}
                itemContent={(index, artist) => {
                    const metadata = artistMetadata[artist.name];
                    const displayImage = metadata?.imageUrl || artist.imageUrl;
                    
                    // Debug first few artists
                    if (index < 3) {
                        console.log(`🎨 Artist "${artist.name}": metadata=${!!metadata}, imageUrl=${displayImage?.substring(0, 50)}...`);
                    }

                    return (
                        <div 
                            className="bg-surface-2 p-6 rounded-lg hover:bg-surface-3 transition-all group cursor-pointer flex flex-col items-center text-center relative overflow-hidden h-full"
                            onClick={() => handleArtistClick(artist.name)}
                            onContextMenu={(e) => openContextMenu(e, ContextMenuType.ARTIST, artist)}
                        >
                            {/* Background blur effect for metadata enhancement hint */}
                            {metadata?.imageUrl && (
                                <div className="absolute inset-0 bg-brand/5 opacity-0 group-hover:opacity-100 transition-opacity pointer-events-none"></div>
                            )}

                            <div 
                                className="w-40 h-40 rounded-full mb-4 shadow-lg flex items-center justify-center text-5xl font-bold text-white/20 relative overflow-hidden border-4 border-surface-3 group-hover:border-surface-border transition-colors flex-shrink-0"
                                style={{ 
                                    background: displayImage
                                        ? `${cssUrl(displayImage)} center/cover no-repeat` 
                                        : generateGradient(artist.name)
                                }}
                            >
                                {!displayImage && artist.name.charAt(0)}
                            </div>
                            <h4 className="font-bold truncate text-text-main mb-1 w-full text-lg">{artist.name}</h4>
                            <p className="text-sm text-text-secondary">Artist</p>
                            <div className="mt-4 opacity-0 group-hover:opacity-100 transition-opacity">
                                <span className="text-xs bg-surface-border px-3 py-1 rounded-full text-white">{artist.songCount} songs • {artist.albumCount} albums</span>
                            </div>
                        </div>
                    );
                }}
             />
        )}
    </div>
  );
};
