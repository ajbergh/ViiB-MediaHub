import React, { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { ArrowLeft, Play, MoreHorizontal, Loader2, Clock, ExternalLink, Download } from 'lucide-react';
import { SpotifyService } from '../services/spotifyService';
import { useStore } from '../store';
import { formatTime } from '../utils';
import { SpotifyAuthError, SpotifyRateLimitError, SpotifyApiError } from '../lib/spotifyErrors';
import api from '../services/api';

interface SpotifyAlbumFull {
  id: string;
  name: string;
  artists: { name: string; id: string }[];
  images: { url: string; height: number; width: number }[];
  release_date: string;
  total_tracks: number;
  label: string;
  copyrights: { text: string; type: string }[];
  genres: string[];
  popularity: number;
  external_urls: { spotify: string };
  tracks: {
    items: {
      id: string;
      name: string;
      track_number: number;
      duration_ms: number;
      explicit: boolean;
      preview_url: string | null;
      artists: { name: string; id: string }[];
    }[];
  };
}

export const SpotifyAlbumDetail: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { addLog } = useStore();
  
  const [album, setAlbum] = useState<SpotifyAlbumFull | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isDownloading, setIsDownloading] = useState(false);

  const handleDownload = async () => {
    if (!album) return;
    
    setIsDownloading(true);
    try {
      await api.downloadAlbum(album.id, album.name, album.artists[0]?.name || 'Unknown Artist');
      addLog('success', `Started download for album: ${album.name}`);
    } catch (err) {
      console.error('Download failed:', err);
      addLog('error', 'Failed to start album download');
    } finally {
      setIsDownloading(false);
    }
  };

  useEffect(() => {
    const fetchAlbum = async () => {
      if (!id) return;
      
      setIsLoading(true);
      setError(null);
      
      try {
        const token = await SpotifyService.getAccessToken();
        if (!token) {
          throw new SpotifyAuthError('No access token available');
        }

        const response = await fetch(`https://api.spotify.com/v1/albums/${id}`, {
          headers: { 'Authorization': `Bearer ${token}` }
        });

        if (response.status === 429) {
          const retryAfter = response.headers.get('Retry-After');
          throw new SpotifyRateLimitError(
            'Rate limited while fetching album',
            retryAfter ? parseInt(retryAfter) : 60
          );
        }

        if (!response.ok) {
          throw new SpotifyApiError(
            'Failed to fetch album details',
            response.status
          );
        }

        const data = await response.json();
        setAlbum(data);
      } catch (err) {
        if (err instanceof SpotifyRateLimitError) {
          setError(`Rate limited. Please try again in ${err.retryAfter} seconds.`);
          addLog('warn', `Rate limited while fetching album`);
        } else if (err instanceof SpotifyAuthError) {
          setError('Authentication failed. Please reconnect to Spotify.');
          addLog('error', 'Spotify auth error fetching album');
        } else if (err instanceof SpotifyApiError) {
          setError('Failed to load album. Please try again.');
          addLog('error', `Spotify API error: ${err.message}`);
        } else {
          setError('An unexpected error occurred.');
          console.error('Album fetch error:', err);
        }
      } finally {
        setIsLoading(false);
      }
    };

    fetchAlbum();
  }, [id, addLog]);

  if (isLoading) {
    return (
      <div className="h-full flex items-center justify-center">
        <Loader2 className="animate-spin text-[#1db954]" size={48} />
      </div>
    );
  }

  if (error || !album) {
    return (
      <div className="h-full flex flex-col items-center justify-center p-8 text-center">
        <p className="text-red-500 mb-4">{error || 'Album not found'}</p>
        <button 
          onClick={() => navigate('/spotify')}
          className="flex items-center gap-2 px-4 py-2 bg-surface-2 hover:bg-surface-3 rounded-full transition-colors"
        >
          <ArrowLeft size={18} /> Back to Spotify
        </button>
      </div>
    );
  }

  const totalDuration = album.tracks.items.reduce((sum, track) => sum + track.duration_ms, 0);
  const releaseYear = album.release_date.split('-')[0];

  return (
    <div className="h-full overflow-y-auto">
      {/* Header */}
      <div className="bg-gradient-to-b from-[#1db954]/20 to-transparent">
        <div className="p-8">
          <button 
            onClick={() => navigate('/spotify')}
            className="flex items-center gap-2 text-text-secondary hover:text-text-main mb-6 transition-colors"
          >
            <ArrowLeft size={20} /> Back
          </button>

          <div className="flex gap-8 items-end">
            <div className="w-64 h-64 flex-shrink-0 shadow-2xl rounded-lg overflow-hidden">
              <img 
                src={album.images[0]?.url} 
                alt={album.name}
                className="w-full h-full object-cover"
              />
            </div>

            <div className="flex-1 pb-4">
              <p className="text-sm font-bold uppercase tracking-wider text-text-secondary mb-2">Album</p>
              <h1 className="text-5xl font-bold mb-4 leading-tight">{album.name}</h1>
              <div className="flex items-center gap-2 text-text-secondary">
                <span className="font-bold text-text-main">
                  {album.artists.map(a => a.name).join(', ')}
                </span>
                <span>•</span>
                <span>{releaseYear}</span>
                <span>•</span>
                <span>{album.total_tracks} songs</span>
                <span>•</span>
                <span>{formatTime(totalDuration / 1000)}</span>
              </div>
              {album.label && (
                <p className="text-text-subtle text-sm mt-2">{album.label}</p>
              )}
            </div>
          </div>
        </div>
      </div>

      {/* Actions */}
      <div className="px-8 py-6 flex items-center gap-4 bg-gradient-to-b from-transparent to-surface-0">
        <button className="w-14 h-14 bg-[#1db954] hover:bg-[#1ed760] rounded-full flex items-center justify-center shadow-xl hover:scale-105 transition-all text-black">
          <Play size={24} fill="black" />
        </button>
        <button 
          onClick={handleDownload}
          disabled={isDownloading}
          className="flex items-center gap-2 px-4 py-2 bg-surface-2 hover:bg-surface-3 rounded-full text-sm font-bold transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {isDownloading ? <Loader2 size={18} className="animate-spin" /> : <Download size={18} />}
          Download
        </button>
        <a 
          href={album.external_urls.spotify} 
          target="_blank" 
          rel="noopener noreferrer"
          className="flex items-center gap-2 px-4 py-2 bg-surface-2 hover:bg-surface-3 rounded-full text-sm font-bold transition-colors"
        >
          Open in Spotify <ExternalLink size={14} />
        </a>
      </div>

      {/* Track List */}
      <div className="px-8 pb-20">
        <div className="bg-surface-1 rounded-xl overflow-hidden">
          {/* Header */}
          <div className="grid grid-cols-[40px_1fr_1fr_80px_40px] gap-4 px-4 py-3 border-b border-surface-border text-text-subtle text-sm font-bold">
            <div className="text-center">#</div>
            <div>Title</div>
            <div>Artist</div>
            <div className="flex items-center justify-center gap-1">
              <Clock size={16} />
            </div>
            <div></div>
          </div>

          {/* Tracks */}
          {album.tracks.items.map((track, idx) => (
            <div 
              key={track.id}
              className="grid grid-cols-[40px_1fr_1fr_80px_40px] gap-4 px-4 py-3 hover:bg-surface-hover group transition-colors border-b border-surface-border last:border-0"
            >
              <div className="text-center text-text-subtle flex items-center justify-center">
                <span className="group-hover:hidden">{track.track_number}</span>
                <Play size={16} className="hidden group-hover:block text-text-main" />
              </div>
              <div className="flex flex-col justify-center min-w-0">
                <div className="font-medium text-text-main truncate group-hover:text-[#1db954] transition-colors">
                  {track.name}
                </div>
                {track.explicit && (
                  <span className="text-xs text-text-subtle bg-surface-3 px-1.5 py-0.5 rounded w-fit mt-1">
                    EXPLICIT
                  </span>
                )}
              </div>
              <div className="text-text-secondary truncate flex items-center">
                {track.artists.map(a => a.name).join(', ')}
              </div>
              <div className="text-text-subtle font-mono text-sm flex items-center justify-center">
                {formatTime(track.duration_ms / 1000)}
              </div>
              <div className="flex items-center justify-center">
                <button className="p-2 text-text-subtle hover:text-white opacity-0 group-hover:opacity-100 transition-opacity">
                  <MoreHorizontal size={18} />
                </button>
              </div>
            </div>
          ))}
        </div>

        {/* Copyright */}
        {album.copyrights && album.copyrights.length > 0 && (
          <div className="mt-8 text-text-subtle text-xs space-y-1">
            {album.copyrights.map((copyright, idx) => (
              <p key={idx}>{copyright.text}</p>
            ))}
          </div>
        )}
      </div>
    </div>
  );
};
