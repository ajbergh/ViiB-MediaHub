package track

import (
	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/key"
	"github.com/ajbergh/viib-mediahub/internal/analysis/tempo"
	"github.com/ajbergh/viib-mediahub/internal/db"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	"testing"
)

func TestPersistPreservesLocalKeyAndTempoUnderProviderProjection(t *testing.T) {
	database, ids := runnerCatalog(t, 1)
	source, err := analysis.ResolveLocalSource(database, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	providerBPM, providerKey, providerMode := 130.0, 9, 0
	result := Result{SongID: ids[0], Source: source, Status: db.TrackAnalysisComplete,
		Tempo:   tempo.Estimate{Known: true, BPM: 120, Confidence: .9, Stability: .95},
		Key:     key.Estimate{Known: true, Tonic: 0, Mode: "major", Confidence: .8, Camelot: "8B", OpenKey: "1d"},
		Spotify: &spotifyanalysis.Observation{BPM: &providerBPM, Key: &providerKey, Mode: &providerMode}}
	if err := Persist(database, result); err != nil {
		t.Fatal(err)
	}
	got, err := database.GetTrackAnalysis(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if got.Local == nil || got.Local.KeyTonic == nil || *got.Local.KeyTonic != 0 || *got.Local.KeyMode != "major" || *got.Local.KeyConfidence != .8 || *got.Local.BPM != 120 || *got.Local.BPMConfidence != .9 {
		t.Fatalf("lost measured alternatives: %+v", got.Local)
	}
	if *got.BPM != 130 || *got.KeyTonic != 9 || *got.KeyMode != "minor" || got.BPMConfidence != nil || got.KeyConfidence != nil {
		t.Fatalf("provider projection: %+v", got)
	}
	// A later partial refresh may change one provider dimension while preserving
	// the coupled local key and all local measurement confidence/provenance.
	refreshed := spotifyanalysis.Observation{BPM: &providerBPM}
	db.ApplySpotifyScalars(&got, refreshed)
	if err := database.UpsertTrackAnalysis(got); err != nil {
		t.Fatal(err)
	}
	got, err = database.GetTrackAnalysis(ids[0])
	if err != nil || *got.Local.KeyTonic != 0 || *got.Local.KeyMode != "major" {
		t.Fatal("partial provider refresh lost local key")
	}
}
