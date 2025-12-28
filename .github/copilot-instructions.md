# ViiB MediaHub AI Instructions

## Project Overview
ViiB MediaHub is a local media player application with a React frontend and Go backend. It compiles to a single native desktop executable using Wails v2 with WebView2, providing a native Windows experience with system tray integration.

## Architecture

### Frontend (React/TypeScript)
- **Framework**: React 19, TypeScript, Vite 6
- **State Management**: Zustand with persistent slices (`store.ts`, `slices/`)
  - `playerSlice.ts`: Playback, queue, audio settings, streaming state
  - `librarySlice.ts`: Songs, playlists, metadata, scanning, likes
  - `spotifySlice.ts`: OAuth tokens, user profile, streaming settings
  - `uiSlice.ts`: Context menus, dialogs, logs, toasts
- **Data Persistence**: 
  - Backend uses SQLite for all persistent data
  - IndexedDB (via `idb`) as fallback for browser-only mode
  - `localStorage` (via Zustand persist) for UI settings
- **Styling**: Tailwind CSS via CDN with custom config (colors: `surface-0`/`surface-1`/`surface-2`, `brand`, etc.)
- **Routing**: `react-router-dom` v7 with `BrowserRouter`
- **Virtual Scrolling**: `react-virtuoso` for large lists

### Backend (Go)
- **Runtime**: Go 1.25+ (requires CGO for SQLite)
- **Desktop Framework**: Wails v2 with WebView2 (embedded frontend)
- **Router**: `chi/v5` for HTTP API
- **Database**: SQLite (`mattn/go-sqlite3`) with WAL mode
- **Entry Points**:
  - `backend/cmd/wails/main.go`: Native desktop app (production)
  - `backend/cmd/viib/main.go`: Web-embedded build (legacy)

### Key Backend Packages
- `internal/api/`: HTTP handlers, SSE events, download manager
- `internal/audio/`: Audio file processing
- `internal/db/`: SQLite database schema and methods
- `internal/scanner/`: File system scanning with fast incremental updates
- `internal/spotify/`: Spotify API integration
- `internal/gemini/`: Google Gemini AI integration for mood analysis
- `internal/crypto/`: AES-256-GCM encryption for sensitive settings
- `internal/logger/`: File-based logging (`%APPDATA%/ViiB-MediaHub/viib.log`)

## Development Workflow

### Start Development Server
Run `scripts/dev-wails.ps1` in PowerShell:
- Starts Go backend on dynamic port (stored in registry)
- Starts Vite frontend on port 5173 with HMR
- Frontend proxies `/api` requests to backend

### Build Production
Run `scripts/build-wails.ps1`:
- Builds optimized frontend with Vite
- Embeds frontend into Go binary
- Produces single executable with system tray support

### Environment Requirements
- Go requires `CGO_ENABLED=1` (for SQLite/TagLib)
- Windows: MSYS2 with GCC for CGO
- Wails CLI installed globally

## Code Conventions

### Frontend

#### API Communication
- **Primary**: Use `services/backendService.ts` for backend interactions
- **Secondary**: Use `services/api.ts` for typed API calls
- Types in services should match backend structs exactly

#### Styling
- Use Tailwind utility classes
- Custom colors defined in `index.html`: `surface-0`, `surface-1`, `surface-2`, `brand`
- Dark theme by default with `bg-surface-0`, `text-neutral-100`, etc.

#### State Management
- Use `useStore` hook from `store.ts` for global state
- Song library is managed in `librarySlice` (synced from SQLite backend)
- Player state in `playerSlice` with queue management
- UI state in `uiSlice` for menus, dialogs, toasts

#### Components
- Functional components with TypeScript interfaces for props
- Use React Error Boundaries for graceful failure handling
- Toast notifications via `showToast({ type, message })` from store
- Loading states via skeleton components from `components/Skeleton.tsx`
- Empty states via `components/EmptyState.tsx`

#### Real-time Updates
- SSE connection in `components/LibraryEventListener.tsx`
- Events: `scan_started`, `scan_progress`, `scan_complete`, `library_updated`, `enrichment_progress`
- Automatic reconnection with fallback polling

### Backend

#### Project Structure
```
backend/
├── cmd/
│   ├── wails/          # Wails desktop app entry point
│   └── viib/           # Web-embedded build entry point
├── internal/
│   ├── api/            # HTTP handlers and SSE
│   ├── audio/          # Audio file processing
│   ├── crypto/         # Encryption utilities
│   ├── db/             # SQLite database layer
│   ├── gemini/         # Google Gemini AI integration
│   ├── logger/         # File logging
│   ├── scanner/        # File system scanning
│   ├── server/         # HTTP server setup
│   ├── spotify/        # Spotify API client
│   └── validation/     # Input validation
```

#### Database
- Use `db` package for all database operations
- Schema includes: songs, playlists, plays, scan_folders, settings, spotify_downloads, album_metadata, artist_metadata
- Sensitive settings encrypted at rest (AES-256-GCM)
- WAL mode for concurrent access

#### API Handlers
- Routes defined in `internal/api/api.go`
- Use `respondJSON(w, data)` and `respondError(w, code, msg)` helpers
- SSE via `libraryEventsSSE` handler

#### Scanner System
- Fast incremental scanning using directory signatures
- Platform-specific filesystem journals (USN on Windows, FSEvents on macOS)
- Background workers for metadata extraction
- File metadata caching for change detection

## Key Files

### Frontend
| File | Purpose |
|------|---------|
| `App.tsx` | Main entry, routing, error boundary, background enrichment |
| `store.ts` | Zustand store combining all slices |
| `slices/types.ts` | TypeScript interfaces for all state slices |
| `services/backendService.ts` | Backend API wrapper |
| `services/api.ts` | Typed API client |
| `components/Layout.tsx` | App shell with sidebar and player |
| `components/Player.tsx` | Audio playback controls |
| `components/LibraryEventListener.tsx` | SSE for real-time updates |
| `hooks/useAudioPlayer.ts` | Audio element management |
| `hooks/useKeyboardNavigation.ts` | Global keyboard shortcuts |
| `hooks/useBackgroundEnrichment.ts` | Album metadata fetching |
| `lib/smartMix.ts` | Smart playlist generation |

### Backend
| File | Purpose |
|------|---------|
| `cmd/wails/main.go` | Wails app entry, system tray, graceful shutdown |
| `internal/api/api.go` | HTTP handlers, SSE, scan management |
| `internal/api/download_manager.go` | Spotify download queue |
| `internal/db/db.go` | SQLite schema, queries, migrations |
| `internal/scanner/scanner.go` | Core scanning logic |
| `internal/scanner/fast_scan.go` | Incremental scanning with signatures |
| `internal/spotify/spotify.go` | Spotify API client |

## Features

### Library Management
- Scan configured folders for music files
- Fast incremental scanning (detects changes without full rescan)
- Automatic deleted file detection
- Metadata extraction (title, artist, album, cover art)
- Play history tracking

### Playback
- Native audio playback via HTML5 Audio
- Queue management (add, reorder, play next)
- Keyboard shortcuts (Space, arrows, Q, E)
- Equalizer with presets
- Crossfade and gapless playback
- Visualizer modes

### Spotify Integration
- OAuth authentication
- Browse saved albums and playlists
- Download tracks (requires librespot-go)
- Streaming support (optional)
- Metadata enrichment from Spotify API

### Smart Features
- Smart Mixes (auto-generated playlists by genre/mood)
- Liked songs and albums
- AI-powered mood analysis (via Gemini)
- Background metadata enrichment

### UI Components
| Component | Purpose |
|-----------|---------|
| `Skeleton.tsx` | Loading skeletons (SkeletonAlbumCard, SkeletonTrackRow) |
| `Toast.tsx` | Toast notifications (ToastContainer) |
| `EmptyState.tsx` | Empty state displays (EmptyLibrary, EmptyPlaylists) |
| `ConfirmDialog.tsx` | Confirmation modals |
| `ContextMenu.tsx` | Right-click menus |
| `context-menus/` | Type-specific context menus |

## Data Flow

### Startup Sequence
1. Wails app starts, creates native window
2. HTTP server starts on dynamic port
3. Frontend loads, calls `initLibrary()`
4. Backend begins startup scan (3-second delay)
5. SSE connection established for real-time updates
6. Scan progress shown in sidebar
7. Library loaded from SQLite

### Scanning Flow
1. User adds folder or app starts
2. Backend checks directory signatures for changes
3. Changed files processed by background workers
4. SSE events emitted: `scan_started` → `scan_progress` → `scan_complete`
5. Frontend polls status as fallback if SSE missed
6. Library refreshed from backend

### Playback Flow
1. User clicks play on song
2. `playerSlice.playSong()` called
3. Queue updated with context (album/playlist)
4. Audio element loads file via `/api/stream/{id}`
5. Media Session API updated for OS integration
6. Play recorded in database

## Debugging

### Logs
- Backend logs: `%APPDATA%/ViiB-MediaHub/viib.log`
- Frontend console: DevTools → Console
- SSE events logged with 📥/📡 emojis

### Common Issues
- **CGO errors**: Ensure GCC installed and `CGO_ENABLED=1`
- **Port conflicts**: Backend uses dynamic port allocation
- **SSE disconnects**: Frontend has reconnection + polling fallback
- **Scan stuck**: Check `viib.log` for scanner errors
