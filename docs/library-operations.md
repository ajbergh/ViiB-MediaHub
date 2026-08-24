# Library Operations

Library Operations is ViiB MediaHub's music-source and maintenance surface. Open it from the **Library Health** shortcut in the desktop layout. It combines source configuration with database diagnostics/recovery while keeping normal browsing and playback in the main ViiB UI.

## Music sources

### Plex Media Server

Library Operations can connect a Plex Media Server music/audio library to ViiB. The panel supports LAN discovery, manual server addresses, Plex authentication/reconnect, music-library selection, explicit synchronization, offline state, and safe source removal.

Plex synchronization writes normalized metadata into the same `songs` catalog used by local filesystem tracks, with source-specific identity/playback information stored separately. ViiB treats Plex as read-only remote media storage: removing/changing the source changes ViiB's cached catalog only and never deletes, moves, renames, or modifies content on PMS.

See [Plex Media Server Music Support](plex-music.md) for the full setup, security, synchronization, playback, and troubleshooting guide.

## Diagnostics

**Run diagnostics** checks:

- SQLite `integrity_check`.
- Missing **local** media files and song records that resolve to directories. Remote Plex catalog URIs are deliberately excluded from filesystem-missing checks.
- Search-index row count and library revision-log state.
- Playlist references whose song IDs no longer exist.
- Files temporarily quarantined after repeated metadata extraction failures.

Diagnostics do not change the library. A temporarily offline Plex server is represented by the Plex source state and does not cause its cached tracks to be classified as deleted media.

## Repair

**Repair indexes** rebuilds the server-search index and removes broken playlist references. It does not remove song records for missing media.

**Repair and remove missing media** performs the same repair and also removes records for local files that diagnostics confirmed are missing. This affects the database only; it never deletes media files from disk and never interprets Plex remote source URIs as missing filesystem paths.

## Metadata editing

The operations API can edit ViiB database metadata for a song: title, artist, album, album artist, track/disc number, genre, and year. Source-file tag write-back is deliberately unavailable in this build. A request that asks for write-back is rejected before database metadata changes.

For Plex tracks these edits are ViiB-local only; they are never silently written back to PMS. A later authoritative Plex synchronization can replace fields with the server's current metadata.

## Backup and staged restore

Creating a backup:

1. Creates a consistent SQLite snapshot using `VACUUM INTO` after a WAL checkpoint.
2. Validates the snapshot with SQLite integrity checking.
3. Stores `library.db` and a SHA-256 manifest in a ZIP archive in the application data directory.

**Preview restore** validates the archive, manifest checksum, and SQLite database without changing the active library.

**Stage restore** copies the validated database into the pending-restore area. Close ViiB, then run `viib-restore` to activate it. The helper keeps the previous `library.db` as a timestamped rollback database before replacement. Restore activation is offline so the application never replaces its live database while SQLite is open.

## Continuous monitoring

Continuous monitoring periodically performs quick change detection for configured local folders, adds deletion detection, coalesces duplicate paths, and queues any work through the bounded background scanner. Choose an interval from 2 seconds to 1 hour. It is useful for filesystems where native journal updates are unavailable or incomplete.

Plex does not use this filesystem watcher. Plex libraries synchronize through the explicit Plex synchronization operation so temporary remote outages cannot be mistaken for mass deletion.

The watcher status reports whether monitoring is running, its interval, check count, the change count from the most recent check, and the last error when one occurred.

## Safety notes

- Backups and restore archives contain local library metadata and may include encrypted integration credentials as part of the database. Store them with the same care as the application data directory.
- Repair and restore act only on the ViiB SQLite database. They do not delete or rewrite local audio files or Plex-hosted media.
- Removing or changing Plex configuration performs no destructive PMS operation.
- A physical Windows release-candidate smoke test remains the final release gate for a real large library, output switching, continuous monitoring, backup, offline restore, long playback, Plex direct-play/seeking, and Spotify reconnection.
