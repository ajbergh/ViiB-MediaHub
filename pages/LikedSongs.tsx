/**
 * ViiB MediaHub - Liked Songs Page
 * 
 * Displays all songs that the user has liked (favorited).
 * Songs are sorted by when they were liked (newest first).
 * 
 * Features:
 * - Virtualized list for performance with large libraries
 * - Play all liked songs in order
 * - Shuffle play functionality
 * - Unlike songs directly from the list
 * - Context menu support for additional actions
 * - Empty state using EmptyState component with centralized copy
 * 
 * Design System Usage:
 * - EmptyState component with copy from lib/emptyStateCopy.ts
 * - ListHeader for virtualized list header
 * - LikeButton for consistent like/unlike UI
 * 
 * @module LikedSongs
 */

import React, { useMemo } from 'react';
import { useNavigate } from 'react-router';
import { useStore, useAlbumCovers } from '../store';
import { Play, Heart, MoreHorizontal, Shuffle, Clock } from 'lucide-react';
import { formatTime, generateGradient } from '../utils';
import { ContextMenuType } from '../types';
import { Virtuoso, Components } from 'react-virtuoso';
import { LikeButton } from '../components/LikeButton';
import { ListHeader } from '../components/ui/Page';
import { EmptyState } from '../components/EmptyState';
import { EMPTY_STATE } from '../lib/emptyStateCopy';

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
        <ListHeader>
            {/* Header Section */}
            <div className="flex flex-col sm:flex-row items-start sm:items-end gap-6 mb-6">
                {/* Spotify-inspired Liked Songs Cover */}
                <div className="w-40 h-40 sm:w-48 sm:h-48 rounded-md bg-gradient-to-br from-accent-purple via-accent-blue to-brand/80 flex items-center justify-center shadow-2xl flex-shrink-0">
                    <Heart size={72} className="text-text-main fill-current drop-shadow-md" />
                </div>
                
                <div className="flex-1 min-w-0">
                    <p className="text-xs uppercase tracking-widest text-text-secondary mb-1.5 font-bold">Playlist</p>
                    <h1 className="text-display font-extrabold tracking-tight text-text-main mb-3">Liked Songs</h1>
                    <p className="text-text-secondary text-sm font-medium">
                        {context?.songCount || 0} songs • {context?.totalDuration || '0m'}
                    </p>
                </div>
            </div>

            {/* Actions */}
            <div className="flex items-center gap-5 mb-6">
                <button 
                    onClick={handlePlayAll}
                    disabled={likedSongs.length === 0}
                    className="w-14 h-14 bg-brand hover:bg-brand-hover rounded-full flex items-center justify-center hover:scale-105 transition-all duration-200 shadow-lg text-surface-0 disabled:opacity-50 disabled:cursor-not-allowed disabled:hover:scale-100"
                    aria-label="Play liked songs"
                >
                    <Play size={26} className="fill-current ml-1" />
                </button>
                <button 
                    onClick={handleShuffle}
                    disabled={likedSongs.length === 0}
                    className="text-text-secondary hover:text-text-main transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                    title="Shuffle play"
                    aria-label="Shuffle liked songs"
                >
                    <Shuffle size={24} />
                </button>
            </div>

            {/* Table Header */}
            <div className="grid grid-cols-[40px_1fr_52px_36px] md:grid-cols-[44px_4fr_3fr_3fr_60px_40px_40px] gap-2 md:gap-4 px-4 py-2 text-text-subtle text-xs uppercase tracking-wider font-semibold border-b border-surface-border mb-1">
                <div className="text-center">#</div>
                <div>Title</div>
                <div className="hidden md:block">Album</div>
                <div className="hidden md:block">Artist</div>
                <div className="flex justify-center"><Clock size={14} aria-label="Duration" /></div>
                <div></div>
                <div className="hidden md:block"></div>
            </div>
        </ListHeader>
    );
};

const Footer: React.FC = () => <div className="h-8" />;

export const LikedSongs: React.FC = () => {
    const { songs, likedSongIds, currentSong, isPlaying, playSong, openContextMenu } = useStore();
    const albumCovers = useAlbumCovers();
    const navigate = useNavigate();
    
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
    const likedCopy = EMPTY_STATE.likedSongs;
    if (likedSongs.length === 0) {
        return (
            <div className="h-full animate-fade-in">
                <div className="px-4 pt-6 sm:px-6 lg:px-8">
                    {/* Header Section */}
                    <div className="flex flex-col sm:flex-row items-start sm:items-end gap-6 mb-8">
                        <div className="w-40 h-40 sm:w-48 sm:h-48 rounded-md bg-gradient-to-br from-accent-purple/70 via-accent-blue/60 to-surface-3 flex items-center justify-center shadow-[0_8px_24px_rgba(0,0,0,0.5)] flex-shrink-0">
                            <Heart size={72} className="text-text-main/80 fill-current" />
                        </div>
                        
                        <div className="flex-1">
                            <p className="text-xs uppercase tracking-widest text-text-secondary mb-1.5 font-bold">Playlist</p>
                            <h1 className="text-display font-extrabold tracking-tight text-text-main mb-2">Liked Songs</h1>
                            <p className="text-text-secondary text-sm">0 songs</p>
                        </div>
                    </div>
                </div>
                
                <div className="flex items-center justify-center py-8">
                    <EmptyState
                        icon={<likedCopy.icon size={40} />}
                        title={likedCopy.title}
                        description={likedCopy.description}
                        action={likedCopy.primaryAction ? {
                            label: likedCopy.primaryAction,
                            onClick: () => navigate('/songs')
                        } : undefined}
                        secondaryAction={likedCopy.secondaryAction ? {
                            label: likedCopy.secondaryAction,
                            onClick: () => navigate('/smart-playlists')
                        } : undefined}
                    />
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
                        <div className="bg-surface-1 px-4 sm:px-6 lg:px-8">
                            <div 
                                className={`grid grid-cols-[40px_1fr_52px_36px] md:grid-cols-[44px_4fr_3fr_3fr_60px_40px_40px] gap-2 md:gap-4 rounded-md px-4 py-2 items-center hover:bg-surface-hover group transition-colors cursor-pointer ${isCurrent ? 'bg-surface-hover' : 'bg-transparent'}`}
                                onClick={() => playSong(song, likedSongs)}
                                onContextMenu={(e) => openContextMenu(e, ContextMenuType.SONG, song)}
                            >
                                <div className="text-center text-text-subtle tabular-nums text-sm relative h-full flex items-center justify-center">
                                    <span className="group-hover:hidden">{isCurrent && isPlaying ? <div className="w-2.5 h-2.5 bg-brand rounded-full animate-pulse"></div> : index + 1}</span>
                                    <Play size={15} className="hidden group-hover:block text-text-main fill-current absolute" />
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
                                        <span className={`text-sm font-medium truncate ${isCurrent ? 'text-brand' : 'text-text-main'}`}>{song.title}</span>
                                        <span className="md:hidden text-xs text-text-subtle truncate mt-0.5">{song.artist}</span>
                                    </div>
                                </div>
                                <div className="hidden md:block text-text-secondary text-sm truncate">{song.album}</div>
                                <div className="hidden md:block text-text-secondary text-sm truncate">{song.artist}</div>
                                <div className="text-text-secondary text-xs tabular-nums text-center">{formatTime(song.duration)}</div>
                                
                                <div className="flex justify-center">
                                    <LikeButton songId={song.id} size={16} />
                                </div>
                                
                                <div className="hidden md:flex justify-center relative">
                                    <button 
                                        onClick={(e) => openContextMenu(e, ContextMenuType.SONG, song)}
                                        className="text-text-subtle hover:text-text-main transition-opacity opacity-0 group-hover:opacity-100"
                                        aria-label="More options"
                                    >
                                        <MoreHorizontal size={18} />
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
