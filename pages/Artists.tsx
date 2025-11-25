
import React, { useEffect, useState, forwardRef } from 'react';
import { useArtists, useStore } from '../store';
import { generateGradient } from '../utils';
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
  const artists = useArtists();
  const { openContextMenu, fetchArtistMetadata, artistMetadata } = useStore();
  const [scrollParent, setScrollParent] = useState<HTMLElement | null>(null);

  useEffect(() => {
    setScrollParent(document.querySelector('main'));
  }, []);

  useEffect(() => {
      // Trigger background fetches for visible artists (limited initial batch)
      artists.slice(0, 50).forEach((artist, idx) => {
          setTimeout(() => {
            fetchArtistMetadata(artist.name);
          }, idx * 100);
      });
  }, [artists.length]); 

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

                    return (
                        <div 
                            className="bg-surface-2 p-6 rounded-lg hover:bg-surface-3 transition-all group cursor-pointer flex flex-col items-center text-center relative overflow-hidden h-full"
                            onContextMenu={(e) => openContextMenu(e, ContextMenuType.ARTIST, artist)}
                        >
                            {/* Background blur effect for metadata enhancement hint */}
                            {metadata?.imageUrl && (
                                <div className="absolute inset-0 bg-brand/5 opacity-0 group-hover:opacity-100 transition-opacity pointer-events-none"></div>
                            )}

                            <div 
                                className="w-40 h-40 rounded-full mb-4 shadow-lg flex items-center justify-center text-5xl font-bold text-white/20 relative overflow-hidden bg-center bg-cover border-4 border-surface-3 group-hover:border-surface-border transition-colors flex-shrink-0"
                                style={{ 
                                    background: displayImage
                                        ? `url(${displayImage})` 
                                        : generateGradient(artist.name),
                                    backgroundSize: 'cover',
                                    backgroundPosition: 'center'
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
