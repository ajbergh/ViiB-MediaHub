/**
 * ViiB MediaHub - Smart Mix Detail Page
 * 
 * Detailed view of an auto-generated smart mix playlist.
 * 
 * Features:
 * - Mix header with gradient background matching mix colors
 * - Track listing from the generated mix
 * - Play all, shuffle, add to queue actions
 * - Save as regular playlist functionality
 * - Mix description and rule information
 * 
 * Smart mixes are regenerated when library changes.
 * 
 * @module SmartMixDetail
 */

import React, { useMemo } from 'react';
import { useParams, useNavigate } from 'react-router';
import { useStore, useAlbumCovers } from '../store';
import { Play, Clock, ArrowLeft, MoreHorizontal, Sparkles, Shuffle, Save } from 'lucide-react';
import { formatTime, generateGradient } from '../utils';
import { ContextMenuType } from '../types';

export const SmartMixDetail: React.FC = () => {
    const { mixId } = useParams<{ mixId: string }>();
    const navigate = useNavigate();
    const { smartMixes, songs, playSong, currentSong, isPlaying, openContextMenu, saveSmartMixAsPlaylist, addToQueue } = useStore();
    const albumCovers = useAlbumCovers();

    const mix = smartMixes.find(m => m.id === mixId);

    // Filter and Sort songs for this mix
    const mixSongs = useMemo(() => {
        if (!mix) return [];
        return mix.songIds
            .map(id => songs.find(s => s.id === id))
            .filter((s): s is typeof s & {} => !!s);
    }, [mix, songs]);

    if (!mix) {
        return (
            <div className="flex flex-col items-center justify-center h-full text-text-subtle">
                <Sparkles size={64} className="mb-4 opacity-50" />
                <h2 className="text-xl font-bold mb-2">Mix not found</h2>
                <button onClick={() => navigate('/')} className="text-brand hover:underline">
                    Back to Home
                </button>
            </div>
        );
    }
    
    const totalDuration = mixSongs.reduce((acc, s) => acc + s.duration, 0);
    const durationMin = Math.floor(totalDuration / 60);
    const durationSec = Math.floor(totalDuration % 60);

    const handleSaveAsPlaylist = () => {
        const name = prompt("Save Smart Mix as Playlist:", mix.name);
        if (name) {
            // We use a custom action in store or simulating adding playlists
            // Since useStore doesn't expose a direct 'createFromIds', we use the specific action we added
            saveSmartMixAsPlaylist(mix.id);
            navigate('/playlists');
        }
    };

    return (
        <div className="flex flex-col min-h-full pb-32 relative">
             {/* Dynamic Background Header */}
             <div 
                className="absolute top-0 left-0 w-full h-[500px] z-0 opacity-40 pointer-events-none"
                style={{ background: `linear-gradient(to bottom, ${mix.coverColors[0]} 0%, rgb(18, 18, 18) 100%)` }}
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

                {/* Cover Collage */}
                <div 
                    className="w-52 h-52 shadow-2xl flex-shrink-0 relative group rounded-lg overflow-hidden grid grid-cols-2"
                    onContextMenu={(e) => openContextMenu(e, ContextMenuType.SMART_MIX, mix)}
                >
                    {mixSongs.slice(0, 4).map((s, i) => (
                        <div key={i} className="w-full h-full bg-surface-3">
                            <img 
                                src={s.coverUrl || albumCovers[s.album]} 
                                className="w-full h-full object-cover" 
                                alt=""
                                onError={(e) => { e.currentTarget.style.display = 'none'; e.currentTarget.parentElement!.style.backgroundColor = mix.coverColors[i % 2]; }}
                            />
                        </div>
                    ))}
                    {mixSongs.length < 4 && (
                        <div className="absolute inset-0 flex items-center justify-center bg-gradient-to-br from-white/10 to-transparent">
                            <Sparkles size={48} className="text-white drop-shadow-lg" />
                        </div>
                    )}
                </div>

                {/* Mix Metadata */}
                <div className="flex flex-col gap-2 z-10 w-full">
                    <span className="text-xs font-bold uppercase tracking-wider text-white flex items-center gap-2">
                        <Sparkles size={14} className="text-accent-orange" />
                        Smart Mix
                    </span>
                    <h1 className="text-display font-bold text-white tracking-tight leading-tight drop-shadow-lg">
                        {mix.name}
                    </h1>
                    <p className="text-white/80 text-lg font-medium">{mix.description}</p>
                    <div className="flex items-center flex-wrap gap-2 text-sm text-white/70 font-medium mt-2">
                         <span>Updated just now</span>
                         <span className="w-1 h-1 bg-white rounded-full mx-1"></span>
                         <span>{mixSongs.length} songs, {durationMin} hr {durationSec} min</span>
                    </div>
                </div>
             </div>

             {/* Action Bar & Tracklist */}
             <div className="bg-surface-1/60 flex-1 p-8 pt-6 backdrop-blur-lg relative z-10 min-h-[500px]">
                {/* Actions */}
                <div className="flex items-center gap-6 mb-8 relative">
                    <button 
                        onClick={() => mixSongs.length > 0 && playSong(mixSongs[0], mixSongs)}
                        className="w-14 h-14 bg-brand hover:bg-brand-hover rounded-full flex items-center justify-center hover:scale-105 transition-all duration-200 shadow-lg shadow-black/40 text-black"
                        aria-label="Play all"
                    >
                        <Play size={28} className="fill-current ml-1" />
                    </button>
                    
                    <button 
                        onClick={() => {
                            // Shuffle play
                            const shuffled = [...mixSongs].sort(() => 0.5 - Math.random());
                            if (shuffled.length > 0) playSong(shuffled[0], shuffled);
                        }}
                        className="text-text-secondary hover:text-white transition-all duration-200 flex items-center gap-2 font-bold text-sm bg-white/10 px-4 py-2 rounded-full hover:bg-white/20"
                    >
                        <Shuffle size={18} /> Shuffle
                    </button>

                     <button 
                        onClick={handleSaveAsPlaylist}
                        className="text-text-secondary hover:text-white transition-all duration-200 flex items-center gap-2 font-bold text-sm bg-white/10 px-4 py-2 rounded-full hover:bg-white/20"
                    >
                        <Save size={18} /> Save as Playlist
                    </button>

                    <div className="relative ml-auto">
                        <button 
                            onClick={(e) => openContextMenu(e, ContextMenuType.SMART_MIX, mix)}
                            className="text-text-secondary hover:text-white transition-all duration-200"
                            aria-label="More options"
                        >
                            <MoreHorizontal size={32} />
                        </button>
                    </div>
                </div>

                {/* Table Header */}
                <div className="grid grid-cols-[40px_4fr_3fr_60px] gap-4 px-4 py-2 border-b border-surface-3 text-text-secondary text-xs uppercase tracking-wider font-medium mb-2 sticky top-0 bg-surface-1 z-20">
                    <div className="text-center">#</div>
                    <div>Title</div>
                    <div className="hidden md:block">Album</div>
                    <div className="text-right pr-4"><Clock size={16} className="inline" /></div>
                </div>

                {/* Tracks List */}
                <div className="space-y-0.5">
                    {mixSongs.map((song, idx) => {
                        const isCurrent = currentSong?.id === song.id;

                        return (
                            <div 
                                key={song.id}
                                className={`grid grid-cols-[40px_4fr_3fr_60px] gap-4 px-4 py-3 rounded-lg hover:bg-surface-hover group transition-all duration-200 cursor-pointer items-center relative ${isCurrent ? 'bg-surface-hover' : ''}`}
                                onClick={() => playSong(song, mixSongs)}
                                onContextMenu={(e) => openContextMenu(e, ContextMenuType.SONG, song)}
                            >
                                <div className="text-center text-text-secondary font-mono text-sm w-full flex justify-center items-center h-full">
                                        {isCurrent && isPlaying ? (
                                            <div className="w-3 h-3 bg-brand rounded-full animate-pulse shadow-[0_0_8px_rgb(29,185,84)]" />
                                        ) : (
                                            <>
                                            <span className={`group-hover:hidden ${isCurrent ? 'text-brand' : ''}`}>{idx + 1}</span>
                                            <Play size={14} className="hidden group-hover:block text-white fill-current" />
                                            </>
                                        )}
                                </div>
                                
                                <div className="flex flex-col min-w-0">
                                    <span className={`text-base font-medium truncate ${isCurrent ? 'text-brand' : 'text-white'}`}>{song.title}</span>
                                    <span className="text-sm text-text-secondary group-hover:text-white truncate transition-all duration-200">{song.artist}</span>
                                </div>

                                <div className="hidden md:block text-text-secondary text-sm truncate">
                                    {song.album}
                                </div>

                                {/* Duration / Menu */}
                                <div className="text-right pr-4 text-text-secondary text-sm font-mono group-hover:hidden">
                                    {formatTime(song.duration)}
                                </div>

                                <div className="hidden group-hover:flex justify-end pr-2 absolute right-2">
                                        <button 
                                        onClick={(e) => openContextMenu(e, ContextMenuType.SONG, song)}
                                        className="text-text-secondary hover:text-white transition-all duration-200"
                                        aria-label="More options"
                                    >
                                        <MoreHorizontal size={20} />
                                    </button>
                                </div>
                            </div>
                        )
                    })}
                </div>
                 
                 {mixSongs.length === 0 && (
                     <div className="py-20 text-center text-text-subtle">
                         <p>Not enough music to generate this mix yet.</p>
                         <p className="text-sm">Try adding more songs or listening to music!</p>
                     </div>
                 )}
             </div>
        </div>
    );
};