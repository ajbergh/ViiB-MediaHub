# Library Operations

Library Operations is the maintenance surface for a local ViiB MediaHub library. Open it from the **Library Health** shortcut in the desktop layout. It is intentionally separate from normal scanning and playback controls because repair and restore actions can affect the local SQLite database.

## Diagnostics

**Run diagnostics** checks:

- SQLite `integrity_check`.
- Missing media files and song records that resolve to directories.
- Search-index row count and library revision-log state.
- Playlist references whose song IDs no longer exist.
- Files temporarily quarantined after repeated metadata extraction failures.

Diagnostics do not change the library.

## Repair

**Repair indexes** rebuilds the server-search index and removes broken playlist references. It does not remove song records for missing media.

**Repair and remove missing media** performs the same repair and also removes records for files that diagnostics confirmed are missing. This affects the database only; it never deletes media files from disk.

## Metadata editing

The operations API can edit database metadata for a song: title, artist, album, album artist, track/disc number, genre, and year. Source-file tag write-back is deliberately unavailable in this build. A request that asks for write-back is rejected before database metadata changes.

## Backup and staged restore

Creating a backup:

1. Creates a consistent SQLite snapshot using `VACUUM INTO` after a WAL checkpoint.
2. Validates the snapshot with SQLite integrity checking.
3. Stores `library.db` and a SHA-256 manifest in a ZIP archive in the application data directory.

**Preview restore** validates the archive, manifest checksum, and SQLite database without changing the active library.

**Stage restore** copies the validated database into the pending-restore area. Close ViiB, then run `viib-restore` to activate it. The helper keeps the previous `library.db` as a timestamped rollback database before replacement. Restore activation is offline so the application never replaces its live database while SQLite is open.

## Continuous monitoring

Continuous monitoring periodically performs quick change detection, adds deletion detection, coalesces duplicate paths, and queues any work through the bounded background scanner. Choose an interval from 2 seconds to 1 hour. It is useful for filesystems where native journal updates are unavailable or incomplete.

The watcher status reports whether monitoring is running, its interval, check count, the change count from the most recent check, and the last error when one occurred.

## Safety notes

- Backups and restore archives contain local library metadata. Store them with the same care as the application data directory.
- Repair and restore act only on the ViiB SQLite database. They do not delete or rewrite audio files.
- A physical Windows release-candidate smoke test remains the final release gate for a real large library, output switching, continuous monitoring, backup, offline restore, long playback, and Spotify reconnection.
