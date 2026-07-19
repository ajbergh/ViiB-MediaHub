// Package scanner provides media library scanning functionality.
//
// optimization.go - Phase 4: Performance Optimization System
//
// This file implements the final phase of the Ultra-Fast Startup Incremental Scan System
// as described in FAST_SCAN_DESIGN.md. It provides performance optimizations including:
//
// # Signature Granularity Tuning
//
// The SignatureConfig type allows fine-tuning of directory signature behavior:
//   - MaxDepth: Limit signature computation depth to reduce overhead
//   - MinFilesForSignature: Skip directories with few audio files
//   - SignatureExpiryHours: Automatically invalidate old signatures
//   - UsePartialHash: Use 4KB+4KB sampling for faster (but less precise) hashing
//
// # Integrity Verification Sampling
//
// The IntegrityVerifier performs periodic random sampling of the library to detect:
//   - Missing files (deleted from disk but still in database)
//   - Corrupted files (unreadable or truncated)
//   - Metadata mismatches (file changed but database not updated)
//   - Size/mtime discrepancies
//
// Verification is weighted by age - files not verified recently have higher
// priority for selection, ensuring comprehensive coverage over time.
//
// # Performance Profiling
//
// ScanMetrics and metricsCollector provide detailed timing breakdowns:
//   - Journal query time
//   - Signature check time
//   - File processing time
//   - Database save time
//   - Files/directories processed vs skipped
//
// Scan history (100 entries) enables trend analysis and optimization tuning.
//
// # Adaptive Scan Scheduling
//
// ScanScheduler automatically adjusts scan frequency based on detected change rates:
//   - High activity: Scans every 5 minutes
//   - Low activity: Scans every hour
//   - Gradual adjustment between extremes
//
// # Stale Data Cleanup
//
// Utility methods remove orphaned entries:
//   - CleanupStaleSignatures: Removes signatures for deleted directories
//   - CleanupStaleMetadataCache: Removes cache entries for deleted files
//
// See FAST_SCAN_DESIGN.md for complete architecture documentation.
package scanner

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
)

// ==============================================================================
// Signature Granularity Configuration
// ==============================================================================

// SignatureConfig controls directory signature computation behavior for the
// fast scan system. Signatures enable quick detection of unchanged directories
// without walking their contents.
//
// Example usage:
//
//	config := scanner.DefaultSignatureConfig()
//	config.SignatureExpiryHours = 24 // Expire after 1 day
//	config.UsePartialHash = true     // Faster but less precise
//	scanner.SetSignatureConfig(config)
type SignatureConfig struct {
	// MaxDepth limits how deep to compute signatures (0 = unlimited).
	// Setting this to 2-3 can reduce overhead for deeply nested libraries.
	MaxDepth int

	// MinFilesForSignature only computes signatures for directories with
	// at least this many audio files. Directories with fewer files are
	// always walked since the overhead of signature management exceeds
	// the benefit.
	MinFilesForSignature int

	// SkipSubdirsWithSignature skips subdirectory walking if parent has
	// a valid (non-expired, matching) signature. This is the main performance
	// optimization - unchanged parent signatures allow skipping entire subtrees.
	SkipSubdirsWithSignature bool

	// SignatureExpiryHours invalidates signatures older than this.
	// Set to 0 for no expiry. Recommended: 24-168 hours (1-7 days).
	// Expired signatures trigger re-verification during the next scan.
	SignatureExpiryHours int

	// UsePartialHash uses first 4KB + last 4KB of files for faster hashing.
	// This is less precise but significantly faster for large files.
	// False = full content hash (slower but 100% accurate)
	// True = partial hash (faster but may miss some mid-file changes)
	UsePartialHash bool
}

// DefaultSignatureConfig returns the default signature configuration optimized
// for accuracy over speed. For faster scans with slightly reduced accuracy,
// enable UsePartialHash and reduce SignatureExpiryHours.
func DefaultSignatureConfig() SignatureConfig {
	return SignatureConfig{
		MaxDepth:                 0,      // Unlimited depth
		MinFilesForSignature:     1,      // All directories with audio files
		SkipSubdirsWithSignature: true,   // Main optimization
		SignatureExpiryHours:     24 * 7, // 7 days
		UsePartialHash:           false,  // Full accuracy by default
	}
}

// signatureConfig holds the current configuration
var signatureConfig = DefaultSignatureConfig()
var signatureConfigMutex sync.RWMutex

// GetSignatureConfig returns the current signature configuration.
func GetSignatureConfig() SignatureConfig {
	signatureConfigMutex.RLock()
	defer signatureConfigMutex.RUnlock()
	return signatureConfig
}

// SetSignatureConfig updates the global signature configuration.
// Changes take effect immediately for all subsequent signature operations.
// Thread-safe for concurrent access.
func SetSignatureConfig(config SignatureConfig) {
	signatureConfigMutex.Lock()
	defer signatureConfigMutex.Unlock()
	signatureConfig = config
	logger.Scanner("Updated signature config: depth=%d, minFiles=%d, expiry=%dh",
		config.MaxDepth, config.MinFilesForSignature, config.SignatureExpiryHours)
}

// IsSignatureExpired checks if a directory signature is older than the
// configured expiry time. Expired signatures should be recomputed during
// the next scan to ensure accuracy.
//
// Returns false if SignatureExpiryHours is 0 (no expiry configured).
func IsSignatureExpired(sig db.DirectorySignature) bool {
	config := GetSignatureConfig()
	if config.SignatureExpiryHours <= 0 {
		return false // No expiry
	}
	expiryTime := time.Now().Add(-time.Duration(config.SignatureExpiryHours) * time.Hour)
	return sig.LastVerified < expiryTime.UnixMilli()
}

// ==============================================================================
// Integrity Verification System
// ==============================================================================

// IntegrityResult contains the results of an integrity verification run.
// It provides summary statistics and detailed issue information for any
// problems detected in the library.
type IntegrityResult struct {
	TotalFiles      int              // Number of files in the verification sample
	VerifiedFiles   int              // Files that passed all checks
	MismatchedFiles int              // Files with size/mtime discrepancies
	MissingFiles    int              // Files deleted from disk but in database
	CorruptedFiles  int              // Files that cannot be read
	Duration        time.Duration    // Time taken for verification
	Issues          []IntegrityIssue // Detailed list of all detected issues
}

// IntegrityIssue represents a single detected integrity problem.
// Issues are categorized by type and severity to enable appropriate handling.
type IntegrityIssue struct {
	FilePath    string // Absolute path to the affected file
	IssueType   string // One of: "missing", "corrupted", "access_error", "size_mismatch", "mtime_mismatch"
	Description string // Human-readable description of the issue
	Severity    string // One of: "error" (requires action), "warning" (may need attention), "info" (informational)
}

// IntegrityVerifier handles periodic library integrity checks using random
// sampling. It verifies that files in the database still exist on disk and
// that their metadata matches the cached values.
//
// The verifier uses weighted random selection to prioritize files that
// haven't been verified recently, ensuring comprehensive coverage over time.
type IntegrityVerifier struct {
	scanner        *Scanner         // Parent scanner for database access
	sampleSize     int              // Number of files to verify per run
	verifyMetadata bool             // Check mtime/size consistency
	verifyHash     bool             // Attempt to read file (expensive)
	running        int32            // Atomic: 1 if verification in progress
	lastRun        time.Time        // Timestamp of last verification
	lastResult     *IntegrityResult // Result of last verification
	mutex          sync.RWMutex     // Protects configuration and results
}

// NewIntegrityVerifier creates a new integrity verifier for the given scanner.
// Default configuration verifies 100 random files per run with metadata checks
// but without hash verification (which requires reading file contents).
func NewIntegrityVerifier(scanner *Scanner) *IntegrityVerifier {
	return &IntegrityVerifier{
		scanner:        scanner,
		sampleSize:     100, // Default: verify 100 random files
		verifyMetadata: true,
		verifyHash:     false, // Hash verification is expensive
	}
}

// SetSampleSize sets how many files to verify in each run.
// Larger samples provide more thorough verification but take longer.
// Recommended: 50-200 for background verification, 500+ for full checks.
func (iv *IntegrityVerifier) SetSampleSize(size int) {
	iv.mutex.Lock()
	defer iv.mutex.Unlock()
	iv.sampleSize = size
}

// SetVerifyHash enables or disables hash verification.
// When enabled, the verifier attempts to read the first bytes of each file
// to detect corruption. This is more thorough but significantly slower
// due to disk I/O. Disabled by default for performance.
func (iv *IntegrityVerifier) SetVerifyHash(enabled bool) {
	iv.mutex.Lock()
	defer iv.mutex.Unlock()
	iv.verifyHash = enabled
}

// IsRunning returns whether a verification is currently in progress.
// Only one verification can run at a time to prevent resource contention.
func (iv *IntegrityVerifier) IsRunning() bool {
	return atomic.LoadInt32(&iv.running) == 1
}

// LastResult returns the result of the most recent verification run.
// Returns nil if no verification has been completed yet.
func (iv *IntegrityVerifier) LastResult() *IntegrityResult {
	iv.mutex.RLock()
	defer iv.mutex.RUnlock()
	return iv.lastResult
}

// RunSampleVerification performs integrity verification on a random sample
// of files from the library. It checks each file for existence, size/mtime
// consistency, and optionally readability.
//
// This method is idempotent and can be called periodically from a background
// goroutine. It will return an error if a verification is already in progress.
//
// The results are stored internally and can be retrieved via LastResult().
func (iv *IntegrityVerifier) RunSampleVerification() (*IntegrityResult, error) {
	if !atomic.CompareAndSwapInt32(&iv.running, 0, 1) {
		return nil, fmt.Errorf("verification already in progress")
	}
	defer atomic.StoreInt32(&iv.running, 0)

	startTime := time.Now()
	result := &IntegrityResult{}

	iv.mutex.RLock()
	sampleSize := iv.sampleSize
	verifyMetadata := iv.verifyMetadata
	verifyHash := iv.verifyHash
	iv.mutex.RUnlock()

	// Get random sample
	sample, err := iv.scanner.db.GetRandomFileSample(sampleSize)
	if err != nil {
		return nil, fmt.Errorf("failed to get file sample: %w", err)
	}

	result.TotalFiles = len(sample)
	logger.Scanner("Starting integrity verification of %d files", result.TotalFiles)

	for _, cached := range sample {
		issue := iv.verifyFile(cached, verifyMetadata, verifyHash)
		if issue != nil {
			result.Issues = append(result.Issues, *issue)
			switch issue.IssueType {
			case "missing":
				result.MissingFiles++
			case "corrupted":
				result.CorruptedFiles++
			default:
				result.MismatchedFiles++
			}
		} else {
			result.VerifiedFiles++
		}
	}

	result.Duration = time.Since(startTime)

	iv.mutex.Lock()
	iv.lastRun = time.Now()
	iv.lastResult = result
	iv.mutex.Unlock()

	logger.Scanner("Integrity verification completed in %s: %d verified, %d issues",
		result.Duration, result.VerifiedFiles, len(result.Issues))

	return result, nil
}

// verifyFile checks a single file for integrity issues
func (iv *IntegrityVerifier) verifyFile(cached db.FileMetadataCache, checkMetadata, checkHash bool) *IntegrityIssue {
	// Check if file exists
	info, err := os.Stat(cached.FilePath)
	if os.IsNotExist(err) {
		return &IntegrityIssue{
			FilePath:    cached.FilePath,
			IssueType:   "missing",
			Description: "File no longer exists on disk",
			Severity:    "error",
		}
	}
	if err != nil {
		return &IntegrityIssue{
			FilePath:    cached.FilePath,
			IssueType:   "access_error",
			Description: fmt.Sprintf("Cannot access file: %v", err),
			Severity:    "error",
		}
	}

	// Check size mismatch
	if info.Size() != cached.FileSize {
		return &IntegrityIssue{
			FilePath:    cached.FilePath,
			IssueType:   "size_mismatch",
			Description: fmt.Sprintf("Size changed: cached=%d, actual=%d", cached.FileSize, info.Size()),
			Severity:    "warning",
		}
	}

	// Check mtime mismatch
	if info.ModTime().UnixMilli() != cached.Mtime {
		return &IntegrityIssue{
			FilePath:    cached.FilePath,
			IssueType:   "mtime_mismatch",
			Description: "Modification time has changed",
			Severity:    "info",
		}
	}

	// Optional: verify file can be read (first few bytes)
	if checkHash {
		if err := iv.verifyFileReadable(cached.FilePath); err != nil {
			return &IntegrityIssue{
				FilePath:    cached.FilePath,
				IssueType:   "corrupted",
				Description: fmt.Sprintf("File read error: %v", err),
				Severity:    "error",
			}
		}
	}

	return nil
}

// verifyFileReadable attempts to read the first 4KB of a file to detect
// corruption or access issues. Returns nil if the file is readable.
func (iv *IntegrityVerifier) verifyFileReadable(filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	// Try to read first 4KB
	buf := make([]byte, 4096)
	_, err = f.Read(buf)
	if err != nil && err != io.EOF {
		return err
	}
	return nil
}

// ==============================================================================
// Performance Profiling
// ==============================================================================

// ScanMetrics tracks detailed performance metrics for a single scan operation.
// These metrics enable performance analysis and optimization tuning.
// Use RecordScanMetrics() to add to history and GetScanHistory() to retrieve.
type ScanMetrics struct {
	StartTime          time.Time     // When the scan started
	EndTime            time.Time     // When the scan completed
	TotalDuration      time.Duration // Total wall-clock time
	JournalQueryTime   time.Duration // Time spent querying filesystem journal
	SignatureCheckTime time.Duration // Time spent checking directory signatures
	FileProcessingTime time.Duration // Time spent processing changed files
	DatabaseSaveTime   time.Duration // Time spent saving to database
	DirectoriesScanned int           // Directories that were walked
	DirectoriesSkipped int           // Directories skipped due to matching signatures
	FilesProcessed     int           // Files that were parsed for metadata
	FilesSkipped       int           // Files skipped due to unchanged mtime/size
	BytesProcessed     int64         // Total bytes of audio files processed
	JournalMethod      string        // Journal method used (e.g., "Windows USN Journal")
	FallbackUsed       bool          // Whether signature fallback was used
}

// scanHistory stores recent scan metrics for performance analysis.
// Access via RecordScanMetrics() and GetScanHistory().
var scanHistory []ScanMetrics
var scanHistoryMutex sync.RWMutex
var maxScanHistory = 100 // Keep last 100 scans

// RecordScanMetrics adds metrics to the global history for trend analysis.
// Automatically removes oldest entry if history exceeds maxScanHistory.
func RecordScanMetrics(metrics ScanMetrics) {
	scanHistoryMutex.Lock()
	defer scanHistoryMutex.Unlock()

	scanHistory = append(scanHistory, metrics)
	if len(scanHistory) > maxScanHistory {
		scanHistory = scanHistory[1:]
	}
}

// GetScanHistory returns a copy of recent scan metrics for analysis.
// Results are ordered oldest to newest.
func GetScanHistory() []ScanMetrics {
	scanHistoryMutex.RLock()
	defer scanHistoryMutex.RUnlock()
	result := make([]ScanMetrics, len(scanHistory))
	copy(result, scanHistory)
	return result
}

// GetAverageScanTime calculates the average total duration across all
// recorded scans. Useful for performance monitoring and trend detection.
// Returns 0 if no scans have been recorded.
func GetAverageScanTime() time.Duration {
	scanHistoryMutex.RLock()
	defer scanHistoryMutex.RUnlock()

	if len(scanHistory) == 0 {
		return 0
	}

	var total time.Duration
	for _, m := range scanHistory {
		total += m.TotalDuration
	}
	return total / time.Duration(len(scanHistory))
}

// ==============================================================================
// Optimized Partial Hash for Large Files
// ==============================================================================

// ComputePartialHash computes a hash from the first 4KB and last 4KB of a file.
// This is much faster than full file hashing while still detecting most changes.
//
// The algorithm:
//  1. Read and hash first 4KB of file
//  2. If file > 8KB, also read and hash last 4KB
//  3. Include file size in hash for additional uniqueness
//
// This approach detects:
//   - Any changes to file headers (where metadata typically lives)
//   - Changes to file endings (audio data, trailers)
//   - File truncation/extension (via size in hash)
//
// It may miss changes to the middle of large files, which is acceptable
// for audio metadata purposes since audio format headers are at the start.
//
// Returns a 16-character hex string suitable for comparison.
func ComputePartialHash(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", err
	}

	hasher := sha256.New()
	buf := make([]byte, 4096)

	// Read first 4KB
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}
	hasher.Write(buf[:n])

	// If file is larger than 8KB, also read last 4KB
	if info.Size() > 8192 {
		_, err = f.Seek(-4096, io.SeekEnd)
		if err != nil {
			return "", err
		}
		n, err = f.Read(buf)
		if err != nil && err != io.EOF {
			return "", err
		}
		hasher.Write(buf[:n])
	}

	// Include file size in hash for additional uniqueness
	hasher.Write([]byte(fmt.Sprintf("|%d|", info.Size())))

	return hex.EncodeToString(hasher.Sum(nil))[:16], nil
}

// ==============================================================================
// Adaptive Scan Scheduling
// ==============================================================================

// ScanScheduler manages adaptive scan timing based on detected change frequency.
// It automatically adjusts scan intervals to balance between responsiveness
// (detecting changes quickly) and efficiency (avoiding unnecessary scans).
//
// Behavior:
//   - High activity (>threshold changes): Scan every minInterval (5 min default)
//   - No activity for 3+ scans: Gradually double interval up to maxInterval (1 hr)
//   - Moderate activity: Maintain current interval
//
// Example usage:
//
//	scheduler := NewScanScheduler(scanner)
//	// After each scan:
//	scheduler.RecordScanResult(changesFound)
//	nextScan := time.Now().Add(scheduler.GetNextScanInterval())
type ScanScheduler struct {
	scanner             *Scanner      // Parent scanner
	minInterval         time.Duration // Minimum scan interval (high activity)
	maxInterval         time.Duration // Maximum scan interval (no activity)
	currentInterval     time.Duration // Current recommended interval
	changeThreshold     int           // Changes above this = high activity
	lastChangeCount     int           // Changes in last scan
	consecutiveNoChange int           // Scans with zero changes
	mutex               sync.Mutex    // Protects all fields
}

// NewScanScheduler creates a new adaptive scan scheduler with default settings:
//   - minInterval: 5 minutes (high activity)
//   - maxInterval: 1 hour (low activity)
//   - changeThreshold: 10 files
func NewScanScheduler(scanner *Scanner) *ScanScheduler {
	return &ScanScheduler{
		scanner:         scanner,
		minInterval:     5 * time.Minute,
		maxInterval:     1 * time.Hour,
		currentInterval: 15 * time.Minute,
		changeThreshold: 10,
	}
}

// RecordScanResult updates the scheduler based on scan results.
// Call this after each scan to adjust the recommended interval.
// The scheduler will automatically increase or decrease interval
// based on change frequency.
func (ss *ScanScheduler) RecordScanResult(changesDetected int) {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()

	if changesDetected > ss.changeThreshold {
		// High activity - scan more frequently
		ss.currentInterval = ss.minInterval
		ss.consecutiveNoChange = 0
	} else if changesDetected == 0 {
		// No changes - gradually increase interval
		ss.consecutiveNoChange++
		if ss.consecutiveNoChange > 3 {
			newInterval := ss.currentInterval * 2
			if newInterval > ss.maxInterval {
				newInterval = ss.maxInterval
			}
			ss.currentInterval = newInterval
		}
	} else {
		// Some changes - maintain moderate interval
		ss.consecutiveNoChange = 0
		if ss.currentInterval < ss.minInterval*2 {
			ss.currentInterval = ss.minInterval * 2
		}
	}

	ss.lastChangeCount = changesDetected
	logger.Scanner("Scan scheduler: %d changes, next scan in %s",
		changesDetected, ss.currentInterval)
}

// GetNextScanInterval returns the recommended interval until the next scan.
// Use this to schedule the next scan:
//
//	nextScanTime := time.Now().Add(scheduler.GetNextScanInterval())
func (ss *ScanScheduler) GetNextScanInterval() time.Duration {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()
	return ss.currentInterval
}

// ==============================================================================
// Stale Signature Cleanup
// ==============================================================================

// CleanupStaleSignatures removes directory signatures for paths that no longer
// exist on disk. This can happen when directories are deleted while the
// application is not running.
//
// Returns the number of stale signatures removed.
// Should be called periodically (e.g., after startup or during maintenance).
func (s *Scanner) CleanupStaleSignatures() (int, error) {
	signatures, err := s.db.GetAllDirectorySignatures()
	if err != nil {
		return 0, fmt.Errorf("failed to get signatures: %w", err)
	}

	var stalePaths []string
	for _, sig := range signatures {
		if _, err := os.Stat(sig.Path); os.IsNotExist(err) {
			stalePaths = append(stalePaths, sig.Path)
		}
	}

	if len(stalePaths) == 0 {
		return 0, nil
	}

	deleted, err := s.db.DeleteDirectorySignatures(stalePaths)
	if err != nil {
		return 0, fmt.Errorf("failed to delete stale signatures: %w", err)
	}

	logger.Scanner("Cleaned up %d stale directory signatures", deleted)
	return deleted, nil
}

// CleanupStaleMetadataCache removes file metadata cache entries for files
// that no longer exist on disk. This can happen when files are deleted
// while the application is not running.
//
// Returns the number of stale cache entries removed.
// Should be called periodically (e.g., after startup or during maintenance).
func (s *Scanner) CleanupStaleMetadataCache() (int, error) {
	caches, err := s.db.GetAllFileMetadataCache()
	if err != nil {
		return 0, fmt.Errorf("failed to get metadata cache: %w", err)
	}

	var stalePaths []string
	for _, cache := range caches {
		if _, err := os.Stat(cache.FilePath); os.IsNotExist(err) {
			stalePaths = append(stalePaths, cache.FilePath)
		}
	}

	if len(stalePaths) == 0 {
		return 0, nil
	}

	if err := s.db.DeleteFileMetadataCacheBatch(stalePaths); err != nil {
		return 0, fmt.Errorf("failed to delete stale cache entries: %w", err)
	}

	logger.Scanner("Cleaned up %d stale metadata cache entries", len(stalePaths))
	return len(stalePaths), nil
}

// ==============================================================================
// Randomized Verification (for background validation)
// ==============================================================================

// SelectRandomFilesForVerification selects a weighted random sample of files
// for integrity verification. Files that haven't been verified recently have
// a higher probability of selection, ensuring comprehensive coverage over time.
//
// The weighting algorithm:
//   - Each file's weight is proportional to hours since last verification
//   - Files verified recently have weight ~1 (low priority)
//   - Files never verified or verified long ago have higher weights
//
// Returns up to 'count' files, or all files if fewer than 'count' exist.
func (s *Scanner) SelectRandomFilesForVerification(count int) ([]db.FileMetadataCache, error) {
	allFiles, err := s.db.GetAllFileMetadataCache()
	if err != nil {
		return nil, err
	}

	if len(allFiles) <= count {
		return allFiles, nil
	}

	// Weight by time since last verification
	now := time.Now().UnixMilli()
	type weightedFile struct {
		file   db.FileMetadataCache
		weight float64
	}

	weighted := make([]weightedFile, len(allFiles))
	var totalWeight float64

	for i, f := range allFiles {
		// Higher weight for older verifications
		age := float64(now - f.LastVerified)
		weight := age / float64(time.Hour.Milliseconds()) // Weight in hours
		if weight < 1 {
			weight = 1
		}
		weighted[i] = weightedFile{file: f, weight: weight}
		totalWeight += weight
	}

	// Weighted random selection
	selected := make([]db.FileMetadataCache, 0, count)
	selectedMap := make(map[string]bool)

	for len(selected) < count {
		r := rand.Float64() * totalWeight
		var cumulative float64
		for _, wf := range weighted {
			if selectedMap[wf.file.FilePath] {
				continue
			}
			cumulative += wf.weight
			if cumulative >= r {
				selected = append(selected, wf.file)
				selectedMap[wf.file.FilePath] = true
				totalWeight -= wf.weight
				break
			}
		}
	}

	return selected, nil
}
