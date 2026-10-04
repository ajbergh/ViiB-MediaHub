package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReferenceReadOnlyRejectsWritesAndDoesNotMigrate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.db")
	writer, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if err := writer.EnsureExternalTrackAnalysisSchema(); err != nil {
		t.Fatal(err)
	}
	reader, err := OpenReferenceReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if _, err := reader.conn.Exec("CREATE TABLE forbidden(id TEXT)"); err == nil {
		t.Fatal("read-only connection allowed a schema write")
	}
	if _, err := reader.conn.Exec("DELETE FROM songs"); err == nil {
		t.Fatal("read-only connection allowed row deletion")
	}
	if err := reader.EnsureExternalTrackAnalysisSchema(); err != nil {
		t.Fatal(err)
	}
	if err := writer.SaveSong(&Song{ID: "later", FilePath: "later.ogg", Title: "later", Artist: "generator", Album: "synthetic", AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	songs, err := reader.GetAllSongs()
	if err != nil {
		t.Fatal(err)
	}
	if len(songs) != 0 {
		t.Fatal("read transaction did not retain a consistent database snapshot")
	}
}

func TestReferenceReadOnlyRequiresExistingInstalledSchema(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "absent.db")
	if d, err := OpenReferenceReadOnly(missing); err == nil {
		d.Close()
		t.Fatal("missing DB was opened")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("read-only opener created missing DB")
	}
	if d, err := OpenReferenceReadOnly(root); err == nil {
		d.Close()
		t.Fatal("directory accepted")
	}
	path := filepath.Join(root, "plain.db")
	writer, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.conn.Exec("DROP TABLE external_track_analysis"); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	if d, err := OpenReferenceReadOnly(path); err == nil {
		d.Close()
		t.Fatal("missing reference schema was migrated")
	}
}
