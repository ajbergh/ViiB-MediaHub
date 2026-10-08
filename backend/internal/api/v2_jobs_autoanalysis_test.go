// Tests and fixtures for v2 jobs autoanalysis behavior.

package api

import (
	"sync"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestQueueAutoAnalysisIsOnByDefault(t *testing.T) {
	database, _, _ := analysisCatalog(t, 1)
	api := &API{db: database}
	api.jobSchedulerOn = true // Inspect the queue rather than draining it.

	api.queueAutoAnalysis("a full scan")
	jobs, err := database.ListJobs(100, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].Type != JobTypeAnalyzeTracks {
		t.Fatalf("jobs with the setting unset = %#v, want one analysis job", jobs)
	}
}

func TestQueueAutoAnalysisHonorsExplicitDisable(t *testing.T) {
	database, _, _ := analysisCatalog(t, 1)
	api := &API{db: database}
	api.jobSchedulerOn = true
	if err := database.SetSetting(SettingAutoAnalyzeNewTracks, "false"); err != nil {
		t.Fatal(err)
	}
	api.queueAutoAnalysis("a full scan")
	jobs, err := database.ListJobs(100, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 {
		t.Fatalf("%d jobs queued when auto-analysis is disabled, want 0", len(jobs))
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

func TestQueueAutoAnalysisSnapshotsAutomaticCueMode(t *testing.T) {
	database, _, _ := analysisCatalog(t, 1)
	api := &API{db: database, jobSchedulerOn: true}
	if err := database.SetSetting(SettingAutoAnalyzeNewTracks, "true"); err != nil {
		t.Fatal(err)
	}
	if err := database.SetSetting(SettingAutoCueMode, "suggest"); err != nil {
		t.Fatal(err)
	}
	api.queueAutoAnalysis("a scan")
	jobs, err := database.ListJobs(100, db.JobStatusQueued)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("queued jobs = %#v err=%v", jobs, err)
	}
	selection, err := db.ParseAnalysisSelection(jobs[0].Parameters)
	if err != nil || selection.AutoCueMode != db.AutomaticCuePointsSuggest {
		t.Fatalf("auto-analysis job mode = %#v err=%v, want suggest snapshot", selection, err)
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

func TestQueueAutoAnalysisAddsOneFollowupBehindRunningRun(t *testing.T) {
	database, _, _ := analysisCatalog(t, 1)
	api := &API{db: database, jobSchedulerOn: true}
	if err := database.CreateJob(db.Job{ID: "active", Type: JobTypeAnalyzeTracks, Status: db.JobStatusRunning, Parameters: autoAnalysisParameters(db.AnalysisSelection{Mode: db.AnalysisSelectionMissing})}); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		api.queueAutoAnalysis("scan during analysis")
	}
	queued, err := database.ListJobs(100, db.JobStatusQueued)
	if err != nil || len(queued) != 1 {
		t.Fatal(queued, err)
	}
	active, err := database.GetJob("active")
	if err != nil || active.Status != db.JobStatusRunning {
		t.Fatal(active, err)
	}
}
func TestQueueAutoAnalysisExplicitSelectionDoesNotCoverScan(t *testing.T) {
	database, _, ids := analysisCatalog(t, 1)
	api := &API{db: database, jobSchedulerOn: true}
	if err := database.CreateJob(db.Job{ID: "explicit", Type: JobTypeAnalyzeTracks, Status: db.JobStatusQueued, Parameters: autoAnalysisParameters(db.AnalysisSelection{Mode: db.AnalysisSelectionIDs, SongIDs: ids})}); err != nil {
		t.Fatal(err)
	}
	api.queueAutoAnalysis("scan")
	jobs, err := database.ListJobs(100, db.JobStatusQueued)
	if err != nil || len(jobs) != 2 {
		t.Fatal(jobs, err)
	}
}

func TestConcurrentScanTriggersCoalesceFollowup(t *testing.T) {
	database, _, _ := analysisCatalog(t, 1)
	api := &API{db: database, jobSchedulerOn: true}
	if err := database.CreateJob(db.Job{ID: "active", Type: JobTypeAnalyzeTracks, Status: db.JobStatusRunning}); err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for range 8 {
		group.Add(1)
		go func() { defer group.Done(); api.queueAutoAnalysis("concurrent scan") }()
	}
	group.Wait()
	jobs, err := database.ListJobs(100, db.JobStatusQueued)
	if err != nil || len(jobs) != 1 {
		t.Fatal(jobs, err)
	}
}
