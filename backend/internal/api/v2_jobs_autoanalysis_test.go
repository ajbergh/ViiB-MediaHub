package api

import (
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestQueueAutoAnalysisIsOffByDefault(t *testing.T) {
	database, _, _ := analysisCatalog(t, 1)
	api := &API{db: database}
	api.jobSchedulerOn = true // Inspect the queue rather than draining it.

	api.queueAutoAnalysis("a full scan")
	jobs, err := database.ListJobs(100, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 {
		t.Fatalf("%d jobs queued with the setting unset, want 0", len(jobs))
	}
}

func TestQueueAutoAnalysisQueuesMissingWorkWhenEnabled(t *testing.T) {
	database, _, _ := analysisCatalog(t, 1)
	api := &API{db: database}
	api.jobSchedulerOn = true
	if err := database.SetSetting(SettingAutoAnalyzeNewTracks, "true"); err != nil {
		t.Fatal(err)
	}

	api.queueAutoAnalysis("a full scan")
	jobs, err := database.ListJobs(100, db.JobStatusQueued)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("%d jobs queued, want 1", len(jobs))
	}
	if jobs[0].Type != JobTypeAnalyzeTracks {
		t.Fatalf("type = %q, want %q", jobs[0].Type, JobTypeAnalyzeTracks)
	}
	if jobs[0].Priority != autoAnalyzePriority {
		t.Fatalf("priority = %d, want %d so it yields to user requests", jobs[0].Priority, autoAnalyzePriority)
	}
	selection, err := db.ParseAnalysisSelection(jobs[0].Parameters)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Mode != db.AnalysisSelectionMissing {
		t.Fatalf("mode = %q, want %q", selection.Mode, db.AnalysisSelectionMissing)
	}
}

// Repeated scans must not stack duplicate analysis runs: a pending run already
// covers whatever the newest scan added, because the work list is expanded when
// the job is claimed rather than when it is queued.
func TestQueueAutoAnalysisDoesNotStackPendingRuns(t *testing.T) {
	database, _, _ := analysisCatalog(t, 1)
	api := &API{db: database}
	api.jobSchedulerOn = true
	if err := database.SetSetting(SettingAutoAnalyzeNewTracks, "on"); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		api.queueAutoAnalysis("a quick scan")
	}
	jobs, err := database.ListJobs(100, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("%d analysis jobs queued after three scans, want 1", len(jobs))
	}
}

func TestIsEnabledSettingAcceptsCommonSpellings(t *testing.T) {
	for _, value := range []string{"1", "true", "TRUE", " yes ", "on", "enabled"} {
		if !isEnabledSetting(value) {
			t.Errorf("isEnabledSetting(%q) = false, want true", value)
		}
	}
	for _, value := range []string{"", "0", "false", "off", "no", "maybe"} {
		if isEnabledSetting(value) {
			t.Errorf("isEnabledSetting(%q) = true, want false", value)
		}
	}
}
