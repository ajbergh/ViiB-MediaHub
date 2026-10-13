// Tests frozen-reference export output and protection of existing output files.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysisbench"
	"github.com/ajbergh/viib-mediahub/internal/db"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
)

func TestExportCommandWritesLoadableSnapshotAndPreservesExistingOutput(t *testing.T) {
	root := t.TempDir()
	audio := filepath.Join(root, "synthetic.ogg")
	if err := os.WriteFile(audio, []byte("synthetic fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	databasePath := filepath.Join(root, "library.db")
	database, err := db.New(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	song := db.Song{ID: "song", FilePath: audio, Title: "fixture", Artist: "generator", Album: "synthetic", AddedAt: 1}
	if err := database.SaveSong(&song); err != nil {
		t.Fatal(err)
	}
	source, err := analysis.ResolveLocalSongSource(song)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.RefreshTrackAnalysisSourceRevision(song.ID, source.Fingerprint); err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("A", 22)
	if ok, err := database.ConfirmSpotifyRecording(song.ID, id, source.Fingerprint, true); err != nil || !ok {
		t.Fatalf("confirmation=%v err=%v", ok, err)
	}
	now := time.Now().UTC()
	bpm := 120.0
	observation := spotifyanalysis.Observation{AccountContext: "export-fixture", TrackID: id, Source: "spotify_internal", SourceEndpoint: "audio_features", RetrievedAt: now.Add(-time.Minute), BPM: &bpm}
	if err := database.ActivateSpotifyMetadataContext("export-fixture"); err != nil {
		t.Fatal(err)
	}
	if err := database.PutExternalAnalysis(observation, "fixture-adapter-v1", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	// The documented hash is whole container bytes, independently computed here.
	hash, err := fileHash(audio)
	if err != nil {
		t.Fatal(err)
	}
	manifest := analysisbench.CorpusManifest{Version: "fixture-v1", EvidenceClass: analysisbench.EvidenceSyntheticCI, Tracks: []analysisbench.CorpusTrack{{ID: "corpus", Path: audio, License: "generated", LabelSource: "generator", Genre: "electronic", Split: analysisbench.SplitHeldOut, ExpectedBPM: &bpm, SpotifyRecording: &analysisbench.SpotifyRecordingIdentity{RecordingID: id, RecordingVersion: "synthetic-v1", AudioSHA256: hash, Confirmed: true}}}}
	manifestPath := filepath.Join(root, "manifest.json")
	data, _ := json.Marshal(manifest)
	if err := os.WriteFile(manifestPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	options := analysisbench.SpotifyReferenceExportOptions{Endpoint: "audio_features", License: "generated", LabelSource: "fixture-cache", Now: now}
	output := filepath.Join(root, "snapshot.json")
	if err := run(databasePath, manifestPath, output, options); err != nil {
		t.Fatal(err)
	}
	snapshot, err := analysisbench.LoadSpotifyReferenceSnapshot(output)
	if err != nil || len(snapshot.Entries) != 1 {
		t.Fatalf("snapshot=%+v err=%v", snapshot, err)
	}
	before, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if err := run(databasePath, manifestPath, output, options); err == nil {
		t.Fatal("existing output overwritten")
	}
	after, _ := os.ReadFile(output)
	if string(before) != string(after) {
		t.Fatal("existing output changed")
	}
	emptyOutput := filepath.Join(root, "empty.json")
	options.Endpoint = "audio_analysis"
	if err := run(databasePath, manifestPath, emptyOutput, options); err == nil {
		t.Fatal("missing cache produced output")
	}
	if _, err := os.Stat(emptyOutput); !os.IsNotExist(err) {
		t.Fatal("empty snapshot file created")
	}
}

func fileHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
