package db

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
)

func TestSpotifyPublicationWaitsBeforeCheckingFence(t *testing.T) {
	for _, kind := range []string{"snapshot", "rest_snapshot", "playlist", "waveform", "analysis", "analysis_status", "resource_status", "confirm_owner"} {
		for _, supersede := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/supersede=%v", kind, supersede), func(t *testing.T) {
				d, _ := evidenceFixture(t)
				if _, err := d.ReserveSpotifyMetadataRuntime("epoch"); err != nil {
					t.Fatal(err)
				}
				if _, err := d.ConfirmSpotifyMetadataOwner("epoch", "oauth", "account", "account-a"); err != nil {
					t.Fatal(err)
				}
				if err := d.EnsureExternalTrackAnalysisSchema(); err != nil {
					t.Fatal(err)
				}
				fence := SpotifyMetadataFence{Epoch: "epoch", ContextKey: "account-a"}
				now := time.Now()
				snapshot := SpotifyEntitySnapshot{SpotifySnapshotKey: SpotifySnapshotKey{EntityType: "track", SpotifyID: referenceID, Resource: "catalog", ContextKey: fence.ContextKey}, SchemaVersion: 1, AdapterRevision: "test", RetrievedAt: now, ExpiresAt: now.Add(time.Hour), Payload: []byte(`{"name":"current"}`)}
				traversal, err := d.BeginSpotifyPlaylistTraversalForRuntime(fence, referenceID)
				if err != nil {
					t.Fatal(err)
				}
				observation := detailedObservation(t, `{"track":{"tempo":120,"duration":10},"sections":[{"start":0,"duration":10}]}`)
				observation.RetrievedAt = now
				waveform := waveformFixture(t)
				write := func() error {
					switch kind {
					case "snapshot":
						return d.PutSpotifyEntitySnapshotsForRuntime(fence, []SpotifyEntitySnapshot{snapshot})
					case "rest_snapshot":
						snapshot.Resource = "rest:catalog"
						return d.PutSpotifyEntitySnapshots([]SpotifyEntitySnapshot{snapshot})
					case "playlist":
						snapshot.EntityType = "playlist"
						return d.PutSpotifyPlaylistSnapshots(traversal, []SpotifyEntitySnapshot{snapshot}, true)
					case "waveform":
						return d.PutSpotifyWaveformForRuntime(fence, referenceID, waveform, "fixture", now, now.Add(time.Hour))
					case "analysis":
						return d.PutExternalAnalysisForRuntime(fence, observation, "fixture", now.Add(time.Hour))
					case "analysis_status":
						return d.PutExternalAnalysisStatusForRuntime(fence, referenceID, "audio_features", ExternalAnalysisStatus{Code: spotifyanalysis.RateLimited, CheckedAt: now, RetryAt: now.Add(time.Hour)})
					case "resource_status":
						return d.PutSpotifyMetadataResourceStatusForRuntime(fence, snapshot.SpotifySnapshotKey, SpotifyMetadataResourceStatus{State: "unavailable", CheckedAt: now, RetryAt: now.Add(time.Hour)})
					default:
						_, err := d.ConfirmSpotifyMetadataOwner("epoch", "oauth", "account", "unused")
						return err
					}
				}
				writer, err := d.conn.Begin()
				if err != nil {
					t.Fatal(err)
				}
				defer writer.Rollback()
				if _, err := writer.Exec(`UPDATE settings SET value=value WHERE 0`); err != nil {
					t.Fatal(err)
				}
				if supersede {
					if _, err := writer.Exec(`UPDATE settings SET value='new-epoch' WHERE key='spotify_metadata_runtime_epoch'`); err != nil {
						t.Fatal(err)
					}
				}
				result := make(chan error, 1)
				go func() { result <- write() }()
				// A deferred reader would read the old fence then fail its upgrade
				// immediately. Publication must instead wait at writer acquisition.
				select {
				case err := <-result:
					t.Fatalf("publication did not wait for writer: %v", err)
				case <-time.After(100 * time.Millisecond):
				}
				if err := writer.Commit(); err != nil {
					t.Fatal(err)
				}
				select {
				case err := <-result:
					if supersede && kind != "rest_snapshot" {
						if !errors.Is(err, ErrSpotifyMetadataRuntimeSuperseded) {
							t.Fatalf("stale publication: %v", err)
						}
					} else if err != nil {
						t.Fatal(err)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("publication remained blocked after writer committed")
				}
			})
		}
	}
}

func TestSpotifySnapshotsPublishAlongsideDownloadsAndAnalysis(t *testing.T) {
	d, path := evidenceFixture(t)
	if _, err := d.ReserveSpotifyMetadataRuntime("epoch"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ConfirmSpotifyMetadataOwner("epoch", "oauth", "account", "account-a"); err != nil {
		t.Fatal(err)
	}
	if err := d.EnsureExternalTrackAnalysisSchema(); err != nil {
		t.Fatal(err)
	}
	if err := d.AddDownload(&SpotifyDownload{ID: "concurrent", SpotifyID: referenceID, Type: "track", Status: "queued"}); err != nil {
		t.Fatal(err)
	}
	if changed, err := d.MarkDownloadStarted("concurrent"); err != nil || !changed {
		t.Fatalf("start: %v %v", changed, err)
	}
	fence := SpotifyMetadataFence{Epoch: "epoch", ContextKey: "account-a"}
	observation := detailedObservation(t, `{"track":{"tempo":120,"duration":10},"sections":[{"start":0,"duration":10}]}`)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	const workers = 6
	results := make(chan error, workers+2)
	start := make(chan struct{})
	for worker := range workers {
		go func(worker int) {
			<-start
			for i := range 12 {
				now := time.Now()
				snapshot := SpotifyEntitySnapshot{SpotifySnapshotKey: SpotifySnapshotKey{EntityType: "track", SpotifyID: fmt.Sprintf("%022d", worker+1), Resource: "rest:catalog", ContextKey: fence.ContextKey}, SchemaVersion: 1, AdapterRevision: "test", RetrievedAt: now, ExpiresAt: now.Add(time.Hour), Payload: []byte(`{}`), Relations: []SpotifyEntityRelation{{Kind: "artist", Position: i, ChildType: "artist", ChildID: referenceID}}}
				if err := d.PutSpotifyEntitySnapshotsForRuntime(fence, []SpotifyEntitySnapshot{snapshot}); err != nil {
					results <- err
					return
				}
			}
			results <- nil
		}(worker)
	}
	go func() {
		<-start
		_, err := d.MarkDownloadCompletedWithEvidence(ctx, "concurrent", path)
		results <- err
	}()
	go func() {
		<-start
		for range 12 {
			observation.RetrievedAt = time.Now()
			if err := d.PutExternalAnalysisForRuntime(fence, observation, "fixture", observation.RetrievedAt.Add(time.Hour)); err != nil {
				results <- err
				return
			}
		}
		results <- nil
	}()
	close(start)
	for range workers + 2 {
		select {
		case err := <-results:
			if err != nil {
				t.Error(err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	var snapshots, relations int
	if err := d.conn.QueryRow(`SELECT count(*) FROM spotify_entity_snapshots WHERE resource='rest:catalog'`).Scan(&snapshots); err != nil {
		t.Fatal(err)
	}
	if err := d.conn.QueryRow(`SELECT count(*) FROM spotify_entity_relations WHERE resource='rest:catalog'`).Scan(&relations); err != nil {
		t.Fatal(err)
	}
	if snapshots != workers || relations != workers*12 {
		t.Fatalf("partial snapshot publication: snapshots=%d relations=%d", snapshots, relations)
	}
	download, err := d.GetDownload("concurrent")
	if err != nil || download.Status != "completed" {
		t.Fatalf("download=%+v err=%v", download, err)
	}
}
