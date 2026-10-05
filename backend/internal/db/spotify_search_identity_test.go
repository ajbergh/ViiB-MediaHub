// Tests and fixtures for spotify search identity behavior.

package db

import (
	"strings"
	"testing"
)

func TestAutomaticSearchIdentityUpgradePreservesExistingLinks(t *testing.T) {
	d, path := evidenceFixture(t)
	if err := d.SaveSong(&Song{ID: "song", Title: "S", Artist: "A", Album: "B", FilePath: path, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := d.RefreshTrackAnalysisSourceRevision("song", "current"); err != nil {
		t.Fatal(err)
	}
	if ok, err := d.ConfirmSpotifyRecording("song", referenceID, "current", true); err != nil || !ok {
		t.Fatalf("manual: %v %v", ok, err)
	}
	var definition string
	if err := d.conn.QueryRow("SELECT sql FROM sqlite_master WHERE name='track_external_identity'").Scan(&definition); err != nil {
		t.Fatal(err)
	}
	definition = strings.ReplaceAll(definition, ",'automatic_search'", "")
	definition = strings.Replace(definition, "track_external_identity", "track_external_identity_old", 1)
	if _, err := d.conn.Exec(definition + `; INSERT INTO track_external_identity_old SELECT * FROM track_external_identity;
 DROP TABLE track_external_identity; ALTER TABLE track_external_identity_old RENAME TO track_external_identity;`); err != nil {
		t.Fatal(err)
	}
	d.externalSchemaReady = false
	if err := d.EnsureExternalTrackAnalysisSchema(); err != nil {
		t.Fatal(err)
	}
	link, err := d.GetSpotifyRecording("song", "current")
	if err != nil || link == nil || link.LinkOrigin != "manual_confirmation" || link.ExternalID != referenceID {
		t.Fatalf("link lost: %+v %v", link, err)
	}
	if err := d.SaveSong(&Song{ID: "other", Title: "S", Artist: "A", Album: "B", FilePath: path + "2", AddedAt: 2}); err != nil {
		t.Fatal(err)
	}
	if err := d.RefreshTrackAnalysisSourceRevision("other", "other-source"); err != nil {
		t.Fatal(err)
	}
	if ok, err := d.SaveSpotifySearchMatch("other", referenceID, "other-source"); err != nil || !ok {
		t.Fatalf("automatic origin rejected: %v %v", ok, err)
	}
}

func TestSpotifySearchIdentityRespectsSourceAndManualPrecedence(t *testing.T) {
	d, path := evidenceFixture(t)
	if err := d.SaveSong(&Song{ID: "song", Title: "S", Artist: "A", Album: "B", FilePath: path, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := d.RefreshTrackAnalysisSourceRevision("song", "current"); err != nil {
		t.Fatal(err)
	}
	if ok, err := d.SaveSpotifySearchMatch("song", referenceID, "stale"); err != nil || ok {
		t.Fatalf("stale: %v %v", ok, err)
	}
	if ok, err := d.SaveSpotifySearchMatch("song", referenceID, "current"); err != nil || !ok {
		t.Fatalf("match: %v %v", ok, err)
	}
	if ok, err := d.ConfirmSpotifyRecording("song", "0123456789012345678901", "current", true); err != nil || !ok {
		t.Fatalf("manual: %v %v", ok, err)
	}
	if ok, err := d.SaveSpotifySearchMatch("song", referenceID, "current"); err != nil || ok {
		t.Fatalf("manual overwritten: %v %v", ok, err)
	}
	if allowed, err := d.SpotifySearchAllowed("song", "current"); err != nil || allowed {
		t.Fatalf("manual search: %v %v", allowed, err)
	}
	if err := d.DeleteSpotifyRecording("song"); err != nil {
		t.Fatal(err)
	}
	if ok, err := d.SaveSpotifySearchMatch("song", referenceID, "current"); err != nil || ok {
		t.Fatalf("suppression ignored: %v %v", ok, err)
	}
}
