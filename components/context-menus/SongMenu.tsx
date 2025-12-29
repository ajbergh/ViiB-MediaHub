/**
 * Song Context Menu
 *
 * Provides contextual actions for songs including Play, Play Next, Add to Queue, Add to Playlist,
 * navigation to album/artist, and download options for streaming Spotify tracks.
 */
import React, { useState } from 'react';
import { Play, SkipForward, ListPlus, ListMusic, ArrowRight, Disc, Mic2, FolderOpen, Download, CheckCircle } from 'lucide-react';
import { useStore } from '../../store';
import { MenuItem, PlaylistsSubmenu } from './MenuShared';
import { useNavigate } from 'react-router-dom';
import { Song } from '../../types';
import { api } from '../../services/api';

/**
 * SongMenu props:
 *  - song: Song object for which to show the menu
 *  - onClose: Callback invoked when the menu closes
 */
export const SongMenu: React.FC<{ song: Song; onClose: () => void }> = ({ song, onClose }) => {
    const { playSong, playNext, addToQueue, showToast } = useStore();
    const navigate = useNavigate();
    const [playlistsSubmenuOpen, setPlaylistsSubmenuOpen] = useState(false);
    const playlistTriggerRef = React.useRef<HTMLButtonElement | null>(null);

    const handleAction = (action: () => void) => {
        action();
        onClose();
    };

    const navigateTo = (path: string) => {
        navigate(path);
        onClose();
    };
    
    const handleDownloadTrack = async () => {
        if (!song.spotifyId) return;
        
        try {
            await api.downloadTrack(
                song.spotifyId,
                song.title,
                song.artist,
                song.album,
                song.duration
            );
            showToast({ type: 'success', message: `Queued for download: ${song.title}` });
        } catch (error) {
            console.error('Failed to queue download:', error);
            showToast({ type: 'error', message: 'Failed to queue download' });
        }
        onClose();
    };
    
    // Check if this is a streaming Spotify track
    const isSpotifyStreaming = song.spotifyId && song.isStreaming;
    const isSpotifyDownloaded = song.spotifyId && !song.isStreaming;

    return (
        <>
            <div className="px-3 py-2 border-b border-surface-border mb-1">
                <div className="font-bold text-text-main truncate text-sm">{song.title}</div>
                <div className="text-xs text-text-secondary truncate">{song.artist}</div>
            </div>

            <MenuItem icon={Play} label="Play Now" onClick={() => handleAction(() => playSong(song))} />
            <MenuItem icon={SkipForward} label="Play Next" onClick={() => handleAction(() => playNext(song))} />
            <MenuItem icon={ListPlus} label="Add to Queue" onClick={() => handleAction(() => addToQueue(song))} />
            
            <div 
                className="relative"
                onMouseEnter={() => setPlaylistsSubmenuOpen(true)}
                onMouseLeave={() => setPlaylistsSubmenuOpen(false)}
                onFocus={() => setPlaylistsSubmenuOpen(true)}
                onBlurCapture={(e) => {
                    const next = e.relatedTarget as Node | null;
                    if (!next || !e.currentTarget.contains(next)) {
                        setPlaylistsSubmenuOpen(false);
                    }
                }}
            >
                <button
                    ref={playlistTriggerRef}
                    role="menuitem"
                    tabIndex={-1}
                    data-viib-label="Add to Playlist"
                    aria-label="Add to Playlist"
                    aria-haspopup="menu"
                    aria-expanded={playlistsSubmenuOpen}
                    className="group w-full text-left px-4 py-2 text-sm text-text-main hover:bg-surface-1/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-0 flex items-center justify-between transition-colors duration-150 motion-reduce:transition-none"
                    onClick={(e) => {
                        e.stopPropagation();
                        setPlaylistsSubmenuOpen((v) => !v);
                        requestAnimationFrame(() => {
                            const root = (e.currentTarget.parentElement as HTMLElement | null) ?? null;
                            const submenu = root?.querySelector('[data-viib-submenu="playlists"]') as HTMLElement | null;
                            const firstItem = submenu?.querySelector('[role="menuitem"]') as HTMLElement | null;
                            firstItem?.focus();
                        });
                    }}
                >
                    <div className="flex items-center gap-3">
                        <ListMusic size={16} />
                        <span>Add to Playlist</span>
                    </div>
                    <ArrowRight size={14} className="text-text-subtle group-hover:text-text-main" />
                </button>
                {playlistsSubmenuOpen && (
                    <PlaylistsSubmenu
                        songId={song.id}
                        onClose={onClose}
                        onBack={() => {
                            setPlaylistsSubmenuOpen(false);
                            requestAnimationFrame(() => playlistTriggerRef.current?.focus());
                        }}
                    />
                )}
            </div>

            <div className="border-t border-surface-border my-1"></div>

            <MenuItem icon={Disc} label="Go to Album" onClick={() => navigateTo(`/album/${encodeURIComponent(song.album)}`)} />
            <MenuItem icon={Mic2} label="Go to Artist" onClick={() => navigateTo(`/artists`)} /> 
            
            <div className="border-t border-surface-border my-1"></div>
            
            {/* Download option for streaming Spotify tracks */}
            {isSpotifyStreaming && (
                <MenuItem icon={Download} label="Download for Offline" onClick={handleDownloadTrack} />
            )}
            {/* Show downloaded indicator for Spotify tracks */}
            {isSpotifyDownloaded && (
                <div className="px-4 py-2 text-sm text-text-secondary flex items-center gap-3">
                    <CheckCircle size={16} className="text-brand" />
                    <span>Downloaded</span>
                </div>
            )}
            
            <MenuItem icon={FolderOpen} label="Show in Files" onClick={() => alert(`Path: ${song.path || 'Unknown'}`)} />
        </>
    );
};
