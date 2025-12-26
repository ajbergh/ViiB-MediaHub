# Likes Feature Implementation

## Overview

Add the ability to "like" songs in ViiB MediaHub. Users can click heart icons to mark songs as favorites, and these likes will be persisted, synced, and integrated into smart playlists and discovery features.

## Current State

### Existing Heart Buttons (Non-functional)

| Location | File | Line | Usage |
|----------|------|------|-------|
| Now Playing | [NowPlaying.tsx](components/NowPlaying.tsx#L317) | 317 | Heart button next to song metadata |
| Album Detail | [AlbumDetail.tsx](pages/AlbumDetail.tsx#L141) | 141 | Heart button for album-level like |
| Smart Playlists | [SmartPlaylists.tsx](pages/SmartPlaylists.tsx#L307) | 307 | Heart icon indicator |

### Current Song Schema (Backend)

From [db.go](backend/internal/db/db.go#L77-L101):
```go
type Song struct {
    ID             string   `json:"id"`
    Title          string   `json:"title"`
    Artist         string   `json:"artist"`
    // ... other fields
    PlayCount      int      `json:"playCount,omitempty"`
    LastPlayed     int64    `json:"lastPlayed,omitempty"`
    SkipCount      int      `json:"skipCount,omitempty"`
    // ... AI fields (mood, energy, etc.)
}
```

**Missing**: No `liked` or `likedAt` field exists.

### Current Song Type (Frontend)

From [types.ts](types.ts#L29-L63):
```typescript
export interface Song {
  id: string;
  title: string;
  // ... other fields
  playCount?: number;
  lastPlayed?: number;
  skipCount?: number;
  // ... AI fields
}
```

**Missing**: No `liked` or `likedAt` field exists.

---

## Implementation Plan

### Phase 1: Database Schema

#### 1.1 Add Fields to Songs Table

**File**: [backend/internal/db/db.go](backend/internal/db/db.go)

Add to the `songs` table migration:
```sql
ALTER TABLE songs ADD COLUMN liked INTEGER DEFAULT 0;
ALTER TABLE songs ADD COLUMN liked_at INTEGER;
```

Add to the `Song` struct:
```go
type Song struct {
    // ... existing fields
    Liked     bool   `json:"liked,omitempty"`
    LikedAt   int64  `json:"likedAt,omitempty"`
}
```

#### 1.2 Add Database Methods

**New methods to add**:
```go
// ToggleLike toggles the liked status of a song
func (d *DB) ToggleLike(songID string) (bool, error)

// SetLike explicitly sets the liked status
func (d *DB) SetLike(songID string, liked bool) error

// GetLikedSongs returns all liked songs
func (d *DB) GetLikedSongs() ([]*Song, error)

// GetLikedSongIDs returns just the IDs of liked songs (for quick lookups)
func (d *DB) GetLikedSongIDs() ([]string, error)
```

---

### Phase 2: API Endpoints

**File**: [backend/internal/api/songs.go](backend/internal/api/songs.go) (create new file or add to existing)

#### 2.1 Toggle Like Endpoint
```
POST /api/songs/{id}/like
Response: { "liked": true, "likedAt": 1735123456789 }
```

#### 2.2 Get All Liked Songs
```
GET /api/songs/liked
Response: [Song, Song, ...]
```

#### 2.3 Get Liked Song IDs (for quick sync)
```
GET /api/songs/liked/ids
Response: { "ids": ["song1", "song2", ...] }
```

#### 2.4 Bulk Like/Unlike (for album or multi-select)
```
POST /api/songs/like/bulk
Body: { "songIds": ["id1", "id2"], "liked": true }
Response: { "updated": 5 }
```

---

### Phase 3: Frontend Types

**File**: [types.ts](types.ts)

```typescript
export interface Song {
  // ... existing fields
  
  // Like status
  liked?: boolean;
  likedAt?: number; // timestamp when liked
}
```

**File**: [services/api.ts](services/api.ts)

Add new API methods:
```typescript
async toggleLike(songId: string): Promise<{ liked: boolean; likedAt: number }>
async getLikedSongs(): Promise<Song[]>
async getLikedSongIds(): Promise<string[]>
async bulkLike(songIds: string[], liked: boolean): Promise<{ updated: number }>
```

---

### Phase 4: Zustand Store

**File**: [slices/librarySlice.ts](slices/librarySlice.ts)

Add to library slice:
```typescript
interface LibrarySlice {
  // ... existing
  likedSongIds: Set<string>;  // Quick lookup set
  
  // Actions
  toggleLikeSong: (songId: string) => Promise<void>;
  syncLikedSongs: () => Promise<void>;
  isLiked: (songId: string) => boolean;
}
```

---

### Phase 5: UI Components

#### 5.1 LikeButton Component

**New File**: [components/LikeButton.tsx](components/LikeButton.tsx)

```tsx
interface LikeButtonProps {
  songId: string;
  size?: number;
  className?: string;
}

export const LikeButton: React.FC<LikeButtonProps> = ({ songId, size = 24, className }) => {
  const { likedSongIds, toggleLikeSong } = useStore();
  const isLiked = likedSongIds.has(songId);
  
  return (
    <button
      onClick={(e) => { e.stopPropagation(); toggleLikeSong(songId); }}
      className={`transition-all ${isLiked ? 'text-brand' : 'text-white/50 hover:text-brand'} ${className}`}
      title={isLiked ? 'Remove from Liked Songs' : 'Add to Liked Songs'}
    >
      <Heart size={size} fill={isLiked ? 'currentColor' : 'none'} />
    </button>
  );
};
```

#### 5.2 Update Existing Heart Buttons

| Component | Change |
|-----------|--------|
| [NowPlaying.tsx](components/NowPlaying.tsx) | Replace static `<Heart>` with `<LikeButton songId={currentSong.id} size={32} />` |
| [AlbumDetail.tsx](pages/AlbumDetail.tsx) | Add album-level like (likes all songs) or per-song in track list |
| [Songs.tsx](pages/Songs.tsx) | Add like button to song rows |
| Track rows everywhere | Add like button or icon indicator |

---

### Phase 6: Liked Songs Page/Section

#### 6.1 Smart Playlist Integration

**File**: [pages/SmartPlaylists.tsx](pages/SmartPlaylists.tsx)

Add a "Liked Songs" smart playlist that:
- Auto-populates with all liked songs
- Updates in real-time as songs are liked/unliked
- Shows as a special system playlist

#### 6.2 Sidebar Integration

**File**: [components/Sidebar.tsx](components/Sidebar.tsx)

Add "Liked Songs" quick link with heart icon and count badge.

---

### Phase 7: Keyboard Shortcut

**File**: [hooks/useKeyboardNavigation.ts](hooks/useKeyboardNavigation.ts)

Add `Ctrl+L` or `L` keyboard shortcut to like/unlike current song (as mentioned in [WOW_FEATURES_PLAN.md](WOW_FEATURES_PLAN.md#L283)).

---

## Integration Points

### Smart Mix Integration

Update smart mix rules in [lib/smartMix.ts](lib/smartMix.ts) to:
- Include "Liked Songs" as a mix type
- Factor liked status into discovery algorithms
- Weight liked songs higher in recommendations

### Play History

Liked songs should influence:
- "Favorites" discover mode
- Song recommendations
- Queue suggestions

### Sync with Spotify (Future)

If user is connected to Spotify:
- Optionally sync liked songs to/from Spotify library
- Show Spotify liked status for streaming tracks

---

## File Changes Summary

| File | Changes |
|------|---------|
| `backend/internal/db/db.go` | Add `liked`, `liked_at` columns; add Song struct fields; add like methods |
| `backend/internal/api/routes.go` | Add like API routes |
| `backend/internal/api/songs.go` | Implement like handlers (new file or existing) |
| `types.ts` | Add `liked`, `likedAt` to Song interface |
| `services/api.ts` | Add like API methods |
| `slices/librarySlice.ts` | Add likedSongIds set and toggleLike action |
| `components/LikeButton.tsx` | **NEW** - Reusable like button component |
| `components/NowPlaying.tsx` | Use LikeButton |
| `pages/AlbumDetail.tsx` | Use LikeButton |
| `pages/Songs.tsx` | Add LikeButton to rows |
| `components/Sidebar.tsx` | Add Liked Songs link |
| `pages/SmartPlaylists.tsx` | Add Liked Songs smart playlist |
| `hooks/useKeyboardNavigation.ts` | Add Ctrl+L shortcut |

---

## Database Migration

```sql
-- Migration: Add likes support to songs table
ALTER TABLE songs ADD COLUMN liked INTEGER DEFAULT 0;
ALTER TABLE songs ADD COLUMN liked_at INTEGER;

-- Create index for fast liked song queries
CREATE INDEX IF NOT EXISTS idx_songs_liked ON songs(liked) WHERE liked = 1;
```

---

## UI/UX Considerations

1. **Heart Animation**: Animate the heart fill on like (scale + color transition)
2. **Toast Notification**: Show brief toast "Added to Liked Songs" / "Removed from Liked Songs"
3. **Bulk Operations**: Allow liking entire albums or selected songs
4. **Visual Indicator**: Liked songs should have a subtle heart indicator in lists
5. **Filter Option**: Add "Show only liked" filter to Songs page
6. **Sort Option**: Add "Liked At" sort option (newest liked first)

---

## Testing Checklist

- [ ] Like a song from Now Playing
- [ ] Unlike a song from Now Playing
- [ ] Like persists after app restart
- [ ] Liked songs appear in Liked Songs playlist
- [ ] Keyboard shortcut works
- [ ] Album-level like works
- [ ] Bulk like/unlike works
- [ ] Like status syncs across views
- [ ] Performance with many liked songs (1000+)

---

## Status

### Phase Completion

- [x] **Phase 1**: Database Schema - Added `liked` and `likedAt` fields to Song struct, migration, and CRUD methods
- [x] **Phase 2**: API Endpoints - Added `/songs/{id}/like`, `/songs/liked`, `/songs/like/bulk` endpoints
- [x] **Phase 3**: Frontend Types - Updated `Song` interface and `ApiSong` with like fields, added API methods
- [x] **Phase 4**: Zustand Store - Added `likedSongIds` set, `toggleLikeSong()`, `syncLikedSongs()`, `isLikedSong()`
- [x] **Phase 5**: UI Components - Created `LikeButton.tsx`, integrated into `NowPlaying.tsx`, `Songs.tsx`, `AlbumDetail.tsx`
- [x] **Phase 6**: Liked Songs Page - Created `LikedSongs.tsx` page with full track listing, added sidebar link with count badge
- [ ] **Phase 7**: Keyboard Shortcut - Add Ctrl+L shortcut (future)

### Implementation Summary

**Backend (Go)**:
- `backend/internal/db/db.go`: Added `Liked bool`, `LikedAt int64` to Song struct; migration for columns; methods: `ToggleLike()`, `SetLike()`, `BulkSetLike()`, `GetLikedSongIDs()`; updated `scanSongsWithMood()` helper to include liked fields in all song queries
- `backend/internal/api/api.go`: Added routes and handlers for toggling likes, getting liked IDs, bulk operations

**Frontend (TypeScript)**:
- `types.ts`: Added `liked?: boolean`, `likedAt?: number` to Song interface
- `services/api.ts`: Added `toggleLike()`, `getLikedSongIds()`, `bulkLikeSongs()` methods
- `slices/types.ts`: Added `likedSongIds: Set<string>` and actions to LibrarySlice interface
- `slices/librarySlice.ts`: Implemented likes state initialization, `toggleLikeSong()`, `syncLikedSongs()`, `isLikedSong()`
- `components/LikeButton.tsx`: **NEW** - Reusable heart button component with filled/unfilled states
- `components/NowPlaying.tsx`: Replaced static heart with LikeButton
- `components/Sidebar.tsx`: Added Liked Songs navigation item with count badge
- `pages/Songs.tsx`: Added LikeButton to song rows
- `pages/AlbumDetail.tsx`: Added LikeButton to track rows
- `pages/LikedSongs.tsx`: **NEW** - Dedicated page for viewing all liked songs with virtualized list
- `App.tsx`: Added `/liked` route for LikedSongs page
- User personalization
- Better recommendations
- Smart playlist improvement
- Essential music library feature expected by users
