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
	if first.RecordingGroup != "label:djexample|firstsong" || second.RecordingGroup != "label:djexample|secondsong" {
		t.Fatal("import lost artist/title identity groups")
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

func TestArtistTitleIdentityResolvesDuplicateAndShortTitles(t *testing.T) {
	rows := []spotifyCSVRow{
		{Title: "Faded", Artist: "Alan Walker"},
		{Title: "Faded", Artist: "ZHU"},
		{Title: "Stay", Artist: "Zedd, Alessia Cara"},
		{Title: "King", Artist: "Olly Alexander (Years & Years)"},
	}
	media := []string{"045-Alan Walker-Faded.ogg", "100-ZHU-Faded.ogg", "036-Zedd-Stay.ogg", "050-Olly Alexander (Years & Years)-King.ogg"}
	matches, missing, unmatched, ambiguous := matchSpotifyCSVRows(media, rows)
	if len(matches) != 4 || len(missing)+len(unmatched)+len(ambiguous) != 0 {
		t.Fatalf("matches=%v missing=%v unmatched=%v ambiguous=%v", matches, missing, unmatched, ambiguous)
	}
	if matches[media[0]].Artist != "Alan Walker" || matches[media[1]].Artist != "ZHU" {
		t.Fatal("duplicate titles were assigned to the wrong artist")
	}
}

func TestArtistTitleIdentityRequiresExactOrExplicitlyTruncatedTitle(t *testing.T) {
	for _, tc := range []struct {
		file, artist, title string
		score               int
	}{
		{"029-Lana Del Rey-Summertime Sadness (Lana Del Rey Vs. Cedric Gervais) - Remix.ogg", "Lana Del Rey, Cedric Gervais", "Summertime Sadness (Lana Del Rey Vs. Cedric Gervai...", 4},
		{"001-Artist-One Long Song Remix.ogg", "Artist", "One Long Song", 0},
		{"001-Other Artist-One Long Song.ogg", "Artist", "One Long Song", 0},
		{"001-ArtistExtra-Stay.ogg", "Artist", "Stay", 0},
		{"001-Artist-Stay.ogg", "Artist", "Sta...", 0},
		{"001-2Pac-Changes.ogg", "2Pac", "Changes", 5},
		{"001-Artist-9 PM.ogg", "Artist", "9 PM", 5},
		{"001-Jay-Z-Song.ogg", "Jay-Z", "Song", 5},
	} {
		if score := spotifyArtistTitleMatchScore(tc.file, spotifyCSVRow{Artist: tc.artist, Title: tc.title}); score != tc.score {
			t.Errorf("%s: score=%d want=%d", tc.file, score, tc.score)
		}
	}
	rows := []spotifyCSVRow{{Artist: "Artist", Title: "Same Song"}, {Artist: "Artist", Title: "Same Song"}}
	matches, _, _, ambiguous := matchSpotifyCSVRows([]string{"001-Artist-Same Song.ogg"}, rows)
	if len(matches) != 0 || len(ambiguous) != 1 {
		t.Fatal("duplicate authoritative rows must remain ambiguous")
	}
}
