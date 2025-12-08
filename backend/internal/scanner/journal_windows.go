//go:build windows
// +build windows

// Package scanner provides media library scanning functionality.
// This file implements the Windows USN (Update Sequence Number) journal reader
// for efficient filesystem change detection.
package scanner

import (
	"fmt"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
	"golang.org/x/sys/windows"
)

// USN Journal constants
const (
	FSCTL_QUERY_USN_JOURNAL    = 0x000900f4
	FSCTL_READ_USN_JOURNAL     = 0x000900bb
	FSCTL_ENUM_USN_DATA        = 0x000900b3
	USN_REASON_DATA_OVERWRITE  = 0x00000001
	USN_REASON_DATA_EXTEND     = 0x00000002
	USN_REASON_DATA_TRUNCATION = 0x00000004
	USN_REASON_FILE_CREATE     = 0x00000100
	USN_REASON_FILE_DELETE     = 0x00000200
	USN_REASON_RENAME_OLD_NAME = 0x00001000
	USN_REASON_RENAME_NEW_NAME = 0x00002000
	USN_REASON_CLOSE           = 0x80000000
)

// USN_JOURNAL_DATA_V0 structure
type USN_JOURNAL_DATA struct {
	UsnJournalID    uint64
	FirstUsn        int64
	NextUsn         int64
	LowestValidUsn  int64
	MaxUsn          int64
	MaximumSize     uint64
	AllocationDelta uint64
}

// READ_USN_JOURNAL_DATA_V0 structure
type READ_USN_JOURNAL_DATA struct {
	StartUsn          int64
	ReasonMask        uint32
	ReturnOnlyOnClose uint32
	Timeout           uint64
	BytesToWaitFor    uint64
	UsnJournalID      uint64
}

// USN_RECORD_V2 structure (variable length)
type USN_RECORD_V2 struct {
	RecordLength        uint32
	MajorVersion        uint16
	MinorVersion        uint16
	FileReferenceNumber uint64
	ParentFileRef       uint64
	Usn                 int64
	TimeStamp           int64 // FILETIME
	Reason              uint32
	SourceInfo          uint32
	SecurityId          uint32
	FileAttributes      uint32
	FileNameLength      uint16
	FileNameOffset      uint16
	// FileName follows (variable length, UTF-16)
}

// WindowsUSNDetector implements JournalChangeDetector using Windows USN journal
type WindowsUSNDetector struct {
	scanner       *Scanner
	volumeHandles map[string]windows.Handle
	journalData   map[string]*USN_JOURNAL_DATA
	lastUSN       map[string]int64
	volumePaths   map[string]string // drive letter -> volume path
}

// newWindowsUSNDetector creates a new USN journal detector
func newWindowsUSNDetector(s *Scanner) *WindowsUSNDetector {
	return &WindowsUSNDetector{
		scanner:       s,
		volumeHandles: make(map[string]windows.Handle),
		journalData:   make(map[string]*USN_JOURNAL_DATA),
		lastUSN:       make(map[string]int64),
		volumePaths:   make(map[string]string),
	}
}

// Name returns the name of this detector
func (u *WindowsUSNDetector) Name() string {
	return "Windows USN Journal"
}

// IsAvailable checks if USN journal is available
func (u *WindowsUSNDetector) IsAvailable() bool {
	// USN journal requires:
	// 1. Windows OS (guaranteed by build tag)
	// 2. NTFS filesystem
	// 3. Sufficient privileges

	// Try to get a handle to the C: drive to test
	testPath := `\\.\C:`
	handle, err := windows.CreateFile(
		syscall.StringToUTF16Ptr(testPath),
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)
	if err != nil {
		logger.Scanner("USN journal not available: cannot open volume: %v", err)
		return false
	}
	windows.CloseHandle(handle)

	logger.Scanner("USN journal is available")
	return true
}

// openVolume opens a handle to the volume containing the given path
func (u *WindowsUSNDetector) openVolume(path string) (windows.Handle, string, error) {
	// Get the volume path (e.g., C:\)
	absPath, err := filepath.Abs(path)
	if err != nil {
		return 0, "", err
	}

	volumePath := filepath.VolumeName(absPath)
	if volumePath == "" {
		return 0, "", fmt.Errorf("cannot determine volume for path: %s", path)
	}

	// Check if we already have this volume open
	if handle, ok := u.volumeHandles[volumePath]; ok {
		return handle, volumePath, nil
	}

	// Open the volume
	volumeDevice := `\\.\` + volumePath
	handle, err := windows.CreateFile(
		syscall.StringToUTF16Ptr(volumeDevice),
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)
	if err != nil {
		return 0, "", fmt.Errorf("cannot open volume %s: %w", volumeDevice, err)
	}

	u.volumeHandles[volumePath] = handle
	u.volumePaths[volumePath] = volumePath

	return handle, volumePath, nil
}

// queryJournal gets information about the USN journal on a volume
func (u *WindowsUSNDetector) queryJournal(handle windows.Handle, volumePath string) (*USN_JOURNAL_DATA, error) {
	// Check cache
	if data, ok := u.journalData[volumePath]; ok {
		return data, nil
	}

	var journalData USN_JOURNAL_DATA
	var bytesReturned uint32

	err := windows.DeviceIoControl(
		handle,
		FSCTL_QUERY_USN_JOURNAL,
		nil,
		0,
		(*byte)(unsafe.Pointer(&journalData)),
		uint32(unsafe.Sizeof(journalData)),
		&bytesReturned,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query USN journal: %w", err)
	}

	u.journalData[volumePath] = &journalData
	return &journalData, nil
}

// GetChangesSince returns all filesystem changes since the given timestamp
func (u *WindowsUSNDetector) GetChangesSince(since time.Time, watchPaths []string) ([]FileChange, error) {
	var allChanges []FileChange

	// Group watch paths by volume
	volumeWatchPaths := make(map[string][]string)
	for _, path := range watchPaths {
		absPath, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		volumePath := filepath.VolumeName(absPath)
		volumeWatchPaths[volumePath] = append(volumeWatchPaths[volumePath], absPath)
	}

	// Process each volume
	for volumePath, paths := range volumeWatchPaths {
		handle, _, err := u.openVolume(paths[0])
		if err != nil {
			logger.Scanner("Cannot open volume %s: %v", volumePath, err)
			continue
		}

		journalData, err := u.queryJournal(handle, volumePath)
		if err != nil {
			logger.Scanner("Cannot query journal for %s: %v", volumePath, err)
			continue
		}

		// Determine start USN
		startUSN := u.lastUSN[volumePath]
		if startUSN == 0 || startUSN < journalData.FirstUsn {
			// No previous state or journal was reset, start from beginning
			startUSN = journalData.FirstUsn
		}

		changes, lastUSN, err := u.readChanges(handle, journalData, startUSN, paths)
		if err != nil {
			logger.Scanner("Error reading USN changes: %v", err)
			continue
		}

		u.lastUSN[volumePath] = lastUSN
		allChanges = append(allChanges, changes...)
	}

	return allChanges, nil
}

// readChanges reads USN records from the journal
func (u *WindowsUSNDetector) readChanges(
	handle windows.Handle,
	journalData *USN_JOURNAL_DATA,
	startUSN int64,
	watchPaths []string,
) ([]FileChange, int64, error) {
	var changes []FileChange
	lastUSN := startUSN

	// Prepare read request
	readData := READ_USN_JOURNAL_DATA{
		StartUsn:          startUSN,
		ReasonMask:        USN_REASON_DATA_OVERWRITE | USN_REASON_DATA_EXTEND | USN_REASON_DATA_TRUNCATION | USN_REASON_FILE_CREATE | USN_REASON_FILE_DELETE | USN_REASON_RENAME_NEW_NAME,
		ReturnOnlyOnClose: 0,
		Timeout:           0,
		BytesToWaitFor:    0,
		UsnJournalID:      journalData.UsnJournalID,
	}

	// Buffer for USN records
	buffer := make([]byte, 64*1024) // 64KB buffer
	var bytesReturned uint32

	for {
		err := windows.DeviceIoControl(
			handle,
			FSCTL_READ_USN_JOURNAL,
			(*byte)(unsafe.Pointer(&readData)),
			uint32(unsafe.Sizeof(readData)),
			&buffer[0],
			uint32(len(buffer)),
			&bytesReturned,
			nil,
		)
		if err != nil {
			if err == windows.ERROR_HANDLE_EOF {
				break
			}
			return changes, lastUSN, fmt.Errorf("failed to read USN journal: %w", err)
		}

		if bytesReturned <= 8 {
			break // Only next USN returned, no records
		}

		// First 8 bytes is the next USN
		nextUSN := *(*int64)(unsafe.Pointer(&buffer[0]))
		readData.StartUsn = nextUSN
		lastUSN = nextUSN

		// Parse records
		offset := uint32(8)
		for offset < bytesReturned {
			record := (*USN_RECORD_V2)(unsafe.Pointer(&buffer[offset]))
			if record.RecordLength == 0 {
				break
			}

			// Extract filename
			fileNamePtr := uintptr(unsafe.Pointer(&buffer[offset])) + uintptr(record.FileNameOffset)
			fileName := syscall.UTF16ToString((*[256]uint16)(unsafe.Pointer(fileNamePtr))[:record.FileNameLength/2])

			// Check if this is an audio file
			ext := strings.ToLower(filepath.Ext(fileName))
			if supportedExtensions[ext] {
				// TODO: Resolve full path from file reference number
				// For now, we'll use the filename and match against watch paths
				change := u.classifyChange(record, fileName, watchPaths)
				if change != nil {
					changes = append(changes, *change)
				}
			}

			offset += record.RecordLength
		}
	}

	logger.Scanner("USN journal: found %d audio file changes", len(changes))
	return changes, lastUSN, nil
}

// classifyChange determines the type of change from USN reason flags
func (u *WindowsUSNDetector) classifyChange(record *USN_RECORD_V2, fileName string, watchPaths []string) *FileChange {
	// Determine change type from reason
	var changeType ChangeType

	reason := record.Reason
	if reason&USN_REASON_FILE_CREATE != 0 {
		changeType = ChangeTypeCreated
	} else if reason&USN_REASON_FILE_DELETE != 0 {
		changeType = ChangeTypeDeleted
	} else if reason&(USN_REASON_DATA_OVERWRITE|USN_REASON_DATA_EXTEND|USN_REASON_DATA_TRUNCATION|USN_REASON_RENAME_NEW_NAME) != 0 {
		changeType = ChangeTypeModified
	} else {
		return nil
	}

	// Convert FILETIME to Unix timestamp
	// FILETIME is 100-nanosecond intervals since January 1, 1601
	ft := record.TimeStamp
	mtime := (ft - 116444736000000000) / 10000 // Convert to milliseconds since Unix epoch

	return &FileChange{
		Path:       fileName, // Note: This is just the filename, not full path
		ChangeType: changeType,
		NewMtime:   mtime,
	}
}

// SaveState saves the current USN positions to the database
func (u *WindowsUSNDetector) SaveState() error {
	// Get existing scan state
	state, err := u.scanner.db.GetScanState()
	if err != nil {
		// Create new state
		state = &db.ScanState{
			LastScanTime: time.Now().UnixMilli(),
		}
	}

	// Save the USN for the primary volume (simplification - could track all volumes)
	for _, usn := range u.lastUSN {
		state.WindowsUSN = usn
		break
	}

	return u.scanner.db.SaveScanState(*state)
}

// LoadState loads the previously saved USN positions
func (u *WindowsUSNDetector) LoadState() error {
	state, err := u.scanner.db.GetScanState()
	if err != nil {
		return err
	}

	if state.WindowsUSN > 0 {
		// Apply to all volumes (simplification)
		for vol := range u.volumeHandles {
			u.lastUSN[vol] = state.WindowsUSN
		}
	}

	return nil
}

// Close releases all volume handles
func (u *WindowsUSNDetector) Close() {
	for _, handle := range u.volumeHandles {
		windows.CloseHandle(handle)
	}
	u.volumeHandles = make(map[string]windows.Handle)
}
