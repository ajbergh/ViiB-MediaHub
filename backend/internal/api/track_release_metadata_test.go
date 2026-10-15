package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/releasedate"
)

func TestTrackReleaseUnavailableSourcePreservesAIAsEstimate(t *testing.T) {
	a, path := newBPMRouteTestAPI(t, false)
	if _, err := a.db.ApplyAIEnrichmentBatch([]db.AIEnrichmentUpdate{{SongID: "song", OriginalYear: 1989}}, true); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	song, err := a.db.GetSongByID("song")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.resolveTrackReleaseMetadata(t.Context(), *song); err != nil {
		t.Fatal(err)
	}
	value, err := a.db.GetTrackReleaseMetadata("song")
	if err != nil || value.State != "source_unavailable" || value.OriginalSource != "ai_inferred" || value.OriginalDate != "1989" || value.RetryAt <= value.CheckedAt {
		t.Fatalf("estimate or retry lost: %+v %v", value, err)
	}
}

func TestTrackReleaseFallbackStoresOriginalSeparatelyFromEdition(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, false)
	song, err := a.db.GetSongByID("song")
	if err != nil {
		t.Fatal(err)
	}
	song.Duration = 200
	song.Year = 2024
	song.ReleaseDate = "2024-03-04"
	if err := a.db.SaveSong(song); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"recordings":[{"id":"12345678-1234-1234-1234-123456789abc","title":"Song","length":200000,"score":100,"first-release-date":"1989","artist-credit":[{"name":"Artist","artist":{"name":"Artist"}}]}]}`))
	}))
	defer server.Close()
	a.releaseMusicBrainz = releasedate.NewClient()
	a.releaseMusicBrainz.BaseURL = server.URL
	a.releaseMusicBrainz.HTTP = server.Client()
	if err := a.resolveTrackReleaseMetadata(t.Context(), *song); err != nil {
		t.Fatal(err)
	}
	got, err := a.db.GetSongByID("song")
	if err != nil || got.Year != 2024 || got.OriginalYear != 1989 {
		t.Fatalf("song=%+v err=%v", got, err)
	}
	release, err := a.db.GetTrackReleaseMetadata("song")
	if err != nil || release.OriginalSource != "musicbrainz" || release.EditionDate != "2024-03-04" || release.OriginalDate != "1989" {
		t.Fatalf("release=%+v err=%v", release, err)
	}
	candidates, err := a.db.ReleaseMetadataCandidates(t.Context(), 8)
	if err != nil || len(candidates) != 0 {
		t.Fatal("successful lookup was not cached", candidates, err)
	}
}
