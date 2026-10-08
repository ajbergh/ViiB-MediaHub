package api

import (
	"context"
	"errors"
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
		return a.readSongProviderScalars(parent, songID, fingerprint, nil)
	}
	ctx, cancel := runtime.requestContext(parent)
	defer cancel()
	var result *SongProviderScalars
	err := runtime.withMetadataRead(ctx, func(fence db.SpotifyMetadataReadFence) error {
		var err error
		result, err = a.readSongProviderScalars(ctx, songID, fingerprint, &fence)
		if ctx.Err() != nil {
			result = nil
			return ctx.Err()
		}
		return err
	})
	return result, err
}

func (a *API) readSongProviderScalars(ctx context.Context, songID, fingerprint string, fence *db.SpotifyMetadataReadFence) (*SongProviderScalars, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
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
	var importedFields []db.SpotifyScalarField
	var importedAttempts []db.SpotifyFieldAttempt
	var importedRecording string
	var importErr error
	if fence == nil || !fence.Pending {
		importedFields, importedAttempts, importedRecording, importErr = a.db.GetDownloadedSpotifyScalarCandidates(ctx, songID, fingerprint)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if importErr != nil {
		return nil, importErr
	}
	if importedRecording != "" && importedRecording != link.ExternalID {
		return nil, nil
	}
	if len(fields) > 128-len(importedFields) || len(attempts) > 128-len(importedAttempts) {
		return nil, errors.New("provider scalar detail limit exceeded")
	}
	fields = append(fields, importedFields...)
	attempts = append(attempts, importedAttempts...)
	if importedRecording != "" {
		link.ExternalID = importedRecording
	}
	// A pending-owner read may expose only its captured owner's private data;
	// downloaded facts are account-independent but can reveal which recording
	// was linked to a local file before profile confirmation, so defer them
	// until that runtime owner check is settled.
	if fence != nil && fence.Pending && len(importedFields)+len(importedAttempts) > 0 {
		if len(importedFields) > 0 {
			fields = fields[:len(fields)-len(importedFields)]
		}
		if len(importedAttempts) > 0 {
			attempts = attempts[:len(attempts)-len(importedAttempts)]
		}
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
	if fence != nil {
		if err := a.db.ValidateSpotifyMetadataRead(*fence); err != nil {
			return nil, err
		}
		finalOwner = fence.ContextKey
	}
	if fence == nil && finalOwner != owner {
		return nil, nil
	}
	provenance := "spotify_private_cache"
	var hasPrivate, hasDurable bool
	for _, field := range fields {
		hasDurable = hasDurable || field.DurableImport
		hasPrivate = hasPrivate || !field.DurableImport
	}
	for _, attempt := range attempts {
		hasDurable = hasDurable || attempt.DurableImport
		hasPrivate = hasPrivate || !attempt.DurableImport
	}
	if hasDurable && hasPrivate {
		provenance = "spotify_mixed_private_and_download_import"
	} else if hasDurable {
		provenance = "spotify_download_import"
	}
	return &SongProviderScalars{ReadOnly: true, Unverified: fence != nil && fence.Pending, Provenance: provenance, RecordingID: link.ExternalID, SourceFingerprint: fingerprint, Fields: fields, Attempts: attempts, Selected: selectProviderScalarFields(fields), SelectionPolicy: "fresh_then_newest_v1"}, nil
}

func selectProviderScalarFields(fields []db.SpotifyScalarField) []db.SpotifyScalarField {
	return db.SelectProviderScalarFields(fields)
}
