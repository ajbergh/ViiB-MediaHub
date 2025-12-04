import React, { useState } from 'react';
import { Play, SkipForward, ListPlus, ListMusic, ArrowRight, Disc, Mic2, FolderOpen, Download, CheckCircle } from 'lucide-react';
import { useStore } from '../../store';
import { MenuItem, PlaylistsSubmenu } from './MenuShared';
import { useNavigate } from 'react-router-dom';
import { Song } from '../../types';
import { api } from '../../services/api';

export const SongMenu: React.FC<{ song: Song; onClose: () => void }> = ({ song, onClose }) => {
    const { playSong, playNext, addToQueue, showToast } = useStore();
    const navigate = useNavigate();
    const [playlistsSubmenuOpen, setPlaylistsSubmenuOpen] = useState(false);

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
            <div className="px-3 py-2 border-b border-[#333] mb-1">
                <div className="font-bold text-white truncate text-sm">{song.title}</div>
                <div className="text-xs text-gray-400 truncate">{song.artist}</div>
            </div>

            <MenuItem icon={Play} label="Play Now" onClick={() => handleAction(() => playSong(song))} />
            <MenuItem icon={SkipForward} label="Play Next" onClick={() => handleAction(() => playNext(song))} />
            <MenuItem icon={ListPlus} label="Add to Queue" onClick={() => handleAction(() => addToQueue(song))} />
            
            <div 
                className="relative group"
                onMouseEnter={() => setPlaylistsSubmenuOpen(true)}
                onMouseLeave={() => setPlaylistsSubmenuOpen(false)}
            >
                <button className="w-full text-left px-4 py-2 text-sm hover:bg-[#333] hover:text-white flex items-center justify-between transition-colors">
                    <div className="flex items-center gap-3">
                        <ListMusic size={16} />
                        <span>Add to Playlist</span>
                    </div>
                    <ArrowRight size={14} />
                </button>
                {playlistsSubmenuOpen && <PlaylistsSubmenu songId={song.id} onClose={onClose} />}
            </div>

            <div className="border-t border-[#333] my-1"></div>

            <MenuItem icon={Disc} label="Go to Album" onClick={() => navigateTo(`/album/${encodeURIComponent(song.album)}`)} />
            <MenuItem icon={Mic2} label="Go to Artist" onClick={() => navigateTo(`/artists`)} /> 
            
            <div className="border-t border-[#333] my-1"></div>
            
            {/* Download option for streaming Spotify tracks */}
            {isSpotifyStreaming && (
                <MenuItem icon={Download} label="Download for Offline" onClick={handleDownloadTrack} />
            )}
            {/* Show downloaded indicator for Spotify tracks */}
            {isSpotifyDownloaded && (
                <div className="px-4 py-2 text-sm text-gray-400 flex items-center gap-3">
                    <CheckCircle size={16} className="text-brand" />
                    <span>Downloaded</span>
                </div>
            )}
            
            <MenuItem icon={FolderOpen} label="Show in Files" onClick={() => alert(`Path: ${song.path || 'Unknown'}`)} />
        </>
    );
};
