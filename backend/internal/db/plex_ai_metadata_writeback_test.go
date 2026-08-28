package db

import "testing"

func TestAIEnrichmentQueuesAndAuditsPlexWriteback(t *testing.T) {
	database := openPlexTestDB(t)
	defer database.Close()
	const sourceID, libraryID, machineID = "plexsrc-writeback", "2", "machine-writeback"
	if err := database.SavePlexSource(PlexSource{ID: sourceID, MachineIdentifier: machineID, BaseURL: "http://127.0.0.1:32400", Name: "Plex", LibraryID: libraryID, Active: true, Available: true}); err != nil {
		t.Fatal(err)
	}
	track := plexFixture(sourceID, libraryID, machineID, "1", "Queued Track", 100)
	if _, _, _, err := database.SyncPlexLibrary(sourceID, libraryID, []PlexCatalogTrack{track}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.conn.Exec(`UPDATE songs SET year_uncertain=1 WHERE id=?`, track.SongID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ApplyAIEnrichmentBatch([]AIEnrichmentUpdate{{
		SongID: track.SongID, Genres: []string{"dream pop", "indie rock"}, OriginalYear: 1988,
	}}, false); err != nil {
		t.Fatal(err)
	}
	candidates, hasMore, err := database.GetPlexAIWritebackCandidates(sourceID, nil, 100)
	if err != nil || hasMore || len(candidates) != 1 {
		t.Fatalf("candidates=%#v hasMore=%v err=%v", candidates, hasMore, err)
	}
	candidate := candidates[0]
	if candidate.SongID != track.SongID || candidate.RatingKey != "1" || candidate.OriginalYear != 1988 || len(candidate.Genres) != 2 || candidate.Genres[0] != "Dream Pop" || candidate.Genres[1] != "Indie Rock" {
		t.Fatalf("unexpected queued candidate: %#v", candidate)
	}
	if err := database.MarkPlexAIWritebackSynced(track.SongID, 2_000_000_000_000, 2_000_000_000_001); err != nil {
		t.Fatal(err)
	}
	candidates, hasMore, err = database.GetPlexAIWritebackCandidates(sourceID, nil, 100)
	if err != nil || hasMore || len(candidates) != 0 {
		t.Fatalf("synced proposal remained pending: %#v hasMore=%v err=%v", candidates, hasMore, err)
	}
}
