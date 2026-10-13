package db

import "testing"

func TestDownloadedCatalogParentMigration(t *testing.T) {
	d, _ := evidenceFixture(t)
	// Recreate the released direct-track schema with a retained row.
	_, err := d.conn.Exec(`DROP TABLE spotify_download_catalog_relations;
 CREATE TABLE spotify_download_catalog_relations (
 file_path TEXT NOT NULL,content_sha256 TEXT NOT NULL,file_size INTEGER NOT NULL,mtime_ns INTEGER NOT NULL,
 recording_id TEXT NOT NULL,resource TEXT NOT NULL,relation_kind TEXT NOT NULL,position INTEGER NOT NULL,
 child_type TEXT NOT NULL,child_id TEXT NOT NULL,unavailable INTEGER NOT NULL,metadata_json BLOB NOT NULL,
 PRIMARY KEY(file_path,content_sha256,file_size,mtime_ns,recording_id,resource,relation_kind,position));
 INSERT INTO spotify_download_catalog_relations VALUES('file','hash',1,2,'root','catalog','artists',0,'artist','child',0,'{}');`)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := d.EnsureSpotifyMetadataSchema(); err != nil {
			t.Fatal(err)
		}
	}
	var parentType, parentID string
	if err := d.conn.QueryRow(`SELECT parent_type,parent_id FROM spotify_download_catalog_relations`).Scan(&parentType, &parentID); err != nil {
		t.Fatal(err)
	}
	if parentType != "track" || parentID != "root" {
		t.Fatalf("legacy parent: %s/%s", parentType, parentID)
	}
	_, err = d.conn.Exec(`INSERT INTO spotify_download_catalog_relations
 (file_path,content_sha256,file_size,mtime_ns,recording_id,parent_type,parent_id,resource,relation_kind,position,child_type,child_id,unavailable,metadata_json)
 VALUES('file','hash',1,2,'root','album','album','catalog','artists',0,'artist','child',0,'{}')`)
	if err != nil {
		t.Fatalf("distinct parent collided: %v", err)
	}
	var count int
	if err = d.conn.QueryRow(`SELECT COUNT(*) FROM spotify_download_catalog_relations`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}
