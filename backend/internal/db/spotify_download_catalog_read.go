package db

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/ajbergh/viib-mediahub/internal/spotify/metadata"
	"os"
	"time"
)

type DownloadedCatalogSnapshot struct {
	CapturedResource string          `json:"capturedResource,omitempty"`
	CaptureRevision  string          `json:"captureRevision,omitempty"`
	EntityType       string          `json:"entityType"`
	SpotifyID        string          `json:"spotifyId"`
	Resource         string          `json:"resource"`
	SchemaVersion    int             `json:"schemaVersion"`
	AdapterRevision  string          `json:"adapterRevision"`
	Payload          json.RawMessage `json:"payload"`
	RetrievedAt      time.Time       `json:"retrievedAt"`
	ExpiresAt        time.Time       `json:"expiresAt"`
	Stale            bool            `json:"stale"`
}
type DownloadedCatalogRelation struct {
	ParentType  string          `json:"parentType"`
	ParentID    string          `json:"parentId"`
	Resource    string          `json:"resource"`
	Kind        string          `json:"kind"`
	Position    int             `json:"position"`
	ChildType   string          `json:"childType"`
	ChildID     string          `json:"childId"`
	Unavailable bool            `json:"unavailable"`
	Metadata    json.RawMessage `json:"metadata"`
}
type DownloadedSpotifyCatalog struct {
	CollectionStatus  *DownloadedCatalogImportStatus `json:"collectionStatus,omitempty"`
	Origins           []SpotifyDownloadOrigin        `json:"origins"`
	LineageStatus     *DownloadedLineageStatus       `json:"lineageStatus,omitempty"`
	CatalogStatus     *DownloadedCatalogImportStatus `json:"catalogStatus,omitempty"`
	RecordingID       string                         `json:"recordingId"`
	SourceFingerprint string                         `json:"sourceFingerprint"`
	Provenance        string                         `json:"provenance"`
	Snapshots         []DownloadedCatalogSnapshot    `json:"snapshots"`
	Relations         []DownloadedCatalogRelation    `json:"relations"`
}

// verifiedCatalogBinding admits only the actual final file and current explicit
// recording link. Account caches and download queue lifetime are irrelevant.
func (d *DB) verifiedCatalogBinding(ctx context.Context, songID, fingerprint string) (downloadFileRevision, string, bool, error) {
	var revision downloadFileRevision
	var recording, fileHash string
	err := d.conn.QueryRowContext(ctx, `SELECT b.spotify_id,b.file_path,b.content_sha256,b.file_size,b.mtime_ns,COALESCE(s.file_hash,'')
 FROM spotify_download_import_bindings b JOIN songs s ON s.id=b.song_id AND s.file_path=b.file_path
 JOIN track_external_identity i ON i.song_id=b.song_id AND i.provider='spotify' AND i.external_id=b.spotify_id AND i.source_fingerprint=b.source_fingerprint
 WHERE b.song_id=? AND b.source_fingerprint=? AND NOT EXISTS (SELECT 1 FROM track_external_identity_suppression x WHERE x.song_id=b.song_id AND x.source_fingerprint=b.source_fingerprint)`, songID, fingerprint).Scan(&recording, &revision.path, &revision.digest, &revision.size, &revision.mtime, &fileHash)
	if errors.Is(err, sql.ErrNoRows) {
		return revision, "", false, nil
	}
	if err != nil {
		return revision, "", false, err
	}
	actual, err := readDownloadRevision(ctx, revision.path)
	if err != nil {
		if ctx.Err() != nil {
			return revision, "", false, ctx.Err()
		}
		return revision, "", false, nil
	}
	if actual != revision {
		return revision, "", false, nil
	}
	info, err := os.Lstat(revision.path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != revision.size || info.ModTime().UnixNano() != revision.mtime || LocalSourceFingerprint(Song{FilePath: revision.path, FileHash: fileHash}, info) != fingerprint {
		return revision, "", false, nil
	}
	var current int
	err = d.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM spotify_download_import_bindings b
 JOIN songs s ON s.id=b.song_id AND s.file_path=b.file_path
 JOIN track_external_identity i ON i.song_id=b.song_id AND i.provider='spotify' AND i.external_id=b.spotify_id AND i.source_fingerprint=b.source_fingerprint
 WHERE b.song_id=? AND b.source_fingerprint=? AND b.spotify_id=? AND b.file_path=? AND b.content_sha256=? AND b.file_size=? AND b.mtime_ns=? AND COALESCE(s.file_hash,'')=?
 AND NOT EXISTS (SELECT 1 FROM track_external_identity_suppression x WHERE x.song_id=b.song_id AND x.source_fingerprint=b.source_fingerprint)`, songID, fingerprint, recording, revision.path, revision.digest, revision.size, revision.mtime, fileHash).Scan(&current)
	if err != nil {
		return revision, "", false, err
	}
	if current != 1 {
		return revision, "", false, nil
	}
	return revision, recording, true, nil
}

// GetDownloadedSpotifyCatalog reads bounded retained domain evidence without
// provider I/O. Expired snapshots remain explicitly stale inspection evidence.
func (d *DB) GetDownloadedSpotifyCatalog(ctx context.Context, songID, fingerprint string) (*DownloadedSpotifyCatalog, error) {
	if songID == "" || fingerprint == "" {
		return nil, errors.New("current source required")
	}
	revision, recording, admitted, err := d.verifiedCatalogBinding(ctx, songID, fingerprint)
	if err != nil || !admitted {
		return nil, err
	}
	result := &DownloadedSpotifyCatalog{RecordingID: recording, SourceFingerprint: fingerprint, Provenance: "spotify_download_import", Origins: []SpotifyDownloadOrigin{}, Snapshots: []DownloadedCatalogSnapshot{}, Relations: []DownloadedCatalogRelation{}}
	var status DownloadedCatalogImportStatus
	var checked int64
	err = d.conn.QueryRowContext(ctx, `SELECT state,reason,checked_at,scope FROM spotify_download_catalog_import_status WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=? AND recording_id=?`, revision.path, revision.digest, revision.size, revision.mtime, recording).Scan(&status.State, &status.Reason, &checked, &status.Scope)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil {
		switch status.State {
		case "available", "incomplete", "oversized", "not_available":
		default:
			return nil, errors.New("invalid catalog import status")
		}
		validReason := map[string]string{"direct_track_bundle_retained": "available", "track_album_artist_graph_retained": "available", "snapshot_limit": "oversized", "relation_limit": "oversized", "no_eligible_catalog": "not_available", "root_track_unavailable": "incomplete", "related_entity_unavailable": "incomplete"}
		if validReason[status.Reason] != status.State || checked < 0 {
			return nil, errors.New("invalid catalog import status provenance")
		}
		status.CheckedAt = time.UnixMilli(checked).UTC()
		if status.Scope != "track_and_direct_relations_v1" && status.Scope != downloadedCatalogGraphScope {
			return nil, errors.New("invalid catalog import scope")
		}
		result.CatalogStatus = &status
	}
	rows, err := d.conn.QueryContext(ctx, `SELECT entity_type,spotify_id,resource,schema_version,adapter_revision,payload,payload_hash,retrieved_at,expires_at,captured_resource,capture_revision FROM spotify_download_catalog_imports
 WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=? AND recording_id=? ORDER BY entity_type,spotify_id,resource LIMIT 257`, revision.path, revision.digest, revision.size, revision.mtime, recording)
	if err != nil {
		return nil, err
	}
	parentResources := map[[3]string]bool{}
	total := 0
	for rows.Next() {
		var snapshot DownloadedCatalogSnapshot
		var payload []byte
		var hash string
		var retrieved, expires int64
		if err := rows.Scan(&snapshot.EntityType, &snapshot.SpotifyID, &snapshot.Resource, &snapshot.SchemaVersion, &snapshot.AdapterRevision, &payload, &hash, &retrieved, &expires, &snapshot.CapturedResource, &snapshot.CaptureRevision); err != nil {
			rows.Close()
			return nil, err
		}
		if len(snapshot.CapturedResource) > 128 || (snapshot.CapturedResource != "" && !validSnapshotKey(SpotifySnapshotKey{EntityType: snapshot.EntityType, SpotifyID: snapshot.SpotifyID, Resource: snapshot.CapturedResource, ContextKey: "durable"})) || !validSnapshotCaptureRevision(SpotifyEntitySnapshot{SpotifySnapshotKey: SpotifySnapshotKey{EntityType: snapshot.EntityType}, CaptureRevision: snapshot.CaptureRevision}) {
			rows.Close()
			return nil, errors.New("invalid retained collection provenance")
		}
		snapshot.Payload = json.RawMessage(payload)
		total += len(snapshot.Payload)
		sanitized, err := metadata.Sanitize(snapshot.Payload, metadata.CatalogLimit)
		digest := sha256.Sum256(snapshot.Payload)
		if err != nil || !bytes.Equal(sanitized, snapshot.Payload) || hex.EncodeToString(digest[:]) != hash || !validSnapshotKey(SpotifySnapshotKey{EntityType: snapshot.EntityType, SpotifyID: snapshot.SpotifyID, Resource: snapshot.Resource, ContextKey: "durable"}) || snapshot.SchemaVersion != 1 || snapshot.AdapterRevision == "" || expires <= retrieved || len(result.Snapshots) >= 256 || total > 8<<20 {
			rows.Close()
			return nil, errors.New("invalid retained catalog snapshot")
		}
		snapshot.RetrievedAt = time.UnixMilli(retrieved).UTC()
		snapshot.ExpiresAt = time.UnixMilli(expires).UTC()
		snapshot.Stale = !time.Now().Before(snapshot.ExpiresAt)
		parentResources[[3]string{snapshot.EntityType, snapshot.SpotifyID, snapshot.Resource}] = true
		result.Snapshots = append(result.Snapshots, snapshot)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = d.conn.QueryContext(ctx, `SELECT parent_type,parent_id,resource,relation_kind,position,child_type,child_id,unavailable,metadata_json FROM spotify_download_catalog_relations
 WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=? AND recording_id=? ORDER BY parent_type,parent_id,resource,relation_kind,position LIMIT 20001`, revision.path, revision.digest, revision.size, revision.mtime, recording)
	if err != nil {
		return nil, err
	}
	total = 0
	for rows.Next() {
		relation := DownloadedCatalogRelation{}
		var rawMetadata []byte
		if err := rows.Scan(&relation.ParentType, &relation.ParentID, &relation.Resource, &relation.Kind, &relation.Position, &relation.ChildType, &relation.ChildID, &relation.Unavailable, &rawMetadata); err != nil {
			rows.Close()
			return nil, err
		}
		relation.Metadata = json.RawMessage(rawMetadata)
		total += len(relation.Metadata)
		sanitized, err := metadata.Sanitize(relation.Metadata, metadata.ScalarLimit)
		if !parentResources[[3]string{relation.ParentType, relation.ParentID, relation.Resource}] || relation.Kind == "" || len(relation.Kind) > 128 || len(relation.ChildType) > 128 || (relation.ChildID != "" && !ValidSpotifyRecordingID(relation.ChildID)) || err != nil || !bytes.Equal(sanitized, relation.Metadata) || relation.Position < 0 || len(result.Relations) >= 20000 || total > 2<<20 {
			rows.Close()
			return nil, errors.New("invalid retained catalog relation")
		}
		result.Relations = append(result.Relations, relation)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	// Verify the observation identity, including source resource/revision and
	// ordered relation metadata, before exposing immutable requested evidence.
	for _, snapshot := range result.Snapshots {
		if snapshot.CapturedResource == "" {
			if snapshot.CaptureRevision != "" {
				return nil, errors.New("unbound collection revision")
			}
			continue
		}
		if snapshot.EntityType != "playlist" && snapshot.EntityType != "library" {
			return nil, errors.New("invalid collection parent")
		}
		hash := sha256.Sum256(snapshot.Payload)
		source := SpotifyEntitySnapshot{SpotifySnapshotKey: SpotifySnapshotKey{EntityType: snapshot.EntityType, SpotifyID: snapshot.SpotifyID, Resource: snapshot.CapturedResource}, CaptureRevision: snapshot.CaptureRevision, PayloadHash: hex.EncodeToString(hash[:])}
		for _, r := range result.Relations {
			if r.ParentType == snapshot.EntityType && r.ParentID == snapshot.SpotifyID && r.Resource == snapshot.Resource {
				source.Relations = append(source.Relations, SpotifyEntityRelation{Kind: r.Kind, Position: r.Position, ChildType: r.ChildType, ChildID: r.ChildID, Unavailable: r.Unavailable, Metadata: r.Metadata})
			}
		}
		resource, err := requestedObservationResource(source, snapshot.RetrievedAt.UnixMilli(), snapshot.ExpiresAt.UnixMilli())
		if err != nil || resource != snapshot.Resource {
			return nil, errors.New("retained collection identity mismatch")
		}
	}
	if err := readDownloadLineage(ctx, d.conn, revision, recording, result); err != nil {
		return nil, err
	}
	if err := readDownloadCollectionStatus(ctx, d.conn, revision, recording, result); err != nil {
		return nil, err
	}
	final, finalRecording, admitted, err := d.verifiedCatalogBinding(ctx, songID, fingerprint)
	if err != nil {
		return nil, err
	}
	if !admitted || final != revision || finalRecording != recording {
		return nil, nil
	}
	if len(result.Snapshots) == 0 && result.CatalogStatus == nil && len(result.Origins) == 0 && result.LineageStatus == nil && result.CollectionStatus == nil {
		return nil, nil
	}
	return result, nil
}
