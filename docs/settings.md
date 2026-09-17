# Settings

![Settings page](../assets/screenshots/settings.png)

The Settings page provides full configuration for all aspects of ViiB MediaHub.

The screenshots below were captured at 1440×900 against a clean local backend
state, so empty-library and unconfigured-integration states are intentional.

| Settings surface | Screenshot |
|---|---|
| Library Sources | [Library Sources](../assets/screenshots/settings-library-sources.png) |
| Library Operations | [Library Operations](../assets/screenshots/settings-library-operations.png) |
| Playback & Audio | [Playback & Audio](../assets/screenshots/settings-playback-audio.png) |
| Integrations & Spotify | [Integrations & Spotify](../assets/screenshots/settings-integrations-spotify.png) |
| AI & Enrichment | [AI & Enrichment](../assets/screenshots/settings-ai-enrichment.png) |
| Appearance & Now Playing | [Appearance & Now Playing](../assets/screenshots/settings-appearance-now-playing.png) |
| System & Logs | [System & Logs](../assets/screenshots/settings-system-logs.png) |

---

## Sections

| Section | Purpose |
|---|---|
| [Library Sources](#library-sources) | Local folders, scans, monitoring, Plex, maintenance, and reset |
| [Library Operations](#library-operations) | Track analysis, metadata operations, diagnostics, repair, and backup/restore |
| [Playback & Audio](#playback--audio) | Crossfade, gapless, normalization, EQ, and output routing |
| [Integrations & Spotify](#integrations--spotify) | Downloads, conversion, Spotify, and Last.FM |
| [AI & Enrichment](#ai--enrichment) | AI provider, semantic retrieval, and metadata enrichment |
| [Appearance & Now Playing](#appearance--now-playing) | Home layout, Smart Mix visibility, and player presentation |
| [System & Logs](#system--logs) | System information, support tools, and the in-app debug log |

---

## Library Sources

The Library Sources tab includes a backend connection status indicator. A green indicator means the API is reachable; a warning indicator means the frontend is running without an available Go backend.

### Scan Folders

Lists all local music directories ViiB MediaHub monitors. Each folder has:
- **Path** — directory path
- **Remove** button — stop watching this folder

To add a folder click **Add Folder** and use the folder browser dialog.

Continuous monitoring is configured directly below local folder scanning. It checks configured folders for changes between manual scans; choose 5 seconds, 15 seconds, 30 seconds, 1 minute, or 5 minutes.

### Plex Media Server music

Open **Settings → Library Sources → Plex Media Server** to configure Plex as a remote music source. The Plex panel supports:

- bounded automatic LAN discovery through Plex GDM;
- manual hostname, IP, HTTP, or HTTPS server configuration;
- Plex-hosted sign-in/reconnect for claimed servers;
- music/audio-library selection;
- explicit synchronization/resynchronization;
- offline/authentication-required status;
- changing or removing the Plex source without modifying anything on the Plex server.

Plex songs synchronize into the same ViiB catalog used by local files, so they appear in the existing Songs, Albums, Artists, Search, queue, playlists, likes, history, Smart Mixes, AI DJ, and statistics experiences. Plex video libraries are not supported.

The Plex panel also includes an optional **AI Metadata Writeback** workflow. AI enrichment is always local first: preview the per-track genre/year changes, explicitly approve the exact diff, then ViiB writes and locks only those fields in Plex and verifies them. It never writes audio-file tags; mood/energy/tempo/BPM and semantic embeddings remain ViiB-only. The Plex token must have PMS metadata-management permission.

See [Plex Media Server Music Support](plex-music.md) for setup, authentication/security, synchronization behavior, playback details, and troubleshooting.

### Scan Now

Triggers an immediate incremental scan of all configured local folders. Progress is streamed through the library event path and reflected in the app's status surfaces. Plex synchronization is a separate explicit operation in the Plex source panel.

### Reset Library

**Destructive to ViiB's local catalog state.** Removes catalog data from the ViiB database, then re-scans configured local folders. It does not delete source media. Use Library Operations diagnostics/repair before resetting the library when possible.

## Library Operations

The [Library Operations](library-operations.md) tab prepares tracks for DJ features and handles metadata operations, diagnostics, repair, validated backups, and staged restore. Track analysis supports local files and available Plex tracks; **Prepare new tracks automatically** is on by default for local scans, while Plex preparation is explicit.

See [Library Operations](library-operations.md#track-analysis) for the analysis workflow and recovery details.

---

## Appearance & Now Playing

Controls how the [Home](home.md) page is presented.

| Setting | Description |
|---|---|
| Home Layout | Choose **Music Shelves**, **Cover Wall**, or **Compact Dashboard** |
| Show Smart Mixes | Show or hide auto-generated Smart Mix sections on Home |
| Now Playing visuals | Visualizer mode, artwork opacity, fullscreen background, and fullscreen opacity |

Home layout choices persist across reloads.

---

## Playback & Audio

| Setting | Description |
|---|---|
| Crossfade | Duration (0–12 s) of the fade between tracks |
| Gapless Playback | Pre-load next track to eliminate silence between songs |
| Volume Normalization | Adjust playback level so all tracks sound similar |
| Equalizer | Toggle the 10-band EQ panel |

---

### Audio Output Devices

Configures separate audio devices for DJ use:

- **Main Output** — speakers or PA system
- **Headphone / Cue Output** — DJ headphones for previewing tracks without the audience hearing

Device routing uses the Web Audio API's `setSinkId`. Devices are listed after the browser grants audio permission.

---

## Integrations & Spotify

| Setting | Description |
|---|---|
| Client ID | Your Spotify Developer app Client ID |
| Client Secret | Your Spotify Developer app Client Secret |
| Download Location | Folder where downloaded OGG files are saved |
| Concurrent Downloads | How many simultaneous downloads are allowed (1–10, default 3) |
| Quick Scan After Downloads | Number of completed downloads before an automatic quick scan; 0 disables it |
| MP3 Conversion | Optional Ogg-to-MP3 conversion and worker count |

When creating the Spotify Developer app, add this exact Redirect URI: `http://127.0.0.1:34115/callback`. `wails.localhost` is not a valid Spotify callback for the desktop application.

> Refer to [Spotify Integration](spotify.md) for how to create a Developer app.

### Last.FM Integration

| Setting | Description |
|---|---|
| API Key | From your Last.FM developer account |
| Shared Secret | From your Last.FM developer account |
| Test Connection | Verifies the key is valid |
| Scrobbling | Enable scrobbling with your Last.FM username and password |

Last.FM provides community-sourced genre and tag metadata for tracks, artists, and albums.

---

## AI & Enrichment

### AI Provider

Select and configure the LLM backend used for:
- Genre enrichment
- Mood / energy / tempo analysis
- Smart playlist generation
- AI DJ set building

| Provider | Notes |
|---|---|
| Google Gemini | Requires Gemini API key |
| OpenAI | Requires OpenAI API key |
| Anthropic | Requires Anthropic API key |
| OpenRouter | Requires an OpenRouter API key; available text models are loaded into the model dropdown after saving the key |
| Ollama | Local model; set the model name and endpoint URL |
| X.AI (Grok) | Requires X.AI API key |

Enter the API key (or endpoint for Ollama) and click **Save**.

### Semantic Retrieval Index

The Semantic Retrieval Index is configured independently from the AI chat provider. It powers meaning-based candidate recall for Smart Playlists and AI DJ when it is ready; chat/metadata enrichment settings do not silently change its vector space.

| Setting or control | Description |
|---|---|
| Embedding Provider | **Auto**, local **Ollama**, cloud **OpenAI**, **Google Gemini**, **OpenRouter**, or **Disabled**. Auto only reuses a configured key for that same provider. |
| Embedding Model / dimensions | Identity for the embedding space. Changing it safely rebuilds the index rather than mixing vectors. |
| Test Embedding Provider | Sends one non-persistent test embedding and reports the detected dimensions. |
| Reindex | Starts a background rebuild; playback and normal metadata matching remain usable. |
| Retry Errors | Requeues only documents left in an error state after bounded provider retries. |

Ollama uses its embedding endpoint and ViiB never downloads a model automatically. Gemini uses `gemini-embedding-2` at 768 dimensions by default; OpenRouter uses its embeddings API with `openai/text-embedding-3-small` at 512 dimensions by default. Cloud providers receive deterministic semantic document text but never file paths, internal song IDs, or listening history. Before any cloud indexing starts, ViiB displays either OpenAI's current one-time estimate or a Gemini/OpenRouter data-and-cost notice and requires explicit confirmation.

Status reports ready/indexing/configuration/error state plus ready, pending, and error document counts. If the index is unavailable, Smart Playlists and AI DJ use their normal metadata fallback rather than becoming unavailable.

### Metadata enrichment source

Choose AI, Last.FM, or Hybrid as the source used for automatic metadata enrichment during library scans. The Library Operations tab provides the **Unified AI Enrichment** action for a full run across genres, mood, energy, tempo, BPM, and release year.

- Progress is shown in the Library Operations status panel and the in-app log when events are emitted.
- Genres are written to the `songs.genre` column in SQLite. For Plex-backed tracks, a separate explicit AI Metadata Writeback preview can later send approved genre changes to Plex; no source audio file tags are modified.

### Unified AI Enrichment

Runs full metadata enrichment: genres, mood, energy, tempo, BPM, and release year.

- Click **Run Unified Enrichment** to start.
- This is slower than genre-only enrichment but produces richer data for AI DJ.

---

## System & Logs

A scrollable in-app event log containing events recorded by the frontend and backend-facing workflows:
- Library scan progress
- Plex source/synchronization activity
- Enrichment status
- Download events
- API errors

Click **Clear** to remove old entries. The log is in-memory only and does not persist after restart.
