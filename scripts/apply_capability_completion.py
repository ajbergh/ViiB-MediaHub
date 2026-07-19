#!/usr/bin/env python3
"""Complete ReplayGain, duplicate management, and M3U playlist capabilities."""
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[1]


def read(path: str) -> str:
    return (ROOT / path).read_text(encoding="utf-8")


def write(path: str, content: str) -> None:
    target = ROOT / path
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(content, encoding="utf-8")


def rewrite(path: str, transform) -> None:
    before = read(path)
    after = transform(before)
    if after != before:
        write(path, after)


def replace_once(text: str, old: str, new: str) -> str:
    if old in text:
        return text.replace(old, new, 1)
    return text


def block_replace(text: str, start: str, end: str, old: str, new: str) -> str:
    start_index = text.find(start)
    if start_index < 0:
        return text
    end_index = text.find(end, start_index + len(start))
    if end_index < 0:
        return text
    block = text[start_index:end_index]
    if old not in block:
        return text
    block = block.replace(old, new, 1)
    return text[:start_index] + block + text[end_index:]


# ---------------------------------------------------------------------------
# ReplayGain persistence and playback
# ---------------------------------------------------------------------------

def fix_db_replaygain(text: str) -> str:
    text = replace_once(
        text,
        '\tDuration       float64  `json:"duration"`\n\tFilePath',
        '\tDuration       float64  `json:"duration"`\n\tReplayGainDB   float64  `json:"replayGainDb,omitempty"`\n\tReplayPeak     float64  `json:"replayPeak,omitempty"`\n\tFilePath',
    )

    migration_marker = '\t// Migration: Add Last.FM enrichment columns to songs table\n'
    if 'replay_gain_db' not in text and migration_marker in text:
        migration = '''\t// Migration: Add ReplayGain metadata used by the normalization audio stage.
\treplayGainMigrations := []string{
\t\t`ALTER TABLE songs ADD COLUMN replay_gain_db REAL`,
\t\t`ALTER TABLE songs ADD COLUMN replay_peak REAL`,
\t\t`ALTER TABLE songs ADD COLUMN ignored INTEGER DEFAULT 0`,
\t}
\tfor _, m := range replayGainMigrations {
\t\tif _, err := d.conn.Exec(m); err != nil && !strings.Contains(err.Error(), "duplicate column") {
\t\t\treturn err
\t\t}
\t}
\tif _, err := d.conn.Exec(`CREATE INDEX IF NOT EXISTS idx_songs_ignored ON songs(ignored)`); err != nil {
\t\treturn err
\t}

'''
        text = text.replace(migration_marker, migration + migration_marker, 1)

    text = block_replace(
        text,
        'func (d *DB) GetAllSongs()',
        '// SaveSong inserts',
        'duration, file_path, cover_path, added_at, play_count, last_played,',
        'duration, replay_gain_db, replay_peak, file_path, cover_path, added_at, play_count, last_played,',
    )
    text = block_replace(
        text,
        'func (d *DB) GetAllSongs()',
        '// SaveSong inserts',
        '\t\tFROM songs\n\t\tORDER BY',
        '\t\tFROM songs\n\t\tWHERE COALESCE(ignored, 0) = 0\n\t\tORDER BY',
    )
    text = block_replace(
        text,
        'func (d *DB) GetAllSongs()',
        '// SaveSong inserts',
        '\t\tvar liked, likedAt, lastFMListeners, lastFMPlaycount, lastFMEnrichedAt sql.NullInt64\n',
        '\t\tvar liked, likedAt, lastFMListeners, lastFMPlaycount, lastFMEnrichedAt sql.NullInt64\n\t\tvar replayGainDB, replayPeak sql.NullFloat64\n',
    )
    text = block_replace(
        text,
        'func (d *DB) GetAllSongs()',
        '// SaveSong inserts',
        '&s.Duration, &s.FilePath, &coverPath, &s.AddedAt, &playCount, &lastPlayed,',
        '&s.Duration, &replayGainDB, &replayPeak, &s.FilePath, &coverPath, &s.AddedAt, &playCount, &lastPlayed,',
    )
    assignment_marker = '''\t\tif yearAnalyzedAt.Valid {
\t\t\ts.YearAnalyzedAt = yearAnalyzedAt.Int64
\t\t}
'''
    if 's.ReplayGainDB = replayGainDB.Float64' not in text:
        text = block_replace(
            text,
            'func (d *DB) GetAllSongs()',
            '// SaveSong inserts',
            assignment_marker,
            assignment_marker + '''\t\tif replayGainDB.Valid {
\t\t\ts.ReplayGainDB = replayGainDB.Float64
\t\t}
\t\tif replayPeak.Valid {
\t\t\ts.ReplayPeak = replayPeak.Float64
\t\t}
''',
        )

    text = block_replace(
        text,
        'func (d *DB) SaveSong',
        '// SaveSongs inserts',
        'genre, year, duration, file_path, cover_path, added_at,',
        'genre, year, duration, replay_gain_db, replay_peak, file_path, cover_path, added_at,',
    )
    text = block_replace(
        text,
        'func (d *DB) SaveSong',
        '// SaveSongs inserts',
        ') VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)',
        ') VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)',
    )
    text = block_replace(
        text,
        'func (d *DB) SaveSong',
        '// SaveSongs inserts',
        '\t\t\tduration = excluded.duration,\n',
        '\t\t\tduration = excluded.duration,\n\t\t\treplay_gain_db = excluded.replay_gain_db,\n\t\t\treplay_peak = excluded.replay_peak,\n',
    )
    text = block_replace(
        text,
        'func (d *DB) SaveSong',
        '// SaveSongs inserts',
        'string(genreJSON), s.Year, s.Duration, s.FilePath, s.CoverPath, s.AddedAt,',
        'string(genreJSON), s.Year, s.Duration, s.ReplayGainDB, s.ReplayPeak, s.FilePath, s.CoverPath, s.AddedAt,',
    )

    text = block_replace(
        text,
        'func (d *DB) SaveSongs',
        '// DeleteSong removes',
        'genre, year, duration, file_path, cover_path, added_at,',
        'genre, year, duration, replay_gain_db, replay_peak, file_path, cover_path, added_at,',
    )
    text = block_replace(
        text,
        'func (d *DB) SaveSongs',
        '// DeleteSong removes',
        ') VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)',
        ') VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)',
    )
    text = block_replace(
        text,
        'func (d *DB) SaveSongs',
        '// DeleteSong removes',
        '\t\t\tduration = excluded.duration,\n',
        '\t\t\tduration = excluded.duration,\n\t\t\treplay_gain_db = excluded.replay_gain_db,\n\t\t\treplay_peak = excluded.replay_peak,\n',
    )
    text = block_replace(
        text,
        'func (d *DB) SaveSongs',
        '// DeleteSong removes',
        'string(genreJSON), s.Year, s.Duration, s.FilePath, s.CoverPath, s.AddedAt,',
        'string(genreJSON), s.Year, s.Duration, s.ReplayGainDB, s.ReplayPeak, s.FilePath, s.CoverPath, s.AddedAt,',
    )
    return text


def fix_scanner_replaygain(text: str) -> str:
    text = replace_once(
        text,
        '\tDuration    float64\n\tFilePath',
        '\tDuration    float64\n\tReplayGainDB float64\n\tReplayPeak   float64\n\tFilePath',
    )
    text = replace_once(
        text,
        '\t\t\tDuration:    song.Duration,\n\t\t\tFilePath:',
        '\t\t\tDuration:    song.Duration,\n\t\t\tReplayGainDB: song.ReplayGainDB,\n\t\t\tReplayPeak:   song.ReplayPeak,\n\t\t\tFilePath:',
    )

    helper_marker = '''\t\tgetTag := func(key string) string {
\t\t\tif vals, ok := tags[key]; ok && len(vals) > 0 {
\t\t\t\treturn vals[0]
\t\t\t}
\t\t\treturn ""
\t\t}
'''
    if 'getTagFold := func' not in text and helper_marker in text:
        helpers = helper_marker + '''
\t\tgetTagFold := func(keys ...string) string {
\t\t\tfor actualKey, values := range tags {
\t\t\t\tfor _, key := range keys {
\t\t\t\t\tif strings.EqualFold(actualKey, key) && len(values) > 0 {
\t\t\t\t\t\treturn values[0]
\t\t\t\t\t}
\t\t\t\t}
\t\t\t}
\t\t\treturn ""
\t\t}
\t\tparseReplayValue := func(value string) float64 {
\t\t\tvalue = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(value, " dB"), "dB"))
\t\t\tparsed, _ := strconv.ParseFloat(value, 64)
\t\t\treturn parsed
\t\t}
'''
        text = text.replace(helper_marker, helpers, 1)

    song_marker = '''\t\tsong := &SongMetadata{
\t\t\tID:       id,
\t\t\tFileHash: fingerprint,
\t\t\tTitle:    getTag(taglib.Title),
\t\t\tArtist:   getTag(taglib.Artist),
\t\t\tAlbum:    getTag(taglib.Album),
\t\t\tDuration: float64(props.Length) / float64(time.Second),
\t\t\tFilePath: filePath,
\t\t}
'''
    if 'REPLAYGAIN_TRACK_GAIN' not in text and song_marker in text:
        replay = song_marker + '''
\t\tif value := getTagFold("REPLAYGAIN_TRACK_GAIN"); value != "" {
\t\t\tsong.ReplayGainDB = parseReplayValue(value)
\t\t} else if value := getTagFold("R128_TRACK_GAIN"); value != "" {
\t\t\t// Opus R128 gain is stored in Q7.8 dB units.
\t\t\tsong.ReplayGainDB = parseReplayValue(value) / 256
\t\t}
\t\tif value := getTagFold("REPLAYGAIN_TRACK_PEAK"); value != "" {
\t\t\tsong.ReplayPeak = parseReplayValue(value)
\t\t}
'''
        text = text.replace(song_marker, replay, 1)

    ignored_marker = '\tresult := &ScanResult{}\n\tvar songs []db.Song\n\tvar scannedPaths []string\n'
    if 'ignoredPathSet' not in text and ignored_marker in text:
        ignored = ignored_marker + '''\tignoredPathSet := make(map[string]struct{})
\tif ignoredPaths, err := s.db.GetIgnoredFilePaths(); err == nil {
\t\tfor _, ignoredPath := range ignoredPaths {
\t\t\tignoredPathSet[filepath.Clean(ignoredPath)] = struct{}{}
\t\t}
\t}
'''
        text = text.replace(ignored_marker, ignored, 1)

    extension_marker = '''\t\text := strings.ToLower(filepath.Ext(path))
\t\tif !supportedExtensions[ext] {
\t\t\treturn nil
\t\t}
'''
    if 'ignoredPathSet[filepath.Clean(path)]' not in text and extension_marker in text:
        text = text.replace(extension_marker, extension_marker + '''\t\tif _, ignored := ignoredPathSet[filepath.Clean(path)]; ignored {
\t\t\treturn nil
\t\t}
''', 1)
    return text


def fix_frontend_replaygain_types(text: str) -> str:
    text = replace_once(text, '  duration: number; // in seconds\n', '  duration: number; // in seconds\n  replayGainDb?: number;\n  replayPeak?: number;\n')
    return text


def fix_api_song_type(text: str) -> str:
    return replace_once(text, '  duration: number;\n', '  duration: number;\n  replayGainDb?: number;\n  replayPeak?: number;\n')


def fix_backend_mapping(text: str) -> str:
    return replace_once(text, '    duration: apiSong.duration,\n', '    duration: apiSong.duration,\n    replayGainDb: apiSong.replayGainDb,\n    replayPeak: apiSong.replayPeak,\n')


def fix_playback_lifecycle(text: str) -> str:
    if 'calculateReplayGain' not in text:
        text += '''

export const calculateReplayGain = (gainDb?: number, peak?: number): number => {
  if (!Number.isFinite(gainDb)) return 1;
  const clampedDb = Math.max(-24, Math.min(12, gainDb ?? 0));
  let linear = Math.pow(10, clampedDb / 20);
  if (Number.isFinite(peak) && (peak ?? 0) > 0 && linear * (peak ?? 0) > 1) {
    linear = 1 / (peak ?? 1);
  }
  return Math.max(0.05, Math.min(4, linear));
};
'''
    return text


def fix_playback_lifecycle_test(text: str) -> str:
    text = text.replace(
        "import { ManagedObjectUrlRegistry, normalizeCrossfadeDuration } from './playbackLifecycle';",
        "import { calculateReplayGain, ManagedObjectUrlRegistry, normalizeCrossfadeDuration } from './playbackLifecycle';",
    )
    if "describe('calculateReplayGain'" not in text:
        text += '''

describe('calculateReplayGain', () => {
  it('converts dB to linear gain', () => {
    expect(calculateReplayGain(-6)).toBeCloseTo(0.501, 2);
  });
  it('limits gain to prevent clipping when peak metadata is present', () => {
    expect(calculateReplayGain(6, 0.8)).toBeCloseTo(1.25, 4);
  });
  it('returns unity without metadata', () => {
    expect(calculateReplayGain()).toBe(1);
  });
});
'''
    return text


def fix_audio_engine(text: str) -> str:
    if 'private normalizationGain' not in text:
        text = text.replace(
            '  private masterGain: GainNode | null = null;\n',
            '  private masterGain: GainNode | null = null;\n  private normalizationGain: GainNode | null = null;\n',
        )
        text = text.replace(
            '    this.masterGain = this.context.createGain();\n    this.masterGain.gain.value = 1;\n',
            '    this.masterGain = this.context.createGain();\n    this.masterGain.gain.value = 1;\n    this.normalizationGain = this.context.createGain();\n    this.normalizationGain.gain.value = 1;\n',
        )
        text = text.replace(
            '    // 3. Link Analyser to Master\n    this.analyser.connect(this.masterGain);\n\n    // 4. Link Master to Dest\n',
            '    // 3. Link Analyser through track normalization to Master\n    this.analyser.connect(this.normalizationGain);\n    this.normalizationGain.connect(this.masterGain);\n\n    // 4. Link Master to Dest\n',
        )
        text = text.replace(
            '  setEqBands(gains: number[]) {\n',
            '''  setNormalizationGain(linearGain: number) {
    if (!this.normalizationGain || !this.context) return;
    const safeGain = Number.isFinite(linearGain) ? Math.max(0.05, Math.min(4, linearGain)) : 1;
    this.normalizationGain.gain.cancelScheduledValues(this.context.currentTime);
    this.normalizationGain.gain.setTargetAtTime(safeGain, this.context.currentTime, 0.05);
  }

  setEqBands(gains: number[]) {
''',
        )
    return text


def fix_audio_hook_normalization(text: str) -> str:
    text = text.replace(
        "import { isActivePlaybackEvent, normalizeCrossfadeDuration } from '../lib/playbackLifecycle';",
        "import { calculateReplayGain, isActivePlaybackEvent, normalizeCrossfadeDuration } from '../lib/playbackLifecycle';",
    )
    marker = '''    useEffect(() => {
        audioEngine.setVolume(volume);
    }, [volume]);
'''
    if 'audioEngine.setNormalizationGain' not in text and marker in text:
        text = text.replace(marker, marker + '''
    useEffect(() => {
        const gain = audioSettings.normalization
            ? calculateReplayGain(currentSong?.replayGainDb, currentSong?.replayPeak)
            : 1;
        audioEngine.setNormalizationGain(gain);
    }, [audioSettings.normalization, currentSong?.id, currentSong?.replayGainDb, currentSong?.replayPeak]);
''', 1)
    return text


# ---------------------------------------------------------------------------
# Duplicate management
# ---------------------------------------------------------------------------

write(
    'backend/internal/db/duplicates.go',
    r'''package db

import "database/sql"

// DuplicateGroup represents active library entries with the same media fingerprint.
type DuplicateGroup struct {
    FileHash string `json:"fileHash"`
    Songs    []Song `json:"songs"`
}

func (d *DB) GetDuplicateGroups() ([]DuplicateGroup, error) {
    songs, err := d.GetAllSongs()
    if err != nil {
        return nil, err
    }
    grouped := make(map[string][]Song)
    for _, song := range songs {
        if song.FileHash != "" {
            grouped[song.FileHash] = append(grouped[song.FileHash], song)
        }
    }
    result := make([]DuplicateGroup, 0)
    for hash, candidates := range grouped {
        if len(candidates) > 1 {
            result = append(result, DuplicateGroup{FileHash: hash, Songs: candidates})
        }
    }
    return result, nil
}

func (d *DB) SetSongIgnored(id string, ignored bool) error {
    value := 0
    if ignored {
        value = 1
    }
    result, err := d.conn.Exec(`UPDATE songs SET ignored = ? WHERE id = ?`, value, id)
    if err != nil {
        return err
    }
    affected, err := result.RowsAffected()
    if err != nil {
        return err
    }
    if affected == 0 {
        return sql.ErrNoRows
    }
    return nil
}

func (d *DB) GetIgnoredSongs() ([]Song, error) {
    rows, err := d.conn.Query(`SELECT id FROM songs WHERE COALESCE(ignored, 0) = 1 ORDER BY artist, album, title`)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    songs := make([]Song, 0)
    for rows.Next() {
        var id string
        if err := rows.Scan(&id); err != nil {
            return nil, err
        }
        song, err := d.GetSongByID(id)
        if err != nil {
            return nil, err
        }
        songs = append(songs, *song)
    }
    return songs, rows.Err()
}

func (d *DB) GetIgnoredFilePaths() ([]string, error) {
    rows, err := d.conn.Query(`SELECT file_path FROM songs WHERE COALESCE(ignored, 0) = 1`)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    paths := make([]string, 0)
    for rows.Next() {
        var path string
        if err := rows.Scan(&path); err != nil {
            return nil, err
        }
        paths = append(paths, path)
    }
    return paths, rows.Err()
}
''',
)

write(
    'backend/internal/api/duplicates.go',
    r'''package api

import (
    "database/sql"
    "encoding/json"
    "net/http"

    "github.com/ajbergh/viib-mediahub/internal/scanner"
)

func (a *API) getDuplicateGroups(w http.ResponseWriter, r *http.Request) {
    groups, err := a.db.GetDuplicateGroups()
    if err != nil {
        respondError(w, http.StatusInternalServerError, err.Error())
        return
    }
    if groups == nil {
        groups = []db.DuplicateGroup{}
    }
    respondJSON(w, groups)
}

func (a *API) getIgnoredSongs(w http.ResponseWriter, r *http.Request) {
    songs, err := a.db.GetIgnoredSongs()
    if err != nil {
        respondError(w, http.StatusInternalServerError, err.Error())
        return
    }
    for i := range songs {
        songs[i].FilePath = "/api/audio/" + songs[i].ID
        if songs[i].CoverPath != "" {
            songs[i].CoverPath = "/api/cover/" + songs[i].ID
        }
    }
    respondJSON(w, songs)
}

func (a *API) setDuplicateIgnored(w http.ResponseWriter, r *http.Request) {
    var request struct {
        SongID  string `json:"songId"`
        Ignored bool   `json:"ignored"`
    }
    if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.SongID == "" {
        respondError(w, http.StatusBadRequest, "songId is required")
        return
    }
    if err := a.db.SetSongIgnored(request.SongID, request.Ignored); err != nil {
        if err == sql.ErrNoRows {
            respondError(w, http.StatusNotFound, "Song not found")
        } else {
            respondError(w, http.StatusInternalServerError, err.Error())
        }
        return
    }
    if a.scanner != nil {
        a.scanner.EmitEvent(scanner.LibraryEvent{Type: "library_updated", Message: "Duplicate visibility updated"})
    }
    respondJSON(w, map[string]any{"status": "ok", "ignored": request.Ignored})
}
'''.replace('"github.com/ajbergh/viib-mediahub/internal/scanner"', '"github.com/ajbergh/viib-mediahub/internal/db"\n    "github.com/ajbergh/viib-mediahub/internal/scanner"'),
)

write(
    'pages/Duplicates.tsx',
    r'''import React, { useCallback, useEffect, useState } from 'react';
import { Copy, Eye, EyeOff, RefreshCw } from 'lucide-react';
import { api, DuplicateGroup } from '../services/api';
import { useStore } from '../store';
import { Page, PageHeader } from '../components/ui/Page';
import { Button } from '../components/ui/Button';

export const Duplicates: React.FC = () => {
  const refreshLibrary = useStore(state => state.refreshLibrary);
  const [groups, setGroups] = useState<DuplicateGroup[]>([]);
  const [ignored, setIgnored] = useState<DuplicateGroup['songs']>([]);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [duplicateGroups, ignoredSongs] = await Promise.all([
        api.getDuplicateGroups(),
        api.getIgnoredSongs(),
      ]);
      setGroups(duplicateGroups);
      setIgnored(ignoredSongs);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void load(); }, [load]);

  const setIgnoredState = async (songId: string, value: boolean) => {
    await api.setDuplicateIgnored(songId, value);
    await Promise.all([load(), refreshLibrary()]);
  };

  return (
    <Page>
      <PageHeader
        heading="Duplicate Manager"
        subheading="Hide redundant library copies without deleting the source files or losing metadata."
        actions={<Button variant="secondary" onClick={() => void load()}><RefreshCw size={16} /> Refresh</Button>}
      />

      {loading ? (
        <div className="text-text-secondary">Scanning duplicate fingerprints…</div>
      ) : groups.length === 0 ? (
        <div className="rounded-xl border border-surface-border bg-surface-2 p-8 text-center">
          <Copy className="mx-auto mb-3 text-brand" size={36} />
          <h2 className="font-semibold text-text-main">No active duplicates found</h2>
          <p className="mt-2 text-sm text-text-secondary">Copies are grouped by their stable media fingerprint.</p>
        </div>
      ) : (
        <div className="space-y-6">
          {groups.map(group => (
            <section key={group.fileHash} className="rounded-xl border border-surface-border bg-surface-2 p-5">
              <h2 className="mb-4 font-semibold text-text-main">{group.songs.length} identical copies</h2>
              <div className="space-y-3">
                {group.songs.map((song, index) => (
                  <div key={song.id} className="flex items-center gap-4 rounded-lg bg-surface-1 p-3">
                    <div className="min-w-0 flex-1">
                      <div className="truncate font-medium text-text-main">{song.title}</div>
                      <div className="truncate text-sm text-text-secondary">{song.artist} · {song.album}</div>
                      <div className="truncate text-xs text-text-subtle">{song.sourcePath || song.filePath}</div>
                    </div>
                    {index === 0 ? (
                      <span className="rounded-full bg-brand/15 px-3 py-1 text-xs font-semibold text-brand">Suggested keep</span>
                    ) : (
                      <Button variant="secondary" onClick={() => void setIgnoredState(song.id, true)}>
                        <EyeOff size={15} /> Hide copy
                      </Button>
                    )}
                  </div>
                ))}
              </div>
            </section>
          ))}
        </div>
      )}

      {ignored.length > 0 && (
        <section className="mt-8 rounded-xl border border-surface-border bg-surface-2 p-5">
          <h2 className="mb-4 font-semibold text-text-main">Hidden duplicate copies</h2>
          <div className="space-y-3">
            {ignored.map(song => (
              <div key={song.id} className="flex items-center gap-4 rounded-lg bg-surface-1 p-3">
                <div className="min-w-0 flex-1">
                  <div className="truncate font-medium text-text-main">{song.title}</div>
                  <div className="truncate text-sm text-text-secondary">{song.artist} · {song.album}</div>
                </div>
                <Button variant="secondary" onClick={() => void setIgnoredState(song.id, false)}>
                  <Eye size={15} /> Restore
                </Button>
              </div>
            ))}
          </div>
        </section>
      )}
    </Page>
  );
};
''',
)


# ---------------------------------------------------------------------------
# M3U playlist import/export
# ---------------------------------------------------------------------------

write(
    'backend/internal/db/playlist_io.go',
    r'''package db

import (
    "database/sql"
    "encoding/json"
)

func (d *DB) GetPlaylistByID(id string) (*Playlist, error) {
    var playlist Playlist
    var songIDs string
    var coverPath sql.NullString
    if err := d.conn.QueryRow(`SELECT id, name, song_ids, cover_path, created_at FROM playlists WHERE id = ?`, id).Scan(
        &playlist.ID, &playlist.Name, &songIDs, &coverPath, &playlist.CreatedAt,
    ); err != nil {
        return nil, err
    }
    if err := json.Unmarshal([]byte(songIDs), &playlist.SongIDs); err != nil {
        return nil, err
    }
    if coverPath.Valid {
        playlist.CoverPath = coverPath.String
    }
    return &playlist, nil
}
''',
)

write(
    'backend/internal/api/playlist_io.go',
    r'''package api

import (
    "bufio"
    "database/sql"
    "encoding/json"
    "fmt"
    "net/http"
    "path/filepath"
    "runtime"
    "strings"
    "time"

    "github.com/ajbergh/viib-mediahub/internal/db"
    "github.com/go-chi/chi/v5"
)

func normalizePlaylistPath(path string) string {
    path = filepath.Clean(strings.TrimSpace(path))
    if runtime.GOOS == "windows" {
        return strings.ToLower(path)
    }
    return path
}

func parseM3U(content string) []string {
    scanner := bufio.NewScanner(strings.NewReader(content))
    scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
    paths := make([]string, 0)
    for scanner.Scan() {
        line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\ufeff"))
        if line == "" || strings.HasPrefix(line, "#") {
            continue
        }
        paths = append(paths, line)
    }
    return paths
}

func safePlaylistFilename(name string) string {
    name = strings.TrimSpace(name)
    if name == "" {
        name = "playlist"
    }
    replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", "\"", "", "*", "", "?", "", "<", "", ">", "", "|", "")
    return replacer.Replace(name)
}

func (a *API) exportPlaylistM3U(w http.ResponseWriter, r *http.Request) {
    playlist, err := a.db.GetPlaylistByID(chi.URLParam(r, "id"))
    if err != nil {
        if err == sql.ErrNoRows {
            respondError(w, http.StatusNotFound, "Playlist not found")
        } else {
            respondError(w, http.StatusInternalServerError, err.Error())
        }
        return
    }

    var output strings.Builder
    output.WriteString("#EXTM3U\n")
    for _, songID := range playlist.SongIDs {
        song, err := a.db.GetSongByID(songID)
        if err != nil {
            continue
        }
        fmt.Fprintf(&output, "#EXTINF:%d,%s - %s\n%s\n", int(song.Duration), song.Artist, song.Title, song.FilePath)
    }
    w.Header().Set("Content-Type", "audio/x-mpegurl; charset=utf-8")
    w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.m3u8"`, safePlaylistFilename(playlist.Name)))
    _, _ = w.Write([]byte(output.String()))
}

func (a *API) importPlaylistM3U(w http.ResponseWriter, r *http.Request) {
    var request struct {
        Name    string `json:"name"`
        Content string `json:"content"`
    }
    if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&request); err != nil {
        respondError(w, http.StatusBadRequest, "Invalid M3U import")
        return
    }
    paths := parseM3U(request.Content)
    songs, err := a.db.GetAllSongs()
    if err != nil {
        respondError(w, http.StatusInternalServerError, err.Error())
        return
    }
    songByPath := make(map[string]string, len(songs))
    for _, song := range songs {
        songByPath[normalizePlaylistPath(song.FilePath)] = song.ID
    }
    songIDs := make([]string, 0, len(paths))
    unmatched := make([]string, 0)
    seen := make(map[string]struct{})
    for _, path := range paths {
        id, found := songByPath[normalizePlaylistPath(path)]
        if !found {
            unmatched = append(unmatched, path)
            continue
        }
        if _, duplicate := seen[id]; duplicate {
            continue
        }
        seen[id] = struct{}{}
        songIDs = append(songIDs, id)
    }
    name := strings.TrimSpace(request.Name)
    if name == "" {
        name = "Imported Playlist"
    }
    playlist := &db.Playlist{
        ID:        fmt.Sprintf("pl_%d", time.Now().UnixNano()),
        Name:      name,
        SongIDs:   songIDs,
        CreatedAt: time.Now().UnixMilli(),
    }
    if err := a.db.SavePlaylist(playlist); err != nil {
        respondError(w, http.StatusInternalServerError, err.Error())
        return
    }
    respondJSON(w, map[string]any{
        "playlist": playlist,
        "matched": len(songIDs),
        "unmatched": unmatched,
    })
}
''',
)


def fix_routes(text: str) -> str:
    playlist_marker = '''\tr.Get("/playlists", a.getPlaylists)
\tr.Post("/playlists", a.createPlaylist)
'''
    if '/playlists/import/m3u' not in text and playlist_marker in text:
        text = text.replace(playlist_marker, '''\tr.Get("/playlists", a.getPlaylists)
\tr.Post("/playlists/import/m3u", a.importPlaylistM3U)
\tr.Get("/playlists/{id}/export.m3u", a.exportPlaylistM3U)
\tr.Post("/playlists", a.createPlaylist)
''', 1)
    library_marker = '\tr.Post("/library/enrich-genres", a.enrichGenres)\n'
    if '/library/duplicates' not in text and library_marker in text:
        text = text.replace(library_marker, library_marker + '''\tr.Get("/library/duplicates", a.getDuplicateGroups)
\tr.Get("/library/duplicates/ignored", a.getIgnoredSongs)
\tr.Post("/library/duplicates/ignore", a.setDuplicateIgnored)
''', 1)
    return text


def fix_api_client(text: str) -> str:
    if 'export interface DuplicateGroup' not in text:
        type_marker = 'export interface ApiPlaylist {'
        duplicate_types = '''export interface DuplicateSong extends ApiSong {
  sourcePath?: string;
}

export interface DuplicateGroup {
  fileHash: string;
  songs: DuplicateSong[];
}

export interface M3UImportResult {
  playlist: ApiPlaylist;
  matched: number;
  unmatched: string[];
}

'''
        text = text.replace(type_marker, duplicate_types + type_marker, 1)

    object_marker = '  // Scanning\n'
    if 'getDuplicateGroups()' not in text and object_marker in text:
        methods = '''  // Library integrity
  async getDuplicateGroups(): Promise<DuplicateGroup[]> {
    const response = await fetch(`${API_BASE}/library/duplicates`);
    return handleResponse(response);
  },

  async getIgnoredSongs(): Promise<DuplicateSong[]> {
    const response = await fetch(`${API_BASE}/library/duplicates/ignored`);
    return handleResponse(response);
  },

  async setDuplicateIgnored(songId: string, ignored: boolean): Promise<void> {
    const response = await fetch(`${API_BASE}/library/duplicates/ignore`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ songId, ignored }),
    });
    await handleResponse(response);
  },

  async importPlaylistM3U(name: string, content: string): Promise<M3UImportResult> {
    const response = await fetch(`${API_BASE}/playlists/import/m3u`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, content }),
    });
    return handleResponse(response);
  },

  async exportPlaylistM3U(id: string): Promise<Blob> {
    const response = await fetch(`${API_BASE}/playlists/${encodeURIComponent(id)}/export.m3u`);
    if (!response.ok) {
      const error = await response.json().catch(() => ({ error: `HTTP ${response.status}` }));
      throw new Error(error.error || `HTTP ${response.status}`);
    }
    return response.blob();
  },

'''
        text = text.replace(object_marker, methods + object_marker, 1)
    return text


def fix_app_routes(text: str) -> str:
    if "import { Duplicates }" not in text:
        last_import = "import { Settings } from './pages/Settings';\n"
        if last_import in text:
            text = text.replace(last_import, last_import + "import { Duplicates } from './pages/Duplicates';\n", 1)
        else:
            text = text.replace("import Settings from './pages/Settings';\n", "import Settings from './pages/Settings';\nimport { Duplicates } from './pages/Duplicates';\n", 1)
    route_marker = '          <Route path="/stats" element={<Stats />} />\n'
    if 'path="/duplicates"' not in text and route_marker in text:
        text = text.replace(route_marker, route_marker + '          <Route path="/duplicates" element={<Duplicates />} />\n', 1)
    return text


def fix_sidebar(text: str) -> str:
    text = text.replace('ChevronLeft, ChevronRight, BarChart3, Heart, Disc3, X', 'ChevronLeft, ChevronRight, BarChart3, Heart, Disc3, Copy, X')
    marker = '          <SidebarItem to="/stats" icon={BarChart3} label="Stats" collapsed={effectiveCollapsed} onNavigate={onMobileClose} />\n'
    if 'to="/duplicates"' not in text and marker in text:
        text = text.replace(marker, marker + '          <SidebarItem to="/duplicates" icon={Copy} label="Duplicates" collapsed={effectiveCollapsed} onNavigate={onMobileClose} />\n', 1)
    return text


def fix_playlists_page(text: str) -> str:
    text = text.replace("import React, { useState } from 'react';", "import React, { useRef, useState } from 'react';")
    text = text.replace("import { ListMusic, Plus } from 'lucide-react';", "import { Download, FileUp, ListMusic, Plus } from 'lucide-react';")
    if "import { api } from '../services/api';" not in text:
        text = text.replace("import { CardSizeSlider } from '../components/ui/CardSizeSlider';\n", "import { CardSizeSlider } from '../components/ui/CardSizeSlider';\nimport { api } from '../services/api';\n")
    text = text.replace('  const { playlists, createPlaylist, openContextMenu } = useStore();', '  const { playlists, createPlaylist, openContextMenu, refreshLibrary } = useStore();')
    if 'const importInputRef' not in text:
        state_marker = "  const [cardCols, setCardCols] = useState(() => Number(localStorage.getItem('playlists-card-cols') ?? 5));\n"
        functions = state_marker + '''  const importInputRef = useRef<HTMLInputElement>(null);

  const handleImport = async (file?: File) => {
    if (!file) return;
    const content = await file.text();
    const name = file.name.replace(/\.(m3u8?|txt)$/i, '') || 'Imported Playlist';
    await api.importPlaylistM3U(name, content);
    await refreshLibrary();
    if (importInputRef.current) importInputRef.current.value = '';
  };

  const handleExport = async (id: string, name: string) => {
    const blob = await api.exportPlaylistM3U(id);
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = `${name.replace(/[^a-z0-9_-]+/gi, '-')}.m3u8`;
    anchor.click();
    URL.revokeObjectURL(url);
  };
'''
        text = text.replace(state_marker, functions, 1)
    actions_marker = '''          <button
            onClick={() => setShowInput(true)}
'''
    if 'Import M3U' not in text and actions_marker in text:
        import_button = '''          <input
            ref={importInputRef}
            type="file"
            accept=".m3u,.m3u8,audio/x-mpegurl"
            className="hidden"
            onChange={(event) => void handleImport(event.target.files?.[0])}
          />
          <button
            onClick={() => importInputRef.current?.click()}
            className="flex items-center gap-2 bg-surface-hover hover:bg-surface-border text-text-main px-4 py-2 rounded-full font-medium transition-colors text-sm"
          >
            <FileUp size={16} /> Import M3U
          </button>
'''
        text = text.replace(actions_marker, import_button + actions_marker, 1)
    card_marker = '                    <p className="text-sm text-text-secondary">{pl.songIds.length} songs</p>\n'
    if 'handleExport(pl.id' not in text and card_marker in text:
        text = text.replace(card_marker, card_marker + '''                    <button
                      type="button"
                      className="mt-3 inline-flex items-center gap-2 text-xs font-medium text-text-secondary hover:text-brand"
                      onClick={(event) => { event.stopPropagation(); void handleExport(pl.id, pl.name); }}
                    >
                      <Download size={14} /> Export M3U
                    </button>
''', 1)
    return text


def fix_duplicate_page_paths(text: str) -> str:
    # API intentionally exposes media URLs, not source paths. The duplicate hash and metadata are sufficient for safe selection.
    return text.replace('{song.sourcePath || song.filePath}', '{song.id}')


rewrite('backend/internal/db/db.go', fix_db_replaygain)
rewrite('backend/internal/scanner/scanner.go', fix_scanner_replaygain)
rewrite('types.ts', fix_frontend_replaygain_types)
rewrite('services/api.ts', fix_api_song_type)
rewrite('services/backendService.ts', fix_backend_mapping)
rewrite('lib/playbackLifecycle.ts', fix_playback_lifecycle)
rewrite('lib/playbackLifecycle.test.ts', fix_playback_lifecycle_test)
rewrite('lib/audio.ts', fix_audio_engine)
rewrite('hooks/useAudioPlayer.ts', fix_audio_hook_normalization)
rewrite('backend/internal/api/api.go', fix_routes)
rewrite('services/api.ts', fix_api_client)
rewrite('App.tsx', fix_app_routes)
rewrite('components/Sidebar.tsx', fix_sidebar)
rewrite('pages/Playlists.tsx', fix_playlists_page)
rewrite('pages/Duplicates.tsx', fix_duplicate_page_paths)
print('Capability completion applied')
