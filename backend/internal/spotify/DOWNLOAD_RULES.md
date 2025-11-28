# Spotify Download Rules and Metadata Standards

This document defines the file organization and metadata tagging rules for Spotify downloads in Supersonic.

## General Rules

- **All downloads** for the local media provider respect the `config.toml` media folder setting
- **Download source independence**: All rules apply identically regardless of download method:
  - Downloads initiated via Spotify search page context menu
  - Downloads initiated via direct URL/URI input on the downloads page

## Album Download Rules

### Directory Structure
Albums are organized by album artist and album name:
```
/AlbumArtist/Album/
```

### File Naming Convention
Files are named using track number, album artist, and title:
```
TrackNumber-AlbumArtist-Title.ogg
```

**Example:**
```
01-Pink Floyd-Speak to Me.ogg
02-Pink Floyd-Breathe.ogg
```

### Metadata Tags for Albums

| Metadata Field | Value Source | Description |
|---------------|-------------|-------------|
| `TrackNumber` | TrackNumber | Original track number from album |
| `AlbumArtist` | AlbumArtist | Primary album artist |
| `Artist` | Artist | Track-specific artist(s) |
| `Album` | Album | Album name |
| `DiscNumber` | DiscNumber | Disc number for multi-disc albums |
| `Title` | Title | Track title |
| `Date` | ReleaseDate | Album release date (if available) |
| `Genre` | Genre | Genre classification (if available) |

## Spotify Playlist Download Rules

### Directory Structure
Playlists are organized by playlist name:
```
/PlaylistName/
```

### File Naming Convention
Files are named using playlist order, artist, and title:
```
PlaylistTrackOrderNumber-Artist-Title.ogg
```

**Example:**
```
001-The Beatles-Come Together.ogg
002-Led Zeppelin-Stairway to Heaven.ogg
003-Queen-Bohemian Rhapsody.ogg
```

**Note:** Playlist track numbers are zero-padded to 3 digits to maintain proper sorting.

### Metadata Tags for Playlist Tracks

| Metadata Field | Value Source | Description |
|---------------|-------------|-------------|
| `TrackNumber` | PlaylistTrackOrderNumber | Sequential position in playlist (1-based) |
| `AlbumArtist` | PlaylistName | Playlist name serves as album artist |
| `Artist` | Artist | Original track artist(s) |
| `Album` | PlaylistName | Playlist name serves as album name |
| `DiscNumber` | 1 | Always set to 1 for playlists |
| `Title` | Title | Track title |
| `Date` | ReleaseDate | Original track release date (if available) |
| `Genre` | Genre | Genre classification (if available) |

## Implementation Notes

### File Format
- All downloaded tracks are saved in **Ogg Vorbis** format (`.ogg`)
- Metadata is written using Vorbis comment tags

### Filename Sanitization
- Invalid filesystem characters are removed or replaced
- Filenames are sanitized to ensure cross-platform compatibility

### Artwork Handling
- Album artwork is downloaded and saved alongside tracks
- Artwork files are named to match the album/playlist folder structure

### Local Media Provider Integration
- Downloaded tracks are automatically added to the local media library
- Auto-rescan is triggered after successful downloads when local media provider is configured
- Downloads respect the base path configured in `config.toml`

### Duplicate Detection
- Existing files are not re-downloaded
- File existence checks occur before download initiation
- Metadata verification/updates may occur for existing files

## Configuration

The download base directory is determined by the following precedence:

1. Explicit `downloadBaseDir` set via `SetDownloadBaseDir()` (typically set to local media path)
2. `appConfig.DownloadDir` value from configuration
3. Default `"downloads"` directory as fallback

When a local media provider is configured, the download directory is automatically set to the local media provider's path to ensure seamless library integration.
