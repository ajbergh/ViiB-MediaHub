package db

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
)

func detailedObservation(t *testing.T, body string) spotifyanalysis.Observation {
	t.Helper()
	o, err := spotifyanalysis.ValidateDomainPayload(referenceID, "audio_analysis", []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	o.AccountContext = "account-a"
	o.RetrievedAt = time.Unix(1700000000, 0).UTC()
	return o
}
func TestSpotifyAudioArtifactsAtomicRoundTripAndLastGood(t *testing.T) {
	path := filepath.Join(t.TempDir(), "artifacts.db")
	d, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	o := detailedObservation(t, `{"track":{"tempo":120,"duration":10},"meta":{"version":"fixture"},"beats":[{"start":0,"duration":1}],"sections":[{"start":0,"duration":10,"unknown":false}]}`)
	expires := o.RetrievedAt.Add(time.Hour)
	if err = d.PutExternalAnalysis(o, "fixture", expires); err != nil {
		d.Close()
		t.Fatal(err)
	}
	newer := detailedObservation(t, `{"track":{"tempo":121,"duration":10},"sections":[{"start":0,"duration":10,"unknown":true}]}`)
	newer.RetrievedAt = o.RetrievedAt.Add(time.Minute)
	if err = d.PutExternalAnalysis(newer, "fixture", expires); err != nil {
		d.Close()
		t.Fatal(err)
	}
	d.Close()
	d, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if cache, err := d.GetExternalAnalysis(referenceID, "audio_analysis"); err != nil || cache != nil {
		t.Fatal("private scalar cache remained eligible after restart")
	}
	// Reactivate the synthetic owner to inspect storage rollback. Real runtime
	// generations use fresh UUIDs and do not reactivate a retired context.
	if err := d.ActivateSpotifyMetadataContext("account-a"); err != nil {
		t.Fatal(err)
	}
	beats, err := d.GetSpotifyAudioArtifact(referenceID, "audio_analysis", "beats", "account-a")
	if err != nil || beats == nil || !beats.RetrievedAt.Equal(o.RetrievedAt) {
		t.Fatal("last-good beats lost", err)
	}
	sections, err := d.GetSpotifyAudioArtifact(referenceID, "audio_analysis", "sections", "account-a")
	if err != nil || sections == nil || !bytes.Contains(sections.Payload, []byte(`"unknown":true`)) {
		t.Fatal("sections failed to round trip", err)
	}
	if artifact, err := d.GetSpotifyAudioArtifact(referenceID, "audio_analysis", "beats", "account-b"); err != nil || artifact != nil {
		t.Fatal("account artifact leaked")
	}
	invalid := newer
	invalid.RetrievedAt = newer.RetrievedAt.Add(time.Minute)
	bad := 122.0
	invalid.BPM = &bad
	invalid.DomainPayload = []byte(`{"track":{"tempo":122},"beats":[{"duration":1}]}`)
	if err = d.PutExternalAnalysis(invalid, "fixture", expires); err == nil {
		t.Fatal("invalid domain accepted")
	}
	cache, err := d.GetExternalAnalysis(referenceID, "audio_analysis")
	if err != nil || cache == nil || *cache.Observation.BPM != 121 {
		t.Fatal("artifact failure published scalar")
	}
	encoded, _ := json.Marshal(cache.Observation)
	if bytes.Contains(encoded, []byte("unknown")) {
		t.Fatal("large artifact leaked into cache")
	}
	if err = d.PurgeSpotifyMetadata(); err != nil {
		t.Fatal(err)
	}
	if artifact, err := d.GetSpotifyAudioArtifact(referenceID, "audio_analysis", "beats", "account-a"); err != nil || artifact != nil {
		t.Fatal("private artifact survived retirement")
	}
}
