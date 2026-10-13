-- Generated from v1.0.0-rc5 DB.New + EnsureTrackAnalysisSchema. Synthetic schema only.
CREATE TABLE album_metadata (
		album_key TEXT PRIMARY KEY,
		album_name TEXT NOT NULL,
		artist_name TEXT NOT NULL,
		spotify_id TEXT,
		cover_url TEXT,
		local_cover_path TEXT,
		description TEXT,
		genre TEXT,
		release_date TEXT,
		spotify_url TEXT,
		copyright TEXT,
		spotify_checked INTEGER DEFAULT 0,
		spotify_found INTEGER DEFAULT 0,
		fetched_at INTEGER,
		updated_at INTEGER
	, liked INTEGER DEFAULT 0, liked_at INTEGER);
CREATE TABLE artist_metadata (
		artist_name TEXT PRIMARY KEY,
		spotify_id TEXT,
		image_url TEXT,
		plex_image_url TEXT,
		local_image_path TEXT,
		spotify_url TEXT,
		spotify_checked INTEGER DEFAULT 0,
		spotify_found INTEGER DEFAULT 0,
		fetched_at INTEGER,
		updated_at INTEGER
	, lastfm_listeners INTEGER, lastfm_playcount INTEGER, lastfm_tags TEXT, lastfm_bio TEXT, lastfm_url TEXT, lastfm_mbid TEXT, lastfm_enriched_at INTEGER);
CREATE TABLE directory_signatures (
		path TEXT PRIMARY KEY,
		file_count INTEGER NOT NULL,
		total_size INTEGER NOT NULL,
		latest_mtime INTEGER NOT NULL,
		content_hash TEXT NOT NULL,
		last_verified INTEGER NOT NULL
	);
CREATE TABLE dj_hot_cue_suppressions (
		song_id TEXT NOT NULL,
		slot INTEGER NOT NULL CHECK(slot BETWEEN 1 AND 8),
		kind TEXT NOT NULL,
		created_at INTEGER NOT NULL,
		PRIMARY KEY(song_id, slot, kind),
		FOREIGN KEY (song_id) REFERENCES songs(id) ON DELETE CASCADE
	);
CREATE TABLE dj_hot_cues (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		song_id TEXT NOT NULL,
		slot INTEGER NOT NULL,
		position REAL NOT NULL,
		label TEXT,
		color TEXT DEFAULT '#FF5500',
		origin TEXT NOT NULL DEFAULT 'user',
		generator_version TEXT,
		confidence REAL,
		kind TEXT,
		locked INTEGER NOT NULL DEFAULT 0,
		rationale TEXT,
		source_fingerprint TEXT,
		downbeat_aligned INTEGER NOT NULL DEFAULT 0,
		updated_at INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL,
		FOREIGN KEY (song_id) REFERENCES songs(id) ON DELETE CASCADE,
		UNIQUE(song_id, slot)
	);
CREATE TABLE dj_waveform_cache (
		song_id TEXT PRIMARY KEY,
		duration REAL NOT NULL,
		sample_rate INTEGER NOT NULL,
		resolution INTEGER NOT NULL,
		peaks_data BLOB NOT NULL,
		peak_count INTEGER NOT NULL,
		created_at INTEGER NOT NULL,
		FOREIGN KEY (song_id) REFERENCES songs(id) ON DELETE CASCADE
	);
CREATE TABLE external_track_analysis (
 provider TEXT NOT NULL CHECK(provider='spotify'),
 external_id TEXT NOT NULL,
 endpoint TEXT NOT NULL CHECK(endpoint IN ('audio_analysis','audio_features')),
 schema_version INTEGER NOT NULL,
 observation_json TEXT NOT NULL CHECK(length(observation_json)<=16384),
 observation_hash TEXT NOT NULL,
 adapter_revision TEXT NOT NULL,
 retrieved_at INTEGER NOT NULL,
 expires_at INTEGER NOT NULL,
 PRIMARY KEY(provider,external_id,endpoint,schema_version));
CREATE TABLE external_track_analysis_status (
 provider TEXT NOT NULL CHECK(provider='spotify'),
 external_id TEXT NOT NULL,
 endpoint TEXT NOT NULL CHECK(endpoint IN ('audio_analysis','audio_features')),
 schema_version INTEGER NOT NULL,
 code TEXT NOT NULL,
 checked_at INTEGER NOT NULL,
 retry_at INTEGER NOT NULL,
 PRIMARY KEY(provider,external_id,endpoint,schema_version));
CREATE TABLE file_metadata_cache (
		file_path TEXT PRIMARY KEY,
		file_size INTEGER NOT NULL,
		mtime INTEGER NOT NULL,
		metadata_hash TEXT,
		last_verified INTEGER NOT NULL
	);
CREATE TABLE genre_preferences (
		genre TEXT PRIMARY KEY,
		play_count INTEGER DEFAULT 0,
		skip_count INTEGER DEFAULT 0,
		complete_rate REAL DEFAULT 0.5,  -- 0.0-1.0, ratio of plays that complete
		skip_early_rate REAL DEFAULT 0,  -- 0.0-1.0, ratio of skips < 10s
		affinity_score REAL DEFAULT 0.5, -- 0.0-1.0, overall preference score
		last_updated INTEGER
	);
CREATE TABLE genre_stats (
		name TEXT PRIMARY KEY,
		count INTEGER NOT NULL,
		artists TEXT NOT NULL, -- JSON array of top artists
		cover_url TEXT
	);
CREATE TABLE lastfm_similar_artists (
		artist_name TEXT NOT NULL,
		similar_artist TEXT NOT NULL,
		match_score REAL NOT NULL,
		PRIMARY KEY (artist_name, similar_artist)
	);
CREATE TABLE lastfm_similar_tracks (
		song_id TEXT NOT NULL,
		similar_artist TEXT NOT NULL,
		similar_track TEXT NOT NULL,
		match_score REAL NOT NULL,
		similar_song_id TEXT,
		FOREIGN KEY (song_id) REFERENCES songs(id) ON DELETE CASCADE,
		PRIMARY KEY (song_id, similar_artist, similar_track)
	);
CREATE TABLE listening_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		song_id TEXT NOT NULL,
		event_type TEXT NOT NULL,  -- 'play_complete', 'skip_early' (<10s), 'skip_mid' (10-30s), 'skip_late' (>30s)
		play_duration REAL,        -- How many seconds played before skip/complete
		song_duration REAL,        -- Total song duration
		timestamp INTEGER NOT NULL,
		genre TEXT,                -- Cached genre for aggregation
		mood TEXT,                 -- Cached mood for aggregation
		energy TEXT,               -- Cached energy for aggregation
		context TEXT               -- 'ai_dj', 'album', 'playlist', 'queue', 'search'
	);
CREATE TABLE playlists (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		song_ids TEXT NOT NULL,
		cover_path TEXT,
		created_at INTEGER NOT NULL
	);
CREATE TABLE scan_folders (
		id TEXT PRIMARY KEY,
		path TEXT UNIQUE NOT NULL,
		added_at INTEGER NOT NULL,
		last_scan INTEGER,
		song_count INTEGER DEFAULT 0
	);
CREATE TABLE scan_state (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		last_scan_time INTEGER NOT NULL,
		windows_usn INTEGER,
		macos_event_id INTEGER,
		linux_last_mtime INTEGER,
		scan_duration_ms INTEGER,
		files_scanned INTEGER,
		files_changed INTEGER
	);
CREATE TABLE settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);
CREATE TABLE songs (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		artist TEXT NOT NULL,
		album TEXT NOT NULL,
		album_artist TEXT,
		track_number INTEGER,
		disc_number INTEGER,
		genre TEXT,
		year INTEGER,
		duration REAL,
		file_path TEXT UNIQUE NOT NULL,
		cover_path TEXT,
		added_at INTEGER NOT NULL,
		play_count INTEGER DEFAULT 0,
		last_played INTEGER,
		skip_count INTEGER DEFAULT 0,
		file_hash TEXT,
		mood TEXT,
		energy TEXT,
		tempo TEXT,
		bpm INTEGER,
		instrumental INTEGER DEFAULT 0,
		mood_analyzed_at INTEGER
	, liked INTEGER DEFAULT 0, liked_at INTEGER, original_year INTEGER, year_uncertain INTEGER DEFAULT 0, year_analyzed_at INTEGER, replay_gain_db REAL, replay_peak REAL, ignored INTEGER DEFAULT 0, lastfm_listeners INTEGER, lastfm_playcount INTEGER, lastfm_tags TEXT, lastfm_url TEXT, lastfm_mbid TEXT, lastfm_enriched_at INTEGER);
CREATE TABLE spotify_download_evidence (
  file_path TEXT NOT NULL, content_sha256 TEXT NOT NULL, file_size INTEGER NOT NULL,
  mtime_ns INTEGER NOT NULL, spotify_id TEXT NOT NULL, completed_at INTEGER NOT NULL, features_json TEXT NOT NULL DEFAULT '' CHECK(length(features_json)<=16384),
  PRIMARY KEY(file_path,content_sha256,file_size,mtime_ns,spotify_id));
CREATE TABLE spotify_downloads (
		id TEXT PRIMARY KEY,
		spotify_id TEXT NOT NULL,
		spotify_uri TEXT NOT NULL,
		type TEXT NOT NULL,
		title TEXT NOT NULL,
		artist TEXT,
		album TEXT,
		status TEXT NOT NULL,
		progress INTEGER DEFAULT 0,
		error TEXT,
		file_path TEXT,
		added_at INTEGER NOT NULL,
		started_at INTEGER,
		completed_at INTEGER,
		metadata TEXT
	);
CREATE TABLE track_analysis (
			song_id TEXT PRIMARY KEY REFERENCES songs(id) ON DELETE CASCADE,
			status TEXT NOT NULL CHECK(status IN ('pending', 'running', 'complete', 'partial', 'failed', 'unsupported')),
			analysis_version INTEGER NOT NULL,
			algorithm_version TEXT NOT NULL,
			decoder_id TEXT,
			source_fingerprint TEXT NOT NULL,
			source_size INTEGER,
			source_mtime INTEGER,
			source_revision TEXT,
			bpm REAL,
			bpm_confidence REAL,
			bpm_alt_candidate REAL,
			tempo_stability REAL,
			tempo_kind TEXT CHECK(tempo_kind IS NULL OR tempo_kind IN ('unknown', 'static', 'dynamic-candidate', 'dynamic')),
			bpm_source TEXT CHECK(bpm_source IS NULL OR bpm_source IN ('measured', 'spotify', 'imported', 'manual', 'legacy-ai')),
			key_tonic INTEGER CHECK(key_tonic IS NULL OR key_tonic BETWEEN 0 AND 11),
			key_mode TEXT CHECK(key_mode IS NULL OR key_mode IN ('major', 'minor')),
			key_confidence REAL,
			key_source TEXT CHECK(key_source IS NULL OR key_source IN ('measured', 'spotify', 'imported', 'manual')),
			camelot_key TEXT,
			open_key TEXT,
			energy_level INTEGER CHECK(energy_level IS NULL OR energy_level BETWEEN 1 AND 10),
			energy_level_confidence REAL CHECK(energy_level_confidence IS NULL OR energy_level_confidence BETWEEN 0 AND 1),
			energy_algorithm_version TEXT,
			analyzed_at INTEGER,
			error_code TEXT,
			error_message TEXT
		);
CREATE TABLE track_analysis_artifacts (
			id TEXT PRIMARY KEY,
			song_id TEXT NOT NULL REFERENCES songs(id) ON DELETE CASCADE,
			kind TEXT NOT NULL,
			format_version INTEGER NOT NULL,
			algorithm_version TEXT NOT NULL,
			encoding TEXT NOT NULL,
			provenance TEXT NOT NULL DEFAULT 'unknown' CHECK(provenance IN ('measured', 'inferred-from-meter', 'manual', 'unknown')),
			source_fingerprint TEXT NOT NULL DEFAULT '',
			data BLOB NOT NULL,
			created_at INTEGER NOT NULL,
			UNIQUE(song_id, kind, format_version, algorithm_version)
		);
CREATE TABLE track_analysis_overrides (
			song_id TEXT PRIMARY KEY REFERENCES songs(id) ON DELETE CASCADE,
			bpm REAL,
			bpm_source_fingerprint TEXT NOT NULL DEFAULT '',
			key_tonic INTEGER CHECK(key_tonic IS NULL OR key_tonic BETWEEN 0 AND 11),
			key_mode TEXT CHECK(key_mode IS NULL OR key_mode IN ('major', 'minor')),
			beatgrid_artifact_id TEXT,
			bpm_locked INTEGER NOT NULL DEFAULT 0 CHECK(bpm_locked IN (0, 1)),
			key_locked INTEGER NOT NULL DEFAULT 0 CHECK(key_locked IN (0, 1)),
			beatgrid_locked INTEGER NOT NULL DEFAULT 0 CHECK(beatgrid_locked IN (0, 1)),
			updated_at INTEGER NOT NULL
		);
CREATE TABLE track_analysis_source_revisions (
			song_id TEXT PRIMARY KEY REFERENCES songs(id) ON DELETE CASCADE,
			source_fingerprint TEXT NOT NULL,
			updated_at INTEGER NOT NULL
		);
CREATE TABLE track_external_identity (
 song_id TEXT NOT NULL REFERENCES songs(id) ON DELETE CASCADE,
 provider TEXT NOT NULL CHECK(provider='spotify'),
 external_id TEXT NOT NULL,
 link_origin TEXT NOT NULL CHECK(link_origin IN ('manual_confirmation','download_completion','automatic_search')),
 source_fingerprint TEXT NOT NULL,
 confirmed_at INTEGER NOT NULL,
 PRIMARY KEY(song_id,provider));
CREATE TABLE track_external_identity_suppression (
  song_id TEXT NOT NULL REFERENCES songs(id) ON DELETE CASCADE,
  source_fingerprint TEXT NOT NULL, PRIMARY KEY(song_id,source_fingerprint));
CREATE INDEX idx_album_metadata_artist ON album_metadata(artist_name);
CREATE INDEX idx_album_metadata_liked ON album_metadata(liked) WHERE liked = 1;
CREATE INDEX idx_dir_sig_mtime ON directory_signatures(latest_mtime);
CREATE INDEX idx_dj_hot_cues_song ON dj_hot_cues(song_id);
CREATE INDEX idx_downloads_added_at ON spotify_downloads(added_at);
CREATE INDEX idx_downloads_status ON spotify_downloads(status);
CREATE INDEX idx_external_analysis_cooldown ON external_track_analysis_status(provider,code,schema_version,retry_at);
CREATE INDEX idx_listening_events_song ON listening_events(song_id);
CREATE INDEX idx_listening_events_timestamp ON listening_events(timestamp);
CREATE INDEX idx_listening_events_type ON listening_events(event_type);
CREATE INDEX idx_metadata_cache_mtime ON file_metadata_cache(mtime);
CREATE INDEX idx_similar_artists ON lastfm_similar_artists(artist_name);
CREATE INDEX idx_similar_tracks_similar_song ON lastfm_similar_tracks(similar_song_id);
CREATE INDEX idx_similar_tracks_song ON lastfm_similar_tracks(song_id);
CREATE INDEX idx_songs_album ON songs(album);
CREATE INDEX idx_songs_artist ON songs(artist);
CREATE INDEX idx_songs_file_path ON songs(file_path);
CREATE INDEX idx_songs_ignored ON songs(ignored);
CREATE INDEX idx_songs_liked ON songs(liked) WHERE liked = 1;
CREATE INDEX idx_songs_original_year ON songs(original_year);
CREATE INDEX idx_songs_year_uncertain ON songs(year_uncertain) WHERE year_uncertain = 1;
CREATE INDEX idx_track_analysis_artifacts_song ON track_analysis_artifacts(song_id);
CREATE INDEX idx_track_analysis_fingerprint ON track_analysis(source_fingerprint);
CREATE INDEX idx_track_analysis_status ON track_analysis(status);
