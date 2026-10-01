# ViiB MediaHub - Project Review Findings

Review date: September 28, 2026  
Verification pass: September 29, 2026 (every finding re-traced against commit `9240afd`; see "Revision notes")  
Source: OneDrive-synced ViiB-MediaHub project snapshot  
Status: Findings updated; no application source changes made

## Executive assessment

ViiB MediaHub has a substantial architecture and test foundation, but the catalog-data, metadata-editing, and playback defects below should be addressed before treating this snapshot as release-ready. The highest-priority risks concern catalog records and user metadata; this review did not establish that the application deletes underlying music files.

Findings F01-F22 were independently re-traced in the verification pass and all remain supported by current source. Several are broader than originally written, notably:

- **F02:** an offline or unmounted library root can remove every cataloged song under that root.
- **F22:** the Spotify stream limit is never enforced, not merely racy.

A few have narrower real-world impact than originally implied (F03, F07, F20). The macOS packaging issue previously labeled F23 was fixed in commit `9240afd`.

Prioritize reliability, recovery, and failure handling over new features. This is a risk-focused source review, not an exhaustive line-by-line audit, security certification, or successful cross-platform runtime validation.

## Scope and evidence

- The inventory covers the 799 Git-tracked files (the untracked review document excluded). The prior review collected 755 source/documentation files from that set.
- 651 tracked Go/TypeScript files excluding generated Wails bindings, including 127 Go test files and 48 TypeScript test files (re-counted with `git ls-files`).
- Reviewed frontend playback, queues, metadata editing, state, persistence, backend APIs, database behavior, scanning, synchronization, recovery, integrations, analysis, build scripts, release workflows, and supporting documentation.
- Excluded dependency trees, Git internals, private assistant settings, generated build output, and binary media/assets from source inspection.
- Large UI, visualizer, and analysis areas received targeted sampling rather than exhaustive reading.
- References below are repository-relative filenames and line ranges checked against commit `9240afd`. They can shift after edits.
- **Source-traced:** established by following implementation and relevant callers/tests, without executing the full feature.
- **Isolated reproduction:** actual frontend logic exercised with mocked dependencies or controlled timers during the prior review; not rerun in the verification pass (which re-traced the source instead) and not a live browser/Wails/audio-device test.
- **P1:** high-priority data integrity or core-functionality issue. **P2:** important correctness/reliability issue. Priorities are review recommendations, not measured production incident severity.

## P1 - Data integrity and core functionality

### F01 - Full scans can permanently delete ignored song records

**Evidence:** `backend/internal/scanner/scanner.go:711-751,893-905`; `backend/internal/db/db.go:1200-1201,1219-1243`; `backend/internal/db/duplicates.go:31-47`.  
**Validation:** Source-traced.

**Trigger:** Ignore a cataloged duplicate, then run a full scan in which that song's root reports no errors.

Ignored paths return (`scanner.go:893-895`) before being appended to `scannedPaths` (`:905`). `GetAllFilePaths` has no `ignored` filter, so reconciliation treats the still-existing file as missing. It then deletes the row for any root in the error-free "deletion-safe" set.

**Impact:** The ignore decision and associated song data are lost. Because the deleted row no longer appears in `GetIgnoredFilePaths`, the next full scan re-imports the file with `ignored=0`, so the duplicate silently reappears. On alternate scans the duplicate reappears as visible and is then deleted again.

**Recommended fix:** Add ignored paths to the found set before skipping metadata extraction, or exclude `ignored=1` rows from destructive reconciliation.

**Regression test:** Ignore a song, run two full scans, and verify its record, ignore state, and associated data survive both. No current test combines scanning with the ignore flag.

### F02 - Fallback incremental scans convert access errors and offline roots into deletions

**Evidence:** `backend/internal/scanner/journal_mtime.go:54-58,111-128`; `backend/internal/scanner/incremental_changes.go:92-101`; fallback selection in `backend/internal/scanner/journal_windows.go:120-150` and `backend/internal/scanner/journal.go:35-46`.  
**Validation:** Source-traced.

**Trigger:** A previously scanned folder or entire library root becomes inaccessible (permissions, unplugged drive, offline NAS) while the mtime fallback detector is active.

The walk callback discards every error (`if err != nil { return nil // Skip errors }`). Any cached path under a watch path that was not seen is emitted as `ChangeTypeDeleted`, and `ProcessChanges` passes those paths to `DeleteSongsByFilePaths` with no stat recheck or root-availability check. If the root itself is missing, the walk visits nothing and every cached entry under it is emitted as deleted.

Two aggravating factors:

- The watch-path match is `strings.HasPrefix` without a separator boundary, so a watch path of `C:\Music` also matches `C:\Music2\...`.
- The Windows USN detector opens the volume handle (`\\.\C:`), which normally requires administrator rights. The mtime fallback is therefore likely the common path for non-admin Windows users. It is also used by macOS no-cgo builds. The Linux detector does not emit deletions.

`DetectDeletedFiles` (`signatures.go:160-184`) correctly requires `os.IsNotExist`, but it runs in addition to this detector rather than replacing it.

**Impact:** A temporary permission, network-share, or removable-drive outage can remove valid catalog records, and potentially an entire root's catalog, together with associated user data.

**Recommended fix:** Track walk errors per root/subtree and suppress deletions for failed or unavailable roots; confirm absence with `os.Stat` + `os.IsNotExist` before deleting; use a path-boundary-aware containment check instead of `HasPrefix`.

**Regression test:** Simulate a missing root, permission errors, and transient I/O failures; verify catalog records remain intact and the source is reported unavailable. Add a sibling-prefix case (`Music` vs `Music2`). No current test covers this detector's deletion path.

### F03 - Restore rollback omits committed WAL-only changes

**Evidence:** `backend/internal/db/pending_restore.go:12-46`; sole caller `backend/cmd/viib-restore/main.go:22`.  
**Validation:** Source-traced.

**Trigger:** Run the offline `viib-restore` helper after an unclean shutdown (crash, forced kill, power loss) that left committed but uncheckpointed transactions in the write-ahead log (WAL), or while the application is still running.

The rollback copy contains only the main database file. The restore then removes the original `-wal` and `-shm` files (removal errors are ignored), and both failure branches copy the main-file-only rollback back.

**Scope:** `ApplyPendingRestore` is only called from the offline `viib-restore` CLI, never inside the running app. A clean application exit checkpoints and removes the WAL, so the common path loses nothing. The existing test closes the database cleanly first and therefore does not exercise the defect. Nothing verifies that the application has exited (no lock or PID check). On Unix, deleting a live WAL can corrupt the database. The rollback copy is also not integrity-validated.

**Impact:** After an unclean shutdown, or if run concurrently with the app, rollback can lose recent committed changes, including when activation fails.

**Recommended fix:** Require exclusive offline access (lock check). Checkpoint the original (`PRAGMA wal_checkpoint(TRUNCATE)`) or create the rollback with `VACUUM INTO`/the SQLite backup API before replacing files, and validate the rollback copy.

**Regression test:** Create committed WAL-only changes, simulate an unclean shutdown and activation failure, then verify the rollback retains those changes. Existing clean-close tests do not establish this property.

### F04 - Remove-missing repair deletes inaccessible catalog entries

**Evidence:** `backend/internal/db/library_diagnostics.go:78-84,117-132`; UI trigger `pages/LibraryOperations.tsx:159`.  
**Validation:** Source-traced.

**Trigger:** Run repair with `removeMissing:true` when filesystem inspection returns permission denied, another transient error, or ENOENT because a drive/share is not mounted.

Every `os.Stat` error enters the missing-media list (there is no `os.IsNotExist` filter), and repair re-runs diagnostics server-side and deletes every listed path. The only exclusion is `plex://` URIs. The "Remove missing files" button calls `repair(true)` without a confirmation step, and it deletes a freshly recomputed list rather than the list the user previewed.

**Impact:** Valid songs and user metadata can disappear from the catalog; playlist references may subsequently be removed.

**Recommended fix:** Separate missing, inaccessible, and invalid-path diagnostics. An `IsNotExist` filter alone is insufficient, because an unmounted root also reports ENOENT. Also verify that the owning root is available and readable, recheck before deleting, and delete only the IDs the user confirmed.

**Regression test:** Cover permission denied, transient I/O errors, and an unmounted root separately from genuine absence.

### F05 - An unfinished metadata edit can overwrite another song

**Evidence:** `components/SongInfoDialog.tsx:61-121,205-208`; `slices/uiSlice.ts:116-117`; persistent mounting in `App.tsx:192`.  
**Validation:** Isolated reproduction (prior review); source re-traced.

**Trigger:** Open song A, start editing tags, close the dialog without cancelling, open song B, and save.

The dialog stays mounted and only returns `null` when no song is selected. It has no `useEffect` reset and no `key`. `closeSongInfoModal` clears only the selected song, so `isEditing` and the edit fields survive. Reopening on B renders the edit form with A's values, and save calls `updateSongMetadata(B.id, {A's title, artist, album})`. The dialog is reachable with a different song from many surfaces: player, now playing, songs list, song/queue menus, and a keyboard shortcut.

**Recommended fix:** Mount a keyed editor (`<SongInfoDialog key={songInfoModalSong?.id} />`) or reset edit state on close and on song change. Explicitly bind every draft to its originating song ID.

**Regression test:** Switch songs after closing an unfinished edit; verify old draft values cannot be saved to the new song.

### F06 - An obsolete crossfade timer stops a newer track

**Evidence:** `lib/audio.ts:173-183`; `hooks/useAudioPlayer.ts:444-486`.  
**Validation:** Isolated reproduction with controlled timers (prior review); source re-traced.

**Trigger:** Advance twice before the first transition's cleanup timer fires.

Transition A→B schedules an untracked `setTimeout` that pauses and rewinds the outgoing element P0. Transition B→C then plays C on P0, and the stale timer pauses and rewinds it. The window is short with defaults: about 300 ms (0.2 s fade + 100 ms), or 100 ms in gapless mode. It is correspondingly longer with long crossfades.

**Impact:** Current playback abruptly stops and resets while the store still reports `isPlaying=true`.

**Recommended fix:** Track cleanup timers or playback generations per element and cancel/ignore them when that element becomes the incoming element. `slices/playerSlice.ts:231` has a similar untracked delayed Blob URL revocation.

**Regression test:** Perform consecutive transitions within one fade duration; assert that earlier callbacks cannot pause the active track.

### F07 - Partial initial embedding failures suppress successful search indexes

**Evidence:** `backend/internal/semantic/service.go:166-179,538-542`; unchanged-catalog return at `219-220`; startup load at `91`.  
**Validation:** Source-traced.

**Trigger:** Initial indexing (or a full rebuild) includes successful embedding batches and at least one failed batch.

Successful vectors are persisted without updating in-memory indexes. Any failure returns (`service.go:173-176`) before `loadReadyIndexesLocked`, and periodic sync does not reload indexes when the catalog is unchanged.

**Impact:** Semantic search is empty (first install) or stale (rebuild) for the rest of the session. The loss is not permanent: `Start()` loads ready indexes on the next launch, and the "reload" rebuild scope and `RetryErrors` also recover.

**Recommended fix:** Load successfully persisted vectors before returning the error status, report partial readiness, and retain the retry path for failed documents.

**Regression test:** Mix successful and permanently failing batches; successful documents must be searchable in the same session. Existing tests cover only the all-fail case.

### F08 - Changing embedding dimensions does not rebuild old vectors

**Evidence:** `backend/internal/semantic/service.go:336-348,549+`; `backend/internal/semantic/embedding.go:73-74`; `backend/internal/api/semantic.go:233-302` (settings update and restart); related provider construction and restart at `104-117,148-167`.  
**Validation:** Source-traced across settings and indexing layers.

**Trigger:** Change dimensions while retaining the same embedding provider/model (OpenAI, Gemini, and OpenRouter send configured dimensions to the provider).

Persistent embedding identity (`service.go:345`) compares provider, model, and input prefix but omits dimensions. The settings API saves and restarts the service without resetting embeddings (`ResetSemanticEmbeddings` is only called from the identity check). Content hashes are unchanged, so nothing is re-embedded.

**Impact:** The failure is loud rather than silently wrong. Every query fails dimensional validation against the stored index dimension, and new or changed documents also fail. Semantic search stays broken until a manual reset.

**Recommended fix:** Include dimensions in persistent identity and invalidate/rebuild on change. Providers currently expose no dimensions accessor, so one needs adding, returning 0 where the model determines the size (e.g., Ollama).

**Regression test:** Change only dimensions, restart the service, and verify reindexing and successful retrieval.

## P2 - Playback and persistence

### F09 - Duplicate queue entries break queue progression

**Evidence:** `slices/playerSlice.ts:238-248,284-296,377-382`.  
**Validation:** Isolated reproduction (prior review); source re-traced.

`nextSong` calls `playSong(queue[nextIndex], queue)`, and `playSong` resolves position with `findIndex(s => s.id === song.id)`. For `[A, B, A]`, advancing from B lands on index 0 instead of 2, so playback loops A→B→A→B. `prevSong`, `playQueueItem`, and `retryStream` share the flaw. `addToQueue`/`playNext` do not deduplicate, so duplicates are easy to create.

**Fix:** Preserve queue-occurrence identity/index through playback actions (explicit index or per-entry queue IDs). Test next, previous, and direct selection with repeated songs.

### F10 - Failed metadata writes are reported as saved

**Evidence:** `slices/librarySlice.ts:458-489`; `components/SongInfoDialog.tsx:99-120`.  
**Validation:** Isolated reproduction with a rejecting metadata service (prior review); source re-traced.

The state update is optimistic; the backend error is caught and only logged, with no rollback, and the action resolves normally, so the dialog shows success. Related gaps:

- An unknown song ID returns silently and is also reported as success.
- In browser mode the storage write is not awaited.
- After saving, the dialog still displays the stale `songInfoModalSong` object.

**Fix:** Propagate failures and roll back optimistic changes, or commit state only after persistence succeeds. Await browser-storage writes and refresh the displayed song. Test a failed metadata PATCH and verify an error, not a success message.

### F11 - Backend creation failures leave playlists that disappear from view

**Evidence:** `slices/librarySlice.ts:286-311,354-379`; initialization/refresh at `98-137,182-191`.  
**Validation:** Isolated reproduction (prior review); source re-traced.

Playlist creation silently falls back to IndexedDB with a random ID while `backendAvailable` stays true. A later server refresh replaces the list wholesale, and backend-mode initialization never reads IndexedDB playlists, so the playlist is also gone after restart. Later edits call `updatePlaylist` on an ID the backend does not know; that failure is only logged.

**Fix:** Do not fall back while the backend is marked available; surface the error, or implement a persistent pending-write queue with reconciliation and ID mapping. Test failure followed by refresh/restart and successful recovery.

### F12 - File-handle playback reuses invalid Blob URLs (browser-only mode)

**Evidence:** `slices/playerSlice.ts:204-210,226-231`; `lib/playbackLifecycle.ts:22-40`; persistence via `lib/parsers.ts:132`, `pages/Settings.tsx:1058-1063`, `slices/librarySlice.ts:207`, and `services/libraryService.ts:40-44`.  
**Validation:** Isolated reproduction (prior review); source re-traced.

A `blob:` prefix is treated as evidence that an audio URL is reusable. Two paths produce invalid ones:

- **Across sessions:** `parseSong` creates a Blob URL that is saved with the song into IndexedDB. After a reload that URL is dead, but the prefix check skips regeneration from the file handle.
- **Within a session:** managed URLs are written back into the queue and later revoked (`:231`). Returning to that entry via previous/direct selection reuses the revoked URL.

URLs from `parseSong` are never registered, so they are also never revoked (a leak). Backend mode is unaffected because it uses `/api/audio/...` URLs.

*(Correction: the original citation `slices/librarySlice.ts:159-164` recreates cover URLs, not audio URLs.)*

**Fix:** Reuse a Blob URL only if the lifecycle registry still owns it, regenerate from file handles otherwise, and do not persist `url`/`coverUrl`. Test replay after revocation and after restart.

### F13 - A pending streaming retry overrides a newer selection

**Evidence:** `slices/playerSlice.ts:616-661`; `hooks/useAudioPlayer.ts:311-318`.  
**Validation:** Isolated reproduction with controlled timers (prior review); source re-traced.

`retryStream` captures `currentSong` and `queue`, waits with exponential backoff, then calls `playSong` unconditionally. The existing `latestPlaybackRequestId` guard only protects calls already in flight: the delayed call obtains a new ID and wins. The 2-second auto-advance timer for unavailable tracks (`useAudioPlayer.ts:344`) has the same stale-selection pattern. Separately, because the stream URL for a track is stable, the retry may resume the same `src` without calling `load()`. Whether the retry is effective at all deserves its own check.

**Fix:** Capture a playback-generation token and cancel the retry if song, queue, or user playback intent changes. Test selection of B while A's retry is pending.

## P2 - Catalog synchronization and scanning

### F14 - Directory shortcuts miss file edits and nested changes

**Evidence:** `backend/internal/scanner/journal_linux.go:97-111`; `backend/internal/scanner/fast_startup.go:193-202`; `backend/internal/scanner/signatures.go:37-40,67-71`.  
**Validation:** Source-traced.

- **Linux detector:** it skips a directory when its ctime is older than the last verification and the *stored* latest mtime is older than the cutoff. A directory's ctime changes only when entries are added, removed, or renamed, so an in-place tag rewrite is skipped. `SkipDir` also skips all subdirectories, so a change in a nested disc folder is missed because the parent's ctime did not change.
- **Signature shortcut:** it includes the size and mtime of immediate audio files, so in-place edits of direct children *are* detected. The gap is descendants only: a matching signature returns `SkipDir`.

**Mitigations:**

- Taggers that write a temp file and rename it do change directory ctime.
- Signatures are stored only for directories that contain audio.
- A full scan catches everything.

The continuous watcher also uses the quick-startup path, however, so it inherits the gap.

**Fix:** Linux: stat files rather than relying on directory ctime, or drop the shortcut. Signatures: skip only the current directory's files and continue descending, or make the signature recursive. Test in-place edits and nested additions.

### F15 - Empty delta responses can skip concurrent commits

**Evidence:** `backend/internal/db/library_sync_queries.go:94-127`; cursor adoption in `services/libraryV2.ts:24` and `components/LibraryEventListener.tsx:91-103`.  
**Validation:** Source-traced.

The change query and the subsequent `LibraryRevision()` read run as separate statements on a 4-connection WAL pool, not in one snapshot. If a writer commits between them, an empty page reports `ToRevision = current`, and the client adopts it, skipping the change permanently. Non-empty pages use the last returned change's revision and are not affected. `ListSongsPage` in the same file already uses the correct read-only-transaction pattern.

**Fix:** Read changes, payloads, and revision in one read-only transaction, or return `ToRevision = since` for an empty page. Add a controlled concurrent-writer regression.

### F16 - Expired synchronization cursors silently receive incomplete history

**Evidence:** `backend/internal/api/v2_library.go:147-165`; `backend/internal/db/library_sync_queries.go:149-159,180-194`.  
**Validation:** Source-traced.

The delta endpoint prunes to a 100,000-revision window but never compares the client's cursor with the oldest retained revision, and the client has no gap check. Clients behind the window receive the surviving suffix. Missed deletions, inserts, and updates can leave the library stale indefinitely. `OldestLibraryChangeRevision` exists and the semantic service already uses the correct gap test (`oldest > cursor+1`), but the library API does not.

**Fix:** Apply the same gap test in the delta API, return an explicit resnapshot-required response (e.g., 410 or a flag), and have clients re-initialize their snapshot. Test resuming beyond retention.

### F17 - Ignoring a song does not remove it from synchronized clients

**Evidence:** `backend/internal/db/duplicates.go:36`; `backend/internal/db/library_sync_schema.go:96-100`; `backend/internal/db/library_sync_queries.go:162-178`; `lib/libraryIndex.ts:51-52`.  
**Validation:** Source-traced across backend and frontend.

The update trigger always logs `'upsert'`, payload retrieval filters `ignored = 0`, and the frontend skips upserts that have no payload rather than removing the item. The song therefore stays visible on every synchronized client until a full snapshot, including the client that ignored it. Un-ignoring works because the upsert then carries a payload.

**Fix:** Emit a removal/tombstone when a song becomes invisible (server-side, via a trigger migration, is cleaner and covers other visibility filters), or treat a payload-less upsert as a removal on the client. Test two clients with one ignoring a song.

## P2 - AI and music integrations

### F18 - Replacing an index invalidates in-flight search snapshots

**Evidence:** `backend/internal/semantic/retriever.go:110-123,495-517`; `backend/internal/semantic/service.go:326-332,687-692`; `backend/internal/semantic/scan_index.go:83,224-230`.  
**Validation:** Source-traced.

Search snapshots index references, then awaits the network query embedding. Reindex/reload/retry swaps the maps and calls `Close()` on the old indexes, which zeroes their dimensions and nils their data. The in-flight search then returns empty or partial results *as success* (track and album arenas can disagree). `Service.Close` during a settings restart has the same effect. The window is narrow, and there is no memory-safety issue.

**Fix:** Stop calling `Close()` on replaced indexes and let garbage collection reclaim them (`Close` only frees memory); refcounting is unnecessary. Test a query whose embedding call is blocked during index replacement.

### F19 - Last.fm track-info tags are filtered out of genre enrichment

**Evidence:** `backend/internal/lastfm/client.go:153-159,176-214`; `backend/internal/lastfm/enricher.go:113-126`; `backend/internal/lastfm/tag_mapper.go:58-61,83-90`.  
**Validation:** Source-traced.

`track.getInfo` tags carry no counts in the client library, so every `Count` is 0. The genre gate forces a minimum of 30 (both callers pass 30), so no genre is ever derived. Only genres are affected: mood, energy, tempo, and instrumental mapping have no count gate, raw tag names are still stored, and existing genres are not wiped. `GetTrackTopTags`, which returns real counts, already exists but has no callers. The `lastfm` package has no tests.

**Fix:** Use `GetTrackTopTags` for genre mapping, or treat a zero count as unknown. Test the client-to-enricher contract with recognized genre tags.

### F20 - Last.fm cancellation returns before workers finish

**Evidence:** `backend/internal/lastfm/enricher.go:59-110`; `backend/internal/api/lastfm.go:298`.  
**Validation:** Source-traced; race test not executed.

On cancellation the function returns without `wg.Wait()` while workers still mutate the result, and the semaphore send does not select on the context. The practical impact is lower than "returns mutated results": the scanner passes `context.Background()`, and the API caller does not read the result on error. The real consequence is that the API's "enrichment running" guard is cleared while old workers are still writing to the database, so a new run can overlap them. Separately, a `GetTrackInfo` error (including cancellation) is counted as `Enriched`.

**Fix:** Use a context-aware semaphore, join all started workers on every exit, and count errors correctly. Add cancellation and race-detector tests.

### F21 - DJ selection discards eligible songs before excluding used tracks

**Evidence:** `backend/internal/dj/sequencer.go:44-47,315-353`.  
**Validation:** Source-traced.

The scored pool is truncated to 50 before used song IDs receive zero weight, and scoring does not consider used IDs. When most of the top 50 are used, the loop exits with the phase short, most likely in `BuildQueue`, which gives every phase the same candidate list. The global song-count check in the semantic DJ path does not cover per-phase shortfalls. Side issue: removing picks with `append(pool[:i], pool[i+1:]...)` rewrites the caller's `scored` slice in place.

**Fix:** Exclude used IDs before truncation and replenish the sampling window; copy the pool before mutating. Test overlapping phases with pools larger than 50.

### F22 - Spotify concurrent-stream limit is never enforced

**Evidence:** `backend/internal/spotify/streamer.go:140-144,194-201,292-295`; `backend/internal/api/spotify.go:1675`.  
**Validation:** Source-traced.

The originally reported race is real: the count is read under a read lock, and the stream is registered only after session acquisition and track pinning. More importantly, the API constructs a new `Streamer` for every request (`spotify.NewStreamer` at `api/spotify.go:1675`, the only call site). Each request sees an empty `activeStreams` map, so the `maxConcurrent: 5` limit is never applied. The only effective throttle is the audio-key request serialization in the session.

**Fix:** Share one `Streamer` per session manager/API instance, then reserve capacity atomically before preparation (single write lock or a buffered-channel semaphore) and release on failure or close. Test simultaneous stream starts through the HTTP handler, not only the `Streamer` type.

## Build, release, and maintainability

### F23 - Repeated macOS packaging (resolved in current source)

**Previous priority:** P2.  
**Current evidence:** `scripts/build-wails-macos.sh:207-233,240-263`; commit `9240afd` (`Fix stale macOS app after rebuild`, changes only this script).  
**Validation:** Source-traced fix; native repeated packaging was not rerun.

The current script clears previous Wails bundles, removes the published destination (`rm -rf "$OUTPUT_APP"`) before copying, checks the copied executable exists, and fails if it differs from the source (`cmp -s`). The original F23 finding no longer applies. A two-build native macOS regression remains useful validation.

### O01 - Shell scripts are CRLF in Windows working copies (and OneDrive-synced copies)

**Evidence:** `git ls-files --eol scripts/*.sh` reports `i/lf w/crlf` for `build-wails-linux.sh`, `build-wails-macos.sh`, `build-web-linux.sh`, and `build-web-macos.sh`; `core.autocrlf=true` locally; no `.gitattributes` in the repository.  
**Validation:** Direct inspection in the verification pass.

*(Correction: the original wording implied a repository defect.)* The committed blobs are LF, so a fresh Git checkout on macOS/Linux gets working scripts. The CRLF files come from this Windows checkout's `core.autocrlf=true` conversion. The practical risk is running a Windows working copy on Unix, which a OneDrive-synced folder makes easy: Unix Bash fails on `\r`, which is what the prior review observed. The verification pass ran `bash -n` under Git for Windows Bash, and both CRLF and LF variants passed. Git Bash tolerates CRLF, so that result does not reproduce the Unix failure.

**Recommendation:** Add `.gitattributes` with `*.sh text eol=lf` so every checkout gets LF regardless of `core.autocrlf`, and keep Unix-side script validation in CI.

### O02 - Release backend gates are weaker than main CI

**Evidence:** `.github/workflows/release.yml:70-76,88-92,343-349,589-591`; `.github/workflows/ci.yml:74-101,103-212`.  
**Validation:** Source-traced; partially correct as originally written.

Release backend validation is `go test ./...` and `go vet ./...` (plus `govulncheck` on the Windows binary). Main CI additionally runs `go test -race`, Staticcheck, `govulncheck` on web and Wails binaries, the three-OS analysis matrix and determinism check, and semantic cross-compilation. The release workflow triggers on push to `main` (`.release-version`), PR, or dispatch, and has no dependency on CI; branch protection is not visible from the repository.

*(Correction: release does run the full frontend gate: `npm ci`, `npm run check` (palette, raw colors, `tsc --noEmit`, Vitest, build), and `npm audit`.)*

**Recommendation:** Reuse the CI jobs via `workflow_call`, add at least race tests and Staticcheck to release, or explicitly require a successful CI run for the same commit before publication.

### O03 - TypeScript strict mode is not enabled

**Evidence:** `tsconfig.json:1-28`.

No `strict`, `strict*`, or `noImplicit*` options are set, and `allowJs` is enabled. This reduces compile-time protection around nullability and implicit typing.

**Recommendation:** Adopt strict checking incrementally, beginning with persistence, playback, and API boundary modules.

### O04 - Styling checks permit existing inconsistencies

**Evidence:** `package.json:9-15`; `scripts/check-no-tailwind-palette.mjs:4,72-83`; `scripts/check-no-raw-colors-tsx.mjs:70-86`; executed style checks.

The palette check reported 31 files and exited 0, because it fails only with `--strict` and `package.json` does not pass that flag. The raw-color check is a per-file ratchet that accepted 102 legacy literals across 15 files. Its baseline allows 103, so one file is below its allowance and nothing tightens the baseline automatically. The hex check scans only `.tsx` files.

**Recommendation:** Treat this as lower-priority maintenance work. Prevent new regressions (auto-tighten the baseline, cover `.ts`/CSS), and reduce the existing baseline; do not prioritize styling over data integrity.

### O05 - Several modules concentrate substantial responsibility

**Evidence:** Line counts at `9240afd` (re-verified): `backend/internal/db/db.go` (5,236), `pages/Settings.tsx` (3,407), `lib/djAudio.ts` (3,011), `backend/internal/api/api.go` (2,890), and `services/api.ts` (2,427).

Large files are not defects by themselves, but they increase the scope of changes and make ownership and lifecycle reasoning harder.

**Recommendation:** After correctness fixes, separate persistence, synchronization, playback lifecycle, and settings responsibilities behind tested boundaries.

## Strengths

- Coherent unified local/Plex catalog, with Spotify handled as a separate integration. See `README.md` (Overview, Architecture) and `docs/index.md` (Source model).
- Main CI defines frontend checks, backend race tests, static analysis, vulnerability scanning, cross-platform analysis comparisons, semantic cross-compilation, and desktop packaging. Configuration is not proof that the latest run passed.
- Reviewed tests address database connection policies, snapshot/delta behavior, job recovery, scanner identity, malformed analysis data, interrupted DJ loads, and stale responses.
- Several subsystems already contain the correct pattern for a finding elsewhere, which should make fixes cheap:
  - `ListSongsPage` uses a read-only transaction (relevant to F15).
  - The semantic service's retention gap test (F16).
  - `DetectDeletedFiles` uses `os.IsNotExist` (F02, F04).
  - `GetTrackTopTags` returns real tag counts (F19).
- Audio and analysis code includes bounded chunking, unsupported-codec handling, finite-vector validation, and transactional conversion safeguards.
- `package.json` and `package-lock.json` agree (prior review). The local `node_modules` does not match them (see below).

## Validation results and limits

All source findings were re-traced against commit `9240afd` in the verification pass. Isolated reproductions and other checks marked **prior review** were not rerun.

| Check | Result | Interpretation |
| --- | --- | --- |
| Source re-trace of F01-F22 (verification) | All confirmed; several broadened, some narrowed, citations corrected | Static tracing of implementation, callers, and tests; not runtime execution |
| File/test inventory (verification) | 799 tracked files; 651 Go/TS files; 127 Go and 48 TS test files | Matches original counts |
| Python benchmark-contract tests (prior review) | 5 passed | Limited to the development benchmark contract; not rerun |
| Raw-color baseline check (verification) | Exit 0; 102 literals across 15 files accepted | Baseline allows 103; ratchet not tightened |
| Palette check (verification) | 31 files reported; exit 0 | Non-strict mode never fails |
| Shell script line endings (verification) | Index LF, working tree CRLF via `core.autocrlf=true` | Local-checkout artifact; see O01 |
| Shell script syntax (verification) | `bash -n` passed for CRLF and LF under Git for Windows Bash | Git Bash tolerates CRLF; prior-review Unix failure not reproducible here |
| Frontend defect reproductions (prior review) | Seven scenarios reproduced in isolation | Source re-traced and consistent; not rerun or live browser tests |
| macOS repeated-copy behavior (prior review) | Reproduced using temporary fixtures | Fixed in `9240afd`; native two-build packaging still untested |
| Package/lock agreement (prior review) | Passed | No vulnerability conclusion |
| Local `node_modules` state (verification) | Incomplete/stale | `vitest`, `jsdom`, `@playwright/test`, `@types/react`, `@types/react-dom` declared but not installed; `vite` 6.4.1 installed vs 6.4.3 declared |
| TypeScript typecheck (verification) | `tsc --noEmit` exit 2, 77 errors | **Inconclusive.** 49 are missing `vitest` modules and most others follow from missing `@types/react` (ErrorBoundary `state`/`props`, `React` namespace). Rerun after `npm ci` before attributing any errors to source. |
| Vitest tests (verification) | Could not start | `vitest` not installed, despite being declared at 3.2.7; environment issue, not a project defect |
| Go tests/race checks (verification) | Could not start | Installed Go 1.25.4 is below `backend/go.mod` requirement `go 1.26.8` |
| Native builds and installer execution | Not completed | No cross-platform runtime validation |
| Live Plex, Spotify, AI, and audio-device tests | Not performed | Integration findings remain source-traced |

No packages were installed and no application source files were modified. Current GitHub CI outcomes and deployed behavior were not verified. Findings are based on source in the local checkout; source-level risks have not all been reproduced at runtime.

## Recommended remediation sequence

### 1. Protect catalog data

- [ ] F02 and F04: Suppress deletions for unavailable/errored roots, require `IsNotExist` plus root availability, and fix the prefix-boundary match. Highest blast radius: an offline root can empty its catalog.
- [ ] F01: Preserve ignored records through full scans.
- [ ] F03: Enforce offline exclusivity in `viib-restore` and make rollback SQLite-consistent, including crash/WAL cases.
- [ ] Add failure-injection regressions before changing destructive behavior.

### 2. Correct metadata and playback ownership

- [ ] F05: Bind metadata drafts to song identity (keyed dialog).
- [ ] F06 and F13: Guard delayed playback actions with generation/ownership checks.
- [ ] F09 and F12: Preserve queue-occurrence identity and valid URL lifetimes.
- [ ] F10 and F11: Make persistence failures visible and recoverable.

### 3. Make synchronization and integration recovery reliable

- [ ] F14-F17: Fix scanning shortcuts, consistent revisions, expired cursors, and visibility tombstones.
- [ ] F07-F08 and F18: Handle partial indexing, configuration identity, and index snapshot lifetimes.
- [ ] F22: Share one Spotify `Streamer` and reserve capacity atomically.
- [ ] F19-F21: Correct tag-count handling, cancellation joins, and DJ candidate selection.

### 4. Validate release readiness

- [x] F23: Replace the stale macOS destination bundle and verify the copied executable (implemented in `9240afd`; native two-build validation remains outstanding).
- [ ] O01: Add `.gitattributes` enforcing LF for `*.sh`; validate scripts on Unix.
- [ ] O02: Add race tests and Staticcheck to release, or gate release on CI for the same commit.
- [ ] Restore a working toolchain (`npm ci`; Go ≥ 1.26.8), then run complete frontend tests/type checks and Go tests/race/static checks.
- [ ] Exercise interrupted scans, inaccessible shares, crash recovery, repeated playback transitions, and offline/online reconnection.
- [ ] Test real Plex/Spotify/AI integrations and audio devices on supported desktop platforms.
- [ ] Verify installed-package startup and upgrades, not only successful compilation.

## Revision notes (September 29 verification pass)

- **Broadened:**
  - **F01:** the ignore state is lost and the duplicate re-imports on alternate scans.
  - **F02:** an offline root triggers wholesale deletion; the prefix match has no path boundary; the fallback is likely the default for non-admin Windows users.
  - **F04:** ENOENT on unmounted roots; no confirmation; recomputed delete list.
  - **F09:** previous, direct selection, and retry are also affected.
  - **F10:** unknown ID, unawaited browser write, stale dialog.
  - **F11:** the playlist is also lost on restart.
  - **F13:** the request-ID guard does not help, and the auto-advance timer is also affected.
  - **F22:** the limit is never enforced, because a new `Streamer` is created per request.
- **Narrowed:**
  - **F03:** offline CLI only; requires an unclean shutdown or concurrent app.
  - **F06:** the default window is about 300 ms.
  - **F07:** recovers on next launch.
  - **F08:** fails loudly.
  - **F14:** the signature path does catch direct-child edits.
  - **F18:** narrow window, no memory-safety issue.
  - **F19:** only genres affected.
  - **F20:** result not consumed on error; the real impact is overlapping runs.
- **Corrected:**
  - **F12:** wrong persistence citation; browser-only scope.
  - **F17:** citation.
  - Minor line ranges in F01, F05, F06, F14, F18, F22, and F23.
  - **O01:** the repository stores LF; the problem is local conversion.
  - **O02:** release does run the frontend gate.
  - **Validation table:** the typecheck failure is attributable to an incomplete `node_modules`; Bash is available here via Git for Windows.
- **Reprioritized:** F02/F04 moved to the top of remediation step 1; F22 split out as its own item.

## Conclusion

The main recommendation is to stabilize failure handling and asynchronous state transitions before expanding features. The most urgent single change is to stop treating inaccessible media as deleted (F02/F04). The architecture provides a useful foundation, and several of the needed patterns already exist elsewhere in the codebase. Data-safety, playback, synchronization, and recovery regressions still need explicit tests and fixes before a release-readiness decision.
