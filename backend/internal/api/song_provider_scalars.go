package api

import (
	"context"
	"sort"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

// SongProviderScalars are provider candidates for the verified current recording.
// They remain separate from local energy, playback duration and BS.1770 metrics.
type SongProviderScalars struct {
	ReadOnly          bool                     `json:"readOnly"`
	Unverified        bool                     `json:"unverified"`
	Provenance        string                   `json:"provenance"`
	RecordingID       string                   `json:"recordingId"`
	SourceFingerprint string                   `json:"sourceFingerprint"`
	Fields            []db.SpotifyScalarField  `json:"fields"`
	Selected          []db.SpotifyScalarField  `json:"selected"`
	Attempts          []db.SpotifyFieldAttempt `json:"attempts"`
	SelectionPolicy   string                   `json:"selectionPolicy"`
}

func (a *API) songProviderScalars(songID, fingerprint string) (*SongProviderScalars, error) {
	return a.songProviderScalarsContext(context.Background(), songID, fingerprint)
}

func (a *API) songProviderScalarsContext(parent context.Context, songID, fingerprint string) (*SongProviderScalars, error) {
	if err := parent.Err(); err != nil {
		return nil, err
	}
	a.spotifyAuthMu.Lock()
	runtime := a.spotifyAuth
	a.spotifyAuthMu.Unlock()
	if runtime == nil {
		return a.readSongProviderScalars(songID, fingerprint, nil)
	}
	ctx, cancel := runtime.requestContext(parent)
	defer cancel()
	var result *SongProviderScalars
	err := runtime.withMetadataRead(ctx, func(fence db.SpotifyMetadataReadFence) error {
		var err error
		result, err = a.readSongProviderScalars(songID, fingerprint, &fence)
		if ctx.Err() != nil {
			result = nil
			return ctx.Err()
		}
		return err
	})
	return result, err
}

func (a *API) readSongProviderScalars(songID, fingerprint string, fence *db.SpotifyMetadataReadFence) (*SongProviderScalars, error) {
	if fingerprint == "" {
		return nil, nil
	}
	owner, err := a.db.GetSetting("spotify_metadata_active_context")
	if err != nil {
		return nil, err
	}
	if fence != nil {
		owner = fence.ContextKey
	}
	if owner == "" {
		return nil, nil
	}
	link, err := a.db.GetSpotifyRecording(songID, fingerprint)
	if err != nil {
		return nil, err
	}
	if link == nil {
		return nil, nil
	}
	var fields []db.SpotifyScalarField
	var attempts []db.SpotifyFieldAttempt
	if fence != nil {
		fields, attempts, err = a.db.GetSpotifyScalarCandidatesForRuntime(*fence, link.ExternalID, time.Now())
	} else {
		fields, err = a.db.GetSpotifyScalarFields(link.ExternalID, time.Now())
		if err == nil {
			attempts, err = a.db.GetSpotifyFieldAttempts(link.ExternalID)
		}
	}
	if err != nil {
		return nil, err
	}
	if len(fields) == 0 && len(attempts) == 0 {
		return nil, nil
	}
	// Recheck the live bytes, recording decision and generation after storage
	// reads. A replacement during assembly cannot borrow the former candidates.
	current, err := a.currentAnalysisSourceFingerprints([]string{songID})
	if err != nil {
		return nil, err
	}
	if current[songID] != fingerprint {
		return nil, nil
	}
	finalLink, err := a.db.GetSpotifyRecording(songID, fingerprint)
	if err != nil {
		return nil, err
	}
	if finalLink == nil || finalLink.ExternalID != link.ExternalID {
		return nil, nil
	}
	finalOwner, err := a.db.GetSetting("spotify_metadata_active_context")
	if err != nil {
		return nil, err
	}
	if fence != nil && fence.Pending {
		if err := a.db.ValidateSpotifyMetadataRead(*fence); err != nil {
			return nil, err
		}
		finalOwner = fence.ContextKey
	}
	if finalOwner != owner {
		return nil, nil
	}
	return &SongProviderScalars{ReadOnly: true, Unverified: fence != nil && fence.Pending, Provenance: "spotify_private_cache", RecordingID: link.ExternalID, SourceFingerprint: fingerprint, Fields: fields, Attempts: attempts, Selected: selectProviderScalarFields(fields), SelectionPolicy: "fresh_then_newest_v1"}, nil
}

// Selection compares candidates only for the same registered metric and units.
// Freshness wins over recency; stale immutable recording facts remain explicitly
// stale last-good candidates. Equal retrievals prefer detailed analysis for its
// analysis context. All alternatives remain in Fields.
func selectProviderScalarFields(fields []db.SpotifyScalarField) []db.SpotifyScalarField {
	selected := map[string]db.SpotifyScalarField{}
	for _, candidate := range fields {
		previous, exists := selected[candidate.Key]
		if exists && (candidate.Metric != previous.Metric || candidate.Units != previous.Units) {
			continue
		}
		better := !exists || (previous.Stale && !candidate.Stale)
		if exists && previous.Stale == candidate.Stale {
			better = candidate.RetrievedAt.After(previous.RetrievedAt) || (candidate.RetrievedAt.Equal(previous.RetrievedAt) && candidate.Endpoint == "audio_analysis" && previous.Endpoint != "audio_analysis")
		}
		if better {
			selected[candidate.Key] = candidate
		}
	}
	keys := make([]string, 0, len(selected))
	for key := range selected {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]db.SpotifyScalarField, 0, len(keys))
	for _, key := range keys {
		result = append(result, selected[key])
	}
	return result
}
