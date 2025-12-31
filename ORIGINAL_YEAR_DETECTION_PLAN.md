# Original Year Detection Feature

## Problem Statement

When music is remastered, re-released, or included in compilation albums, the embedded metadata year often reflects the **release date of that edition** rather than the **original release year** of the song.

**Example:**
- Song: "Smells Like Teen Spirit" by Nirvana
- Original Release: 1991 (Nevermind album)
- 2021 Remaster Release: 2021 (30th Anniversary Deluxe Edition)
- Metadata `year`: 2021 ❌

This causes AI DJ queries like "90s rock" to miss classic songs that were remastered recently.

## Goal

Accurately detect the **original release year** of songs to enable correct decade-based filtering in the AI DJ.

---

## Solution Architecture

### Phase 1: Schema & Query Updates

**1.1 Add `original_year` Column**
```sql
ALTER TABLE songs ADD COLUMN original_year INTEGER;
CREATE INDEX idx_songs_original_year ON songs(original_year);
```

**1.2 Update AI DJ Queries**
- Replace `year` with `COALESCE(original_year, year)` in:
  - `GetSongsByExactGenreWithYears()`
  - `GetSongsBySmartFilter()`
- This uses `original_year` when available, falls back to `year`

**1.3 Update Frontend Song Type**
- Add `originalYear?: number` to `Song` interface

---

### Phase 2: Heuristic Detection (No API Required)

**2.1 Remaster Pattern Detection**
Detect common remaster indicators in album names:

```go
var remasterPatterns = []string{
    "remaster",
    "remastered",
    "anniversary",
    "deluxe edition",
    "expanded edition",
    "collector's edition",
    "legacy edition",
    "super deluxe",
    "(\\d{4}\\s+remaster)", // e.g., "(2021 Remaster)"
}
```

**2.2 Year Extraction from Album Name**
Extract original year from patterns like:
- `Album Name (1991 Remaster)` → original_year = 1991
- `Album Name [Remastered 2020]` → flag for enrichment
- `Album Name - 30th Anniversary Edition` → calculate from album year

**2.3 Artist Active Period Lookup**
For artists with well-known active periods (stored in artist_metadata):
- If song year > artist end year → likely a remaster
- Example: Nirvana (1987-1994), song year 2021 → flag

**2.4 Flagging System**
Add `year_uncertain BOOLEAN` flag to songs:
- Set when heuristics detect likely remaster
- Used to prioritize Gemini enrichment

---

### Phase 3: Gemini AI Enrichment

**3.1 Enrichment Prompt**
```
For each song, determine the ORIGINAL release year (not remaster/re-release dates).

Song: {title}
Artist: {artist}
Album: {album}
Current Year Tag: {year}

Return ONLY the original release year as a 4-digit number.
If uncertain, return "unknown".
```

**3.2 Batch Processing**
- Process in batches of 20 songs (like mood enrichment)
- SSE streaming for progress updates
- Store results in `original_year` column

**3.3 Priority Queue**
Process songs in order:
1. `year_uncertain = true` (flagged by heuristics)
2. `year > 2000 AND original_year IS NULL` (likely newer remasters)
3. Remaining songs without `original_year`

---

### Phase 4: UI Integration

**4.1 Settings Page**
Add "Detect Original Years" section:
```
📅 Detect Original Years
Some songs may have their year set to a remaster or re-release date.
This feature uses AI to detect the original release year of your songs.

[✨ Detect Original Years]  [Progress bar]

Status: Processed 150/500 songs - Found 45 remasters
```

**4.2 Song Details Display**
Show both years when different:
```
Year: 2021 (Originally: 1991)
```

**4.3 Album Detail View**
Show "Original Release" badge for remastered albums

---

## Implementation Phases

### Phase 1: Foundation ✅ COMPLETE
- [x] Add `original_year` column to songs table
- [x] Add migration logic in db.go
- [x] Update `Song` struct with `OriginalYear` field
- [x] Update AI DJ queries to use `COALESCE(original_year, year)`
- [x] Update frontend `Song` type
- [x] Test AI DJ with manual `original_year` values

### Phase 2: Heuristic Detection ✅ COMPLETE
- [x] Create `detectRemasterPatterns()` function in `year_detection.go`
- [x] Implement year extraction from album names
- [x] Add `year_uncertain` flag logic
- [x] Create `DetectRemasterSongs()` batch function
- [x] Add `/api/library/detect-remasters` endpoint

### Phase 3: Gemini Enrichment ✅ COMPLETE
- [x] Create `AnalyzeOriginalYear()` Gemini function
- [x] Add `/api/library/enrich-years/stream` SSE endpoint
- [x] Implement batch processing with priority queue
- [x] Add progress tracking
- [x] Handle rate limiting and errors

### Phase 4: UI ✅ COMPLETE
- [x] Add Settings section for original year detection (two-step process)
- [x] Show dual years in album detail view "(Originally: 1991)"
- [x] Add enrichment progress display via SSE
- [ ] Add remaster badge to album cards (optional enhancement)

---

## Database Schema Changes

```sql
-- Add original year tracking
ALTER TABLE songs ADD COLUMN original_year INTEGER;
ALTER TABLE songs ADD COLUMN year_uncertain INTEGER DEFAULT 0;
ALTER TABLE songs ADD COLUMN year_analyzed_at INTEGER;

-- Index for efficient querying
CREATE INDEX idx_songs_original_year ON songs(original_year);
CREATE INDEX idx_songs_year_uncertain ON songs(year_uncertain) WHERE year_uncertain = 1;
```

---

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/library/detect-remasters` | POST | Run heuristic detection on library |
| `/api/library/enrich-years/stream` | GET | SSE stream for Gemini year enrichment |
| `/api/songs/{id}/original-year` | PATCH | Manually set original year |

---

## Success Metrics

1. **Accuracy**: 90%+ of remasters correctly identified
2. **Coverage**: Process 500 songs in < 5 minutes
3. **User Experience**: Clear progress feedback, no freezing
4. **AI DJ Improvement**: "90s rock" returns classic songs regardless of remaster year

---

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Gemini rate limiting | Batch processing with delays, caching |
| Incorrect year detection | Allow manual override, show confidence |
| Large library performance | Process in background, don't block UI |
| API key not configured | Show helpful message, disable feature |

---

## Future Enhancements

1. **Spotify Original Release Lookup**
   - Use Spotify's `release_date_precision` and album linking
   - Fall back to Gemini for non-Spotify content

2. **MusicBrainz Integration**
   - Query MusicBrainz for authoritative release dates
   - Cross-reference with existing metadata

3. **User Corrections**
   - Allow users to correct detected years
   - Learn from corrections to improve heuristics

4. **Remaster Chain Display**
   - Show all versions of an album
   - Group by original release
