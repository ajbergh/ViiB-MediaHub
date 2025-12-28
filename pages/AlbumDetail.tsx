/**
 * ViiB MediaHub - Album Detail Page
 * 
 * Detailed view of a single album with track listing and metadata.
 * 
 * Features:
 * - Album header with cover art and metadata
 * - Spotify-enriched metadata (description, genre, release date)
 * - Track listing with disc organization for multi-disc albums
 * - Play all, shuffle, add to queue actions
 * - Individual track playback and context menus
 * - Navigation back to albums list
 * 
 * Fetches enhanced metadata from Spotify when connected.
 * 
 * @module AlbumDetail
 */

import React, { useMemo, useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useStore, useAlbumCovers } from '../store';
import { Play, Clock, ArrowLeft, Disc, Download, Heart, MoreHorizontal, ExternalLink, Info, ChevronDown, ChevronUp, RefreshCw } from 'lucide-react';
import { formatTime, generateGradient } from '../utils';
import { ContextMenuType, Song } from '../types';
import { Virtuoso, Components } from 'react-virtuoso';
import api from '../services/api';
import { LikeButton } from '../components/LikeButton';
import { AlbumLikeButton } from '../components/AlbumLikeButton';

// Separate Header Component to be stable
const AlbumHeader: React.FC<{ context?: any }> = ({ context }) => {
    if (!context) return null;

    const {
        decodedAlbumName,
        getHeaderGradient,
        navigate,
        openContextMenu,
        albumObject,
        coverUrl,
        metadata,
        metadataKey,
        firstSong,
        artist,
        year,
        albumSongs,
        durationHours,
        durationMin,
        durationSec,
        playSong,
        showFullDesc,
        setShowFullDesc,
        isRefreshing,
        handleRefreshMetadata,
    } = context;

    return (
        <>
            {/* Dynamic Background Header */}
            <div 
                className="absolute top-0 left-0 w-full h-[500px] z-0 opacity-40 pointer-events-none"
                style={{ background: getHeaderGradient() }}
            />

            {/* Header Content */}
            <div className="relative z-10 p-8 pt-16 flex flex-col md:flex-row gap-8 items-end">
                <button 
                    onClick={() => navigate(-1)} 
                    className="absolute top-6 left-6 w-8 h-8 bg-black/40 rounded-full flex items-center justify-center hover:bg-black/60 text-white transition-colors z-20"
                >
                    <ArrowLeft size={20} />
                </button>

                {/* Album Cover */}
                <div 
                    className="w-52 h-52 shadow-2xl flex-shrink-0 relative group cursor-pointer"
                    onContextMenu={(e) => openContextMenu(e, ContextMenuType.ALBUM, albumObject)}
                >
                    {coverUrl ? (
                        <img src={coverUrl} alt={decodedAlbumName} className="w-full h-full object-cover rounded shadow-lg" />
                    ) : (
                        <div 
                            className="w-full h-full flex items-center justify-center rounded shadow-lg text-white/30 text-6xl font-bold bg-surface-3"
                            style={{ background: generateGradient(decodedAlbumName) }}
                        >
                            {decodedAlbumName.charAt(0)}
                        </div>
                    )}
                </div>

                {/* Album Metadata */}
                <div className="flex flex-col gap-2 z-10 w-full min-w-0">
                    <div className="flex items-center gap-2">
                        <span className="text-xs font-bold uppercase tracking-wider text-white">Album</span>
                        {metadata && <span className="text-[10px] bg-white/20 px-2 py-0.5 rounded text-white font-medium">Enhanced</span>}
                    </div>
                    
                    <h1 
                        className="text-4xl md:text-6xl lg:text-7xl font-bold text-white tracking-tight leading-snug line-clamp-2 drop-shadow-lg"
                        onContextMenu={(e) => openContextMenu(e, ContextMenuType.ALBUM, albumObject)}
                    >
                        {decodedAlbumName}
                    </h1>
                    <div className="flex items-center flex-wrap gap-2 text-sm text-white font-medium mt-4 shadow-black drop-shadow-md">
                        <div className="flex items-center gap-2">
                            <div className="w-6 h-6 rounded-full bg-surface-2 flex items-center justify-center overflow-hidden">
                                {firstSong?.coverUrl ? (
                                    <img src={firstSong.coverUrl} className="w-full h-full object-cover blur-sm scale-150" alt="" />
                                ) : (
                                    <span className="text-[10px]">{artist?.charAt(0)}</span>
                                )}
                            </div>
                            <span 
                                className="hover:underline cursor-pointer font-bold"
                                onContextMenu={(e) => openContextMenu(e, ContextMenuType.ARTIST, {name: artist})}
                            >
                                {artist}
                            </span>
                        </div>
                        <span className="w-1 h-1 bg-white rounded-full mx-1"></span>
                        <span>{metadata?.releaseDate ? new Date(metadata.releaseDate).getFullYear() : (year || 'Unknown Year')}</span>
                        <span className="w-1 h-1 bg-white rounded-full mx-1"></span>
                        <span>{albumSongs.length} songs, <span className="text-text-subtle">{durationHours > 0 ? `${durationHours} hr ` : ''}{durationMin} min</span></span>
                        
                        {metadata?.genre && (
                            <>
                                <span className="w-1 h-1 bg-white rounded-full mx-1"></span>
                                <span className="text-text-subtle">{metadata.genre}</span>
                            </>
                        )}
                    </div>
                </div>
            </div>

            {/* Actions & Description Container */}
            <div className="bg-surface-1/60 p-8 pt-6 backdrop-blur-lg relative z-10">
                <div className="flex items-center gap-6 mb-8 relative">
                    <button 
                        onClick={() => playSong(albumSongs[0], albumSongs)}
                        className="w-14 h-14 bg-brand hover:bg-brand-hover rounded-full flex items-center justify-center hover:scale-105 transition-all duration-200 shadow-lg shadow-black/40 text-black"
                    >
                        <Play size={28} className="fill-current ml-1" />
                    </button>
                    <AlbumLikeButton albumKey={metadataKey} size={32} className="text-text-secondary hover:text-white" />
                    <button className="text-text-secondary hover:text-white transition-all duration-200" aria-label="Download album"><Download size={32} /></button>
                    <div className="relative">
                        <button 
                            onClick={(e) => openContextMenu(e, ContextMenuType.ALBUM, albumObject)}
                            className="text-text-secondary hover:text-white transition-all duration-200"
                            aria-label="More options"
                        >
                            <MoreHorizontal size={32} />
                        </button>
                    </div>
                    {metadata?.url && (
                        <a 
                            href={metadata.url} 
                            target="_blank" 
                            rel="noopener noreferrer"
                            className="ml-auto flex items-center gap-2 text-xs font-bold text-text-secondary hover:text-white bg-white/5 hover:bg-white/10 px-4 py-2 rounded-full transition-all duration-200"
                        >
                            <ExternalLink size={14} /> Spotify
                        </a>
                    )}
                    <button
                        onClick={handleRefreshMetadata}
                        disabled={isRefreshing}
                        className={`${metadata?.url ? '' : 'ml-auto'} flex items-center gap-2 text-xs font-bold text-text-secondary hover:text-white bg-white/5 hover:bg-white/10 px-4 py-2 rounded-full transition-all duration-200 disabled:opacity-50 disabled:cursor-not-allowed`}
                        title="Refresh metadata from Spotify"
                    >
                        <RefreshCw size={14} className={isRefreshing ? 'animate-spin' : ''} /> 
                        {isRefreshing ? 'Refreshing...' : 'Refresh'}
                    </button>
                </div>

                {metadata?.description && (
                    <div className="mb-8 max-w-3xl">
                        <div className={`text-text-secondary text-sm leading-relaxed ${!showFullDesc && 'line-clamp-3'}`}>
                            {metadata.description.replace(/<[^>]*>?/gm, '')}
                        </div>
                        {metadata.description.length > 300 && (
                            <button 
                                onClick={() => setShowFullDesc(!showFullDesc)}
                                className="mt-2 text-white text-xs font-bold hover:underline flex items-center gap-1"
                            >
                                {showFullDesc ? 'Show Less' : 'More'}
                                {showFullDesc ? <ChevronUp size={12} /> : <ChevronDown size={12} />}
                            </button>
                        )}
                    </div>
                )}

                {/* Table Header */}
                <div className="grid grid-cols-[40px_4fr_1fr_60px] gap-4 px-4 py-2 border-b border-surface-3 text-text-secondary text-xs uppercase tracking-wider font-medium mb-2 sticky top-0 bg-surface-1 z-20">
                    <div className="text-center">#</div>
                    <div>Title</div>
                    <div className="hidden md:block text-right pr-8">Plays</div>
                    <div className="text-right pr-4"><Clock size={16} className="inline" /></div>
                </div>
            </div>
        </>
    );
};

const AlbumFooter: React.FC<{ context?: any }> = ({ context }) => {
    if (!context) return null;
    const { year, artist, metadata } = context;
    return (
        <div className="pb-32 px-4 pt-12 text-xs text-text-subtle bg-surface-1/60">
                <p>{year} {artist}</p>
                {metadata?.copyright ? <p>{metadata.copyright}</p> : <p>© {year} {artist}</p>}
        </div>
    );
}

export const AlbumDetail: React.FC = () => {
    const { albumName } = useParams<{ albumName: string }>();
    const navigate = useNavigate();
    const { songs, playSong, currentSong, isPlaying, openContextMenu, fetchAlbumMetadata, albumMetadata, clearAlbumMetadata } = useStore();
    const albumCovers = useAlbumCovers();
    const [showFullDesc, setShowFullDesc] = useState(false);
    const [scrollParent, setScrollParent] = useState<HTMLElement | null>(null);
    const [isRefreshing, setIsRefreshing] = useState(false);

    useEffect(() => {
        setScrollParent(document.querySelector('main'));
    }, []);
    
    // Safely decode the URL parameter
    const decodedAlbumName = useMemo(() => {
        try {
            return decodeURIComponent(albumName || '');
        } catch (e) {
            return albumName || '';
        }
    }, [albumName]);

    // Filter and Sort songs for this album
    const albumSongs = useMemo(() => {
        return songs.filter(s => s.album === decodedAlbumName)
            .sort((a, b) => {
                if ((a.discNumber || 1) !== (b.discNumber || 1)) {
                    return (a.discNumber || 1) - (b.discNumber || 1);
                }
                return (a.trackNumber || 0) - (b.trackNumber || 0);
            });
    }, [songs, decodedAlbumName]);

    const firstSong = albumSongs[0];
    const artist = firstSong ? (firstSong.albumArtist || firstSong.artist) : '';

    // Fetch metadata on mount
    useEffect(() => {
        if (decodedAlbumName && artist) {
            fetchAlbumMetadata(decodedAlbumName, artist);
        }
    }, [decodedAlbumName, artist]);

    const year = firstSong?.year;
    
    // Merge local cover with metadata cover (prefer metadata)
    const metadataKey = `${decodedAlbumName}::${artist}`;
    const metadata = albumMetadata[metadataKey];
    
    // Handler to refresh metadata from Spotify
    const handleRefreshMetadata = async () => {
        if (!decodedAlbumName || !artist || isRefreshing) return;
        
        setIsRefreshing(true);
        try {
            // Reset the backend cache for this album
            await api.resetAlbumMetadata(metadataKey);
            // Clear from frontend store
            clearAlbumMetadata(metadataKey);
            // Re-fetch from Spotify
            await fetchAlbumMetadata(decodedAlbumName, artist);
        } catch (error) {
            console.error('Failed to refresh album metadata:', error);
        } finally {
            setIsRefreshing(false);
        }
    };
    
    const coverUrl = metadata?.coverUrl || firstSong?.coverUrl || albumCovers[decodedAlbumName];
    
    const totalDuration = albumSongs.reduce((acc, s) => acc + s.duration, 0);
    const durationHours = Math.floor(totalDuration / 3600);
    const durationMin = Math.floor((totalDuration % 3600) / 60);
    const durationSec = Math.floor(totalDuration % 60);

    // Flatten logic for virtualization (Handling Discs)
    const virtualItems = useMemo(() => {
        const items: Array<{ type: 'HEADER' | 'SONG'; data: any }> = [];
        const discs: Record<number, Song[]> = {};
        
        albumSongs.forEach(song => {
            const disc = song.discNumber || 1;
            if (!discs[disc]) discs[disc] = [];
            discs[disc].push(song);
        });

        const discNumbers = Object.keys(discs).map(Number).sort((a, b) => a - b);
        const hasMultipleDiscs = discNumbers.length > 1;

        discNumbers.forEach(discNum => {
            if (hasMultipleDiscs) {
                items.push({ type: 'HEADER', data: discNum });
            }
            discs[discNum].forEach(song => {
                items.push({ type: 'SONG', data: song });
            });
        });
        
        return items;
    }, [albumSongs]);

    if (!albumName || albumSongs.length === 0) {
        return (
            <div className="flex flex-col items-center justify-center h-full text-text-subtle">
                <Disc size={64} className="mb-4 opacity-50" />
                <h2 className="text-xl font-bold mb-2">Album not found</h2>
                <button onClick={() => navigate('/albums')} className="text-brand hover:underline">
                    Back to Albums
                </button>
            </div>
        );
    }

    const getHeaderGradient = () => {
        const gradient = generateGradient(decodedAlbumName);
        const match = gradient.match(/hsl\((\d+),\s*(\d+)%,\s*(\d+)%\)/);
        if (match) {
            const [_, h, s, l] = match;
            return `linear-gradient(to bottom, hsl(${h}, ${s}%, ${Math.max(20, parseInt(l)) / 2}%) 0%, rgb(18, 18, 18) 100%)`;
        }
        return 'linear-gradient(to bottom, rgb(64, 64, 64) 0%, rgb(18, 18, 18) 100%)';
    };
  
    const albumObject = {
        name: decodedAlbumName,
        artist: artist,
        songCount: albumSongs.length,
        coverUrl: coverUrl
    };

    const contextValue = {
        decodedAlbumName,
        getHeaderGradient,
        navigate,
        openContextMenu,
        albumObject,
        coverUrl,
        metadata,
        metadataKey,
        firstSong,
        artist,
        year,
        albumSongs,
        durationHours,
        durationMin,
        durationSec,
        playSong,
        showFullDesc,
        setShowFullDesc,
        isRefreshing,
        handleRefreshMetadata,
    };

    const components: Components<any, any> = {
        Header: AlbumHeader,
        Footer: AlbumFooter
    };

    return (
        <div className="h-full relative">
             <Virtuoso
                useWindowScroll={false}
                customScrollParent={scrollParent}
                data={virtualItems}
                context={contextValue}
                components={components}
                itemContent={(index, item) => {
                    if (item.type === 'HEADER') {
                        return (
                            <div className="bg-surface-1/60 px-8 py-2">
                                <div className="flex items-center gap-4 text-text-secondary">
                                    <Disc size={18} />
                                    <span className="font-bold text-sm">Disc {item.data}</span>
                                </div>
                            </div>
                        );
                    }

                    const song = item.data as Song;
                    const isCurrent = currentSong?.id === song.id;

                    return (
                        <div className="bg-surface-1/60 px-8">
                            <div 
                                className={`grid grid-cols-[40px_4fr_1fr_60px] gap-4 px-4 py-3 rounded-lg hover:bg-surface-hover group transition-all duration-200 cursor-pointer items-center relative ${isCurrent ? 'bg-surface-hover' : ''}`}
                                onClick={() => playSong(song, albumSongs)}
                                onContextMenu={(e) => openContextMenu(e, ContextMenuType.SONG, song)}
                            >
                                <div className="text-center text-text-secondary font-mono text-sm w-full flex justify-center items-center h-full">
                                        {isCurrent && isPlaying ? (
                                            <div className="w-3 h-3 bg-brand rounded-full animate-pulse shadow-[0_0_8px_rgb(29,185,84)]" />
                                        ) : (
                                            <>
                                            <span className={`group-hover:hidden ${isCurrent ? 'text-brand' : ''}`}>{song.trackNumber}</span>
                                            <Play size={14} className="hidden group-hover:block text-white fill-current" />
                                            </>
                                        )}
                                </div>
                                
                                <div className="flex flex-col min-w-0">
                                    <span className={`text-base font-medium truncate ${isCurrent ? 'text-brand' : 'text-white'}`}>{song.title}</span>
                                    <span className="text-sm text-text-secondary group-hover:text-white truncate transition-all duration-200">{song.artist}</span>
                                </div>

                                <div className="hidden md:block text-right pr-8 text-text-secondary text-sm font-mono opacity-0 group-hover:opacity-100 transition-all duration-200">
                                    {song.playCount || 0}
                                </div>

                                <div className="text-right pr-4 text-text-secondary text-sm font-mono group-hover:hidden">
                                    {formatTime(song.duration)}
                                </div>

                                <div className="hidden group-hover:flex items-center gap-2 justify-end pr-2 absolute right-2">
                                        <LikeButton songId={song.id} size={18} />
                                        <button 
                                        onClick={(e) => openContextMenu(e, ContextMenuType.SONG, song)}
                                        className="text-text-secondary hover:text-white transition-all duration-200"
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