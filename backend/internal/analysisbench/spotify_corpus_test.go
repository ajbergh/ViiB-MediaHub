package analysisbench

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestImportSpotifyCorpusMatchesMediaByTitleAndConvertsCamelot(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "EDM Hits")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"001-DJ Example-First Song.mp3", "002-DJ Example-Second Song.ogg", "003-DJ Example-Unlabeled Song.mp3"} {
		if err := os.WriteFile(filepath.Join(directory, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeSpotifyCSV(t, filepath.Join(directory, "labels.csv"), "#,Title,Artist,BPM,Key,Duration\n1,First Song,DJ Example,128,8A,3:00\n2,Second Song,DJ Example,126,11B,3:00\n3,Missing Song,DJ Example,120,1A,3:00\n")

	options := SpotifyCorpusOptions{EvidenceClass: EvidenceLawfulRealAudio, License: "private-local", LabelSource: "Spotify confirmed BPM/key data"}
	report, err := ImportSpotifyCorpus(root, options)
	if err != nil {
		t.Fatal(err)
	}
	if report.MediaFiles != 3 || report.CSVRows != 3 || report.MatchedTracks != 2 {
		t.Fatalf("import report = %+v", report)
	}
	if report.Corpus.Tracks != 2 || report.Corpus.HeldOutTracks == 0 || report.Corpus.Phase0Ready {
		t.Fatalf("import coverage = %+v", report.Corpus)
	}
	if len(report.UnmatchedMedia) != 1 || len(report.UnmatchedCSVRows) != 1 || len(report.AmbiguousMedia) != 0 {
		t.Fatalf("matching diagnostics = %+v", report)
	}
	if err := report.Manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	var first, second CorpusTrack
	for _, track := range report.Manifest.Tracks {
		switch filepath.Base(track.Path) {
		case "001-DJ Example-First Song.mp3":
			first = track
		case "002-DJ Example-Second Song.ogg":
			second = track
		}
	}
	if first.ExpectedBPM == nil || *first.ExpectedBPM != 128 || first.ExpectedKey != "A minor" || first.LabelSource != options.LabelSource {
		t.Fatalf("first imported track = %+v", first)
	}
	if second.ExpectedBPM == nil || *second.ExpectedBPM != 126 || second.ExpectedKey != "A major" {
		t.Fatalf("second imported track = %+v", second)
	}
	secondReport, err := ImportSpotifyCorpus(root, options)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(report.Manifest.Tracks, secondReport.Manifest.Tracks) {
		t.Fatal("re-import changed stable IDs or held-out assignments")
	}
}

func TestImportSpotifyCorpusRejectsInvalidCamelotKey(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "001-DJ Example-First Song.mp3"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	writeSpotifyCSV(t, filepath.Join(root, "labels.csv"), "Title,Artist,BPM,Key\nFirst Song,DJ Example,128,13A\n")
	_, err := ImportSpotifyCorpus(root, SpotifyCorpusOptions{EvidenceClass: EvidenceLawfulRealAudio, License: "private-local", LabelSource: "Spotify"})
	if err == nil {
		t.Fatal("invalid Camelot key was accepted")
	}
}

func TestImportSpotifyCorpusRequiresProvenanceDeclaration(t *testing.T) {
	if _, err := ImportSpotifyCorpus(t.TempDir(), SpotifyCorpusOptions{}); err == nil {
		t.Fatal("missing provenance declaration was accepted")
	}
}

func writeSpotifyCSV(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
}
