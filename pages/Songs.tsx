import React, { useState, useEffect, useMemo } from 'react';
import { useStore, useAlbumCovers } from '../store';
import { Play, Clock, MoreHorizontal, Search } from 'lucide-react';
import { formatTime, generateGradient } from '../utils';
import { ContextMenuType } from '../types';
import { Virtuoso, Components } from 'react-virtuoso';

// Context interface for the Virtuoso list
interface SongsContext {
    filter: string;
    setFilter: (val: string) => void;
}

// Define Header outside to maintain stability
const SongsHeader: React.FC<{ context?: SongsContext }> = ({ context }) => {
    // Safety check for context
    const { filter, setFilter } = context || { filter: '', setFilter: () => {} };

    return (
        <div className="p-8 pb-0">
            <div className="flex flex-col md:flex-row items-start md:items-center justify-between mb-8 gap-4">
                <h1 className="text-3xl font-bold">All Songs</h1>
                <div className="relative w-full md:w-96">
                    <Search className="absolute left-3 top-1/2 -translate-y-1/2 text-text-secondary" size={18} />
                    <input 
                        type="text" 
                        placeholder="Search songs, artists, or albums..."
                        value={filter}
                        onChange={(e) => setFilter(e.target.value)}
                        className="w-full bg-surface-highlight border border-transparent focus:border-surface-slider rounded-full py-2 pl-10 pr-4 text-sm text-text-main outline-none placeholder-text-subtle"
                    />
                </div>
            </div>

            {/* Table Header */}
            <div className="bg-surface-1 rounded-t-lg sticky top-0 z-10 border-b border-surface-3 grid grid-cols-[50px_4fr_3fr_3fr_100px_50px] gap-4 px-4 py-3 text-text-secondary text-xs uppercase tracking-wider font-medium shadow-md">
                <div className="text-center">#</div>
                <div>Title</div>
                <div>Album</div>
                <div>Artist</div>
                <div className="flex justify-end pr-2"><Clock size={16} /></div>
                <div></div>
            </div>
        </div>
    );
};

const Footer = () => <div className="h-32 bg-transparent" />;

export const Songs: React.FC = () => {
  const { songs, playSong, currentSong, isPlaying, openContextMenu } = useStore();
  const albumCovers = useAlbumCovers();
  const [filter, setFilter] = useState('');
  const [scrollParent, setScrollParent] = useState<HTMLElement | null>(null);

  useEffect(() => {
    // Find the main scroll container from Layout
    setScrollParent(document.querySelector('main'));
  }, []);

  const filteredSongs = useMemo(() => songs.filter(
    s => s.title.toLowerCase().includes(filter.toLowerCase()) || 
         s.artist.toLowerCase().includes(filter.toLowerCase()) ||
         s.album.toLowerCase().includes(filter.toLowerCase())
  ), [songs, filter]);

  // Memoize components to prevent re-renders of the list structure
  const components: Components<any, any> = useMemo(() => ({
    Header: SongsHeader,
    Footer: Footer
  }), []);

  return (
    <div className="h-full">
        <Virtuoso
            useWindowScroll={false}
            customScrollParent={scrollParent}
            data={filteredSongs}
            context={{ filter, setFilter }}
            components={components}
            itemContent={(index, song) => {
               const isCurrent = currentSong?.id === song.id;
               const displayCover = song.coverUrl || albumCovers[song.album];
               
               return (
                <div className="bg-surface-1 px-8"> {/* Wrapper to match page padding visually for bg */}
                    <div 
                        className={`grid grid-cols-[50px_4fr_3fr_3fr_100px_50px] gap-4 px-4 py-3 items-center hover:bg-surface-hover group transition-colors cursor-pointer border-b border-transparent hover:border-surface-highlight ${isCurrent ? 'bg-surface-hover' : 'bg-surface-1'}`}
                        onClick={() => playSong(song)}
                        onContextMenu={(e) => openContextMenu(e, ContextMenuType.SONG, song)}
                    >
                        <div className="text-center text-text-subtle font-mono text-sm relative h-full flex items-center justify-center">
                            <span className="group-hover:hidden">{isCurrent && isPlaying ? <div className="w-3 h-3 bg-brand rounded-full animate-pulse"></div> : index + 1}</span>
                            <Play size={16} className="hidden group-hover:block text-text-main fill-current absolute" />
                        </div>
                        <div className="flex items-center gap-3 overflow-hidden">
                            <div className="w-10 h-10 flex-shrink-0 rounded bg-surface-3 overflow-hidden relative">
                                    {displayCover ? (
                                    <img src={displayCover} alt={song.album} className="w-full h-full object-cover" />
                                    ) : (
                                    <div className="w-full h-full" style={{ background: generateGradient(song.album) }}></div>
                                    )}
                            </div>
                            <div className="flex flex-col truncate">
                                <span className={`font-medium truncate ${isCurrent ? 'text-brand' : 'text-text-main'}`}>{song.title}</span>
                            </div>
                        </div>
                        <div className="text-text-secondary text-sm truncate">{song.album}</div>
                        <div className="text-text-secondary text-sm truncate">{song.artist}</div>
                        <div className="text-text-secondary text-sm font-mono text-right pr-2">{formatTime(song.duration)}</div>
                        
                        <div className="flex justify-center relative">
                            <button 
                                onClick={(e) => openContextMenu(e, ContextMenuType.SONG, song)}
                                className={`text-text-subtle hover:text-text-main transition-opacity opacity-0 group-hover:opacity-100`}
                            >
                                <MoreHorizontal size={20} />
                            </button>
                        </div>
                    </div>
                </div>
               );
            }}
        />
        
        {filteredSongs.length === 0 && (
             <div className="p-12 text-center text-text-subtle absolute top-40 w-full pointer-events-none">
                <p>No songs found.</p>
             </div>
        )}
    </div>
  );
};