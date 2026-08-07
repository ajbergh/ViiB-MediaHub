/**
 * ViiB MediaHub - Song Info / Properties Dialog
 * 
 * Displays rich, multi-layered metadata for any track in the library:
 * - Overview: High-res artwork, core audio tags, original vs remaster release year
 * - Vibe & AI Profile: Mood, Energy, Tempo, BPM, Instrumental flag, Genre & Last.fm tags
 * - Listening Analytics: Personal play count, skip rate, date added, last played, Last.fm global listeners
 * - Technical & File: Audio format, ReplayGain offset & peak, file path, Spotify streaming status
 * 
 * Includes interactive metadata chips (click BPM/Mood/Genre to search or filter)
 * and file path copying / file location capabilities.
 * 
 * @module SongInfoDialog
 */

import React, { useState } from 'react';
import { useNavigate } from 'react-router';
import {
  X,
  Play,
  ListPlus,
  Music,
  Sparkles,
  BarChart3,
  FileText,
  FolderOpen,
  ExternalLink,
  Clock,
  Calendar,
  Activity,
  Headphones,
  Copy,
  Check,
  Disc,
  Mic2,
  Tag,
  Radio,
  BarChart,
  Percent,
  Sliders,
  Volume2,
  Edit3,
  Save,
  RotateCcw
} from 'lucide-react';
import { useStore, useAlbumCovers } from '../store';
import { useFocusTrap } from '../hooks/useFocusTrap';
import { formatTime, generateGradient } from '../utils';
import { Chip } from './ui/Chip';
import { LikeButton } from './LikeButton';
import { Button } from './ui/Button';

type TabType = 'vibe' | 'overview' | 'stats' | 'technical';

export const SongInfoDialog: React.FC = () => {
  const navigate = useNavigate();
  const { songInfoModalSong, closeSongInfoModal, playSong, addToQueue, setLocalSearchQuery, showToast, updateSongMetadata } = useStore();
  const albumCovers = useAlbumCovers();
  const [activeTab, setActiveTab] = useState<TabType>('overview');
  const [copiedPath, setCopiedPath] = useState(false);

  // Edit Mode state
  const [isEditing, setIsEditing] = useState(false);
  const [editTitle, setEditTitle] = useState('');
  const [editArtist, setEditArtist] = useState('');
  const [editAlbum, setEditAlbum] = useState('');
  const [editAlbumArtist, setEditAlbumArtist] = useState('');
  const [editTrackNumber, setEditTrackNumber] = useState<number | string>('');
  const [editDiscNumber, setEditDiscNumber] = useState<number | string>('');
  const [editYear, setEditYear] = useState<number | string>('');
  const [editGenres, setEditGenres] = useState('');
  const [isSaving, setIsSaving] = useState(false);

  const containerRef = useFocusTrap<HTMLDivElement>(!!songInfoModalSong, closeSongInfoModal);

  if (!songInfoModalSong) return null;

  const song = songInfoModalSong;
  const coverUrl = song.coverUrl || albumCovers[song.album];

  const handleStartEditing = () => {
    setEditTitle(song.title);
    setEditArtist(song.artist);
    setEditAlbum(song.album);
    setEditAlbumArtist(song.albumArtist || '');
    setEditTrackNumber(song.trackNumber || '');
    setEditDiscNumber(song.discNumber || '');
    setEditYear(song.year || '');
    setEditGenres(song.genre ? song.genre.join(', ') : '');
    setIsEditing(true);
    setActiveTab('overview');
  };

  const handleSaveTags = async () => {
    if (!editTitle.trim() || !editArtist.trim() || !editAlbum.trim()) {
      showToast({ type: 'error', message: 'Title, Artist, and Album cannot be empty' });
      return;
    }

    setIsSaving(true);
    try {
      const genresArray = editGenres.split(',').map(g => g.trim()).filter(Boolean);
      await updateSongMetadata(song.id, {
        title: editTitle.trim(),
        artist: editArtist.trim(),
        album: editAlbum.trim(),
        albumArtist: editAlbumArtist.trim() || undefined,
        trackNumber: editTrackNumber !== '' ? Number(editTrackNumber) : undefined,
        discNumber: editDiscNumber !== '' ? Number(editDiscNumber) : undefined,
        year: editYear !== '' ? Number(editYear) : undefined,
        genre: genresArray.length > 0 ? genresArray : undefined,
      });

      showToast({ type: 'success', message: `Saved tags for "${editTitle.trim()}"` });
      setIsEditing(false);
    } catch (err) {
      console.error('Save tags failed:', err);
      showToast({ type: 'error', message: 'Failed to update song metadata' });
    } finally {
      setIsSaving(false);
    }
  };

  const handleSearchFilter = (query: string) => {
    setLocalSearchQuery(query);
    closeSongInfoModal();
    navigate('/search', { state: { query } });
  };

  const handleCopyPath = () => {
    if (song.path || song.url) {
      navigator.clipboard.writeText(song.path || song.url);
      setCopiedPath(true);
      showToast({ type: 'success', message: 'Path copied to clipboard' });
      setTimeout(() => setCopiedPath(false), 2000);
    }
  };

  // Calculate skip rate
  const totalAttempts = (song.playCount || 0) + (song.skipCount || 0);
  const skipRate = totalAttempts > 0 ? Math.round(((song.skipCount || 0) / totalAttempts) * 100) : 0;
  const completionRate = totalAttempts > 0 ? 100 - skipRate : 100;

  // Format timestamp helper
  const formatDate = (timestamp?: number) => {
    if (!timestamp) return 'Never / Unknown';
    return new Date(timestamp).toLocaleDateString(undefined, {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });
  };

  // Extract tags from Last.fm if available
  let parsedLastFmTags: string[] = [];
  if (song.lastfmTags) {
    try {
      parsedLastFmTags = JSON.parse(song.lastfmTags);
    } catch {
      parsedLastFmTags = song.lastfmTags.split(',').map(t => t.trim()).filter(Boolean);
    }
  }

  return (
    <div
      className="fixed inset-0 z-[120] flex items-center justify-center bg-black/80 backdrop-blur-md p-4 animate-in fade-in duration-200"
      role="dialog"
      aria-modal="true"
      aria-labelledby="song-info-title"
      onClick={(e) => {
        if (e.target === e.currentTarget) closeSongInfoModal();
      }}
    >
      <div
        ref={containerRef}
        className="relative w-full max-w-2xl max-h-[90vh] flex flex-col bg-surface-1 border border-surface-border/80 rounded-2xl shadow-2xl overflow-hidden text-text-main animate-in zoom-in-95 duration-200"
      >
        {/* Modal Header Bar */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-surface-border bg-surface-2/60">
          <div className="flex items-center gap-2">
            <FileText size={18} className="text-brand" />
            <span className="font-semibold text-sm tracking-wide text-text-main">Song Information & Properties</span>
          </div>
          <div className="flex items-center gap-2">
            {!isEditing ? (
              <Button
                variant="secondary"
                className="text-xs py-1 px-2.5"
                onClick={handleStartEditing}
                leftIcon={<Edit3 size={13} />}
              >
                Edit Tags
              </Button>
            ) : (
              <Button
                variant="ghost"
                className="text-xs py-1 px-2.5 text-text-secondary"
                onClick={() => setIsEditing(false)}
                leftIcon={<RotateCcw size={13} />}
              >
                Cancel Edit
              </Button>
            )}
            <button
              type="button"
              onClick={closeSongInfoModal}
              className="p-1.5 rounded-full hover:bg-surface-3 text-text-secondary hover:text-text-main transition-colors"
              aria-label="Close dialog"
            >
              <X size={18} />
            </button>
          </div>
        </div>

        {/* Header Hero Section */}
        <div className="p-6 bg-gradient-to-b from-surface-2/80 to-surface-1 border-b border-surface-border/60">
          <div className="flex flex-col sm:flex-row items-center sm:items-start gap-5">
            {/* Cover Art */}
            <div className="w-28 h-28 sm:w-32 sm:h-32 rounded-xl shadow-lg overflow-hidden flex-shrink-0 bg-surface-3 relative group">
              {coverUrl ? (
                <img src={coverUrl} alt={song.album} className="w-full h-full object-cover" />
              ) : (
                <div
                  className="w-full h-full flex items-center justify-center font-bold text-3xl text-text-subtle"
                  style={{ background: generateGradient(song.album) }}
                >
                  {song.title.charAt(0)}
                </div>
              )}
              {song.spotifyId && song.isStreaming && (
                <div className="absolute top-2 left-2 px-2 py-0.5 rounded-full bg-black/70 backdrop-blur-md text-[10px] font-medium text-spotify flex items-center gap-1">
                  <Radio size={10} />
                  <span>Spotify</span>
                </div>
              )}
            </div>

            {/* Title & Core Metadata */}
            <div className="flex-1 min-w-0 text-center sm:text-left">
              <div className="flex items-center justify-center sm:justify-start gap-2 mb-1 flex-wrap">
                <h2 id="song-info-title" className="text-xl sm:text-2xl font-bold truncate text-text-main">
                  {song.title}
                </h2>
                <LikeButton songId={song.id} size={20} />
              </div>

              <p className="text-sm text-text-secondary font-medium truncate flex items-center justify-center sm:justify-start gap-1.5">
                <Mic2 size={14} className="text-brand flex-shrink-0" />
                <button
                  type="button"
                  onClick={() => {
                    closeSongInfoModal();
                    navigate(`/artists`);
                  }}
                  className="hover:underline hover:text-text-main text-left"
                >
                  {song.artist}
                </button>
              </p>

              <p className="text-xs text-text-subtle truncate mt-1 flex items-center justify-center sm:justify-start gap-1.5">
                <Disc size={13} className="flex-shrink-0" />
                <button
                  type="button"
                  onClick={() => {
                    closeSongInfoModal();
                    navigate(`/album/${encodeURIComponent(song.album)}`);
                  }}
                  className="hover:underline hover:text-text-secondary text-left"
                >
                  {song.album} {song.albumArtist && song.albumArtist !== song.artist ? `(${song.albumArtist})` : ''}
                </button>
              </p>

              {/* Year & Release Badge */}
              <div className="flex items-center justify-center sm:justify-start gap-2 mt-3 flex-wrap">
                {song.originalYear && (
                  <span
                    className="inline-flex items-center gap-1 px-2.5 py-0.5 rounded-full text-xs font-semibold bg-amber-500/15 text-amber-300 ring-1 ring-amber-500/30 cursor-pointer hover:bg-amber-500/25 transition-colors"
                    onClick={() => handleSearchFilter(String(song.originalYear))}
                    title="Original Release Year"
                  >
                    <Calendar size={11} />
                    Original: {song.originalYear}
                  </span>
                )}
                {song.year && (
                  <span
                    className="inline-flex items-center gap-1 px-2.5 py-0.5 rounded-full text-xs font-medium bg-surface-3 text-text-secondary ring-1 ring-surface-border cursor-pointer hover:bg-surface-hover hover:text-text-main transition-colors"
                    onClick={() => handleSearchFilter(String(song.year))}
                    title="Release / Remaster Year"
                  >
                    {song.originalYear ? `Release: ${song.year}` : `Year: ${song.year}`}
                  </span>
                )}
                <span className="inline-flex items-center gap-1 px-2.5 py-0.5 rounded-full text-xs font-mono bg-surface-3 text-text-secondary">
                  <Clock size={11} />
                  {formatTime(song.duration)}
                </span>
                {song.bpm && (
                  <span
                    className="inline-flex items-center gap-1 px-2.5 py-0.5 rounded-full text-xs font-bold bg-accent-pink/15 text-accent-pink ring-1 ring-accent-pink/30 cursor-pointer hover:bg-accent-pink/25 transition-colors"
                    onClick={() => handleSearchFilter(`${song.bpm} BPM`)}
                    title="Beats Per Minute"
                  >
                    <Activity size={11} />
                    {song.bpm} BPM
                  </span>
                )}
              </div>

              {/* Action Buttons */}
              <div className="flex items-center justify-center sm:justify-start gap-2 mt-4">
                <Button
                  variant="primary"
                  className="text-xs py-1.5 px-3"
                  onClick={() => {
                    playSong(song);
                    closeSongInfoModal();
                  }}
                  leftIcon={<Play size={14} className="fill-current" />}
                >
                  Play
                </Button>
                <Button
                  variant="secondary"
                  className="text-xs py-1.5 px-3"
                  onClick={() => {
                    addToQueue(song);
                    showToast({ type: 'success', message: `Added ${song.title} to queue` });
                  }}
                  leftIcon={<ListPlus size={14} />}
                >
                  Add to Queue
                </Button>
              </div>
            </div>
          </div>
        </div>

        {/* Navigation Tabs */}
        <div className="flex border-b border-surface-border bg-surface-2/40 px-6 gap-2">
          <button
            type="button"
            onClick={() => setActiveTab('overview')}
            className={`flex items-center gap-2 py-3 px-3 text-xs font-semibold border-b-2 transition-colors ${
              activeTab === 'overview'
                ? 'border-brand text-brand'
                : 'border-transparent text-text-secondary hover:text-text-main'
            }`}
          >
            <Music size={14} />
            Overview
          </button>
          <button
            type="button"
            onClick={() => setActiveTab('vibe')}
            className={`flex items-center gap-2 py-3 px-3 text-xs font-semibold border-b-2 transition-colors ${
              activeTab === 'vibe'
                ? 'border-brand text-brand'
                : 'border-transparent text-text-secondary hover:text-text-main'
            }`}
          >
            <Sparkles size={14} />
            AI & Vibe Profile
          </button>
          <button
            type="button"
            onClick={() => setActiveTab('stats')}
            className={`flex items-center gap-2 py-3 px-3 text-xs font-semibold border-b-2 transition-colors ${
              activeTab === 'stats'
                ? 'border-brand text-brand'
                : 'border-transparent text-text-secondary hover:text-text-main'
            }`}
          >
            <BarChart3 size={14} />
            Listening Stats
          </button>
          <button
            type="button"
            onClick={() => setActiveTab('technical')}
            className={`flex items-center gap-2 py-3 px-3 text-xs font-semibold border-b-2 transition-colors ${
              activeTab === 'technical'
                ? 'border-brand text-brand'
                : 'border-transparent text-text-secondary hover:text-text-main'
            }`}
          >
            <Sliders size={14} />
            Technical & File
          </button>
        </div>

        {/* Tab Contents Area */}
        <div className="p-6 overflow-y-auto flex-1 min-h-[220px] space-y-4">
          {/* TAB 1: OVERVIEW / EDIT FORM */}
          {activeTab === 'overview' && (
            isEditing ? (
              <div className="space-y-4 animate-in fade-in duration-150">
                <div className="flex items-center justify-between border-b border-surface-border/60 pb-2">
                  <span className="text-xs font-bold uppercase tracking-wider text-brand flex items-center gap-1.5">
                    <Edit3 size={14} />
                    Edit Song Metadata Tags
                  </span>
                  <span className="text-[10px] text-text-subtle">Saved to local database</span>
                </div>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-3 text-xs">
                  <div>
                    <label className="block text-text-subtle mb-1 font-semibold">Title *</label>
                    <input
                      type="text"
                      value={editTitle}
                      onChange={e => setEditTitle(e.target.value)}
                      className="w-full rounded-lg bg-surface-2 border border-surface-border px-3 py-2 text-text-main focus:outline-none focus:border-brand"
                    />
                  </div>

                  <div>
                    <label className="block text-text-subtle mb-1 font-semibold">Artist *</label>
                    <input
                      type="text"
                      value={editArtist}
                      onChange={e => setEditArtist(e.target.value)}
                      className="w-full rounded-lg bg-surface-2 border border-surface-border px-3 py-2 text-text-main focus:outline-none focus:border-brand"
                    />
                  </div>

                  <div>
                    <label className="block text-text-subtle mb-1 font-semibold">Album *</label>
                    <input
                      type="text"
                      value={editAlbum}
                      onChange={e => setEditAlbum(e.target.value)}
                      className="w-full rounded-lg bg-surface-2 border border-surface-border px-3 py-2 text-text-main focus:outline-none focus:border-brand"
                    />
                  </div>

                  <div>
                    <label className="block text-text-subtle mb-1 font-semibold">Album Artist</label>
                    <input
                      type="text"
                      value={editAlbumArtist}
                      onChange={e => setEditAlbumArtist(e.target.value)}
                      placeholder="Same as Artist if empty"
                      className="w-full rounded-lg bg-surface-2 border border-surface-border px-3 py-2 text-text-main focus:outline-none focus:border-brand"
                    />
                  </div>

                  <div>
                    <label className="block text-text-subtle mb-1 font-semibold">Track Number</label>
                    <input
                      type="number"
                      value={editTrackNumber}
                      onChange={e => setEditTrackNumber(e.target.value)}
                      placeholder="e.g. 1"
                      className="w-full rounded-lg bg-surface-2 border border-surface-border px-3 py-2 text-text-main focus:outline-none focus:border-brand font-mono"
                    />
                  </div>

                  <div>
                    <label className="block text-text-subtle mb-1 font-semibold">Disc Number</label>
                    <input
                      type="number"
                      value={editDiscNumber}
                      onChange={e => setEditDiscNumber(e.target.value)}
                      placeholder="e.g. 1"
                      className="w-full rounded-lg bg-surface-2 border border-surface-border px-3 py-2 text-text-main focus:outline-none focus:border-brand font-mono"
                    />
                  </div>

                  <div>
                    <label className="block text-text-subtle mb-1 font-semibold">Release Year</label>
                    <input
                      type="number"
                      value={editYear}
                      onChange={e => setEditYear(e.target.value)}
                      placeholder="e.g. 1999"
                      className="w-full rounded-lg bg-surface-2 border border-surface-border px-3 py-2 text-text-main focus:outline-none focus:border-brand font-mono"
                    />
                  </div>

                  <div>
                    <label className="block text-text-subtle mb-1 font-semibold">Genres (comma separated)</label>
                    <input
                      type="text"
                      value={editGenres}
                      onChange={e => setEditGenres(e.target.value)}
                      placeholder="e.g. Rock, Synthwave, 80s"
                      className="w-full rounded-lg bg-surface-2 border border-surface-border px-3 py-2 text-text-main focus:outline-none focus:border-brand"
                    />
                  </div>
                </div>

                <div className="flex items-center justify-end gap-2 pt-2 border-t border-surface-border/40">
                  <Button
                    variant="ghost"
                    className="text-xs py-1.5 px-3 text-text-secondary"
                    onClick={() => setIsEditing(false)}
                    disabled={isSaving}
                  >
                    Cancel
                  </Button>
                  <Button
                    variant="primary"
                    className="text-xs py-1.5 px-4"
                    onClick={handleSaveTags}
                    disabled={isSaving}
                    leftIcon={<Save size={14} />}
                  >
                    {isSaving ? 'Saving...' : 'Save Tags'}
                  </Button>
                </div>
              </div>
            ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="p-3.5 rounded-xl bg-surface-2/60 border border-surface-border/60">
                <span className="text-[11px] font-semibold uppercase tracking-wider text-text-subtle block mb-1">
                  Track Title
                </span>
                <span className="text-sm font-medium text-text-main">{song.title}</span>
              </div>

              <div className="p-3.5 rounded-xl bg-surface-2/60 border border-surface-border/60">
                <span className="text-[11px] font-semibold uppercase tracking-wider text-text-subtle block mb-1">
                  Artist
                </span>
                <span className="text-sm font-medium text-text-main">{song.artist}</span>
              </div>

              <div className="p-3.5 rounded-xl bg-surface-2/60 border border-surface-border/60">
                <span className="text-[11px] font-semibold uppercase tracking-wider text-text-subtle block mb-1">
                  Album
                </span>
                <span className="text-sm font-medium text-text-main">{song.album}</span>
              </div>

              <div className="p-3.5 rounded-xl bg-surface-2/60 border border-surface-border/60">
                <span className="text-[11px] font-semibold uppercase tracking-wider text-text-subtle block mb-1">
                  Album Artist
                </span>
                <span className="text-sm font-medium text-text-main">
                  {song.albumArtist || song.artist || 'N/A'}
                </span>
              </div>

              <div className="p-3.5 rounded-xl bg-surface-2/60 border border-surface-border/60 flex items-center justify-between">
                <div>
                  <span className="text-[11px] font-semibold uppercase tracking-wider text-text-subtle block mb-0.5">
                    Track / Disc Number
                  </span>
                  <span className="text-sm font-medium text-text-main font-mono">
                    Track {song.trackNumber || 1} {song.discNumber ? `(Disc ${song.discNumber})` : ''}
                  </span>
                </div>
              </div>

              <div className="p-3.5 rounded-xl bg-surface-2/60 border border-surface-border/60 flex items-center justify-between">
                <div>
                  <span className="text-[11px] font-semibold uppercase tracking-wider text-text-subtle block mb-0.5">
                    Duration
                  </span>
                  <span className="text-sm font-medium text-text-main font-mono">
                    {formatTime(song.duration)} ({Math.round(song.duration)}s)
                  </span>
                </div>
              </div>

              {/* Genres Section */}
              <div className="p-3.5 rounded-xl bg-surface-2/60 border border-surface-border/60 md:col-span-2">
                <span className="text-[11px] font-semibold uppercase tracking-wider text-text-subtle block mb-2 flex items-center gap-1.5">
                  <Tag size={12} className="text-brand" />
                  Genres
                </span>
                {song.genre && song.genre.length > 0 ? (
                  <div className="flex flex-wrap gap-2">
                    {song.genre.map((g, i) => (
                      <Chip
                        key={i}
                        accent="brand"
                        onClick={() => handleSearchFilter(g)}
                        title={`Filter library by genre "${g}"`}
                        className="cursor-pointer hover:scale-105"
                      >
                        {g}
                      </Chip>
                    ))}
                  </div>
                ) : (
                  <span className="text-xs text-text-subtle italic">No genres tagged. Run AI enrichment in Settings to detect genres automatically.</span>
                )}
              </div>
            </div>
            )
          )}

          {/* TAB 2: AI & VIBE PROFILE */}
          {activeTab === 'vibe' && (
            <div className="space-y-4">
              <div className="p-4 rounded-xl bg-surface-2/60 border border-surface-border/60">
                <div className="flex items-center gap-2 mb-3">
                  <Sparkles size={16} className="text-accent-pink" />
                  <span className="text-xs font-semibold uppercase tracking-wider text-text-main">
                    AI Music Intelligence (Gemini/LLM)
                  </span>
                  {song.moodAnalyzedAt && (
                    <span className="ml-auto text-[10px] text-text-subtle">
                      Analyzed {new Date(song.moodAnalyzedAt).toLocaleDateString()}
                    </span>
                  )}
                </div>

                <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
                  <div className="p-2.5 rounded-lg bg-surface-3/60 text-center">
                    <span className="text-[10px] uppercase text-text-subtle font-semibold block mb-1">Mood</span>
                    <span
                      onClick={() => song.mood && handleSearchFilter(song.mood)}
                      className={`text-xs font-bold capitalize ${
                        song.mood ? 'text-accent-pink cursor-pointer hover:underline' : 'text-text-subtle'
                      }`}
                    >
                      {song.mood || 'Not Analyzed'}
                    </span>
                  </div>

                  <div className="p-2.5 rounded-lg bg-surface-3/60 text-center">
                    <span className="text-[10px] uppercase text-text-subtle font-semibold block mb-1">Energy</span>
                    <span
                      onClick={() => song.energy && handleSearchFilter(song.energy)}
                      className={`text-xs font-bold capitalize ${
                        song.energy ? 'text-amber-400 cursor-pointer hover:underline' : 'text-text-subtle'
                      }`}
                    >
                      {song.energy || 'Not Analyzed'}
                    </span>
                  </div>

                  <div className="p-2.5 rounded-lg bg-surface-3/60 text-center">
                    <span className="text-[10px] uppercase text-text-subtle font-semibold block mb-1">Tempo</span>
                    <span
                      onClick={() => song.tempo && handleSearchFilter(song.tempo)}
                      className={`text-xs font-bold capitalize ${
                        song.tempo ? 'text-accent-blue cursor-pointer hover:underline' : 'text-text-subtle'
                      }`}
                    >
                      {song.tempo || 'Not Analyzed'}
                    </span>
                  </div>

                  <div className="p-2.5 rounded-lg bg-surface-3/60 text-center">
                    <span className="text-[10px] uppercase text-text-subtle font-semibold block mb-1">BPM</span>
                    <span
                      onClick={() => song.bpm && handleSearchFilter(`${song.bpm} BPM`)}
                      className={`text-xs font-bold font-mono ${
                        song.bpm ? 'text-accent-green cursor-pointer hover:underline' : 'text-text-subtle'
                      }`}
                    >
                      {song.bpm ? `${song.bpm} BPM` : 'N/A'}
                    </span>
                  </div>
                </div>

                {song.instrumental !== undefined && (
                  <div className="mt-3 flex items-center justify-between px-3 py-2 rounded-lg bg-surface-3/40 text-xs">
                    <span className="text-text-secondary">Vocal Profile:</span>
                    <span className="font-semibold text-text-main">
                      {song.instrumental ? '🎹 Instrumental (No Vocals)' : '🎤 Vocal Track'}
                    </span>
                  </div>
                )}
              </div>

              {/* Last.fm Tags */}
              <div className="p-4 rounded-xl bg-surface-2/60 border border-surface-border/60">
                <div className="flex items-center gap-2 mb-2">
                  <Tag size={14} className="text-brand" />
                  <span className="text-xs font-semibold uppercase tracking-wider text-text-main">
                    Last.fm Community Tags
                  </span>
                </div>
                {parsedLastFmTags.length > 0 ? (
                  <div className="flex flex-wrap gap-2 mt-2">
                    {parsedLastFmTags.map((tag, idx) => (
                      <Chip
                        key={idx}
                        accent="stats"
                        onClick={() => handleSearchFilter(tag)}
                        className="cursor-pointer hover:scale-105"
                      >
                        #{tag}
                      </Chip>
                    ))}
                  </div>
                ) : (
                  <span className="text-xs text-text-subtle italic">No community tags loaded. Enable Last.fm enrichment in Settings.</span>
                )}
              </div>
            </div>
          )}

          {/* TAB 3: LISTENING STATS */}
          {activeTab === 'stats' && (
            <div className="space-y-4">
              <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
                <div className="p-3.5 rounded-xl bg-surface-2/60 border border-surface-border/60 text-center">
                  <span className="text-[10px] font-semibold uppercase tracking-wider text-text-subtle block mb-1">
                    Plays
                  </span>
                  <span className="text-xl font-bold font-mono text-accent-green">{song.playCount || 0}</span>
                </div>

                <div className="p-3.5 rounded-xl bg-surface-2/60 border border-surface-border/60 text-center">
                  <span className="text-[10px] font-semibold uppercase tracking-wider text-text-subtle block mb-1">
                    Skips
                  </span>
                  <span className="text-xl font-bold font-mono text-amber-400">{song.skipCount || 0}</span>
                </div>

                <div className="p-3.5 rounded-xl bg-surface-2/60 border border-surface-border/60 text-center">
                  <span className="text-[10px] font-semibold uppercase tracking-wider text-text-subtle block mb-1 flex items-center justify-center gap-1">
                    <Percent size={10} /> Completion
                  </span>
                  <span className="text-xl font-bold font-mono text-brand">{completionRate}%</span>
                </div>

                <div className="p-3.5 rounded-xl bg-surface-2/60 border border-surface-border/60 text-center">
                  <span className="text-[10px] font-semibold uppercase tracking-wider text-text-subtle block mb-1">
                    Liked
                  </span>
                  <span className="text-sm font-bold text-text-main">
                    {song.liked ? '❤️ Yes' : '🤍 No'}
                  </span>
                </div>
              </div>

              <div className="p-4 rounded-xl bg-surface-2/60 border border-surface-border/60 space-y-3">
                <div className="flex justify-between items-center text-xs">
                  <span className="text-text-secondary flex items-center gap-1.5">
                    <Calendar size={13} /> Added to Library:
                  </span>
                  <span className="font-medium text-text-main font-mono">{formatDate(song.addedAt)}</span>
                </div>

                <div className="flex justify-between items-center text-xs border-t border-surface-border/40 pt-2">
                  <span className="text-text-secondary flex items-center gap-1.5">
                    <Clock size={13} /> Last Played:
                  </span>
                  <span className="font-medium text-text-main font-mono">{formatDate(song.lastPlayed)}</span>
                </div>

                {song.likedAt && (
                  <div className="flex justify-between items-center text-xs border-t border-surface-border/40 pt-2">
                    <span className="text-text-secondary flex items-center gap-1.5">
                      <Calendar size={13} /> Liked Date:
                    </span>
                    <span className="font-medium text-text-main font-mono">{formatDate(song.likedAt)}</span>
                  </div>
                )}
              </div>

              {/* Last.fm Global Stats */}
              {(song.lastfmListeners !== undefined || song.lastfmPlaycount !== undefined || song.lastfmUrl) && (
                <div className="p-4 rounded-xl bg-surface-2/60 border border-surface-border/60 space-y-3">
                  <div className="flex items-center justify-between">
                    <span className="text-xs font-semibold uppercase tracking-wider text-text-main flex items-center gap-1.5">
                      <Radio size={14} className="text-red-400" />
                      Last.fm Global Audience
                    </span>
                    {song.lastfmUrl && (
                      <a
                        href={song.lastfmUrl}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="text-xs text-brand hover:underline flex items-center gap-1"
                      >
                        Last.fm Page <ExternalLink size={11} />
                      </a>
                    )}
                  </div>

                  <div className="grid grid-cols-2 gap-3 text-center pt-1">
                    <div className="p-2 rounded-lg bg-surface-3/50">
                      <span className="text-[10px] text-text-subtle uppercase block">Global Listeners</span>
                      <span className="text-sm font-bold font-mono text-text-main">
                        {song.lastfmListeners ? song.lastfmListeners.toLocaleString() : 'N/A'}
                      </span>
                    </div>

                    <div className="p-2 rounded-lg bg-surface-3/50">
                      <span className="text-[10px] text-text-subtle uppercase block">Global Scrobbles</span>
                      <span className="text-sm font-bold font-mono text-text-main">
                        {song.lastfmPlaycount ? song.lastfmPlaycount.toLocaleString() : 'N/A'}
                      </span>
                    </div>
                  </div>
                </div>
              )}
            </div>
          )}

          {/* TAB 4: TECHNICAL & FILE */}
          {activeTab === 'technical' && (
            <div className="space-y-4">
              <div className="p-4 rounded-xl bg-surface-2/60 border border-surface-border/60 space-y-3">
                <span className="text-xs font-semibold uppercase tracking-wider text-text-main block">
                  File & Storage Location
                </span>

                <div className="flex items-center gap-2 p-2.5 rounded-lg bg-surface-3/80 font-mono text-xs text-text-secondary overflow-x-auto">
                  <span className="truncate flex-1">{song.path || song.url || 'No physical path'}</span>
                  <button
                    type="button"
                    onClick={handleCopyPath}
                    className="p-1 rounded hover:bg-surface-border text-text-subtle hover:text-text-main transition-colors flex-shrink-0"
                    title="Copy path to clipboard"
                  >
                    {copiedPath ? <Check size={14} className="text-accent-green" /> : <Copy size={14} />}
                  </button>
                </div>
              </div>

              {/* ReplayGain & Loudness Specs */}
              <div className="p-4 rounded-xl bg-surface-2/60 border border-surface-border/60 space-y-3">
                <div className="flex items-center gap-2">
                  <Volume2 size={16} className="text-brand" />
                  <span className="text-xs font-semibold uppercase tracking-wider text-text-main">
                    ReplayGain & Loudness Normalization
                  </span>
                </div>

                <div className="grid grid-cols-2 gap-3 text-center">
                  <div className="p-3 rounded-lg bg-surface-3/50">
                    <span className="text-[10px] text-text-subtle uppercase block mb-0.5">Gain Offset</span>
                    <span className="text-sm font-bold font-mono text-text-main">
                      {song.replayGainDb !== undefined ? `${song.replayGainDb.toFixed(2)} dB` : 'None / Not Scanned'}
                    </span>
                  </div>

                  <div className="p-3 rounded-lg bg-surface-3/50">
                    <span className="text-[10px] text-text-subtle uppercase block mb-0.5">Peak Amplitude</span>
                    <span className="text-sm font-bold font-mono text-text-main">
                      {song.replayPeak !== undefined ? song.replayPeak.toFixed(4) : 'None / Not Scanned'}
                    </span>
                  </div>
                </div>
              </div>

              {/* Technical Identifiers */}
              <div className="p-4 rounded-xl bg-surface-2/60 border border-surface-border/60 space-y-2 text-xs">
                <span className="text-[11px] font-semibold uppercase tracking-wider text-text-main block mb-2">
                  System Identifiers
                </span>

                <div className="flex justify-between items-center py-1 border-b border-surface-border/40">
                  <span className="text-text-subtle">Internal Track ID:</span>
                  <span className="font-mono text-text-secondary select-all">{song.id}</span>
                </div>

                {song.spotifyId && (
                  <div className="flex justify-between items-center py-1 border-b border-surface-border/40">
                    <span className="text-text-subtle">Spotify Track ID:</span>
                    <span className="font-mono text-spotify select-all">{song.spotifyId}</span>
                  </div>
                )}

                {song.lastfmMbid && (
                  <div className="flex justify-between items-center py-1 border-b border-surface-border/40">
                    <span className="text-text-subtle">MusicBrainz ID (MBID):</span>
                    <span className="font-mono text-text-secondary select-all">{song.lastfmMbid}</span>
                  </div>
                )}

                {song.fileHash && (
                  <div className="flex justify-between items-center py-1">
                    <span className="text-text-subtle">File Hash:</span>
                    <span className="font-mono text-text-secondary select-all truncate max-w-[250px]">{song.fileHash}</span>
                  </div>
                )}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
