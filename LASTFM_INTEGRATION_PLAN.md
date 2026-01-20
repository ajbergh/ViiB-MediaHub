# Last.FM API Integration Plan

> **Purpose:** Technical specification for integrating Last.FM API as an alternative metadata enrichment source alongside AI-based enrichment.
>
> **Created:** 2025-12-31
>
> **Status:** Phase 1, 2 & 3 Complete - Scanner Integration Done
>
> **Last Updated:** 2025-12-31

---

## Implementation Status Summary

| Component | Status | Notes |
|-----------|--------|-------|
| **Backend: lastfm package** | ✅ Complete | types.go, client.go, cache.go, tag_mapper.go, enricher.go |
| **Backend: Database migrations** | ✅ Complete | 6 new columns in songs table, 2 new tables |
| **Backend: API handlers** | ✅ Complete | 10 endpoints in lastfm.go, routes registered |
| **Backend: Encrypted settings** | ✅ Complete | lastfm_shared_secret, lastfm_session_key auto-encrypted |
| **Backend: Enrichment source** | ✅ Complete | `enrichment_source` setting (ai/lastfm/hybrid) |
| **Backend: Scanner integration** | ✅ Complete | Scanner uses enrichment source setting, calls Last.FM enricher |
| **Frontend: api.ts functions** | ✅ Complete | 10 typed API functions + 8 interfaces (incl. EnrichmentSource) |
| **Frontend: Settings UI** | ✅ Complete | Full Last.FM config + Enrichment Source selection |
| **Frontend: First Launch wizard** | ✅ Complete | Step 5: Last.FM setup with enable toggle |
| **AI DJ separation** | ✅ Complete | AI DJ uses LLM regardless of enrichment source |
| **Real-time SSE logging** | ✅ Complete | library_updated events logged for debugging |
| **Scrobbling** | ❌ Not Started | Optional Phase 5 feature |

---

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [Project Goals](#project-goals)
3. [Last.FM API Overview](#lastfm-api-overview)
4. [Available Metadata](#available-metadata)
5. [Database Schema Changes](#database-schema-changes)
6. [Backend Implementation](#backend-implementation)
7. [Frontend Implementation](#frontend-implementation)
8. [Settings & Configuration](#settings--configuration)
9. [Integration with AI DJ](#integration-with-ai-dj)
10. [Rate Limiting & Caching](#rate-limiting--caching)
11. [Scrobbling Support](#scrobbling-support)
12. [Implementation Phases](#implementation-phases)
13. [Technical Considerations](#technical-considerations)

---

## Executive Summary

This document outlines the plan to integrate Last.FM API as a **secondary metadata enrichment option** that works alongside the existing AI-based (LLM) enrichment system. Users can choose between:

1. **AI-based enrichment** (Gemini, OpenAI, Anthropic, Ollama, X.AI) - Current system
2. **Last.FM API enrichment** - New option using community-sourced metadata
3. **Hybrid mode** - Combine both sources for maximum coverage

**Enrichment Source Selection** - The user can now select their preferred enrichment source in Settings. When Last.FM is selected, AI enrichment is disabled for automatic scans but AI DJ features still use the configured LLM provider.

### Key Benefits of Last.FM Integration

| Benefit | Description |
|---------|-------------|
| **No AI costs** | Free API with generous rate limits (5 calls/sec) |
| **Community data** | Tags from millions of users, reflecting real-world classifications |
| **Similar tracks/artists** | Built-in similarity data for playlist generation |
| **Global popularity** | Play counts and listener statistics |
| **MusicBrainz IDs** | Cross-reference with other music databases |
| **Rich metadata** | Wiki content, release dates, album art URLs |

---

## Project Goals

### Primary Goals

1. ✅ Add Last.FM API key configuration in Settings (Backend ✅, Frontend ✅)
2. ✅ Implement Last.FM enrichment as alternative to AI enrichment
3. ✅ Populate song database with Last.FM metadata
4. ✅ Store similar tracks/artists for playlist generation
5. ✅ Provide fallback when AI enrichment is unavailable
6. ✅ Add enrichment source selection (AI vs Last.FM vs Hybrid)

### Secondary Goals

1. 🔄 Optional scrobbling support (track user's listening)
2. 🔄 User-specific tag integration (if authenticated)
3. 🔄 Hybrid enrichment mode (AI + Last.FM combined)

---

## Last.FM API Overview

### API Basics

- **API Root URL:** `http://ws.audioscrobbler.com/2.0/`
- **Authentication:** API key required for read-only access; session key for write access
- **Response Formats:** JSON or XML
- **Rate Limits:** ~5 calls per second per API key
- **Cost:** Free for non-commercial use

### Recommended Go Client

**gobble-fm** - A comprehensive Go library for Last.FM API

```bash
go get github.com/twoscott/gobble-fm
```

**Key Features:**
- Comprehensive API coverage
- Typed parameter structs
- Typed response fields (no string conversions)
- Supports both authenticated and unauthenticated calls
- Active development (Go 1.25+ supported)

```go
import "github.com/twoscott/gobble-fm/api"

// Unauthenticated client (read-only)
fm := api.NewClientKeyOnly("API_KEY")

// Authenticated client (for scrobbling)
fm := api.NewClient("API_KEY", "SHARED_SECRET")
```

### Authentication Flows

| Flow | Use Case | Requires |
|------|----------|----------|
| **Key-only** | Read metadata, tags, similar tracks | API Key only |
| **Mobile Auth** | Scrobbling on desktop app | API Key + Shared Secret + Username/Password |
| **Desktop Auth** | Scrobbling with user approval | API Key + Shared Secret + Token flow |
| **Web Auth** | Web-based authentication | API Key + Shared Secret + Callback URL |

For ViiB MediaHub, **Mobile Auth** is recommended for the desktop app:
- User enters username/password once
- Session key stored encrypted in settings
- Enables scrobbling and personalized tags

### gobble-fm API Implementation Notes

**Important Type Details:**
- `AutoCorrect` field is `*bool` (use `boolPtr(true)` helper, not `true` directly)
- `Album`, `Wiki`, `Bio` are inline structs, not pointers (check `.Title != ""` not `!= nil`)
- Use `TrackSimilarParams` not `SimilarTracksParams`
- `SimilarArtist.Match` is the field name (not `MatchScore`)
- `client.SessionKey` is a field, not a method

```go
// Helper for *bool pointer
func boolPtr(b bool) *bool {
    return &b
}

// Correct usage
params := lastfm.TrackInfoParams{
    Artist:      artist,
    Track:       track,
    AutoCorrect: boolPtr(true), // Not just `true`
}
```

---

## Available Metadata

### Track-Level Metadata (`track.getInfo`, `track.getTopTags`, `track.getSimilar`)

| Field | Source | Use Case | AI DJ Value |
|-------|--------|----------|-------------|
| `name` | track.getInfo | Corrected track name | Name normalization |
| `artist.name` | track.getInfo | Corrected artist name | Name normalization |
| `album.title` | track.getInfo | Album association | Album grouping |
| `duration` | track.getInfo | Duration in ms | Playback verification |
| `listeners` | track.getInfo | Global listener count | Popularity scoring |
| `playcount` | track.getInfo | Global play count | Popularity scoring |
| `toptags` | track.getInfo | Top tags with counts | Genre detection |
| `toptags[].name` | track.getTopTags | Tag name (e.g., "rock", "energetic") | Genre/mood |
| `toptags[].count` | track.getTopTags | Tag weight (0-100) | Tag confidence |
| `similar[]` | track.getSimilar | Similar tracks with match score | Similar song discovery |
| `mbid` | track.getInfo | MusicBrainz Track ID | Cross-reference |
| `url` | track.getInfo | Last.FM track page | External link |
| `wiki.summary` | track.getInfo | Track description | Artist/song context |

### Artist-Level Metadata (`artist.getInfo`, `artist.getSimilar`, `artist.getTopTags`)

| Field | Source | Use Case | AI DJ Value |
|-------|--------|----------|-------------|
| `name` | artist.getInfo | Corrected artist name | Name normalization |
| `listeners` | artist.getInfo | Global listener count | Artist popularity |
| `playcount` | artist.getInfo | Global play count | Artist popularity |
| `tags[]` | artist.getInfo | Artist genre tags | Artist-level genres |
| `similar[]` | artist.getSimilar | Similar artists (0-1 match) | Artist discovery |
| `bio.summary` | artist.getInfo | Artist biography | Context for prompts |
| `mbid` | artist.getInfo | MusicBrainz Artist ID | Cross-reference |

### Album-Level Metadata (`album.getInfo`, `album.getTopTags`)

| Field | Source | Use Case | AI DJ Value |
|-------|--------|----------|-------------|
| `name` | album.getInfo | Album title | Album metadata |
| `artist` | album.getInfo | Album artist | Album artist |
| `releasedate` | album.getInfo | Release date string | Year extraction |
| `listeners` | album.getInfo | Global listener count | Album popularity |
| `playcount` | album.getInfo | Global play count | Album popularity |
| `toptags[]` | album.getInfo | Album genre tags | Album genres |
| `tracks[]` | album.getInfo | Track listing with durations | Album structure |
| `mbid` | album.getInfo | MusicBrainz Album ID | Cross-reference |
| `image[]` | album.getInfo | Album art URLs (various sizes) | Cover art fallback |

### Tag Metadata (`tag.getInfo`, `tag.getSimilar`)

| Field | Source | Use Case | AI DJ Value |
|-------|--------|----------|-------------|
| `name` | tag.getInfo | Tag name | Genre/mood name |
| `reach` | tag.getInfo | Number of users using tag | Tag popularity |
| `taggings` | tag.getInfo | Total tag applications | Tag significance |
| `similar[]` | tag.getSimilar | Related tags | Genre relationships |
| `wiki.summary` | tag.getInfo | Tag description | Context |

---

## Mapping Last.FM Tags to AI DJ Fields

Last.FM tags are user-generated and include genres, moods, energy levels, and more. We can map these to existing AI DJ fields:

### Genre Extraction

```go
// Top N tags with count > 50 treated as genres
// Filter out mood/energy/tempo tags
genreTags := []string{"rock", "pop", "electronic", "jazz", "hip-hop", "classical", ...}
```

### Mood Detection from Tags

| Last.FM Tags | Mood Value |
|--------------|------------|
| "happy", "uplifting", "feel good", "positive" | "happy" |
| "sad", "melancholic", "depressing", "emotional" | "sad" |
| "chill", "relaxing", "calm", "mellow" | "calm" |
| "energetic", "upbeat", "driving", "anthemic" | "energetic" |
| "dark", "brooding", "atmospheric", "moody" | "dark" |
| "romantic", "love", "sensual" | "romantic" |
| "aggressive", "angry", "intense" | "aggressive" |
| "dreamy", "ethereal", "ambient" | "dreamy" |

### Energy Detection from Tags

| Last.FM Tags | Energy Value |
|--------------|--------------|
| "high energy", "energetic", "driving", "anthemic", "party" | "high" |
| "medium energy", "groovy", "uptempo" | "medium" |
| "low energy", "chill", "calm", "relaxing", "ambient" | "low" |

### Tempo Detection from Tags

| Last.FM Tags | Tempo Value |
|--------------|-------------|
| "fast", "upbeat", "uptempo", "high tempo" | "fast" |
| "medium tempo", "mid-tempo", "groovy" | "medium" |
| "slow", "downtempo", "slow tempo", "ballad" | "slow" |

### Instrumental Detection from Tags

| Last.FM Tags | Instrumental Value |
|--------------|-------------------|
| "instrumental", "no vocals", "post-rock" (often instrumental) | `true` |
| "vocal", "singer-songwriter" | `false` |

---

## Database Schema Changes

### New Columns in `songs` Table

```sql
-- Last.FM enrichment metadata
ALTER TABLE songs ADD COLUMN lastfm_listeners INTEGER;          -- Global listener count
ALTER TABLE songs ADD COLUMN lastfm_playcount INTEGER;          -- Global play count
ALTER TABLE songs ADD COLUMN lastfm_tags TEXT;                  -- JSON array of {name, count}
ALTER TABLE songs ADD COLUMN lastfm_url TEXT;                   -- Last.FM track page URL
ALTER TABLE songs ADD COLUMN lastfm_mbid TEXT;                  -- MusicBrainz track ID
ALTER TABLE songs ADD COLUMN lastfm_enriched_at INTEGER;        -- Timestamp of Last.FM enrichment
ALTER TABLE songs ADD COLUMN lastfm_similar_tracks TEXT;        -- JSON array of similar track IDs
```

### New Table: `lastfm_similar_tracks`

```sql
-- Similar tracks relationship table (for tracks in our library)
CREATE TABLE IF NOT EXISTS lastfm_similar_tracks (
    song_id TEXT NOT NULL,                -- Our song ID
    similar_artist TEXT NOT NULL,         -- Similar track artist
    similar_track TEXT NOT NULL,          -- Similar track title
    match_score REAL NOT NULL,            -- Match score 0-1
    similar_song_id TEXT,                 -- Our song ID if we have this track
    FOREIGN KEY (song_id) REFERENCES songs(id) ON DELETE CASCADE,
    PRIMARY KEY (song_id, similar_artist, similar_track)
);

CREATE INDEX idx_similar_tracks_song ON lastfm_similar_tracks(song_id);
CREATE INDEX idx_similar_tracks_similar_song ON lastfm_similar_tracks(similar_song_id);
```

### New Table: `lastfm_similar_artists`

```sql
-- Similar artists relationship table
CREATE TABLE IF NOT EXISTS lastfm_similar_artists (
    artist_name TEXT NOT NULL,            -- Artist name (normalized)
    similar_artist TEXT NOT NULL,         -- Similar artist name
    match_score REAL NOT NULL,            -- Match score 0-1
    PRIMARY KEY (artist_name, similar_artist)
);

CREATE INDEX idx_similar_artists ON lastfm_similar_artists(artist_name);
```

### Enhanced `artist_metadata` Table

```sql
-- Add Last.FM fields to existing artist_metadata table
ALTER TABLE artist_metadata ADD COLUMN lastfm_listeners INTEGER;
ALTER TABLE artist_metadata ADD COLUMN lastfm_playcount INTEGER;
ALTER TABLE artist_metadata ADD COLUMN lastfm_tags TEXT;        -- JSON array
ALTER TABLE artist_metadata ADD COLUMN lastfm_bio TEXT;         -- Bio summary
ALTER TABLE artist_metadata ADD COLUMN lastfm_url TEXT;
ALTER TABLE artist_metadata ADD COLUMN lastfm_mbid TEXT;
ALTER TABLE artist_metadata ADD COLUMN lastfm_enriched_at INTEGER;
```

### Settings Table (Encrypted)

```sql
-- Last.FM credentials stored encrypted in settings table
-- Keys: lastfm_api_key, lastfm_shared_secret, lastfm_session_key, lastfm_username
-- Encrypted using existing crypto.Encrypt/Decrypt
```

---

## Backend Implementation

### New Package: `internal/lastfm`

```
backend/internal/lastfm/
├── client.go           # gobble-fm wrapper with caching
├── enricher.go         # Batch enrichment logic
├── tag_mapper.go       # Tag to mood/energy/tempo mapping
├── similar.go          # Similar track/artist processing
├── scrobbler.go        # Optional scrobbling support
└── types.go            # Internal types and constants
```

### Client Implementation (`client.go`)

```go
package lastfm

import (
    "github.com/twoscott/gobble-fm/api"
    "github.com/twoscott/gobble-fm/lastfm"
    "github.com/twoscott/gobble-fm/session"
)

type Client struct {
    apiKey       string
    sharedSecret string
    api          *api.Client
    session      *session.Client  // For authenticated calls
    rateLimiter  *rate.Limiter    // 5 req/sec
    cache        *EnrichmentCache // LRU cache
}

func NewClient(apiKey, sharedSecret string) *Client {
    return &Client{
        apiKey:       apiKey,
        sharedSecret: sharedSecret,
        api:          api.NewClient(apiKey, sharedSecret),
        rateLimiter:  rate.NewLimiter(rate.Limit(5), 1), // 5 req/sec
        cache:        NewEnrichmentCache(1000),          // 1000 entries
    }
}

// GetTrackInfo fetches track metadata with caching
func (c *Client) GetTrackInfo(artist, track string) (*TrackInfo, error) {
    cacheKey := fmt.Sprintf("track:%s:%s", artist, track)
    if cached, ok := c.cache.Get(cacheKey); ok {
        return cached.(*TrackInfo), nil
    }
    
    c.rateLimiter.Wait(context.Background())
    
    params := lastfm.TrackInfoParams{
        Artist:      artist,
        Track:       track,
        Autocorrect: true,
    }
    
    res, err := c.api.Track.Info(params)
    if err != nil {
        return nil, err
    }
    
    info := mapTrackInfo(res)
    c.cache.Set(cacheKey, info)
    return info, nil
}

// GetSimilarTracks fetches tracks similar to this one
func (c *Client) GetSimilarTracks(artist, track string, limit int) ([]SimilarTrack, error) {
    // Implementation with caching and rate limiting
}

// GetArtistInfo fetches artist metadata with caching
func (c *Client) GetArtistInfo(artist string) (*ArtistInfo, error) {
    // Implementation with caching and rate limiting
}

// GetSimilarArtists fetches similar artists
func (c *Client) GetSimilarArtists(artist string, limit int) ([]SimilarArtist, error) {
    // Implementation with caching and rate limiting
}
```

### Enricher Implementation (`enricher.go`)

```go
package lastfm

import "github.com/ajbergh/viib-mediahub/internal/db"

type Enricher struct {
    client    *Client
    db        *db.DB
    tagMapper *TagMapper
}

// EnrichSongs enriches a batch of songs with Last.FM metadata
func (e *Enricher) EnrichSongs(songs []db.Song, options EnrichOptions) (*EnrichResult, error) {
    result := &EnrichResult{}
    
    for _, song := range songs {
        // Skip if already enriched and not forcing
        if song.LastFMEnrichedAt > 0 && !options.Force {
            result.Skipped++
            continue
        }
        
        // Fetch track info
        trackInfo, err := e.client.GetTrackInfo(song.Artist, song.Title)
        if err != nil {
            result.Errors++
            continue
        }
        
        // Map tags to mood/energy/tempo
        enrichment := e.tagMapper.MapTags(trackInfo.TopTags)
        
        // Update song in database
        update := db.SongUpdate{
            ID:               song.ID,
            Genres:           enrichment.Genres,
            Mood:             enrichment.Mood,
            Energy:           enrichment.Energy,
            Tempo:            enrichment.Tempo,
            LastFMListeners:  trackInfo.Listeners,
            LastFMPlaycount:  trackInfo.Playcount,
            LastFMTags:       trackInfo.TopTags, // JSON encoded
            LastFMEnrichedAt: time.Now().Unix(),
        }
        
        if err := e.db.UpdateSongLastFM(song.ID, update); err != nil {
            result.Errors++
            continue
        }
        
        result.Enriched++
    }
    
    return result, nil
}

type EnrichOptions struct {
    Force          bool  // Re-enrich even if already enriched
    FetchSimilar   bool  // Also fetch similar tracks
    FetchArtist    bool  // Also fetch artist info
    BatchSize      int   // Songs per batch
    MaxConcurrency int   // Concurrent API calls
}

type EnrichResult struct {
    Enriched int
    Skipped  int
    Errors   int
}
```

### Tag Mapper (`tag_mapper.go`)

```go
package lastfm

type TagMapper struct {
    genreTags       map[string]bool
    moodMappings    map[string]string   // tag -> mood
    energyMappings  map[string]string   // tag -> energy
    tempoMappings   map[string]string   // tag -> tempo
    instrumentalTags map[string]bool
}

func NewTagMapper() *TagMapper {
    return &TagMapper{
        genreTags: map[string]bool{
            "rock": true, "pop": true, "electronic": true, "jazz": true,
            "hip-hop": true, "classical": true, "r&b": true, "metal": true,
            "folk": true, "country": true, "blues": true, "punk": true,
            "indie": true, "alternative": true, "soul": true, "funk": true,
            "reggae": true, "disco": true, "techno": true, "house": true,
            // ... more genres
        },
        moodMappings: map[string]string{
            "happy": "happy", "uplifting": "happy", "feel good": "happy",
            "sad": "sad", "melancholic": "sad", "emotional": "sad",
            "chill": "calm", "relaxing": "calm", "mellow": "calm",
            "energetic": "energetic", "anthemic": "energetic",
            "dark": "dark", "brooding": "dark", "atmospheric": "dark",
            "romantic": "romantic", "love": "romantic",
            "aggressive": "aggressive", "angry": "aggressive",
            "dreamy": "dreamy", "ethereal": "dreamy",
            // ... more mappings
        },
        // ... energy and tempo mappings
    }
}

func (m *TagMapper) MapTags(tags []TagWithCount) *TagEnrichment {
    enrichment := &TagEnrichment{}
    
    // Sort tags by count (highest first)
    sort.Slice(tags, func(i, j int) bool {
        return tags[i].Count > tags[j].Count
    })
    
    // Extract genres (tags matching genre list with count > threshold)
    for _, tag := range tags {
        normalizedTag := strings.ToLower(tag.Name)
        if m.genreTags[normalizedTag] && tag.Count >= 30 {
            enrichment.Genres = append(enrichment.Genres, tag.Name)
        }
    }
    
    // Detect mood from highest-count mood tag
    for _, tag := range tags {
        if mood, ok := m.moodMappings[strings.ToLower(tag.Name)]; ok {
            enrichment.Mood = mood
            break
        }
    }
    
    // Similar for energy and tempo
    // ...
    
    return enrichment
}
```

### API Handlers (`internal/api/lastfm.go`)

```go
package api

// POST /api/lastfm/settings - Save Last.FM credentials
func (s *Server) handleSaveLastFMSettings(w http.ResponseWriter, r *http.Request) {
    var req struct {
        APIKey       string `json:"apiKey"`
        SharedSecret string `json:"sharedSecret"`
        Username     string `json:"username,omitempty"`
        Password     string `json:"password,omitempty"`
    }
    
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        respondError(w, http.StatusBadRequest, "Invalid request")
        return
    }
    
    // Store encrypted
    if err := s.db.SetEncryptedSetting("lastfm_api_key", req.APIKey); err != nil {
        respondError(w, http.StatusInternalServerError, "Failed to save")
        return
    }
    if err := s.db.SetEncryptedSetting("lastfm_shared_secret", req.SharedSecret); err != nil {
        respondError(w, http.StatusInternalServerError, "Failed to save")
        return
    }
    
    // If username/password provided, authenticate and get session key
    if req.Username != "" && req.Password != "" {
        sessionKey, err := s.lastfmClient.Authenticate(req.Username, req.Password)
        if err != nil {
            respondError(w, http.StatusUnauthorized, "Authentication failed")
            return
        }
        s.db.SetEncryptedSetting("lastfm_session_key", sessionKey)
        s.db.SetSetting("lastfm_username", req.Username)
    }
    
    respondJSON(w, map[string]string{"status": "ok"})
}

// GET /api/lastfm/settings - Get Last.FM settings (masked secrets)
func (s *Server) handleGetLastFMSettings(w http.ResponseWriter, r *http.Request) {
    apiKey, _ := s.db.GetEncryptedSetting("lastfm_api_key")
    username, _ := s.db.GetSetting("lastfm_username")
    
    respondJSON(w, map[string]interface{}{
        "configured": apiKey != "",
        "username":   username,
        "scrobbling": s.db.GetSetting("lastfm_scrobbling_enabled") == "true",
    })
}

// POST /api/lastfm/test - Test Last.FM connection
func (s *Server) handleTestLastFM(w http.ResponseWriter, r *http.Request) {
    // Attempt to fetch a known track to verify API key works
    _, err := s.lastfmClient.GetTrackInfo("Cher", "Believe")
    if err != nil {
        respondJSON(w, map[string]interface{}{
            "success": false,
            "error":   err.Error(),
        })
        return
    }
    
    respondJSON(w, map[string]interface{}{
        "success": true,
        "message": "Last.FM API connection successful",
    })
}

// POST /api/lastfm/enrich - Trigger Last.FM enrichment
func (s *Server) handleLastFMEnrich(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Force       bool `json:"force"`
        FetchSimilar bool `json:"fetchSimilar"`
    }
    json.NewDecoder(r.Body).Decode(&req)
    
    // Start enrichment in background
    go s.runLastFMEnrichment(req.Force, req.FetchSimilar)
    
    respondJSON(w, map[string]string{"status": "started"})
}
```

---

## Frontend Implementation

### Settings UI Component

Add a new section in `pages/Settings.tsx` under "Library Intelligence":

```tsx
// Last.FM Settings Section
const [lastfmApiKey, setLastfmApiKey] = useState('');
const [lastfmSecret, setLastfmSecret] = useState('');
const [lastfmUsername, setLastfmUsername] = useState('');
const [lastfmPassword, setLastfmPassword] = useState('');
const [lastfmConfigured, setLastfmConfigured] = useState(false);
const [lastfmTestStatus, setLastfmTestStatus] = useState<'idle' | 'testing' | 'success' | 'error'>('idle');
const [lastfmSaveStatus, setLastfmSaveStatus] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle');
const [scrobblingEnabled, setScrobblingEnabled] = useState(false);

// Load Last.FM settings on mount
useEffect(() => {
    const loadLastFMSettings = async () => {
        try {
            const settings = await api.getLastFMSettings();
            setLastfmConfigured(settings.configured);
            setLastfmUsername(settings.username || '');
            setScrobblingEnabled(settings.scrobbling);
        } catch (e) {
            console.error('Failed to load Last.FM settings:', e);
        }
    };
    if (backendAvailable) {
        loadLastFMSettings();
    }
}, [backendAvailable]);

const handleSaveLastFM = async () => {
    setLastfmSaveStatus('saving');
    try {
        await api.saveLastFMSettings({
            apiKey: lastfmApiKey,
            sharedSecret: lastfmSecret,
            username: lastfmUsername,
            password: lastfmPassword,
        });
        setLastfmSaveStatus('saved');
        setLastfmConfigured(true);
        setTimeout(() => setLastfmSaveStatus('idle'), 3000);
    } catch (e) {
        setLastfmSaveStatus('error');
        addLog('error', 'Failed to save Last.FM settings', e);
    }
};

const handleTestLastFM = async () => {
    setLastfmTestStatus('testing');
    try {
        const result = await api.testLastFM();
        setLastfmTestStatus(result.success ? 'success' : 'error');
    } catch (e) {
        setLastfmTestStatus('error');
    }
};
```

### UI Mockup

```
┌─────────────────────────────────────────────────────────────────┐
│ 🎵 Last.FM Integration                                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│ Last.FM provides community-sourced tags, similar tracks, and   │
│ listening statistics to enrich your library metadata.          │
│                                                                 │
│ ┌─ API Credentials ──────────────────────────────────────────┐ │
│ │ API Key         [••••••••••••••••••••••••••••••]           │ │
│ │ Shared Secret   [••••••••••••••••••••••••••••••]           │ │
│ │                                                             │ │
│ │ [Test Connection]  ✓ Connected                             │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                                                                 │
│ ┌─ Scrobbling (Optional) ────────────────────────────────────┐ │
│ │ Username        [________________________]                  │ │
│ │ Password        [••••••••••••••••••••••]                   │ │
│ │                                                             │ │
│ │ [x] Enable scrobbling (track listening history)            │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                                                                 │
│                              [Save Last.FM Settings]            │
│                                                                 │
├─────────────────────────────────────────────────────────────────┤
│ 📊 Enrichment Options                                           │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│ Metadata Enrichment Source:                                     │
│ ( ) AI-based (uses configured LLM provider)                    │
│ (•) Last.FM API (community tags and metadata)                  │
│ ( ) Hybrid (Last.FM first, AI for missing data)                │
│                                                                 │
│ [x] Fetch similar tracks for playlist generation                │
│ [x] Fetch similar artists for discovery                         │
│ [ ] Force re-enrich already processed songs                     │
│                                                                 │
│ [▶ Start Last.FM Enrichment]                                   │
│ Progress: 1,234 / 5,678 songs enriched                         │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## Settings & Configuration

### Settings Keys (in `settings` table)

| Key | Type | Description | Encrypted |
|-----|------|-------------|-----------|
| `lastfm_api_key` | string | Last.FM API key | ✅ Yes |
| `lastfm_shared_secret` | string | Last.FM shared secret | ✅ Yes |
| `lastfm_session_key` | string | Authenticated session key | ✅ Yes |
| `lastfm_username` | string | Last.FM username | No |
| `lastfm_scrobbling_enabled` | bool | Enable scrobbling | No |
| `enrichment_source` | string | "ai", "lastfm", or "hybrid" | No |
| `lastfm_fetch_similar` | bool | Fetch similar tracks | No |
| `lastfm_min_tag_count` | int | Minimum tag count threshold | No |

### User Configuration (Callback URL)

User has configured callback URL as: `http://172.0.0.1/callback`

**Note:** This appears to be a localhost URL. For web authentication flow, ensure:
1. Backend listens on the correct interface (127.0.0.1 or 0.0.0.0)
2. Callback route is registered: `/callback`
3. If using Wails, webview navigation may need special handling

For desktop app, **Mobile Auth** is simpler (no callback needed):
- User enters username/password
- `auth.getMobileSession` called directly
- Session key returned and stored

---

## Integration with AI DJ

### Enhanced Song Scoring

Last.FM data provides additional signals for the AI DJ scoring algorithm:

```go
// In backend/internal/dj/scoring.go

func (s *Scorer) ScoreSong(song db.Song, phase DJPhase, ctx ScoreContext) float64 {
    score := 0.0
    
    // Existing scoring factors...
    score += s.scoreGenreMatch(song, phase)
    score += s.scoreMoodMatch(song, phase)
    score += s.scoreEnergyMatch(song, phase)
    
    // NEW: Last.FM-based scoring
    if song.LastFMListeners > 0 {
        // Popularity boost (normalized 0-1)
        popularityScore := math.Log10(float64(song.LastFMListeners)) / 8.0 // log10(100M) ≈ 8
        score += popularityScore * ctx.PopularityWeight
    }
    
    // Similar track bonus if previous song has similar tracks containing this one
    if ctx.PreviousSong != nil && s.isSimilarTrack(ctx.PreviousSong, song) {
        score += 0.2 // Similarity bonus
    }
    
    return score
}

func (s *Scorer) isSimilarTrack(prev, curr db.Song) bool {
    // Check lastfm_similar_tracks table
    return s.db.AreSongsSimilar(prev.ID, curr.ID)
}
```

### Similar Track Discovery for Playlists

```go
// In backend/internal/api/smart_playlist.go

func (s *Server) expandWithSimilarTracks(songs []db.Song, targetCount int) []db.Song {
    if !s.lastfmEnabled() {
        return songs
    }
    
    seenIDs := make(map[string]bool)
    for _, song := range songs {
        seenIDs[song.ID] = true
    }
    
    for _, song := range songs {
        if len(songs) >= targetCount {
            break
        }
        
        // Get similar tracks from our library
        similar, _ := s.db.GetSimilarTracksInLibrary(song.ID, 5)
        for _, sim := range similar {
            if !seenIDs[sim.ID] {
                songs = append(songs, sim)
                seenIDs[sim.ID] = true
            }
        }
    }
    
    return songs
}
```

---

## Rate Limiting & Caching

### Rate Limiter

```go
import "golang.org/x/time/rate"

// 5 requests per second limit
limiter := rate.NewLimiter(rate.Limit(5), 1)

// Before each API call
limiter.Wait(context.Background())
```

### Caching Strategy

| Cache Level | TTL | Size | Invalidation |
|-------------|-----|------|--------------|
| Track info | 24h | 10,000 entries | LRU eviction |
| Artist info | 24h | 1,000 entries | LRU eviction |
| Similar tracks | 7d | 5,000 entries | LRU eviction |
| Tags | 24h | 10,000 entries | LRU eviction |

```go
type EnrichmentCache struct {
    trackInfo    *lru.Cache // Track info responses
    artistInfo   *lru.Cache // Artist info responses
    similarTracks *lru.Cache // Similar tracks responses
    ttl          time.Duration
}
```

---

## Scrobbling Support

### When to Scrobble

Per Last.FM rules:
1. Track must be played for at least 50% of its duration OR 4 minutes, whichever comes first
2. Track must be at least 30 seconds long
3. Scrobble should be submitted at the completion point

### Implementation

```go
// In backend/internal/lastfm/scrobbler.go

type Scrobbler struct {
    client     *session.Client
    enabled    bool
    minDuration time.Duration // 30 seconds
}

func (s *Scrobbler) ShouldScrobble(song db.Song, playedDuration time.Duration) bool {
    if !s.enabled {
        return false
    }
    
    // Track must be at least 30 seconds
    if song.Duration < 30 {
        return false
    }
    
    // Must play 50% or 4 minutes, whichever is less
    halfDuration := time.Duration(song.Duration/2) * time.Second
    threshold := halfDuration
    if threshold > 4*time.Minute {
        threshold = 4 * time.Minute
    }
    
    return playedDuration >= threshold
}

func (s *Scrobbler) Scrobble(song db.Song, timestamp time.Time) error {
    params := lastfm.ScrobbleParams{
        Artist:    song.Artist,
        Track:     song.Title,
        Album:     song.Album,
        Timestamp: timestamp,
    }
    
    _, err := s.client.Track.Scrobble(params)
    return err
}

func (s *Scrobbler) UpdateNowPlaying(song db.Song) error {
    params := lastfm.NowPlayingParams{
        Artist: song.Artist,
        Track:  song.Title,
        Album:  song.Album,
    }
    
    _, err := s.client.Track.UpdateNowPlaying(params)
    return err
}
```

### Frontend Integration

```tsx
// In hooks/useAudioPlayer.ts

// When song starts
if (scrobblingEnabled) {
    api.updateNowPlaying(currentSong.id);
}

// When song completes or passes threshold
if (scrobblingEnabled && playedDuration >= scrobbleThreshold) {
    api.scrobble(currentSong.id, startTime);
}
```

---

## Implementation Phases

### Phase 1: Core Infrastructure (Week 1) ✅ COMPLETE

- [x] Create `internal/lastfm` package with gobble-fm wrapper
  - `types.go` - Core types (TrackInfo, ArtistInfo, TagEnrichment, etc.)
  - `client.go` - gobble-fm wrapper with rate limiting and caching
  - `cache.go` - Thread-safe LRU cache for API responses
  - `tag_mapper.go` - Map Last.FM tags to mood/energy/tempo/genres
  - `enricher.go` - Batch enrichment with concurrency control
- [x] Add database schema migrations for Last.FM fields
  - 6 columns added to songs table (listeners, playcount, tags, url, mbid, enriched_at)
  - `lastfm_similar_tracks` table for track similarity
  - `lastfm_similar_artists` table for artist similarity
- [x] Implement rate limiter and basic caching
  - Rate limiter: 5 req/sec with burst of 1
  - LRU cache: 10,000 entries, 24h TTL
- [x] Add API key settings storage (encrypted)
  - `lastfm_shared_secret` and `lastfm_session_key` in crypto.sensitiveKeys
- [x] Create Settings UI for Last.FM credentials ✅
- [x] Implement connection test endpoint (`/api/lastfm/test`)

**Files Created:**
- `backend/internal/lastfm/types.go`
- `backend/internal/lastfm/client.go`
- `backend/internal/lastfm/cache.go`
- `backend/internal/lastfm/tag_mapper.go`
- `backend/internal/lastfm/enricher.go`
- `backend/internal/api/lastfm.go`

### Phase 2: Basic Enrichment (Week 2) ✅ COMPLETE

- [x] Implement track.getInfo fetching and parsing
- [x] Implement tag mapping to mood/energy/tempo
- [x] Create batch enrichment endpoint (`POST /api/lastfm/enrich/songs`)
- [x] Add enrichment progress UI with status display ✅
- [x] Store enrichment results in database (`UpdateSongLastFM()`)
- [x] Add enrichment source selection in Settings UI ✅
- [x] Add Last.FM option in First Launch wizard (Step 5) ✅

**API Endpoints Created:**
- `GET /api/lastfm/settings` - Get Last.FM configuration
- `POST /api/lastfm/settings` - Save Last.FM credentials
- `POST /api/lastfm/test` - Test connection
- `POST /api/lastfm/authenticate` - Authenticate for scrobbling
- `GET /api/lastfm/status` - Get connection status and stats
- `POST /api/lastfm/enrich/songs` - Trigger song enrichment
- `POST /api/lastfm/enrich/artists` - Trigger artist enrichment
- `GET /api/lastfm/track` - Get track info (artist, track params)
- `GET /api/lastfm/similar` - Get similar tracks

**Frontend Files Modified:**
- `services/api.ts` - 10 API functions + 7 interfaces
- `pages/Settings.tsx` - Full Last.FM configuration section
- `components/FirstLaunchDialog.tsx` - Step 5: Last.FM setup

### Phase 3: Similar Tracks (Week 3) ✅ BACKEND COMPLETE

- [x] Implement track.getSimilar fetching
- [x] Create similar tracks database table (`lastfm_similar_tracks`)
- [x] Match similar tracks to local library (`FindSongByArtistAndTitle()`)
- [x] Implement artist.getSimilar
- [x] Create similar artists database table (`lastfm_similar_artists`)
- [ ] **Add UI to view similar tracks/artists** ← FUTURE ENHANCEMENT

### Phase 4: AI DJ Integration (Week 4) ❌ NOT STARTED

- [ ] Add Last.FM popularity scoring to DJ algorithm
- [ ] Implement similar track bonus in scoring
- [ ] Add similar track expansion for playlists
- [ ] Test hybrid AI + Last.FM enrichment
- [ ] Performance optimization and caching tuning

### Phase 5: Scrobbling (Optional - Week 5) 🔄 BACKEND READY

- [x] Implement Mobile Auth flow (authentication method in client.go)
- [ ] Add scrobbling logic in playback hooks
- [ ] Implement Now Playing updates
- [ ] Add scrobbling toggle in settings
- [ ] Test scrobbling with Last.FM account

---

## Frontend Implementation Status

> **All frontend tasks are now complete.** See implementation details below.

### Completed Components

#### 1. services/api.ts - Last.FM API Functions ✅

10 typed API functions added:
- `getLastFMSettings()` - Retrieve current settings
- `saveLastFMSettings(settings)` - Save API key, secret, enabled state
- `testLastFMConnection()` - Test API connectivity
- `authenticateLastFM(username, password)` - Get session key for scrobbling
- `getLastFMStatus()` - Check configuration and connection status
- `triggerLastFMEnrichment(options)` - Start background enrichment
- `triggerLastFMArtistEnrichment(options)` - Enrich artist metadata
- `getLastFMTrackInfo(artist, track)` - Get track details
- `getLastFMSimilarTracks(artist, track, limit)` - Get similar tracks

7 TypeScript interfaces:
- `LastFMSettings`, `LastFMSettingsRequest`, `LastFMStatus`
- `LastFMTag`, `LastFMTrackInfo`, `LastFMSimilarTrack`

#### 2. pages/Settings.tsx - Last.FM Configuration UI ✅

Full settings section under "Library Intelligence":
- API Key input (masked when saved)
- Shared Secret input (masked)
- Enable/disable toggle
- Test Connection button with status feedback
- Save Settings button with status
- Authentication section for scrobbling (username/password)
- Trigger enrichment button with progress display

#### 3. components/FirstLaunchDialog.tsx - Step 5: Last.FM Setup ✅

New wizard step added:
- Description of Last.FM benefits
- Enable toggle for enrichment during scan
- Optional API Key and Shared Secret inputs
- Test Connection button (shown when API key provided)
- Skip and Continue navigation
- Progress bar updated to 5 steps (was 4)

### UI Implementation

The Settings UI includes:
- Connection status indicator (green/yellow/red)
- API credentials section with masked inputs
- Enable toggle with description
- Save and Test buttons with loading states
- Authentication section for scrobbling features
- Enrichment trigger with progress display

The First Launch wizard includes:
- Step 5 positioned after AI Provider setup
- Clear explanation of Last.FM benefits
- Emphasis that API keys are optional
- Enable toggle for automatic enrichment
- Skip option to proceed without configuration

---

## Technical Considerations

### Error Handling

```go
// Handle Last.FM API errors gracefully
var lastfmErr *api.LastFMError
if errors.As(err, &lastfmErr) {
    switch lastfmErr.Code {
    case api.ErrInvalidAPIKey:
        return fmt.Errorf("invalid Last.FM API key")
    case api.ErrRateLimitExceeded:
        time.Sleep(time.Minute) // Back off
        return retry()
    case api.ErrTrackNotFound:
        return nil // Track not in Last.FM database
    }
}
```

### Graceful Degradation

If Last.FM API is unavailable:
1. Log warning but continue
2. Fall back to AI enrichment if configured
3. Use cached data if available
4. Skip enrichment for affected songs

### Privacy Considerations

- API key and session key stored encrypted
- Username stored in plaintext (not sensitive)
- Password never stored (only used to get session key)
- Scrobbling is opt-in
- Similar tracks data is public music metadata

### Performance Optimization

1. **Batch by artist/album**: Group songs to minimize API calls
2. **Prioritize popular artists**: More likely to have data
3. **Skip instrumental/unknown**: Lower match rates
4. **Parallel fetching**: Use semaphore for concurrency control
5. **Incremental enrichment**: Only process new/changed songs

---

## Appendix: Full API Method Reference

### Methods Used by ViiB MediaHub

| Method | Purpose | Auth Required |
|--------|---------|---------------|
| `track.getInfo` | Track metadata, tags, wiki | No |
| `track.getTopTags` | Top user tags for track | No |
| `track.getSimilar` | Similar tracks | No |
| `artist.getInfo` | Artist metadata, bio | No |
| `artist.getTopTags` | Top tags for artist | No |
| `artist.getSimilar` | Similar artists | No |
| `album.getInfo` | Album metadata, tracks | No |
| `album.getTopTags` | Top tags for album | No |
| `tag.getInfo` | Tag description, stats | No |
| `auth.getMobileSession` | Get session key | Yes (Secret) |
| `track.scrobble` | Submit scrobble | Yes (Session) |
| `track.updateNowPlaying` | Update now playing | Yes (Session) |

### Response Format (JSON)

Set `format=json` parameter for JSON responses instead of XML.

---

## References

- [Last.FM API Documentation](https://www.last.fm/api/intro)
- [gobble-fm Go Package](https://github.com/twoscott/gobble-fm)
- [gobble-fm GoDoc](https://pkg.go.dev/github.com/twoscott/gobble-fm)
- [Last.FM Terms of Service](https://www.last.fm/api/tos)
- [MusicBrainz](https://musicbrainz.org/) - Cross-reference database
