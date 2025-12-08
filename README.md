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

### 🎨 User Experience
- **Keyboard Navigation** - Full keyboard control for power users:
  - Space: Play/Pause
  - Arrow Up/Down: Volume control (±5%)
  - Shift+Arrow Left/Right: Previous/Next track
  - M: Toggle mute
  - Q: Toggle queue panel
  - E: Toggle equalizer
  - N: Toggle now playing view
  - Escape: Close panels and dialogs
- **Toast Notifications** - Non-intrusive feedback for actions and errors
- **Loading Skeletons** - Smooth shimmer animations while content loads
- **Empty States** - Helpful guidance when no content is available
- **Responsive Design** - Collapsible sidebar and adaptive player controls
- **Page Transitions** - Smooth fade-in animations between pages
- **Drag & Drop** - Reorder queue items by dragging
- **Accessibility** - ARIA labels, focus indicators, screen reader support

### ⏱️ Sleep Timer (New)
- Set a timer to stop playback after a preset or custom time (15-120 minutes), after X songs, or at the end of the current song.
- Optionally fades volume in the last 30 seconds before pausing to make for a smooth fall-asleep experience.
- Accessible via the moon icon on the player controls.

### ⏮️ Play Next vs Add to Queue (New)
- Right-click context menus now support "Play Next" which inserts selected songs immediately after the current track, as well as "Add to Queue" which appends to the end.

### 🕒 Recently Played (New)
- The Home page shows a Recently Played section with the last 10 tracks and human-friendly relative timestamps (e.g., "2h ago").
- Useful for quickly returning to what you were listening to.

### 📊 Listening Stats (New)
- New `Stats` dashboard (accessible from the sidebar) provides listening insights:
  - Total listening time, total plays
  - Top artists, albums, genres
  - Most played song highlight
  - Library overview (total songs, albums, artists)
  - Weekly/monthly play counts

### 📚 Library Management
- **Folder Scanning** - Add multiple music folders with incremental scanning
- **Ultra-Fast Startup** - Near-instant library loading using filesystem journals and directory signatures
- **Smart Mixes** - Auto-generated playlists based on listening patterns:
  - Heavy Rotation (most played recently)
  - Rediscover Favorites (old favorites you haven't heard in a while)
  - Fresh Finds (recently added)
  - Genre-based mixes (Chill Acoustic, 90s Alternative, etc.)
- **Playlists** - Create, edit, and manage custom playlists
  - Playlists are persisted to the backend when available. When running in browser-only mode, playlists are stored in IndexedDB.
- **Background Enrichment** - Automatic Spotify metadata enhancement for albums/artists

### ⚡ Ultra-Fast Startup Scan System (New)
- **Instant Startup** - Sub-second library loading even with 100,000+ songs
- **Platform-Optimized Detection**:
  - Windows: USN (Update Sequence Number) journal for kernel-level change tracking
  - macOS: Optimized mtime scanning with directory-level skipping
  - Linux: mtime + ctime detection with signature integration
- **Directory Signatures** - Content-based hashing allows skipping unchanged folders entirely
- **Background Processing** - Priority-based worker pool for deferred operations
- **Integrity Verification** - Periodic sampling detects missing, corrupted, or modified files
- **Adaptive Scheduling** - Automatically adjusts scan frequency based on activity

See `FAST_SCAN_DESIGN.md` for complete architecture documentation.

### 🎧 Spotify Integration
**Streaming & Direct Downloads** - Stream and download Spotify tracks (Premium required)
  - **Direct Streaming**: Stream tracks directly from Spotify with configurable quality (High/Medium/Low) and HTTP Range support for seeking.
  - **Direct Downloads**: Download tracks, albums, and playlists to OGG Vorbis for offline playback.
  - Configurable concurrent downloads (1-10)
  - Real-time progress via Server-Sent Events
  - Organized file structure for downloads: `{Artist}/{Album}/{Track}.ogg`
  - Auto-rescan after downloads complete

### 💾 Data & Settings
- **SQLite Database** - Persistent storage for library, playlists, and settings
- **System Tray** - Minimize to tray on Windows
- **Configurable Data Directory** - Choose where your data is stored

Persistence notes:
- Play counts and timestamps (used for the Stats dashboard) are saved to the backend database when running with the backend server. If running in browser-only mode (no backend), these values are persisted in IndexedDB.
- Playlists and library changes are synced to the backend when available; otherwise they are stored locally via IndexedDB.

## Architecture

```
ViiB-MediaHub/
├── components/         # React UI components
│   ├── context-menus/  # Right-click menu components
│   ├── now-playing/    # Full-screen player components
│   ├── Skeleton.tsx    # Loading skeleton components
│   ├── Toast.tsx       # Toast notification system
│   └── EmptyState.tsx  # Empty state displays
├── pages/              # Route page components
├── services/           # API client, backend service, Spotify service
├── slices/             # Zustand state slices (player, library, spotify, ui)
├── hooks/              # Custom React hooks
│   ├── useAudioPlayer.ts       # Audio playback management
│   ├── useBackgroundEnrichment.ts  # Metadata enrichment
│   └── useKeyboardNavigation.ts    # Global keyboard shortcuts
├── lib/                # Audio engine, smart mix generator, parsers
├── workers/            # Web workers for background tasks
│
└── backend/            # Go HTTP server
    ├── cmd/viib/       # Entry point with system tray
    └── internal/
        ├── api/        # REST API + SSE handlers
        ├── audio/      # Metadata extraction (taglib + dhowden/tag)
        ├── db/         # SQLite with WAL mode
        ├── gemini/     # AI-powered music analysis
        ├── logger/     # Shared logging facility
        ├── scanner/    # Library scanning with fast startup
        │   ├── scanner.go       # Core scanning logic
        │   ├── fast_scan.go     # Incremental scan with signatures
        │   ├── background.go    # Priority-based worker pool
        │   ├── optimization.go  # Performance tuning utilities
        │   ├── journal_*.go     # Platform-specific change detection
        │   └── journal.go       # Change detector interface
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

### Quick Tips: Using New Features
- Sleep Timer: Open the player and click the moon icon. Set a preset or custom duration, or set the timer to "End of current song" or "After X songs". The volume fades over the last 30 seconds before pausing.
- Play Next vs Add to Queue: Right-click (context menu) on a song/album/playlist or mix to select "Play Next" (insert after current song) or "Add to Queue" (append to end).
- Recently Played: Access the Home page to view your last 10 played tracks, with a quick click to resume playback.
- Stats Dashboard: Click "Stats" in the sidebar to view your listening history and insights.

### Verification: Persistence Checks
If you're running with the backend server (default dev mode), the following can be used to validate persistence:

1) Verify play count updates
  - Play a song until it ends. The frontend will record the play count to the backend.
  - Verify via API: `curl http://127.0.0.1:8080/api/songs` and inspect the `playCount` and `lastPlayed` fields for the track.

2) Verify playlist persistence
  - Create a playlist via the UI or using the `Playlists` page.
  - Verify via API: `curl http://127.0.0.1:8080/api/playlists` and ensure the new playlist is present.

If you're running in browser-only mode (backend not available), the application persists to IndexedDB. Use developer tools -> Application -> IndexedDB -> `mediahub-db` to inspect `songs` and `playlists` stores.

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
| GET | `/api/spotify/stream/{id}` | Stream audio for Spotify track (GET parameter: `?quality=high|medium|low`) |
| GET | `/api/spotify/downloads/events` | SSE stream for download progress |

### Settings

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/settings/{key}` | Get setting value |
| POST | `/api/settings/{key}` | Set setting value |

## Spotify Streaming & Downloads

ViiB MediaHub can stream and download tracks directly from Spotify (Premium account required).

### Setup

1. Create a Spotify Developer App at [developer.spotify.com](https://developer.spotify.com/dashboard)
2. Set the Redirect URI to `http://localhost:3000/callback` (dev) or your app URL
3. Enter your Client ID and Client Secret in Settings
4. Navigate to the Spotify page and log in

### Usage

1. **Search** - Search for tracks, albums, artists, or playlists
2. **Browse** - View your saved albums, playlists, or recently played
3. **Download / Play** - Click the download button on any item to save it, or click the Play button to stream directly from Spotify
4. **Monitor** - Track progress on the Downloads page
5. **Play** - Downloads auto-scan into your library; streaming does not require a download

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

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| Space | Play/Pause current track |
| Arrow Up | Increase volume 5% |
| Arrow Down | Decrease volume 5% |
| Shift + Left | Previous track |
| Shift + Right | Next track |
| M | Toggle mute |
| Q | Toggle queue panel |
| E | Toggle equalizer panel |
| N | Toggle now playing view |
| Escape | Close active panel or dialog |

## Browser Support

Works best in Chromium-based browsers (Chrome, Edge, Brave) for full Web Audio API support. Firefox and Safari may have limited visualizer or EQ functionality.

## License

MIT

## Contributing

Contributions welcome! Please open an issue or PR.

---

Built with ❤️ for music lovers who want control over their library.
