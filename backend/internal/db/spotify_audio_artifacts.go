package db

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"time"

	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	"github.com/ajbergh/viib-mediahub/internal/spotify/metadata"
	"github.com/ajbergh/viib-mediahub/internal/spotify/waveform"
)

type SpotifyAudioArtifact struct {
	ProviderETag    string    `json:"providerEtag,omitempty"`
	TrackID         string    `json:"trackId"`
	Resource        string    `json:"resource"`
	Kind            string    `json:"kind"`
	ContextKey      string    `json:"-"`
	SchemaVersion   int       `json:"schemaVersion"`
	AdapterRevision string    `json:"adapterRevision"`
	Payload         []byte    `json:"-"`
	PayloadHash     string    `json:"payloadHash"`
	RetrievedAt     time.Time `json:"retrievedAt"`
	ExpiresAt       time.Time `json:"expiresAt"`
}

// putSpotifyAudioDomainTx retains complete domain snapshots and independently
// validated arrays. Missing/rejected arrays never replace last-good artifacts.
func putSpotifyAudioDomainTx(tx *sql.Tx, o spotifyanalysis.Observation, revision string, expires time.Time) error {
	if len(o.DomainPayload) == 0 {
		if len(o.ArtifactCapabilities) > 0 {
			return errors.New("detailed capabilities require durable artifacts")
		}
		return nil
	}
	if o.AccountContext == "" || len(o.AccountContext) > 256 {
		return errors.New("audio artifact account context required")
	}
	limit := metadata.DetailedLimit
	if o.SourceEndpoint == "audio_features" {
		limit = metadata.ScalarLimit
	}
	safe, err := metadata.Sanitize(o.DomainPayload, limit)
	if err != nil {
		return err
	}
	validated, err := spotifyanalysis.ValidateDomainPayload(o.TrackID, o.SourceEndpoint, safe)
	if err != nil {
		return err
	}
	if !slices.Equal(validated.ArtifactCapabilities, o.ArtifactCapabilities) {
		return errors.New("audio artifact capabilities mismatch")
	}
	artifacts := map[string][]byte{"domain": safe}
	if o.SourceEndpoint == "audio_analysis" {
		var object map[string]json.RawMessage
		if json.Unmarshal(safe, &object) != nil {
			return errors.New("invalid detailed domain")
		}
		for _, name := range o.ArtifactCapabilities {
			raw, ok := object[name]
			if !ok {
				return errors.New("missing detailed capability payload")
			}
			encoded, err := json.Marshal(map[string]json.RawMessage{name: raw})
			if err != nil {
				return err
			}
			artifacts[name] = encoded
		}
	}
	for kind, payload := range artifacts {
		var packed bytes.Buffer
		writer := gzip.NewWriter(&packed)
		if _, err = writer.Write(payload); err != nil {
			return err
		}
		if err = writer.Close(); err != nil {
			return err
		}
		if packed.Len() > metadata.DetailedLimit {
			return errors.New("compressed audio artifact exceeds limit")
		}
		sum := sha256.Sum256(payload)
		_, err = tx.Exec(`INSERT INTO spotify_audio_artifacts(spotify_id,resource,artifact_kind,context_key,schema_version,adapter_revision,encoding,payload,payload_hash,decoded_size,retrieved_at,expires_at)
 VALUES(?,?,?,?,?,?,'gzip-json',?,?,?,?,?) ON CONFLICT(spotify_id,resource,artifact_kind,context_key) DO UPDATE SET
 schema_version=excluded.schema_version,adapter_revision=excluded.adapter_revision,encoding=excluded.encoding,payload=excluded.payload,payload_hash=excluded.payload_hash,decoded_size=excluded.decoded_size,retrieved_at=excluded.retrieved_at,expires_at=excluded.expires_at
 WHERE excluded.retrieved_at>spotify_audio_artifacts.retrieved_at`, o.TrackID, o.SourceEndpoint, kind, o.AccountContext, 1, revision, packed.Bytes(), hex.EncodeToString(sum[:]), len(payload), o.RetrievedAt.UnixMilli(), expires.UnixMilli())
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) GetSpotifyAudioArtifact(id, resource, kind, contextKey string) (*SpotifyAudioArtifact, error) {
	return getSpotifyAudioArtifact(d.conn, id, resource, kind, contextKey)
}

func getSpotifyAudioArtifact(query interface{ QueryRow(string, ...any) *sql.Row }, id, resource, kind, contextKey string) (*SpotifyAudioArtifact, error) {
	if !(validExternalKey(id, resource) || (ValidSpotifyRecordingID(id) && resource == "three_band_waveform" && kind == "spotify_three_band")) || contextKey == "" || len(contextKey) > 256 {
		return nil, errors.New("invalid Spotify audio artifact key")
	}
	switch kind {
	case "domain", "bars", "beats", "tatums", "sections", "segments", "spotify_three_band":
	default:
		return nil, errors.New("invalid Spotify audio artifact kind")
	}
	value := SpotifyAudioArtifact{TrackID: id, Resource: resource, Kind: kind, ContextKey: contextKey}
	var encoded []byte
	var encoding string
	var size int
	var retrieved, expires int64
	err := query.QueryRow(`SELECT schema_version,adapter_revision,encoding,payload,payload_hash,decoded_size,retrieved_at,expires_at,provider_etag FROM spotify_audio_artifacts WHERE spotify_id=? AND resource=? AND artifact_kind=? AND context_key=?`, id, resource, kind, contextKey).Scan(&value.SchemaVersion, &value.AdapterRevision, &encoding, &encoded, &value.PayloadHash, &size, &retrieved, &expires, &value.ProviderETag)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return decodeSpotifyAudioArtifact(value, encoding, encoded, size, retrieved, expires)
}

func decodeSpotifyAudioArtifact(value SpotifyAudioArtifact, encoding string, encoded []byte, size int, retrieved, expires int64) (*SpotifyAudioArtifact, error) {
	resource := value.Resource
	if (encoding != "gzip-json" && !(resource == "three_band_waveform" && encoding == "gzip-protobuf")) || len(encoded) > metadata.DetailedLimit || size < 0 || size > metadata.DetailedLimit {
		return nil, errors.New("invalid audio artifact encoding or size")
	}
	reader, err := gzip.NewReader(bytes.NewReader(encoded))
	if err != nil {
		return nil, errors.New("invalid compressed audio artifact")
	}
	defer reader.Close()
	payload, err := io.ReadAll(io.LimitReader(reader, metadata.DetailedLimit+1))
	if err != nil || len(payload) != size || len(payload) > metadata.DetailedLimit {
		return nil, errors.New("invalid decoded audio artifact size")
	}
	sum := sha256.Sum256(payload)
	if hex.EncodeToString(sum[:]) != value.PayloadHash {
		return nil, errors.New("audio artifact integrity mismatch")
	}
	limit := metadata.DetailedLimit
	if resource == "audio_features" {
		limit = metadata.ScalarLimit
	}
	if resource == "three_band_waveform" {
		if _, err = waveform.DecodeDomain(payload); err != nil {
			return nil, err
		}
	} else if _, err = metadata.Sanitize(payload, limit); err != nil {
		return nil, err
	}
	value.Payload = payload
	value.RetrievedAt = time.UnixMilli(retrieved).UTC()
	value.ExpiresAt = time.UnixMilli(expires).UTC()
	return &value, nil
}

// PutSpotifyWaveform persists validated native samples and unknown protobuf fields.
func (d *DB) PutSpotifyWaveform(id, contextKey string, w waveform.Waveform, revision string, retrieved, expires time.Time) error {
	return d.putSpotifyWaveform(id, contextKey, w, revision, retrieved, expires, nil)
}

func (d *DB) PutSpotifyWaveformForRuntime(fence SpotifyMetadataFence, id string, w waveform.Waveform, revision string, retrieved, expires time.Time) error {
	return d.putSpotifyWaveform(id, fence.ContextKey, w, revision, retrieved, expires, &fence)
}

func (d *DB) putSpotifyWaveform(id, contextKey string, w waveform.Waveform, revision string, retrieved, expires time.Time, fence *SpotifyMetadataFence) error {
	if !ValidSpotifyRecordingID(id) || w.TrackID != id || contextKey == "" || len(contextKey) > 256 || revision == "" || len(revision) > 256 || retrieved.IsZero() || !expires.After(retrieved) || w.Validate() != nil {
		return errors.New("invalid Spotify waveform provenance")
	}
	decoded, err := waveform.DecodeDomain(w.Domain)
	if err != nil {
		return err
	}
	if decoded.SampleRate != w.SampleRate || decoded.WindowMilliseconds != w.WindowMilliseconds || !slices.Equal(decoded.Lows, w.Lows) || !slices.Equal(decoded.Mids, w.Mids) || !slices.Equal(decoded.Highs, w.Highs) {
		return errors.New("Spotify waveform projection mismatch")
	}
	var packed bytes.Buffer
	writer := gzip.NewWriter(&packed)
	if _, err = writer.Write(w.Domain); err != nil {
		return err
	}
	if err = writer.Close(); err != nil {
		return err
	}
	if packed.Len() > waveform.MaxBody {
		return errors.New("Spotify waveform exceeds storage limit")
	}
	sum := sha256.Sum256(w.Domain)
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if fence != nil {
		if err := checkSpotifyMetadataFenceTx(tx, *fence); err != nil {
			return err
		}
	}
	result, err := tx.Exec(`INSERT INTO spotify_audio_artifacts(spotify_id,resource,artifact_kind,context_key,schema_version,adapter_revision,encoding,payload,payload_hash,decoded_size,retrieved_at,expires_at,provider_etag)
 VALUES(?,'three_band_waveform','spotify_three_band',?,1,?,'gzip-protobuf',?,?,?,?,?,?)
 ON CONFLICT(spotify_id,resource,artifact_kind,context_key) DO UPDATE SET adapter_revision=excluded.adapter_revision,payload=excluded.payload,payload_hash=excluded.payload_hash,decoded_size=excluded.decoded_size,retrieved_at=excluded.retrieved_at,expires_at=excluded.expires_at,provider_etag=excluded.provider_etag WHERE excluded.retrieved_at>spotify_audio_artifacts.retrieved_at`, id, contextKey, revision, packed.Bytes(), hex.EncodeToString(sum[:]), len(w.Domain), retrieved.UnixMilli(), expires.UnixMilli(), w.ETag)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count > 0 {
		_, err = tx.Exec(`INSERT INTO spotify_metadata_resource_status(entity_type,spotify_id,resource,context_key,state,reason,checked_at,retry_at) VALUES('track',?,'three_band_waveform',?,'available','',?,?) ON CONFLICT(entity_type,spotify_id,resource,context_key) DO UPDATE SET state=excluded.state,reason=excluded.reason,checked_at=excluded.checked_at,retry_at=excluded.retry_at WHERE excluded.checked_at>spotify_metadata_resource_status.checked_at`, id, contextKey, retrieved.UnixMilli(), retrieved.UnixMilli())
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
