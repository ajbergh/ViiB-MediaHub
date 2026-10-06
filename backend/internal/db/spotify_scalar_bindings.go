package db

import (
	"errors"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
)

type SpotifyScalarBinding struct {
	TrackID           string `json:"trackId"`
	SourceFingerprint string `json:"sourceFingerprint"`
	AccountContext    string `json:"accountContext,omitempty"`
	Endpoint          string `json:"endpoint"`
	RetrievedAt       int64  `json:"retrievedAt"`
	Durable           bool   `json:"durable,omitempty"`
	Eligible          bool   `json:"-"`
}
type SpotifyScalarBindings struct {
	BPM *SpotifyScalarBinding `json:"bpm,omitempty"`
	Key *SpotifyScalarBinding `json:"key,omitempty"`
}

func (b *SpotifyScalarBindings) evaluate(fingerprint, active string) {
	for _, binding := range []*SpotifyScalarBinding{b.BPM, b.Key} {
		if binding != nil {
			binding.Eligible = binding.SourceFingerprint == fingerprint && (binding.Durable || (binding.AccountContext != "" && binding.AccountContext == active))
		}
	}
}
func bindingForObservation(record *TrackAnalysis, o spotifyanalysis.Observation) *SpotifyScalarBinding {
	if o.AccountContext == "" && !o.DurableImport {
		return nil
	}
	contextKey := o.AccountContext
	if o.DurableImport {
		contextKey = ""
	}
	return &SpotifyScalarBinding{TrackID: o.TrackID, SourceFingerprint: record.SourceFingerprint, AccountContext: contextKey, Endpoint: o.SourceEndpoint, RetrievedAt: o.RetrievedAt.UnixMilli(), Durable: o.DurableImport, Eligible: true}
}
func checkSpotifyBindings(executor preparationExecutor, a TrackAnalysis) error {
	if a.SpotifyBindings == nil {
		return nil
	}
	var active string
	if err := executor.QueryRow(`SELECT COALESCE((SELECT value FROM settings WHERE key='spotify_metadata_active_context'),'')`).Scan(&active); err != nil {
		return err
	}
	for _, v := range []struct {
		binding *SpotifyScalarBinding
		source  *string
	}{{a.SpotifyBindings.BPM, a.BPMSource}, {a.SpotifyBindings.Key, a.KeySource}} {
		if v.binding == nil || v.source == nil || *v.source != "spotify" {
			continue
		}
		b := v.binding
		if !validExternalKey(b.TrackID, b.Endpoint) || b.SourceFingerprint != a.SourceFingerprint || (b.Endpoint != "audio_features" && b.Endpoint != "audio_analysis") || len(b.AccountContext) > 1024 {
			return errors.New("invalid Spotify scalar binding")
		}
		if !b.Durable && (b.AccountContext == "" || b.AccountContext != active) {
			return errors.New("retired Spotify scalar binding")
		}
	}
	return nil
}

func (d *DB) ActivateSpotifyMetadataContext(accountContext string) error {
	if len(accountContext) > 1024 {
		return errors.New("invalid Spotify metadata context")
	}
	return d.SetSetting("spotify_metadata_active_context", accountContext)
}

// RetirePrivateSpotifyScalarBindings removes private projections and ownership
// metadata after the active generation is cleared, preserving durable imports
// and independent local facts. Late publications still fail the context guard.
func (d *DB) RetirePrivateSpotifyScalarBindings() error {
	analyses, err := d.ListTrackAnalysis()
	if err != nil {
		return err
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, a := range analyses {
		if a.SpotifyBindings == nil {
			continue
		}
		ApplySpotifyScalars(&a, spotifyanalysis.Observation{})
		if a.SpotifyBindings.BPM == nil && a.SpotifyBindings.Key == nil {
			a.SpotifyBindings = nil
		}
		if err := upsertTrackAnalysis(tx, a); err != nil {
			return err
		}
	}
	return tx.Commit()
}
