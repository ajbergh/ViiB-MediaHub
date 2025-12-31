/**
 * ViiB MediaHub - Home Page
 * 
 * Landing page displaying personalized content and quick navigation.
 * Establishes visual hierarchy with a hero section, stats cards, and content grids.
 * 
 * Sections:
 * - Global search bar for songs, albums, artists
 * - Featured Mix Hero: Prominent card featuring first available Smart Mix (uses Card variant="hero")
 * - Smart Mixes: Horizontal scroll of auto-generated playlists (uses SmartMixCard component)
 * - Stats Cards: Quick library overview with Total Songs, Albums, Artists (uses Card component)
 * - Recently Played: Last 20 played tracks with timestamps
 * - Recently Added: Latest additions to the library
 * - Top Artists: Most listened artists
 * 
 * Design System Usage:
 * - Card component with variant="hero" for featured content
 * - Card component with interactive prop for clickable stats
 * - SmartMixCard component for gradient Smart Mix cards
 * - Page component for consistent layout
 * - TextInput component for search
 * 
 * @module Home
 */

import React, { useEffect, useState, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { useStore, useAlbums, useArtists } from '../store';
import { Sparkles, Search, Play, Clock, History } from 'lucide-react';
import { generateGradient, coverBackground, formatTime } from '../utils';
import { ContextMenuType, Song } from '../types';
import { TextInput } from '../components/ui/TextInput';
import { Page } from '../components/ui/Page';
import { Card } from '../components/ui/Card';
import { SmartMixCard } from '../components/SmartMixCard';

export const Home: React.FC = () => {
  const { songs, smartMixes, refreshSmartMixes, playSong, openContextMenu, showSmartMixes } = useStore();
  const albums = useAlbums();
  const artists = useArtists();
  const navigate = useNavigate();
  const [searchQuery, setSearchQuery] = useState('');

  // Get recently played songs (sorted by lastPlayed, descending)
  const recentlyPlayed = useMemo(() => {
    return [...songs]
      .filter(s => s.lastPlayed && s.lastPlayed > 0)
      .sort((a, b) => (b.lastPlayed || 0) - (a.lastPlayed || 0))
      .slice(0, 20);
  }, [songs]);

  // Format relative time (e.g., "2 hours ago", "Yesterday")
  const formatRelativeTime = (timestamp: number): string => {
    const now = Date.now();
    const diff = now - timestamp;
    const minutes = Math.floor(diff / 60000);
    const hours = Math.floor(diff / 3600000);
    const days = Math.floor(diff / 86400000);

    if (minutes < 1) return 'Just now';
    if (minutes < 60) return `${minutes}m ago`;
    if (hours < 24) return `${hours}h ago`;
    if (days === 1) return 'Yesterday';
    if (days < 7) return `${days}d ago`;
    return new Date(timestamp).toLocaleDateString();
  };

  // Initial refresh of smart mixes just in case
  useEffect(() => {
      if (songs.length > 0) {
          refreshSmartMixes();
      }
  }, [songs.length]);

  const handleSearch = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && searchQuery.trim()) {
        navigate('/search', { state: { query: searchQuery } });
    }
  };

  const playMix = (mixId: string) => {
      const mix = smartMixes.find(m => m.id === mixId);
      if (mix && mix.songIds.length > 0) {
          const mixSongs = songs.filter(s => mix.songIds.includes(s.id));
          if (mixSongs.length > 0) {
              playSong(mixSongs[0], mixSongs);
          }
      }
  };

  return (
    <Page>
      {/* Header Section */}
      <section className="mb-12 flex flex-col items-center justify-center pt-8">
        <h1 className="text-display mb-8 text-center">Let's ViiB</h1>
        
        {/* Global Search Input */}
        <div className="w-full max-w-2xl flex justify-center">
          <TextInput
            leftIcon={<Search size={18} className="text-text-secondary" aria-hidden="true" />}
            type="text"
            placeholder="What do you want to feel?"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            onKeyDown={handleSearch}
          />
        </div>
      </section>

      {/* Featured Mix Hero */}
      {songs.length > 0 && showSmartMixes && smartMixes.length > 0 && (
        <section className="mb-12">
          <Card 
            variant="hero" 
            interactive 
            className="cursor-pointer relative overflow-hidden"
            onClick={() => navigate(`/smart-mix/${smartMixes[0].id}`)}
            style={{ 
              background: `linear-gradient(135deg, ${smartMixes[0].coverColors[0]}40, ${smartMixes[0].coverColors[1]}40)` 
            }}
          >
            <div className="flex items-center gap-3 mb-4">
              <Sparkles className="text-brand" size={28} />
              <span className="text-meta font-semibold uppercase tracking-wide text-text-secondary">Featured Mix</span>
            </div>
            <h2 className="text-display mb-2">{smartMixes[0].name}</h2>
            <p className="text-text-secondary text-body">{smartMixes[0].description}</p>
            <p className="text-text-subtle text-sm mt-4">{smartMixes[0].songIds.length} tracks • Ready to play</p>
          </Card>
        </section>
      )}

      {/* Smart Mixes Section */}
      {songs.length > 0 && showSmartMixes && (
          <section className="mb-12">
            <div className="flex items-center gap-2 mb-6">
            <Sparkles className="text-brand" size={24} />
            <h2 className="text-section">Smart Mixes</h2>
            </div>
            
            <div className="flex overflow-x-auto gap-6 pb-4 scrollbar-hide snap-x">
                {smartMixes.map((mix) => (
                    <SmartMixCard
                        key={mix.id}
                        mix={mix}
                        onPlay={() => playMix(mix.id)}
                        onClick={() => navigate(`/smart-mix/${mix.id}`)}
                        onContextMenu={(e) => openContextMenu(e, ContextMenuType.SMART_MIX, mix)}
                    />
                ))}
            </div>
          </section>
      )}

      {/* Stats Cards */}
      <section className="grid grid-cols-1 md:grid-cols-3 gap-6 mb-12">
        <Card 
            interactive
            className="group relative overflow-hidden cursor-pointer"
            onClick={() => navigate('/songs')}
        >
            <div className="relative z-10">
            <MusicIcon className="text-brand mb-4" />
              <h3 className="text-section font-bold mb-1">{songs.length}</h3>
                <p className="text-text-secondary text-sm font-medium">Total Songs</p>
            </div>
          <div className="absolute top-0 right-0 w-24 h-24 bg-brand/10 rounded-full -mr-4 -mt-4 blur-2xl transition-all group-hover:bg-brand/20"></div>
        </Card>

        <Card 
            interactive
            className="group relative overflow-hidden cursor-pointer"
            onClick={() => navigate('/albums')}
        >
            <div className="relative z-10">
            <AlbumIcon className="text-brand mb-4" />
              <h3 className="text-section font-bold mb-1">{albums.length}</h3>
                <p className="text-text-secondary text-sm font-medium">Albums</p>
            </div>
          <div className="absolute top-0 right-0 w-24 h-24 bg-brand/10 rounded-full -mr-4 -mt-4 blur-2xl transition-all group-hover:bg-brand/20"></div>
        </Card>

        <Card 
            interactive
            className="group relative overflow-hidden cursor-pointer"
            onClick={() => navigate('/artists')}
        >
             <div className="relative z-10">
            <ArtistIcon className="text-brand mb-4" />
              <h3 className="text-section font-bold mb-1">{artists.length}</h3>
                <p className="text-text-secondary text-sm font-medium">Artists</p>
            </div>
          <div className="absolute top-0 right-0 w-24 h-24 bg-brand/10 rounded-full -mr-4 -mt-4 blur-2xl transition-all group-hover:bg-brand/20"></div>
        </Card>
      </section>

      {/* Recently Played Section */}
      {recentlyPlayed.length > 0 && (
        <section className="mb-12">
          <div className="flex items-center gap-2 mb-6">
            <History className="text-accent-blue" size={24} />
            <h2 className="text-section font-semibold">Recently Played</h2>
          </div>
          
          <div className="bg-surface-2 rounded-xl border border-surface-3 overflow-hidden">
            <div className="divide-y divide-surface-3">
              {recentlyPlayed.slice(0, 10).map((song, idx) => (
                <div 
                  key={`${song.id}-${idx}`}
                  className="flex items-center gap-4 p-3 hover:bg-surface-hover transition-colors cursor-pointer group"
                  onClick={() => playSong(song)}
                  onContextMenu={(e) => openContextMenu(e, ContextMenuType.SONG, song)}
                >
                  {/* Album Art */}
                  <div 
                    className="w-12 h-12 rounded-md flex-shrink-0 relative overflow-hidden bg-surface-3"
                    style={{ background: coverBackground(song.coverUrl, song.album) }}
                  >
                    {song.coverUrl && (
                      <img src={song.coverUrl} alt="" className="w-full h-full object-cover" />
                    )}
                    <div className="absolute inset-0 bg-black/40 opacity-0 group-hover:opacity-100 flex items-center justify-center transition-opacity">
                      <Play size={20} className="text-white fill-current" />
                    </div>
                  </div>

                  {/* Song Info */}
                  <div className="flex-1 min-w-0">
                    <p className="font-medium text-text-main truncate group-hover:text-brand transition-colors">
                      {song.title}
                    </p>
                    <p className="text-sm text-text-secondary truncate">
                      {song.artist}
                    </p>
                  </div>

                  {/* Duration */}
                  <div className="hidden md:block text-sm text-text-secondary font-mono">
                    {formatTime(song.duration)}
                  </div>

                  {/* Time Ago */}
                  <div className="flex items-center gap-1 text-xs text-text-subtle min-w-[80px] justify-end">
                    <Clock size={12} />
                    <span>{formatRelativeTime(song.lastPlayed!)}</span>
                  </div>
                </div>
              ))}
            </div>
            
            {recentlyPlayed.length > 10 && (
              <button 
                onClick={() => navigate('/songs', { state: { sortBy: 'lastPlayed' } })}
                className="w-full py-3 text-sm text-text-secondary hover:text-text-main hover:bg-surface-hover transition-colors border-t border-surface-3"
              >
                View all recently played →
              </button>
            )}
          </div>
        </section>
      )}

      {/* Recently Added Albums */}
      <section>
        <h2 className="text-section font-semibold mb-6">Recently Added Albums</h2>
        {albums.length === 0 ? (
           <div className="bg-surface-2 rounded-xl p-8 text-center border border-dashed border-surface-border">
                <p className="text-text-secondary">No albums found. Import some music in Settings to get started!</p>
           </div>
        ) : (
            <div className="grid grid-cols-2 md:grid-cols-4 lg:grid-cols-5 gap-6">
            {albums.slice(0, 5).map((album, idx) => (
                <div 
                    key={idx} 
                    className="bg-surface-2 p-4 rounded-lg hover:bg-surface-3 transition-all group cursor-pointer"
                    onClick={() => navigate(`/album/${encodeURIComponent(album.name)}`)}
                >
                <div 
                  className="w-full aspect-square rounded-md mb-4 shadow-lg flex items-center justify-center text-display font-bold text-white/20 relative overflow-hidden bg-surface-3"
                    style={{ background: coverBackground(album.coverUrl, album.name) }}
                >
                     {!album.coverUrl && album.name.charAt(0)}
                </div>
                <h4 className="font-bold truncate text-text-main mb-1 group-hover:text-brand transition-colors">{album.name}</h4>
                <p className="text-sm text-text-secondary truncate">{album.artist}</p>
                </div>
            ))}
            </div>
        )}
      </section>
    </Page>
  );
};

// Icons Helpers
const MusicIcon = ({ className }: {className?: string}) => (
    <svg className={`w-8 h-8 ${className}`} fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 19V6l12-3v13M9 19c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zm12-3c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zM9 10l12-3" />
    </svg>
)
const AlbumIcon = ({ className }: {className?: string}) => (
    <svg className={`w-8 h-8 ${className}`} fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
    </svg>
)
const ArtistIcon = ({ className }: {className?: string}) => (
    <svg className={`w-8 h-8 ${className}`} fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
    </svg>
)
