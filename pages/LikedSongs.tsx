/**
 * ViiB MediaHub - Liked Songs Page
 * 
 * Displays all songs that the user has liked (favorited).
 * Songs are sorted by when they were liked (newest first).
 * 
 * Features:
 * - Virtualized list for performance with large libraries
 * - Play all liked songs in order
 * - Unlike songs directly from the list
 * - Empty state for users with no liked songs
 * 
 * @module LikedSongs
 */

import React, { useMemo } from 'react';
import { useStore, useAlbumCovers } from '../store';
import { Play, Heart, MoreHorizontal, Shuffle } from 'lucide-react';
import { formatTime, generateGradient } from '../utils';
import { ContextMenuType, Song } from '../types';
import { Virtuoso, Components } from 'react-virtuoso';
import { LikeButton } from '../components/LikeButton';

// Context interface for the Virtuoso list
interface LikedContext {
    totalDuration: string;
    songCount: number;
}

// Define Header component
const LikedHeader: React.FC<{ context?: LikedContext }> = ({ context }) => {
    const { playSong, songs, likedSongIds } = useStore();
    
    // Get liked songs sorted by likedAt (newest first)
    const likedSongs = useMemo(() => {
        return songs
            .filter(s => likedSongIds.has(s.id))
            .sort((a, b) => (b.likedAt || 0) - (a.likedAt || 0));
    }, [songs, likedSongIds]);

    const handlePlayAll = () => {
        if (likedSongs.length > 0) {
            playSong(likedSongs[0], likedSongs);
        }
    };

    const handleShuffle = () => {
        if (likedSongs.length > 0) {
            const shuffled = [...likedSongs].sort(() => Math.random() - 0.5);
            playSong(shuffled[0], shuffled);
        }
    };

    return (
        <div className="p-8 pb-0">
            {/* Header Section */}
            <div className="flex items-end gap-6 mb-8">
                {/* Liked Songs Icon */}
                <div className="w-48 h-48 rounded-lg bg-gradient-to-br from-brand to-surface-1 flex items-center justify-center shadow-2xl shadow-brand/30">
                    <Heart size={80} className="text-white fill-current" />
                </div>
                
                <div className="flex-1">
                    <p className="text-xs uppercase tracking-widest text-text-secondary mb-2 font-semibold">Playlist</p>
                    <h1 className="text-5xl font-black text-text-main mb-4">Liked Songs</h1>
                    <p className="text-text-secondary text-sm">
                        {context?.songCount || 0} songs • {context?.totalDuration || '0:00'} total
                    </p>
                </div>
            </div>

            {/* Actions */}
            <div className="flex items-center gap-4 mb-8">
                <button 
                    onClick={handlePlayAll}
                    disabled={likedSongs.length === 0}
                    className="w-14 h-14 bg-brand hover:bg-brand-hover rounded-full flex items-center justify-center hover:scale-105 transition-all duration-200 shadow-lg shadow-black/40 text-black disabled:opacity-50 disabled:cursor-not-allowed disabled:hover:scale-100"
                >
                    <Play size={28} className="fill-current ml-1" />
                </button>
                <button 
                    onClick={handleShuffle}
                    disabled={likedSongs.length === 0}
                    className="text-text-secondary hover:text-white transition-all duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
                    title="Shuffle play"
                >
                    <Shuffle size={28} />
                </button>
            </div>

            {/* Table Header */}
            <div className="grid grid-cols-[50px_4fr_3fr_3fr_60px_40px_50px] gap-4 px-4 py-2 text-text-secondary text-xs uppercase tracking-wider font-medium border-b border-surface-3 mb-2">
                <div className="text-center">#</div>
                <div>Title</div>
                <div>Album</div>
                <div>Artist</div>
                <div className="text-right pr-2">Duration</div>
                <div></div>
                <div></div>
            </div>
        </div>
    );
};

const Footer: React.FC = () => <div className="h-8" />;

export const LikedSongs: React.FC = () => {
    const { songs, likedSongIds, currentSong, isPlaying, playSong, openContextMenu } = useStore();
    const albumCovers = useAlbumCovers();
    
    const scrollParent = document.getElementById('main-content-scroll') as HTMLElement | undefined;

    // Get liked songs sorted by likedAt (newest first)
    const likedSongs = useMemo(() => {
        return songs
            .filter(s => likedSongIds.has(s.id))
            .sort((a, b) => (b.likedAt || 0) - (a.likedAt || 0));
    }, [songs, likedSongIds]);

    // Calculate total duration
    const totalDuration = useMemo(() => {
        const total = likedSongs.reduce((acc, s) => acc + s.duration, 0);
        const hours = Math.floor(total / 3600);
        const minutes = Math.floor((total % 3600) / 60);
        if (hours > 0) {
            return `${hours}h ${minutes}m`;
        }
        return `${minutes}m`;
    }, [likedSongs]);

    const components: Components<any, any> = useMemo(() => ({
        Header: LikedHeader,
        Footer: Footer
    }), []);

    const context: LikedContext = {
        totalDuration,
        songCount: likedSongs.length
    };

    // Empty state
    if (likedSongs.length === 0) {
        return (
            <div className="h-full animate-fade-in">
                <div className="p-8">
                    {/* Header Section */}
                    <div className="flex items-end gap-6 mb-8">
                        <div className="w-48 h-48 rounded-lg bg-gradient-to-br from-brand/40 to-surface-1 flex items-center justify-center shadow-2xl">
                            <Heart size={80} className="text-white/50" />
                        </div>
                        
                        <div className="flex-1">
                            <p className="text-xs uppercase tracking-widest text-text-secondary mb-2 font-semibold">Playlist</p>
                            <h1 className="text-5xl font-black text-text-main mb-4">Liked Songs</h1>
                            <p className="text-text-secondary text-sm">No liked songs yet</p>
                        </div>
                    </div>
                </div>
                
                <div className="flex flex-col items-center justify-center p-12 text-center">
                    <Heart size={64} className="text-text-subtle mb-4" />
                    <h2 className="text-xl font-bold text-text-main mb-2">Songs you like will appear here</h2>
                    <p className="text-text-secondary max-w-md">
                        Click the heart icon on any song to add it to your Liked Songs collection.
                    </p>
                </div>
            </div>
        );
    }

    return (
        <div className="h-full animate-fade-in">
            <Virtuoso
                useWindowScroll={false}
                customScrollParent={scrollParent}
                data={likedSongs}
                context={context}
                components={components}
                itemContent={(index, song) => {
                    const isCurrent = currentSong?.id === song.id;
                    const displayCover = song.coverUrl || albumCovers[song.album];

                    return (
                        <div className="bg-surface-1 px-8">
                            <div 
                                className={`grid grid-cols-[50px_4fr_3fr_3fr_60px_40px_50px] gap-4 px-4 py-3 items-center hover:bg-surface-hover group transition-colors cursor-pointer border-b border-transparent hover:border-surface-highlight ${isCurrent ? 'bg-surface-hover' : 'bg-surface-1'}`}
                                onClick={() => playSong(song, likedSongs)}
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
                                
                                <div className="flex justify-center">
                                    <LikeButton songId={song.id} size={18} />
                                </div>
                                
                                <div className="flex justify-center relative">
                                    <button 
                                        onClick={(e) => openContextMenu(e, ContextMenuType.SONG, song)}
                                        className={`text-text-subtle hover:text-text-main transition-opacity opacity-0 group-hover:opacity-100`}
                                        aria-label="More options"
                                    >
                                        <MoreHorizontal size={20} />
                                    </button>
                                </div>
                            </div>
                        </div>
                    );
                }}
            />
        </div>
    );
};

export default LikedSongs;
