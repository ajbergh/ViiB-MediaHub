package db

import (
	"path/filepath"
	"testing"
)

func TestOAuthAccountSettingsBatchRollback(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	old := map[string]string{"spotify_credentials": "old-credentials", "spotify_webplayer_session": "old-owner", "spotify_metadata_active_context": "old-context"}
	if err := d.SetSettingsBatch(old); err != nil {
		t.Fatal(err)
	}
	if _, err := d.conn.Exec(`CREATE TRIGGER fail_owner_insert BEFORE INSERT ON settings WHEN NEW.key='spotify_webplayer_session' BEGIN SELECT RAISE(ABORT,'fixture owner failure'); END;`); err != nil {
		t.Fatal(err)
	}
	next := map[string]string{"spotify_credentials": "new-credentials", "spotify_webplayer_session": "", "spotify_metadata_active_context": "new-context"}
	if err := d.SetSettingsBatch(next); err == nil {
		t.Fatal("settings failure ignored")
	}
	for key, value := range old {
		got, err := d.GetSetting(key)
		if err != nil || got != value {
			t.Fatalf("partial credential replacement for %s", key)
		}
	}
	if _, err := d.conn.Exec(`DROP TRIGGER fail_owner_insert`); err != nil {
		t.Fatal(err)
	}
	if err := d.SetSettingsBatch(next); err != nil {
		t.Fatal(err)
	}
	for key, value := range next {
		got, err := d.GetSetting(key)
		if err != nil || got != value {
			t.Fatalf("replacement missing %s", key)
		}
	}
}
