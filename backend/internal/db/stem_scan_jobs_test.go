package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
)

func TestStemScanRequestsCoalesceAndSerialize(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.EnsureJobSchema(); err != nil {
		t.Fatal(err)
	}
	parameters := json.RawMessage(`{"locations":["stems"]}`)
	type result struct {
		id  string
		err error
	}
	results := make(chan result, 8)
	for i := 0; i < cap(results); i++ {
		go func(i int) {
			id, err := database.QueueStemLibraryScan(Job{ID: fmt.Sprintf("scan-%d", i), Parameters: parameters, Priority: 10})
			results <- result{id, err}
		}(i)
	}
	var activeID string
	for i := 0; i < cap(results); i++ {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		if activeID != "" && result.id != activeID {
			t.Fatalf("concurrent requests created different jobs: %s, %s", activeID, result.id)
		}
		activeID = result.id
	}
	active, err := database.ClaimNextQueuedJob("Starting")
	if err != nil || active.ID != activeID {
		t.Fatalf("unexpected active scan: %+v err=%v", active, err)
	}
	// A changed configuration needs another pass, but it must not overlap.
	followUpID, err := database.QueueStemLibraryScan(Job{ID: "changed-roots", Parameters: json.RawMessage(`{"locations":["other"]}`), Priority: 50})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ClaimNextQueuedJob("Overlapping scan"); err != sql.ErrNoRows {
		t.Fatalf("overlapping stem scan was claimable: %v", err)
	}
	if err := database.StartJob(followUpID, "Bypass scheduler"); err != sql.ErrNoRows {
		t.Fatalf("direct start bypassed serialization: %v", err)
	}
	if err := database.CompleteJob(activeID, nil, "Done"); err != nil {
		t.Fatal(err)
	}
	next, err := database.ClaimNextQueuedJob("Starting follow-up")
	if err != nil || next.ID != followUpID {
		t.Fatalf("follow-up scan stranded: %+v err=%v", next, err)
	}
}
