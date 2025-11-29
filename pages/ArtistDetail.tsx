/**
 * ViiB MediaHub - Artist Detail Page
 * 
 * Detailed view of a single artist with discography and stats.
 * 
 * Features:
 * - Artist header with image (from Spotify or gradient fallback)
 * - Complete discography organized by album
 * - Track counts and total duration statistics
 * - Play all, shuffle all actions
 * - Album grouping with collapsible track lists
 * - Smart artist name splitting for featured artists
 * 
 * Uses the same splitArtistNames logic as store.ts for consistency.
 * 
 * @module ArtistDetail
 */

import React, { useMemo, useEffect, useState, forwardRef } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useStore } from '../store';
import { Play, Clock, ArrowLeft, User, MoreHorizontal, ExternalLink } from 'lucide-react';
import { formatTime, generateGradient } from '../utils';
import { ContextMenuType, Song } from '../types';
import { Virtuoso, Components } from 'react-virtuoso';

/**
 * Splits an artist string into individual artists.
 * Must match the logic in store.ts useArtists
 */
const splitArtistNames = (artistString: string): string[] => {
  if (!artistString) return [];
  
  const separators = [
    ' feat. ', ' feat ', ' ft. ', ' ft ', 
    ' featuring ', ' & ', ' x ', ' and ', 
    ', ', ' / ', ' vs. ', ' vs '
  ];
  
  let artists = [artistString];
  
  for (const sep of separators) {
    const newArtists: string[] = [];
    for (const artist of artists) {
      const parts = artist.split(new RegExp(sep.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'i'));
      newArtists.push(...parts);
    }
    artists = newArtists;
  }
  
  return artists.map(a => a.trim()).filter(a => a.length > 0);
};

// Header Component
const ArtistHeader: React.FC<{ context?: any }> = ({ context }) => {
    if (!context) return null;

    const {
        decodedArtistName,
        getHeaderGradient,
        navigate,
        openContextMenu,
        artistObject,
        imageUrl,
        metadata,
        artistSongs,
        uniqueAlbums,
        totalDuration,
        playSong,
    } = context;

    const durationMin = Math.floor(totalDuration / 3600);
    const durationSec = Math.floor((totalDuration % 3600) / 60);

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

                {/* Artist Image */}
                <div 
                    className="w-52 h-52 shadow-2xl flex-shrink-0 relative group cursor-pointer rounded-full overflow-hidden"
                    onContextMenu={(e) => openContextMenu(e, ContextMenuType.ARTIST, artistObject)}
                >
                    {imageUrl ? (
                        <img src={imageUrl} alt={decodedArtistName} className="w-full h-full object-cover" />
                    ) : (
                        <div 
                            className="w-full h-full flex items-center justify-center text-white/30 text-6xl font-bold"
                            style={{ background: generateGradient(decodedArtistName) }}
                        >
                            {decodedArtistName.charAt(0)}
                        </div>
                    )}
                </div>

                {/* Artist Metadata */}
                <div className="flex flex-col gap-2 z-10 w-full min-w-0">
                    <div className="flex items-center gap-2">
                        <span className="text-xs font-bold uppercase tracking-wider text-white">Artist</span>
                        {metadata && <span className="text-[10px] bg-white/20 px-2 py-0.5 rounded text-white font-medium">Enhanced</span>}
                    </div>
                    
                    <h1 
                        className="text-4xl md:text-6xl lg:text-7xl font-bold text-white tracking-tight leading-tight line-clamp-2 drop-shadow-lg"
                        onContextMenu={(e) => openContextMenu(e, ContextMenuType.ARTIST, artistObject)}
                    >
                        {decodedArtistName}
                    </h1>
                    <div className="flex items-center flex-wrap gap-2 text-sm text-white font-medium mt-4 shadow-black drop-shadow-md">
                        <span>{artistSongs.length} songs</span>
                        <span className="w-1 h-1 bg-white rounded-full mx-1"></span>
                        <span>{uniqueAlbums.length} albums</span>
                        <span className="w-1 h-1 bg-white rounded-full mx-1"></span>
                        <span className="text-gray-300">{durationMin > 0 ? `${durationMin} hr ${durationSec} min` : `${durationSec} min`}</span>
                    </div>
                </div>
            </div>

            {/* Actions Container */}
            <div className="bg-[#121212]/60 p-8 pt-6 backdrop-blur-lg relative z-10">
                <div className="flex items-center gap-6 mb-8 relative">
                    <button 
                        onClick={() => playSong(artistSongs[0], artistSongs)}
                        className="w-14 h-14 bg-[#1db954] hover:bg-[#1ed760] rounded-full flex items-center justify-center hover:scale-105 transition-transform shadow-lg shadow-black/40 text-black"
                    >
                        <Play size={28} className="fill-current ml-1" />
                    </button>
                    <div className="relative">
                        <button 
                            onClick={(e) => openContextMenu(e, ContextMenuType.ARTIST, artistObject)}
                            className="text-[#b3b8c1] hover:text-white transition-colors"
                        >
                            <MoreHorizontal size={32} />
                        </button>
                    </div>
                    {metadata?.url && (
                        <a 
                            href={metadata.url} 
                            target="_blank" 
                            rel="noopener noreferrer"
                            className="ml-auto flex items-center gap-2 text-xs font-bold text-[#b3b8c1] hover:text-white bg-white/5 hover:bg-white/10 px-4 py-2 rounded-full transition-colors"
                        >
                            <ExternalLink size={14} /> Spotify
                        </a>
                    )}
                </div>

                {/* Table Header */}
                <div className="grid grid-cols-[40px_4fr_2fr_60px] gap-4 px-4 py-2 border-b border-[#282828] text-[#b3b8c1] text-xs uppercase tracking-wider font-medium mb-2 sticky top-0 bg-[#121212] z-20">
                    <div className="text-center">#</div>
                    <div>Title</div>
                    <div className="hidden md:block">Album</div>
                    <div className="text-right pr-4"><Clock size={16} className="inline" /></div>
                </div>
            </div>
        </>
    );
};

const ArtistFooter: React.FC<{ context?: any }> = ({ context }) => {
    if (!context) return null;
    const { artistSongs } = context;
    const totalDuration = artistSongs.reduce((acc: number, s: Song) => acc + s.duration, 0);
    const hours = Math.floor(totalDuration / 3600);
    const minutes = Math.floor((totalDuration % 3600) / 60);
    
    return (
        <div className="pb-32 px-4 pt-12 text-xs text-[#6f7480] bg-[#121212]/60">
            <p>{artistSongs.length} songs, {hours > 0 ? `${hours} hr ` : ''}{minutes} min</p>
        </div>
    );
};

export const ArtistDetail: React.FC = () => {
    const { artistName } = useParams<{ artistName: string }>();
    const navigate = useNavigate();
    const { songs, playSong, currentSong, isPlaying, openContextMenu, fetchArtistMetadata, artistMetadata } = useStore();
    const [scrollParent, setScrollParent] = useState<HTMLElement | null>(null);

    useEffect(() => {
        setScrollParent(document.querySelector('main'));
    }, []);
    
    // Safely decode the URL parameter
    const decodedArtistName = useMemo(() => {
        try {
            return decodeURIComponent(artistName || '');
        } catch (e) {
            return artistName || '';
        }
    }, [artistName]);

    // Filter songs for this artist (including featured appearances)
    const artistSongs = useMemo(() => {
        return songs.filter(s => {
            const artistsInSong = splitArtistNames(s.artist);
            return artistsInSong.some(a => a.toLowerCase() === decodedArtistName.toLowerCase());
        }).sort((a, b) => {
            // Sort by album, then by track number
            if (a.album !== b.album) {
                return a.album.localeCompare(b.album);
            }
            return (a.trackNumber || 0) - (b.trackNumber || 0);
        });
    }, [songs, decodedArtistName]);

    // Get unique albums
    const uniqueAlbums = useMemo(() => {
        const albums = new Set(artistSongs.map(s => s.album));
        return Array.from(albums);
    }, [artistSongs]);

    // Fetch metadata on mount
    useEffect(() => {
        if (decodedArtistName) {
            fetchArtistMetadata(decodedArtistName);
        }
    }, [decodedArtistName]);

    const metadata = artistMetadata[decodedArtistName];
    const imageUrl = metadata?.imageUrl || artistSongs[0]?.coverUrl;
    const totalDuration = artistSongs.reduce((acc, s) => acc + s.duration, 0);

    if (!artistName || artistSongs.length === 0) {
        return (
            <div className="flex flex-col items-center justify-center h-full text-[#6f7480]">
                <User size={64} className="mb-4 opacity-50" />
                <h2 className="text-xl font-bold mb-2">Artist not found</h2>
                <button onClick={() => navigate('/artists')} className="text-green-500 hover:underline">
                    Back to Artists
                </button>
            </div>
        );
    }

    const getHeaderGradient = () => {
        const gradient = generateGradient(decodedArtistName);
        const match = gradient.match(/hsl\((\d+),\s*(\d+)%,\s*(\d+)%\)/);
        if (match) {
            const [_, h, s, l] = match;
            return `linear-gradient(to bottom, hsl(${h}, ${s}%, ${Math.max(20, parseInt(l)) / 2}%) 0%, #121212 100%)`;
        }
        return 'linear-gradient(to bottom, #404040 0%, #121212 100%)';
    };
  
    const artistObject = {
        name: decodedArtistName,
        songCount: artistSongs.length,
        albumCount: uniqueAlbums.length,
        imageUrl: imageUrl
    };

    const contextValue = {
        decodedArtistName,
        getHeaderGradient,
        navigate,
        openContextMenu,
        artistObject,
        imageUrl,
        metadata,
        artistSongs,
        uniqueAlbums,
        totalDuration,
        playSong,
    };

    const components: Components<any, any> = {
        Header: ArtistHeader,
        Footer: ArtistFooter
    };

    return (
        <div className="h-full relative">
             <Virtuoso
                useWindowScroll={false}
                customScrollParent={scrollParent}
                data={artistSongs}
                context={contextValue}
                components={components}
                itemContent={(index, song) => {
                    const isCurrent = currentSong?.id === song.id;

                    return (
                        <div className="bg-[#121212]/60 px-8">
                            <div 
                                className={`grid grid-cols-[40px_4fr_2fr_60px] gap-4 px-4 py-3 rounded-md hover:bg-[#2a2a2a] group transition-colors cursor-pointer items-center relative ${isCurrent ? 'bg-[#2a2a2a]' : ''}`}
                                onClick={() => playSong(song, artistSongs)}
                                onContextMenu={(e) => openContextMenu(e, ContextMenuType.SONG, song)}
                            >
                                <div className="text-center text-[#b3b8c1] font-mono text-sm w-full flex justify-center items-center h-full">
                                        {isCurrent && isPlaying ? (
                                            <div className="w-3 h-3 bg-[#1db954] rounded-full animate-pulse shadow-[0_0_8px_#1db954]" />
                                        ) : (
                                            <>
                                            <span className={`group-hover:hidden ${isCurrent ? 'text-[#1db954]' : ''}`}>{index + 1}</span>
                                            <Play size={14} className="hidden group-hover:block text-white fill-current" />
                                            </>
                                        )}
                                </div>
                                
                                <div className="flex items-center gap-3 min-w-0">
                                    {song.coverUrl && (
                                        <img src={song.coverUrl} alt="" className="w-10 h-10 rounded flex-shrink-0" />
                                    )}
                                    <div className="flex flex-col min-w-0">
                                        <span className={`text-base font-medium truncate ${isCurrent ? 'text-[#1db954]' : 'text-white'}`}>{song.title}</span>
                                        <span className="text-sm text-[#b3b8c1] group-hover:text-white truncate transition-colors">{song.artist}</span>
                                    </div>
                                </div>

                                <div 
                                    className="hidden md:block text-[#b3b8c1] text-sm truncate hover:underline cursor-pointer"
                                    onClick={(e) => {
                                        e.stopPropagation();
                                        navigate(`/album/${encodeURIComponent(song.album)}`);
                                    }}
                                >
                                    {song.album}
                                </div>

                                <div className="text-right pr-4 text-[#b3b8c1] text-sm font-mono group-hover:hidden">
                                    {formatTime(song.duration)}
                                </div>

                                <div className="hidden group-hover:flex justify-end pr-2 absolute right-2">
                                        <button 
                                        onClick={(e) => { e.stopPropagation(); openContextMenu(e, ContextMenuType.SONG, song); }}
                                        className={`text-[#b3b8c1] hover:text-white transition-colors`}
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
