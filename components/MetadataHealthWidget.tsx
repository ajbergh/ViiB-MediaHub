/**
 * ViiB MediaHub - Metadata Health & Completeness Widget
 * 
 * Analyzes the entire song library and displays a comprehensive health breakdown:
 * - Core Tagging (Title, Artist, Album, Year)
 * - Genres Coverage
 * - AI Music Profile (Mood, Energy, Tempo, BPM)
 * - Last.fm Community Enrichment
 * - ReplayGain Loudness Analysis
 * 
 * Provides quick actions to trigger enrichment or filter tracks missing metadata.
 * 
 * @module MetadataHealthWidget
 */

import React, { useMemo } from 'react';
import { useNavigate } from 'react-router';
import {
  Activity,
  Sparkles,
  Tag,
  Calendar,
  Volume2,
  Radio,
  FileCheck,
  CheckCircle2,
  AlertTriangle,
  ArrowRight,
  Zap,
  Sliders
} from 'lucide-react';
import { useStore } from '../store';
import { Button } from './ui/Button';

export const MetadataHealthWidget: React.FC = () => {
  const navigate = useNavigate();
  const songs = useStore(state => state.songs);
  const setLocalSearchQuery = useStore(state => state.setLocalSearchQuery);

  const stats = useMemo(() => {
    const total = songs.length;
    if (total === 0) {
      return {
        total: 0,
        corePercent: 0,
        genrePercent: 0,
        yearPercent: 0,
        originalYearPercent: 0,
        aiPercent: 0,
        lastfmPercent: 0,
        replayGainPercent: 0,
        overallScore: 0,
      };
    }

    let coreCount = 0;
    let genreCount = 0;
    let yearCount = 0;
    let originalYearCount = 0;
    let aiCount = 0;
    let lastfmCount = 0;
    let replayGainCount = 0;

    for (const song of songs) {
      if (song.title && song.artist && song.album) coreCount++;
      if (song.genre && song.genre.length > 0) genreCount++;
      if (song.year && song.year > 0) yearCount++;
      if (song.originalYear && song.originalYear > 0) originalYearCount++;
      if (song.mood || song.bpm) aiCount++;
      if (song.lastfmEnrichedAt || song.lastfmTags) lastfmCount++;
      if (song.replayGainDb !== undefined) replayGainCount++;
    }

    const corePercent = Math.round((coreCount / total) * 100);
    const genrePercent = Math.round((genreCount / total) * 100);
    const yearPercent = Math.round((yearCount / total) * 100);
    const originalYearPercent = Math.round((originalYearCount / total) * 100);
    const aiPercent = Math.round((aiCount / total) * 100);
    const lastfmPercent = Math.round((lastfmCount / total) * 100);
    const replayGainPercent = Math.round((replayGainCount / total) * 100);

    const overallScore = Math.round(
      (corePercent * 0.25) +
      (genrePercent * 0.20) +
      (yearPercent * 0.15) +
      (aiPercent * 0.20) +
      (lastfmPercent * 0.10) +
      (replayGainPercent * 0.10)
    );

    return {
      total,
      corePercent,
      genrePercent,
      yearPercent,
      originalYearPercent,
      aiPercent,
      lastfmPercent,
      replayGainPercent,
      overallScore,
    };
  }, [songs]);

  const handleFilterMissing = (type: string) => {
    setLocalSearchQuery(type);
    navigate('/search');
  };

  const getScoreColor = (score: number) => {
    if (score >= 85) return 'text-accent-green border-accent-green/40 bg-accent-green/10';
    if (score >= 60) return 'text-brand border-brand/40 bg-brand/10';
    return 'text-amber-400 border-amber-400/40 bg-amber-400/10';
  };

  return (
    <section className="rounded-xl border border-surface-highlight bg-surface-1 p-5 shadow-lg space-y-5">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 border-b border-surface-border/60 pb-4">
        <div className="flex items-center gap-3">
          <div className="p-2.5 rounded-lg bg-brand/15 text-brand">
            <FileCheck size={22} />
          </div>
          <div>
            <h2 className="text-xl font-bold text-text-main flex items-center gap-2">
              Library Metadata Health
            </h2>
            <p className="text-xs text-text-secondary">
              Completeness and enrichment coverage across {stats.total.toLocaleString()} tracks
            </p>
          </div>
        </div>

        {/* Overall Score Badge */}
        <div className="flex items-center gap-3 self-start sm:self-auto">
          <div className={`px-3.5 py-1.5 rounded-xl border font-mono text-sm font-bold flex items-center gap-2 ${getScoreColor(stats.overallScore)}`}>
            <Zap size={15} />
            <span>Health Score: {stats.overallScore}%</span>
          </div>
        </div>
      </div>

      {/* Progress Bars Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 text-xs">
        {/* Core Tags */}
        <div className="p-3.5 rounded-xl bg-surface-2/60 border border-surface-border/60 space-y-2">
          <div className="flex justify-between items-center">
            <span className="font-semibold text-text-main flex items-center gap-1.5">
              <CheckCircle2 size={14} className="text-brand" />
              Core Audio Tags
            </span>
            <span className="font-mono font-bold text-text-main">{stats.corePercent}%</span>
          </div>
          <div className="w-full h-2 bg-surface-3 rounded-full overflow-hidden">
            <div className="h-full bg-brand transition-all duration-500" style={{ width: `${stats.corePercent}%` }} />
          </div>
          <span className="text-[10px] text-text-subtle block">Title, Artist & Album metadata</span>
        </div>

        {/* Genres */}
        <div className="p-3.5 rounded-xl bg-surface-2/60 border border-surface-border/60 space-y-2">
          <div className="flex justify-between items-center">
            <span className="font-semibold text-text-main flex items-center gap-1.5">
              <Tag size={14} className="text-accent-orange" />
              Genres Tagged
            </span>
            <span className="font-mono font-bold text-text-main">{stats.genrePercent}%</span>
          </div>
          <div className="w-full h-2 bg-surface-3 rounded-full overflow-hidden">
            <div className="h-full bg-accent-orange transition-all duration-500" style={{ width: `${stats.genrePercent}%` }} />
          </div>
          <span className="text-[10px] text-text-subtle block">Tracks with assigned genre tags</span>
        </div>

        {/* Release Year */}
        <div className="p-3.5 rounded-xl bg-surface-2/60 border border-surface-border/60 space-y-2">
          <div className="flex justify-between items-center">
            <span className="font-semibold text-text-main flex items-center gap-1.5">
              <Calendar size={14} className="text-amber-400" />
              Release / Original Year
            </span>
            <span className="font-mono font-bold text-text-main">{stats.yearPercent}%</span>
          </div>
          <div className="w-full h-2 bg-surface-3 rounded-full overflow-hidden">
            <div className="h-full bg-amber-400 transition-all duration-500" style={{ width: `${stats.yearPercent}%` }} />
          </div>
          <span className="text-[10px] text-text-subtle block">Year tags ({stats.originalYearPercent}% original release)</span>
        </div>

        {/* AI Vibe & Mood */}
        <div className="p-3.5 rounded-xl bg-surface-2/60 border border-surface-border/60 space-y-2">
          <div className="flex justify-between items-center">
            <span className="font-semibold text-text-main flex items-center gap-1.5">
              <Sparkles size={14} className="text-accent-pink" />
              AI Mood & BPM Profile
            </span>
            <span className="font-mono font-bold text-text-main">{stats.aiPercent}%</span>
          </div>
          <div className="w-full h-2 bg-surface-3 rounded-full overflow-hidden">
            <div className="h-full bg-accent-pink transition-all duration-500" style={{ width: `${stats.aiPercent}%` }} />
          </div>
          <span className="text-[10px] text-text-subtle block">Gemini / LLM vibe & tempo analysis</span>
        </div>

        {/* Last.fm Enrichment */}
        <div className="p-3.5 rounded-xl bg-surface-2/60 border border-surface-border/60 space-y-2">
          <div className="flex justify-between items-center">
            <span className="font-semibold text-text-main flex items-center gap-1.5">
              <Radio size={14} className="text-red-400" />
              Last.fm Community Data
            </span>
            <span className="font-mono font-bold text-text-main">{stats.lastfmPercent}%</span>
          </div>
          <div className="w-full h-2 bg-surface-3 rounded-full overflow-hidden">
            <div className="h-full bg-red-400 transition-all duration-500" style={{ width: `${stats.lastfmPercent}%` }} />
          </div>
          <span className="text-[10px] text-text-subtle block">Global scrobbles & community tags</span>
        </div>

        {/* ReplayGain Normalization */}
        <div className="p-3.5 rounded-xl bg-surface-2/60 border border-surface-border/60 space-y-2">
          <div className="flex justify-between items-center">
            <span className="font-semibold text-text-main flex items-center gap-1.5">
              <Volume2 size={14} className="text-accent-blue" />
              ReplayGain Normalization
            </span>
            <span className="font-mono font-bold text-text-main">{stats.replayGainPercent}%</span>
          </div>
          <div className="w-full h-2 bg-surface-3 rounded-full overflow-hidden">
            <div className="h-full bg-accent-blue transition-all duration-500" style={{ width: `${stats.replayGainPercent}%` }} />
          </div>
          <span className="text-[10px] text-text-subtle block">Loudness offset tags calculated</span>
        </div>
      </div>

      {/* Action Footer */}
      <div className="flex flex-wrap items-center justify-between gap-3 pt-2 border-t border-surface-border/40 text-xs">
        <span className="text-text-subtle">
          Want to complete your library metadata? Configure AI or Last.fm enrichment in Settings.
        </span>
        <div className="flex items-center gap-2">
          <Button
            variant="secondary"
            onClick={() => navigate('/settings')}
            className="text-xs py-1.5 px-3"
            leftIcon={<Sliders size={13} />}
          >
            Configure Enrichment
          </Button>
        </div>
      </div>
    </section>
  );
};
