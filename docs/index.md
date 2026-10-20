# ViiB MediaHub — Documentation

ViiB MediaHub is a local-first music hub for Windows, macOS, and Linux. It combines a React frontend with a Go backend and ships as a Wails v2 desktop application or local web/server build.

ViiB can build one canonical music catalog from two first-class sources:

- **Local filesystem folders** scanned by ViiB.
- **Plex Media Server music/audio libraries** synchronized into the same catalog and streamed through the ViiB backend.

Spotify remains a separate integration: Free/unknown-plan sessions can browse, while streaming and downloads require confirmed Premium. Local playback works independently of Spotify Premium.

> Plex support is audio/music only. ViiB does not support Plex movie, TV, photo, music-video, general video playback, or video transcoding.

---

## User guides

| Page | Description |
|---|---|
| [Development and builds](development.md) | Source setup, native packaging, API entry points, data handling, and contributor checks |
| [UI screenshots](screenshots.md) | Reproduce and review the README and user-guide captures |
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

Branch scope and references reviewed on 2026-10-12. Further local scanning/DSP improvements are deferred to a future branch. Implementation status and dated validation evidence are distinct: an implemented adapter or displayed analysis result does not close a parity or professional-quality gate.

The [October 15 documentation refresh](DOCUMENTATION_REVIEW_2026-10-15.md) reconciles current user-facing behavior and screenshots; it does not change those qualification gates.

| Reference | Purpose and status |
|---|---|
| [Product & Engineering Roadmap](ROADMAP.md) | Active overarching phases, priorities, release gates, dependencies and qualified-vs-planned status |
| [Accessibility qualification](ACCESSIBILITY_QUALIFICATION.md) | Draft WCAG 2.2 AA workflow/host test plan, DJ alternative tasks and findings register; execution remains open |
| [Release dependency inventories](release-dependency-inventories.md) | Proposed checksummed npm/Go release inventories, reproducibility and coverage limits |
| [Proposed release notes](releases/UNRELEASED.md) | Reviewed change candidates for a future release; not a published-release claim |
| [Repository governance](repository-governance.md) | Main-only required CI ruleset, stacked-PR handling and reviewed emergency bypass process |
| [Release signing policy](release-signing-policy.md) | Proposed fail-closed stable release guard and remaining signing/notarization qualification |
| [OpenAPI v2](openapi-v2.yaml) | Machine-readable core API routes; additional analysis detail routes currently follow backend handler contracts |
| [Stem package v1](VIIB_STEM_PACKAGE_V1.md) | Current `.viibstems` interchange and validation contract |
| [DJ professional analysis roadmap](DJV2_PROFESSIONAL_TRACK_ANALYSIS_ROADMAP.md) | Delivered analysis engineering and remaining accuracy/calibration/release gates |
| [DJ analysis context](DJV2_TRACK_ANALYSIS_CONCISE_CONTEXT.md) | Current scope freeze and links to preserved measurement evidence |
| [DJ stems, cues, and mix planning roadmap](DJV2_PRO_STEMS_CUES_MIX_INTELLIGENCE_ROADMAP.md) | Implementation status and remaining package/cue/planning work |
| [BPM reference benchmark](REFERENCE_BPM_BENCHMARK.md) | Reproducible external comparison workflow and evidence |
| [Rhythm/structure reference benchmark](REFERENCE_RHYTHM_STRUCTURE_BENCHMARK.md) | Beat, downbeat, structure, and energy comparison workflow |
| [Spotify implementation plan](SPOTIFY_WEBPLAYER_AUTH_AUDIO_ANALYSIS_IMPLEMENTATION_PLAN.md) | Browser sign-in, session ownership, catalog adapters, analysis, and implementation checkpoint |
| [Spotify parity audit](SPOTIFY_COOKIE_AUTH_PARITY_AUDIT.md) | Current open functionality and platform gates |
| [Spotify validation log](SPOTIFY_WEBPLAYER_VALIDATION.md) | Dated source, fixture, browser, and native evidence; earlier entries retain their original status |
| [Spotify macOS auth validation](SPOTIFY_MACOS_AUTH_VALIDATION.md) | Platform-specific fixtures and remaining live-host validation |
| [Spotify BPM/key import](SPOTIFY_BPM_KEY_IMPORT.md) | Current download tags, recording links, scalar provenance, and local artifact fallback |
| [Spotify reference benchmark](SPOTIFY_REFERENCE_BENCHMARK.md) | Frozen reference export/comparison contract; real-audio qualification remains open |
| [Spotify branch scope](SPOTIFY_BRANCH_REMAINING_WORK.md) | Included behavior, disclosed limitations and new PR boundary |
| [Spotify PR closeout](SPOTIFY_BRANCH_CLOSEOUT_NEXT_STEPS.md) | Final bug review, verification and publication state |
| [Future local scanning/analysis](LOCAL_ANALYSIS_FUTURE_BRANCH.md) | Deferred estimator, scan/backfill, corpus and quality work |
| [Connected resource inventory](SPOTIFY_CONNECTED_RESOURCE_INVENTORY.md) | Provider contract/fixture/live evidence and limits |
| [Preparation performance](SPOTIFY_PREPARATION_PERFORMANCE.md) | Reproducible existing offline measurements, not full qualification |
| [Documentation review](DOCUMENTATION_REVIEW_2026-10-12.md) | Scope reconciliation, archive and documentation validation |
| [Spotify catalog adapter sources](../backend/internal/spotify/catalog/SOURCES.md) | Implementation source/protocol provenance |

## Historical records

The [documentation archive](archive/README.md) indexes completed implementation plans, remediation records, reviews, and dated research. Use current guides and active references above to determine present behavior. Archiving preserves evidence and does not mark unresolved validation work complete.

The [Spotify metadata foundation archive](archive/spotify-metadata-foundation/README.md) preserves the superseded broad plan, milestone logs, public-corpus/Beat This reviews and dated JSON evidence.

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
│ AI Smart Mix                                            │
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

The full DJ workspace replaces the normal bottom Player and Queue with deck/mixer controls; skinny player mode can still display the Player.

Plex does not add duplicate Plex-only Songs, Albums, Artists, Search, or Now Playing screens. Configure Plex through **Settings → Library Sources**, then use the normal ViiB UI.

---

## Architecture Summary

- **Frontend:** React 19 + TypeScript + Vite, Zustand state, Tailwind CSS
- **Backend:** Go **1.26.9+**, chi HTTP router, SQLite/WAL, Wails v2
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

See [Development and builds](development.md) for development/build requirements and the [README](../README.md) for the product tour.
