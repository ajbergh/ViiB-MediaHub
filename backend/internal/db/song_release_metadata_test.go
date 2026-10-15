package db

import (
	"testing"
)

func TestTrackReleaseFactsSurviveRescansAndProtectAgainstAI(t *testing.T) {
	d, _ := evidenceFixture(t)
	song := Song{ID: "release", Title: "Track", Artist: "Artist", Album: "Remaster", FileHash: "source", Year: 2024, ReleaseDate: "2024-02-03"}
	if err := d.SaveSong(&song); err != nil {
		t.Fatal(err)
	}
	value := TrackReleaseMetadata{EditionDate: "2024-02-03", EditionSource: "spotify", OriginalDate: "1989-03-03", OriginalSource: "musicbrainz", State: "available"}
	if applied, err := d.ApplyTrackReleaseMetadata(t.Context(), song, value); err != nil || !applied {
		t.Fatal(applied, err)
	}
	if result, err := d.ApplyAIEnrichmentBatch([]AIEnrichmentUpdate{{SongID: song.ID, OriginalYear: 1999}}, true); err != nil || result.Years != 0 {
		t.Fatalf("AI replaced fact: %+v %v", result, err)
	}
	song.Year = 0
	song.ReleaseDate = ""
	if err := d.SaveSongs([]Song{song}); err != nil {
		t.Fatal(err)
	}
	got, err := d.GetSongByID(song.ID)
	if err != nil || got.Year != 2024 || got.OriginalYear != 1989 {
		t.Fatalf("rescan lost dates: %+v %v", got, err)
	}
	manual := 1988
	if _, err := d.UpdateSongMetadata(song.ID, SongMetadataPatch{Year: &manual}); err != nil {
		t.Fatal(err)
	}
	song.Year = 2025
	if err := d.SaveSongs([]Song{song}); err != nil {
		t.Fatal(err)
	}
	if applied, err := d.ApplyTrackReleaseMetadata(t.Context(), song, value); err != nil || applied {
		t.Fatal("manual overwritten", applied, err)
	}
	got, err = d.GetSongByID(song.ID)
	if err != nil || got.Year != 1988 || got.OriginalYear != 1988 {
		t.Fatalf("manual lost: %+v %v", got, err)
	}
	release, err := d.GetTrackReleaseMetadata(song.ID)
	if err != nil || release.OriginalSource != "manual" || release.EditionDate != "2024-02-03" {
		t.Fatalf("provenance=%+v err=%v", release, err)
	}
}

func TestTrackReleaseDatesRejectChangedIdentityAndUnknownOriginal(t *testing.T) {
	d, _ := evidenceFixture(t)
	song := Song{ID: "release", Title: "Track", Artist: "Artist", Album: "Album", Year: 2024, FileHash: "old"}
	if err := d.SaveSong(&song); err != nil {
		t.Fatal(err)
	}
	value := TrackReleaseMetadata{OriginalDate: "1989", OriginalSource: "musicbrainz", State: "available"}
	if _, err := d.ApplyTrackReleaseMetadata(t.Context(), song, value); err != nil {
		t.Fatal(err)
	}
	changed := song
	changed.Title = "Different track"
	changed.FileHash = "new"
	if err := d.SaveSong(&changed); err != nil {
		t.Fatal(err)
	}
	if applied, err := d.ApplyTrackReleaseMetadata(t.Context(), song, value); err != nil || applied {
		t.Fatal("stale lookup admitted", applied, err)
	}
	got, err := d.GetSongByID(song.ID)
	if err != nil || got.OriginalYear != 0 {
		t.Fatal("stale original year retained", got, err)
	}
	value.OriginalSource = "ai_inferred"
	if _, err := d.ApplyTrackReleaseMetadata(t.Context(), changed, value); err == nil {
		t.Fatal("AI accepted as verified fact")
	}
	if applied, err := d.ApplyTrackReleaseMetadata(t.Context(), changed, TrackReleaseMetadata{State: "ambiguous"}); err != nil || !applied {
		t.Fatal(applied, err)
	}
	got, _ = d.GetSongByID(song.ID)
	if got.OriginalYear != 0 {
		t.Fatal("invented year")
	}
}

func TestOriginalDateTagsPersistAndManualClearBlocksFallback(t *testing.T) {
	d, _ := evidenceFixture(t)
	song := Song{ID: "tagged", Title: "Track", Artist: "Artist", Album: "Album", Year: 2024, OriginalYear: 1989, ReleaseDate: "2024-02", OriginalReleaseDate: "1989", FileHash: "source"}
	if err := d.SaveSongs([]Song{song}); err != nil {
		t.Fatal(err)
	}
	value, err := d.GetTrackReleaseMetadata(song.ID)
	if err != nil || value.EditionDate != "2024-02" || value.OriginalDate != "1989" || value.OriginalSource != "file_tag" {
		t.Fatal(value, err)
	}
	zero := 0
	if _, err := d.UpdateSongMetadata(song.ID, SongMetadataPatch{Year: &zero}); err != nil {
		t.Fatal(err)
	}
	if err := d.SaveSongs([]Song{song}); err != nil {
		t.Fatal(err)
	}
	got, _ := d.GetSongByID(song.ID)
	if got.Year != 0 || got.OriginalYear != 0 {
		t.Fatal("manual clear lost", got)
	}
}

func TestTrackReleaseIdentityCorrectionClearsVerifiedOriginal(t *testing.T) {
	d, _ := evidenceFixture(t)
	song := Song{ID: "corrected", Title: "Wrong title", Artist: "Artist", Album: "Album", OriginalYear: 1989, OriginalReleaseDate: "1989"}
	if err := d.SaveSong(&song); err != nil {
		t.Fatal(err)
	}
	title := "Correct title"
	got, err := d.UpdateSongMetadata(song.ID, SongMetadataPatch{Title: &title})
	if err != nil || got.OriginalYear != 0 {
		t.Fatalf("stale year retained: %+v %v", got, err)
	}
	value, err := d.GetTrackReleaseMetadata(song.ID)
	if err != nil || value.OriginalDate != "" {
		t.Fatalf("stale date returned: %+v %v", value, err)
	}
	if _, err := d.ApplyTrackReleaseMetadata(t.Context(), got, TrackReleaseMetadata{State: "not_found"}); err != nil {
		t.Fatal(err)
	}
	value, err = d.GetTrackReleaseMetadata(song.ID)
	if err != nil || value.OriginalDate != "" {
		t.Fatalf("failed lookup restored stale date: %+v %v", value, err)
	}
}
