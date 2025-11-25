
import React, { useEffect, useState, forwardRef } from 'react';
import { useAlbums, useStore } from '../store';
import { generateGradient } from '../utils';
import { useNavigate } from 'react-router-dom';
import { ContextMenuType } from '../types';
import { VirtuosoGrid } from 'react-virtuoso';

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

  useEffect(() => {
    setScrollParent(document.querySelector('main'));
  }, []);

  useEffect(() => {
    // Lazily fetch metadata for albums to get better covers
    // We only trigger this for the first 50 to avoid network spam on load
    albums.slice(0, 50).forEach((album, idx) => {
        setTimeout(() => {
            fetchAlbumMetadata(album.name, album.artist);
        }, idx * 100);
    });
  }, [albums.length]);

  return (
    <div className="p-8 h-full">
        <div className="mb-8">
            <h1 className="text-3xl font-bold mb-2">Albums</h1>
            <p className="text-text-secondary">{albums.length} albums</p>
        </div>

        {albums.length === 0 ? (
             <div className="flex flex-col items-center justify-center py-20 text-text-subtle">
                <p>No albums found.</p>
            </div>
        ) : (
            <VirtuosoGrid
                useWindowScroll={false}
                customScrollParent={scrollParent}
                data={albums}
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
                                style={{ background: coverUrl ? `url(${coverUrl}) center/cover no-repeat` : generateGradient(album.name) }}
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
