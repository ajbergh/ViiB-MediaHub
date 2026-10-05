/** Defines song columns, saved column preferences, and displayed values. */

import { Song } from '../types';
import { TrackAnalysisFeature } from '../services/trackAnalysisContracts';
import { formatTime } from '../utils';

export const SONG_COLUMNS = [
  { id: 'album', label: 'Album', width: 'minmax(130px, 3fr)', minWidth: 130 },
  { id: 'artist', label: 'Artist', width: 'minmax(130px, 3fr)', minWidth: 130 },
  { id: 'plays', label: 'Plays', width: '60px', minWidth: 60 },
  { id: 'duration', label: 'Duration', width: '64px', minWidth: 64 },
  { id: 'bpm', label: 'BPM', width: '72px', minWidth: 72 },
  { id: 'key', label: 'Key', width: '96px', minWidth: 96 },
  { id: 'camelot', label: 'Camelot Key', width: '100px', minWidth: 100 },
  { id: 'genre', label: 'Genre', width: '140px', minWidth: 140 },
  { id: 'year', label: 'Year', width: '64px', minWidth: 64 },
  { id: 'mood', label: 'Mood', width: '100px', minWidth: 100 },
  { id: 'energy', label: 'Energy', width: '80px', minWidth: 80 },
  { id: 'trackNumber', label: 'Track #', width: '72px', minWidth: 72 },
  { id: 'discNumber', label: 'Disc #', width: '64px', minWidth: 64 },
  { id: 'added', label: 'Date Added', width: '112px', minWidth: 112 },
  { id: 'lastPlayed', label: 'Last Played', width: '112px', minWidth: 112 },
  { id: 'source', label: 'Source', width: '120px', minWidth: 120 },
] as const;
export type SongColumnId = typeof SONG_COLUMNS[number]['id'];
export const SONG_COLUMNS_STORAGE_KEY = 'viib.songs.columns';
export const DEFAULT_SONG_COLUMNS: SongColumnId[] = ['album', 'artist', 'plays', 'duration'];
export function loadSongColumns(): SongColumnId[] {
  try {
    const stored: unknown = JSON.parse(localStorage.getItem(SONG_COLUMNS_STORAGE_KEY) ?? 'null');
    if (Array.isArray(stored)) return SONG_COLUMNS.filter(column => stored.includes(column.id)).map(column => column.id);
  } catch { /* Invalid or unavailable storage uses the default layout. */ }
  return [...DEFAULT_SONG_COLUMNS];
}
const date = (value?: number) => value && Number.isFinite(value) ? new Date(value).toLocaleDateString() : '—';
export function songColumnValue(id: SongColumnId, song: Song, analysis?: TrackAnalysisFeature): string {
  switch (id) {
    case 'album': return song.album || '—';
    case 'artist': return song.artist || '—';
    case 'plays': return String(song.playCount ?? 0);
    case 'duration': return formatTime(song.duration);
    case 'bpm': { const bpm = analysis?.bpm ?? song.bpm; return bpm && Number.isFinite(bpm) ? String(Math.round(bpm * 10) / 10) : '—'; }
    case 'key': return analysis?.key || '—';
    case 'camelot': return analysis?.camelotKey || '—';
    case 'genre': return song.genre?.join(', ') || '—';
    case 'year': return String(song.originalYear || song.year || '—');
    case 'mood': return song.mood || '—';
    case 'energy': return analysis?.energyLevel != null ? String(analysis.energyLevel) : song.energy || '—';
    case 'trackNumber': return String(song.trackNumber || '—');
    case 'discNumber': return String(song.discNumber || '—');
    case 'added': return date(song.addedAt);
    case 'lastPlayed': return date(song.lastPlayed);
    case 'source': return song.sourceName || (song.source === 'plex' ? 'Plex' : song.isStreaming ? 'Spotify' : 'Local');
  }
}
