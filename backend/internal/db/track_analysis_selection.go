// track_analysis_selection.go derives the analysis work list from the catalog
// rather than storing it in job state. Because per-track status lives in
// track_analysis, "what is left to do" can always be recomputed, which is what
// makes a multi-day analysis run resumable across restarts.
package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Analysis selection modes. The mode plus its arguments are persisted in the
// job's parameters so a resumed job re-expands the identical selection.
const (
	AnalysisSelectionAll      = "all"
	AnalysisSelectionMissing  = "missing"
	AnalysisSelectionStale    = "stale"
	AnalysisSelectionIDs      = "ids"
	AnalysisSelectionPlaylist = "playlist"
)

// maxAnalysisSelection bounds one job's work list so a pathological catalog
// cannot produce an unbounded in-memory slice.
const maxAnalysisSelection = 200000

// AnalysisSelection describes which tracks a job covers.
type AnalysisSelection struct {
	Mode       string   `json:"mode"`
	SongIDs    []string `json:"songIds,omitempty"`
	PlaylistID string   `json:"playlistId,omitempty"`
}

// ParseAnalysisSelection reads a selection from persisted job parameters.
// An empty parameter block means the whole library, which matches the
// "Analyze Missing" default rather than a forced full re-analysis.
func ParseAnalysisSelection(parameters json.RawMessage) (AnalysisSelection, error) {
	selection := AnalysisSelection{Mode: AnalysisSelectionMissing}
	if len(parameters) > 0 {
		if err := json.Unmarshal(parameters, &selection); err != nil {
			return AnalysisSelection{}, fmt.Errorf("invalid analysis selection: %w", err)
		}
	}
	selection.Mode = strings.ToLower(strings.TrimSpace(selection.Mode))
	if selection.Mode == "" {
		selection.Mode = AnalysisSelectionMissing
	}
	return selection, selection.Validate()
}

// Validate rejects a selection that cannot be expanded.
func (s AnalysisSelection) Validate() error {
	switch s.Mode {
	case AnalysisSelectionAll, AnalysisSelectionMissing, AnalysisSelectionStale:
		return nil
	case AnalysisSelectionIDs:
		if len(s.SongIDs) == 0 {
			return fmt.Errorf("analysis selection %q requires song IDs", s.Mode)
		}
		return nil
	case AnalysisSelectionPlaylist:
		if strings.TrimSpace(s.PlaylistID) == "" {
			return fmt.Errorf("analysis selection %q requires a playlist ID", s.Mode)
		}
		return nil
	default:
		return fmt.Errorf("unsupported analysis selection mode %q", s.Mode)
	}
}

// ExpandAnalysisSelection returns the candidate song IDs for a selection in a
// stable order. Plex-backed songs are excluded because they need an
// authenticated source adapter that does not exist yet; including them would
// only produce a run of source_unavailable failures.
//
// Expansion is deliberately coarse: it filters on what SQL can see cheaply
// (presence of a row, analysis version, algorithm version). The authoritative
// per-track validity check compares the source fingerprint and must happen in
// the runner, because it requires touching the file.
func (d *DB) ExpandAnalysisSelection(selection AnalysisSelection, analysisVersion int, algorithmVersion string) ([]string, error) {
	if err := selection.Validate(); err != nil {
		return nil, err
	}
	if err := d.EnsureTrackAnalysisSchema(); err != nil {
		return nil, err
	}
	// The exclusion below reads plex_tracks, which is installed lazily and is
	// absent in a catalog that never connected a Plex source.
	if err := d.EnsurePlexSchema(); err != nil {
		return nil, err
	}

	// Restrict to songs this analyzer can actually open.
	base := `SELECT s.id FROM songs s
		LEFT JOIN track_analysis a ON a.song_id = s.id
		WHERE s.file_path IS NOT NULL AND TRIM(s.file_path) != ''
		  AND NOT EXISTS (SELECT 1 FROM plex_tracks p WHERE p.song_id = s.id)`
	args := []any{}

	switch selection.Mode {
	case AnalysisSelectionAll:
		// Every eligible song; the runner still skips tracks that are valid.
	case AnalysisSelectionMissing:
		base += ` AND a.song_id IS NULL`
	case AnalysisSelectionStale:
		base += ` AND (a.song_id IS NULL OR a.analysis_version != ? OR a.algorithm_version != ?)`
		args = append(args, analysisVersion, algorithmVersion)
	case AnalysisSelectionIDs:
		placeholders, values := inPlaceholders(selection.SongIDs)
		base += ` AND s.id IN (` + placeholders + `)`
		args = append(args, values...)
	case AnalysisSelectionPlaylist:
		playlist, err := d.GetPlaylistByID(selection.PlaylistID)
		if err != nil {
			return nil, err
		}
		if playlist == nil || len(playlist.SongIDs) == 0 {
			return []string{}, nil
		}
		placeholders, values := inPlaceholders(playlist.SongIDs)
		base += ` AND s.id IN (` + placeholders + `)`
		args = append(args, values...)
	}
	base += ` ORDER BY s.added_at ASC, s.id ASC LIMIT ?`
	args = append(args, maxAnalysisSelection)

	rows, err := d.conn.Query(base, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// inPlaceholders builds a bounded IN clause. Empty and duplicate IDs are
// removed so a caller cannot inflate the work list with repeats.
func inPlaceholders(values []string) (string, []any) {
	seen := make(map[string]struct{}, len(values))
	args := make([]any, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, duplicate := seen[trimmed]; duplicate {
			continue
		}
		seen[trimmed] = struct{}{}
		args = append(args, trimmed)
		if len(args) >= maxAnalysisSelection {
			break
		}
	}
	if len(args) == 0 {
		// A guaranteed-empty clause keeps the caller's single query shape.
		return `NULL`, nil
	}
	return strings.TrimSuffix(strings.Repeat("?,", len(args)), ","), args
}

// TrackAnalysisLeaseMillis bounds how long one worker may hold a track before
// another may take it. It must exceed the slowest single-track analysis but
// stay short enough that a crashed run does not park work for long.
const TrackAnalysisLeaseMillis int64 = 10 * 60 * 1000

// ClaimTrackAnalysis atomically marks one song as being analyzed and reports
// whether this caller won the claim. It is what single-flights two overlapping
// jobs that expanded the same selection: without it, both would decode the
// same file and duplicate the work.
//
// A claim is granted when no other worker holds the row or when the previous
// holder's lease expired, which is how work orphaned by a crash is recovered.
// Measured columns are untouched, so a lost claim never destroys a result.
func (d *DB) ClaimTrackAnalysis(songID, sourceFingerprint string, analysisVersion int, algorithmVersion string) (bool, error) {
	if err := d.EnsureTrackAnalysisSchema(); err != nil {
		return false, err
	}
	now := time.Now().UnixMilli()
	result, err := d.conn.Exec(`
		INSERT INTO track_analysis(song_id, status, analysis_version, algorithm_version, source_fingerprint, analyzed_at)
		VALUES(?, ?, ?, ?, ?, ?)
		ON CONFLICT(song_id) DO UPDATE SET
			status = excluded.status,
			analysis_version = excluded.analysis_version,
			algorithm_version = excluded.algorithm_version,
			source_fingerprint = excluded.source_fingerprint,
			analyzed_at = excluded.analyzed_at
		WHERE track_analysis.status != ?
		   OR track_analysis.analyzed_at IS NULL
		   OR track_analysis.analyzed_at < ?`,
		songID, TrackAnalysisRunning, analysisVersion, algorithmVersion, sourceFingerprint, now,
		TrackAnalysisRunning, now-TrackAnalysisLeaseMillis)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

// ReleaseTrackAnalysis returns a claimed track to the work list without
// recording a result. A canceled run must call it, otherwise the track stays
// leased and an immediate resume would skip it for the whole lease window.
func (d *DB) ReleaseTrackAnalysis(songID string) error {
	if err := d.EnsureTrackAnalysisSchema(); err != nil {
		return err
	}
	_, err := d.conn.Exec(`UPDATE track_analysis SET status = ?, analyzed_at = NULL WHERE song_id = ? AND status = ?`,
		TrackAnalysisPending, songID, TrackAnalysisRunning)
	return err
}

// TrackAnalysisValid reports whether an existing record was produced by the
// current analyzer from the current source bytes, which is what allows an
// interrupted run to skip work it already completed.
//
// A failed or unsupported record made from the same source is also treated as
// settled: retrying a poison file on every pass would starve the queue.
func (d *DB) TrackAnalysisValid(songID, sourceFingerprint string, analysisVersion int, algorithmVersion string) (bool, error) {
	analysis, err := d.GetTrackAnalysis(songID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if analysis.AnalysisVersion != analysisVersion || analysis.AlgorithmVersion != algorithmVersion {
		return false, nil
	}
	if analysis.SourceFingerprint != sourceFingerprint {
		return false, nil
	}
	switch analysis.Status {
	case TrackAnalysisComplete, TrackAnalysisPartial, TrackAnalysisFailed, TrackAnalysisUnsupported:
		return true, nil
	default:
		return false, nil
	}
}
