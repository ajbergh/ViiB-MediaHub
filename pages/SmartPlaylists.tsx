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
 * - Mode Toggle: Playlist mode vs DJ mode
 * - Natural language prompt input (e.g., "90s alternative rock", "chill evening vibes")
 * - Multi-Genre Mix: AI DJ always creates cross-genre blends based on user input
 * - Discovery Mode: Balanced, Discover New, or Favorites
 * - Avoid Recently Played: Configurable time window (1h to 1 week)
 * - One Per Artist: Ensures variety by limiting artist repetition
 * - Time-Aware Mode: Adjusts recommendations based on time of day
 * 
 * DJ Mode Features:
 * - Persona selection (6 DJ personalities with different scoring biases)
 * - Set duration control (15-120 minutes)
 * - Flow strictness slider (BPM continuity)
 * - Talk mode for DJ narration cues
 * - Phase visualization with energy arc
 * 
 * The backend uses a four-tier matching system plus intelligent sequencing
 * in DJ mode for structured sets with deliberate energy progression.
 */

import React, { useState, useEffect } from 'react';
import { useStore } from '../store';
import { Sparkles, Play, Save, RefreshCw, Music, Zap, Heart, Clock, Shuffle, User, Compass, Sun, Radio, Mic2, Timer, BarChart3 } from 'lucide-react';
import { Song } from '../types';
import { api, MatchedGenre, SmartPlaylistFilter, DJPersonaDefinition } from '../services/api';
import { apiSongToSong } from '../services/backendService';
import { formatTime } from '../utils';
import { Button } from '../components/ui/Button';
import { Chip } from '../components/ui/Chip';
import { TextInput } from '../components/ui/TextInput';

// DJ Persona descriptions for UI
const PERSONA_DESCRIPTIONS: Record<string, { name: string; description: string; icon: React.ReactNode }> = {
  FlowMaster: { name: 'Flow Master', description: 'Smooth transitions, strong BPM continuity', icon: <BarChart3 size={18} /> },
  CrowdPleaser: { name: 'Crowd Pleaser', description: 'Your favorites and highest-rated tracks', icon: <Heart size={18} /> },
  DeepCutDJ: { name: 'Deep Cut DJ', description: 'Hidden gems and underplayed tracks', icon: <Music size={18} /> },
  Explorer: { name: 'Explorer', description: 'Balanced novelty and familiar comfort', icon: <Compass size={18} /> },
  Curator: { name: 'Curator', description: 'Strict genre purity, one artist per set', icon: <User size={18} /> },
  NightDrive: { name: 'Night Drive', description: 'Smooth tempos, medium energy, atmospheric', icon: <Radio size={18} /> },
};

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
    // DJ Mode state
    aiDjMode,
    aiDjPersona,
    aiDjTargetDurationMinutes,
    aiDjFlowStrictness,
    aiDjTalkMode,
    aiDjPlan,
    aiDjPhases,
    aiDjNarration,
    // AI DJ actions
    setAIDJPrompt,
    setAIDJGeneratedSongs,
    setAIDJFilter,
    setAIDJIsLoading,
    setAIDJDiscoverMode,
    setAIDJAvoidRecentlyHours,
    setAIDJOnePerArtist,
    setAIDJUseTimeContext,
    // DJ Mode actions
    setAIDJMode,
    setAIDJPersona,
    setAIDJTargetDurationMinutes,
    setAIDJFlowStrictness,
    setAIDJTalkMode,
    setAIDJDJResult,
  } = useStore();

  // Available personas (could be fetched from API)
  const personas = Object.keys(PERSONA_DESCRIPTIONS);

  // Calculate estimated song count based on duration
  const estimatedSongCount = Math.round(aiDjTargetDurationMinutes / 3.5);

  const handleGenerate = async () => {
    if (!aiDjPrompt.trim()) return;

    setAIDJIsLoading(true);
    setAIDJGeneratedSongs([]);
    setAIDJFilter(null);

    try {
      const result = await api.generateSmartPlaylist(aiDjPrompt, { 
        blendMode: 'mixed',
        targetSongs: aiDjMode ? estimatedSongCount : 50,
        discoverMode: aiDjDiscoverMode,
        avoidRecentlyHours: aiDjAvoidRecentlyHours,
        onePerArtist: aiDjOnePerArtist,
        useTimeContext: aiDjUseTimeContext,
        // DJ Mode options
        mode: aiDjMode ? 'dj' : 'playlist',
        persona: aiDjPersona,
        targetDurationMinutes: aiDjTargetDurationMinutes,
        talkMode: aiDjTalkMode,
        flowStrictness: aiDjFlowStrictness,
      });
      const apiSongs = result.songs || [];
      // Convert ApiSong[] to Song[] so they have the correct url field for playback
      const songs = apiSongs.map(apiSongToSong);
      
      if (aiDjMode && result.dj) {
        // Update DJ mode state with full result
        setAIDJDJResult(
          aiDjPrompt,
          songs,
          result.filter || null,
          result.dj.plan,
          result.dj.phases,
          result.dj.narration || null
        );
      } else {
        setAIDJGeneratedSongs(songs);
        setAIDJFilter(result.filter || null);
      }
      
      if (songs.length === 0) {
        showToast({ 
          type: 'error', 
          message: aiDjMode 
            ? 'No songs found for your DJ set. Try a different prompt or check your library.' 
            : 'No songs found matching your request. Try different keywords.'
        });
      } else if (aiDjMode) {
        showToast({
          type: 'success',
          message: `DJ set ready! ${songs.length} songs across ${result.dj?.plan?.phases?.length || 0} phases.`
        });
      }
    } catch (error: any) {
      console.error('Failed to generate playlist:', error);
      
      // Provide more helpful error messages
      let errorMessage = 'Failed to generate playlist. Please try again.';
      if (error?.message?.includes('not configured') || error?.message?.includes('API key')) {
        errorMessage = 'AI provider not configured. Go to Settings to set up Ollama or add an API key.';
      } else if (error?.message?.includes('empty') || error?.message?.includes('no songs')) {
        errorMessage = 'Your library is empty. Scan some folders first in Settings.';
      } else if (error?.message?.includes('network') || error?.message?.includes('fetch')) {
        errorMessage = 'Network error. Check your connection and try again.';
      }
      
      showToast({ type: 'error', message: errorMessage });
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
        <div className="flex items-center gap-3 mb-4">
          <Sparkles className="text-brand" size={32} />
          <h1 className="text-display text-text-main">AI DJ</h1>
        </div>

        {/* Mode Toggle */}
        <div className="flex gap-2 mb-4">
          <Chip
            selected={!aiDjMode}
            accent="brand"
            onClick={() => setAIDJMode(false)}
            className="rounded-full px-4 py-2 text-body font-medium"
          >
            <Music size={16} className="mr-2" />
            Playlist
          </Chip>
          <Chip
            selected={aiDjMode}
            accent="brand"
            onClick={() => setAIDJMode(true)}
            className="rounded-full px-4 py-2 text-body font-medium"
          >
            <Radio size={16} className="mr-2" />
            DJ Mode
          </Chip>
        </div>
        
        <p className="text-body text-text-secondary mb-4 max-w-2xl">
          {aiDjMode 
            ? "Describe your vibe and I'll build a structured DJ set with deliberate energy flow and phase progression."
            : "Describe the vibe, genre, era, or mood you're looking for, and I'll build a custom playlist from your library."
          }
        </p>

        <div className="flex gap-4 max-w-3xl">
          <TextInput
            type="text"
            placeholder={aiDjMode 
              ? "e.g., 'Late night deep house session' or 'High energy festival peak time'"
              : "e.g., 'Upbeat 80s pop songs for a workout' or 'Chill jazz for studying'"
            }
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
            {aiDjMode ? 'Start Set' : 'Generate'}
          </Button>
        </div>

        {/* DJ Mode Controls */}
        {aiDjMode && (
          <div className="mt-6 p-4 bg-surface-1 rounded-xl border border-surface-highlight">
            <h3 className="text-meta text-text-secondary mb-3 flex items-center gap-2">
              <Radio size={14} />
              DJ Settings
            </h3>
            
            {/* Persona Selection */}
            <div className="mb-4">
              <label className="text-meta text-text-secondary mb-2 block">Select Persona</label>
              <div className="flex flex-wrap gap-2">
                {personas.map(p => (
                  <button
                    key={p}
                    onClick={() => setAIDJPersona(p as any)}
                    className={`flex items-center gap-2 px-3 py-2 rounded-lg text-sm transition-all
                      ${aiDjPersona === p 
                        ? 'bg-brand text-white' 
                        : 'bg-surface-2 text-text-secondary hover:bg-surface-highlight'
                      }`}
                    title={PERSONA_DESCRIPTIONS[p]?.description}
                  >
                    {PERSONA_DESCRIPTIONS[p]?.icon}
                    {PERSONA_DESCRIPTIONS[p]?.name || p}
                  </button>
                ))}
              </div>
              {aiDjPersona && PERSONA_DESCRIPTIONS[aiDjPersona] && (
                <p className="text-meta text-text-subtle mt-2">
                  {PERSONA_DESCRIPTIONS[aiDjPersona].description}
                </p>
              )}
            </div>

            {/* Duration & Flow Controls */}
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <label className="text-meta text-text-secondary mb-2 flex items-center gap-2">
                  <Timer size={14} />
                  Set Duration: {aiDjTargetDurationMinutes} min (~{estimatedSongCount} songs)
                </label>
                <input
                  type="range"
                  min={15}
                  max={120}
                  step={5}
                  value={aiDjTargetDurationMinutes}
                  onChange={(e) => setAIDJTargetDurationMinutes(Number(e.target.value))}
                  className="w-full accent-brand"
                />
                <div className="flex justify-between text-meta text-text-subtle">
                  <span>15 min</span>
                  <span>120 min</span>
                </div>
              </div>
              
              <div>
                <label className="text-meta text-text-secondary mb-2 flex items-center gap-2">
                  <BarChart3 size={14} />
                  Flow Strictness: {aiDjFlowStrictness}%
                </label>
                <input
                  type="range"
                  min={0}
                  max={100}
                  step={5}
                  value={aiDjFlowStrictness}
                  onChange={(e) => setAIDJFlowStrictness(Number(e.target.value))}
                  className="w-full accent-brand"
                />
                <div className="flex justify-between text-meta text-text-subtle">
                  <span>Loose</span>
                  <span>Strict BPM</span>
                </div>
              </div>
            </div>

            {/* Talk Mode Toggle */}
            <div className="mt-4">
              <label className="flex items-center gap-2 text-meta text-text-secondary cursor-pointer">
                <input
                  type="checkbox"
                  checked={aiDjTalkMode}
                  onChange={(e) => setAIDJTalkMode(e.target.checked)}
                  className="w-4 h-4 rounded bg-surface-1 ring-1 ring-surface-3/80 accent-brand"
                />
                <Mic2 size={14} className={aiDjTalkMode ? 'text-brand' : 'text-text-subtle'} />
                <span>DJ Talk Mode</span>
                <span className="text-text-subtle">(show narration cues)</span>
              </label>
            </div>
          </div>
        )}

        {/* Common Controls (shown in both modes) */}
        {!aiDjMode && (
          <>
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
          </>
        )}
      </div>

      {/* Results */}
      <div className="flex-1 overflow-y-auto p-8 pt-0">
        {aiDjGeneratedSongs.length > 0 && (
          <div className="animate-fade-in">
            {/* DJ Mode: Phase Timeline and Plan Info */}
            {aiDjMode && aiDjPlan && (
              <div className="mb-6">
                {/* Intent Summary */}
                <div className="bg-surface-1 rounded-xl p-4 mb-4 border border-surface-highlight">
                  <h3 className="text-body text-text-main mb-2 flex items-center gap-2">
                    <Radio size={18} className="text-brand" />
                    {aiDjPlan.intentSummary || 'DJ Set'}
                  </h3>
                  <div className="flex gap-4 text-meta text-text-secondary">
                    <span>Persona: {PERSONA_DESCRIPTIONS[aiDjPlan.persona]?.name || aiDjPlan.persona}</span>
                    <span>Duration: {aiDjPlan.targetDurationMin} min</span>
                    <span>{aiDjGeneratedSongs.length} songs</span>
                    {aiDjPlan.fromCache && (
                      <span className="text-xs bg-surface-highlight px-2 py-0.5 rounded-full">cached</span>
                    )}
                  </div>
                </div>

                {/* Phase Timeline */}
                {aiDjPlan.phases && aiDjPlan.phases.length > 0 && (
                  <div className="mb-4">
                    <h4 className="text-meta text-text-secondary mb-2">Set Phases</h4>
                    <div className="flex gap-1">
                      {aiDjPlan.phases.map((phase, i) => {
                        // Calculate phase width based on target count
                        const totalSongs = aiDjPlan.phases.reduce((sum, p) => sum + p.targetCount, 0);
                        const widthPercent = (phase.targetCount / totalSongs) * 100;
                        
                        // Color based on energy level
                        const energyColors: Record<string, string> = {
                          low: 'bg-blue-500/70',
                          medium: 'bg-brand/70',
                          high: 'bg-orange-500/70',
                        };
                        const bgColor = energyColors[phase.targetEnergy] || 'bg-surface-highlight';
                        
                        return (
                          <div 
                            key={i}
                            className={`${bgColor} rounded-lg p-2 text-center transition-all hover:opacity-80`}
                            style={{ width: `${widthPercent}%`, minWidth: '60px' }}
                            title={`${phase.name}: ${phase.minBPM}-${phase.maxBPM} BPM, ${phase.targetEnergy} energy, ${phase.targetCount} songs`}
                          >
                            <div className="text-meta text-white font-medium truncate">{phase.name}</div>
                            <div className="text-xs text-white/70">{phase.targetCount} songs</div>
                            <div className="text-xs text-white/70">{phase.minBPM}-{phase.maxBPM} BPM</div>
                          </div>
                        );
                      })}
                    </div>
                  </div>
                )}

                {/* DJ Narration (if talk mode) */}
                {aiDjTalkMode && aiDjNarration && (
                  <div className="bg-surface-2 rounded-xl p-4 border border-surface-highlight">
                    <h4 className="text-meta text-text-secondary mb-2 flex items-center gap-2">
                      <Mic2 size={14} />
                      DJ Talk Cues
                    </h4>
                    {aiDjNarration.intro && (
                      <p className="text-body text-text-main italic mb-2">"{aiDjNarration.intro}"</p>
                    )}
                    {aiDjNarration.phaseIntros && aiDjNarration.phaseIntros.length > 0 && (
                      <div className="text-meta text-text-secondary space-y-1">
                        {aiDjNarration.phaseIntros.map((intro, i) => (
                          <p key={i}>Phase {i + 1}: "{intro}"</p>
                        ))}
                      </div>
                    )}
                    {aiDjNarration.outro && (
                      <p className="text-body text-text-main italic mt-2">"{aiDjNarration.outro}"</p>
                    )}
                  </div>
                )}
              </div>
            )}

            <div className="flex items-center justify-between mb-6">
              <div>
                <h2 className="text-section text-text-main mb-1 flex items-center gap-2">
                  {aiDjMode ? 'DJ Set Queue' : (aiDjFilter?.description || 'Generated Playlist')}
                  {!aiDjMode && aiDjFilter?.fromCache && (
                    <span className="text-xs bg-surface-highlight text-text-subtle px-2 py-0.5 rounded-full font-normal">
                      cached
                    </span>
                  )}
                </h2>
                {/* Genre tags with proportions for multi-genre blend (playlist mode only) */}
                {!aiDjMode && (
                  <>
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
                  </>
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
                  {aiDjMode ? 'Play Set' : 'Play All'}
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
                    <tr key={song.id} className="group hover:bg-surface-highlight/50 transition-colors cursor-pointer" onClick={() => playSong(song, aiDjGeneratedSongs)}>
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

        {/* Loading State */}
        {aiDjIsLoading && (
          <div className="flex flex-col items-center justify-center h-64 text-text-subtle animate-pulse">
            <RefreshCw size={48} className="mb-4 text-brand animate-spin" />
            <p className="text-body text-text-main mb-2">
              {aiDjMode ? 'Building your DJ set...' : 'Generating playlist...'}
            </p>
            <p className="text-meta text-text-subtle">
              {aiDjMode 
                ? 'Analyzing your library and crafting the perfect energy arc' 
                : 'Finding the best matches in your library'
              }
            </p>
          </div>
        )}

        {aiDjGeneratedSongs.length === 0 && !aiDjIsLoading && (
          <div className="flex flex-col items-center justify-center h-64 text-text-subtle">
            {aiDjMode ? <Radio size={48} className="mb-4 opacity-20" /> : <Music size={48} className="mb-4 opacity-20" />}
            <p>{aiDjMode ? 'Enter a prompt above to start your DJ set.' : 'Enter a prompt above to generate a playlist.'}</p>
          </div>
        )}
      </div>
    </div>
  );
};
