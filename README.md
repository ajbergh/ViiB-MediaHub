<p align="center">
  <img src="assets/banner.svg" alt="ViiB MediaHub" width="100%"/>
</p>

<p align="center">
  <strong>A modern, local-first music hub — React 19 frontend · Go backend · Native desktop via Wails</strong>
</p>

<p align="center">
  <a href="#quick-start">Quick Start</a> ·
  <a href="#features">Features</a> ·
  <a href="docs/index.md">Documentation</a> ·
  <a href="#architecture">Architecture</a> ·
  <a href="#api">API</a> ·
  <a href="#validation">Validation</a>
</p>

<p align="center">
  <img src="assets/screenshots/home.png" alt="ViiB MediaHub Home" width="75%"/>
</p>

---

## Overview

ViiB MediaHub is a self-hosted, local-first music player for Windows, macOS, and Linux. It combines a React interface with a Go backend, SQLite catalog, native Wails desktop shell, local filesystem scanning, Plex Media Server music-library support, Spotify integration, and AI-assisted library intelligence.

ViiB's normal library experience is source-transparent: local files and synchronized Plex music are normalized into the same ViiB song catalog, so Songs, Albums, Artists, Search, Queue, playlists, likes, history, Smart Mixes, AI DJ, and statistics do not need separate Plex-specific screens.

Spotify remains a distinct integration for Spotify browsing, streaming, and downloads.

| Mode | Description | Output |
|---|---|---|
| **Wails desktop** | Native desktop window backed by the local Go service | `.exe`, `.app`, native binary |
| **Web/browser** | Local HTTP server opened in the default browser | Single executable |

Both modes share the same Go backend and React frontend.

> **Plex scope:** ViiB supports Plex **music/audio libraries only**. Movies, TV, photos, music videos, general Plex video playback, and video transcoding are intentionally unsupported.

---

## Platforms

| Platform | Architecture | Wails desktop | Web/browser |
|---|---|:---:|:---:|
| Windows | x86-64 (amd64) | ✅ | ✅ |
| Windows | ARM64 | ✅ | ✅ |
| macOS | Apple Silicon (arm64) | ✅ | ✅ |
| macOS | Intel (amd64) | ✅ | ✅ |
| macOS | Universal | ✅ | — |
| Linux | x86-64 (amd64) | ✅ | ✅ |
| Linux | ARM64 | ✅ | ✅ |

---

## Features

### Audio playback

- MP3, FLAC, M4A, AAC, OGG, OPUS, WAV, WMA, and other formats supported by the active playback stack
- Gapless playback and configurable crossfade
- Queue management with reorder, Play Next, and Add to Queue
- HTTP Range-aware backend media delivery and seeking
- Persistent Now Playing, queue, equalizer, sleep timer, and media-session integration

### Unified music library

- **Local folders** — add and scan multiple filesystem music roots
- **Plex Media Server music** — discover or manually configure PMS, authenticate, select a music library, synchronize metadata, and play through the normal ViiB player
- One canonical ViiB `songs` catalog for local and Plex tracks
- Source-transparent Albums, Artists, Search, playlists, likes, history, Smart Mixes, AI DJ, and Stats
- Revisioned library synchronization and indexed backend search for large catalogs
- Library Operations for diagnostics, repair, backup, staged offline restore, and continuous local-folder monitoring

### Plex Media Server

- Backend GDM discovery using Plex multicast `239.0.0.250:32414`
- Per-interface IPv4 discovery for multi-NIC/VPN/link-local environments
- Manual hostname/IP/HTTP/HTTPS configuration when GDM is unavailable
- PMS identity validation before configuration is accepted
- Plex JWT/PIN device authentication with a locally generated ED25519 device key
- Music/audio-library filtering; video-oriented Plex libraries are never offered as ViiB sources
- Transactional metadata synchronization with add/update/remove reconciliation
- Offline-safe catalog retention: temporary PMS outages never become mass deletions
- Backend-authenticated audio and artwork proxying; Plex tokens are not placed in browser-visible media URLs
- Range forwarding and seeking through the existing `/api/audio/{songId}` contract
- Read-only source behavior: ViiB never deletes, moves, renames, or reconfigures Plex media

See [Plex Media Server Music Support](docs/plex-music.md).

### Spotify

- OAuth authentication
- Saved albums, playlists, recently played, and Spotify search/browse experiences
- Direct Spotify streaming with configurable quality
- Track/album/playlist downloads with metadata and progress events
- Automatic local-library rescan after completed downloads

See [Spotify Integration](docs/spotify.md).

### AI DJ and library intelligence

- Natural-language playlist generation with semantic track, album, and artist recall when the optional local SQLite index is ready
- Semantic AI DJ phase retrieval with local source/filter enforcement, behaviour-aware ranking, diversity, BPM flow, and deterministic metadata fallback
- Separate semantic embedding configuration: local Ollama or explicitly cost-confirmed OpenAI embeddings; chat-provider settings and listening history are never repurposed as embedding content
- Smart Mixes based on catalog metadata and listening history
- Multi-provider LLM support including Gemini, OpenAI, Anthropic, OpenRouter, Ollama, and X.AI where configured
- Genre, mood, energy, tempo, BPM, and year enrichment
- AI DJ set generation using the same ViiB catalog that contains local and synchronized Plex tracks

### DJ Mode

- Two-deck mixing interface
- BPM and key analysis
- Cue points and sampler
- Pitch/tempo controls
- MIDI controller support through Web MIDI where available

### Audio enhancement

- 10-band equalizer
- Built-in EQ presets and Auto-EQ
- Multiple real-time visualizers
- Web Audio based visualization and processing

### Stats, likes, and history

- Listening-time and play-count statistics
- Top artists, albums, and genres
- Recently played and persistent play history
- Liked songs and albums stored in ViiB, independent of Plex or Spotify favorites

---

## Quick Start

### Prerequisites

| Requirement | Minimum | Notes |
|---|---|---|
| Node.js | 20 LTS | Frontend tooling |
| Go | **1.25.13+** | Current security-patched project baseline |
| GCC / Clang | — | Required for CGO/SQLite |
| Wails CLI | v2 | Desktop builds |

Install Wails:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.11.0
```

Platform-native build dependencies are documented by Wails. On Windows, the repository CI uses MSYS2/MinGW for CGO.

### Development

Windows/Wails:

```powershell
.\scripts\dev-wails.ps1
```

Windows/browser mode:

```powershell
.\scripts\dev.ps1
```

macOS/Linux manual development:

```bash
# Terminal 1
cd backend
go run ./cmd/viib -port 8080 -no-browser

# Terminal 2
npm run dev
```

### Normal validation

```bash
npm run check
cd backend
go test ./...
go test -race ./...
go vet ./...
staticcheck ./...
```

Pull-request CI also builds browser/Wails targets, runs binary `govulncheck`, packages the Windows Wails executable, and scans the packaged Windows binary.

---

## Configuring music sources

### Local folders

Use **Settings** to add local music folders. ViiB scans configured roots and incrementally reconciles filesystem changes.

### Plex Media Server

Open **Settings → Library Health / Library Operations** and use the Plex Music Source panel:

1. Search the local network or enter a PMS address manually.
2. Validate/connect the server.
3. Sign in to Plex when the server requires authentication.
4. Select one of the returned music/audio libraries.
5. Synchronize the selected library.

Plex tracks then appear in the normal ViiB library UI. Use **Resynchronize** to refresh the remote catalog. A temporary PMS outage retains already imported catalog metadata and surfaces source availability/authentication state instead of deleting tracks.

See [docs/plex-music.md](docs/plex-music.md) for security, discovery, troubleshooting, and playback details.

---

## Architecture

```text
React / TypeScript UI
        │
        │ REST + SSE + normal media URLs
        ▼
Go backend (chi)
        │
        ├── SQLite canonical catalog
        │     ├── local filesystem songs
        │     └── synchronized Plex songs
        │
        ├── local scanner / metadata extraction
        ├── Plex client / discovery / auth / sync / media proxy
        ├── Spotify session / proxy / downloader
        ├── AI / enrichment services
        └── Library Operations / jobs / recovery
```

Important backend packages:

```text
backend/internal/
├── api/          REST/SSE handlers and source-aware media routes
├── audio/        local audio metadata extraction
├── crypto/       sensitive-setting encryption
├── db/           SQLite catalog, Plex source records, playlists, history
├── llm/          AI providers and enrichment
├── plex/         PMS client, GDM discovery, auth, metadata mapping
├── scanner/      local filesystem scan/reconciliation
├── server/       HTTP router/middleware
├── spotify/      Spotify session, streaming, and downloads
└── validation/   input and sensitive-key classification
```

The Plex integration deliberately extends the existing catalog/playback abstractions rather than creating a second Plex-specific media application.

---

## Data and security

Application data is stored under the OS user configuration directory in `ViiB-MediaHub/`, including the SQLite database, artwork cache, logs, and downloaded Spotify media.

Sensitive settings are encrypted before persistence using ViiB's machine-bound AES-256-GCM settings mechanism. This includes Plex account/server tokens and Plex device private-key material.

Plex security properties:

- credentials are not returned through Plex configuration APIs;
- credentials are not stored in frontend localStorage/Zustand state;
- browser media URLs contain ViiB song IDs, not Plex tokens;
- PMS authentication is attached server-side;
- credential-like error material is redacted;
- cross-origin Plex asset resolution does not forward the PMS token.

Backups contain the ViiB database and should be protected like the application data directory because encrypted integration credentials may be present in the database.

---

## API

ViiB retains legacy `/api` routes and provides an additive versioned `/api/v2` surface for scalable library synchronization, indexed search, durable jobs, Library Operations, and Plex source management.

Key source-transparent media routes:

| Method | Endpoint | Purpose |
|---|---|---|
| `GET` | `/api/audio/{songId}` | Play local or Plex catalog audio; Plex requests are authenticated/proxied by the backend |
| `GET` | `/api/cover/{songId}` | Return local or proxied Plex artwork |

Plex management routes live under `/api/v2/plex`:

| Method | Endpoint | Purpose |
|---|---|---|
| `POST` | `/discover` | Run bounded LAN GDM discovery |
| `POST` | `/connect` | Normalize, validate, and connect a PMS address |
| `GET` | `/config` | Return sanitized Plex source/configuration state |
| `DELETE` | `/config` | Remove the ViiB Plex source/cache without modifying PMS |
| `POST` | `/auth/start` | Start Plex device authentication |
| `GET` | `/auth/status` | Poll authentication state |
| `GET` | `/libraries` | List selectable music/audio libraries only |
| `PUT` | `/library` | Select a Plex music library |
| `POST` | `/sync` | Start Plex library synchronization |
| `GET` | `/sync/status` | Read Plex synchronization/source status |

The complete versioned contract is [docs/openapi-v2.yaml](docs/openapi-v2.yaml).

---

## Build targets

Repository build scripts cover Windows, macOS, and Linux browser/Wails targets. See the scripts under `scripts/` and the CI workflows under `.github/workflows/` for the exact supported build commands and toolchain used by automation.

Every pull request and `main` push packages native Wails desktop builds for Linux x64,
macOS Apple Silicon, macOS Intel, and Windows x64. Successful CI runs expose the Linux
binary as a `.tar.gz` artifact and the unsigned macOS `.app` bundles as `.zip` artifacts;
the macOS CI bundles are intended for validation and require release signing/notarization
before distribution.

The Wails v2 desktop shell keeps the external system tray on Windows and Linux. On
macOS it uses the standard application lifecycle (closing the window exits) because
the legacy tray library and Wails both install a native macOS application delegate.

Typical Windows desktop build:

```powershell
.\scripts\build-wails.ps1
```

Typical web/server build:

```powershell
.\scripts\build.ps1
```

---

## Documentation

Start at [docs/index.md](docs/index.md).

Important references:

- [Plex Music](docs/plex-music.md)
- [Library Operations](docs/library-operations.md)
- [Settings](docs/settings.md)
- [Player](docs/player.md)
- [Search](docs/search.md)
- [Spotify](docs/spotify.md)
- [OpenAPI v2](docs/openapi-v2.yaml)

---

## Validation

The current CI baseline validates:

- frontend checks and production dependency audit;
- Go tests and race tests;
- `go vet` and Staticcheck;
- Linux browser and Wails builds;
- binary vulnerability scanning;
- Windows frontend production build;
- Windows Wails `.exe` packaging;
- vulnerability scanning of the packaged Windows executable.

The Plex implementation is covered with fake/`httptest` PMS behavior and does not require a live Plex server in CI. Real-PMS smoke testing remains appropriate before a release candidate, especially for LAN discovery, authentication, large-library sync, codec coverage, and long-running direct playback.

---

## License

MIT

## Contributing

Contributions are welcome. Please open an issue or pull request and keep changes aligned with the existing source-transparent catalog and playback architecture.
