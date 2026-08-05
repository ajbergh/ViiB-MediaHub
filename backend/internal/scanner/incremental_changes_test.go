package scanner

import (
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
