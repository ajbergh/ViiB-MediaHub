/**
 * SmartPlaylists.tsx - AI DJ Interface
 * 
 * This component provides the main UI for ViiB MediaHub's AI DJ feature,
 * allowing users to generate playlists using natural language prompts.
 * 
 * Design System Alignment (Phase 4 complete 2025-12-28):
 * - Uses DS primitives: Button, Chip, TextInput.
 * - Typography: text-display, text-section, text-body, text-meta.
 * - Accent: Brand Purple for all state badges and highlights.
 * 
 * Features:
 * - Natural language prompt input (e.g., "90s alternative rock", "chill evening vibes")
 * - Blend Mode toggle: Single Genre vs Multi-Genre Mix
 * - Discovery Mode: Balanced, Discover New, or Favorites
 * - Avoid Recently Played: Configurable time window (1h to 1 week)
 * - One Per Artist: Ensures variety by limiting artist repetition
 * - Time-Aware Mode: Adjusts recommendations based on time of day
 * 
 * The backend uses a three-tier matching system:
 * 1. Artist-based: "more like Radiohead" finds similar artists
 * 2. Local genre: Direct match against indexed genres (no API call)
 * 3. Gemini AI: Complex prompts parsed by AI with smart genre scoring
 * 
 * Results display includes matched genres with scores and proportions,
 * filterable song list, and save-to-playlist functionality.
 */

import React, { useState } from 'react';
import { useStore } from '../store';
import { Sparkles, Play, Save, RefreshCw, Music, Zap, Heart, Clock, Shuffle, Target, User, Eye, EyeOff, ChevronDown, ChevronUp, Compass, Sun } from 'lucide-react';
import { Song } from '../types';
import { api, MatchedGenre, SmartPlaylistFilter } from '../services/api';
import { apiSongToSong } from '../services/backendService';
import { formatTime } from '../utils';
import { Button } from '../components/ui/Button';
import { Chip } from '../components/ui/Chip';
import { TextInput } from '../components/ui/TextInput';

export const SmartPlaylists: React.FC = () => {
  const { playSong, showToast, createPlaylist } = useStore();
  const [prompt, setPrompt] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [generatedSongs, setGeneratedSongs] = useState<Song[]>([]);
  const [filter, setFilter] = useState<SmartPlaylistFilter | null>(null);
  const [blendMode, setBlendMode] = useState<'single' | 'mixed'>('single');
  
  // Play history preferences
  const [discoverMode, setDiscoverMode] = useState<'balanced' | 'discover' | 'favorites'>('balanced');
  const [avoidRecentlyHours, setAvoidRecentlyHours] = useState(0);
  const [onePerArtist, setOnePerArtist] = useState(false);
  const [useTimeContext, setUseTimeContext] = useState(false);

  const handleGenerate = async () => {
    if (!prompt.trim()) return;

    setIsLoading(true);
    setGeneratedSongs([]);
    setFilter(null);

    try {
      const result = await api.generateSmartPlaylist(prompt, { 
        blendMode, 
        targetSongs: 50,
        discoverMode,
        avoidRecentlyHours,
        onePerArtist,
        useTimeContext,
      });
      const apiSongs = result.songs || [];
      // Convert ApiSong[] to Song[] so they have the correct url field for playback
      const songs = apiSongs.map(apiSongToSong);
      setGeneratedSongs(songs);
      setFilter(result.filter || null);
      
      if (songs.length === 0) {
        showToast({ type: 'error', message: 'No songs found matching your request.' });
      }
    } catch (error) {
      console.error('Failed to generate playlist:', error);
      showToast({ type: 'error', message: 'Failed to generate playlist. Please try again.' });
    } finally {
      setIsLoading(false);
    }
  };

  const handlePlay = () => {
    if (generatedSongs.length > 0) {
      playSong(generatedSongs[0], generatedSongs);
    }
  };

  const handleSave = async () => {
    if (generatedSongs.length === 0 || !filter) return;
    
    try {
      const name = filter.description || prompt;
      // Use store's createPlaylist to update both backend and frontend state
      await createPlaylist(name, generatedSongs.map(s => s.id));
      showToast({ type: 'success', message: `Playlist "${name}" saved!` });
    } catch (error) {
      console.error('Failed to save playlist:', error);
      showToast({ type: 'error', message: 'Failed to save playlist.' });
    }
  };

  return (
    <div className="h-full flex flex-col bg-surface-0 overflow-hidden">
      {/* Header & Input */}
      <div className="p-8 pb-4">
        <div className="flex items-center gap-3 mb-6">
          <Sparkles className="text-brand" size={32} />
          <h1 className="text-display text-text-main">AI DJ</h1>
        </div>
        
        <p className="text-body text-text-secondary mb-6 max-w-2xl">
          Describe the vibe, genre, era, or mood you're looking for, and I'll build a custom playlist from your library.
        </p>

        <div className="flex gap-4 max-w-3xl">
          <TextInput
            type="text"
            placeholder="e.g., 'Upbeat 80s pop songs for a workout' or 'Chill jazz for studying'"
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleGenerate()}
            className="flex-1 rounded-xl px-6 py-4 ring-1 ring-surface-highlight"
          />
          <Button
            onClick={handleGenerate}
            disabled={isLoading || !prompt.trim()}
            variant="primary"
            accent="brand"
            leftIcon={isLoading ? <RefreshCw className="animate-spin" /> : <Sparkles />}
            className="px-8 py-4 rounded-xl text-body font-semibold"
          >
            Generate
          </Button>
        </div>

        {/* Blend Mode Toggle */}
        <div className="flex gap-4 mt-4">
          <Chip
            selected={blendMode === 'single'}
            accent="brand"
            onClick={() => setBlendMode('single')}
            className="rounded-lg px-4 py-2 text-meta font-medium"
          >
            <span className="inline-flex items-center gap-2">
              <Target size={16} />
              Single Genre
            </span>
          </Chip>
          <Chip
            selected={blendMode === 'mixed'}
            accent="brand"
            onClick={() => setBlendMode('mixed')}
            className="rounded-lg px-4 py-2 text-meta font-medium"
          >
            <span className="inline-flex items-center gap-2">
              <Shuffle size={16} />
              Multi-Genre Mix
            </span>
          </Chip>
        </div>

        {/* Discovery Mode */}
        <div className="mt-4">
          <h4 className="text-meta text-text-secondary mb-2 flex items-center gap-2">
            <Compass size={14} />
            Discovery Mode
          </h4>
          <div className="flex gap-2">
            <Chip
              selected={discoverMode === 'balanced'}
              accent="brand"
              onClick={() => setDiscoverMode('balanced')}
              className="rounded-lg px-3 py-1.5 text-meta font-medium"
            >
              Balanced
            </Chip>
            <Chip
              selected={discoverMode === 'discover'}
              accent="brand"
              onClick={() => setDiscoverMode('discover')}
              className="rounded-lg px-3 py-1.5 text-meta font-medium"
            >
              Discover New
            </Chip>
            <Chip
              selected={discoverMode === 'favorites'}
              accent="brand"
              onClick={() => setDiscoverMode('favorites')}
              className="rounded-lg px-3 py-1.5 text-meta font-medium"
            >
              Favorites
            </Chip>
          </div>
        </div>

        {/* Additional Options */}
        <div className="mt-4 flex flex-wrap gap-4 items-center">
          <label className="flex items-center gap-2 text-meta text-text-secondary">
            <span>Avoid recently played:</span>
            <select
              value={avoidRecentlyHours}
              onChange={(e) => setAvoidRecentlyHours(Number(e.target.value))}
              className={
                'bg-surface-1 text-text-main rounded-lg px-2 py-1 text-sm ' +
                'ring-1 ring-surface-3/80 ' +
                'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/40 focus-visible:ring-offset-2 focus-visible:ring-offset-surface-0'
              }
            >
              <option value={0}>Off</option>
              <option value={1}>1 hour</option>
              <option value={6}>6 hours</option>
              <option value={24}>24 hours</option>
              <option value={72}>3 days</option>
              <option value={168}>1 week</option>
            </select>
          </label>
          <label className="flex items-center gap-2 text-meta text-text-secondary cursor-pointer">
            <input
              type="checkbox"
              checked={onePerArtist}
              onChange={(e) => setOnePerArtist(e.target.checked)}
              className={
                'w-4 h-4 rounded bg-surface-1 ' +
                'ring-1 ring-surface-3/80 accent-brand ' +
                'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/40 focus-visible:ring-offset-2 focus-visible:ring-offset-surface-0'
              }
            />
            <span>One song per artist</span>
          </label>
          <label className="flex items-center gap-2 text-meta text-text-secondary cursor-pointer" title="Adjust recommendations based on time of day">
            <input
              type="checkbox"
              checked={useTimeContext}
              onChange={(e) => setUseTimeContext(e.target.checked)}
              className={
                'w-4 h-4 rounded bg-surface-1 ' +
                'ring-1 ring-surface-3/80 accent-brand ' +
                'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/40 focus-visible:ring-offset-2 focus-visible:ring-offset-surface-0'
              }
            />
            <Sun size={14} className={useTimeContext ? 'text-brand' : 'text-text-subtle'} />
            <span>Time-aware</span>
          </label>
        </div>
      </div>

      {/* Results */}
      <div className="flex-1 overflow-y-auto p-8 pt-0">
        {generatedSongs.length > 0 && (
          <div className="animate-fade-in">
            <div className="flex items-center justify-between mb-6">
              <div>
                <h2 className="text-section text-text-main mb-1 flex items-center gap-2">
                  {filter?.description || 'Generated Playlist'}
                  {filter?.fromCache && (
                    <span className="text-xs bg-surface-highlight text-text-subtle px-2 py-0.5 rounded-full font-normal">
                      cached
                    </span>
                  )}
                </h2>
                {/* Genre tags with proportions for multi-genre blend */}
                <div className="flex flex-wrap gap-2 text-meta text-text-secondary mb-2">
                  {filter?.matchedGenres && filter.matchedGenres.length > 0 ? (
                    filter.matchedGenres.map(g => (
                      <span 
                        key={g.name} 
                        className="bg-surface-highlight px-2 py-1 rounded-lg flex items-center gap-1"
                        title={`Score: ${g.score}, Songs: ${g.songCount}`}
                      >
                        {g.name}
                        {filter.blendMode === 'mixed' && (
                          <span className="text-brand text-meta font-medium">
                            {Math.round(g.proportion * 100)}%
                          </span>
                        )}
                      </span>
                    ))
                  ) : (
                    filter?.genres?.map(g => (
                      <span key={g} className="bg-surface-highlight px-2 py-1 rounded-lg">{g}</span>
                    ))
                  )}
                  {filter?.minYear && filter.minYear > 0 && (
                    <span className="bg-surface-highlight px-2 py-1 rounded-lg">{filter.minYear}-{filter.maxYear}</span>
                  )}
                  {filter?.blendMode === 'mixed' && (
                    <span className="bg-brand/20 text-brand px-2 py-1 rounded-lg flex items-center gap-1">
                      <Shuffle size={12} />
                      Blended
                    </span>
                  )}
                </div>
                {/* Mood/Energy/Occasion indicators */}
                {(filter?.mood || filter?.energy || filter?.tempo || filter?.occasion) && (
                  <div className="flex flex-wrap gap-2 text-meta">
                    {filter?.mood && (
                      <span className="flex items-center gap-1 bg-brand/20 text-brand px-2 py-1 rounded-lg">
                        <Heart size={12} />
                        {filter.mood}
                      </span>
                    )}
                    {filter?.energy && (
                      <span className="flex items-center gap-1 bg-brand/15 text-brand px-2 py-1 rounded-lg">
                        <Zap size={12} />
                        {filter.energy} energy
                      </span>
                    )}
                    {filter?.tempo && (
                      <span className="flex items-center gap-1 bg-brand/15 text-brand px-2 py-1 rounded-lg">
                        <Clock size={12} />
                        {filter.tempo} tempo
                      </span>
                    )}
                    {filter?.occasion && (
                      <span className="bg-brand/20 text-brand px-2 py-1 rounded-lg">
                        {filter.occasion}
                      </span>
                    )}
                    {filter?.instrumental && (
                      <span className="bg-brand/15 text-brand px-2 py-1 rounded-lg">
                        Instrumental
                      </span>
                    )}
                  </div>
                )}
              </div>
              
              <div className="flex gap-3">
                <Button
                  onClick={handlePlay}
                  variant="primary"
                  accent="brand"
                  leftIcon={<Play size={18} fill="currentColor" />}
                  className="rounded-full px-6 py-2 font-semibold"
                >
                  Play All
                </Button>
                <Button
                  onClick={handleSave}
                  variant="secondary"
                  leftIcon={<Save size={18} />}
                  className="rounded-full px-6 py-2 font-semibold"
                >
                  Save Playlist
                </Button>
              </div>
            </div>

            <div className="bg-surface-1 rounded-xl border border-surface-highlight overflow-hidden">
              <table className="w-full text-left">
                <thead className="bg-surface-2 text-text-subtle text-xs uppercase tracking-wider font-medium">
                  <tr>
                    <th className="px-6 py-3 w-12">#</th>
                    <th className="px-6 py-3">Title</th>
                    <th className="px-6 py-3">Artist</th>
                    <th className="px-6 py-3">Album</th>
                    <th className="px-6 py-3 text-right">Duration</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-surface-highlight">
                  {generatedSongs.map((song, index) => (
                    <tr key={song.id} className="group hover:bg-surface-highlight/50 transition-colors">
                      <td className="px-6 py-3 text-text-subtle text-center">{index + 1}</td>
                      <td className="px-6 py-3 font-medium text-text-main">{song.title}</td>
                      <td className="px-6 py-3 text-text-secondary">{song.artist}</td>
                      <td className="px-6 py-3 text-text-secondary">{song.album}</td>
                      <td className="px-6 py-3 text-text-subtle text-right font-mono text-xs">
                        {formatTime(song.duration)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}

        {generatedSongs.length === 0 && !isLoading && (
          <div className="flex flex-col items-center justify-center h-64 text-text-subtle">
            <Music size={48} className="mb-4 opacity-20" />
            <p>Enter a prompt above to generate a playlist.</p>
          </div>
        )}
      </div>
    </div>
  );
};
