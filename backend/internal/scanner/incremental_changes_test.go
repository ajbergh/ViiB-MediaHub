// Tests and fixtures for incremental changes behavior.

package scanner

import (
	"bytes"
	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/track"
	"github.com/ajbergh/viib-mediahub/internal/analysisbench"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"os"
	"path/filepath"
	"testing"
)

func TestPersistedSongUsesStableFingerprint(t *testing.T) {
	metadata := &SongMetadata{
		ID:           "path-specific-id",
		FileHash:     "stable-media-fingerprint",
		Title:        "Track",
		Artist:       "Artist",
		Album:        "Album",
		AlbumArtist:  "Album Artist",
		TrackNumber:  2,
		DiscNumber:   1,
		Genre:        []string{"Rock"},
		Year:         2001,
		Duration:     123.4,
		ReplayGainDB: -7.5,
		ReplayPeak:   0.91,
	}

	song := persistedSongFromMetadata(metadata, filepath.Join("music", "track.flac"), "cover.jpg", "resolved-logical-id", 42)
	if song.ID != "resolved-logical-id" {
		t.Fatalf("expected resolved ID, got %q", song.ID)
	}
	if song.FileHash != metadata.FileHash {
		t.Fatalf("expected stable fingerprint %q, got %q", metadata.FileHash, song.FileHash)
	}
	if song.FileHash == metadata.ID {
		t.Fatalf("incremental persistence must not store the path-specific ID as file hash")
	}
	if song.ReplayGainDB != metadata.ReplayGainDB || song.ReplayPeak != metadata.ReplayPeak {
		t.Fatalf("ReplayGain fields were not preserved")
	}
}

func TestCoalesceFileChanges(t *testing.T) {
	path := filepath.Join("music", "song.mp3")
	changes := coalesceFileChanges([]FileChange{
		{Path: path, ChangeType: ChangeTypeCreated, NewMtime: 1, NewSize: 10},
		{Path: path, ChangeType: ChangeTypeModified, NewMtime: 2, NewSize: 11},
	})
	if len(changes) != 1 {
		t.Fatalf("expected one coalesced change, got %d", len(changes))
	}
	if changes[0].ChangeType != ChangeTypeCreated {
		t.Fatalf("created then modified should remain created, got %s", changes[0].ChangeType)
	}
	if changes[0].NewMtime != 2 || changes[0].NewSize != 11 {
		t.Fatalf("expected newest file attributes, got mtime=%d size=%d", changes[0].NewMtime, changes[0].NewSize)
	}
}

func TestCoalesceDeleteThenCreateAsModification(t *testing.T) {
	path := filepath.Join("music", "song.mp3")
	changes := coalesceFileChanges([]FileChange{
		{Path: path, ChangeType: ChangeTypeDeleted},
		{Path: path, ChangeType: ChangeTypeCreated, NewMtime: 2, NewSize: 11},
	})
	if len(changes) != 1 || changes[0].ChangeType != ChangeTypeModified {
		t.Fatalf("delete/create replacement should become one modification: %#v", changes)
	}
}

func TestIncrementalSavedReplacementReachesMissingPreparation(t *testing.T) {
	directory := t.TempDir()
	database, err := db.New(filepath.Join(directory, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	fixture, err := analysisbench.NewClickTrack("clicks", 128, 3, 22050, 1)
	if err != nil {
		t.Fatal(err)
	}
	var wav bytes.Buffer
	if err := analysisbench.WriteWAVPCM16(&wav, fixture); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "song.wav")
	if err := os.WriteFile(path, wav.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	song := db.Song{ID: "song", Title: "Song", Artist: "Artist", Album: "Album", FilePath: path, FileHash: "original-content-hash", AddedAt: 1}
	if err := database.SaveSong(&song); err != nil {
		t.Fatal(err)
	}
	if _, err := track.Run(t.Context(), database, analysis.NewDefaultDecoderRegistry(), []string{song.ID}, track.RunOptions{}); err != nil {
		t.Fatal(err)
	}
	selection := db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}
	selected, err := track.ExpandPreparationSelection(t.Context(), database, selection, nil)
	if err != nil || len(selected) != 0 {
		t.Fatal(selected, err)
	}
	// Exercise the actual incremental batch-save seam with a changed scanner hash,
	// retaining path/size/mtime and canonical song ID.
	song.FileHash = "replacement-content-hash"
	scanner := &Scanner{db: database}
	if err := scanner.saveIncrementalBatch([]preparedIncrementalSong{{song: song}}, []string{path}, &ScanResult{}); err != nil {
		t.Fatal(err)
	}
	selected, err = track.ExpandPreparationSelection(t.Context(), database, selection, nil)
	if err != nil || len(selected) != 1 || selected[0] != song.ID {
		t.Fatal("scanner replacement omitted", selected, err)
	}
}
