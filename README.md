# ViiB MediaHub

A modern local media player for audio files, built with React + TypeScript frontend and Go backend. Compiles to a single Windows executable that opens in your web browser.

## Features

- 🎵 **Local Audio Playback** - Play MP3, FLAC, M4A, AAC, OGG, OPUS, WAV, and WMA files
- 🎨 **Album Art & Metadata** - Automatic extraction from audio file tags
- 🎚️ **10-Band Equalizer** - Fine-tune your audio experience
- 📊 **Audio Visualizer** - Real-time spectrum analyzer
- 🔀 **Crossfade** - Smooth transitions between tracks
- 🧠 **Smart Mixes** - Auto-generated playlists based on your library
- 📁 **Folder Scanning** - Add music folders and scan for new files
- 🎧 **Spotify Integration** - Connect to enhance metadata (optional)
- 💾 **Local Database** - SQLite storage for your library

## Architecture

```
ViiB-MediaHub/
├── frontend/           # React + TypeScript + Vite
│   ├── components/     # UI components
│   ├── pages/          # Route pages
│   ├── services/       # API client, library service
│   ├── slices/         # Zustand state slices
│   ├── hooks/          # Custom React hooks
│   └── lib/            # Audio engine, utilities
│
└── backend/            # Go HTTP server
    ├── cmd/viib/       # Main entry point
    └── internal/
        ├── api/        # REST API handlers
        ├── audio/      # Metadata extraction
        ├── db/         # SQLite database
        └── server/     # HTTP server setup
```

## Quick Start

### Prerequisites

- **Node.js** 18+ and npm
- **Go** 1.22+
- **GCC** (for SQLite CGO) - On Windows, install via [MSYS2](https://www.msys2.org/) or [TDM-GCC](https://jmeubank.github.io/tdm-gcc/)

### Development Mode

Run frontend and backend separately for hot-reload:

```powershell
# Terminal 1: Start Go backend
cd backend
go run ./cmd/viib -port 8080 -no-browser

# Terminal 2: Start Vite dev server
npm run dev
```

Or use the dev script:

```powershell
.\scripts\dev.ps1
```

The frontend dev server proxies `/api` requests to the backend.

### Build for Production

Build a single Windows executable:

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

#### Command Line Options

```
-port <n>        Port to run on (0 = auto-select)
-no-browser      Don't open browser automatically
-data <path>     Custom data directory
```

## API Endpoints

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
| GET | `/api/audio/{id}` | Stream audio file |
| GET | `/api/cover/{id}` | Get album cover |
| POST | `/api/browse` | Browse folders |

## Technology Stack

### Frontend
- React 19
- TypeScript
- Vite
- Zustand (state management)
- Tailwind CSS
- Lucide Icons
- Web Audio API

### Backend
- Go 1.22
- Chi Router
- SQLite (via go-sqlite3)
- dhowden/tag (metadata extraction)

## Browser Support

Works best in Chromium-based browsers (Chrome, Edge, Brave) for full Web Audio API support.

## License

MIT

## Contributing

Contributions welcome! Please open an issue or PR.
