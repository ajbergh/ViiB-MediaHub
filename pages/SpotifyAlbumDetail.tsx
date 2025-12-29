/**
 * ViiB MediaHub - Spotify Album Detail Page
 * 
 * Browse and download albums from the Spotify catalog.
 * 
 * Features:
 * - Full album metadata from Spotify API
 * - Track listing with duration
 * - Download entire album button
 * - Download individual tracks
 * - External link to Spotify
 * - Album artwork display
 * 
 * Requires Spotify authentication and Premium for downloads.
 * 
 * @module SpotifyAlbumDetail
 */

import React, { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { ArrowLeft, Play, MoreHorizontal, Loader2, Clock, ExternalLink, Download, Shuffle, ListPlus, CheckCircle } from 'lucide-react';
import { SpotifyService } from '../services/spotifyService';
import { useStore } from '../store';
import { formatTime } from '../utils';
import { SpotifyAuthError, SpotifyRateLimitError, SpotifyApiError } from '../lib/spotifyErrors';
import api from '../services/api';
import { spotifyAlbumToSongs, spotifyTrackToSong } from '../lib/spotifyHelpers';
import { ContextMenuType } from '../types';
import { Button } from '../components/ui/Button';

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
  const { addLog, playSong, addToQueue, showToast, openContextMenu } = useStore();
  
  const [album, setAlbum] = useState<SpotifyAlbumFull | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isDownloading, setIsDownloading] = useState(false);
  const [downloadingTracks, setDownloadingTracks] = useState<Set<string>>(new Set());
  const [downloadedSpotifyIds, setDownloadedSpotifyIds] = useState<Set<string>>(new Set());

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

  // Play the entire album
  const handlePlayAlbum = () => {
    if (!album) return;
    const songs = spotifyAlbumToSongs(album);
    if (songs.length > 0) {
      playSong(songs[0], songs);
      addLog('info', `▶ Playing album: ${album.name}`);
    }
  };

  // Shuffle play the album
  const handleShuffleAlbum = () => {
    if (!album) return;
    const songs = spotifyAlbumToSongs(album);
    if (songs.length > 0) {
      const shuffled = [...songs].sort(() => Math.random() - 0.5);
      playSong(shuffled[0], shuffled);
      addLog('info', `🔀 Shuffling album: ${album.name}`);
    }
  };

  // Add album to queue
  const handleAddAlbumToQueue = () => {
    if (!album) return;
    const songs = spotifyAlbumToSongs(album);
    if (songs.length > 0) {
      addToQueue(songs);
      showToast({ type: 'success', message: `Added ${songs.length} tracks to queue` });
    }
  };

  // Play a specific track (with rest of album as context)
  const handlePlayTrack = (trackIndex: number) => {
    if (!album) return;
    const songs = spotifyAlbumToSongs(album);
    if (songs.length > 0 && trackIndex < songs.length) {
      playSong(songs[trackIndex], songs);
    }
  };

  // Download a single track
  const handleDownloadTrack = async (track: SpotifyAlbumFull['tracks']['items'][0]) => {
    if (downloadingTracks.has(track.id)) return;
    
    setDownloadingTracks(prev => new Set(prev).add(track.id));
    try {
      await api.downloadTrack(
        track.id,
        track.name,
        track.artists?.map(a => a.name).join(', ') || album?.artists[0]?.name || 'Unknown Artist',
        album?.name || 'Unknown Album',
        Math.floor(track.duration_ms / 1000)
      );
      showToast({ type: 'success', message: `Queued for download: ${track.name}` });
    } catch (error) {
      console.error('Failed to queue download:', error);
      showToast({ type: 'error', message: 'Failed to queue download' });
    } finally {
      setDownloadingTracks(prev => {
        const newSet = new Set(prev);
        newSet.delete(track.id);
        return newSet;
      });
    }
  };

  // Open context menu for a track
  const handleTrackContextMenu = (e: React.MouseEvent, track: SpotifyAlbumFull['tracks']['items'][0]) => {
    e.preventDefault();
    const song = spotifyTrackToSong({
      ...track,
      album: { name: album?.name || '', images: album?.images }
    });
    openContextMenu(e, ContextMenuType.SONG, song);
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
        <Loader2 className="animate-spin text-brand" size={48} />
      </div>
    );
  }

  if (error || !album) {
    return (
      <div className="h-full flex flex-col items-center justify-center p-8 text-center">
        <p className="text-error mb-4">{error || 'Album not found'}</p>
        <Button
          variant="secondary"
          onClick={() => navigate('/spotify')}
          leftIcon={<ArrowLeft size={18} />}
          className="rounded-full px-4 py-2"
        >
          Back to Spotify
        </Button>
      </div>
    );
  }

  const totalDuration = album.tracks.items.reduce((sum, track) => sum + track.duration_ms, 0);
  const releaseYear = album.release_date.split('-')[0];

  return (
    <div className="h-full overflow-y-auto">
      {/* Header */}
      <div className="bg-gradient-to-b from-brand/20 to-transparent">
        <div className="p-8">
          <Button
            variant="ghost"
            onClick={() => navigate('/spotify')}
            leftIcon={<ArrowLeft size={20} />}
            className="px-0 py-0 text-text-secondary hover:text-text-main hover:bg-transparent"
          >
            Back
          </Button>

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
              <h1 className="text-display font-bold mb-4 leading-tight">{album.name}</h1>
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
        <Button
          variant="primary"
          accent="brand"
          onClick={handlePlayAlbum}
          className="w-14 h-14 p-0 rounded-full shadow-xl hover:scale-105 transition-all duration-200"
          aria-label="Play album"
        >
          <Play size={24} fill="black" />
        </Button>
        <Button
          variant="secondary"
          onClick={handleShuffleAlbum}
          className="w-10 h-10 p-0 rounded-full"
          aria-label="Shuffle album"
          title="Shuffle"
        >
          <Shuffle size={18} />
        </Button>
        <Button
          variant="secondary"
          onClick={handleAddAlbumToQueue}
          className="w-10 h-10 p-0 rounded-full"
          aria-label="Add to queue"
          title="Add to queue"
        >
          <ListPlus size={18} />
        </Button>
        <Button
          variant="secondary"
          onClick={handleDownload}
          disabled={isDownloading}
          leftIcon={isDownloading ? <Loader2 size={18} className="animate-spin" /> : <Download size={18} />}
          className="rounded-full px-4 py-2 text-sm font-bold"
          aria-label="Download album"
        >
          Download
        </Button>
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
      <div className="px-8 pb-32">
        <div className="bg-surface-1 rounded-xl overflow-hidden">
          {/* Header */}
          <div className="grid grid-cols-[40px_1fr_1fr_80px_80px] gap-4 px-4 py-3 border-b border-surface-border text-text-subtle text-sm font-bold">
            <div className="text-center">#</div>
            <div>Title</div>
            <div>Artist</div>
            <div className="flex items-center justify-center gap-1">
              <Clock size={16} />
            </div>
            <div></div>
          </div>

          {/* Tracks */}
          {album.tracks.items.map((track, idx) => {
            const isDownloaded = downloadedSpotifyIds.has(track.id);
            return (
            <div 
              key={track.id}
              onClick={() => handlePlayTrack(idx)}
              onContextMenu={(e) => handleTrackContextMenu(e, track)}
              className="grid grid-cols-[40px_1fr_1fr_80px_80px] gap-4 px-4 py-3 hover:bg-surface-hover group transition-all duration-200 border-b border-surface-border last:border-0 cursor-pointer"
            >
              <div className="text-center text-text-subtle flex items-center justify-center">
                <span className="group-hover:hidden">{track.track_number}</span>
                <Play size={16} className="hidden group-hover:block text-brand fill-current" />
              </div>
              <div className="flex flex-col justify-center min-w-0">
                <div className="font-medium text-text-main truncate group-hover:text-brand transition-all duration-200">
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
              <div className="flex items-center justify-center gap-1">
                {isDownloaded ? (
                  <div className="p-2 text-brand" title="Downloaded">
                    <CheckCircle size={16} />
                  </div>
                ) : (
                  <Button
                    variant="ghost"
                    onClick={(e) => { e.stopPropagation(); handleDownloadTrack(track); }}
                    disabled={downloadingTracks.has(track.id)}
                    className="p-2 text-text-subtle hover:text-white opacity-0 group-hover:opacity-100 transition-opacity disabled:opacity-50"
                    title="Download"
                    aria-label="Download"
                  >
                    {downloadingTracks.has(track.id) ? (
                      <Loader2 size={16} className="animate-spin" />
                    ) : (
                      <Download size={16} />
                    )}
                  </Button>
                )}
                <Button
                  variant="ghost"
                  onClick={(e) => { e.stopPropagation(); handleTrackContextMenu(e, track); }}
                  className="p-2 text-text-subtle hover:text-white opacity-0 group-hover:opacity-100 transition-opacity"
                  aria-label="More"
                >
                  <MoreHorizontal size={18} />
                </Button>
              </div>
            </div>
          );
          })}
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
