# ViiB MediaHub

A modern local media player for audio files, built with React 19 + TypeScript frontend and Go 1.22+ backend. Compiles to a single Windows executable that opens in your web browser.

## Features

### 🎵 Audio Playback
- **Multi-Format Support** - MP3, FLAC, M4A, AAC, OGG, OPUS, WAV, and WMA
- **Gapless Playback** - Seamless transitions between tracks
- **Crossfade** - Smooth audio transitions with configurable duration
- **Queue Management** - Add, reorder, and manage playback queue with drag-and-drop

### 🎚️ Audio Enhancement
- **10-Band Equalizer** - Full parametric EQ (32Hz - 16kHz)
- **22 EQ Presets** - Rock, Jazz, Classical, Electronic, Vocal Boost, and more
- **Auto-EQ** - Automatic preset selection based on genre tags
- **Real-time Visualizer** - Waveform, Spectrum, and Aurora modes

### 📚 Library Management
- **Folder Scanning** - Add multiple music folders with incremental scanning
- **Smart Mixes** - Auto-generated playlists based on listening patterns:
  - Heavy Rotation (most played recently)
  - Rediscover Favorites (old favorites you haven't heard in a while)
  - Fresh Finds (recently added)
  - Genre-based mixes (Chill Acoustic, 90s Alternative, etc.)
- **Playlists** - Create, edit, and manage custom playlists
- **Background Enrichment** - Automatic Spotify metadata enhancement for albums/artists

### 🎧 Spotify Integration
- **OAuth PKCE Authentication** - Secure login without exposing client secrets
- **Catalog Search** - Search Spotify for tracks, albums, artists, and playlists
- **Library Access** - Browse your saved albums, playlists, and recently played
- **Metadata Enhancement** - High-resolution artwork and artist images from Spotify
- **Direct Downloads** - Download tracks, albums, and playlists (Premium required)
  - Configurable concurrent downloads (1-10)
  - Real-time progress via Server-Sent Events
  - Organized file structure: `{Artist}/{Album}/{Track}.ogg`
  - Auto-rescan after downloads complete

### 💾 Data & Settings
- **SQLite Database** - Persistent storage for library, playlists, and settings
- **System Tray** - Minimize to tray on Windows
- **Configurable Data Directory** - Choose where your data is stored

## Architecture

```
ViiB-MediaHub/
├── components/         # React UI components
│   ├── context-menus/  # Right-click menu components
│   └── now-playing/    # Full-screen player components
├── pages/              # Route page components
├── services/           # API client, backend service, Spotify service
├── slices/             # Zustand state slices (player, library, spotify, ui)
├── hooks/              # Custom React hooks (audio player, background enrichment)
├── lib/                # Audio engine, smart mix generator, parsers
├── workers/            # Web workers for background tasks
│
└── backend/            # Go HTTP server
    ├── cmd/viib/       # Entry point with system tray
    └── internal/
        ├── api/        # REST API + SSE handlers
        ├── audio/      # Metadata extraction (taglib + dhowden/tag)
        ├── db/         # SQLite with WAL mode
        ├── logger/     # Shared logging facility
        ├── scanner/    # Library scanning with event broadcasting
        ├── server/     # Chi router + middleware
        └── spotify/    # librespot session + downloader
```

## Quick Start

### Prerequisites

- **Node.js** 18+ and npm
- **Go** 1.22+
- **GCC** (for SQLite and librespot-go CGO)
  - Windows: Install via [MSYS2](https://www.msys2.org/) or [TDM-GCC](https://jmeubank.github.io/tdm-gcc/)
  - Ensure `CGO_ENABLED=1` environment variable is set

### Development Mode

Use the dev script for simultaneous frontend and backend:

```powershell
.\scripts\dev.ps1
```

Or run manually:

```powershell
# Terminal 1: Start Go backend (port 8080)
cd backend
$env:CGO_ENABLED=1
go run ./cmd/viib -port 8080 -no-browser -no-tray

# Terminal 2: Start Vite dev server (port 3000)
npm run dev
```

The Vite dev server proxies `/api` requests to the backend.

### Build for Production

```powershell
.\scripts\build.ps1
```

This will:
1. Build the React frontend with Vite
2. Embed the frontend into the Go binary
3. Output `build/ViiB-MediaHub.exe`

### Run the Executable

```powershell
.\build\ViiB-MediaHub.exe
```

The app will:
1. Start an HTTP server on an available port
2. Open your default browser to the app
3. Store data in `%APPDATA%/ViiB-MediaHub/`
4. Display a system tray icon

### Command Line Options

```
-port <n>        Port to run on (0 = auto-select)
-no-browser      Don't open browser automatically
-no-tray         Disable system tray icon
-data <path>     Custom data directory
```

## API Endpoints

### Library

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/songs` | Get all songs |
| DELETE | `/api/songs` | Clear library |
| POST | `/api/songs/{id}/play` | Record play count |
| GET | `/api/playlists` | Get playlists |
| POST | `/api/playlists` | Create playlist |
| PUT | `/api/playlists/{id}` | Update playlist |
| DELETE | `/api/playlists/{id}` | Delete playlist |
| GET | `/api/folders` | Get scan folders |
| POST | `/api/folders` | Add scan folder |
| DELETE | `/api/folders/{id}` | Remove folder |
| POST | `/api/scan` | Start library scan |
| GET | `/api/scan/status` | Get scan status |
| GET | `/api/library/events` | SSE stream for library events |

### Media

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/audio/{id}` | Stream audio file |
| GET | `/api/cover/{id}` | Get album cover |
| POST | `/api/browse` | Browse filesystem folders |

### Metadata Cache

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/albums/metadata` | Get all cached album metadata |
| GET | `/api/albums/metadata/unchecked` | Get albums not yet checked on Spotify |
| POST | `/api/albums/metadata` | Save album metadata |
| GET | `/api/artists/metadata` | Get all cached artist metadata |
| POST | `/api/artists/metadata` | Save artist metadata |

### Spotify

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/spotify/credentials` | Save OAuth credentials |
| GET | `/api/spotify/credentials` | Get OAuth credentials |
| GET | `/api/spotify/search` | Proxy Spotify search API |
| GET | `/api/spotify/me` | Get user profile |
| GET/POST | `/api/spotify/proxy` | Proxy any Spotify API call |
| POST | `/api/spotify/download/track` | Queue track download |
| POST | `/api/spotify/download/album` | Queue album download |
| POST | `/api/spotify/download/playlist` | Queue playlist download |
| POST | `/api/spotify/download/url` | Download from Spotify URL |
| GET | `/api/spotify/downloads` | List all downloads |
| GET | `/api/spotify/downloads/{id}` | Get download status |
| DELETE | `/api/spotify/downloads/{id}` | Delete download |
| POST | `/api/spotify/downloads/{id}/retry` | Retry failed download |
| DELETE | `/api/spotify/downloads/completed` | Clear completed downloads |
| GET | `/api/spotify/downloads/events` | SSE stream for download progress |

### Settings

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/settings/{key}` | Get setting value |
| POST | `/api/settings/{key}` | Set setting value |

## Spotify Downloads

ViiB MediaHub can download tracks directly from Spotify (Premium account required).

### Setup

1. Create a Spotify Developer App at [developer.spotify.com](https://developer.spotify.com/dashboard)
2. Set the Redirect URI to `http://localhost:3000/callback` (dev) or your app URL
3. Enter your Client ID and Client Secret in Settings
4. Navigate to the Spotify page and log in

### Usage

1. **Search** - Search for tracks, albums, artists, or playlists
2. **Browse** - View your saved albums, playlists, or recently played
3. **Download** - Click the download button on any item
4. **Monitor** - Track progress on the Downloads page
5. **Play** - Downloads auto-scan into your library

### Technical Details

- Uses `librespot-go` for direct Spotify streaming
- OAuth tokens shared between Web API and downloads
- Configurable concurrent downloads (1-10 workers)
- Files saved as OGG Vorbis format with full metadata
- Organized by: `{Artist}/{Album}/{TrackNum}-{Artist}-{Title}.ogg`
- Real-time progress via Server-Sent Events (SSE)
- Automatic library rescan after downloads complete

## Technology Stack

### Frontend
- **React 19** - UI framework with hooks
- **TypeScript** - Type-safe JavaScript
- **Vite** - Fast build tooling
- **Zustand** - Lightweight state management
- **Tailwind CSS** (via CDN) - Utility-first styling
- **react-router-dom** - Client-side routing
- **react-virtuoso** - Virtualized lists for large libraries
- **Lucide Icons** - Beautiful icon set
- **Web Audio API** - EQ, visualizer, crossfade
- **IndexedDB** (via idb) - Browser fallback storage

### Backend
- **Go 1.22+** - Efficient, compiled backend
- **Chi Router** - Lightweight HTTP router
- **SQLite** (via go-sqlite3) - Embedded database with WAL mode
- **taglib-go** (go.senan.xyz/taglib) - Primary metadata extraction
- **dhowden/tag** - Fallback metadata extraction
- **librespot-go** - Spotify streaming client
- **getlantern/systray** - System tray integration

## Data Storage

All data is stored in `%APPDATA%/ViiB-MediaHub/` by default:

```
ViiB-MediaHub/
├── library.db          # SQLite database
├── covers/             # Cached album artwork
├── spotify_downloads/  # Downloaded Spotify tracks
├── viib.log           # Application log
└── crash.log          # Crash recovery log
```

## Browser Support

Works best in Chromium-based browsers (Chrome, Edge, Brave) for full Web Audio API support. Firefox and Safari may have limited visualizer or EQ functionality.

## License

MIT

## Contributing

Contributions welcome! Please open an issue or PR.

---

Built with ❤️ for music lovers who want control over their library.
