/**
 * SmartPlaylists.tsx - AI DJ Interface
 * 
 * This component provides the main UI for ViiB MediaHub's AI DJ feature,
 * allowing users to generate playlists using natural language prompts.
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
import { api, MatchedGenre } from '../services/api';
import { apiSongToSong } from '../services/backendService';
import { formatTime } from '../utils';

/**
 * PlaylistFilter represents the criteria used to generate a playlist.
 * Returned by the backend after processing the user's prompt.
 */
interface PlaylistFilter {
  genres: string[];
  artists: string[];
  minYear: number;
  maxYear: number;
  description: string;
  mood?: string;            // Detected mood (happy, sad, energetic, etc.)
  energy?: string;          // Energy level (high, medium, low)
  tempo?: string;           // Tempo (fast, medium, slow)
  occasion?: string;        // Occasion hint (party, workout, study, etc.)
  instrumental?: boolean;   // Whether instrumental music is preferred
  fromCache?: boolean;      // True if result came from Gemini cache
  blendMode?: 'single' | 'mixed';  // Genre blending mode
  matchedGenres?: MatchedGenre[];  // Scored genres with proportions
}

export const SmartPlaylists: React.FC = () => {
  const { playSong, showToast, createPlaylist } = useStore();
  const [prompt, setPrompt] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [generatedSongs, setGeneratedSongs] = useState<Song[]>([]);
  const [filter, setFilter] = useState<PlaylistFilter | null>(null);
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
          <h1 className="text-4xl font-bold text-white">AI DJ</h1>
        </div>
        
        <p className="text-text-secondary mb-6 max-w-2xl">
          Describe the vibe, genre, era, or mood you're looking for, and I'll build a custom playlist from your library.
        </p>

        <div className="flex gap-4 max-w-3xl">
          <input
            type="text"
            placeholder="e.g., 'Upbeat 80s pop songs for a workout' or 'Chill jazz for studying'"
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleGenerate()}
            className="flex-1 bg-surface-1 border border-surface-highlight rounded-xl px-6 py-4 text-lg text-text-main placeholder:text-text-subtle focus:outline-none focus:border-brand transition-colors"
          />
          <button
            onClick={handleGenerate}
            disabled={isLoading || !prompt.trim()}
            className="bg-brand text-white px-8 py-4 rounded-xl font-bold hover:bg-brand-hover disabled:opacity-50 disabled:cursor-not-allowed transition-colors flex items-center gap-2"
          >
            {isLoading ? <RefreshCw className="animate-spin" /> : <Sparkles />}
            Generate
          </button>
        </div>

        {/* Blend Mode Toggle */}
        <div className="flex gap-4 mt-4">
          <button
            onClick={() => setBlendMode('single')}
            className={`flex items-center gap-2 px-4 py-2 rounded-lg font-medium transition-colors ${
              blendMode === 'single' 
                ? 'bg-brand text-white' 
                : 'bg-surface-1 text-text-secondary hover:bg-surface-2'
            }`}
          >
            <Target size={16} />
            Single Genre
          </button>
          <button
            onClick={() => setBlendMode('mixed')}
            className={`flex items-center gap-2 px-4 py-2 rounded-lg font-medium transition-colors ${
              blendMode === 'mixed' 
                ? 'bg-brand text-white' 
                : 'bg-surface-1 text-text-secondary hover:bg-surface-2'
            }`}
          >
            <Shuffle size={16} />
            Multi-Genre Mix
          </button>
        </div>

        {/* Discovery Mode */}
        <div className="mt-4">
          <h4 className="text-sm font-medium text-text-secondary mb-2 flex items-center gap-2">
            <Compass size={14} />
            Discovery Mode
          </h4>
          <div className="flex gap-2">
            <button
              onClick={() => setDiscoverMode('balanced')}
              className={`px-3 py-1.5 rounded-lg text-sm font-medium transition-colors ${
                discoverMode === 'balanced' 
                  ? 'bg-brand text-white' 
                  : 'bg-surface-1 text-text-secondary hover:bg-surface-2'
              }`}
            >
              Balanced
            </button>
            <button
              onClick={() => setDiscoverMode('discover')}
              className={`px-3 py-1.5 rounded-lg text-sm font-medium transition-colors ${
                discoverMode === 'discover' 
                  ? 'bg-green-600 text-white' 
                  : 'bg-surface-1 text-text-secondary hover:bg-surface-2'
              }`}
            >
              Discover New
            </button>
            <button
              onClick={() => setDiscoverMode('favorites')}
              className={`px-3 py-1.5 rounded-lg text-sm font-medium transition-colors ${
                discoverMode === 'favorites' 
                  ? 'bg-amber-600 text-white' 
                  : 'bg-surface-1 text-text-secondary hover:bg-surface-2'
              }`}
            >
              Favorites
            </button>
          </div>
        </div>

        {/* Additional Options */}
        <div className="mt-4 flex flex-wrap gap-4 items-center">
          <label className="flex items-center gap-2 text-sm text-text-secondary">
            <span>Avoid recently played:</span>
            <select
              value={avoidRecentlyHours}
              onChange={(e) => setAvoidRecentlyHours(Number(e.target.value))}
              className="bg-surface-1 border border-surface-2 rounded px-2 py-1 text-text-primary text-sm"
            >
              <option value={0}>Off</option>
              <option value={1}>1 hour</option>
              <option value={6}>6 hours</option>
              <option value={24}>24 hours</option>
              <option value={72}>3 days</option>
              <option value={168}>1 week</option>
            </select>
          </label>
          <label className="flex items-center gap-2 text-sm text-text-secondary cursor-pointer">
            <input
              type="checkbox"
              checked={onePerArtist}
              onChange={(e) => setOnePerArtist(e.target.checked)}
              className="w-4 h-4 rounded border-surface-2 bg-surface-1 text-brand focus:ring-brand"
            />
            <span>One song per artist</span>
          </label>
          <label className="flex items-center gap-2 text-sm text-text-secondary cursor-pointer" title="Adjust recommendations based on time of day">
            <input
              type="checkbox"
              checked={useTimeContext}
              onChange={(e) => setUseTimeContext(e.target.checked)}
              className="w-4 h-4 rounded border-surface-2 bg-surface-1 text-brand focus:ring-brand"
            />
            <Sun size={14} className={useTimeContext ? 'text-amber-400' : ''} />
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
                <h2 className="text-2xl font-bold text-white mb-1 flex items-center gap-2">
                  {filter?.description || 'Generated Playlist'}
                  {filter?.fromCache && (
                    <span className="text-xs bg-surface-highlight text-text-subtle px-2 py-0.5 rounded-full font-normal">
                      cached
                    </span>
                  )}
                </h2>
                {/* Genre tags with proportions for multi-genre blend */}
                <div className="flex flex-wrap gap-2 text-sm text-text-secondary mb-2">
                  {filter?.matchedGenres && filter.matchedGenres.length > 0 ? (
                    filter.matchedGenres.map(g => (
                      <span 
                        key={g.name} 
                        className="bg-surface-highlight px-2 py-1 rounded-md flex items-center gap-1"
                        title={`Score: ${g.score}, Songs: ${g.songCount}`}
                      >
                        {g.name}
                        {filter.blendMode === 'mixed' && (
                          <span className="text-brand text-xs font-medium">
                            {Math.round(g.proportion * 100)}%
                          </span>
                        )}
                      </span>
                    ))
                  ) : (
                    filter?.genres?.map(g => (
                      <span key={g} className="bg-surface-highlight px-2 py-1 rounded-md">{g}</span>
                    ))
                  )}
                  {filter?.minYear && filter.minYear > 0 && (
                    <span className="bg-surface-highlight px-2 py-1 rounded-md">{filter.minYear}-{filter.maxYear}</span>
                  )}
                  {filter?.blendMode === 'mixed' && (
                    <span className="bg-brand/20 text-brand px-2 py-1 rounded-md flex items-center gap-1">
                      <Shuffle size={12} />
                      Blended
                    </span>
                  )}
                </div>
                {/* Mood/Energy/Occasion indicators */}
                {(filter?.mood || filter?.energy || filter?.tempo || filter?.occasion) && (
                  <div className="flex flex-wrap gap-2 text-xs">
                    {filter?.mood && (
                      <span className="flex items-center gap-1 bg-pink-500/20 text-pink-300 px-2 py-1 rounded-md">
                        <Heart size={12} />
                        {filter.mood}
                      </span>
                    )}
                    {filter?.energy && (
                      <span className="flex items-center gap-1 bg-yellow-500/20 text-yellow-300 px-2 py-1 rounded-md">
                        <Zap size={12} />
                        {filter.energy} energy
                      </span>
                    )}
                    {filter?.tempo && (
                      <span className="flex items-center gap-1 bg-blue-500/20 text-blue-300 px-2 py-1 rounded-md">
                        <Clock size={12} />
                        {filter.tempo} tempo
                      </span>
                    )}
                    {filter?.occasion && (
                      <span className="bg-purple-500/20 text-purple-300 px-2 py-1 rounded-md">
                        {filter.occasion}
                      </span>
                    )}
                    {filter?.instrumental && (
                      <span className="bg-green-500/20 text-green-300 px-2 py-1 rounded-md">
                        Instrumental
                      </span>
                    )}
                  </div>
                )}
              </div>
              
              <div className="flex gap-3">
                <button
                  onClick={handlePlay}
                  className="flex items-center gap-2 px-6 py-2 bg-brand text-white rounded-full font-bold hover:bg-brand-hover transition-colors"
                >
                  <Play size={18} fill="currentColor" />
                  Play All
                </button>
                <button
                  onClick={handleSave}
                  className="flex items-center gap-2 px-6 py-2 bg-surface-3 text-text-main rounded-full font-bold hover:bg-surface-4 transition-colors"
                >
                  <Save size={18} />
                  Save Playlist
                </button>
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
