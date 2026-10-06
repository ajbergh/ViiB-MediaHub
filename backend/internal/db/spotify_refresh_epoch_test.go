package db

import (
	"bytes"
	"errors"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	"path/filepath"
	"testing"
	"time"
)

func TestSpotifyRefreshEpochPreservesFactsArtifactsAndAttempts(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "refresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.ReserveSpotifyMetadataRuntime("epoch1"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ConfirmSpotifyMetadataOwner("epoch1", "webplayer", "account", "account-a"); err != nil {
		t.Fatal(err)
	}
	old := SpotifyMetadataFence{Epoch: "epoch1", ContextKey: "account-a"}
	now := time.Now()
	o := detailedObservation(t, `{"track":{"tempo":120,"duration":10},"sections":[{"start":0,"duration":10,"unknown":false}]}`)
	o.RetrievedAt = now
	if err := d.PutExternalAnalysisForRuntime(old, o, "fixture", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ReserveSpotifyMetadataRuntime("epoch2"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ConfirmSpotifyMetadataOwner("epoch2", "webplayer", "account", "unused"); err != nil {
		t.Fatal(err)
	}
	late := detailedObservation(t, `{"track":{"tempo":140,"duration":10},"sections":[{"start":0,"duration":10,"unknown":true}]}`)
	late.RetrievedAt = now.Add(time.Minute)
	if err := d.PutExternalAnalysisForRuntime(old, late, "stale", now.Add(time.Hour)); !errors.Is(err, ErrSpotifyMetadataRuntimeSuperseded) {
		t.Fatal("old refresh published", err)
	}
	if err := d.PutExternalAnalysisStatusForRuntime(old, referenceID, "audio_analysis", ExternalAnalysisStatus{Code: spotifyanalysis.RateLimited, CheckedAt: late.RetrievedAt, RetryAt: now.Add(time.Hour)}); !errors.Is(err, ErrSpotifyMetadataRuntimeSuperseded) {
		t.Fatal("old failure published", err)
	}
	cache, err := d.GetExternalAnalysis(referenceID, "audio_analysis")
	if err != nil || cache == nil || cache.Observation.BPM == nil || *cache.Observation.BPM != 120 {
		t.Fatal("old cache replaced", cache, err)
	}
	fields, err := d.GetSpotifyScalarFields(referenceID, now)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, field := range fields {
		if field.Key == "tempo_bpm" {
			found = true
			if string(field.Value) != "120" || field.RetrievedAt.UnixMilli() != now.UnixMilli() {
				t.Fatal("old authoritative field replaced", field)
			}
		}
	}
	if !found {
		t.Fatal("current tempo field missing")
	}
	attempts, err := d.GetSpotifyFieldAttempts(referenceID)
	if err != nil || len(attempts) == 0 {
		t.Fatal("current field attempts missing", attempts, err)
	}
	for _, attempt := range attempts {
		if attempt.CheckedAt.UnixMilli() != now.UnixMilli() || attempt.AdapterRevision != "fixture" {
			t.Fatal("old successful-response attempt published", attempt)
		}
	}
	artifact, err := d.GetSpotifyAudioArtifact(referenceID, "audio_analysis", "sections", "account-a")
	if err != nil || artifact == nil || !bytes.Contains(artifact.Payload, []byte(`"unknown":false`)) {
		t.Fatal("old artifacts replaced", artifact, err)
	}
	if status, err := d.GetExternalAnalysisStatus(referenceID, "audio_analysis"); err != nil || status != nil {
		t.Fatal("old retry state published", status, err)
	}
	fresh := SpotifyMetadataFence{Epoch: "epoch2", ContextKey: "account-a"}
	wrong := late
	wrong.AccountContext = "other-account"
	if err := d.PutExternalAnalysisForRuntime(fresh, wrong, "fixture", now.Add(time.Hour)); !errors.Is(err, ErrSpotifyMetadataRuntimeSuperseded) {
		t.Fatal("cross-account observation allowed", err)
	}
	if err := d.PutExternalAnalysisForRuntime(fresh, late, "fixture", now.Add(time.Hour)); err != nil {
		t.Fatal("current refresh rejected", err)
	}
}
