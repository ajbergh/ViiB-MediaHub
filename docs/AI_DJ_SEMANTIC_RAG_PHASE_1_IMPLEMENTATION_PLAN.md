# AI DJ Semantic Retrieval / RAG — Phase 1 Implementation Plan

**Repository:** `ajbergh/ViiB-MediaHub`
**Status:** PR 2 in progress — semantic embedding and indexing pipeline
**Objective:** Replace metadata-dependent AI DJ candidate selection with a semantic music retrieval pipeline that works over 20,000+ tracks even when genre, mood, album, year, and BPM tags are incomplete or absent.
**Phase 1 scope:** Text-semantic retrieval. Audio-content embeddings are deferred to Phase 2.

---

## 1. Problem and approach

### 1.1 Problem

The AI DJ contains sound planning, scoring, persona, recency, and sequencing logic, but
candidate discovery depends on structured metadata that is frequently missing:

- `backend/internal/api/smart_playlist.go` — `handleDJMode` and `handleGenerateSmartPlaylist` load the whole catalog via `db.GetAllSongs()` and select candidates by genre string matching, mood keyword matching, and artist matching.
- `backend/internal/dj/scoring.go` — `scoreEnergyMatch`, `scoreTempoMatch`, and `scoreMoodMatch` return a neutral `0.5` for unknown values, so weakly tagged tracks are indistinguishable from each other and from good matches.
- `backend/internal/dj/sequencer.go` — partitions candidates by energy/tempo bucket, then relaxes constraints when a bucket is sparse.
- `backend/internal/llm/provider.go` — `ParsePlaylistFilterWithContext` constrains the model to genre strings already present in the library, so intent that has no matching local tag cannot be expressed.

A user with 20,000 poorly tagged files gets weak results not because the music is absent
but because the retrieval layer cannot see it.

### 1.2 Approach

Compile the user's prompt into semantic intent, retrieve by meaning against locally
computed embeddings of track/album/artist descriptions, then rank and sequence with the
existing behavioural and BPM logic.

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
      Track vector set   Album vector set   Artist vector set
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

Indexing path:

```text
ViiB songs / Last.fm / AI enrichment
              │
              ▼
   deterministic semantic documents
              │ content hash
              ▼
      batched embedding provider
              │
              └────► SQLite embedding BLOB (authoritative)
                       │
                       └────► in-memory vector arena (derived, disposable)
```

### 1.3 In scope

- Semantic documents for tracks, albums, and artists.
- Persistent embeddings in the existing ViiB SQLite database.
- In-process exact vector search, pure Go, no new third-party dependency.
- Incremental background index maintenance.
- Embedding provider abstraction with Ollama and OpenAI implementations.
- Semantic query generation from the AI DJ LLM.
- Semantic retrieval for both normal AI playlist mode and DJ set mode.
- Hard exclusions and source filters.
- Behavioural reranking (likes, skips, plays, recency, discovery/favorites).
- Diversity control via MMR-style selection.
- Existing BPM/flow sequencing, updated to consume semantic relevance.
- Settings UI for index status and control.
- Automatic fallback to the legacy path while the index is unavailable.
- Tests, logging, metrics, documentation.

### 1.4 Out of scope for Phase 1

- Approximate nearest-neighbour indexing (see §21.1).
- On-disk index serialization.
- CLAP/MuLan/audio-waveform embeddings.
- Python, ONNX, CUDA, CoreML, DirectML, or any audio ML runtime.
- Lyrics ingestion or lyric embeddings.
- Internet-wide music search.
- A hosted vector database.
- Metadata scraping outside integrations already present in ViiB.
- An LLM choosing track names from its training knowledge.
- A final LLM editorial pass over retrieved tracks.
- Cross-user collaborative recommendation.
- Training a recommender model from listening history.

---

## 2. Architecture decisions

### 2.1 Storage and search

- **SQLite is authoritative** for semantic documents, embeddings, status, content hashes, and index state. It is the only durable store.
- **Search is an exact brute-force scan** over L2-normalized `float32` vectors held in three contiguous in-memory arenas (track, album, artist), behind the `VectorIndex` interface in §8.1.
- **Nothing is written to disk** beyond SQLite. There is no index file, no serialization format, and therefore no cache-invalidation, atomic-swap, or corruption-recovery path. Recovery is a table read.

### 2.2 Rationale

Measured on an AMD Ryzen 5 3600 (6C/12T), scalar Go, no SIMD, exact cosine over
L2-normalized `float32`:

| Corpus | Dims | Serial | All cores |
|---|---|---|---|
| 20,000 | 512 | 11.1 ms | 2.5 ms |
| 20,000 | 768 | 17.4 ms | — |
| 100,000 | 512 | 54.7 ms | 14.5 ms |

Retrieval is not the bottleneck at this scale. Query embedding — one HTTP round trip to
Ollama or OpenAI — costs tens to hundreds of milliseconds and dominates the request by an
order of magnitude. An exact scan is therefore fast enough, returns true top-k so
retrieval quotas mean what they say, and is exactly reproducible, which makes evaluation
and testing trustworthy.

Accepted trade-off: the scan is `O(n·d)` per query and will not scale to millions of
vectors. The trigger for revisiting this is defined in §21.1.

### 2.3 Index configuration

```text
Metric       = cosine similarity, computed as a dot product
Storage      = L2-normalized []float32, contiguous arena per entity type
Key type     = int64 (semantic_documents.id)
Parallelism  = min(GOMAXPROCS, 8) shards above 25,000 vectors; single-threaded below
Top-k        = exact, via a bounded min-heap of size k
```

Keep these as named constants in the semantic package. Do not expose them as user
settings.

---

## 3. Execution contract

### 3.1 Required behaviour

- Work from the latest `main` before beginning each implementation PR.
- Create branches/PRs in the order defined in §17.
- Keep this file updated in every implementation PR:
  - flip `[ ]` to `[x]` only after code **and** tests for that item are complete;
  - add implementation notes where the final layout differs from the plan;
  - record intentionally deferred work under §21.
- Do not remove the legacy AI DJ path until the semantic path has a tested fallback.
- Do not send the full song library, the full genre catalog, or thousands of track rows to an LLM.
- Never let the LLM invent song IDs. Every playlist entry must be a local ViiB catalog ID returned by deterministic retrieval.
- Preserve unified local/Plex catalog behaviour. Semantic retrieval is source-agnostic until ViiB source filters are applied.
- Do not add an additional server process or sidecar, a Docker dependency, a Python runtime, a CGO dependency, or an external vector database. ViiB already runs one internal HTTP server on port 34115 behind the Wails asset server; do not add a second.
- Do not auto-download an embedding model without explicit user action.
- Do not block application startup on first-time indexing.
- Do not recompute an embedding when the content hash and model identity are unchanged.
- Batch embedding calls. A 20,000-track library must never produce one HTTP request per track.
- All background work must honour `context.Context` cancellation and application shutdown.

### 3.2 Platform requirement

Every new dependency must cross-compile for all five release targets before it is
committed:

```text
GOOS=windows GOARCH=amd64
GOOS=windows GOARCH=arm64
GOOS=linux   GOARCH=amd64
GOOS=darwin  GOARCH=amd64
GOOS=darwin  GOARCH=arm64
```

Add this check to CI in PR 1. Windows is the primary target; a dependency that fails to
build there is not viable regardless of its other properties.

### 3.3 Database access constraints

`backend/internal/db/runtime_policy.go` configures `journal_mode = WAL`,
`synchronous = NORMAL`, `busy_timeout = 5000`, `foreign_keys = ON`, and
`SetMaxOpenConns(4)`. SQLite permits a single writer.

Consequences that are requirements, not suggestions:

- Bound every indexing transaction: at most 200 rows or ~2 MB of BLOB, then commit and yield. A multi-second write transaction makes user-facing writes (scrobbles, likes) fail with `SQLITE_BUSY`, not merely run slowly.
- Never hold a DB connection open across an embedding HTTP call. Read a pending batch, close the rows, call the provider, then write. Only four connections exist.
- Call `db.CheckpointWAL()` after a bulk indexing run to bound WAL growth.

### 3.4 Quality gates

Before marking any PR complete:

```text
backend/   go build ./...        (and the five cross-compile targets from §3.2)
backend/   go test ./...
backend/   go test -race ./internal/semantic/...
root/      npm run check
```

`npm run check` runs `check:palette`, `check:raw-colors`, `typecheck`, `vitest`, and
`vite build`. New UI must use the project's design tokens: `check:palette` rejects raw
Tailwind palette classes such as `bg-slate-800`, and `check:raw-colors` rejects raw
colour literals in TSX.

---

## 4. Backend package layout

New package:

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
    scan_index.go
    scan_index_test.go
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

Database access:

```text
backend/internal/db/semantic.go
backend/internal/db/semantic_test.go
```

API handlers:

```text
backend/internal/api/semantic.go
```

Add only route registration to `api.go`; it is already 2,740 lines.

---

## 5. Database schema

### 5.1 Schema installation

This repository has no versioned migration framework. Schema evolution uses three
patterns:

- idempotent `CREATE TABLE IF NOT EXISTS` / `CREATE INDEX IF NOT EXISTS` in `db.migrate()`;
- guarded `ALTER TABLE ... ADD COLUMN` in `db.migrateColumns()`, which probes first because SQLite has no `ADD COLUMN IF NOT EXISTS`;
- `Ensure<Feature>Schema()` helpers behind a `sync.Once` — see `EnsurePlexSchema` and `EnsureLibrarySyncSchema`.

Use the third pattern. Add `EnsureSemanticSchema()` in `backend/internal/db/semantic.go`,
guarded by a `sync.Once`, called from semantic service startup. Do not grow
`db.migrate()`, and do not introduce a migration framework as a side effect of this
feature.

### 5.2 `semantic_documents`

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
- Artist `entity_key` = stable canonical key of the normalized artist identity.
- Album `entity_key` = stable canonical key of normalized album artist + album name.
- Never use file paths as semantic identity.
- `content_hash` must incorporate `document_version`.
- Changing a document template increments `SemanticDocumentVersion`, which regenerates affected documents.
- Store embeddings as little-endian `float32` BLOBs, never JSON arrays.
- Store vectors **already L2-normalized**. Normalizing on load wastes startup time and risks an un-normalized vector reaching the scan.
- `song_id` is nullable and is not a foreign key. `songs.id` is not permanent: `db.ResolveSongIdentity` reuses an existing ID for a confirmed file move but mints a new one when the old path still exists (a duplicate copy). Track documents can therefore be orphaned, which `DeleteSemanticDocumentsForMissingSongs` handles. If a foreign key is preferred instead, note that `PRAGMA foreign_keys = ON` is already active, so `ON DELETE CASCADE` has real deletion semantics — choose deliberately.

### 5.3 `semantic_index_state`

```sql
CREATE TABLE IF NOT EXISTS semantic_index_state (
    entity_type TEXT PRIMARY KEY CHECK(entity_type IN ('track','album','artist')),
    document_revision INTEGER NOT NULL DEFAULT 0,
    item_count INTEGER NOT NULL DEFAULT 0,
    dimensions INTEGER NOT NULL DEFAULT 0,
    embedding_provider TEXT NOT NULL DEFAULT '',
    embedding_model TEXT NOT NULL DEFAULT '',
    embedding_input_prefix TEXT NOT NULL DEFAULT '',
    catalog_cursor INTEGER NOT NULL DEFAULT 0,
    last_full_rebuild_at INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT ''
);
```

- `document_revision` increments on any insert/update/delete of a `ready` document for that entity type. Status reporting and the model-change reindex both read it.
- `catalog_cursor` is the highest `library_changes.revision` already folded into this entity type (§9.3).
- `embedding_input_prefix` is part of index identity: changing a task prefix changes the vector space as surely as changing the model (§7.1.1).

Startup behaviour:

- Load each arena from `semantic_documents WHERE status = 'ready'`, preallocating from `item_count` and `dimensions`.
- If rows disagree on `embedding_dimensions`, or disagree with `semantic_index_state`, mark that entity type `error` and refuse the load. That indicates a partially completed provider change and must never silently produce a mixed vector space.

### 5.4 Required DB methods

Implement explicit bulk operations. No N+1 queries.

```go
UpsertSemanticDocuments(ctx context.Context, docs []SemanticDocument) error
GetPendingSemanticDocuments(ctx context.Context, entityType string, limit int) ([]SemanticDocument, error)
StoreSemanticEmbeddings(ctx context.Context, updates []EmbeddingUpdate) error
MarkSemanticDocumentError(ctx context.Context, id int64, err string) error
DeleteSemanticDocumentsForMissingSongs(ctx context.Context) (int, error)
ListReadySemanticEmbeddings(ctx context.Context, entityType string) ([]StoredEmbedding, error)
GetSemanticDocumentsByIDs(ctx context.Context, ids []int64) ([]SemanticDocument, error)
GetSongsByIDs(ctx context.Context, ids []string) ([]Song, error)
GetSongsForSemanticAlbum(ctx context.Context, artist, album string, limit int) ([]Song, error)
GetSongsForSemanticArtist(ctx context.Context, artist string, limit int) ([]Song, error)
GetSemanticIndexState(ctx context.Context, entityType string) (SemanticIndexState, error)
IncrementSemanticDocumentRevision(ctx context.Context, entityType string) error
SetSemanticCatalogCursor(ctx context.Context, entityType string, revision int64) error
GetSemanticIndexStats(ctx context.Context) (SemanticStats, error)
```

Notes:

- Chunk any `IN (...)` query at 900 parameters. SQLite's default variable limit is 999; `FilterSongsForAIDJ` already uses this chunk size.
- `ListReadySemanticEmbeddings` returns the full corpus (20k × 512 × 4 B ≈ 41 MB for tracks). Stream rows into a preallocated arena; do not accumulate 20,000 individually allocated slices.
- Wrap batch embedding writes plus the revision update in one transaction, subject to the bounds in §3.3.

---

## 6. Semantic document generation

### 6.1 Determinism

Phase 1 documents are deterministic. Do not make an LLM call to write prose per track;
that would make first indexing slow, expensive, and irreproducible. Encode existing ViiB
and Last.fm metadata into natural-language-like text.

Document generation must also be **cheap**, because it runs on every candidate surfaced
by the change log (§9.3). Build artist and album context maps once per indexing pass and
reuse them. No per-track SQL query, no per-track JSON re-parse of artist context.

### 6.2 Track document

Template:

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

Available source fields on `db.Song`: `Title`, `Artist`, `Album`, `AlbumArtist`,
`Genre []string`, `Year`, `OriginalYear`, `Mood`, `Energy`, `Tempo`, `BPM`,
`Instrumental`, `LastFMTags` (JSON string).

Rules:

- Omit unknown fields rather than writing `unknown` repeatedly.
- Normalize whitespace and malformed tag values.
- Decode `LastFMTags` JSON defensively; ignore corrupt JSON without failing indexing.
- Cap community tags at a useful top-N (12).
- Cap similar artist names (8).
- Exclude play count, liked state, skip count, last played, and any other volatile user behaviour. Those are ranking-time signals (§12.1).
- Keep ViiB identity as DB metadata only, never in the prose.
- Cap document length at 4 KiB — roughly 1,000 tokens, comfortably inside both providers' 8,192-token input limits.
- **The task prefix is not part of the document.** Store and hash `content` without it; the provider applies its prefix at request time (§7.4.1).

### 6.3 Artist document

One document per normalized artist, built from:

- artist name;
- genres aggregated from local songs;
- Last.fm artist tags (`artist_metadata.lastfm_tags`);
- a cleaned Last.fm bio excerpt (`artist_metadata.lastfm_bio`);
- similar artists (`lastfm_similar_artists`);
- active years only when the data is reliable.

Strip HTML and cap bio context at 600–1,000 characters. Do not embed full biographies.

### 6.4 Album document

One document per normalized `(albumArtist, album)` pair, built from:

- album name and album artist;
- release/original year distribution;
- genres pooled from album tracks;
- Last.fm tags common across tracks;
- mood/energy distribution;
- instrumental/vocal makeup;
- track titles, only when useful and capped.

### 6.5 Hierarchical context

Poorly tagged tracks inherit context without modifying canonical song metadata:

- the track document may include compact artist context;
- the track document may include compact album context;
- album and artist embeddings remain retrieval channels in their own right (§11.1).

Never overwrite a song's stored genre because its artist usually belongs to that genre.

### 6.6 Content hashing

```text
SHA-256(document_version + "\x00" + normalized_document_text)
```

Skip re-embedding when the hash is unchanged **and** the model identity tuple (§7.1.1) is
unchanged.

This gate carries significant load. The `songs` update trigger fires on every column
change, including `play_count`, `last_played`, `skip_count`, and `liked`, so a single
listening session enqueues many change-log candidates. The content hash is what converts
those into no-ops.

---

## 7. Embedding subsystem

### 7.1 Interface

```go
type EmbeddingProvider interface {
    Name() string
    Model() string

    // Task instruction prefixes. Both are "" for models that do not use them.
    DocumentPrefix() string
    QueryPrefix() string

    EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error)
    EmbedQuery(ctx context.Context, text string) ([]float32, error)

    MaxBatchSize() int
    Close() error
}
```

Documents and queries are separate methods because several embedding models require
different input prefixes for indexed text and for queries (§7.4.1).

Reject a provider response when:

- vector count differs from input count;
- dimensions are inconsistent within a batch;
- dimensions conflict with the recorded index state;
- any element is `NaN` or `Inf`;
- a vector is empty or zero-length;
- **a vector has zero magnitude.** This is distinct from zero *length* and it is the case that occurs in practice — an empty or whitespace-only document, or a degrading provider. A zero vector cannot be L2-normalized, and every similarity against it is `NaN`, which compares false against any threshold and ranks unpredictably.

L2-normalize every accepted vector before storing it. Both Phase 1 providers already
return unit vectors, so this is normally a no-op; do it anyway rather than relying on a
provider guarantee.

#### 7.1.1 Model identity

Model identity is the tuple:

```text
(provider, model, dimensions, documentPrefix, queryPrefix)
```

Any change to any element invalidates every stored embedding (§7.6). Persist the tuple in
`semantic_index_state` and compare it on every load.

### 7.2 Settings

```text
semantic_embedding_provider = auto | ollama | openai | disabled
semantic_embedding_model
semantic_embedding_dimensions
semantic_embedding_base_url
semantic_embedding_api_key
```

`semantic_embedding_api_key` is **not** encrypted automatically. `db.SetSetting` encrypts
only keys for which `crypto.IsSensitiveKey` returns true, and that is an exact-match
lookup against the `sensitiveKeys` map in `backend/internal/crypto/crypto.go` — there is
no pattern matching on `_api_key`. Register the key the way `crypto/plex_sensitive.go`
does, via an `init()` that adds it to the map, and add a test mirroring
`TestPlexCredentialsAreSensitive` so a future rename cannot silently start storing the
key in plaintext.

`disabled` lets a user turn semantic retrieval off without clearing settings; AI DJ then
uses the legacy path (§16.1).

Do not expose vector index tuning values as settings.

### 7.3 `auto` resolution

`backend/internal/llm/provider.go` supports six chat providers — `ollama`, `gemini`,
`openai`, `anthropic`, `xai`, `openrouter` — and `handleDJMode` falls back to
`ProviderGemini` with the stored `gemini_api_key` when `llm_*` settings are unset.
Resolution must account for all of them.

Order:

1. If `semantic_embedding_provider` is set explicitly, use it.
2. If the current AI provider is Ollama, use Ollama embeddings at the same base URL with the default embedding model.
3. If the current AI provider is OpenAI, reuse the existing key.
4. If an OpenAI key exists from any source (`llm_api_key` where `llm_provider = openai`), use it.
5. If a local Ollama instance is reachable at the configured or default base URL **and** an embedding-capable model is already pulled, use it. Probe with `GET /api/tags`; never pull.
6. Otherwise `needs_configuration`, with a message naming what would fix it.

Provider capability, to be stated plainly in the Settings UI:

| Chat provider | Phase 1 embeddings | Note |
|---|---|---|
| Ollama | Yes, same instance | Fully local; model must already be pulled |
| OpenAI | Yes, same key | `text-embedding-3-small` |
| Gemini | Pending decision in PR 2 | Provider offers `gemini-embedding-001`; `google.golang.org/genai` is already an indirect dependency. Gemini is this app's chat fallback, so this is the largest gap in `auto` |
| Anthropic | Not possible | Anthropic publishes no embeddings API |
| xAI | No | Not evaluated |
| OpenRouter | No | Chat-completions oriented; do not assume an embeddings route exists without verifying |

PR 2 must either add the Gemini adapter or record the decision that Gemini users get no
semantic retrieval. Recommendation: add it.

### 7.4 Ollama provider

Use `POST /api/embed`. Do not use `POST /api/embeddings`.

| | `/api/embed` | `/api/embeddings` |
|---|---|---|
| Status | current | deprecated, superseded by `/api/embed` |
| Input field | `input` — string or **array of strings** | `prompt` — single string only |
| Response field | `embeddings` (array of vectors) | `embedding` (one vector) |
| Batching | yes | no |

Batching is impossible on the legacy endpoint, which would silently turn a 20,000-track
index into 20,000 sequential HTTP requests.

Requirements:

- `POST {baseURL}/api/embed` with `{"model": ..., "input": [...]}`; read `embeddings`.
- Set `truncate` explicitly to `false`. It defaults to `true`, silently truncating oversized input rather than rejecting it. The 4 KiB document cap (§6.2) should make this unreachable; the explicit flag makes a violation visible.
- Keep the embedding model independent from the chat model.
- Default model `nomic-embed-text` when the user selects Ollama and leaves the field empty.
- Never auto-pull. Probe with `GET /api/tags` and report an error naming the required `ollama pull` command.
- Detect vector dimension from the first successful response and persist it.
- One active embedding request at a time by default. Ollama serialises model execution; parallel requests add queueing, not throughput.
- Batch size 32, as an internal constant.
- Default base URL `http://localhost:11434`, matching `llm.DefaultOllamaEndpoint`.

#### 7.4.1 Task prefixes

`nomic-embed-text` (v1.5) requires task instruction prefixes; upstream states the prompt
*must* include one. Omitting them degrades retrieval, and degrades it **asymmetrically**,
which is the failure mode hardest to notice because results still look plausible.

```text
Indexed documents: "search_document: " + document text
Queries:           "search_query: "    + query text
```

Requirements:

- Prefixes belong to the provider, not the document (§6.2), and are part of model identity (§7.1.1).
- Native dimensionality is 768. Matryoshka reduction to 512/256/128/64 is supported, but reduction requires truncate-then-**re-normalize**, so perform it only inside the provider adapter where normalization is guaranteed.
- L2-normalize output. Do not assume Ollama has done it.
- Maintain a small `model -> (documentPrefix, queryPrefix)` table. Default to empty prefixes for unknown models and surface the model name in the status payload so a mismatch is diagnosable.

### 7.5 OpenAI provider

- Default model `text-embedding-3-small`: 1,536 native dimensions, 8,192 max input tokens, returned vectors already unit length.
- Request **512 dimensions** via the `dimensions` request parameter unless the user explicitly chose a supported alternative. Use the API parameter, not manual truncation — the API re-normalizes, manual truncation does not.
- No task prefixes; `DocumentPrefix()` and `QueryPrefix()` return `""`.
- Batch size 128, and additionally cap each request by total input tokens so a future larger document cap cannot silently produce oversized requests.
- Use existing HTTP/retry conventions and encrypted secret storage.
- Implement as a small adapter behind `EmbeddingProvider`. Do not upgrade or restructure the OmniLLM chat stack for Phase 1 embeddings.

**Cost.** A 20,000-track library produces ~20,000 track, ~1,700 album, and ~1,200 artist
documents. At a realistic ~250 tokens per document that is ≈ 6M tokens; worst case at the
full 4 KiB cap is ≈ 23M tokens. At `text-embedding-3-small` list pricing of $0.02 per 1M
tokens, that is roughly **$0.12 typical and under $0.50 worst case, one time**, with
near-zero incremental cost afterwards because of the content-hash gate. Compute this
estimate from the actual catalog size and show it in Settings before a cloud index build
starts, requiring explicit confirmation. Re-verify pricing at implementation time and
record the figure used.

### 7.6 Model identity changes

Any change to the tuple in §7.1.1 invalidates every stored embedding.

1. Mark all semantic documents `pending` in one transaction and clear `ready`.
2. Keep serving the **existing in-memory arena** while re-embedding runs. The old vectors remain internally consistent, so AI DJ keeps working.
3. Build new embeddings in the background into `semantic_documents`.
4. Swap an arena only when its entity type is fully re-embedded and dimension-consistent.
5. Never mix vectors from two identity tuples in one arena. The arena carries its tuple; a query embedded under a different tuple must be rejected, not compared.

---

## 8. Vector index subsystem

### 8.1 Interface

Keep the concrete implementation private to the semantic package so it can be replaced
(§21.1) without touching the AI DJ contract.

```go
type VectorIndex interface {
    Upsert(id int64, vector []float32) error
    Delete(id int64) error
    Search(vector []float32, k int) ([]VectorMatch, error)
    Len() int
    Dimensions() int
    Rebuild(ctx context.Context, items []StoredEmbedding) error
    Close() error
}

type VectorMatch struct {
    ID         int64
    Similarity float64 // cosine similarity, clamped to [0,1]
}
```

`Search` and `Upsert` must return an error on dimension mismatch, never panic.

### 8.2 Storage layout

```go
type scanIndex struct {
    mu      sync.RWMutex
    dims    int
    keys    []int64       // len == n
    vectors []float32     // len == n * dims, L2-normalized, contiguous
    offset  map[int64]int // key -> row index
    free    []int         // rows released by Delete, reusable
}
```

- One contiguous arena per entity type. Row `i` occupies `vectors[i*dims : (i+1)*dims]`.
- `Upsert` of an existing key overwrites in place: no reallocation, no tombstone.
- `Delete` marks the row free and removes it from `offset`; `Search` skips free rows. Compaction happens only in `Rebuild`, so steady-state churn never copies the arena.
- Keys are `semantic_documents.id` integers. No file paths, prompts, tokens, or API keys ever enter the index.

### 8.3 Search

```text
q = L2-normalized query vector
for each live row i:  score_i = dot(vectors[i], q)   // == cosine similarity
keep top-k by score using a bounded min-heap of size k
```

- Guard `len(q) != dims` and return an error.
- Above ~25,000 vectors, shard the row range across `min(GOMAXPROCS, 8)` goroutines, each with a local top-k heap, then merge. Below that, the goroutine overhead exceeds the work.
- Similarity lands in `[-1, 1]`; clamp to `[0, 1]` before exposing it. Negative and zero similarity are both "unrelated" for ranking.
- Sharded and single-threaded results must be identical (§18.1).

### 8.4 Concurrency

- Guard the arena with `sync.RWMutex`. `Search` takes the read lock; `Upsert`, `Delete`, and `Rebuild` take the write lock.
- Never hold the lock across an HTTP embedding call or SQLite I/O. Read the batch, release, embed, re-acquire to write.
- `Rebuild` constructs a replacement `scanIndex` off to the side and swaps the pointer under the write lock. Never mutate the live arena during a rebuild.

### 8.5 Startup, recovery, and footprint

- On startup, `Rebuild` each entity type from stored `ready` embeddings, preallocating from `semantic_index_state`.
- Validate dimension consistency across rows; a mismatch is a partially completed provider change and must fail loudly (§5.3), not be skipped.
- There is no cache to invalidate and no file to repair. A crash mid-indexing leaves `semantic_documents` consistent because writes are transactional; the next launch resumes the pending queue.
- Footprint at 20k tracks × 512 dims: 20,000 × 512 × 4 B ≈ **41 MB** for tracks, under 6 MB for albums and artists combined. At 768 dims: ≈ 61 MB + 9 MB.

---

## 9. Indexing lifecycle

### 9.1 Service startup

`semantic.Service` is constructed with the database handle and the application data path
(`os.UserConfigDir()/ViiB-MediaHub`), and started during backend initialization.

1. Load semantic settings.
2. Initialize the embedding provider if configured; otherwise publish `needs_configuration` and stop.
3. Load in-memory arenas from stored `ready` embeddings (§8.5).
4. Scan the catalog for documents whose content hash changed or does not exist, starting from `semantic_index_state.catalog_cursor` (§9.3).
5. Queue pending embeddings.
6. Process batches in the background.
7. Publish status and progress.

Application startup must not wait for steps 4–6. Step 3 is on the startup path but is one
indexed table read plus a copy; measure it, and if it exceeds ~250 ms move it to the
background with status reporting `indexing` until it completes.

### 9.2 Initial build order

1. artist documents;
2. album documents;
3. track documents.

All document text may be generated first, but this order makes broad semantic retrieval
useful earlier.

### 9.3 Incremental updates

Drive invalidation from the existing change log, not from individual call sites.

`backend/internal/db/library_sync_schema.go` installs `songs_sync_insert`,
`songs_sync_update`, and `songs_sync_delete` triggers that increment
`library_state.revision` and append to
`library_changes(revision, song_id, operation, changed_at)` on every insert, update, and
delete of `songs`. Because local scanning, Plex sync, Last.fm track enrichment, and LLM
metadata enrichment all write through `songs`, all four are covered automatically — and so
is any write path added later.

Approach:

- Persist a per-entity-type cursor in `semantic_index_state.catalog_cursor`.
- On startup and on a periodic tick, read `library_changes WHERE revision > cursor` in ascending order, regenerate documents for the affected songs plus their album and artist aggregates, compare content hashes, enqueue only genuine changes, then advance the cursor.
- `operation = 'delete'` removes the track document and re-aggregates the affected album and artist documents.

Three conditions must be handled explicitly:

1. **The log means "maybe changed", not "did change".** The update trigger fires on `play_count`, `last_played`, `skip_count`, and `liked`, so a listening session produces many candidates that hash identically. Never treat a `library_changes` row as sufficient reason to re-embed; the content hash decides (§6.6).
2. **The log is pruned.** It is trimmed by `DELETE FROM library_changes WHERE revision < ?`. If `catalog_cursor` falls below the oldest retained revision, tailing would silently skip changes. Detect this (`MIN(revision) > cursor`) and fall back to a full content-hash rescan of all songs.
3. **The log covers `songs` only.** `artist_metadata` (`lastfm_tags`, `lastfm_bio`) and `album_metadata` have no trigger, so Last.fm *artist* enrichment is invisible to the cursor. Either add explicit invalidation in the Last.fm artist enrichment path or rescan artist/album document hashes on a slower schedule. Record which was chosen in the implementation notes.

Never re-embed for: play count, last played, skip count, liked state, playlist
membership, or playback source availability. Those are ranking-time signals, enforced by
test (§18.1).

### 9.4 Retry behaviour

- Retry transient provider failures with exponential backoff plus jitter.
- Persist `retry_count` and `last_error`.
- After a bounded retry count, leave the document in `error` and continue with others.
- A single malformed track must never stall the queue.
- A manual retry action resets eligible `error` rows.

---

## 10. LLM intent compiler

### 10.1 Stop constraining to local taxonomy

`ParsePlaylistFilterWithContext` currently passes `getAvailableGenreNames()` to the model
so it can map the prompt onto genre strings the library already has. Semantic retrieval
removes that need. The model's role is to interpret musical meaning, not to map the user
into an imperfect local taxonomy.

### 10.2 `PlaylistIntent`

Add to `backend/internal/dj/types.go` or a dedicated intent file:

```go
type PlaylistIntent struct {
    IntentSummary         string   `json:"intentSummary"`
    SemanticQuery         string   `json:"semanticQuery"`
    NegativeSemanticQuery string   `json:"negativeSemanticQuery,omitempty"`
    IncludeArtists        []string `json:"includeArtists,omitempty"`
    ExcludeArtists        []string `json:"excludeArtists,omitempty"`
    PreferredGenres       []string `json:"preferredGenres,omitempty"`
    MinYear               int      `json:"minYear,omitempty"`
    MaxYear               int      `json:"maxYear,omitempty"`
    YearConstraintHard    bool     `json:"yearConstraintHard,omitempty"`
    InstrumentalOnly      bool     `json:"instrumentalOnly,omitempty"`
    DiscoveryBias         float64  `json:"discoveryBias,omitempty"`
    FamiliarityBias       float64  `json:"familiarityBias,omitempty"`
}
```

`SemanticQuery` is a concise retrieval description, not the raw prompt copied verbatim.

Example prompt:

```text
90 minutes of darker 90s alternative, start mellow like Radiohead,
gradually get heavier, some obscure stuff, no Nirvana
```

Example intent:

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

### 10.3 DJ phases

Extend `dj.DJPhase` with:

```go
SemanticQuery         string   `json:"semanticQuery"`
NegativeSemanticQuery string   `json:"negativeSemanticQuery,omitempty"`
StyleHints            []string `json:"styleHints,omitempty"`
```

The model produces a distinct semantic query per phase when the user asks for an arc:

```text
Warm-up: moody atmospheric introspective alternative rock, restrained energy
Build:   melodic emotionally intense 1990s alternative rock, increasing drive
Peak:    heavy driving noisy alternative rock, cathartic and energetic
Cooldown: spacious reflective alternative/art rock, lower intensity
```

### 10.4 Validation

- Parse strict JSON, as `dj_set_planner.go` already does.
- Normalize arrays and cap lengths: semantic query 512 characters, artist include/exclude lists 50 entries each.
- Clamp bias values to `[0,1]`.
- An invalid intent falls back to a deterministic query derived from the raw prompt. It must not fail the request.

### 10.5 Plan cache key

`dj.PlanCacheKey.String()` in `backend/internal/dj/types.go` must be fixed as part of this
work:

```go
return k.Provider + "|" + k.Model + "|" + k.NormalizedPrompt + "|" +
    k.Persona + "|" + string(rune(k.TargetDurationMin)) + "|" +
    string(rune(k.FlowStrictness)) + "|" + k.TopGenresHash
```

`string(rune(n))` converts an integer to one Unicode code point, not to decimal text. A
45-minute set yields `"-"` (U+002D), which is also the field separator; small values land
on control characters and surrogate-range values become U+FFFD, so distinct inputs
collide. `UseTimeContext` is absent from the struct, so time-aware and time-blind plans
share a cache entry.

Required:

- Encode numeric components with `strconv.Itoa`.
- Add `UseTimeContext bool`, and when enabled include a time-of-day bucket.
- Remove `TopGenresHash` as a required semantic-planning dependency.
- Add a unit test asserting that keys differing only in `TargetDurationMin` or `FlowStrictness` produce different strings.

---

## 11. Semantic retrieval and candidate expansion

### 11.1 Quotas

Per semantic query, as named internal constants:

```text
Track set:  top 300
Album set:  top 40
Artist set: top 30
Album expansion:  max 8 local tracks per album result
Artist expansion: max 10 local tracks per artist result
Maximum deduped candidate pool before hard filters: 900
```

These are exact top-k, so the quotas mean what they say.

### 11.2 Evidence aggregation

Each candidate retains its provenance:

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

Initial combination:

```text
BestSimilarity = max(
  trackSimilarity,
  albumSimilarity  * 0.94,
  artistSimilarity * 0.88,
)
```

Track matches are preferred; album and artist retrieval exist to rescue poorly tagged
tracks.

### 11.3 Hard filters

Applied after expansion and before ranking. A similarity score may never override a hard
exclusion.

- Explicit excluded artist.
- Explicit included artist, when the user phrased it as mandatory.
- Source (`all`, `local`, `plex`) — apply by calling `db.FilterSongsForAIDJ(candidates, source)` rather than reimplementing the rules. `Song.Source` is derived at read time from `plex_tracks` joined to `plex_sources.available`, so it is not a column and cannot be filtered inside the vector index. This method also drops Plex tracks whose source is currently unreachable, which is required behaviour and must not be bypassed.
- Instrumental-only when explicitly requested.
- Year, only when `YearConstraintHard` is true.
- Inaccessible or deleted catalog identities.

### 11.4 Soft metadata signals

Year, genre, mood, tempo, and energy act as soft boosts when they are not hard
constraints. Never require them to exist.

### 11.5 Negative semantic query

When present:

- embed the negative query once;
- compute its similarity against the positive candidate set only;
- subtract a bounded penalty;
- do not run a second full-corpus search.

```text
adjustedSemantic = clamp(positiveSimilarity - 0.25 * negativeSimilarity, 0, 1)
```

### 11.6 Query embedding cache

Cache query embeddings keyed by `(modelIdentityTuple, prefixedQueryText)` with a bounded
LRU. Phase plans reuse queries across relaxation passes, and a DJ set issues one query
per phase; caching removes redundant round trips within a single request.

---

## 12. Hybrid ranking and personalization

### 12.1 Separation of concerns

Volatile user behaviour never enters an embedding. It is applied at ranking time:

- liked state;
- play count;
- completion / skip rate;
- recently played state;
- artist affinity;
- genre affinity;
- discovery / favorites mode.

### 12.2 Standard playlist ranking

A reusable `HybridRanker` for non-DJ playlist mode:

```text
semantic relevance       70%
behavior/personal taste  20%
explicit metadata fit     10%
```

Then apply recency, artist repetition, exclusions, and discovery/favorites policy.

Keep the weights in named constants, not in API contracts. Enforce intent with ordering
tests rather than by asserting exact scores.

### 12.3 DJ scoring integration

Extend `dj.ScoreContext`:

```go
SemanticScores map[string]float64
```

Reweight `ScoreSongForPhase` so semantic relevance is the dominant musical-fit input:

```text
semantic relevance   50%
phase metadata fit   20%
BPM continuity       15%
persona/behavior     10%
genre affinity        5%
```

Then apply the existing penalties (recency, artist repetition, skip rate).

This is what stops missing mood/energy/tempo from dominating the outcome merely because
`scoreEnergyMatch` and `scoreTempoMatch` return a neutral `0.5` for unknown values.

### 12.4 Diversity / MMR

Add an MMR-style pass before sequencing:

```text
MMR(candidate) = lambda * relevance
               - (1-lambda) * maxSimilarity(candidate, alreadySelected)
```

Initial lambda by mode:

```text
favorites: 0.85
balanced:  0.75
discover:  0.60
```

Additional rules:

- Honour the existing one-per-artist setting.
- When one-per-artist is off, still apply a soft artist repetition penalty.
- Avoid long same-album runs unless the user asked for an album-focused result.
- Restrict pairwise MMR to the top-ranked working set (100–150), never the full 900-candidate pool.

---

## 13. DJ sequencer and handler integration

### 13.1 `backend/internal/api/smart_playlist.go`

This is the primary integration surface.

- `handleDJMode` currently calls `db.GetAllSongs()` and materialises the whole catalog on every request. Replace that recall path with semantic retrieval.
- The same full-library load also computes `genreCounts` for `LibraryContext.AvailableGenres` and `avgSongLengthSec` for phase sizing. Semantic retrieval replaces the *recall* use only. Replace the other two with cheap aggregates — `genre_stats` already exists for the former — rather than keeping a 20,000-row load alive to compute an average duration.
- `handleGenerateSmartPlaylist` routes through `HybridRanker` (§12.2).
- Keep `tryLocalGenreMatch`, `tryMoodBasedMatch`, `tryArtistBasedMatch`, `tryMatchIndexedGenre`, `tryMatchMultipleGenres`, `extractDecadeFromPrompt`, and `applyPlayHistoryFilters` intact as the legacy fallback path (§16.1).

### 13.2 `backend/internal/dj/sequencer.go`

Keep it. Do not rewrite it wholesale.

- Accept semantic candidate scores and evidence.
- Stop using energy/tempo bucket selection as the primary recall mechanism; the semantic retriever produces the phase candidate pool.
- Retain existing energy/tempo/BPM logic for phase fit and ordering.
- Retain stochastic selection, artist tracking, BPM sorting, persona logic, and micro-shuffle.
- Candidate relaxation broadens semantic candidate counts or softens soft constraints. It does not fall back to whole-library metadata buckets.
- Use all-library fallback only when semantic retrieval is unavailable, or returns an unusably small set after deterministic recovery steps.

Per-phase flow:

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

## 14. API surface

### 14.1 Endpoints

In `backend/internal/api/semantic.go`, registered on the existing chi router:

```text
GET  /api/semantic/status
POST /api/semantic/rebuild
POST /api/semantic/retry-errors
POST /api/semantic/test-embedding-provider
```

`POST /api/semantic/rebuild` takes an explicit scope, because the two operations differ
substantially in cost:

- `{"scope": "reindex"}` — re-embed everything. Used after a model identity change. Expensive, and may cost money.
- `{"scope": "reload"}` — re-read stored embeddings into memory. Cheap, no provider calls.

Optional development-only endpoint, not a production feature:

```text
POST /api/semantic/search
```

### 14.2 Status response

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

Never expose API keys. Do expose `model` and `dimensions` — they are the fastest way to
diagnose a mixed vector space or a wrong prefix in the field.

### 14.3 AI DJ response

Preserve all existing fields so the UI needs no destructive migration. Add optional
diagnostics:

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

Never return embeddings or raw semantic documents to the normal UI.

---

## 15. Frontend

### 15.1 Settings — Semantic Music Index

Add a section under AI settings showing:

- state and progress counts;
- provider and embedding model;
- test-provider button;
- rebuild-index button (`reindex` and `reload` distinguished);
- retry-failed-items button;
- a concise explanation that the index is local and used for AI music matching.

When the provider is Ollama:

- show the base URL and embedding model;
- state that the model must already be available locally, and name the `ollama pull` command.

When the provider is OpenAI:

- reuse the current key when possible, otherwise accept a separate encrypted key;
- show the one-time cost estimate from §7.5, computed from the actual catalog size, and require explicit confirmation before the first cloud index build.

When the current chat provider cannot supply embeddings (§7.3), say so directly and name
the two remedies: point at a local Ollama instance, or add an OpenAI key. Do not leave the
user at a bare `needs_configuration`.

### 15.2 Smart Playlists / AI DJ page

Preserve current controls in `slices/aiDjSlice.ts` and `pages/SmartPlaylists.tsx`
(`aiDjDiscoverMode`, `aiDjOnePerArtist`, `aiDjSource`, `aiDjPersona`, `aiDjTalkMode`,
`avoidRecentlyHours`).

Add minimal status UX only:

- `Semantic library ready` when ready;
- `Building semantic library: n%` while indexing;
- an unobtrusive `Using legacy matching while semantic index builds` when fallback occurs;
- an actionable error when embeddings need configuration.

Never make the user wait on the AI DJ screen for an initial index build.

### 15.3 Design system

All new UI must use project design tokens. `npm run check:palette` rejects raw Tailwind
palette classes and `npm run check:raw-colors` rejects raw colour literals in TSX, so a
Settings panel written with stock Tailwind colours fails the build gate.

---

## 16. Fallback, observability, and security

### 16.1 Fallback order

1. Semantic intent plus semantic retrieval when the index is sufficiently ready.
2. Partial semantic retrieval when coverage is sufficient — at least 70% of active tracks ready, plus usable artist and album vector sets.
3. The existing legacy metadata path when semantic retrieval is unavailable.
4. A deterministic basic playlist when the LLM itself is unavailable.

Record the path taken in logs and in the `retrieval` diagnostics block.

### 16.2 Logging

Structured logs for:

- semantic service startup;
- arena load/rebuild — entity type, item count, dimensions, duration;
- `library_changes` cursor advance — from/to revision, candidates examined, documents actually re-embedded. The ratio of the last two is the health signal for §6.6;
- document generation counts;
- embedding batch size, provider, model, duration, and **request count per indexing run** — the regression signal for accidental per-track requests;
- embedding retries and errors;
- query embedding duration;
- vector scan duration by entity type;
- candidate expansion, dedup, and filter counts;
- final semantic retrieval count;
- fallback reason;
- rejected-vector counts by reason (dimension, NaN/Inf, zero magnitude).

Never log API keys or full library contents. Keep prompt logging at debug level only.

### 16.3 Privacy and security

- The semantic index is local; embeddings are local application data in SQLite.
- Cloud embedding providers receive semantic document text. The Settings UI must state this plainly.
- Ollama is the fully local option.
- Continue using machine-bound encrypted settings for secrets (§7.2).
- Never send file system paths, Plex tokens, internal song IDs, or listening history to an embedding API. None are needed for semantic quality.

---

## 17. Implementation phases and PR sequence

Four ordered PRs. Each must leave `main` buildable and usable after squash merge.

### Implementation progress

- **2026-08-26:** Implemented PR 1 on `feature/semantic-library-foundation`. The
  scope is the schema, deterministic document builders, vector codec, exact in-memory
  index, tests, and cross-compilation CI. PR [#23](https://github.com/ajbergh/ViiB-MediaHub/pull/23)
  merged to `main` as `e6b1f07` after all frontend, backend, five-target semantic,
  and Windows packaged-build checks passed. No AI DJ production path changes are included.
- **2026-08-26:** PR #23's initial backend CI run passed the semantic tests and all five
  cross-builds, but staticcheck flagged a pre-existing capitalized error string in Plex
  pagination. A behavior-neutral lowercase correction is included so the required backend
  validation can proceed.
- **2026-08-26:** Started PR 2 on `feature/semantic-library-indexer` from the merged PR 1
  head. The active scope is embedding-provider configuration, background document/index
  maintenance, and index status/rebuild controls; semantic DJ retrieval remains PR 3.
- **2026-08-26:** Opened draft PR [#24](https://github.com/ajbergh/ViiB-MediaHub/pull/24)
  for the active PR 2 implementation so CI runs continuously while the remaining OpenAI
  adapter, Gemini decision, incremental arena maintenance, and cloud-cost confirmation
  work is completed.

#### PR 1 implementation notes (2026-08-26)

- `EnsureSemanticSchema()` is an additive, per-DB `sync.Once` installation path; it does
  not change `db.migrate()` or existing catalog data.
- The existing `GetSongsByIDs([]string)` API could not be changed to take a context
  without breaking current callers, so semantic retrieval uses the new bounded,
  context-aware `GetSongsByIDsContext(context.Context, []string)` companion.
- `BuildDocuments` builds artist, album, then track documents from reusable aggregate
  maps. Stored document text is whitespace-normalized, contains no ViiB song ID or task
  prefix, and its hash includes `SemanticDocumentVersion`.
- Vector BLOBs are little-endian, validated, and normalized before storage. The exact
  scan index has deterministic tie-breaking, free-list deletion, replacement-on-rebuild,
  and sharded scans above 25,000 rows.
- CI now cross-compiles the semantic foundation dependency closure (semantic package and
  SQLite persistence) for all five release GOOS/GOARCH targets with `CGO_ENABLED=0`.
  Native Wails application packaging remains covered by its platform-specific build jobs.
- Local verification passed `go test ./internal/db ./internal/semantic`, `go build ./...`,
  `go vet` for the new packages, and the five semantic cross-build targets. The complete
  `go test ./...` run reaches the semantic tests. Its previously failing Windows path
  assertion in `internal/validation.TestSanitizePath` is corrected in PR 2 to expect the
  platform-cleaned separator. `go test -race ./internal/semantic/...` could not run
  locally because this workstation has no C compiler; CI's Linux backend job
  remains the required race-test gate. Frontend `npm run check` did not complete here:
  its unchanged TypeScript check stalled and its orphaned process was stopped.

### PR 1 — Semantic library foundation

**Branch:** `feature/semantic-library-foundation`

No new third-party dependency is added in this PR.

#### Tasks

- [x] Add `EnsureSemanticSchema()` in `backend/internal/db/semantic.go` — tables, indexes, `sync.Once`, no new migration framework.
- [x] Add bulk DB operations from §5.4 with bounded transactions and 900-parameter chunking.
- [x] Add `backend/internal/semantic/types.go`.
- [x] Implement deterministic track/album/artist document builders.
- [x] Implement content hashing and document versioning.
- [x] Implement the float32 BLOB codec with full validation, including zero-magnitude rejection.
- [x] Implement the shared L2 normalization helper used by every write path.
- [x] Implement the `VectorIndex` interface.
- [x] Implement `scanIndex`: contiguous arena, in-place upsert, free-list delete, bounded-heap exact top-k, sharded scan above the parallelism threshold.
- [x] Implement `Rebuild` from stored `ready` embeddings with preallocation and dimension validation.
- [x] Add unit tests for every foundation component.
- [x] Add the five-target cross-compile check to CI.
- [x] Update this plan with implementation notes.

#### Acceptance criteria

- The existing database gains the semantic tables without data loss and without touching `db.migrate()`.
- Semantic documents generate from existing catalog records with no LLM involved.
- Vector round-trip is bit-stable within float32 representation.
- Exact top-k matches a naive reference implementation over random vectors — identical results, not merely overlapping.
- Insert, overwrite, delete, and rebuild all preserve exactness; deleted rows never appear in results.
- Sharded and single-threaded scans return identical results for the same input.
- Startup with no semantic rows, with a partially populated table, and with a dimension-inconsistent table all behave as specified in §8.5, and none prevents ViiB startup.
- `go build` succeeds for all five targets.
- No production AI DJ behaviour changes.

---

### PR 2 — Embedding and background indexing pipeline

**Branch:** `feature/semantic-library-indexer`

#### Tasks

- [x] Add semantic embedding settings; register `semantic_embedding_api_key` in `crypto.sensitiveKeys` via `init()`, with a test asserting it is treated as sensitive.
- [x] Implement the `EmbeddingProvider` abstraction with separate document/query methods and task prefixes.
- [x] Implement the Ollama provider against `POST /api/embed` with array `input`, explicit `truncate: false`, and `nomic-embed-text` task prefixes.
- [ ] Implement the OpenAI provider (`text-embedding-3-small`, `dimensions: 512`, token-capped batches).
- [ ] Decide and record: add the Gemini embeddings adapter, or document that Gemini users get no semantic retrieval (§7.3).
- [x] Implement `auto` resolution including the `GET /api/tags` reachability probe. Never pull.
- [x] Implement the provider test/validation endpoint.
- [x] Implement batching, retries with backoff and jitter, cancellation, and bounded concurrency.
- [x] Implement the semantic service lifecycle.
- [x] Generate artist, then album, then track documents in the background.
- [x] Store embeddings in bounded transactions; `CheckpointWAL()` after bulk runs.
- [ ] Incrementally upsert and delete arena entries.
- [x] Implement `library_changes` cursor tailing, including the pruned-log fallback to full rescan.
- [x] Add explicit invalidation for `artist_metadata` / `album_metadata`, which the trigger log does not cover.
- [x] Add the status, rebuild (`reindex` | `reload`), and retry endpoints.
- [x] Add the Settings UI using design tokens for local/auto provider configuration, status, provider test, reindex, and error retry.
- [ ] Add the OpenAI cloud cost estimate and explicit confirmation before a cloud index build.
- [x] Add unit and integration tests with a fake embedding provider.
- [x] Update this plan with implementation notes.

#### PR 2 implementation notes

- **2026-08-26:** Added the provider-neutral embedding contract, strict L2-normalization/shape validation, and the Ollama `POST /api/embed` adapter. The adapter batches 32 documents, sets `truncate: false`, adds the required `nomic-embed-text` document/query prefixes, and can only probe `/api/tags`; it never pulls a model.
- **2026-08-26:** Added encrypted semantic settings, including the `semantic_embedding_api_key` crypto registration test. `auto` resolution honors explicit settings, reuses configured Ollama/OpenAI chat credentials where appropriate, and falls back to the non-mutating local Ollama probe.
- **2026-08-26:** Added the durable `semantic.Service`: content-hash-gated document generation in artist/album/track order, sequential bounded embedding batches, retry/backoff/jitter, cancellation, model-identity reset, transactional persistence, WAL checkpointing, durable-arena reload, and manual error retry. It is intentionally not yet constructed by `api.New`; that integration follows the OpenAI adapter and semantic API endpoints.
- **2026-08-26:** Added durable `library_changes` cursor tailing. Each observed change window runs a full catalog content-hash reconciliation (so aggregate documents and deletions are correct without re-embedding behavioural-only writes), advances all three entity cursors, and falls back to a full reconciliation if a pruned change log leaves a revision gap. Obsolete track, album, and artist documents are removed transactionally before the replacement arenas load.
- **2026-08-26:** Added the explicit metadata invalidation path that `library_changes` cannot provide. Last.fm artist updates, similar-artist updates, and album metadata saves enqueue durable metadata-change records. The semantic worker loads artist tags/bio/similar artists and album genres in bounded bulk queries, reconciles content hashes in the background, and acknowledges each queue page only after reconciliation. This keeps metadata enrichment asynchronous and avoids an N+1 read pattern.
- **2026-08-26:** API startup now owns the semantic service asynchronously and shutdown closes it cleanly. Added `/api/semantic/status`, `/rebuild` (`reindex` or `reload` only), `/retry-errors`, and `/test-embedding-provider`; no endpoint exposes embeddings, documents, or keys. This slice activates locally configured Ollama indexing. An OpenAI semantic configuration is reported as needing configuration until the dedicated adapter is completed.
- **2026-08-26:** Added `GET`/`PUT /api/semantic/settings` and the Settings semantic-index card. Dedicated semantic settings save atomically; API keys remain encrypted and write-only. A settings change retires the previous service and starts a replacement asynchronously, with a monotonic generation guard so a slow prior provider probe cannot overwrite a newer configuration. The UI exposes auto/local Ollama configuration, live status polling, provider test, reindex, and error retry. It deliberately marks OpenAI cloud controls unavailable until the adapter, current price verification, and explicit cost confirmation are implemented.
- **2026-08-26:** Verified the settings/lifecycle slice with focused backend semantic tests, frontend TypeScript check, 30 frontend unit tests, and a production frontend build. The preceding lifecycle/API checkpoint on PR #24 completed successfully in GitHub Actions: [run 33008542425](https://github.com/ajbergh/ViiB-MediaHub/actions/runs/33008542425).
- **2026-08-26:** Corrected the pre-existing Windows-only expectation in `internal/validation.TestSanitizePath` so it compares the cleaned path for the running platform. The sanitization behavior itself is unchanged.

#### Acceptance criteria

- First launch with an existing library begins indexing without blocking startup.
- Progress survives restart because SQLite is authoritative.
- Re-running the indexer with unchanged content produces zero embedding work.
- A metadata enrichment change re-embeds only affected documents and the relevant album/artist aggregates.
- A listening session advances the `library_changes` cursor but produces **zero** embedding requests.
- Truncating `library_changes` below the stored cursor triggers a full content-hash rescan rather than silently skipping changes.
- A model identity change triggers a safe full reindex with no mixed vector space, and AI DJ keeps serving from the previous arena while it runs.
- Ollama indexing works with no cloud dependency, using batched `/api/embed` — assert request count, not just success.
- An Ollama request for `nomic-embed-text` carries `search_document: `, and a query carries `search_query: `.
- OpenAI indexing works without changing the chat provider implementation.
- Indexing a 20,000-track library causes no user-facing SQLite `busy` timeouts.
- Playback and library features remain usable throughout indexing.

---

### PR 3 — Semantic AI DJ retrieval and ranking

**Branch:** `feature/ai-dj-semantic-retrieval`

#### Tasks

- [ ] Add `PlaylistIntent` and the semantic query fields.
- [ ] Update LLM system prompts to compile semantic intent instead of constraining to local genre taxonomy.
- [ ] Extend `DJPhase` with semantic query, negative query, and style hints.
- [ ] Fix `dj.PlanCacheKey.String()` numeric encoding, add `UseTimeContext` and the time bucket, remove the genre-hash dependency, and add a collision test (§10.5).
- [ ] Implement the query embedding cache.
- [ ] Implement track, album, and artist semantic search.
- [ ] Implement album and artist expansion into valid local catalog tracks.
- [ ] Implement hard filters, delegating source filtering to `db.FilterSongsForAIDJ`.
- [ ] Implement semantic evidence aggregation.
- [ ] Implement the negative semantic penalty.
- [ ] Implement `HybridRanker`.
- [ ] Implement the MMR/diversity pass with a bounded working set.
- [ ] Add `SemanticScores` to `dj.ScoreContext`.
- [ ] Reweight `ScoreSongForPhase` around semantic relevance.
- [ ] Replace the `db.GetAllSongs()` recall path in `handleDJMode`; move genre stats to `genre_stats` and average duration to an aggregate query (§13.1).
- [ ] Update the sequencer to consume semantic phase candidate pools.
- [ ] Preserve the legacy fallback path.
- [ ] Preserve current API response fields; add the optional retrieval diagnostics block.
- [ ] Add unit and integration tests.
- [ ] Update this plan with implementation notes.

#### Acceptance criteria

- AI DJ never sends the full library to the LLM.
- Generated playlists contain only existing ViiB song IDs.
- Explicit artist exclusions are always honoured.
- Local/Plex source selection is honoured, including unavailable Plex sources.
- Poorly tagged tracks enter candidate sets through track context or album/artist retrieval.
- Missing energy, mood, or tempo does not prevent a strong semantic match from ranking well.
- Discover mode measurably increases diversity and deep-cut weighting.
- Favorites mode measurably increases user-preference weighting.
- BPM/flow sequencing remains functional.
- Legacy mode works when the semantic service is unavailable.

---

### PR 4 — UI integration, hardening, quality gates, documentation

**Branch:** `feature/ai-dj-semantic-rag-hardening`

#### Tasks

- [ ] Add semantic readiness, indexing, and fallback status to the Smart Playlists UI.
- [ ] Add retrieval diagnostics only where they aid troubleshooting; do not clutter normal UX.
- [ ] Add the 20k+ synthetic-library performance fixture and benchmarks.
- [ ] Add semantic service lifecycle and shutdown tests.
- [ ] Add exactness tests against a naive reference scan, and dimension-inconsistency recovery tests.
- [ ] Add provider failure and retry tests, including a provider returning zero-magnitude vectors.
- [ ] Add a migration-from-existing-user-library test.
- [ ] Run the manual QA prompt matrix (§19).
- [ ] Update `docs/smart-playlists.md`, `docs/dj-mode.md`, and `docs/architecture.md`.
- [ ] Update relevant AI/settings documentation and the README feature summary.
- [ ] Run final backend and frontend production builds, plus the five cross-compile targets.
- [ ] Resolve all static-analysis, lint, and type errors introduced by Phase 1.
- [ ] Mark completed items in this plan.

#### Acceptance criteria

- The Definition of Done in §20 is satisfied.
- No known semantic-index data-loss path exists.
- The feature is usable with 20,000+ catalog tracks.
- Failure of the embedding provider, or an unusable/empty vector set, does not make ViiB unusable.

---

## 18. Testing plan

### 18.1 Unit tests

#### Document builder

- fully populated local track;
- minimally tagged track;
- Plex track via the unified song catalog;
- malformed `LastFMTags` JSON;
- artist context aggregation;
- album context aggregation;
- stable normalization and hashing;
- hash changes when semantic metadata changes;
- **hash does not change** when `play_count`, `last_played`, `skip_count`, or `liked` change — one assertion per field;
- text length cap enforcement;
- task prefix is absent from stored `content`.

#### Vector codec

- encode/decode round trip;
- empty vector rejection;
- malformed BLOB rejection;
- NaN/Inf rejection;
- zero-magnitude rejection;
- dimension validation.

#### Vector index (`scanIndex`)

- add and search;
- in-place update of the same ID;
- delete, and deleted rows never surfacing in results;
- rebuild with free-list compaction;
- dimension mismatch returns an error and does not panic;
- **exactness: results identical to a naive reference scan over random vectors**, not merely overlapping;
- sharded and single-threaded scans agree exactly;
- concurrent read safety and read/write interleaving under `-race`.

#### Retriever

Use a fake deterministic embedding provider; no network access in tests.

- track match;
- artist-only rescue;
- album-only rescue;
- deduplication;
- source hard filter, including an unavailable Plex source;
- excluded artist;
- hard versus soft year constraint;
- instrumental-only;
- negative semantic penalty;
- empty index fallback.

#### Ranker and MMR

- semantic relevance wins over missing metadata;
- favorite boost;
- skip penalty;
- recent-play penalty;
- discover-mode diversity;
- one-per-artist;
- MMR reduces near-duplicate selection;
- MMR working set is bounded, not the full candidate pool;
- deterministic behaviour with seeded inputs.

#### DJ integration

- semantic scores flow into phase scoring;
- phase target counts retained;
- BPM sequencing retained;
- the semantic candidate pool is not replaced by whole-library fallback unless fallback conditions are met;
- `PlanCacheKey` collision test (§10.5).

### 18.2 Integration tests

Synthetic catalog fixture containing at least:

- strongly tagged tracks;
- minimally tagged tracks with good artist context;
- conflicting genre tags;
- multiple decades;
- repeated artists and albums;
- liked / skipped / recently played patterns;
- both local and Plex source identities.

Use fake vectors with known geometry so expected nearest-neighbour ordering is
deterministic. With an exact scan this holds exactly.

### 18.3 Performance regression fixture

Generate ~20,000 track documents plus representative album and artist documents. Targets
are calibrated against the §2.2 baseline, not aspirational:

- p95 exact scan across all three vector sets **< 40 ms** after query embedding is available;
- local candidate expansion, filtering, and ranking **< 150 ms**;
- arena load from stored embeddings **< 250 ms**, and off the blocking startup path regardless;
- semantic in-memory footprint **< 100 MiB** for all three vector sets plus working retrieval data, excluding SQLite page cache and frontend runtime (expected actual ≈ 47 MB at 512 dims).

Regression guards:

- MMR is not run over the full 900-candidate pool;
- indexing a 20k library issues `ceil(n / batchSize)` embedding requests, not `n`.

If a target fails, optimize the selected design. Do not reopen the vector-engine decision
except under the §21.1 trigger.

---

## 19. Manual QA prompt matrix

Run against a real large library where possible. Record results in the PR 4 description.

1. `90 minutes of darker 90s alternative, start mellow like Radiohead, gradually get heavier, some obscure stuff, no Nirvana.`
   — semantic style, energy arc, soft decade constraint, exclusion, discovery.
2. `Quiet Sunday morning music: warm acoustic, jazz, folk and mellow singer-songwriter. Nothing aggressive.`
   — cross-genre semantic concept retrieval.
3. `80s songs I probably know, but avoid anything I've played in the last week.`
   — familiarity plus recency behaviour.
4. `Female-fronted punk and pop-punk with high energy for a workout.`
   — natural-language attribute retrieval despite imperfect local genre tags.
5. `Instrumental focus music with a steady beat, no vocals, not sleepy ambient.`
   — hard instrumental filter plus negative semantics.
6. `Stuff that feels like The Cure but more electronic, favor deep cuts.`
   — artist/style concept plus discover weighting, without an exact genre string.
7. `Play music from my Plex source that fits a late-night atmospheric drive.`
   — source filtering on the unified catalog.
8. Pick a deliberately poorly tagged track whose artist has good Last.fm context, then use a matching semantic prompt.
   — hierarchical rescue.

Evaluation criteria: coherent theme; exclusions obeyed; no non-existent songs; useful
inclusion of poorly tagged tracks; reasonable artist diversity; expected
discovery/favorites behaviour; sensible flow in DJ mode.

---

## 20. Definition of Done

- [ ] ViiB contains a pure-Go semantic index subsystem backed by SQLite, with no new third-party dependency and no on-disk index artifact.
- [ ] `go build` passes for `windows/amd64`, `windows/arm64`, `linux/amd64`, `darwin/amd64`, and `darwin/arm64`.
- [ ] Track, album, and artist semantic documents generate deterministically.
- [ ] Embeddings persist in SQLite, L2-normalized, and the in-memory arena is fully derivable from them.
- [ ] Exact top-k is verified against a naive reference implementation.
- [ ] Ollama embeddings work via batched `/api/embed` with correct task prefixes.
- [ ] OpenAI embeddings work at 512 dimensions.
- [ ] The `auto` decision for all six chat providers is implemented and documented, including those resolving to `needs_configuration`.
- [ ] `semantic_embedding_api_key` is encrypted at rest, with a test proving it.
- [ ] Initial indexing is batched, cancellable, resumable, and non-blocking.
- [ ] Incremental changes update only affected documents, driven by the `library_changes` log including the pruned-log fallback.
- [ ] Model identity changes perform a safe full reindex with no mixed vector space.
- [ ] AI DJ LLM output contains semantic retrieval intent.
- [ ] AI DJ does not rely on exact local genre taxonomy for candidate recall.
- [ ] Both normal playlist mode and DJ mode use semantic retrieval when available.
- [ ] Track, album, and artist retrieval are combined and deduplicated.
- [ ] Hard artist, source, and instrumental constraints are deterministic.
- [ ] User behaviour remains a ranking signal, never embedding content.
- [ ] `dj.PlanCacheKey.String()` no longer encodes integers as Unicode code points and includes time context.
- [ ] Diversity/MMR prevents obvious semantic clustering, over a bounded working set.
- [ ] Existing BPM, flow, and persona logic remain integrated.
- [ ] Legacy fallback remains available and tested.
- [ ] Semantic status and rebuild controls exist in Settings, using design tokens.
- [ ] 20k synthetic-library performance guardrails pass (§18.3).
- [ ] Backend tests pass, including `-race` on the semantic package.
- [ ] `npm run check` passes.
- [ ] Documentation is updated.
- [ ] This plan reflects final implementation status.

---

## 21. Phase 1 deferred items

### 21.1 Approximate nearest-neighbour indexing

Deferred behind `VectorIndex`. Reopen only when **one** of the following is measured, not
anticipated:

- any single entity corpus exceeds 250,000 vectors; or
- p95 retrieval excluding query embedding exceeds 60 ms on target hardware; or
- profiling shows the scan — not the embedding call, not ranking, not SQLite — dominates an AI DJ request.

Prerequisites and constraints established for `github.com/coder/hnsw`, the leading
pure-Go candidate, so the work does not have to re-derive them:

**Blocking.** `encode.go` calls `renameio.TempFile`, and every file in
`github.com/google/renameio` v1.0.1 defining that symbol is guarded `//go:build !windows`.
The package therefore fails to compile for `windows/amd64` and `windows/arm64` on every
published tag and on `main`. This is a compile error in the dependency's own package, so
avoiding `SavedGraph.Save()` does not help. Resolve upstream — replace `renameio` with
temp-file plus `os.Rename` (Go's `os.Rename` on Windows already uses `MoveFileEx` with
`MOVEFILE_REPLACE_EXISTING`), or move to `renameio/v2` — before adopting it.

**Versions.** Tags run to `v0.6.1`. Two required behaviours exist only on unreleased
`main` (`36cab6028fed`, pseudo-version `v0.6.2-0.20260622133054-36cab6028fed`):
`SearchWithDistance`, and non-panicking re-add of an existing key. On `v0.6.1`, `Search`
returns `[]Node[K]` with no distance, and `Add` on an existing key panics with
`"node not added"` despite the doc comment claiming replacement.

**Configuration.** `NewGraph()` defaults are `M = 16`, `Ml = 0.25`, `EfSearch = 20`,
`Distance = CosineDistance`. `EfSearch` must be `>= k`, conventionally `2k`; the Phase 1
quotas (`k = 300` for tracks) require `EfSearch >= 300`. `Import` overwrites `M`, `Ml`,
and `EfSearch` from the file, so configuration must be re-applied after every import.
`Export` writes no dimension field and no checksum; validate via `Graph.Dims()` plus the
DB state row. `Ml == 0` panics. On `v0.6.1`, `layerNode.replenish` hardcodes
`CosineDistance` regardless of the configured metric, so cosine is the only safe metric on
tagged releases.

**Panic and nondeterminism surface.** `assertDims` panics on dimension mismatch in both
`Add` and `Search`, so a wrapper must validate before delegating. `Graph.Rng` seeds from
`time.Now().UnixNano()` unless set. `layer.entry()` returns an arbitrary Go map element,
so the search entry point varies between runs even with a pinned RNG — combined with
approximate search, exact result ordering is not reproducible, and tests must assert
recall against exact ground truth rather than ordering. A corrupt import can leave nil
neighbour pointers that panic later rather than failing `Import`, so import recovery needs
`recover()`. `addNeighbor` documents its own NaN hazard: a zero vector makes
`CosineDistance` return NaN and neighbour eviction becomes arbitrary — the zero-magnitude
rejection in §7.1 is a hard prerequisite.

**Cost.** Transitive dependencies are `github.com/viterin/vek` (AVX/AVX2/AVX512 Go
assembly with a pure-Go fallback, CGO-free), `github.com/chewxy/math32`,
`github.com/google/renameio`, and `golang.org/x/exp`. Vectors are written for every node
in every layer, so an exported file is roughly `1.33 × n × dims × 4` bytes — about 54 MB
at 20k × 512, duplicating data already in SQLite. In memory, vector backing arrays are
shared across layers, so the real overhead is per-node neighbour maps at roughly 1–2 KB
per node, i.e. 20–40 MB at 20k nodes.

### 21.2 Other deferred items

- On-disk index serialization. Worth adding only if arena load becomes a measurable startup cost.
- Gemini embeddings provider, if PR 2 defers the decision (§7.3).
- Audio-native embeddings (CLAP/MuLan-like).
- Audio analysis runtime packaging and hardware acceleration.
- Lyrics semantic indexing.
- Playlist-vector centroids and playlist continuation as a first-class feature.
- Semantic global library search outside AI DJ.
- A `More Like This` context action.
- An optional LLM final-curator pass over a bounded retrieved candidate set.
- Learned user preference vectors.
- Automatic exploration/exploitation tuning from long-term feedback.
- Multiple embedding models active simultaneously.
- Index quantization or compression.

The Phase 1 schema and interfaces must not prevent adding an `audio-semantic-v1`
embedding family later. Phase 1 does not implement it.

---

## 22. Guiding principle

> **The LLM understands the user's intent. The semantic index finds music that matches
> that meaning. ViiB's local ranking and DJ engine decide which valid tracks to play and
> in what order.**

The LLM no longer reasons over the whole library, and AI DJ quality no longer collapses
because a user's files have incomplete genre or mood metadata.
