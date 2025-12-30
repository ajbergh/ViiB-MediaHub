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
 * - Multi-Genre Mix: AI DJ always creates cross-genre blends based on user input
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

import React from 'react';
import { useStore } from '../store';
import { Sparkles, Play, Save, RefreshCw, Music, Zap, Heart, Clock, Shuffle, User, Eye, EyeOff, ChevronDown, ChevronUp, Compass, Sun } from 'lucide-react';
import { Song } from '../types';
import { api, MatchedGenre, SmartPlaylistFilter } from '../services/api';
import { apiSongToSong } from '../services/backendService';
import { formatTime } from '../utils';
import { Button } from '../components/ui/Button';
import { Chip } from '../components/ui/Chip';
import { TextInput } from '../components/ui/TextInput';

export const SmartPlaylists: React.FC = () => {
  const { 
    playSong, 
    showToast, 
    createPlaylist,
    // AI DJ state
    aiDjPrompt,
    aiDjGeneratedSongs,
    aiDjFilter,
    aiDjIsLoading,
    aiDjDiscoverMode,
    aiDjAvoidRecentlyHours,
    aiDjOnePerArtist,
    aiDjUseTimeContext,
    // AI DJ actions
    setAIDJPrompt,
    setAIDJGeneratedSongs,
    setAIDJFilter,
    setAIDJIsLoading,
    setAIDJDiscoverMode,
    setAIDJAvoidRecentlyHours,
    setAIDJOnePerArtist,
    setAIDJUseTimeContext,
  } = useStore();

  const handleGenerate = async () => {
    if (!aiDjPrompt.trim()) return;

    setAIDJIsLoading(true);
    setAIDJGeneratedSongs([]);
    setAIDJFilter(null);

    try {
      const result = await api.generateSmartPlaylist(aiDjPrompt, { 
        blendMode: 'mixed',
        targetSongs: 50,
        discoverMode: aiDjDiscoverMode,
        avoidRecentlyHours: aiDjAvoidRecentlyHours,
        onePerArtist: aiDjOnePerArtist,
        useTimeContext: aiDjUseTimeContext,
      });
      const apiSongs = result.songs || [];
      // Convert ApiSong[] to Song[] so they have the correct url field for playback
      const songs = apiSongs.map(apiSongToSong);
      setAIDJGeneratedSongs(songs);
      setAIDJFilter(result.filter || null);
      
      if (songs.length === 0) {
        showToast({ type: 'error', message: 'No songs found matching your request.' });
      }
    } catch (error) {
      console.error('Failed to generate playlist:', error);
      showToast({ type: 'error', message: 'Failed to generate playlist. Please try again.' });
    } finally {
      setAIDJIsLoading(false);
    }
  };

  const handlePlay = () => {
    if (aiDjGeneratedSongs.length > 0) {
      playSong(aiDjGeneratedSongs[0], aiDjGeneratedSongs);
    }
  };

  const handleSave = async () => {
    if (aiDjGeneratedSongs.length === 0 || !aiDjFilter) return;
    
    try {
      const name = aiDjFilter.description || aiDjPrompt;
      // Use store's createPlaylist to update both backend and frontend state
      await createPlaylist(name, aiDjGeneratedSongs.map(s => s.id));
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
            value={aiDjPrompt}
            onChange={(e) => setAIDJPrompt(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleGenerate()}
            className="flex-1 rounded-xl px-6 py-4 ring-1 ring-surface-highlight"
          />
          <Button
            onClick={handleGenerate}
            disabled={aiDjIsLoading || !aiDjPrompt.trim()}
            variant="primary"
            accent="brand"
            leftIcon={aiDjIsLoading ? <RefreshCw className="animate-spin" /> : <Sparkles />}
            className="px-8 py-4 rounded-xl text-body font-semibold"
          >
            Generate
          </Button>
        </div>

        {/* Discovery Mode */}
        <div className="mt-4">
          <h4 className="text-meta text-text-secondary mb-2 flex items-center gap-2">
            <Compass size={14} />
            Discovery Mode
          </h4>
          <div className="flex gap-2">
            <Chip
              selected={aiDjDiscoverMode === 'balanced'}
              accent="brand"
              onClick={() => setAIDJDiscoverMode('balanced')}
              className="rounded-lg px-3 py-1.5 text-meta font-medium"
            >
              Balanced
            </Chip>
            <Chip
              selected={aiDjDiscoverMode === 'discover'}
              accent="brand"
              onClick={() => setAIDJDiscoverMode('discover')}
              className="rounded-lg px-3 py-1.5 text-meta font-medium"
            >
              Discover New
            </Chip>
            <Chip
              selected={aiDjDiscoverMode === 'favorites'}
              accent="brand"
              onClick={() => setAIDJDiscoverMode('favorites')}
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
              value={aiDjAvoidRecentlyHours}
              onChange={(e) => setAIDJAvoidRecentlyHours(Number(e.target.value))}
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
              checked={aiDjOnePerArtist}
              onChange={(e) => setAIDJOnePerArtist(e.target.checked)}
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
              checked={aiDjUseTimeContext}
              onChange={(e) => setAIDJUseTimeContext(e.target.checked)}
              className={
                'w-4 h-4 rounded bg-surface-1 ' +
                'ring-1 ring-surface-3/80 accent-brand ' +
                'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/40 focus-visible:ring-offset-2 focus-visible:ring-offset-surface-0'
              }
            />
            <Sun size={14} className={aiDjUseTimeContext ? 'text-brand' : 'text-text-subtle'} />
            <span>Time-aware</span>
          </label>
        </div>
      </div>

      {/* Results */}
      <div className="flex-1 overflow-y-auto p-8 pt-0">
        {aiDjGeneratedSongs.length > 0 && (
          <div className="animate-fade-in">
            <div className="flex items-center justify-between mb-6">
              <div>
                <h2 className="text-section text-text-main mb-1 flex items-center gap-2">
                  {aiDjFilter?.description || 'Generated Playlist'}
                  {aiDjFilter?.fromCache && (
                    <span className="text-xs bg-surface-highlight text-text-subtle px-2 py-0.5 rounded-full font-normal">
                      cached
                    </span>
                  )}
                </h2>
                {/* Genre tags with proportions for multi-genre blend */}
                <div className="flex flex-wrap gap-2 text-meta text-text-secondary mb-2">
                  {aiDjFilter?.matchedGenres && aiDjFilter.matchedGenres.length > 0 ? (
                    aiDjFilter.matchedGenres.map(g => (
                      <span 
                        key={g.name} 
                        className="bg-surface-highlight px-2 py-1 rounded-lg flex items-center gap-1"
                        title={`Score: ${g.score}, Songs: ${g.songCount}`}
                      >
                        {g.name}
                        {aiDjFilter.blendMode === 'mixed' && (
                          <span className="text-brand text-meta font-medium">
                            {Math.round(g.proportion * 100)}%
                          </span>
                        )}
                      </span>
                    ))
                  ) : (
                    aiDjFilter?.genres?.map(g => (
                      <span key={g} className="bg-surface-highlight px-2 py-1 rounded-lg">{g}</span>
                    ))
                  )}
                  {aiDjFilter?.minYear && aiDjFilter.minYear > 0 && (
                    <span className="bg-surface-highlight px-2 py-1 rounded-lg">{aiDjFilter.minYear}-{aiDjFilter.maxYear}</span>
                  )}
                  {aiDjFilter?.blendMode === 'mixed' && (
                    <span className="bg-brand/20 text-brand px-2 py-1 rounded-lg flex items-center gap-1">
                      <Shuffle size={12} />
                      Blended
                    </span>
                  )}
                </div>
                {/* Mood/Energy/Occasion indicators */}
                {(aiDjFilter?.mood || aiDjFilter?.energy || aiDjFilter?.tempo || aiDjFilter?.occasion) && (
                  <div className="flex flex-wrap gap-2 text-meta">
                    {aiDjFilter?.mood && (
                      <span className="flex items-center gap-1 bg-brand/20 text-brand px-2 py-1 rounded-lg">
                        <Heart size={12} />
                        {aiDjFilter.mood}
                      </span>
                    )}
                    {aiDjFilter?.energy && (
                      <span className="flex items-center gap-1 bg-brand/15 text-brand px-2 py-1 rounded-lg">
                        <Zap size={12} />
                        {aiDjFilter.energy} energy
                      </span>
                    )}
                    {aiDjFilter?.tempo && (
                      <span className="flex items-center gap-1 bg-brand/15 text-brand px-2 py-1 rounded-lg">
                        <Clock size={12} />
                        {aiDjFilter.tempo} tempo
                      </span>
                    )}
                    {aiDjFilter?.occasion && (
                      <span className="bg-brand/20 text-brand px-2 py-1 rounded-lg">
                        {aiDjFilter.occasion}
                      </span>
                    )}
                    {aiDjFilter?.instrumental && (
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
                  {aiDjGeneratedSongs.map((song, index) => (
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

        {aiDjGeneratedSongs.length === 0 && !aiDjIsLoading && (
          <div className="flex flex-col items-center justify-center h-64 text-text-subtle">
            <Music size={48} className="mb-4 opacity-20" />
            <p>Enter a prompt above to generate a playlist.</p>
          </div>
        )}
      </div>
    </div>
  );
};
