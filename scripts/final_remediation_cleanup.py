#!/usr/bin/env python3
"""Generate the final remediation cleanup patch against the current branch."""
from pathlib import Path


def replace_once(path_name: str, old: str, new: str) -> None:
    path = Path(path_name)
    text = path.read_text(encoding="utf-8")
    if old not in text:
        raise RuntimeError(f"anchor missing in {path_name}: {old[:120]!r}")
    path.write_text(text.replace(old, new, 1), encoding="utf-8", newline="\n")


# Remove adjacent migration duplicates and close services in both entrypoints.
replace_once(
    "backend/cmd/viib/main.go",
    "\tdefer apiHandler.Close()\n\tdefer apiHandler.Close()\n",
    "\tdefer apiHandler.Close()\n",
)
replace_once(
    "backend/cmd/wails/main.go",
    '\tapiHandler := api.New(database, *dataDir)\n\tlogger.Main("API handler created")',
    '\tapiHandler := api.New(database, *dataDir)\n\tdefer apiHandler.Close()\n\tlogger.Main("API handler created")',
)
replace_once(
    "backend/internal/api/download_manager.go",
    "\tgo dm.broadcastProgress()\n\n\tgo dm.broadcastProgress()\n",
    "\tgo dm.broadcastProgress()\n",
)

security_middleware = '''\tr.Use(func(next http.Handler) http.Handler {
\t\treturn http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
\t\t\tw.Header().Set("X-Content-Type-Options", "nosniff")
\t\t\tw.Header().Set("Referrer-Policy", "no-referrer")
\t\t\tw.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
\t\t\tw.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
\t\t\tnext.ServeHTTP(w, req)
\t\t})
\t})
'''
replace_once(
    "backend/internal/server/server.go",
    security_middleware + security_middleware,
    security_middleware,
)

# Composite album identity in the primary selector.
path = Path("store.ts")
text = path.read_text(encoding="utf-8")
start = text.index("export const useAlbums = () => {")
end = text.index("/**\n * Splits an artist string", start)
replacement = '''export const useAlbums = () => {
  const songs = useStore((state) => state.songs);
  return useMemo(() => {
    const albumsMap = new Map<string, Album>();

    songs.forEach((song) => {
      const artist = song.albumArtist || song.artist || 'Unknown Artist';
      const key = `${song.album}::${artist}`;
      const existing = albumsMap.get(key);
      if (existing) {
        existing.songCount += 1;
        existing.addedAt = Math.max(existing.addedAt || 0, song.addedAt || 0);
        if (!existing.coverUrl && song.coverUrl) existing.coverUrl = song.coverUrl;
        return;
      }
      albumsMap.set(key, {
        name: song.album || 'Unknown Album',
        artist,
        songCount: 1,
        coverUrl: song.coverUrl,
        addedAt: song.addedAt || 0,
      });
    });

    return Array.from(albumsMap.values()).sort(
      (a, b) => (b.addedAt || 0) - (a.addedAt || 0) || a.name.localeCompare(b.name) || a.artist.localeCompare(b.artist),
    );
  }, [songs]);
};

'''
path.write_text(text[:start] + replacement + text[end:], encoding="utf-8", newline="\n")

# Complete frontend DTO mapping for Last.fm enrichment.
replace_once(
    "types.ts",
    '''  moodAnalyzedAt?: number; // timestamp of mood analysis

  // User preferences''',
    '''  moodAnalyzedAt?: number; // timestamp of mood analysis

  // Last.fm enrichment
  lastfmListeners?: number;
  lastfmPlaycount?: number;
  lastfmTags?: string;
  lastfmUrl?: string;
  lastfmMbid?: string;
  lastfmEnrichedAt?: number;

  // User preferences''',
)
replace_once(
    "services/backendService.ts",
    '''    moodAnalyzedAt: apiSong.moodAnalyzedAt,
    liked: apiSong.liked,''',
    '''    moodAnalyzedAt: apiSong.moodAnalyzedAt,
    lastfmListeners: apiSong.lastfmListeners,
    lastfmPlaycount: apiSong.lastfmPlaycount,
    lastfmTags: apiSong.lastfmTags,
    lastfmUrl: apiSong.lastfmUrl,
    lastfmMbid: apiSong.lastfmMbid,
    lastfmEnrichedAt: apiSong.lastfmEnrichedAt,
    liked: apiSong.liked,''',
)

# Sensitive setting reads report actual configured state without returning values.
replace_once(
    "backend/internal/api/api.go",
    '''\tif validation.IsSensitiveSettingKey(key) {
\t\trespondJSON(w, map[string]string{"key": key, "value": "", "configured": "true"})
\t\treturn
\t}
''',
    '''\tif validation.IsSensitiveSettingKey(key) {
\t\tvalue, err := a.db.GetSetting(key)
\t\tif err != nil {
\t\t\trespondError(w, http.StatusInternalServerError, "Failed to get setting")
\t\t\treturn
\t\t}
\t\trespondJSON(w, map[string]interface{}{"key": key, "value": "", "configured": value != ""})
\t\treturn
\t}
''',
)

# Scanner: use album-artist identity and make enrichment cancellation-aware.
path = Path("backend/internal/scanner/scanner.go")
text = path.read_text(encoding="utf-8")
old = '''\t// Background enrichment
\tenrichmentQueue        chan []db.Song
\tenrichmentMutex        sync.Mutex
\tenrichmentTotal        int
\tenrichmentProcessed    int
\tenrichmentBatchNum     int
\tenrichmentTotalBatches int
\tenrichmentActive       bool
'''
new = '''\t// Background enrichment
\tenrichmentQueue        chan []db.Song
\tenrichmentMutex        sync.Mutex
\tenrichmentTotal        int
\tenrichmentProcessed    int
\tenrichmentBatchNum     int
\tenrichmentTotalBatches int
\tenrichmentActive       bool
\tctx                    context.Context
\tcancel                 context.CancelFunc
\tenrichmentWg           sync.WaitGroup
\tcloseOnce              sync.Once
'''
if old not in text:
    raise RuntimeError("scanner enrichment fields anchor missing")
text = text.replace(old, new, 1)
text = text.replace(
    '''func New(database *db.DB, dataDir string) *Scanner {
\tcoverDir := filepath.Join(dataDir, "covers")
\tos.MkdirAll(coverDir, 0700)

\ts := &Scanner{''',
    '''func New(database *db.DB, dataDir string) *Scanner {
\tcoverDir := filepath.Join(dataDir, "covers")
\tos.MkdirAll(coverDir, 0700)
\tctx, cancel := context.WithCancel(context.Background())

\ts := &Scanner{''',
    1,
)
text = text.replace(
    '''\t\tenrichmentQueue: make(chan []db.Song, 1000), // Buffer for pending batches
\t}''',
    '''\t\tenrichmentQueue: make(chan []db.Song, 1000), // Buffer for pending batches
\t\tctx:             ctx,
\t\tcancel:          cancel,
\t}''',
    1,
)
text = text.replace(
    '''\t// Start background enrichment worker
\tgo s.processEnrichmentQueue()
''',
    '''\t// Start background enrichment worker
\ts.enrichmentWg.Add(1)
\tgo func() {
\t\tdefer s.enrichmentWg.Done()
\t\ts.processEnrichmentQueue()
\t}()
''',
    1,
)
text = text.replace(
    '''\t\t// Get or create album cover
\t\tcoverPath := s.getAlbumCover(song.Artist, song.Album, filepath.Dir(path), path)
''',
    '''\t\t// Get or create album cover
\t\tcoverArtist := song.AlbumArtist
\t\tif coverArtist == "" {
\t\t\tcoverArtist = song.Artist
\t\t}
\t\tcoverPath := s.getAlbumCover(coverArtist, song.Album, filepath.Dir(path), path)
''',
    1,
)
text = text.replace(
    '''\tfor _, song := range songs {
\t\t// Create album key in the same format as frontend: "album::artist"
\t\talbumKey := fmt.Sprintf("%s::%s", song.Album, song.Artist)
''',
    '''\tfor _, song := range songs {
\t\t// Create album key in the same format as frontend: "album::albumArtist".
\t\talbumArtist := song.AlbumArtist
\t\tif albumArtist == "" {
\t\t\talbumArtist = song.Artist
\t\t}
\t\talbumKey := fmt.Sprintf("%s::%s", song.Album, albumArtist)
''',
    1,
)
text = text.replace("\t\t\tArtistName:     song.Artist,\n", "\t\t\tArtistName:     albumArtist,\n", 1)

process_start = text.index("// processEnrichmentQueue handles background enrichment of songs")
process_replacement = '''// processEnrichmentQueue handles background enrichment of songs.
func (s *Scanner) processEnrichmentQueue() {
\tlogger.ScannerDebug("Enrichment worker started")
\tfor {
\t\tvar batch []db.Song
\t\tselect {
\t\tcase <-s.ctx.Done():
\t\t\treturn
\t\tcase batch = <-s.enrichmentQueue:
\t\t}

\t\tprovider, err := llm.GetConfiguredProvider(s.db)
\t\tif err != nil {
\t\t\tcontinue
\t\t}

\t\ts.enrichmentMutex.Lock()
\t\ts.enrichmentBatchNum++
\t\tcurrentBatch := s.enrichmentBatchNum
\t\ttotalBatches := s.enrichmentTotalBatches
\t\ttotalSongs := s.enrichmentTotal
\t\tisFirstBatch := currentBatch == 1
\t\ts.enrichmentMutex.Unlock()

\t\tif isFirstBatch {
\t\t\tlogger.Scanner("Starting enrichment of %d songs needing genres across %d batches using %s", totalSongs, totalBatches, provider.GetProviderName())
\t\t\ts.emitEvent(LibraryEvent{
\t\t\t\tType:    "enrichment_started",
\t\t\t\tMessage: fmt.Sprintf("Enriching %d songs", totalSongs),
\t\t\t\tData:    map[string]interface{}{"totalSongs": totalSongs, "totalBatches": totalBatches},
\t\t\t})
\t\t}

\t\tfor attempt := 0; attempt < 3; attempt++ {
\t\t\tenrichedGenres, enrichErr := provider.EnrichGenres(s.ctx, batch)
\t\t\tif enrichErr == nil {
\t\t\t\tcount := 0
\t\t\t\tfor songID, genres := range enrichedGenres {
\t\t\t\t\tif err := s.db.UpdateSongGenres(songID, genres); err == nil {
\t\t\t\t\t\tcount++
\t\t\t\t\t}
\t\t\t\t}
\t\t\t\tif count > 0 {
\t\t\t\t\ts.enrichmentMutex.Lock()
\t\t\t\t\ts.enrichmentProcessed += count
\t\t\t\t\tprocessedNow := s.enrichmentProcessed
\t\t\t\t\tisComplete := s.enrichmentBatchNum >= s.enrichmentTotalBatches
\t\t\t\t\tif isComplete {
\t\t\t\t\t\ts.enrichmentActive = false
\t\t\t\t\t}
\t\t\t\t\ts.enrichmentMutex.Unlock()
\t\t\t\t\tif err := s.db.UpdateGenreStats(); err != nil {
\t\t\t\t\t\tlogger.Scanner("Failed to update genre stats after enrichment: %v", err)
\t\t\t\t\t}
\t\t\t\t\ts.emitEvent(LibraryEvent{
\t\t\t\t\t\tType:    "enrichment_progress",
\t\t\t\t\t\tMessage: fmt.Sprintf("Enriched batch %d/%d (%d songs)", currentBatch, totalBatches, count),
\t\t\t\t\t\tData: map[string]interface{}{
\t\t\t\t\t\t\t"processedSongs": processedNow,
\t\t\t\t\t\t\t"totalSongs":     totalSongs,
\t\t\t\t\t\t\t"currentBatch":   currentBatch,
\t\t\t\t\t\t\t"totalBatches":   totalBatches,
\t\t\t\t\t\t},
\t\t\t\t\t})
\t\t\t\t\tif isComplete {
\t\t\t\t\t\ts.emitEvent(LibraryEvent{
\t\t\t\t\t\t\tType:    "enrichment_complete",
\t\t\t\t\t\t\tMessage: fmt.Sprintf("Genre enrichment complete: %d songs", processedNow),
\t\t\t\t\t\t\tData:    map[string]interface{}{"processedSongs": processedNow, "totalSongs": totalSongs},
\t\t\t\t\t\t})
\t\t\t\t\t}
\t\t\t\t}
\t\t\t\tbreak
\t\t\t}

\t\t\tif s.ctx.Err() != nil {
\t\t\t\tprovider.Close()
\t\t\t\treturn
\t\t\t}
\t\t\tif strings.Contains(enrichErr.Error(), "429") || strings.Contains(strings.ToLower(enrichErr.Error()), "quota") {
\t\t\t\tlogger.Scanner("LLM rate limit hit, waiting 30s (attempt %d/3)...", attempt+1)
\t\t\t\tselect {
\t\t\t\tcase <-s.ctx.Done():
\t\t\t\t\tprovider.Close()
\t\t\t\t\treturn
\t\t\t\tcase <-time.After(30 * time.Second):
\t\t\t\t}
\t\t\t\tcontinue
\t\t\t}
\t\t\tlogger.Scanner("LLM enrichment failed: %v", enrichErr)
\t\t\tbreak
\t\t}
\t\tprovider.Close()
\t\tselect {
\t\tcase <-s.ctx.Done():
\t\t\treturn
\t\tcase <-time.After(2 * time.Second):
\t\t}
\t}
}
'''
text = text[:process_start] + process_replacement
path.write_text(text, encoding="utf-8", newline="\n")

Path("backend/internal/scanner/lifecycle.go").write_text(
    '''package scanner

// Close stops scanner background services and waits for enrichment work to exit.
func (s *Scanner) Close() {
\ts.closeOnce.Do(func() {
\t\tif s.cancel != nil {
\t\t\ts.cancel()
\t\t}
\t\tif s.backgroundScanner != nil {
\t\t\ts.backgroundScanner.Stop()
\t\t}
\t\ts.enrichmentWg.Wait()
\t})
}
''',
    encoding="utf-8",
    newline="\n",
)

# Harden DNS dialing against rebinding and proxy bypass.
path = Path("backend/internal/mediafetch/mediafetch.go")
text = path.read_text(encoding="utf-8")
insert_at = text.index("\nfunc validateURL")
dialer = '''
func safeDialContext(ctx context.Context, network, address string) (net.Conn, error) {
\thost, port, err := net.SplitHostPort(address)
\tif err != nil {
\t\treturn nil, fmt.Errorf("invalid remote address: %w", err)
\t}
\tips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
\tif err != nil || len(ips) == 0 {
\t\treturn nil, errors.New("remote host could not be resolved")
\t}
\tfor _, ip := range ips {
\t\tif !isPublicIP(ip) {
\t\t\treturn nil, errors.New("private, loopback, or link-local remote hosts are blocked")
\t\t}
\t}
\tdialer := &net.Dialer{Timeout: 8 * time.Second, KeepAlive: 20 * time.Second}
\tvar lastErr error
\tfor _, ip := range ips {
\t\tconn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
\t\tif dialErr == nil {
\t\t\treturn conn, nil
\t\t}
\t\tlastErr = dialErr
\t}
\treturn nil, fmt.Errorf("failed to connect to remote host: %w", lastErr)
}
'''
text = text[:insert_at] + dialer + text[insert_at:]
text = text.replace("Proxy:                 http.ProxyFromEnvironment,", "Proxy:                 nil,", 1)
text = text.replace(
    "DialContext:           (&net.Dialer{Timeout: 8 * time.Second, KeepAlive: 20 * time.Second}).DialContext,",
    "DialContext:           safeDialContext,",
    1,
)
path.write_text(text, encoding="utf-8", newline="\n")

print("Final cleanup generated")
