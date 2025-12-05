# ViiB MediaHub "Wow" Enhancements Tracker

> **Last Updated**: December 2024  
> **Status Legend**: ✅ Implemented | 🟡 Partial | ⬜ Not Started

Phases legend: 
- 📝 Discovery/Design
- 🔍 Feasibility/Spikes
- 🛠️ Backend
- 🎨 Frontend/UX
- 🗃️ Data/Storage
- 🧪 QA/Perf
- 📈 Analytics/Telemetry
- 🚀 Rollout/Docs

---

## Currently Implemented Features (Reference)

These features are already in the codebase and should inform future enhancements:

### Audio & Playback ✅
- 10-Band Parametric Equalizer (32Hz - 16kHz) with 22 presets
- Auto-EQ based on genre tags
- Crossfade transitions (configurable duration)
- Gapless playback support
- Real-time visualizer (Wave, Spectrum, Aurora modes)
- Volume normalization option
- Dual audio elements for seamless crossfading

### Smart Mixes ✅
- Heavy Rotation (most played recently)
- Rediscover Favorites (high play count, not played in 30+ days)
- Fresh Finds (recently added)
- Chill Acoustic Evening (genre-based)
- 90s Alternative Mix (year + genre filtering)

### Lyrics ✅
- Synced lyrics fetching from lrclib.net
- Auto-scroll to current line
- Click-to-seek on lyric lines
- Plain lyrics fallback

### Spotify Integration ✅
- Direct streaming with quality selection (High/Medium/Low)
- Album, track, playlist downloads (OGG Vorbis format)
- Real-time download progress via SSE
- Background metadata enrichment for albums/artists
- Prefer-local-playback toggle

### Keyboard Navigation ✅
- Space: Play/Pause
- Arrow keys: Volume, Seek
- Shift+Arrows: Next/Previous track
- M: Mute, Q: Queue, E: Equalizer, N: Now Playing

---

## AI DJ & Smart Transitions
**Priority**: High | **Complexity**: High

Builds on existing crossfade infrastructure to add intelligent transitions.

- [ ] 📝 Define goals (beat-sync, auto-tempo, crossfades, waveforms)
- [ ] 🔍 Spike beat detection and tempo matching (Go audio pipeline)
  - Consider: essentia-go, aubio bindings, or custom FFT-based detector
  - Alternative: Pre-analyze on import, store BPM in database
- [ ] 🛠️ Backend: expose beatmap/tempo endpoints; transition scheduler
  - Add `bpm`, `key`, `beatgrid` fields to songs table
  - Create `/api/songs/{id}/analysis` endpoint
- [ ] 🎨 Frontend: waveform overlays, transition controls, live visuals
  - Integrate with existing Visualizer component
  - Add waveform component using WaveSurfer.js or similar
- [ ] 🗃️ Cache beatmaps per track; invalidate on metadata changes
- [ ] 🧪 Gapless playback tests; crossfade timing accuracy; CPU/mem
- [ ] 📈 Capture transition success/failure events; skip/like impact
- [ ] 🚀 Feature flag rollout; short tutorial/toast

## Generative Smart Mixes (Vibe/Occasion)
**Priority**: High | **Complexity**: Medium

Extends existing `smartMix.ts` with more sophisticated generators.

- [x] 📝 Define presets - Existing: Heavy Rotation, Rediscover, Fresh Finds, Chill Acoustic, 90s Alt
- [ ] 📝 Add new presets: Focus Mode, Workout Energy, Late Night, Road Trip, Morning Coffee
- [ ] 🔍 Spike scoring model (mood/energy) using local + Spotify audio features
  - Spotify Audio Features API: energy, valence, danceability, tempo
- [ ] 🛠️ Backend: mix generator API; re-rank based on skips/likes
  - Leverage existing `playCount`, `skipCount`, `lastPlayed` fields
- [ ] 🎨 Frontend: mix builder UI, live preview, re-rank in-place
  - Add sliders for energy, mood, tempo range
  - "Regenerate" button with animation
- [ ] 🗃️ Persist mix seeds/params; cache candidates
- [ ] 🧪 Validate diversity/no artist clumping; long-run stability tests
- [ ] 📈 Track engagement, skips, completion rate by mix type
- [ ] 🚀 Gradual enablement; preset templates; help/empty states

## Personal Radio with Context (Time/Weather/Location)
**Priority**: Medium | **Complexity**: Medium

- [ ] 📝 Define context signals allowed offline/privacy rules
  - Time of day (morning, afternoon, evening, late night)
  - Day of week (weekday focus vs weekend vibes)
  - Optional: Weather API integration (OpenWeatherMap free tier)
- [ ] 🔍 Spike context ingestion (time, weather API, coarse location)
- [ ] 🛠️ Backend: context-aware radio endpoint; fallback logic offline
  - Create `GET /api/radio/contextual?time=morning&mood=focus`
- [ ] 🎨 Frontend: energy slider, context chips, quick toggle
  - Add context indicator to Smart Mix cards
- [ ] 🗃️ Cache recent context snapshots; respect opt-in storage
- [ ] 🧪 A/B test context vs non-context outcomes; bias checks
- [ ] 📈 Log context types used and resulting skips/likes
- [ ] 🚀 Opt-in flow; clear privacy copy; toggle in Settings

## Voice & Natural Language Control
**Priority**: Low | **Complexity**: High

- [ ] 📝 Define intents (play by mood/BPM, queue similar, create mixes)
  - "Play something upbeat"
  - "Add more songs like this to queue"
  - "Create a mix for working out"
- [ ] 🔍 Spike on-device vs cloud ASR; intent parser selection
  - Consider: Web Speech API (browser native), Whisper (local)
- [ ] 🛠️ Backend: intent resolver; safety/permissions guardrails
- [ ] 🎨 Frontend: mic affordance, live transcription, error recovery
- [ ] 🗃️ Session transcripts stored ephemerally; redact PII
- [ ] 🧪 Latency, accuracy, fallback to text; noisy-room tests
- [ ] 📈 Track intent success/fail; common misunderstood phrases
- [ ] 🚀 Soft launch with whitelist; help tips

## Adaptive Audio Enhancement
**Priority**: Medium | **Complexity**: Medium

Extends existing 10-band EQ with intelligent presets.

- [x] 📝 Define modes - Existing: 22 EQ presets, Auto-EQ by genre
- [ ] 📝 Add: Loudness leveling, Vocal clarity boost, Room correction presets
- [ ] 🔍 Spike DSP chain performance in Go; headphone vs speaker profiles
  - Consider: ReplayGain calculation on import
- [ ] 🛠️ Backend: per-track enhancement pipeline; device profiles API
  - Add `replayGain` field to songs table
- [ ] 🎨 Frontend: quick toggles, per-room presets, visual meters
  - Add loudness meter to visualizer
- [ ] 🗃️ Store per-device preset mapping; default fallbacks
- [ ] 🧪 ABX tests; latency under load; distortion/clip checks
- [ ] 📈 Measure feature usage, toggles, session duration impact
- [ ] 🚀 Gradual rollout; education tooltips

## Immersive Visualizer Modes
**Priority**: Medium | **Complexity**: Medium

Extends existing Wave/Spectrum/Aurora visualizer.

- [x] 📝 Define visual themes - Existing: Wave, Spectrum, Aurora
- [ ] 📝 Add themes: Album art rooms (3D), Beat-driven shaders, Particle systems
- [ ] 🔍 Spike GPU/WebGL performance; beat-signal hook
  - Consider: Three.js, PIXI.js, or custom WebGL shaders
- [ ] 🛠️ Backend: provide beat/section markers to client
- [ ] 🎨 Frontend: visualizer gallery, lyric-sync typography option
  - Add visualizer mode selector in Now Playing
- [ ] 🗃️ Cache theme assets; quality presets for low-power devices
- [ ] 🧪 FPS, CPU/GPU load, memory leaks; screen-burn safety
- [ ] 📈 Engagement time in visualizer; drop-offs
- [ ] 🚀 Intro carousel; safe defaults

## Lyrics Plus (Karaoke, Translation, Lyric Search)
**Priority**: High | **Complexity**: Medium

Extends existing synced lyrics feature from lrclib.net.

- [x] 📝 UX for karaoke highlighting - Existing: synced lyrics with auto-scroll
- [x] 🎨 Click-to-seek on lyric lines - ✅ Implemented
- [ ] 📝 Add: translation overlay, jump-to-line search, karaoke mode toggle
- [ ] 🔍 Spike alignment accuracy; translation source/options
  - Consider: Google Translate API, DeepL, or LibreTranslate (self-hosted)
- [ ] 🛠️ Backend: lyric sync timestamps; translation service adapter
  - Create `/api/lyrics/{song}/translate?lang=es`
- [ ] 🎨 Frontend: karaoke mode (dim non-active lines), dual-language overlay, search-to-jump
  - Add language selector in Now Playing lyrics view
- [ ] 🗃️ Cache synced lyrics; language preference storage
- [ ] 🧪 Alignment tolerance; RTL testing; network-failure fallbacks
- [ ] 📈 Usage of search-to-play; completion of karaoke sessions
- [ ] 🚀 Staged rollout by locale; clear licensing notes

## Moments & Highlights (Hooks/Choruses)
**Priority**: Medium | **Complexity**: High

- [ ] 📝 Define highlight types (hook, chorus, drop); UX for jump-to-hook
- [ ] 🔍 Spike section detection; scoring heuristics
  - Consider: librosa-style onset detection, chorus repetition finder
- [ ] 🛠️ Backend: store per-track highlight timestamps; API
  - Add `highlights` JSON field to songs table
  - Create `/api/songs/{id}/highlights`
- [ ] 🎨 Frontend: highlight chips on progress bar, sampler mode
  - Add section markers to waveform display
- [ ] 🗃️ Cache highlights; invalidate on re-analysis
- [ ] 🧪 Accuracy vs human labels; regression on diverse genres
- [ ] 📈 Track highlight jumps; sampler engagement
- [ ] 🚀 Soft launch with toggle; helper tips

## Smart Downloads (Predictive Offline)
**Priority**: Medium | **Complexity**: Medium

Extends existing Spotify download system.

- [x] 🛠️ Backend: Download queue with status tracking ✅ Implemented
- [x] 🎨 Frontend: Download progress, retry failed, clear completed ✅ Implemented
- [ ] 📝 Rules for trip/commute modes; storage/quality bounds
- [ ] 🔍 Spike prediction from habits + calendar; offline fallback
  - Analyze listening patterns by time of day
- [ ] 🛠️ Backend: download planner; adaptive quality logic
  - Create `/api/downloads/smart-queue` endpoint
- [ ] 🎨 Frontend: trip mode toggle, forecasted downloads list
- [ ] 🗃️ Track cached sets with expiry; storage guardrails
- [ ] 🧪 Offline scenarios; space pressure; battery impact
- [ ] 📈 Hit rate (played vs cached); purge efficiency
- [ ] 🚀 Opt-in; safety stop; notifications

## Collab & Shared Sessions
**Priority**: Low | **Complexity**: High

- [ ] 📝 Define modes (LAN, link-based), permissions, guardrails
  - Local network discovery via mDNS/Bonjour
  - Shareable session links (WebSocket-based sync)
- [ ] 🔍 Spike sync protocol latency; conflict resolution
- [ ] 🛠️ Backend: session state, reacts, DJ handoff rules
  - Create `/api/sessions` WebSocket endpoint
- [ ] 🎨 Frontend: presence UI, reacts, queue visibility
- [ ] 🗃️ Session persistence with TTL; audit events
- [ ] 🧪 Network loss, hostile edits, multi-client consistency
- [ ] 📈 Session length, participants, edits vs conflicts
- [ ] 🚀 Rolling enablement; mod tools; clear leave/end controls

## Discovery with Explanations
**Priority**: Medium | **Complexity**: Medium

- [ ] 📝 UX for "Why this track" cards and A/B preview
- [ ] 🔍 Spike similarity signals (BPM/genre/era/mood)
  - Leverage existing genre, year fields + Spotify audio features
- [ ] 🛠️ Backend: explanation payloads; preview endpoints
  - Add explanation field to Smart Mix song entries
- [ ] 🎨 Frontend: inline cards, side-by-side previews
  - Add info icon on Smart Mix tracks with tooltip
- [ ] 🗃️ Cache explanations; tie to recommendation version
- [ ] 🧪 Explanation accuracy perception tests; latency
- [ ] 📈 CTR on explained recs; skip delta vs baseline
- [ ] 🚀 Enable per-surface; copy polish

## Advanced Queue Magic
**Priority**: High | **Complexity**: Medium

Extends existing drag-and-drop queue functionality.

- [x] 🎨 Drag-and-drop reorder ✅ Implemented
- [x] 🎨 Remove individual items ✅ Implemented
- [x] 🎨 Clear queue ✅ Implemented
- [ ] 📝 Features: fill-to-duration, anti-repeat, surprise slots
  - "Fill queue to 1 hour with similar songs"
  - "Don't repeat artists within 5 songs"
  - "Add random surprise track every 10 songs"
- [ ] 🔍 Spike queue algorithms and performance on large queues
- [ ] 🛠️ Backend: queue ops API; constraints (artist spacing, duration)
  - Create `/api/queue/autofill?duration=3600&similar=true`
- [ ] 🎨 Frontend: fill-to-duration UI, constraint toggles
  - Add "Smart Fill" button to queue panel
- [ ] 🗃️ Persist queue rules per session; recoverable edits
- [ ] 🧪 Stress with long queues; correctness of constraints
- [ ] 📈 Measure repeat avoidance, surprise slot retention
- [ ] 🚀 Rollout behind toggle; mini-guide

## Contextual Shortcuts & Clips
**Priority**: Medium | **Complexity**: Medium

Extends existing keyboard navigation system.

- [x] 🎨 Existing shortcuts: Space, Arrows, M, Q, E, N, Escape ✅ Implemented
- [ ] 📝 Define hotkeys (instant ID, add to running playlist, clip last 15s)
  - Ctrl+L: Add current song to "Liked" playlist
  - Ctrl+C: Clip last 15 seconds
  - Ctrl+Shift+P: Add to specific playlist (quick picker)
- [ ] 🔍 Spike background permissions; low-latency clipping
- [ ] 🛠️ Backend: clip extraction; quick-tag pipeline
  - Create `/api/clips` endpoint for saving audio snippets
- [ ] 🎨 Frontend: hotkey editor, snackbar confirmations
  - Add keyboard shortcut settings panel
- [ ] 🗃️ Store shortcut profiles; clipped-moment metadata
- [ ] 🧪 Hotkey collisions; clip accuracy; low-perf devices
- [ ] 📈 Shortcut usage; clip save vs discard
- [ ] 🚀 Progressive rollout; cheat sheet

## Clipping & Snippets as Seeds
**Priority**: Low | **Complexity**: High

- [ ] 📝 UX for creating/saving moments and using them as seeds
  - "Find more songs like this 30-second section"
- [ ] 🔍 Spike similarity-from-snippet; non-destructive storage
- [ ] 🛠️ Backend: snippet indexing; seed-to-playlist generator
- [ ] 🎨 Frontend: moment editor, seed-to-mix CTA
- [ ] 🗃️ Store snippets with timestamps; dedupe
- [ ] 🧪 Edge cases on very short/long snippets; loudness norm
- [ ] 📈 Snippet-to-playlist conversion rate
- [ ] 🚀 Launch with tutorial; undo support

## Spatial & Binaural Mode
**Priority**: Low | **Complexity**: High

- [ ] 📝 Define spatial presets and supported devices
  - Headphone-optimized binaural audio
  - Virtual surround presets
- [ ] 🔍 Spike binaural rendering perf; HRTF library selection
  - Consider: Resonance Audio, OpenAL Soft
- [ ] 🛠️ Backend: spatial processing path; per-track opt-out
- [ ] 🎨 Frontend: spatial toggle, intensity slider, per-track badges
  - Add to existing Equalizer panel
- [ ] 🗃️ Cache device capability; remember last setting
- [ ] 🧪 Latency/CPU; ear fatigue tests; mono compatibility
- [ ] 📈 Usage duration; re-toggle rates; complaints
- [ ] 🚀 Limited beta; warnings for unsupported devices

## Dynamic Playlists from Life (Calendar/Location/Weather)
**Priority**: Low | **Complexity**: Medium

Similar to Personal Radio but with persistent playlist generation.

- [ ] 📝 Define triggers (events, commute, weather changes)
  - "Workout playlist auto-plays when calendar event starts"
  - "Rainy day playlist when weather changes"
- [ ] 🔍 Spike calendar integration; privacy boundaries
  - Browser Calendar API, Google Calendar integration
- [ ] 🛠️ Backend: trigger rules engine; fallback logic
- [ ] 🎨 Frontend: trigger editor, preview upcoming mixes
- [ ] 🗃️ Store rules locally; sync opt-in; revoke controls
- [ ] 🧪 False positives; stale triggers; offline behavior
- [ ] 📈 Engagement per trigger type
- [ ] 🚀 Opt-in onboarding; clear disable-all switch

## Library Health & Insights
**Priority**: High | **Complexity**: Medium

- [ ] 📝 Define audits (duplicates, bitrate, missing art/tags)
  - Detect duplicate files (by audio fingerprint or metadata match)
  - Find low-quality files (< 128kbps)
  - Identify missing album art, incomplete tags
- [ ] 🔍 Spike tag fixers and art upscaling sources
  - MusicBrainz for metadata correction
  - Cover Art Archive for missing artwork
- [ ] 🛠️ Backend: audit runner; fix/apply endpoints
  - Create `/api/library/audit` endpoint
  - Create `/api/library/fix` for batch corrections
- [ ] 🎨 Frontend: audit dashboard, one-click fixes, diff preview
  - Show before/after for tag changes
- [ ] 🗃️ Cache audit results; version fixes; backups
- [ ] 🧪 Safety on bulk edits; rollback tests
- [ ] 📈 Fix success rate; before/after library quality
- [ ] 🚀 Guided flow; dry-run mode

## Multi-Source Blends (Local + Spotify + YouTube Audio)
**Priority**: Medium | **Complexity**: High

Extends existing Spotify + Local library unification.

- [x] 🛠️ Unified playback for local and Spotify ✅ Implemented
- [x] 🎨 Prefer-local-playback toggle ✅ Implemented
- [ ] 📝 Define legality/DRM constraints; UX for mixed-source queues
- [ ] 🔍 Spike loudness normalization across sources; stream adapters
  - YouTube: yt-dlp audio extraction (legal considerations)
- [ ] 🛠️ Backend: unified playback pipeline; source fallback order
- [ ] 🎨 Frontend: source badges, availability hints, gapless handling
  - Show Spotify/Local/YouTube icon on track rows
- [ ] 🗃️ Cache source preferences; prefetch policies
- [ ] 🧪 Gapless + normalization tests; regional restrictions
- [ ] 📈 Track source mix ratios; failure causes
- [ ] 🚀 Region-aware rollout; clear availability messaging

## DJ Practice Mode (Dual Decks)
**Priority**: Low | **Complexity**: Very High

Extends existing dual audio element infrastructure.

- [x] 🛠️ Dual audio elements for crossfade ✅ Implemented
- [ ] 📝 Define deck controls (tempo, cue, sync, waveforms)
- [ ] 🔍 Spike low-latency cueing; key detection accuracy
  - Consider: Tone.js for tempo/pitch shifting
- [ ] 🛠️ Backend: dual-stream support; next-track suggestion engine
  - Key/BPM matching suggestions
- [ ] 🎨 Frontend: dual-deck UI, jog controls, key/BPM display
  - Split-screen deck layout
  - Virtual crossfader
- [ ] 🗃️ Cache analyzed key/BPM; per-user deck prefs
- [ ] 🧪 Latency, drift, key/tempo correctness
- [ ] 📈 Session length; suggested-next acceptance
- [ ] 🚀 Beta with pro users; tutorials

## Social Taste Cards & Shareable Mixes
**Priority**: Low | **Complexity**: Medium

- [ ] 📝 Define taste card facets (genres, eras, BPM bands)
  - "Your Top Genres: Rock 45%, Electronic 30%, Jazz 25%"
  - "Listening Personality: Night Owl, Deep Diver, Playlist Hopper"
- [ ] 🔍 Spike aggregation pipeline; privacy defaults
- [ ] 🛠️ Backend: taste card generator; shareable mix endpoints
  - Create `/api/profile/taste-card`
- [ ] 🎨 Frontend: card design, export/share, mix CTA
  - Generate shareable image
  - Copy link to clipboard
- [ ] 🗃️ Store cards locally; invalidate on library changes
- [ ] 🧪 Accuracy vs perception; share flow friction
- [ ] 📈 Shares, replays, saves; opt-out rates
- [ ] 🚀 Opt-in sharing; safe defaults; copy polish

---

## New Feature Ideas (Brainstorm)

### Sleep Timer & Fade Out ✅
**Priority**: High | **Complexity**: Low | **Status**: Implemented
- ✅ Set a timer to stop playback after X minutes (15/30/45/60/90/120 min)
- ✅ Gradual volume fade before stopping (last 30 seconds)
- ✅ "Play X more songs then stop" option
- ✅ "Stop at end of current song" option

### Listening History & Stats ✅
**Priority**: High | **Complexity**: Low | **Status**: Implemented
- ✅ Total listening time display
- ✅ Top artists, albums, genres by play count
- ✅ Most played song highlight card
- ✅ Library overview stats
- [ ] "On This Day" - what you listened to last year (future)
- [ ] Streak tracking (consecutive days of listening) (future)

### Album Completeness Tracker
**Priority**: Medium | **Complexity**: Low
- Show which albums in library are complete vs partial
- "You have 8/12 tracks from this album"
- Option to download missing tracks from Spotify

### Play Next vs Add to Queue ✅
**Priority**: High | **Complexity**: Low | **Status**: Implemented
- ✅ "Play Next" inserts after current song
- ✅ "Add to Queue" appends to end
- ✅ Available in all context menus (Song, Album, Playlist, Smart Mix)

### Recently Played Section ✅
**Priority**: High | **Complexity**: Low | **Status**: Implemented
- ✅ Show last 10 played tracks on Home page
- ✅ Quick access to return to recent listening
- ✅ Relative timestamps ("2 hours ago", "Yesterday")
- ✅ Click to play, context menu for queue actions

### Mini Player Mode
**Priority**: Medium | **Complexity**: Medium
- Picture-in-picture style floating player
- Stays on top while using other apps
- Minimal controls (play/pause, next, close)

### Scrobble to Last.fm
**Priority**: Medium | **Complexity**: Low
- Last.fm integration for tracking
- OAuth flow similar to Spotify
- Submit plays after 50% completion

### Podcast Support (RSS Feeds)
**Priority**: Low | **Complexity**: High
- Add RSS podcast feeds
- Episode management and progress tracking
- Different playback rules (remember position, 1.5x speed default)

### Import/Export Playlists
**Priority**: Medium | **Complexity**: Medium
- Export playlists as M3U, JSON
- Import M3U files
- Cross-device sync via export/import

### Artist Radio (Auto-Queue Similar)
**Priority**: High | **Complexity**: Medium
- "Start Radio" button on artist page
- Automatically queue similar artists/songs
- Uses Spotify recommendations API or local similarity

---

## Priority Matrix

### Quick Wins (High Priority, Low Complexity)
These can be implemented quickly with high user impact:

1. ✅ **Sleep Timer & Fade Out** - Simple timer + volume fade (DONE)
2. ✅ **Play Next vs Add to Queue** - Minor queue logic change (DONE)
3. ✅ **Recently Played Section** - Data already exists in DB (DONE)
4. ✅ **Listening History & Stats** - Aggregate existing play data (DONE)

### High Impact, Medium Effort
Worth investing in for significant UX improvement:

1. **Advanced Queue Magic** - Smart fill, anti-repeat
2. **Lyrics Plus** - Build on existing lyrics implementation
3. **Library Health & Insights** - Duplicate detection, tag fixing
4. **Generative Smart Mixes** - Extend existing smart mix system
5. **Artist Radio** - Leverage Spotify recommendations

### Strategic Investments (High Complexity, High Impact)
Long-term features that differentiate the app:

1. **AI DJ & Smart Transitions** - Beat-synced mixing
2. **DJ Practice Mode** - Dual deck interface
3. **Immersive Visualizer Modes** - WebGL/GPU visuals
4. **Moments & Highlights** - Audio analysis for hooks/choruses

### Consider for Later (Low Priority or Very High Complexity)
Nice-to-have but not urgent:

1. Voice Control - High complexity, niche audience
2. Collab Sessions - Requires significant backend work
3. Podcast Support - Different content type entirely
4. Spatial Audio - Specialized use case

---

## Implementation Notes

### Database Fields to Add
```sql
-- For audio analysis features
ALTER TABLE songs ADD COLUMN bpm INTEGER;
ALTER TABLE songs ADD COLUMN key TEXT;
ALTER TABLE songs ADD COLUMN energy REAL;
ALTER TABLE songs ADD COLUMN valence REAL;
ALTER TABLE songs ADD COLUMN danceability REAL;
ALTER TABLE songs ADD COLUMN highlights TEXT; -- JSON array of timestamps
ALTER TABLE songs ADD COLUMN replay_gain REAL;

-- For listening stats
CREATE TABLE listening_sessions (
    id TEXT PRIMARY KEY,
    started_at INTEGER,
    ended_at INTEGER,
    songs_played INTEGER,
    total_duration REAL
);
```

### API Endpoints to Add
```
GET  /api/library/audit
POST /api/library/fix
GET  /api/songs/{id}/analysis
GET  /api/queue/autofill
GET  /api/radio/contextual
GET  /api/profile/taste-card
GET  /api/stats/listening
POST /api/clips
```

### Frontend Components to Create
- ✅ `SleepTimer.tsx` - Timer dialog with fade settings (DONE - includes useSleepTimer hook)
- ✅ `Stats.tsx` - Stats dashboard page (DONE - accessible via /stats route)
- `LibraryAudit.tsx` - Health check results display
- `WaveformDisplay.tsx` - Seekable waveform visualization
- `TasteCard.tsx` - Shareable listening profile card
