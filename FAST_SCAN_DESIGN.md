# Ultra-Fast Startup Incremental Scan System Design

## Executive Summary

This document describes a comprehensive redesign of ViiB MediaHub's library scanning system to achieve near-instant startup times by leveraging filesystem journals, directory-level signatures, persistent metadata indexing, and background processing.

**Goal**: Reduce startup scan time from O(n) full file walk to O(Δ) incremental change detection, where Δ represents only the changed files since last scan.

---

## Implementation Status

### Phase 1: Foundation ✅ COMPLETED
**Completed: 2024**

| Component | Status | Location |
|-----------|--------|----------|
| Database tables for signatures | ✅ Done | `backend/internal/db/db.go` |
| Database tables for scan state | ✅ Done | `backend/internal/db/db.go` |
| Database tables for metadata cache | ✅ Done | `backend/internal/db/db.go` |
| DirectorySignature struct | ✅ Done | `backend/internal/db/db.go` |
| ScanState struct | ✅ Done | `backend/internal/db/db.go` |
| FileMetadataCache struct | ✅ Done | `backend/internal/db/db.go` |
| DB methods for signatures (CRUD + batch) | ✅ Done | `backend/internal/db/db.go` |
| DB methods for scan state | ✅ Done | `backend/internal/db/db.go` |
| DB methods for metadata cache | ✅ Done | `backend/internal/db/db.go` |
| Directory signature computation | ✅ Done | `backend/internal/scanner/fast_scan.go` |
| Cheap change detection (mtime + size) | ✅ Done | `backend/internal/scanner/fast_scan.go` |
| QuickStartup() method | ✅ Done | `backend/internal/scanner/fast_scan.go` |
| ProcessChanges() method | ✅ Done | `backend/internal/scanner/fast_scan.go` |
| ScanAll saves scan state | ✅ Done | `backend/internal/scanner/scanner.go` |
| ScanAll saves directory signatures | ✅ Done | `backend/internal/scanner/scanner.go` |
| File metadata cache updates | ✅ Done | `backend/internal/scanner/scanner.go` |

**New Files Created:**
- `backend/internal/scanner/fast_scan.go` - Fast scan implementation

**Key Features Implemented:**
- Directory signatures using content hash of (name, size, mtime) tuples
- Signature-based quick startup that can skip unchanged directories
- Cheap mtime + size change detection without opening files
- Full scan now saves state for future quick startups
- Batch operations for efficient database writes

### Phase 2: Platform Journals ✅ COMPLETED
**Completed: 2024**

| Component | Status | Location |
|-----------|--------|----------|
| JournalChangeDetector interface | ✅ Done | `backend/internal/scanner/journal.go` |
| Windows USN journal reader | ✅ Done | `backend/internal/scanner/journal_windows.go` |
| macOS FSEvents integration | ✅ Done | `backend/internal/scanner/journal_darwin.go` |
| Linux mtime-based fallback | ✅ Done | `backend/internal/scanner/journal_linux.go` |
| Universal mtime detector | ✅ Done | `backend/internal/scanner/journal_mtime.go` |
| Platform-specific detector selection (Windows) | ✅ Done | `backend/internal/scanner/journal_platform_windows.go` |
| Platform-specific detector selection (macOS) | ✅ Done | `backend/internal/scanner/journal_platform_darwin.go` |
| Platform-specific detector selection (Linux) | ✅ Done | `backend/internal/scanner/journal_platform_linux.go` |
| QuickStartup uses journal detection | ✅ Done | `backend/internal/scanner/fast_scan.go` |

**New Files Created:**
- `backend/internal/scanner/journal.go` - JournalChangeDetector interface
- `backend/internal/scanner/journal_windows.go` - Windows USN implementation
- `backend/internal/scanner/journal_darwin.go` - macOS FSEvents implementation
- `backend/internal/scanner/journal_linux.go` - Linux optimized mtime
- `backend/internal/scanner/journal_mtime.go` - Universal mtime fallback
- `backend/internal/scanner/journal_platform_*.go` - Platform-specific factory methods

**Key Features Implemented:**
- **Windows**: Full USN journal support with volume handling, journal querying, and change classification
- **macOS**: Optimized mtime-based scanning (pure-Go, no CGO required)
- **Linux**: mtime + ctime-based detection with directory signature integration
- **Universal**: mtime fallback detector for all platforms
- Detection priority: Journal → Signatures → mtime fallback
- State persistence for journal cursors (USN position, FSEvents ID, mtime)

### Phase 3: Background Processing ✅ COMPLETED
**Completed: 2024**

| Component | Status | Location |
|-----------|--------|----------|
| PriorityQueue (min-heap) | ✅ Done | `backend/internal/scanner/background.go` |
| BackgroundTask types | ✅ Done | `backend/internal/scanner/background.go` |
| ScanPriority levels | ✅ Done | `backend/internal/scanner/background.go` |
| TaskType definitions | ✅ Done | `backend/internal/scanner/background.go` |
| BackgroundScanner struct | ✅ Done | `backend/internal/scanner/background.go` |
| Worker pool with goroutines | ✅ Done | `backend/internal/scanner/background.go` |
| Pause/Resume functionality | ✅ Done | `backend/internal/scanner/background.go` |
| Graceful shutdown | ✅ Done | `backend/internal/scanner/background.go` |
| SSE event integration | ✅ Done | `backend/internal/scanner/background.go` |
| Scanner integration | ✅ Done | `backend/internal/scanner/scanner.go` |
| Background control methods | ✅ Done | `backend/internal/scanner/fast_scan.go` |
| Background validation | ✅ Done | `backend/internal/scanner/fast_scan.go` |

**New Files Created:**
- `backend/internal/scanner/background.go` - Background processing system

**Key Features Implemented:**
- **Priority Queue**: Min-heap implementation with 4 priority levels (Immediate, High, Normal, Low)
- **Task Types**: ProcessFile, RescanDirectory, ValidateFile, DeleteFile, ComputeSignature
- **Worker Pool**: Configurable worker count (default 2), context-based shutdown
- **Pause/Resume**: Atomic state management, condition variable signaling
- **Retry Logic**: Failed tasks demoted to Low priority, max 3 attempts
- **SSE Events**: `background_scanner` (state changes), `background_progress` (task completion)
- **Scanner Integration**: Auto-start on Scanner.New(), QueueChangesForBackground(), StartBackgroundValidation()

### Phase 4: Optimization ✅ COMPLETED
**Completed: December 2025**

| Component | Status | Location |
|-----------|--------|----------|
| SignatureConfig (granularity tuning) | ✅ Done | `backend/internal/scanner/optimization.go` |
| Signature expiry management | ✅ Done | `backend/internal/scanner/optimization.go` |
| IntegrityVerifier (sampling) | ✅ Done | `backend/internal/scanner/optimization.go` |
| IntegrityResult & Issue types | ✅ Done | `backend/internal/scanner/optimization.go` |
| ScanMetrics (performance profiling) | ✅ Done | `backend/internal/scanner/optimization.go` |
| MetricsCollector with timers | ✅ Done | `backend/internal/scanner/optimization.go` |
| Scan history tracking | ✅ Done | `backend/internal/scanner/optimization.go` |
| ComputePartialHash (4KB hashing) | ✅ Done | `backend/internal/scanner/optimization.go` |
| ScanScheduler (adaptive timing) | ✅ Done | `backend/internal/scanner/optimization.go` |
| CleanupStaleSignatures | ✅ Done | `backend/internal/scanner/optimization.go` |
| CleanupStaleMetadataCache | ✅ Done | `backend/internal/scanner/optimization.go` |
| Weighted random file selection | ✅ Done | `backend/internal/scanner/optimization.go` |
| DeleteDirectorySignatures (batch) | ✅ Done | `backend/internal/db/db.go` |

**New Files Created:**
- `backend/internal/scanner/optimization.go` - Phase 4 optimization system

**Key Features Implemented:**
- **Signature Granularity**: Configurable depth, min files, expiry, and partial hashing
- **Integrity Verification**: Sample-based verification with issue detection (missing, corrupted, mismatched files)
- **Performance Profiling**: ScanMetrics tracking with duration breakdowns, scan history (100 entries)
- **Partial Hash**: Fast 4KB+4KB hash computation for large files
- **Adaptive Scheduling**: Auto-adjusts scan intervals based on change frequency (5min-1hr range)
- **Stale Cleanup**: Removes signatures and cache entries for deleted directories/files
- **Weighted Selection**: Prioritizes older unverified files for background validation

---

## Current State Analysis

### Existing Implementation
- **Full directory walk** on every scan using `filepath.Walk()`
- **Per-file metadata extraction** using TagLib for every file
- **SQLite storage** with file_path unique constraint
- **File hash** stored but not used for change detection
- **No filesystem monitoring** - relies on manual/scheduled rescans

### Performance Bottlenecks
1. **I/O Overhead**: Walking entire directory tree for every scan
2. **Metadata Parsing**: TagLib extraction for every file, even unchanged
3. **Database Queries**: Checking if each file exists in DB
4. **No Incremental Knowledge**: No way to know what changed since last scan

---

## Proposed Architecture

### 1. Multi-Layer Change Detection

```
┌─────────────────────────────────────────────────────────────────┐
│                    STARTUP SCAN FLOW                             │
├─────────────────────────────────────────────────────────────────┤
│  Layer 1: Quick Signature Check                                  │
│  ├── Compare stored directory signatures with current state      │
│  └── If signatures match → Skip directory entirely               │
│                                                                   │
│  Layer 2: Filesystem Journal Query (USN/FSEvents/inotify)       │
│  ├── Query OS for changes since last scan timestamp              │
│  └── Get list of modified/created/deleted paths                  │
│                                                                   │
│  Layer 3: Cheap File-Level Check (fallback)                      │
│  ├── Compare mtime + size for suspect files                      │
│  └── Only parse metadata if file actually changed                │
│                                                                   │
│  Layer 4: Background Full Scan (verification)                    │
│  └── Low-priority background thread validates integrity          │
└─────────────────────────────────────────────────────────────────┘
```

---

## Component Design

### 2.1 Directory Signature System

**Purpose**: Quickly detect if a directory has any changes without walking its contents.

```go
// DirectorySignature stores a compact representation of directory state
type DirectorySignature struct {
    Path           string    `json:"path"`
    FileCount      int       `json:"fileCount"`
    TotalSize      int64     `json:"totalSize"`
    LatestMtime    time.Time `json:"latestMtime"`
    ContentHash    string    `json:"contentHash"`    // Hash of sorted (name, size, mtime) tuples
    LastVerified   time.Time `json:"lastVerified"`
}
```

**How it works**:
1. At scan completion, compute signatures for each scanned directory
2. At startup, read signatures from SQLite
3. For each directory, compute current quick stats (file count, total size, latest mtime)
4. If stats match, skip the directory entirely
5. If stats differ, mark directory for deeper inspection

**Benefits**:
- Single `os.ReadDir()` call per directory (vs recursive walk)
- No file opening or metadata parsing
- Can skip entire subtrees if parent matches

### 2.2 Filesystem Journal Integration

#### Windows: USN (Update Sequence Number) Journal

```go
// USNChangeDetector queries Windows NTFS journals for changes
type USNChangeDetector struct {
    lastUSN        uint64              // Last processed USN
    volumeHandle   windows.Handle      // Handle to volume
    journalData    *USN_JOURNAL_DATA   // Journal state
}

// GetChangesSince returns all filesystem changes since the last scan
func (u *USNChangeDetector) GetChangesSince(lastScan time.Time) ([]FileChange, error) {
    // Use DeviceIoControl with FSCTL_READ_USN_JOURNAL
    // Filter by timestamp and supported extensions
    // Return: created, modified, deleted, renamed paths
}
```

**Windows USN Benefits**:
- Kernel-level change tracking - no events missed
- Can query historical changes even after app restart
- Filters by file extension at kernel level
- Sub-millisecond query for thousands of changes

#### macOS: FSEvents API

```go
// FSEventsDetector uses macOS FSEvents for change detection
type FSEventsDetector struct {
    lastEventID    uint64              // Last processed event ID
    sinceWhen      FSEventStreamEventId
    devNo          dev_t               // Device number for persistence
}

// GetChangesSince queries FSEvents for changes
func (f *FSEventsDetector) GetChangesSince(lastScan time.Time) ([]FileChange, error) {
    // Use FSEventStreamCreate with kFSEventStreamEventFlagHistoryDone
    // Parse event flags for creation, modification, deletion
    // Return consolidated change list
}
```

**macOS FSEvents Benefits**:
- Persistent event IDs survive reboots
- Can request historical events from system log
- Directory-level granularity with file-level fallback

#### Linux: inotify + fanotify

```go
// LinuxChangeDetector combines inotify for real-time + journal fallback
type LinuxChangeDetector struct {
    inotifyFd      int                 // inotify file descriptor
    watchDirs      map[string]int      // path -> watch descriptor
    lastScanTime   time.Time           // Fallback for missed events
}

// GetChangesSince uses mtime scan as fallback (inotify doesn't persist)
func (l *LinuxChangeDetector) GetChangesSince(lastScan time.Time) ([]FileChange, error) {
    // inotify doesn't persist, so use mtime-based detection
    // Walk directories and compare mtime > lastScan
    // Much faster than full metadata parse
}
```

**Linux Limitations**:
- inotify events don't persist across restarts
- Falls back to mtime-based detection
- fanotify requires CAP_SYS_ADMIN

### 2.3 Persistent Metadata Index

**Database Schema Extensions**:

```sql
-- Directory signatures for quick change detection
CREATE TABLE directory_signatures (
    path TEXT PRIMARY KEY,
    file_count INTEGER NOT NULL,
    total_size INTEGER NOT NULL,
    latest_mtime INTEGER NOT NULL,
    content_hash TEXT NOT NULL,
    last_verified INTEGER NOT NULL
);

-- Scan state for journal-based detection
CREATE TABLE scan_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    last_scan_time INTEGER NOT NULL,
    windows_usn INTEGER,              -- Windows: last USN number
    macos_event_id INTEGER,           -- macOS: last FSEvents ID
    linux_last_mtime INTEGER,         -- Linux: fallback mtime
    scan_duration_ms INTEGER,
    files_scanned INTEGER,
    files_changed INTEGER
);

-- File metadata cache (extends existing songs table)
CREATE TABLE file_metadata_cache (
    file_path TEXT PRIMARY KEY,
    file_size INTEGER NOT NULL,
    mtime INTEGER NOT NULL,
    metadata_hash TEXT,               -- Hash of extracted metadata
    last_verified INTEGER NOT NULL,
    FOREIGN KEY (file_path) REFERENCES songs(file_path)
);

CREATE INDEX idx_metadata_mtime ON file_metadata_cache(mtime);
```

### 2.4 Cheap Change Detectors

**Purpose**: Determine if a file needs re-parsing without opening it.

```go
// CheapChangeCheck performs fast change detection without metadata parsing
type CheapChangeCheck struct {
    Path           string
    CachedSize     int64
    CachedMtime    time.Time
    CachedMetaHash string
}

// NeedsReparse checks if file needs metadata re-extraction
func (c *CheapChangeCheck) NeedsReparse() (bool, error) {
    stat, err := os.Stat(c.Path)
    if err != nil {
        if os.IsNotExist(err) {
            return false, ErrFileDeleted
        }
        return true, err
    }
    
    // Quick check: if size and mtime unchanged, skip
    if stat.Size() == c.CachedSize && stat.ModTime().Equal(c.CachedMtime) {
        return false, nil
    }
    
    // File changed, needs reparse
    return true, nil
}
```

**Three-Tier Verification**:
1. **Tier 1 (Instant)**: mtime + size check (< 1μs per file)
2. **Tier 2 (Fast)**: First 4KB hash check (< 100μs per file)
3. **Tier 3 (Full)**: Complete metadata extraction (~ 5-50ms per file)

### 2.5 Background Processing Pipeline

```go
// BackgroundScanner handles deferred and validation work
type BackgroundScanner struct {
    priorityQueue   *PriorityQueue     // High-priority: new files
    validationQueue *Queue             // Low-priority: verify existing
    workers         int
    pauseOnUserActivity bool           // Pause when user is active
}

// ScanPriority determines processing order
type ScanPriority int
const (
    PriorityImmediate ScanPriority = iota  // New files from journal
    PriorityHigh                            // Modified files
    PriorityNormal                          // Periodic validation
    PriorityLow                             // Deep integrity check
)
```

---

## Startup Flow Implementation

### Phase 1: Instant Startup (< 100ms)

```go
func (s *Scanner) QuickStartup() error {
    // 1. Load scan state from database
    state, err := s.db.GetScanState()
    if err != nil {
        return s.FallbackFullScan()
    }
    
    // 2. Query filesystem journal for changes
    detector := s.getChangeDetector() // Platform-specific
    changes, err := detector.GetChangesSince(state.LastScanTime)
    if err != nil {
        // Journal unavailable, fall back to signature check
        return s.SignatureBasedScan()
    }
    
    // 3. Process immediate changes
    if len(changes) > 0 {
        s.processChanges(changes) // Background worker
    }
    
    // 4. UI is ready - library appears up-to-date
    return nil
}
```

### Phase 2: Background Validation (deferred)

```go
func (s *Scanner) BackgroundValidation() {
    // Run after UI is responsive
    go func() {
        // 1. Verify directory signatures
        dirs, _ := s.db.GetDirectorySignatures()
        for _, dir := range dirs {
            if s.signatureChanged(dir) {
                s.queueDirectoryRescan(dir.Path, PriorityNormal)
            }
        }
        
        // 2. Spot-check random sample of files
        sample := s.db.GetRandomFileSample(100)
        for _, file := range sample {
            if s.fileNeedsReparse(file) {
                s.queueFileUpdate(file.Path, PriorityLow)
            }
        }
    }()
}
```

---

## Performance Projections

### Scan Time Comparison

| Scenario | Current | Proposed |
|----------|---------|----------|
| Startup (no changes) | 30-60s | < 100ms |
| Startup (10 changed files) | 30-60s | < 500ms |
| Startup (100 changed files) | 30-60s | < 2s |
| Full validation (background) | N/A | 5-30s |
| Library size: 10,000 songs | 45s | < 200ms |
| Library size: 100,000 songs | 8min | < 500ms |

### Resource Usage

| Metric | Current | Proposed |
|--------|---------|----------|
| Startup CPU | High (100%) | Low (< 5%) |
| Startup I/O | Sequential read all files | Journal query only |
| Memory (startup) | Proportional to library | Fixed ~10MB |
| Background CPU | N/A | Low priority, pausable |

---

## Implementation Plan

### Phase 1: Foundation (Week 1-2) ✅ COMPLETED
1. ✅ Add database tables for signatures and scan state
2. ✅ Implement directory signature computation
3. ✅ Add cheap change detection (mtime + size)
4. ✅ Modify ScanAll to save/restore scan state

### Phase 2: Platform Journals (Week 3-4) ✅ COMPLETED
1. ✅ **Windows**: Implement USN journal reader
2. ✅ **macOS**: Implement FSEvents integration (mtime-based, pure Go)
3. ✅ **Linux**: Implement mtime-based fallback with optimization

### Phase 3: Background Processing (Week 5) ✅ COMPLETED
1. ✅ Implement priority queue for background work
2. ✅ Add worker pool with pause/resume
3. ✅ Integrate with existing SSE event system

### Phase 4: Optimization (Week 6) ✅ COMPLETED
1. ✅ Tune signature granularity (SignatureConfig with depth, min files, expiry)
2. ✅ Add integrity verification sampling (IntegrityVerifier with weighted selection)
3. ✅ Performance profiling and benchmarking (ScanMetrics, scan history tracking)
4. ✅ Partial hash computation for large files (4KB+4KB sampling)
5. ✅ Adaptive scan scheduling based on change frequency
6. ✅ Stale signature/cache cleanup utilities

---

## API Changes

### New Scanner Methods

```go
// QuickStartup performs fast incremental scan using journals
func (s *Scanner) QuickStartup() (*ScanResult, error)

// GetScanState returns the current scan checkpoint
func (s *Scanner) GetScanState() (*ScanState, error)

// ValidateLibrary performs background integrity check
func (s *Scanner) ValidateLibrary() error

// PauseBackgroundWork pauses background scanning
func (s *Scanner) PauseBackgroundWork()

// ResumeBackgroundWork resumes background scanning
func (s *Scanner) ResumeBackgroundWork()
```

### New Database Methods

```go
// Directory signatures
func (d *DB) SaveDirectorySignature(sig DirectorySignature) error
func (d *DB) GetDirectorySignatures() ([]DirectorySignature, error)
func (d *DB) GetDirectorySignature(path string) (*DirectorySignature, error)

// Scan state
func (d *DB) SaveScanState(state ScanState) error
func (d *DB) GetScanState() (*ScanState, error)

// File metadata cache
func (d *DB) SaveFileMetadataCache(cache FileMetadataCache) error
func (d *DB) GetFileMetadataCache(path string) (*FileMetadataCache, error)
func (d *DB) GetRandomFileSample(count int) ([]FileMetadataCache, error)
func (d *DB) DeleteDirectorySignatures(paths []string) (int, error)
```

### Phase 4 Optimization Methods (NEW)

```go
// Signature configuration
func GetSignatureConfig() SignatureConfig
func SetSignatureConfig(config SignatureConfig)
func IsSignatureExpired(sig DirectorySignature) bool

// Integrity verification
func NewIntegrityVerifier(scanner *Scanner) *IntegrityVerifier
func (iv *IntegrityVerifier) RunSampleVerification() (*IntegrityResult, error)
func (iv *IntegrityVerifier) SetSampleSize(size int)
func (iv *IntegrityVerifier) SetVerifyHash(enabled bool)

// Performance profiling
func RecordScanMetrics(metrics ScanMetrics)
func GetScanHistory() []ScanMetrics
func GetAverageScanTime() time.Duration

// Adaptive scheduling
func NewScanScheduler(scanner *Scanner) *ScanScheduler
func (ss *ScanScheduler) RecordScanResult(changesDetected int)
func (ss *ScanScheduler) GetNextScanInterval() time.Duration

// Cleanup utilities
func (s *Scanner) CleanupStaleSignatures() (int, error)
func (s *Scanner) CleanupStaleMetadataCache() (int, error)
func (s *Scanner) SelectRandomFilesForVerification(count int) ([]FileMetadataCache, error)

// Fast hashing
func ComputePartialHash(filePath string) (string, error)
```

---

## Fallback Strategies

### When Journals Unavailable
1. Fall back to directory signature comparison
2. Use mtime-based change detection
3. Progressive background scan

### When Signatures Stale
1. Recompute signatures for changed directories
2. Queue affected files for verification
3. Update database atomically

### When Database Corrupted
1. Clear scan state
2. Perform full clean scan
3. Rebuild all signatures

---

## Security Considerations

1. **USN Journal Access**: Requires admin/elevated privileges on Windows
2. **Path Validation**: Sanitize all paths from journal queries
3. **Race Conditions**: Handle files deleted between detection and processing
4. **Symlink Handling**: Avoid infinite loops in directory traversal

---

## Testing Strategy

1. **Unit Tests**: Mock filesystem operations, journal queries
2. **Integration Tests**: Real filesystem with synthetic changes
3. **Performance Benchmarks**: Measure startup time with various library sizes
4. **Platform Testing**: Windows 10/11, macOS 12+, Ubuntu 22.04+
5. **Edge Cases**: Network drives, external media, permission errors

---

## Appendix A: Platform-Specific Implementation Notes

### Windows USN Journal
```go
// Required imports
import (
    "golang.org/x/sys/windows"
    "unsafe"
)

// Key structures
type USN_JOURNAL_DATA struct {
    UsnJournalID    uint64
    FirstUsn        int64
    NextUsn         int64
    LowestValidUsn  int64
    MaxUsn          int64
    MaximumSize     uint64
    AllocationDelta uint64
}

type USN_RECORD_V2 struct {
    RecordLength              uint32
    MajorVersion              uint16
    MinorVersion              uint16
    FileReferenceNumber       uint64
    ParentFileReferenceNumber uint64
    Usn                       int64
    TimeStamp                 int64
    Reason                    uint32
    SourceInfo                uint32
    SecurityId                uint32
    FileAttributes            uint32
    FileNameLength            uint16
    FileNameOffset            uint16
    // FileName follows (variable length, UTF-16)
}

// Reason flags we care about
const (
    USN_REASON_DATA_OVERWRITE     = 0x00000001
    USN_REASON_DATA_EXTEND        = 0x00000002
    USN_REASON_DATA_TRUNCATION    = 0x00000004
    USN_REASON_FILE_CREATE        = 0x00000100
    USN_REASON_FILE_DELETE        = 0x00000200
    USN_REASON_RENAME_NEW_NAME    = 0x00002000
)
```

### macOS FSEvents
```go
// Required: cgo with CoreServices framework
// #cgo LDFLAGS: -framework CoreServices
// #include <CoreServices/CoreServices.h>

type FSEventStreamEventFlags uint32
const (
    kFSEventStreamEventFlagItemCreated    = 0x00000100
    kFSEventStreamEventFlagItemRemoved    = 0x00000200
    kFSEventStreamEventFlagItemRenamed    = 0x00000800
    kFSEventStreamEventFlagItemModified   = 0x00001000
    kFSEventStreamEventFlagItemIsFile     = 0x00010000
)
```

### Linux inotify
```go
import "golang.org/x/sys/unix"

// Watch masks
const (
    IN_CREATE    = unix.IN_CREATE
    IN_DELETE    = unix.IN_DELETE
    IN_MODIFY    = unix.IN_MODIFY
    IN_MOVED_TO  = unix.IN_MOVED_TO
    IN_MOVED_FROM = unix.IN_MOVED_FROM
)

// Note: inotify has per-user watch limits (/proc/sys/fs/inotify/max_user_watches)
// Default is 8192, may need to be increased for large libraries
```

---

## Appendix B: Benchmark Targets

| Metric | Target |
|--------|--------|
| Cold startup (no journal) | < 5s for 10k songs |
| Warm startup (journal available) | < 200ms |
| Change detection latency | < 100ms |
| Background scan impact | < 10% CPU |
| Memory overhead | < 50MB |
| Database growth | < 1KB per song |

---

## Conclusion

This incremental scan system will transform ViiB MediaHub's startup experience from a potentially minute-long wait to near-instant responsiveness. By leveraging OS-level filesystem journals and smart caching strategies, we can detect and process only actual changes while maintaining library integrity through background verification.

The implementation is designed to gracefully degrade when advanced features (like USN journals) are unavailable, ensuring reliable operation across all supported platforms.
