# ViiB MediaHub — Documentation

ViiB MediaHub is a local-first music hub for Windows, macOS, and Linux. It combines a React frontend with a Go backend and ships as a Wails v2 desktop application or local web/server build.

ViiB can build one canonical music catalog from two first-class sources:

- **Local filesystem folders** scanned by ViiB.
- **Plex Media Server music/audio libraries** synchronized into the same catalog and streamed through the ViiB backend.

Spotify remains a separate integration for Spotify browsing, streaming, and downloads.

> Plex support is audio/music only. ViiB does not support Plex movie, TV, photo, music-video, general video playback, or video transcoding.

---

## User guides

| Page | Description |
|---|---|
| [Architecture](architecture.md) | Current media-source, catalog, playback, synchronization, and security model |
| [Home](home.md) | Dashboard with Smart Mixes, recently played, recently added, and library stats |
| [Songs](songs.md) | Unified local/Plex song catalog with sort, filter, and playback |
| [Albums](albums.md) | Source-transparent album grid and detail view |
| [Artists](artists.md) | Artist browser and discographies from the ViiB catalog |
| [Genres](genres.md) | Genre browser |
| [Playlists](playlists.md) | ViiB playlists that can contain local or Plex catalog tracks |
| [Liked Songs & Albums](liked.md) | ViiB-local likes for catalog tracks and albums |
| [AI Smart Mix](smart-playlists.md) | AI-generated playlists and DJ sets over the ViiB catalog |
| [Search](search.md) | Indexed search across the unified ViiB catalog |
| [Spotify](spotify.md) | Spotify integration, streaming, browse, and downloads |
| [Plex Music](plex-music.md) | PMS discovery, account/server selection, library sync, proxied playback, explicit metadata writeback, security, and troubleshooting |
| [DJ Mode](dj-mode.md) | Full-featured two-deck DJ interface |
| [Downloads](downloads.md) | Spotify download queue and status |
| [Duplicates](duplicates.md) | Duplicate-track review and ignore state |
| [Stats](stats.md) | Listening history and statistics across ViiB catalog playback |
| [Settings](settings.md) | Library sources, library operations, playback, integrations, AI, appearance, and system configuration |
| [Library Operations](library-operations.md) | Track analysis, metadata operations, diagnostics, repair, backup, and staged restore |
| [Player](player.md) | Source-transparent playback, queue, EQ, visualizer, and Now Playing |
| [Keyboard Shortcuts](keyboard-shortcuts.md) | Keyboard shortcut reference |

## Maintainer and reference documentation

Reviewed against the repository on 2026-10-04. Implementation status and dated validation evidence are distinct: an implemented adapter or displayed analysis result does not close a parity or professional-quality gate.

| Reference | Purpose and status |
|---|---|
| [OpenAPI v2](openapi-v2.yaml) | Machine-readable versioned backend API contract |
| [Stem package v1](VIIB_STEM_PACKAGE_V1.md) | Current `.viibstems` interchange and validation contract |
| [DJ professional analysis roadmap](DJV2_PROFESSIONAL_TRACK_ANALYSIS_ROADMAP.md) | Delivered analysis engineering and remaining accuracy/calibration/release gates |
| [DJ analysis context](DJV2_TRACK_ANALYSIS_CONCISE_CONTEXT.md) | Dated measurement handoff and next actions; the roadmap owns the full gate record |
| [DJ stems, cues, and mix planning roadmap](DJV2_PRO_STEMS_CUES_MIX_INTELLIGENCE_ROADMAP.md) | Implementation status and remaining package/cue/planning work |
| [BPM reference benchmark](REFERENCE_BPM_BENCHMARK.md) | Reproducible external comparison workflow and evidence |
| [Rhythm/structure reference benchmark](REFERENCE_RHYTHM_STRUCTURE_BENCHMARK.md) | Beat, downbeat, structure, and energy comparison workflow |
| [Spotify implementation plan](SPOTIFY_WEBPLAYER_AUTH_AUDIO_ANALYSIS_IMPLEMENTATION_PLAN.md) | Browser sign-in, session ownership, catalog adapters, analysis, and implementation checkpoint |
| [Spotify parity audit](SPOTIFY_COOKIE_AUTH_PARITY_AUDIT.md) | Current open functionality and platform gates |
| [Spotify validation log](SPOTIFY_WEBPLAYER_VALIDATION.md) | Dated source, fixture, browser, and native evidence; earlier entries retain their original status |
| [Spotify macOS auth validation](SPOTIFY_MACOS_AUTH_VALIDATION.md) | Platform-specific fixtures and remaining live-host validation |
| [Spotify BPM/key import](SPOTIFY_BPM_KEY_IMPORT.md) | Current download tags, recording links, scalar provenance, and local artifact fallback |
| [Spotify reference benchmark](SPOTIFY_REFERENCE_BENCHMARK.md) | Frozen reference export/comparison contract; real-audio qualification remains open |
| [Spotify metadata/local fallback plan](SPOTIFY_METADATA_AND_LOCAL_FALLBACK_IMPLEMENTATION_PLAN.md) | Proposed expanded persistence and three-band waveform integration |
| [Spotify catalog adapter sources](../backend/internal/spotify/catalog/SOURCES.md) | Implementation source/protocol provenance |

## Historical records

The [documentation archive](archive/README.md) indexes completed implementation plans, remediation records, reviews, and dated research. Use current guides and active references above to determine present behavior. Archiving preserves evidence and does not mark unresolved validation work complete.

---

## Source model

Normal ViiB browsing is intentionally source-transparent.

```text
Local folders ──scan────────────┐
                                ├──> ViiB songs catalog ──> Songs / Albums / Artists
Plex music ────synchronize──────┘                          Search / Queue / Playlists
                                                           Likes / History / Stats
                                                           Smart Mixes / AI Smart Mix

Spotify ───────separate integration──> Spotify browse / stream / download
```

Local and Plex tracks use the same ViiB song IDs at the frontend/API boundary. Plex-specific remote identity such as PMS machine identifier, library ID, rating key, and media part key is maintained by the backend.

See [Architecture and Media Source Model](architecture.md) for the detailed backend/frontend boundaries and synchronization rules.

---

## Quick Overview

```text
┌──────────────────────────────────────────────────────────┐
│ Sidebar (navigation)            Main content area        │
│ ─────────────────────           ──────────────────────   │
│ Home                            Changes per route        │
│ Songs                                                   │
│ Albums                                                  │
│ Artists                                                 │
│ Genres                                                  │
│ Playlists                                               │
│ Liked Songs / Albums                                    │
│ Smart Playlists                                         │
│ Search                                                  │
│ Spotify                                                 │
│ DJ Mode                                                 │
│ Downloads                                               │
│ Stats                                                   │
│ Settings                                                │
├──────────────────────────────────────────────────────────┤
│ Player bar (persistent, bottom)                         │
│ Artwork · Track info · Controls · Volume · Queue · EQ    │
└──────────────────────────────────────────────────────────┘
```

Plex does not add duplicate Plex-only Songs, Albums, Artists, Search, or Now Playing screens. Configure Plex through **Settings → Library Sources**, then use the normal ViiB UI.

---

## Architecture Summary

- **Frontend:** React 19 + TypeScript + Vite, Zustand state, Tailwind CSS
- **Backend:** Go **1.26.8+**, chi HTTP router, SQLite/WAL, Wails v2
- **Catalog:** SQLite `songs` is the canonical catalog for local and synchronized Plex tracks
- **Local media:** filesystem scanner, platform-aware change detection, metadata extraction, local cover handling
- **Plex media:** backend GDM discovery, manual PMS configuration, JWT/PIN authentication, metadata sync, authenticated audio/artwork proxying
- **Spotify:** separate Spotify browse/stream/download integration
- **Search:** indexed server-side prefix search over the ViiB catalog and playlists
- **Real-time:** SSE for scan/library/job events where applicable
- **Recovery:** diagnostics, repair, validated SQLite backup, staged offline restore, continuous local-folder monitoring
- **Audio:** HTML5/Web Audio player; local and Plex catalog tracks use the same `/api/audio/{songId}` frontend contract
- **Security:** machine-bound AES-256-GCM sensitive-setting storage; Plex tokens stay server-side for PMS media requests
- **Platform:** Wails desktop on Windows/macOS/Linux plus browser/server mode

See the [README](../README.md) for development/build requirements.
