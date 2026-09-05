package scanner

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestNotifyDownloadCompleteScansDownloadedAudioAtThreshold(t *testing.T) {
	sc, database, tempDir := newDownloadRescanTestScanner(t)
	downloadDir := filepath.Join(tempDir, "downloads")
	audioPath := filepath.Join(downloadDir, "downloaded.wav")
	writeTestWAV(t, audioPath)
	sc.SetSpotifyDownloadDir(downloadDir)
	sc.SetRescanThreshold(2)

	events := sc.Subscribe()
	t.Cleanup(func() { sc.Unsubscribe(events) })

	if sc.NotifyDownloadComplete() {
		t.Fatal("first completed download should not reach threshold 2")
	}
	if !sc.NotifyDownloadComplete() {
		t.Fatal("second completed download should schedule a quick scan")
	}
	waitForAutomaticScan(t, events)

	songs, err := database.GetAllSongs()
	if err != nil {
		t.Fatalf("get scanned songs: %v", err)
	}
	if len(songs) != 1 || filepath.Clean(songs[0].FilePath) != filepath.Clean(audioPath) {
		t.Fatalf("expected downloaded audio %s in library, got %+v", audioPath, songs)
	}

	folders, err := database.GetScanFolders()
	if err != nil {
		t.Fatalf("get scan folders: %v", err)
	}
	if len(folders) != 1 || filepath.Clean(folders[0].Path) != filepath.Clean(downloadDir) {
		t.Fatalf("expected download directory %s to be registered, got %+v", downloadDir, folders)
	}
}

func TestNotifyDownloadCompleteWaitsForActiveScan(t *testing.T) {
	sc, database, tempDir := newDownloadRescanTestScanner(t)
	downloadDir := filepath.Join(tempDir, "downloads")
	audioPath := filepath.Join(downloadDir, "queued.wav")
	writeTestWAV(t, audioPath)
	sc.SetSpotifyDownloadDir(downloadDir)
	sc.SetRescanThreshold(1)

	events := sc.Subscribe()
	t.Cleanup(func() { sc.Unsubscribe(events) })
	if !sc.TryBeginScan() {
		t.Fatal("failed to reserve scanner for test")
	}
	if !sc.NotifyDownloadComplete() {
		t.Fatal("completed download should queue an automatic scan")
	}

	select {
	case event := <-events:
		t.Fatalf("automatic scan should wait while another scan is active, got event %+v", event)
	case <-time.After(250 * time.Millisecond):
	}

	sc.EndScan()
	waitForAutomaticScan(t, events)

	songs, err := database.GetAllSongs()
	if err != nil {
		t.Fatalf("get scanned songs: %v", err)
	}
	if len(songs) != 1 || filepath.Clean(songs[0].FilePath) != filepath.Clean(audioPath) {
		t.Fatalf("queued automatic scan did not add %s: %+v", audioPath, songs)
	}
}

func TestNotifyDownloadCompleteRetainsDownloadsFinishedDuringScan(t *testing.T) {
	sc, database, tempDir := newDownloadRescanTestScanner(t)
	downloadDir := filepath.Join(tempDir, "downloads")
	firstPath := filepath.Join(downloadDir, "first.wav")
	secondPath := filepath.Join(downloadDir, "second.wav")
	writeTestWAV(t, firstPath)
	sc.SetSpotifyDownloadDir(downloadDir)
	sc.SetRescanThreshold(1)

	events := sc.Subscribe()
	t.Cleanup(func() { sc.Unsubscribe(events) })
	if !sc.NotifyDownloadComplete() {
		t.Fatal("first completed download should schedule a quick scan")
	}
	waitForEventType(t, events, "scan_started")

	writeTestWAV(t, secondPath)
	sc.NotifyDownloadComplete()
	waitForEventType(t, events, "scan_complete")
	waitForEventType(t, events, "scan_complete")

	songs, err := database.GetAllSongs()
	if err != nil {
		t.Fatalf("get scanned songs: %v", err)
	}
	if len(songs) != 2 {
		t.Fatalf("expected both completed downloads in library, got %+v", songs)
	}
}

func TestNotifyDownloadCompleteDisabledAtZero(t *testing.T) {
	sc, _, tempDir := newDownloadRescanTestScanner(t)
	sc.SetSpotifyDownloadDir(filepath.Join(tempDir, "downloads"))
	sc.SetRescanThreshold(0)

	if sc.NotifyDownloadComplete() {
		t.Fatal("threshold 0 should disable automatic scans")
	}

	sc.rescanMutex.Lock()
	defer sc.rescanMutex.Unlock()
	if sc.downloadsSinceLastScan != 0 || sc.autoScanWorkerRunning {
		t.Fatalf("disabled scanner retained work: downloads=%d worker=%v", sc.downloadsSinceLastScan, sc.autoScanWorkerRunning)
	}
}

func TestEnsureDownloadDirIsScannedDoesNotMistakeNestedRootForCoverage(t *testing.T) {
	sc, database, tempDir := newDownloadRescanTestScanner(t)
	downloadDir := filepath.Join(tempDir, "downloads")
	nestedRoot := filepath.Join(downloadDir, "one-album")
	if err := os.MkdirAll(nestedRoot, 0o700); err != nil {
		t.Fatalf("create nested scan root: %v", err)
	}
	if err := database.AddScanFolder(&db.ScanFolder{ID: "nested", Path: nestedRoot, AddedAt: time.Now().UnixMilli()}); err != nil {
		t.Fatalf("add nested scan root: %v", err)
	}

	if err := sc.ensureDownloadDirIsScanned(downloadDir); err != nil {
		t.Fatalf("ensure download directory coverage: %v", err)
	}
	folders, err := database.GetScanFolders()
	if err != nil {
		t.Fatalf("get scan folders: %v", err)
	}
	if len(folders) != 2 {
		t.Fatalf("expected download root to be added alongside nested root, got %+v", folders)
	}
}

func newDownloadRescanTestScanner(t *testing.T) (*Scanner, *db.DB, string) {
	t.Helper()
	tempDir := t.TempDir()
	database, err := db.New(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	sc := New(database, tempDir)
	t.Cleanup(sc.Close)
	return sc, database, tempDir
}

func waitForAutomaticScan(t *testing.T, events <-chan LibraryEvent) {
	t.Helper()
	event := waitForEventType(t, events, "scan_complete")
	message := strings.ToLower(event.Message)
	if strings.Contains(message, "fail") || strings.Contains(message, "could not") {
		t.Fatalf("automatic scan failed: %s", event.Message)
	}
}

func waitForEventType(t *testing.T, events <-chan LibraryEvent, eventType string) LibraryEvent {
	t.Helper()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	for {
		select {
		case event := <-events:
			if event.Type != eventType {
				continue
			}
			return event
		case <-timer.C:
			t.Fatalf("timed out waiting for scanner event %q", eventType)
		}
	}
}

func writeTestWAV(t *testing.T, path string) {
	t.Helper()
	const (
		sampleRate    = uint32(8000)
		channels      = uint16(1)
		bitsPerSample = uint16(16)
		sampleCount   = 800
	)
	dataSize := uint32(sampleCount) * uint32(channels) * uint32(bitsPerSample/8)

	var wav bytes.Buffer
	wav.WriteString("RIFF")
	_ = binary.Write(&wav, binary.LittleEndian, uint32(36)+dataSize)
	wav.WriteString("WAVEfmt ")
	_ = binary.Write(&wav, binary.LittleEndian, uint32(16))
	_ = binary.Write(&wav, binary.LittleEndian, uint16(1))
	_ = binary.Write(&wav, binary.LittleEndian, channels)
	_ = binary.Write(&wav, binary.LittleEndian, sampleRate)
	byteRate := sampleRate * uint32(channels) * uint32(bitsPerSample/8)
	_ = binary.Write(&wav, binary.LittleEndian, byteRate)
	_ = binary.Write(&wav, binary.LittleEndian, channels*(bitsPerSample/8))
	_ = binary.Write(&wav, binary.LittleEndian, bitsPerSample)
	wav.WriteString("data")
	_ = binary.Write(&wav, binary.LittleEndian, dataSize)
	wav.Write(make([]byte, dataSize))

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create test audio directory: %v", err)
	}
	if err := os.WriteFile(path, wav.Bytes(), 0o600); err != nil {
		t.Fatalf("write test WAV: %v", err)
	}
}
