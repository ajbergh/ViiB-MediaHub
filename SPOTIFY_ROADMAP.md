# Spotify Integration Roadmap

This document outlines the roadmap for improving the Spotify integration in ViiB MediaHub, moving from immediate stability fixes to full download capabilities.

## Phase 1: Stability & Types (Immediate) ✅

- [x] **Fix `SpotifyProfile` Type Definition**
  - Update `types.ts` to include `followers` and `external_urls`.
  - Ensure strict typing across `Spotify.tsx` and `spotifyService.ts`.

- [x] **Robust Error Handling**
  - Create typed error classes (`SpotifyAuthError`, `SpotifyRateLimitError`, `SpotifyApiError`).
  - Implement specific error handling in `spotifyService.ts` instead of generic catch blocks.
  - Add user-friendly error toasts/notifications in the UI.

- [x] **Token Refresh Race Condition Fix**
  - Implement a mutex/lock pattern in `spotifyService.ts` to prevent multiple simultaneous refresh requests.
  - Ensure all pending requests wait for the single refresh operation to complete.

## Phase 2: UI/UX Enhancements ✅

- [x] **Search Pagination**
  - Update `SpotifyService.search` to accept `offset` and `limit`.
  - Implement "Load More" or infinite scroll in `Spotify.tsx`.

- [x] **Detail Views**
  - Create `SpotifyAlbumDetail.tsx` and `SpotifyPlaylistDetail.tsx`.
  - Add routing to navigate from search results to these detail views.
  - Allow playing individual tracks from these views.

- [x] **Improved Artist Matching**
  - Replace the basic string inclusion check with a Levenshtein distance or similar fuzzy matching algorithm in `spotifyService.ts`.
  - Improve accuracy of metadata fetching for local library artists.

- [x] **User Library Features**
  - Add tabs for "Recently Played", "Saved Albums", and "Saved Playlists".
  - Allow users to browse their own Spotify library within ViiB.

## Phase 3: Backend Migration (Security & Performance) ✅

- [x] **Move API Calls to Backend**
  - Create new endpoints in `backend/internal/api/spotify.go` to proxy requests to Spotify.
  - Added `/api/spotify/search`, `/api/spotify/me`, and `/api/spotify/proxy` endpoints.
  - Client secret is already stored in backend via existing credentials system.

- [x] **Session Management**
  - Store Spotify session state securely in the backend via `spotify_credentials` setting.
  - Token refresh handled by frontend for now (can be moved to backend in future enhancement).
  
**Note:** For full security, frontend code should be updated to use backend proxy endpoints instead of calling Spotify API directly. This would require updating `spotifyService.ts` to call `/api/spotify/search` and `/api/spotify/proxy` endpoints.

## Phase 4: Download Capability (Partial) 🚧

- [x] **Backend Download Infrastructure**
  - Created `spotify_downloads` table in database with indexes for status and spotify_id.
  - Implemented `DownloadManager` struct in `backend/internal/api/download_manager.go` with queue processing.
  - Implemented endpoints:
    - `POST /api/spotify/download/track` (with metadata)
    - `POST /api/spotify/download/album` (fetches tracks from Spotify API)
    - `POST /api/spotify/download/playlist` (fetches tracks from Spotify API)
    - `GET /api/spotify/downloads` (list all downloads)
    - `GET /api/spotify/downloads/{id}` (get download status)
    - `DELETE /api/spotify/downloads/{id}` (delete download)
    - `GET /api/spotify/downloads/events` (SSE for real-time progress)

- [x] **Download Queue System**
  - Implemented persistent queue in SQLite (`spotify_downloads` table).
  - Created background worker goroutine with 2-second ticker to process queue.
  - Implemented SSE (Server-Sent Events) for real-time progress updates to frontend.
  - Added helper functions to fetch track/album/playlist metadata from Spotify API.

- [x] **Librespot Integration**
  - Integrated `librespot-go` library (github.com/art-media-platform/librespot-go).
  - Created `SessionManager` (`backend/internal/spotify/session.go`) for managing Spotify authentication.
  - Created `Downloader` (`backend/internal/spotify/downloader.go`) for downloading tracks.
  - Updated `DownloadManager` to use actual librespot-go instead of simulation.
  - Implemented real-time progress tracking during downloads.
  - **OAuth Integration**: Uses existing OAuth access tokens from Spotify Web API authentication (no separate credentials needed).
  - Session automatically retrieves access token from database before each download.

- [ ] **Library Integration**
  - Automatically scan and import downloaded files into the ViiB library.
  - Write metadata (ID3/Vorbis comments) to downloaded OGG files.
  - Link downloaded songs to their Spotify IDs in the database.

- [x] **Frontend Download UI**
  - Created `DownloadManager` component (`components/DownloadManager.tsx`) with:
    - EventSource connection for real-time SSE updates
    - Collapsible widget showing active, failed, and completed downloads
    - Progress bars for active downloads
    - Delete functionality for completed/failed downloads
  - Created full `Downloads` page (`pages/Downloads.tsx`) with:
    - Real-time download queue display with SSE updates
    - Stats dashboard showing active, completed, and failed downloads
    - Filter tabs to view downloads by status
    - Individual download cards with progress bars and error messages
    - Delete functionality for completed/failed downloads
  - Added download API service methods to `services/api.ts`.
  - Integrated download buttons into Spotify UI:
    - Track rows have download icon buttons with loading states
    - Album cards have download buttons (top-right corner) with loading states
    - Playlist cards have download buttons (top-right corner) with loading states
    - Visual feedback (spinners) while queueing downloads
  - Download actions integrated with toast notifications via `addLog`.
  - Added download location setting in Settings page:
    - Backend API endpoints for settings storage (`GET/POST /api/settings/{key}`)
    - UI to configure Spotify download directory path
    - Setting persisted in SQLite database

## Next Steps

The download infrastructure is now complete with OAuth authentication! The remaining work involves:

1. **Library Integration** - After downloads complete, automatically scan the download directory and import files into the ViiB library with proper metadata.
   - Add post-download hook to trigger library scan
   - Write proper ID3/Vorbis metadata to OGG files
   - Link downloaded songs to their Spotify IDs in the database

2. **Optional Enhancements**:
   - Add download quality settings (320kbps, 160kbps, 96kbps)
   - Implement concurrent downloads (currently limited to one at a time for stability)
   - Add pause/resume functionality for downloads
   - Visual indicators in Spotify UI for tracks that are already downloaded/downloading
   - Option to automatically download albums or playlists when added to library
   - Batch download operations with progress aggregation
   - Download history and statistics
   - Automatic re-download for failed items

## Testing the Download System

To test downloads:

1. Start the application with `scripts/dev.ps1`

2. Navigate to the Spotify page and log in via OAuth (Spotify Premium account required)

3. Search for a track, album, or playlist

4. Click the download button (download icon on hover for tracks, top-right button for albums/playlists)

5. Open the DownloadManager widget (bottom-right corner of the screen)

6. Watch real-time download progress with live updates via Server-Sent Events (SSE)

7. Downloaded files are saved to `data/spotify_downloads/{artist}/{track}.ogg`

**Requirements**: 
- Spotify Premium account required for downloading
- OAuth authentication completed via Spotify Web API
- The librespot-go library uses your OAuth access token to authenticate and stream audio directly from Spotify servers
- No additional credentials or environment variables needed
