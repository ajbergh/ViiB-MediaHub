package db

// Domain-ID reachability is shared by promotion and outcome reporting. Exact
// parent-resource joins prevent expired resources from supplying graph edges.
// UNION terminates cycles; track listings are retained but never traversed.
const downloadedCatalogGraphCTE = `WITH RECURSIVE
 active AS (SELECT value AS context_key FROM settings WHERE key='spotify_metadata_active_context' AND value<>''),
 fresh AS (SELECT s.* FROM spotify_entity_snapshots s JOIN active a ON a.context_key=s.context_key
 WHERE s.schema_version=1 AND s.expires_at>? AND s.entity_type IN ('track','album','artist')),
 reachable(entity_type,spotify_id) AS (
 SELECT entity_type,spotify_id FROM fresh WHERE entity_type='track' AND spotify_id=?
 UNION
 SELECT r.child_type,r.child_id FROM reachable p
 JOIN fresh parent ON parent.entity_type=p.entity_type AND parent.spotify_id=p.spotify_id
 JOIN spotify_entity_relations r ON r.entity_type=parent.entity_type AND r.spotify_id=parent.spotify_id
 AND r.resource=parent.resource AND r.context_key=parent.context_key
 JOIN fresh child ON child.entity_type=r.child_type AND child.spotify_id=r.child_id
 WHERE r.unavailable=0 AND r.child_type IN ('album','artist')),
 eligible AS (SELECT s.* FROM fresh s JOIN reachable p ON p.entity_type=s.entity_type AND p.spotify_id=s.spotify_id),
 relations AS (SELECT r.* FROM spotify_entity_relations r JOIN eligible p
 ON p.entity_type=r.entity_type AND p.spotify_id=r.spotify_id AND p.resource=r.resource AND p.context_key=r.context_key
 WHERE r.child_type IN ('track','album','artist') OR (r.unavailable=1 AND r.child_type=''))
`

const downloadedCatalogGraphScope = "track_album_artist_graph_v2"
