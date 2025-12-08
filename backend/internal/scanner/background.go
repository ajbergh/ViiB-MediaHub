// Package scanner provides media library scanning functionality.
// This file implements background processing with priority queues
// and a pausable worker pool for deferred scan work.
package scanner

import (
	"container/heap"
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/logger"
)

// ScanPriority determines processing order for background tasks
type ScanPriority int

const (
	// PriorityImmediate is for new files detected during quick startup
	PriorityImmediate ScanPriority = iota
	// PriorityHigh is for modified files needing re-parsing
	PriorityHigh
	// PriorityNormal is for periodic validation tasks
	PriorityNormal
	// PriorityLow is for deep integrity checks
	PriorityLow
)

// String returns a human-readable name for the priority
func (p ScanPriority) String() string {
	switch p {
	case PriorityImmediate:
		return "immediate"
	case PriorityHigh:
		return "high"
	case PriorityNormal:
		return "normal"
	case PriorityLow:
		return "low"
	default:
		return "unknown"
	}
}

// BackgroundTask represents a unit of work for the background scanner
type BackgroundTask struct {
	Type        TaskType
	Priority    ScanPriority
	FilePath    string
	DirPath     string
	CreatedAt   time.Time
	Attempts    int
	MaxAttempts int
	index       int // for heap operations
}

// TaskType defines the type of background task
type TaskType int

const (
	// TaskProcessFile processes a single audio file
	TaskProcessFile TaskType = iota
	// TaskRescanDirectory rescans an entire directory
	TaskRescanDirectory
	// TaskValidateFile verifies a file's metadata is up-to-date
	TaskValidateFile
	// TaskDeleteFile removes a deleted file from the database
	TaskDeleteFile
	// TaskComputeSignature computes/updates a directory signature
	TaskComputeSignature
)

// String returns a human-readable name for the task type
func (t TaskType) String() string {
	switch t {
	case TaskProcessFile:
		return "process_file"
	case TaskRescanDirectory:
		return "rescan_directory"
	case TaskValidateFile:
		return "validate_file"
	case TaskDeleteFile:
		return "delete_file"
	case TaskComputeSignature:
		return "compute_signature"
	default:
		return "unknown"
	}
}

// PriorityQueue implements a min-heap for background tasks
// Lower priority value = higher actual priority
type PriorityQueue []*BackgroundTask

func (pq PriorityQueue) Len() int { return len(pq) }

func (pq PriorityQueue) Less(i, j int) bool {
	// First compare by priority (lower = higher priority)
	if pq[i].Priority != pq[j].Priority {
		return pq[i].Priority < pq[j].Priority
	}
	// Then by creation time (older = higher priority)
	return pq[i].CreatedAt.Before(pq[j].CreatedAt)
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	task := x.(*BackgroundTask)
	task.index = n
	*pq = append(*pq, task)
}

func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	task := old[n-1]
	old[n-1] = nil  // avoid memory leak
	task.index = -1 // for safety
	*pq = old[0 : n-1]
	return task
}

// BackgroundScanner handles deferred and validation work
type BackgroundScanner struct {
	scanner       *Scanner
	taskQueue     PriorityQueue
	queueMutex    sync.Mutex
	queueCond     *sync.Cond
	workerCount   int
	activeWorkers int32
	paused        int32
	shutdown      int32
	ctx           context.Context
	cancel        context.CancelFunc

	// Stats
	processedCount int64
	errorCount     int64
	lastProcessed  time.Time

	// Event throttling to prevent excessive SSE traffic
	lastEventTime   time.Time
	eventMutex      sync.Mutex
	pendingAdded    int
	pendingUpdated  int
	pendingRemoved  int
	eventThrottleMs int64 // Minimum ms between events
}

// NewBackgroundScanner creates a new background scanner
func NewBackgroundScanner(scanner *Scanner, workerCount int) *BackgroundScanner {
	ctx, cancel := context.WithCancel(context.Background())
	bs := &BackgroundScanner{
		scanner:         scanner,
		taskQueue:       make(PriorityQueue, 0),
		workerCount:     workerCount,
		ctx:             ctx,
		cancel:          cancel,
		eventThrottleMs: 500, // Emit events at most every 500ms
	}
	bs.queueCond = sync.NewCond(&bs.queueMutex)
	heap.Init(&bs.taskQueue)
	return bs
}

// Start begins the background worker pool
func (bs *BackgroundScanner) Start() {
	logger.Scanner("Starting background scanner with %d workers", bs.workerCount)
	for i := 0; i < bs.workerCount; i++ {
		go bs.worker(i)
	}
}

// Stop gracefully shuts down the background scanner
func (bs *BackgroundScanner) Stop() {
	logger.Scanner("Stopping background scanner...")
	atomic.StoreInt32(&bs.shutdown, 1)
	bs.cancel()
	bs.queueCond.Broadcast() // Wake all workers
}

// Pause temporarily pauses all background processing
func (bs *BackgroundScanner) Pause() {
	if atomic.CompareAndSwapInt32(&bs.paused, 0, 1) {
		logger.Scanner("Background scanner paused")
		bs.emitStateEvent("paused")
	}
}

// Resume continues background processing
func (bs *BackgroundScanner) Resume() {
	if atomic.CompareAndSwapInt32(&bs.paused, 1, 0) {
		logger.Scanner("Background scanner resumed")
		bs.queueCond.Broadcast() // Wake workers
		bs.emitStateEvent("resumed")
	}
}

// IsPaused returns whether background processing is paused
func (bs *BackgroundScanner) IsPaused() bool {
	return atomic.LoadInt32(&bs.paused) == 1
}

// QueueTask adds a task to the priority queue
func (bs *BackgroundScanner) QueueTask(task *BackgroundTask) {
	if atomic.LoadInt32(&bs.shutdown) == 1 {
		return
	}

	task.CreatedAt = time.Now()
	if task.MaxAttempts == 0 {
		task.MaxAttempts = 3
	}

	bs.queueMutex.Lock()
	heap.Push(&bs.taskQueue, task)
	bs.queueMutex.Unlock()
	bs.queueCond.Signal() // Wake one worker

	logger.Scanner("Queued task: %s (%s) - queue size: %d",
		task.Type.String(), task.Priority.String(), bs.QueueSize())
}

// QueueFileTasks queues multiple file tasks at once
func (bs *BackgroundScanner) QueueFileTasks(files []FileChange, priority ScanPriority) {
	if len(files) == 0 {
		return
	}

	bs.queueMutex.Lock()
	for _, file := range files {
		taskType := TaskProcessFile
		if file.ChangeType == ChangeTypeDeleted {
			taskType = TaskDeleteFile
		} else if file.ChangeType == ChangeTypeModified {
			taskType = TaskValidateFile
		}

		task := &BackgroundTask{
			Type:        taskType,
			Priority:    priority,
			FilePath:    file.Path,
			CreatedAt:   time.Now(),
			MaxAttempts: 3,
		}
		heap.Push(&bs.taskQueue, task)
	}
	bs.queueMutex.Unlock()
	bs.queueCond.Broadcast() // Wake all workers

	logger.Scanner("Queued %d file tasks at priority %s", len(files), priority.String())
}

// QueueSize returns the current queue size
func (bs *BackgroundScanner) QueueSize() int {
	bs.queueMutex.Lock()
	defer bs.queueMutex.Unlock()
	return bs.taskQueue.Len()
}

// Stats returns background scanner statistics
func (bs *BackgroundScanner) Stats() (processed, errors int64, queueSize int, activeWorkers int32) {
	return atomic.LoadInt64(&bs.processedCount),
		atomic.LoadInt64(&bs.errorCount),
		bs.QueueSize(),
		atomic.LoadInt32(&bs.activeWorkers)
}

// worker processes tasks from the queue
func (bs *BackgroundScanner) worker(id int) {
	logger.Scanner("Background worker %d started", id)

	for {
		// Check shutdown
		if atomic.LoadInt32(&bs.shutdown) == 1 {
			break
		}

		// Get next task
		bs.queueMutex.Lock()
		for bs.taskQueue.Len() == 0 || bs.IsPaused() {
			if atomic.LoadInt32(&bs.shutdown) == 1 {
				bs.queueMutex.Unlock()
				logger.Scanner("Background worker %d shutting down", id)
				return
			}
			bs.queueCond.Wait()
		}

		task := heap.Pop(&bs.taskQueue).(*BackgroundTask)
		queueEmpty := bs.taskQueue.Len() == 0
		bs.queueMutex.Unlock()

		// Process task
		atomic.AddInt32(&bs.activeWorkers, 1)
		err := bs.processTask(task)
		atomic.AddInt32(&bs.activeWorkers, -1)

		if err != nil {
			atomic.AddInt64(&bs.errorCount, 1)
			logger.Scanner("Background worker %d error processing %s: %v", id, task.FilePath, err)

			// Retry if attempts remaining
			task.Attempts++
			if task.Attempts < task.MaxAttempts {
				task.Priority = PriorityLow // Demote failed tasks
				bs.QueueTask(task)
			}
		} else {
			atomic.AddInt64(&bs.processedCount, 1)
			bs.lastProcessed = time.Now()

			// Flush pending events when queue is empty to ensure final progress is reported
			if queueEmpty {
				bs.flushProgressEvents()
			}
		}
	}

	logger.Scanner("Background worker %d stopped", id)
}

// processTask handles a single background task
func (bs *BackgroundScanner) processTask(task *BackgroundTask) error {
	switch task.Type {
	case TaskProcessFile:
		return bs.processFileTask(task)
	case TaskRescanDirectory:
		return bs.rescanDirectoryTask(task)
	case TaskValidateFile:
		return bs.validateFileTask(task)
	case TaskDeleteFile:
		return bs.deleteFileTask(task)
	case TaskComputeSignature:
		return bs.computeSignatureTask(task)
	default:
		logger.Scanner("Unknown task type: %d", task.Type)
		return nil
	}
}

// processFileTask extracts metadata from a new file
func (bs *BackgroundScanner) processFileTask(task *BackgroundTask) error {
	changes := []FileChange{{
		Path:       task.FilePath,
		ChangeType: ChangeTypeCreated,
	}}

	result, err := bs.scanner.ProcessChanges(changes)
	if err != nil {
		return err
	}

	if result.NewSongs > 0 {
		bs.emitProgressEvent(task, "added", result.NewSongs)
	}
	return nil
}

// rescanDirectoryTask rescans an entire directory
func (bs *BackgroundScanner) rescanDirectoryTask(task *BackgroundTask) error {
	result, _, err := bs.scanner.ScanFolderWithPaths(task.DirPath)
	if err != nil {
		return err
	}

	if result.NewSongs > 0 || result.UpdatedSongs > 0 {
		bs.emitProgressEvent(task, "scanned", result.NewSongs+result.UpdatedSongs)
	}
	return nil
}

// validateFileTask verifies a file's metadata is current
func (bs *BackgroundScanner) validateFileTask(task *BackgroundTask) error {
	changes := []FileChange{{
		Path:       task.FilePath,
		ChangeType: ChangeTypeModified,
	}}

	result, err := bs.scanner.ProcessChanges(changes)
	if err != nil {
		return err
	}

	if result.UpdatedSongs > 0 {
		bs.emitProgressEvent(task, "validated", result.UpdatedSongs)
	}
	return nil
}

// deleteFileTask removes a deleted file from the database
func (bs *BackgroundScanner) deleteFileTask(task *BackgroundTask) error {
	changes := []FileChange{{
		Path:       task.FilePath,
		ChangeType: ChangeTypeDeleted,
	}}

	result, err := bs.scanner.ProcessChanges(changes)
	if err != nil {
		return err
	}

	if result.RemovedSongs > 0 {
		bs.emitProgressEvent(task, "removed", result.RemovedSongs)
	}
	return nil
}

// computeSignatureTask updates a directory's signature
func (bs *BackgroundScanner) computeSignatureTask(task *BackgroundTask) error {
	sig, err := bs.scanner.computeQuickDirectorySignature(task.DirPath)
	if err != nil {
		return err
	}

	return bs.scanner.db.SaveDirectorySignature(sig)
}

// emitStateEvent broadcasts a background scanner state change
func (bs *BackgroundScanner) emitStateEvent(state string) {
	bs.scanner.emitEvent(LibraryEvent{
		Type:    "background_scanner",
		Message: "Background scanner " + state,
		Data: map[string]interface{}{
			"state":     state,
			"queueSize": bs.QueueSize(),
			"processed": atomic.LoadInt64(&bs.processedCount),
			"errors":    atomic.LoadInt64(&bs.errorCount),
			"workers":   atomic.LoadInt32(&bs.activeWorkers),
		},
	})
}

// emitProgressEvent broadcasts background task progress with throttling
// to prevent excessive SSE traffic during bulk operations
func (bs *BackgroundScanner) emitProgressEvent(task *BackgroundTask, action string, count int) {
	bs.eventMutex.Lock()
	defer bs.eventMutex.Unlock()

	// Accumulate counts
	switch action {
	case "added":
		bs.pendingAdded += count
	case "validated", "scanned":
		bs.pendingUpdated += count
	case "removed":
		bs.pendingRemoved += count
	}

	// Check if we should emit an event (throttle based on time)
	now := time.Now()
	elapsed := now.Sub(bs.lastEventTime).Milliseconds()
	if elapsed < bs.eventThrottleMs {
		return // Too soon, accumulate more
	}

	// Emit accumulated progress
	totalPending := bs.pendingAdded + bs.pendingUpdated + bs.pendingRemoved
	if totalPending == 0 {
		return
	}

	bs.scanner.emitEvent(LibraryEvent{
		Type:    "background_progress",
		Message: "Background: processing files",
		Data: map[string]interface{}{
			"added":     bs.pendingAdded,
			"updated":   bs.pendingUpdated,
			"removed":   bs.pendingRemoved,
			"taskType":  task.Type.String(),
			"priority":  task.Priority.String(),
			"queueSize": bs.QueueSize(),
			"processed": atomic.LoadInt64(&bs.processedCount),
		},
	})

	// Reset pending counts and update time
	bs.pendingAdded = 0
	bs.pendingUpdated = 0
	bs.pendingRemoved = 0
	bs.lastEventTime = now
}

// flushProgressEvents emits any pending progress events immediately
func (bs *BackgroundScanner) flushProgressEvents() {
	bs.eventMutex.Lock()
	defer bs.eventMutex.Unlock()

	totalPending := bs.pendingAdded + bs.pendingUpdated + bs.pendingRemoved
	if totalPending == 0 {
		return
	}

	bs.scanner.emitEvent(LibraryEvent{
		Type:    "background_progress",
		Message: "Background: processing complete",
		Data: map[string]interface{}{
			"added":     bs.pendingAdded,
			"updated":   bs.pendingUpdated,
			"removed":   bs.pendingRemoved,
			"queueSize": bs.QueueSize(),
			"processed": atomic.LoadInt64(&bs.processedCount),
		},
	})

	bs.pendingAdded = 0
	bs.pendingUpdated = 0
	bs.pendingRemoved = 0
	bs.lastEventTime = time.Now()
}
