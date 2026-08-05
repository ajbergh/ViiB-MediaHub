package scanner

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
)

const incrementalBatchSize = 50

type preparedIncrementalSong struct {
	song           db.Song
	reusedIdentity bool
}

// ProcessChanges applies coalesced create, modify, and delete events. Every
// ingest path now uses the same fingerprint and identity resolution contract as
// a full scan, so a quick scan cannot replace the move-stable file hash with the
// path-specific proposed ID.
func (s *Scanner) ProcessChanges(changes []FileChange) (*ScanResult, error) {
	result := &ScanResult{}
	startTime := time.Now()
	changes = coalesceFileChanges(changes)

	filesToDelete := make([]string, 0)
	filesToProcess := make([]FileChange, 0, len(changes))
	for _, change := range changes {
		switch change.ChangeType {
		case ChangeTypeDeleted:
			filesToDelete = append(filesToDelete, filepath.Clean(change.Path))
		case ChangeTypeCreated, ChangeTypeModified:
			filesToProcess = append(filesToProcess, change)
		}
	}

	if len(filesToProcess) > 1 {
		logger.Scanner("Processing %d incremental files", len(filesToProcess))
	}

	batch := make([]preparedIncrementalSong, 0, incrementalBatchSize)
	processedPaths := make([]string, 0, incrementalBatchSize)
	processed := 0

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := s.saveIncrementalBatch(batch, processedPaths, result); err != nil {
			return err
		}
		processed += len(batch)
		s.SetProgress(fmt.Sprintf("Processing files... (%d/%d)", processed, len(filesToProcess)))
		s.emitEvent(LibraryEvent{
			Type:    "scan_progress",
			Message: fmt.Sprintf("Processing files... (%d/%d)", processed, len(filesToProcess)),
		})
		batch = batch[:0]
		processedPaths = processedPaths[:0]
		return nil
	}

	for _, change := range filesToProcess {
		prepared, err := s.prepareIncrementalSong(change)
		if err != nil {
			logger.Scanner("Error preparing metadata for %s: %v", change.Path, err)
			result.Errors++
			continue
		}
		batch = append(batch, prepared)
		processedPaths = append(processedPaths, prepared.song.FilePath)
		result.TotalFiles++
		if len(batch) >= incrementalBatchSize {
			if err := flush(); err != nil {
				result.Errors++
				result.Duration = time.Since(startTime)
				return result, err
			}
		}
	}
	if err := flush(); err != nil {
		result.Errors++
		result.Duration = time.Since(startTime)
		return result, err
	}

	if len(filesToDelete) > 0 {
		logger.Scanner("Removing %d deleted files from library", len(filesToDelete))
		removed, err := s.db.DeleteSongsByFilePaths(filesToDelete)
		if err != nil {
			result.Errors++
			result.Duration = time.Since(startTime)
			return result, fmt.Errorf("remove deleted songs: %w", err)
		}
		result.RemovedSongs = removed
		if err := s.db.DeleteFileMetadataCacheBatch(filesToDelete); err != nil {
			logger.Scanner("Error cleaning metadata cache: %v", err)
		}
		if removed > 0 {
			s.emitEvent(LibraryEvent{
				Type:         "library_updated",
				Message:      fmt.Sprintf("Removed %d songs", removed),
				RemovedSongs: removed,
			})
		}
	}

	// Coalesce the expensive aggregate refresh to one call per ProcessChanges
	// invocation rather than launching a goroutine for every 50-song batch.
	if result.NewSongs > 0 || result.UpdatedSongs > 0 || result.RemovedSongs > 0 {
		if err := s.db.UpdateGenreStats(); err != nil {
			logger.Scanner("Failed to update genre stats after incremental scan: %v", err)
		}
	}

	result.Duration = time.Since(startTime)
	return result, nil
}

func (s *Scanner) prepareIncrementalSong(change FileChange) (preparedIncrementalSong, error) {
	filePath := filepath.Clean(change.Path)
	metadata, err := s.extractMetadata(filePath)
	if err != nil {
		return preparedIncrementalSong{}, err
	}

	resolvedID, err := s.db.ResolveSongIdentity(filePath, metadata.FileHash, metadata.ID)
	if err != nil {
		return preparedIncrementalSong{}, fmt.Errorf("resolve stable identity: %w", err)
	}

	coverArtist := metadata.AlbumArtist
	if coverArtist == "" {
		coverArtist = metadata.Artist
	}
	coverPath := s.getAlbumCover(coverArtist, metadata.Album, filepath.Dir(filePath), filePath)

	return preparedIncrementalSong{
		song:           persistedSongFromMetadata(metadata, filePath, coverPath, resolvedID, time.Now().UnixMilli()),
		reusedIdentity: resolvedID != metadata.ID,
	}, nil
}

func persistedSongFromMetadata(
	metadata *SongMetadata,
	filePath string,
	coverPath string,
	resolvedID string,
	addedAt int64,
) db.Song {
	return db.Song{
		ID:           resolvedID,
		Title:        metadata.Title,
		Artist:       metadata.Artist,
		Album:        metadata.Album,
		AlbumArtist:  metadata.AlbumArtist,
		TrackNumber:  metadata.TrackNumber,
		DiscNumber:   metadata.DiscNumber,
		Genre:        metadata.Genre,
		Year:         metadata.Year,
		Duration:     metadata.Duration,
		ReplayGainDB: metadata.ReplayGainDB,
		ReplayPeak:   metadata.ReplayPeak,
		FilePath:     filepath.Clean(filePath),
		CoverPath:    coverPath,
		AddedAt:      addedAt,
		FileHash:     metadata.FileHash,
	}
}

func (s *Scanner) saveIncrementalBatch(
	prepared []preparedIncrementalSong,
	processedPaths []string,
	result *ScanResult,
) error {
	songs := make([]db.Song, 0, len(prepared))
	reusedIdentities := 0
	for _, item := range prepared {
		songs = append(songs, item.song)
		if item.reusedIdentity {
			reusedIdentities++
		}
	}

	upsert, err := s.db.SaveSongsWithResult(songs)
	if err != nil {
		return fmt.Errorf("save incremental song batch: %w", err)
	}

	// SaveSongsWithResult classifies by current path. A move has a new path but a
	// reused logical ID, so report only those reused identities as updates.
	moveAdjust := reusedIdentities
	if moveAdjust > upsert.Inserted {
		moveAdjust = upsert.Inserted
	}
	added := upsert.Inserted - moveAdjust
	updated := upsert.Updated + moveAdjust
	result.NewSongs += added
	result.UpdatedSongs += updated

	s.createAlbumMetadataEntries(songs)
	if err := s.UpdateFileMetadataCache(processedPaths); err != nil {
		logger.Scanner("Error updating metadata cache: %v", err)
	}

	s.emitEvent(LibraryEvent{
		Type:         "library_updated",
		Message:      fmt.Sprintf("Library updated: %d added, %d updated", added, updated),
		NewSongs:     added,
		UpdatedSongs: updated,
	})
	return nil
}
