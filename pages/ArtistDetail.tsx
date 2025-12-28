/**
 * ViiB MediaHub - Artist Detail Page
 * 
 * Detailed view of a single artist showing their albums in a grid.
 * 
 * Features:
 * - Artist header with image (from Spotify or gradient fallback)
 * - Album card grid showing all albums by this artist
 * - Track counts and total duration statistics
 * - Play all, shuffle all actions
 * - Click album card to navigate to album detail
 * - Smart artist name splitting for featured artists
 * 
 * Uses the same splitArtistNames logic as store.ts for consistency.
 * 
 * @module ArtistDetail
 */

import React, { useMemo, useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useStore, useAlbums } from '../store';
import { Play, ArrowLeft, User, MoreHorizontal, ExternalLink, Shuffle, Disc } from 'lucide-react';
import { generateGradient, coverBackground } from '../utils';
import { ContextMenuType } from '../types';

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

export const ArtistDetail: React.FC = () => {
    const { artistName } = useParams<{ artistName: string }>();
    const navigate = useNavigate();
    const { songs, playSong, openContextMenu, fetchArtistMetadata, artistMetadata, albumMetadata } = useStore();
    const allAlbums = useAlbums();
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
            const albumArtists = splitArtistNames(s.albumArtist || '');
            return artistsInSong.some(a => a.toLowerCase() === decodedArtistName.toLowerCase()) ||
                   albumArtists.some(a => a.toLowerCase() === decodedArtistName.toLowerCase());
        }).sort((a, b) => {
            if (a.album !== b.album) {
                return a.album.localeCompare(b.album);
            }
            return (a.trackNumber || 0) - (b.trackNumber || 0);
        });
    }, [songs, decodedArtistName]);

    // Get unique albums for this artist with full Album objects
    const artistAlbums = useMemo(() => {
        const albumNames = new Set(artistSongs.map(s => s.album));
        return allAlbums.filter(album => albumNames.has(album.name))
            .sort((a, b) => (b.addedAt || 0) - (a.addedAt || 0));
    }, [artistSongs, allAlbums]);

    // Fetch metadata on mount
    useEffect(() => {
        if (decodedArtistName) {
            fetchArtistMetadata(decodedArtistName);
        }
    }, [decodedArtistName]);

    const metadata = artistMetadata[decodedArtistName];
    const imageUrl = metadata?.imageUrl || artistSongs[0]?.coverUrl;
    const totalDuration = artistSongs.reduce((acc, s) => acc + s.duration, 0);
    const durationHours = Math.floor(totalDuration / 3600);
    const durationMin = Math.floor((totalDuration % 3600) / 60);

    if (!artistName || artistSongs.length === 0) {
        return (
            <div className="flex flex-col items-center justify-center h-full text-text-subtle">
                <User size={64} className="mb-4 opacity-50" />
                <h2 className="text-xl font-bold mb-2">Artist not found</h2>
                <button onClick={() => navigate('/artists')} className="text-brand hover:underline">
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
            return `linear-gradient(to bottom, hsl(${h}, ${s}%, ${Math.max(20, parseInt(l)) / 2}%) 0%, rgb(18, 18, 18) 100%)`;
        }
        return 'linear-gradient(to bottom, rgb(64, 64, 64) 0%, rgb(18, 18, 18) 100%)';
    };
  
    const artistObject = {
        name: decodedArtistName,
        songCount: artistSongs.length,
        albumCount: artistAlbums.length,
        imageUrl: imageUrl
    };

    const handlePlayAll = () => {
        if (artistSongs.length > 0) {
            playSong(artistSongs[0], artistSongs);
        }
    };

    const handleShuffle = () => {
        if (artistSongs.length > 0) {
            const shuffled = [...artistSongs].sort(() => Math.random() - 0.5);
            playSong(shuffled[0], shuffled);
        }
    };

    return (
        <div className="h-full relative animate-fade-in overflow-y-auto" id="artist-detail-scroll">
            {/* Dynamic Background Header */}
            <div 
                className="absolute top-0 left-0 w-full h-[400px] z-0 opacity-40 pointer-events-none"
                style={{ background: getHeaderGradient() }}
            />

            {/* Header Content */}
            <div className="relative z-10 p-8 pt-16 flex flex-col md:flex-row gap-8 items-end">
                <button 
                    onClick={() => navigate(-1)} 
                    className="absolute top-6 left-6 w-8 h-8 bg-black/40 rounded-full flex items-center justify-center hover:bg-black/60 text-white transition-all duration-200 z-20"
                    aria-label="Go back"
                >
                    <ArrowLeft size={20} />
                </button>

                {/* Artist Image */}
                <div 
                    className="w-48 h-48 md:w-52 md:h-52 shadow-2xl flex-shrink-0 relative group cursor-pointer rounded-full overflow-hidden"
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
                        <span>{artistAlbums.length} albums</span>
                        <span className="w-1 h-1 bg-white rounded-full mx-1"></span>
                        <span className="text-text-subtle">{durationHours > 0 ? `${durationHours} hr ${durationMin} min` : `${durationMin} min`}</span>
                    </div>
                </div>
            </div>

            {/* Actions */}
            <div className="relative z-10 px-8 py-6 flex items-center gap-6">
                <button 
                    onClick={handlePlayAll}
                    className="w-14 h-14 bg-brand hover:bg-brand-hover rounded-full flex items-center justify-center hover:scale-105 transition-all duration-200 shadow-lg shadow-black/40 text-black"
                    aria-label="Play all"
                >
                    <Play size={28} className="fill-current ml-1" />
                </button>
                <button 
                    onClick={handleShuffle}
                    className="text-text-secondary hover:text-white transition-all duration-200"
                    title="Shuffle play"
                >
                    <Shuffle size={28} />
                </button>
                <div className="relative">
                    <button 
                        onClick={(e) => openContextMenu(e, ContextMenuType.ARTIST, artistObject)}
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
            </div>

            {/* Albums Section Header */}
            <div className="px-8 pt-4 pb-6 relative z-10">
                <h2 className="text-xl font-bold text-text-main flex items-center gap-2">
                    <Disc size={20} className="text-text-secondary" />
                    Discography
                </h2>
            </div>

            {/* Album Grid */}
            {artistAlbums.length === 0 ? (
                <div className="flex flex-col items-center justify-center p-12 text-center relative z-10">
                    <Disc size={64} className="text-text-subtle mb-4" />
                    <h3 className="text-lg font-bold text-text-main mb-2">No albums found</h3>
                    <p className="text-text-secondary">This artist doesn't have any albums in your library.</p>
                </div>
            ) : (
                <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-6 px-8 pb-32 relative z-10">
                    {artistAlbums.map((album) => {
                        const metadataKey = `${album.name}::${album.artist}`;
                        const albumMeta = albumMetadata[metadataKey];
                        const coverUrl = albumMeta?.coverUrl || album.coverUrl;

                        return (
                            <div 
                                key={album.name}
                                className="bg-surface-2 p-4 rounded-lg hover:bg-surface-3 transition-all group cursor-pointer border border-transparent hover:border-surface-border flex flex-col"
                                onClick={() => navigate(`/album/${encodeURIComponent(album.name)}`)}
                                onContextMenu={(e) => openContextMenu(e, ContextMenuType.ALBUM, album)}
                            >
                                <div 
                                    className="w-full aspect-square rounded-md mb-4 shadow-lg flex items-center justify-center text-5xl font-bold text-white/20 relative overflow-hidden bg-surface-3"
                                    style={{ background: coverBackground(coverUrl, album.name) }}
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
                                    <p className="text-sm text-text-secondary">{album.songCount} songs</p>
                                    {albumMeta?.releaseDate && (
                                        <span className="text-[10px] text-text-subtle font-mono">{new Date(albumMeta.releaseDate).getFullYear()}</span>
                                    )}
                                </div>
                            </div>
                        );
                    })}
                </div>
            )}
        </div>
    );
};
