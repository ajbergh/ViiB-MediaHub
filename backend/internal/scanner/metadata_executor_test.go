package scanner

import (
	"context"
	"errors"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMetadataExecutorRetainsTimedOutAndCanceledWork(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		t.Run(map[bool]string{false: "timeout", true: "canceled"}[canceled], func(t *testing.T) {
			e := metadataExecutor{slots: make(chan struct{}, 1)}
			release, started := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			releaseParser := func() { releaseOnce.Do(func() { close(release) }) }
			defer releaseParser()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			returned := make(chan error, 1)
			go func() {
				_, err := e.extract(ctx, 25*time.Millisecond, func() (*SongMetadata, error) {
					close(started)
					<-release
					return &SongMetadata{}, nil
				})
				returned <- err
			}()
			<-started
			if canceled {
				cancel()
			}
			err := <-returned
			expected := errMetadataTimeout
			if canceled {
				expected = context.Canceled
			}
			if !errors.Is(err, expected) {
				t.Fatalf("return: %v", err)
			}
			if atomic.LoadInt64(&e.active) != 1 || len(e.slots) != 1 {
				t.Fatal("abandoned parser released its slot")
			}
			_, err = e.extract(t.Context(), 10*time.Millisecond, func() (*SongMetadata, error) {
				t.Error("parser admitted while full")
				return nil, nil
			})
			if !errors.Is(err, errMetadataCapacity) {
				t.Fatalf("capacity: %v", err)
			}
			releaseParser()
			if _, err := e.extract(t.Context(), time.Second, func() (*SongMetadata, error) { return &SongMetadata{}, nil }); err != nil {
				t.Fatalf("slot not released on eventual completion: %v", err)
			}
			if atomic.LoadInt64(&e.active) != 0 || len(e.slots) != 0 {
				t.Fatal("completed parser leaked")
			}
		})
	}
}

func TestMetadataExecutorRecoversPanicAndReleasesSlot(t *testing.T) {
	e := metadataExecutor{slots: make(chan struct{}, 1)}
	if _, err := e.extract(t.Context(), time.Second, func() (*SongMetadata, error) { panic("bad media") }); err == nil {
		t.Fatal("panic lost")
	}
	if atomic.LoadInt64(&e.active) != 0 || len(e.slots) != 0 {
		t.Fatal("slot leaked")
	}
	want := &SongMetadata{Title: "next file"}
	got, err := e.extract(t.Context(), time.Second, func() (*SongMetadata, error) { return want, nil })
	if err != nil || got != want {
		t.Fatal("executor did not recover", err)
	}
}

func TestFullFolderScanHonorsParserQuarantine(t *testing.T) {
	root := t.TempDir()
	music := filepath.Join(root, "music")
	if err := os.Mkdir(music, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(music, "bad.mp3")
	if err := os.WriteFile(path, []byte("malformed"), 0600); err != nil {
		t.Fatal(err)
	}
	database, err := db.New(filepath.Join(root, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	s := New(database, filepath.Join(root, "data"))
	defer s.Close()
	if err := database.RecordScannerFailure(path, "metadata_timeout", "test"); err != nil {
		t.Fatal(err)
	}
	result, _, err := s.ScanFolderWithPaths(music)
	if err != nil || result.Errors != 1 || result.NewSongs != 0 {
		t.Fatalf("quarantine bypassed: %+v %v", result, err)
	}
}
