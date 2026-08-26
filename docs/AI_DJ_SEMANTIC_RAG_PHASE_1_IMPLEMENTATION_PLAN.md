# AI DJ Semantic Retrieval / RAG — Phase 1 Implementation Plan

**Repository:** `ajbergh/ViiB-MediaHub`  
**Status:** Proposed / implementation-ready  
**Primary objective:** Replace metadata-dependent AI DJ candidate selection with a fast semantic music retrieval pipeline that can operate effectively over 20,000+ tracks even when genre, mood, album, year, BPM, and other local tags are incomplete.  
**Architecture decision:** Existing SQLite database remains authoritative; `github.com/coder/hnsw` provides the pure-Go approximate-nearest-neighbor index used as a rebuildable acceleration layer.  
**Phase 1 scope:** Text-semantic retrieval only. Audio-content embeddings are explicitly deferred.

---

## 1. Codex execution contract

This document is intended to be executable by Codex with minimal design interpretation. During implementation, Codex should treat the decisions in this document as requirements unless a verified repository constraint makes a specific item impossible.

### Required working behavior

- Work from the latest `main` before beginning each implementation PR.
- Create implementation branches/PRs in the order defined in **Section 18**.
- Keep this file updated in every implementation PR:
  - change phase status from `[ ]` to `[x]` only after code + tests for that item are complete;
  - add concise implementation notes where the final implementation differs from the planned file layout;
  - record any intentionally deferred work under **Phase 1 Deferred Items**.
- Do not replace or delete the current AI DJ path until the semantic path has a tested fallback strategy.
- Do not send the complete song library, large genre catalog, or thousands of track rows to an LLM.
- Never allow the LLM to invent final song IDs. All final playlist entries must come from local ViiB catalog IDs returned by deterministic retrieval.
- Preserve unified local/Plex catalog behavior. Semantic retrieval is source-agnostic until normal ViiB source filters are applied.
- Do not add a server process, Docker dependency, Python runtime, CGO dependency, or external vector database.
- Do not auto-download an embedding model without explicit user action.
- Do not block application startup on first-time semantic indexing.
- Do not recompute embeddings when the semantic document content hash and embedding model identity are unchanged.
- Batch embedding calls. A 20,000-track library must never cause one HTTP request per track.
- All new background work must honor `context.Context` cancellation and application shutdown.
- Run backend tests, frontend tests/type checking, linting, and production builds appropriate to the repository before marking each PR complete.

---

## 2. Problem statement

The current AI DJ architecture contains useful planning, scoring, persona, recency, and sequencing logic, but candidate discovery depends too heavily on structured metadata that is frequently absent or inconsistent.

Current relevant implementation:

- `backend/internal/dj/dj_set_planner.go`
  - LLM produces coarse DJ phases such as energy, tempo, mood, BPM range, and target count.
- `backend/internal/dj/scoring.go`
  - scores songs using energy, tempo, mood, BPM, listening behavior, genre affinity, recency, and artist repetition.
  - unknown metadata receives neutral scores, which causes weakly tagged tracks to become difficult to distinguish semantically.
- `backend/internal/dj/sequencer.go`
  - partitions candidates primarily by energy/tempo buckets and relaxes constraints when a bucket is sparse.
- `backend/internal/llm/provider.go`
  - parses natural-language prompts into structured metadata filters and currently attempts to align prompts to genres already present in the library.
- `backend/internal/llm/enrichment.go`
  - already contains optional AI enrichment for genres, mood, energy, tempo, BPM, instrumental state, and original year.
- `backend/internal/lastfm/enricher.go`
  - already stores Last.fm tags, similar tracks/artists, and artist metadata that can improve semantic context.
- `backend/internal/db/db.go`
  - stores unified local/Plex songs plus listening behavior and enrichment fields.
- `slices/aiDjSlice.ts` / `pages/SmartPlaylists.tsx`
  - contain the persisted AI DJ user experience and options.

The Phase 1 goal is to preserve the good parts of the current system while replacing the weak candidate-discovery layer.

---

## 3. Architecture decision

### 3.1 Selected vector approach

Use:

- **SQLite** for semantic documents, embeddings, status, content hashes, and rebuild authority.
- **`github.com/coder/hnsw`** for in-memory nearest-neighbor search and binary graph persistence.
- **Three logical HNSW graphs:** track, album, and artist.

Pin the dependency to the June 22, 2026 recall-fix revision or a later specifically reviewed revision. Do **not** depend on `@main` in committed code.

Reference revision at planning time:

```text
36cab6028fed4adc9c3edf2323a06f0a95c1f030
```

That revision contains the corrected HNSW search termination/result-set behavior. If Codex upgrades beyond this commit, inspect intervening changes and record the chosen revision in this document.

### 3.2 Why this is the chosen design

- Pure Go.
- No CGO requirement.
- No additional daemon or sidecar.
- Fast in-memory ANN search suitable for 20k–100k scale libraries.
- Supports add/delete/search and graph export/import.
- SQLite stays the recovery source; a missing/corrupt HNSW file is never data loss.
- HNSW implementation remains behind a ViiB interface so it can be replaced later without changing the AI DJ contract.

### 3.3 HNSW graph configuration

Initial Phase 1 defaults:

```text
Distance   = cosine
M          = 16
Ml         = 0.25
EfSearch   = 128
Key type   = int64 (semantic_documents.id)
Vector type = []float32
```

Do not expose these as user settings in Phase 1. Keep them named constants so future tuning does not affect storage/API contracts.

The graph is an acceleration layer only. Do not rely on graph serialization as the sole storage location for embeddings.

---

## 4. Target architecture

```text
                   ┌─────────────────────────┐
User prompt ──────►│ LLM Intent Compiler     │
                   │ no library dump         │
                   └────────────┬────────────┘
                                │ PlaylistIntent / phase queries
                                ▼
                   ┌─────────────────────────┐
                   │ Embedding Provider      │
                   │ query -> []float32      │
                   └────────────┬────────────┘
                                │
             ┌──────────────────┼──────────────────┐
             ▼                  ▼                  ▼
      Track HNSW index    Album HNSW index   Artist HNSW index
             │                  │                  │
             └──────────────────┼──────────────────┘
                                ▼
                   ┌─────────────────────────┐
                   │ Semantic Retriever      │
                   │ expand + dedupe         │
                   │ hard filters            │
                   └────────────┬────────────┘
                                ▼
                   ┌─────────────────────────┐
                   │ Hybrid Ranker           │
                   │ semantic + behavior     │
                   │ metadata + diversity    │
                   └────────────┬────────────┘
                                ▼
                   ┌─────────────────────────┐
                   │ Existing DJ Sequencer   │
                   │ phase / BPM / flow      │
                   └────────────┬────────────┘
                                ▼
                         Valid ViiB song IDs
```

Background indexing path:

```text
ViiB songs / Last.fm / AI enrichment
              │
              ▼
   deterministic semantic documents
              │ content hash
              ▼
      batched embedding provider
              │
              ├────► SQLite embedding BLOB (authoritative)
              │
              └────► HNSW graph (rebuildable cache)
```

---

## 5. Phase 1 scope

### In scope

- Semantic documents for **tracks, albums, and artists**.
- Persistent embeddings in existing ViiB SQLite.
- Pure-Go HNSW indexes.
- Incremental/background index maintenance.
- Embedding provider abstraction.
- Phase 1 embedding providers:
  - Ollama local embedding endpoint.
  - OpenAI embeddings endpoint.
- Semantic query generation from the existing AI DJ LLM.
- Semantic retrieval for both normal AI playlist mode and DJ set mode.
- Hard exclusions and source filters.
- Behavioral reranking using likes, skips, plays, recency, and discovery/favorites settings.
- Diversity control / MMR-style selection.
- Existing DJ BPM/flow sequencing retained and updated to consume semantic relevance.
- Settings/status UI for semantic indexing.
- Automatic fallback to legacy AI DJ selection while indexing is unavailable.
- Test coverage, logging, metrics, and documentation.

### Explicitly out of scope for Phase 1

- CLAP/MuLan/audio-waveform embeddings.
- Python, ONNX, CUDA, CoreML, DirectML, or other audio ML runtimes.
- Lyrics ingestion or lyric embeddings.
- Internet-wide music search.
- A hosted vector database.
- Automatic metadata scraping outside integrations already present in ViiB.
- An LLM choosing arbitrary track names from its training knowledge.
- Final LLM editorial pass over retrieved tracks.
- Cross-user collaborative recommendation.
- Training a recommender model from listening history.

---

## 6. New backend package layout

Create a new package:

```text
backend/internal/semantic/
    types.go
    service.go
    document_builder.go
    document_builder_test.go
    embedding.go
    embedding_ollama.go
    embedding_openai.go
    embedding_test.go
    vector_codec.go
    vector_codec_test.go
    index.go
    hnsw_index.go
    hnsw_index_test.go
    index_manager.go
    index_manager_test.go
    retriever.go
    retriever_test.go
    ranker.go
    ranker_test.go
    mmr.go
    mmr_test.go
    lifecycle.go
```

Database methods may remain under `backend/internal/db/`, preferably split into:

```text
backend/internal/db/semantic.go
backend/internal/db/semantic_test.go
```

API handlers should be placed in:

```text
backend/internal/api/semantic.go
```

Avoid adding semantic concerns directly to the already-large `api.go` unless only route registration is required there.

---

## 7. Database schema and migrations

Add migration(s) in the existing migration mechanism. SQLite remains authoritative for semantic state.

### 7.1 `semantic_documents`

Recommended schema:

```sql
CREATE TABLE IF NOT EXISTS semantic_documents (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    entity_type TEXT NOT NULL CHECK(entity_type IN ('track','album','artist')),
    entity_key TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    song_id TEXT,
    artist TEXT NOT NULL DEFAULT '',
    album TEXT NOT NULL DEFAULT '',
    content TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    document_version INTEGER NOT NULL,
    embedding_provider TEXT NOT NULL DEFAULT '',
    embedding_model TEXT NOT NULL DEFAULT '',
    embedding_dimensions INTEGER NOT NULL DEFAULT 0,
    embedding BLOB,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK(status IN ('pending','embedding','ready','error')),
    retry_count INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    embedded_at INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(entity_type, entity_key)
);

CREATE INDEX IF NOT EXISTS idx_semantic_documents_status
    ON semantic_documents(entity_type, status);
CREATE INDEX IF NOT EXISTS idx_semantic_documents_song_id
    ON semantic_documents(song_id);
CREATE INDEX IF NOT EXISTS idx_semantic_documents_artist
    ON semantic_documents(artist);
```

Rules:

- Track `entity_key` = ViiB `song.ID`.
- Artist `entity_key` = stable hash/canonical key of normalized artist identity.
- Album `entity_key` = stable hash/canonical key of normalized album artist + album name.
- Do not use file paths as semantic identity.
- `content_hash` must include `document_version`.
- Changing semantic document templates increments `SemanticDocumentVersion` and causes affected documents to be regenerated.
- Store embedding values as little-endian float32 BLOBs, never JSON arrays.

### 7.2 `semantic_index_state`

```sql
CREATE TABLE IF NOT EXISTS semantic_index_state (
    entity_type TEXT PRIMARY KEY CHECK(entity_type IN ('track','album','artist')),
    document_revision INTEGER NOT NULL DEFAULT 0,
    graph_revision INTEGER NOT NULL DEFAULT 0,
    item_count INTEGER NOT NULL DEFAULT 0,
    graph_dimensions INTEGER NOT NULL DEFAULT 0,
    embedding_provider TEXT NOT NULL DEFAULT '',
    embedding_model TEXT NOT NULL DEFAULT '',
    graph_saved_at INTEGER NOT NULL DEFAULT 0,
    last_full_rebuild_at INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT ''
);
```

Behavior:

- Any insert/update/delete of a ready semantic document increments `document_revision` for its entity type.
- A graph save sets `graph_revision = document_revision`.
- Startup may trust a persisted HNSW graph only when:
  - graph file exists;
  - import succeeds;
  - dimensions/provider/model agree with DB state;
  - `graph_revision == document_revision`.
- Otherwise rebuild the graph from SQLite ready embeddings.

### 7.3 DB access methods

Implement explicit bulk methods. Avoid N+1 queries.

At minimum:

```go
UpsertSemanticDocuments(ctx context.Context, docs []SemanticDocument) error
GetPendingSemanticDocuments(ctx context.Context, entityType string, limit int) ([]SemanticDocument, error)
StoreSemanticEmbeddings(ctx context.Context, updates []EmbeddingUpdate) error
MarkSemanticDocumentError(...)
DeleteSemanticDocumentsForMissingSongs(...)
ListReadySemanticEmbeddings(ctx context.Context, entityType string) ([]StoredEmbedding, error)
GetSemanticDocumentsByIDs(ctx context.Context, ids []int64) ([]SemanticDocument, error)
GetSongsByIDs(ctx context.Context, ids []string) ([]Song, error)
GetSongsForSemanticAlbum(ctx context.Context, artist, album string, limit int) ([]Song, error)
GetSongsForSemanticArtist(ctx context.Context, artist string, limit int) ([]Song, error)
GetSemanticIndexState(...)
IncrementSemanticDocumentRevision(...)
SetSemanticGraphRevision(...)
GetSemanticIndexStats(...)
```

Use transactions for batch embedding writes + revision changes.

---

## 8. Semantic document generation

### 8.1 Core requirement

Phase 1 semantic documents are **deterministic**. Do not make a separate LLM call to write a prose description for every track. That would make first indexing slow, expensive, and non-reproducible.

Use existing ViiB and Last.fm metadata as evidence and encode it into natural-language-like text suitable for embeddings.

### 8.2 Track document

Example template:

```text
Track: Let Down.
Artist: Radiohead.
Album: OK Computer.
Album artist: Radiohead.
Original year: 1997.
Genres and styles: alternative rock, art rock, indie rock.
Community tags: melancholic, atmospheric, british, 90s, alternative.
Mood: melancholic.
Energy: medium.
Tempo: medium.
BPM: 103.
Vocals: vocal.
Artist context: alternative rock, art rock, experimental rock; similar artists include ...
Album context: 1990s alternative/art rock; common album tags include ...
```

Rules:

- Omit unknown fields instead of writing `unknown` repeatedly.
- Normalize whitespace and malformed tag values.
- Decode `LastFMTags` JSON safely; ignore corrupt JSON without failing indexing.
- Limit community tags to a useful top-N (recommended 12).
- Limit similar artist names (recommended 8).
- Do not include play count, liked state, skip count, last played, or other volatile user behavior in the embedding text.
- Include source-independent ViiB identity only as DB metadata, not semantic prose.
- Cap semantic text length (recommended 4 KiB) to prevent oversized embedding requests.

### 8.3 Artist document

Build one document per normalized artist using:

- artist name;
- aggregated genres from local songs;
- Last.fm artist tags;
- short cleaned Last.fm bio excerpt when present;
- similar artists available from Last.fm data;
- active years only when reliable data exists.

Do not embed full Last.fm biographies. Strip HTML and cap bio context (recommended 600–1,000 characters).

### 8.4 Album document

Build one document per normalized `(albumArtist, album)` pair using:

- album name and album artist;
- release/original year distribution;
- genres pooled from album tracks;
- common Last.fm tags across tracks;
- mood/energy distribution;
- instrumental/vocal makeup;
- track titles only when useful and capped.

### 8.5 Hierarchical context

Poorly tagged tracks should inherit context without modifying the canonical song metadata:

- Track document may include compact artist context.
- Track document may include compact album context.
- Separate album/artist embeddings remain retrieval channels in their own right.

This is critical: do **not** permanently overwrite a song genre merely because the artist usually belongs to that genre.

### 8.6 Content hashing

Compute:

```text
SHA-256(document_version + "\x00" + normalized_document_text)
```

If the hash is unchanged and provider/model/dimensions are unchanged, do not re-embed.

---

## 9. Embedding subsystem

### 9.1 Interface

Create an implementation-independent contract:

```go
type EmbeddingProvider interface {
    Name() string
    Model() string
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    MaxBatchSize() int
    Close() error
}
```

A provider response must be rejected when:

- vector count differs from input count;
- dimensions are inconsistent within a batch;
- dimensions conflict with the existing index model state;
- any element is NaN or Inf;
- a vector is empty/zero-length.

### 9.2 Provider configuration

Add settings:

```text
semantic_embedding_provider = auto | ollama | openai
semantic_embedding_model
semantic_embedding_dimensions
semantic_embedding_base_url
semantic_embedding_api_key   (encrypted using existing secret-setting path)
```

Do not expose HNSW tuning values.

### 9.3 Auto behavior

`auto` resolves in this order:

1. If current AI provider is Ollama, use Ollama embeddings with the same base URL.
2. If current AI provider is OpenAI, use the existing OpenAI key.
3. If a separate semantic embedding provider is already configured successfully, use it.
4. Otherwise semantic indexing state becomes `needs_configuration`; the rest of ViiB remains functional and AI DJ may use the legacy fallback.

### 9.4 Ollama provider

- Use Ollama's embedding endpoint at the configured local base URL.
- Keep embedding model independent from the chat model.
- Initial default model name may be `nomic-embed-text` when the user selects Ollama and leaves the field empty.
- Never auto-pull the model.
- Detect vector dimension from the first successful response and persist it.
- Use conservative concurrency (one active embedding request by default).
- Batch inputs; recommended starting batch size 32, configurable as an internal constant.

### 9.5 OpenAI provider

- Use `text-embedding-3-small` as the initial default.
- Request **512 dimensions** for Phase 1 unless the user explicitly chose a supported alternative model/dimension in settings.
- Batch aggressively (recommended internal batch size 128).
- Use the existing HTTP/retry conventions and encrypted secret storage.
- Avoid coupling the feature to an OmniLLM chat-client upgrade. A small provider adapter behind `EmbeddingProvider` is preferable to changing the entire chat stack solely for Phase 1 embeddings.

### 9.6 Embedding model changes

Provider, model, or dimension changes invalidate every existing semantic embedding.

Required behavior:

1. Mark all semantic documents pending.
2. Preserve old graph files until the new index is successfully usable.
3. Build new embeddings in background.
4. Build replacement graphs.
5. Atomically swap the active index when all three graph types pass validation.
6. Remove obsolete graph files only after successful replacement.

Do not leave AI DJ with a partially mixed vector space.

---

## 10. HNSW index subsystem

### 10.1 Interface

Do not expose `coder/hnsw` types outside the semantic package.

```go
type VectorIndex interface {
    Upsert(id int64, vector []float32) error
    Delete(id int64) error
    Search(vector []float32, k int) ([]VectorMatch, error)
    Len() int
    Dimensions() int
    Save(ctx context.Context) error
    Rebuild(ctx context.Context, items []StoredEmbedding) error
    Close() error
}
```

### 10.2 Index files

Place graph files beside/in the ViiB application data directory, not in the music library:

```text
semantic-track-v1.hnsw
semantic-album-v1.hnsw
semantic-artist-v1.hnsw
```

Never place Plex tokens, prompts, API keys, or file paths in graph keys.

### 10.3 Thread safety

Assume the upstream graph is not safe for unsynchronized mixed read/write access.

- Wrap each graph in a `sync.RWMutex`.
- Searches acquire read lock.
- Upsert/delete/rebuild/import swap acquire write lock.
- Do not hold graph locks while performing HTTP embedding calls or SQLite I/O longer than required.

### 10.4 Persistence

- Debounce graph saves after mutations (for example 5–10 seconds).
- Force save during graceful shutdown.
- Save only after SQLite embedding transaction is committed.
- If app crashes after DB commit but before graph save, revision mismatch triggers rebuild on next launch.
- Treat graph import failure as recoverable: log, rename/delete bad cache, rebuild from SQLite.

### 10.5 Rebuild strategy

Build a new graph off to the side from SQLite `ready` embeddings, validate count/dimensions, then atomically swap the pointer. Do not empty the active graph at the beginning of a rebuild.

---

## 11. Indexing lifecycle

### 11.1 Service lifecycle

`semantic.Service` should be created with the database and application data path, then started during backend initialization.

Startup sequence:

1. Load semantic settings.
2. Initialize embedding provider if configured.
3. Load/validate graph cache(s).
4. If graph revision mismatch exists, rebuild graph(s) from already stored embeddings immediately/background without re-embedding.
5. Scan catalog for semantic documents whose content hash changed or do not exist.
6. Queue pending embeddings.
7. Process batches in background.
8. Publish status/progress.

Application startup must not wait for step 5–7.

### 11.2 Initial build order

Recommended:

1. artist documents;
2. album documents;
3. track documents.

All document text can be generated first, but this order makes useful broad semantic retrieval available earlier.

### 11.3 Incremental updates

Trigger semantic invalidation when relevant catalog data changes:

- local scan adds/updates/deletes a track;
- Plex sync adds/updates/deletes a catalog track;
- Last.fm enrichment updates tags/bio/similar entities;
- LLM metadata enrichment updates genre/mood/energy/tempo/year;
- album/artist identity changes.

Do not trigger re-embedding for:

- play count;
- last played;
- skip count;
- liked state;
- playlist membership;
- playback source availability.

Those are ranking-time signals.

### 11.4 Retry behavior

- Retry transient provider failures with exponential backoff + jitter.
- Persist `retry_count` and `last_error`.
- After a bounded retry count, leave the document in `error` and continue indexing others.
- A single malformed track must never stop the queue.
- A manual rebuild/retry action resets eligible error rows.

---

## 12. LLM Intent Compiler

### 12.1 Replace genre-constrained planning

The current context-aware prompt attempts to constrain the LLM to genres already present in the user's tags. The new semantic architecture should stop doing that.

The LLM's role is to interpret musical meaning, not map the user into the library's imperfect taxonomy.

### 12.2 New common intent type

Add a common semantic intent type, preferably in `backend/internal/dj/types.go` or a dedicated intent file:

```go
type PlaylistIntent struct {
    IntentSummary        string   `json:"intentSummary"`
    SemanticQuery        string   `json:"semanticQuery"`
    NegativeSemanticQuery string  `json:"negativeSemanticQuery,omitempty"`
    IncludeArtists       []string `json:"includeArtists,omitempty"`
    ExcludeArtists       []string `json:"excludeArtists,omitempty"`
    PreferredGenres      []string `json:"preferredGenres,omitempty"`
    MinYear              int      `json:"minYear,omitempty"`
    MaxYear              int      `json:"maxYear,omitempty"`
    YearConstraintHard   bool     `json:"yearConstraintHard,omitempty"`
    InstrumentalOnly     bool     `json:"instrumentalOnly,omitempty"`
    DiscoveryBias        float64  `json:"discoveryBias,omitempty"`
    FamiliarityBias      float64  `json:"familiarityBias,omitempty"`
}
```

`SemanticQuery` must be a concise retrieval description, not the raw prompt blindly copied.

Example user prompt:

```text
90 minutes of darker 90s alternative, start mellow like Radiohead,
gradually get heavier, some obscure stuff, no Nirvana
```

Example structured intent:

```json
{
  "intentSummary": "Dark 1990s alternative rock with a gradual energy build and some deep cuts",
  "semanticQuery": "dark atmospheric introspective 1990s alternative and art rock with a melodic edge",
  "negativeSemanticQuery": "bright cheerful pop rock",
  "excludeArtists": ["Nirvana"],
  "minYear": 1990,
  "maxYear": 1999,
  "yearConstraintHard": false,
  "discoveryBias": 0.65,
  "familiarityBias": 0.35
}
```

### 12.3 DJ phases

Extend `DJPhase` with at least:

```go
SemanticQuery         string   `json:"semanticQuery"`
NegativeSemanticQuery string   `json:"negativeSemanticQuery,omitempty"`
StyleHints            []string `json:"styleHints,omitempty"`
```

The LLM should create a different semantic query for each phase when the user requests an arc.

Example:

```text
Warm-up: moody atmospheric introspective alternative rock, restrained energy
Build: melodic emotionally intense 1990s alternative rock, increasing drive
Peak: heavy driving noisy alternative rock, cathartic and energetic
Cooldown: spacious reflective alternative/art rock, lower intensity
```

### 12.4 Validation

- Parse strict JSON as the current planner already does.
- Normalize arrays and cap lengths.
- Maximum semantic query length: recommended 512 characters.
- Maximum artist exclusion/include list: recommended 50 each.
- Clamp numeric bias values to `[0,1]`.
- Invalid intent should fall back to a deterministic query derived from the raw prompt, not fail the whole request.

### 12.5 Planner cache

Update the current plan cache key:

- include normalized prompt, persona, duration, flow strictness;
- when `UseTimeContext` is enabled, include the time-of-day bucket in the cache key;
- remove library genre hash as a required semantic-planning dependency.

---

## 13. Semantic retrieval and candidate expansion

### 13.1 Retrieval quotas

Starting defaults per semantic query:

```text
Track graph:  top 300
Album graph:  top 40
Artist graph: top 30
Album expansion: max 8 local tracks per album result
Artist expansion: max 10 local tracks per artist result
Maximum deduped candidate pool before hard filters: 900
```

Keep as named internal constants.

### 13.2 Match normalization

`coder/hnsw` returns distance. Convert cosine distance to a stable similarity score before exposing it elsewhere.

Clamp score to `[0,1]`.

Each candidate should retain evidence:

```go
type SemanticEvidence struct {
    TrackSimilarity  float64
    AlbumSimilarity  float64
    ArtistSimilarity float64
    BestSimilarity   float64
    MatchedAlbum     string
    MatchedArtist    string
}
```

Recommended initial `BestSimilarity`:

```text
max(
  trackSimilarity,
  albumSimilarity  * 0.94,
  artistSimilarity * 0.88,
)
```

Track matches are preferred, while album/artist retrieval can rescue poorly tagged tracks.

### 13.3 Hard filters

Apply deterministic hard filters after expansion and before final ranking:

- explicit excluded artist;
- explicit included artist if user phrased it as mandatory;
- source (`all`, `local`, selected Plex behavior using existing source rules);
- instrumental-only when explicitly requested;
- year only when `YearConstraintHard` is true;
- inaccessible/deleted catalog identities.

A semantic similarity score may never override a hard exclusion.

### 13.4 Soft metadata signals

Use year, genre, mood, tempo, and energy as soft boosts when not hard constraints. Do not require those fields to exist.

### 13.5 Negative semantic query

When present:

- embed the negative query once;
- calculate its similarity only for the positive candidate set;
- subtract a bounded negative penalty;
- do not perform a second full-library negative search.

Recommended initial penalty:

```text
adjustedSemantic = clamp(positiveSimilarity - 0.25 * negativeSimilarity, 0, 1)
```

---

## 14. Hybrid ranking and personalization

### 14.1 Separation of concerns

Do not put volatile user behavior into embeddings.

Keep these at ranking time:

- liked state;
- play count;
- completion/skip rate;
- recently played state;
- artist affinity;
- genre affinity;
- discovery/favorites mode.

### 14.2 Standard playlist ranking

Create a reusable `HybridRanker` for non-DJ playlist mode.

Recommended base weighting:

```text
semantic relevance      70%
behavior/personal taste 20%
explicit metadata fit   10%
```

Then apply recency, artist repetition, exclusions, and discovery/favorites policy.

Do not force the exact numbers into public API contracts. Put weights in named constants and add tests that enforce intended ordering.

### 14.3 DJ scoring integration

Extend `ScoreContext` with:

```go
SemanticScores map[string]float64
```

Update `ScoreSongForPhase` so semantic relevance becomes the dominant musical-fit input.

Recommended Phase 1 weighting:

```text
semantic relevance   50%
phase metadata fit   20%
BPM continuity       15%
persona/behavior     10%
genre affinity        5%
```

Then apply existing penalties.

This specifically prevents missing mood/energy/tempo fields from dominating the outcome merely because they currently receive neutral scores.

### 14.4 Diversity / MMR

Add an MMR-style selection pass before final sequencing to avoid clusters of almost-identical results.

Conceptually:

```text
MMR(candidate) = lambda * relevance
               - (1-lambda) * maxSimilarity(candidate, alreadySelected)
```

Initial lambda:

```text
favorites: 0.85
balanced:  0.75
discover:  0.60
```

Additional diversity rules:

- honor existing one-per-artist setting;
- when one-per-artist is off, still apply a soft artist repetition penalty;
- avoid excessive same-album runs unless the user explicitly asked for an album-focused result;
- do not make MMR pairwise calculations over all 900 candidates; restrict it to the top ranked working set (for example top 100–150).

---

## 15. Existing DJ sequencer changes

Keep `backend/internal/dj/sequencer.go`. Do not rewrite it wholesale.

Required changes:

- Accept semantic candidate scores/evidence.
- Stop using energy/tempo bucket selection as the primary recall mechanism.
- Semantic retriever should produce the phase candidate pool first.
- Existing energy/tempo/BPM logic remains useful for phase fit and ordering.
- Keep stochastic selection, artist tracking, BPM sorting, persona logic, and micro-shuffle where they remain valid.
- Candidate relaxation should broaden semantic candidate counts or soft constraints, not immediately fall back to arbitrary whole-library metadata buckets.
- Only use all-library fallback if semantic indexing is unavailable or retrieval returns an unusably small result after deterministic recovery steps.

Recommended per-phase flow:

```text
phase semantic query
 -> semantic retrieval
 -> hard filters
 -> hybrid ranking
 -> diversity pass
 -> ScoreSongForPhase
 -> stochastic top pool selection
 -> BPM/flow ordering
```

---

## 16. API and frontend changes

### 16.1 Backend API

Add:

```text
GET  /api/semantic/status
POST /api/semantic/rebuild
POST /api/semantic/retry-errors
POST /api/semantic/test-embedding-provider
```

Optional development-only endpoint if useful during implementation:

```text
POST /api/semantic/search
```

Do not make the development search endpoint a required production feature.

### 16.2 Status response

Return at least:

```json
{
  "state": "ready|indexing|degraded|needs_configuration|error",
  "provider": "ollama",
  "model": "nomic-embed-text",
  "dimensions": 768,
  "tracks": {"total": 20000, "ready": 20000, "pending": 0, "errors": 0},
  "albums": {"total": 1700, "ready": 1700, "pending": 0, "errors": 0},
  "artists": {"total": 1200, "ready": 1200, "pending": 0, "errors": 0},
  "progress": 1.0,
  "lastError": ""
}
```

Never expose API keys.

### 16.3 Existing AI DJ response

Preserve existing fields so the UI does not require a destructive migration.

Add optional diagnostics:

```json
{
  "retrieval": {
    "mode": "semantic",
    "candidateCount": 537,
    "indexModel": "...",
    "fallbackUsed": false
  }
}
```

Do not return full embeddings or internal semantic documents to the normal UI.

### 16.4 Frontend settings

Add a **Semantic Music Index** section under AI settings:

- status;
- provider;
- embedding model;
- progress counts;
- test provider button;
- rebuild index button;
- retry failed items button;
- concise explanation that the index is local and used for AI music matching.

When provider is Ollama:

- show Ollama base URL and embedding model;
- explain model must already be available locally.

When provider is OpenAI:

- reuse current key when possible;
- otherwise allow separate encrypted semantic embedding key.

### 16.5 Smart Playlists / AI DJ page

Preserve current controls in `slices/aiDjSlice.ts` and `pages/SmartPlaylists.tsx`.

Add only minimal status UX:

- `Semantic library ready` when ready;
- `Building semantic library: n%` while indexing;
- unobtrusive `Using legacy matching while semantic index builds` if fallback occurs;
- actionable error if embeddings require configuration.

Do not make users wait on the AI DJ screen for a full initial index build.

---

## 17. Fallback, reliability, observability, and security

### 17.1 Fallback order

1. Semantic intent + semantic retrieval when index is ready enough.
2. Partial semantic retrieval if there is sufficient ready coverage (recommended threshold: at least 70% of active tracks plus usable artist/album graphs).
3. Existing legacy metadata path when semantic retrieval is unavailable.
4. Deterministic basic playlist fallback if the LLM itself is unavailable.

Record which path was used in logs/diagnostics.

### 17.2 Logging

Add structured/consistent logs for:

- semantic service startup;
- graph import/rebuild/save;
- document generation counts;
- embedding batch size/provider/model/duration;
- embedding retries/errors;
- query embedding duration;
- HNSW retrieval duration by entity type;
- candidate expansion/dedup/filter counts;
- final semantic retrieval count;
- fallback reason.

Do **not** log API keys or full user library contents.

Avoid logging full prompts at normal info level if current privacy conventions prefer reduced logging. If prompts are logged for debug, clearly keep them debug-only.

### 17.3 Privacy/security

- Semantic index is stored locally.
- SQLite embeddings are local application data.
- Cloud embedding providers receive semantic document text; Settings UI must make this clear.
- Ollama provides the fully local option.
- Continue using machine-bound encrypted settings for secrets.
- Do not send file system paths, Plex tokens, internal song IDs, or listening history to embedding APIs because none are needed for semantic quality.

---

## 18. Implementation phases and PR sequence

Phase 1 should be implemented as four ordered PRs. Each PR must leave `main` buildable and usable after squash merge.

### PR 1 — Semantic library foundation

**Branch:** `feature/semantic-library-foundation`

#### Tasks

- [ ] Add pinned `coder/hnsw` dependency.
- [ ] Add SQLite semantic tables/indexes/migration.
- [ ] Add `backend/internal/db/semantic.go` bulk operations.
- [ ] Add `backend/internal/semantic/types.go`.
- [ ] Implement deterministic track/album/artist document builders.
- [ ] Implement content hashing and document versioning.
- [ ] Implement float32 BLOB codec with validation.
- [ ] Implement `VectorIndex` interface.
- [ ] Implement HNSW wrapper for int64 keys.
- [ ] Implement graph load/save/rebuild + revision validation.
- [ ] Add graph corruption recovery.
- [ ] Add unit tests for all foundation components.
- [ ] Update this plan with implementation notes.

#### Acceptance criteria

- Existing DB migrates without data loss.
- Semantic documents can be generated from existing catalog records without an LLM.
- Vector round-trip is bit-stable within float32 representation.
- HNSW insert/search/delete works with deterministic test vectors.
- Graph can be persisted, reloaded, and searched.
- Revision mismatch rebuilds from SQLite.
- Corrupt/missing graph cache does not prevent ViiB startup.
- No production AI DJ behavior changes yet.

---

### PR 2 — Embedding and background indexing pipeline

**Branch:** `feature/semantic-library-indexer`

#### Tasks

- [ ] Add semantic embedding settings and encrypted secret handling.
- [ ] Implement `EmbeddingProvider` abstraction.
- [ ] Implement Ollama embedding provider.
- [ ] Implement OpenAI embedding provider.
- [ ] Implement provider test/validation.
- [ ] Implement batching, retries, cancellation, and bounded concurrency.
- [ ] Implement semantic service lifecycle.
- [ ] Generate artist/album/track documents in background.
- [ ] Store embeddings transactionally in SQLite.
- [ ] Incrementally upsert/delete HNSW entries.
- [ ] Add debounced graph persistence.
- [ ] Hook local scanner updates into semantic invalidation.
- [ ] Hook Plex catalog synchronization into semantic invalidation.
- [ ] Hook Last.fm and LLM metadata enrichment into semantic invalidation.
- [ ] Add semantic status/rebuild/retry API endpoints.
- [ ] Add Settings UI for provider/model/status/rebuild.
- [ ] Add unit + integration tests with fake embedding provider.
- [ ] Update this plan with implementation notes.

#### Acceptance criteria

- First launch with existing library begins indexing without blocking startup.
- Progress survives restart because SQLite remains authoritative.
- Re-running indexer with unchanged content creates zero unnecessary embedding work.
- A metadata enrichment change re-embeds only affected documents and relevant album/artist aggregates.
- Changing embedding provider/model triggers safe full reindex without mixing vector spaces.
- Ollama indexing works with no cloud dependency.
- OpenAI indexing works without changing the current chat provider implementation.
- User can continue using existing playback/library features during indexing.

---

### PR 3 — Semantic AI DJ retrieval and ranking

**Branch:** `feature/ai-dj-semantic-retrieval`

#### Tasks

- [ ] Add `PlaylistIntent` and semantic query fields.
- [ ] Update LLM system prompts to compile semantic intent instead of constraining to local genre taxonomy.
- [ ] Extend DJ phase plan with semantic query/negative query.
- [ ] Update plan cache key for time context and remove genre-hash dependency.
- [ ] Implement query embedding cache.
- [ ] Implement track/album/artist semantic search.
- [ ] Implement album/artist expansion into valid local catalog tracks.
- [ ] Implement hard filters and source filtering.
- [ ] Implement semantic evidence aggregation.
- [ ] Implement negative semantic penalty.
- [ ] Implement standard `HybridRanker`.
- [ ] Implement MMR/diversity pass.
- [ ] Add semantic scores to DJ `ScoreContext`.
- [ ] Reweight `ScoreSongForPhase` around semantic relevance.
- [ ] Update sequencer to use semantic phase candidate pools.
- [ ] Preserve legacy fallback.
- [ ] Preserve current API response fields; add optional retrieval diagnostics.
- [ ] Add comprehensive unit/integration tests.
- [ ] Update this plan with implementation notes.

#### Acceptance criteria

- AI DJ never sends the full library to the LLM.
- A generated playlist contains only existing ViiB song IDs.
- Explicit artist exclusions are always honored.
- Local/Plex source selection is honored.
- Poorly tagged tracks can enter candidate sets through track semantic context or artist/album retrieval.
- Missing energy/mood/tempo does not prevent a strongly semantic match from ranking well.
- Discover mode materially increases diversity/deep-cut weighting.
- Favorites mode materially increases user-preference weighting.
- Existing BPM/flow sequencing remains functional.
- Legacy mode works when semantic service is unavailable.

---

### PR 4 — UI integration, hardening, quality gates, and documentation

**Branch:** `feature/ai-dj-semantic-rag-hardening`

#### Tasks

- [ ] Add semantic readiness/indexing/fallback status to Smart Playlists UI.
- [ ] Add retrieval diagnostics only where useful for troubleshooting; do not clutter normal UX.
- [ ] Add 20k+ synthetic-library performance test/benchmark fixture.
- [ ] Add semantic service lifecycle/shutdown tests.
- [ ] Add graph corruption/recovery tests.
- [ ] Add provider failure/retry tests.
- [ ] Add migration-from-existing-user-library test.
- [ ] Add manual QA prompt matrix.
- [ ] Update `docs/smart-playlists.md`.
- [ ] Update relevant AI/settings documentation.
- [ ] Update `docs/architecture.md`.
- [ ] Update README feature summary if AI DJ is called out there.
- [ ] Run final backend/frontend production builds.
- [ ] Resolve all static-analysis/lint/type errors introduced by Phase 1.
- [ ] Mark completed items in this plan.

#### Acceptance criteria

- Phase 1 Definition of Done in **Section 21** is satisfied.
- No known semantic-index data-loss path exists.
- Feature is usable with 20,000+ catalog tracks.
- Failure of embedding provider or HNSW cache does not make ViiB unusable.

---

## 19. Testing plan

### 19.1 Unit tests

#### Semantic document builder

Cover:

- fully populated local track;
- minimally tagged track;
- Plex track represented through unified song catalog;
- malformed Last.fm tag JSON;
- artist context aggregation;
- album context aggregation;
- stable normalization/hash;
- hash changes when semantic metadata changes;
- hash does not change for play-count/liked changes;
- text-length cap.

#### Vector codec

- encode/decode float32 vectors;
- empty vector rejection;
- malformed BLOB rejection;
- NaN/Inf rejection;
- dimension validation.

#### HNSW wrapper

- add/search;
- update same ID;
- delete;
- persistence round-trip;
- rebuild;
- revision mismatch;
- dimension mismatch;
- corrupt file recovery;
- concurrent read safety under wrapper locks.

#### Retriever

Use a fake deterministic embedding provider so tests do not call network services.

Cover:

- track match;
- artist-only rescue;
- album-only rescue;
- deduplication;
- source hard filter;
- excluded artist;
- hard vs soft year constraint;
- instrumental-only;
- negative semantic penalty;
- empty index fallback.

#### Ranker/MMR

Cover:

- semantic relevance wins over missing metadata;
- favorite boost;
- skip penalty;
- recent-play penalty;
- discover-mode diversity;
- one-per-artist;
- MMR reduces near-duplicate selection;
- deterministic behavior with seeded test inputs.

#### DJ integration

- semantic scores flow into phase scoring;
- phase target counts retained;
- BPM sequencing retained;
- semantic candidate pool is not replaced by arbitrary whole-library fallback unless fallback conditions are met.

### 19.2 Integration tests

Create a synthetic catalog fixture containing at least:

- strongly tagged tracks;
- minimally tagged tracks with well-described artist context;
- conflicting genre tags;
- multiple decades;
- repeated artists/albums;
- liked/skipped/recently played patterns;
- both local and Plex source identities.

Use fake vectors with known geometry so expected nearest-neighbor ordering is deterministic.

### 19.3 Performance regression fixture

This is **not** an alternative-vector-engine benchmark. The vector implementation is already selected.

Generate ~20,000 track docs plus representative album/artist docs and verify on normal developer hardware:

- graph search itself is comfortably sub-second;
- target: p95 HNSW retrieval across all three graphs **< 100 ms** after query embedding is available;
- target: local candidate expansion/filter/ranking **< 150 ms**;
- target: graph load/rebuild from stored embeddings does not block UI startup;
- target: semantic in-memory footprint for a typical 20k-track / 512-dimension OpenAI index remains within a reasonable desktop-app budget (initial guardrail: < 250 MiB for all three active graphs + working retrieval data, excluding SQLite page cache and frontend runtime).

If targets fail, optimize the selected design; do not reopen the vector-database bakeoff unless a correctness blocker is discovered.

---

## 20. Manual QA prompt matrix

Use a real large library where possible. Record results in the final PR description.

1. `90 minutes of darker 90s alternative, start mellow like Radiohead, gradually get heavier, some obscure stuff, no Nirvana.`
   - verifies semantic style, energy arc, decade softness, exclusion, discovery.

2. `Quiet Sunday morning music: warm acoustic, jazz, folk and mellow singer-songwriter. Nothing aggressive.`
   - verifies cross-genre semantic concept retrieval.

3. `80s songs I probably know, but avoid anything I've played in the last week.`
   - verifies familiarity + recency behavior.

4. `Female-fronted punk and pop-punk with high energy for a workout.`
   - verifies natural-language attribute retrieval despite imperfect local genre tags.

5. `Instrumental focus music with a steady beat, no vocals, not sleepy ambient.`
   - verifies hard instrumental + negative semantics.

6. `Stuff that feels like The Cure but more electronic, favor deep cuts.`
   - verifies artist/style concept + discover weighting without requiring an exact genre string.

7. `Play music from my Plex source that fits a late-night atmospheric drive.`
   - verifies source filtering on unified catalog.

8. Select a deliberately poorly tagged track whose artist has good Last.fm context and use a matching semantic prompt.
   - verifies hierarchical rescue.

Evaluation criteria:

- coherent theme;
- explicit exclusions obeyed;
- no non-existent songs;
- useful inclusion of poorly tagged tracks;
- reasonable artist diversity;
- expected discovery/favorites behavior;
- sensible DJ flow when DJ mode enabled.

---

## 21. Phase 1 Definition of Done

Phase 1 is complete only when all of the following are true:

- [ ] ViiB contains a pure-Go semantic index subsystem backed by SQLite + pinned `coder/hnsw`.
- [ ] Track, album, and artist semantic documents are generated deterministically.
- [ ] Embeddings persist in SQLite and graph files are disposable/rebuildable.
- [ ] Ollama embeddings work.
- [ ] OpenAI embeddings work.
- [ ] Initial indexing is batched, cancellable, resumable, and non-blocking.
- [ ] Incremental catalog/enrichment changes update only affected semantic documents.
- [ ] Model/provider/dimension changes perform a safe full reindex.
- [ ] AI DJ LLM output contains semantic retrieval intent.
- [ ] AI DJ does not rely on exact local genre taxonomy for candidate recall.
- [ ] Both normal playlist mode and DJ mode use semantic retrieval when available.
- [ ] Track/album/artist retrieval is combined and deduplicated.
- [ ] Hard artist/source/instrumental constraints are deterministic.
- [ ] User behavior remains a ranking signal, not embedding content.
- [ ] Diversity/MMR prevents obvious semantic clustering.
- [ ] Existing BPM/flow/persona logic remains integrated.
- [ ] Legacy fallback remains available and tested.
- [ ] Semantic status/rebuild controls exist in Settings.
- [ ] 20k synthetic-library performance guardrails pass.
- [ ] Backend tests pass.
- [ ] Frontend typecheck/tests/build pass.
- [ ] Relevant documentation is updated.
- [ ] This roadmap reflects final implementation status.

---

## 22. Phase 1 Deferred Items

Keep these visible for a future Phase 2 rather than quietly adding them to Phase 1:

- Audio-native embeddings (CLAP/MuLan-like).
- Audio analysis runtime packaging and hardware acceleration.
- Lyrics semantic indexing.
- Playlist-vector centroids / playlist continuation as a first-class UI feature.
- Semantic global library search outside AI DJ.
- `More Like This` context action.
- Optional LLM final-curator pass over a bounded retrieved candidate set.
- Learned user preference vectors.
- Automatic exploration/exploitation tuning from long-term feedback.
- Multiple embedding models active simultaneously.
- Index quantization/compression if future library sizes make it necessary.

The Phase 1 schema/interface design must not prevent adding an `audio-semantic-v1` embedding family later, but Phase 1 should not implement it.

---

## 23. Final implementation principle

The end-state of Phase 1 should be:

> **The LLM understands the user's intent. The semantic index finds music that matches that meaning. ViiB's local ranking and DJ engine decide which valid tracks to play and in what order.**

The LLM should no longer attempt to reason over the whole library, and the quality of the AI DJ should no longer collapse simply because a user's files have incomplete genre/mood metadata.
