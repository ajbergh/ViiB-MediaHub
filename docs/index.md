# ViiB MediaHub — Documentation

ViiB MediaHub is a local music player for Windows, macOS, and Linux. It combines a React frontend with a Go backend into a single native desktop executable powered by Wails v2 (WebView2 on Windows, WebKit on macOS and Linux).

---

## Contents

| Page | Description |
|---|---|
| [Home](home.md) | Dashboard with smart mixes, recently played, and stats |
| [Songs](songs.md) | Full song library with sort, filter, and playback |
| [Albums](albums.md) | Album grid with detail view |
| [Artists](artists.md) | Artist browser with discography |
| [Genres](genres.md) | Genre browser |
| [Playlists](playlists.md) | User-created playlists |
| [Liked Songs & Albums](liked.md) | Your liked tracks and albums |
| [Smart Playlists / AI DJ](smart-playlists.md) | AI-generated playlists and DJ sets |
| [Search](search.md) | Global library search |
| [Spotify](spotify.md) | Spotify integration, browse, and download |
| [DJ Mode](dj-mode.md) | Full-featured two-deck DJ interface |
| [Downloads](downloads.md) | Spotify download queue and status |
| [Stats](stats.md) | Listening history, top artists, heatmap |
| [Settings](settings.md) | Library, audio, Spotify, AI, and system settings |
| [Library Operations](library-operations.md) | Diagnostics, repair, backup, staged restore, and continuous monitoring |
| [Player](player.md) | Playback controls, queue, EQ, visualizer, and Now Playing |
| [Keyboard Shortcuts](keyboard-shortcuts.md) | Full keyboard shortcut reference |

---

## Quick Overview

```
┌──────────────────────────────────────────────────────────┐
│  Sidebar (navigation)           Main content area        │
│  ─────────────────────          ──────────────────────   │
│  Home                           Changes per route        │
│  Songs                                                   │
│  Albums                                                  │
│  Artists                                                 │
│  Genres                                                  │
│  Playlists                                               │
│  Liked Songs / Albums                                    │
│  Smart Playlists                                         │
│  Search                                                  │
│  Spotify                                                 │
│  DJ Mode                                                 │
│  Downloads                                               │
│  Stats                                                   │
│  Settings                                                │
│  Library Health shortcut                                 │
├──────────────────────────────────────────────────────────┤
│  Player bar (persistent, bottom)                         │
│  Album art · Track info · Controls · Volume · Queue · EQ │
└──────────────────────────────────────────────────────────┘
```

---

## Architecture Summary

- **Frontend**: React 19 + TypeScript + Vite, Zustand state, Tailwind CSS
- **Backend**: Go 1.25.12+, chi HTTP router, SQLite (WAL mode), Wails v2
- **Data**: SQLite primary store; IndexedDB fallback for browser-only mode; `localStorage` for UI settings
- **Real-time**: legacy SSE (`/api/events`) for scan progress plus revision and job SSE streams under `/api/v2`
- **Library scale**: cursor-based snapshots, revisioned deltas, indexed backend search, and bounded scanner/SQLite work
- **Recovery**: local diagnostics, validated SQLite backups, and staged offline restore through Library Operations
- **Audio**: HTML5 `<audio>` with Web Audio API for EQ and visualization
- **Platform**: Single native executable. Windows via WebView2; macOS via WKWebView; Linux via WebKitGTK

See the [development and build guide](../README.md#quick-start) for environment setup and platform build commands.
