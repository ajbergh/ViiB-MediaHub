# Development and builds

Setup, architecture, API entry points, data handling, and validation for contributors. For the product tour and downloads, see the [README](../README.md).

## Quick Start

### Prerequisites

| Requirement | Minimum | Notes |
|---|---|---|
| Node.js | 20+ | Frontend tooling; CI uses Node 24 |
| Go | **1.26.9+** | Current security-patched Go 1.26 baseline |
| GCC / Clang | — | Required for native Wails builds and packages that opt into CGO |
| Wails CLI | v2 | Desktop builds |

Install Wails:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.11.0
```

Platform-native build dependencies are documented by Wails. The backend uses the pure-Go `modernc.org/sqlite` driver and can build without CGO; native Wails packaging still uses the repository's platform toolchain and may enable CGO for other native dependencies.

### Development

Install frontend dependencies from the repository root before starting either mode:

```bash
npm ci
```

Windows/Wails:

```powershell
.\scripts\dev-wails.ps1
```

Windows/browser mode:

```powershell
.\scripts\dev.ps1
```

macOS/Linux manual development:

```bash
# Terminal 1
cd backend
go run ./cmd/viib -port 8080 -no-browser

# Terminal 2 (from the repository root)
npm run dev
```

### Normal validation

```bash
npm run check
cd backend
go test ./...
go test -race ./...
go vet ./...
staticcheck ./...
```

Pull-request CI also builds browser/Wails targets, runs binary `govulncheck`, packages the Windows Wails executable, and scans the packaged Windows binary.

---

## Configuring music sources

### Local folders

Use **Settings** to add local music folders. ViiB scans configured roots and incrementally reconciles filesystem changes.

### Plex Media Server

Open **Settings → Library Sources** and use the **Plex Media Server** panel:

1. Search the local network or enter a PMS address manually.
2. Validate/connect the server.
3. Sign in to Plex when the server requires authentication.
4. Select one of the returned music/audio libraries.
5. Synchronize the selected library.

Plex tracks then appear in the normal ViiB library UI. Use **Resynchronize** to refresh the remote catalog. A temporary PMS outage retains already imported catalog metadata and surfaces source availability/authentication state instead of deleting tracks.

See [docs/plex-music.md](plex-music.md) for security, discovery, troubleshooting, and playback details.

---

## Architecture

```text
React / TypeScript UI
        │
        │ REST + SSE + normal media URLs
        ▼
Go backend (chi)
        │
        ├── SQLite canonical catalog
        │     ├── local filesystem songs
        │     └── synchronized Plex songs
        │
        ├── local scanner / metadata extraction
        ├── Plex client / discovery / auth / sync / media proxy
        ├── Spotify session / proxy / downloader
        ├── AI / enrichment services
        └── Library Operations / jobs / recovery
```

Important backend packages:

```text
backend/internal/
├── api/          REST/SSE handlers and source-aware media routes
├── audio/        local audio metadata extraction
├── crypto/       sensitive-setting encryption
├── db/           SQLite catalog, Plex source records, playlists, history
├── llm/          AI providers and enrichment
├── plex/         PMS client, GDM discovery, auth, metadata mapping
├── scanner/      local filesystem scan/reconciliation
├── server/       HTTP router/middleware
├── spotify/      Spotify session, streaming, and downloads
└── validation/   input and sensitive-key classification
```

The Plex integration deliberately extends the existing catalog/playback abstractions rather than creating a second Plex-specific media application.

---

## Data and security

Application data is stored under the OS user configuration directory in `ViiB-MediaHub/`, including the SQLite database, artwork cache, logs, and downloaded Spotify media.

Sensitive settings are encrypted before persistence using ViiB's machine-bound AES-256-GCM settings mechanism. This includes Plex account/server tokens, Plex device private-key material, and Spotify cookie sessions. Normal Spotify requests use backend-owned authentication; session cookies and bearer tokens are omitted from public status responses.

Plex security properties:

- credentials are not returned through Plex configuration APIs;
- credentials are not stored in frontend localStorage/Zustand state;
- browser media URLs contain ViiB song IDs, not Plex tokens;
- PMS authentication is attached server-side;
- credential-like error material is redacted;
- cross-origin Plex asset resolution does not forward the PMS token.

New backups exclude Spotify cookie sessions and OAuth credentials, so Spotify sign-in is required after restore. Other encrypted integration credentials and older archives may still contain sensitive data; protect backups like the application data directory.

---

## API

ViiB retains legacy `/api` routes and provides an additive versioned `/api/v2` surface for scalable library synchronization, indexed search, durable jobs, Library Operations, and Plex source management.

Key source-transparent media routes:

| Method | Endpoint | Purpose |
|---|---|---|
| `GET` | `/api/audio/{songId}` | Play local or Plex catalog audio; Plex requests are authenticated/proxied by the backend |
| `GET` | `/api/cover/{songId}` | Return local or proxied Plex artwork |

Plex management routes live under `/api/v2/plex`:

| Method | Endpoint | Purpose |
|---|---|---|
| `POST` | `/discover` | Run bounded LAN GDM discovery |
| `POST` | `/connect` | Normalize, validate, and connect a PMS address |
| `GET` | `/config` | Return sanitized Plex source/configuration state |
| `DELETE` | `/config` | Remove the ViiB Plex source/cache without modifying PMS |
| `POST` | `/auth/start` | Start Plex device authentication |
| `GET` | `/auth/status` | Poll authentication state |
| `GET` | `/servers` | List Plex servers available to the signed-in account |
| `POST` | `/servers/select` | Connect one account-visible or shared Plex server |
| `GET` | `/artist-artwork/{name}` | Return proxied Plex artist artwork without exposing credentials |
| `GET` | `/libraries` | List selectable music/audio libraries only |
| `PUT` | `/library` | Select a Plex music library |
| `POST` | `/sync` | Start Plex library synchronization |
| `GET` | `/sync/status` | Read Plex synchronization/source status |
| `POST` | `/metadata-writeback/preview` | Review eligible AI genre/year changes without writing to Plex |
| `POST` | `/metadata-writeback/sync` | Apply the exact approved metadata preview and verify it |

The core versioned contract is [docs/openapi-v2.yaml](openapi-v2.yaml).

---

## Build targets

Repository build scripts cover Windows, macOS, and Linux browser/Wails targets. See the scripts under `scripts/` and the CI workflows under `.github/workflows/` for the exact supported build commands and toolchain used by automation.

Every pull request and `main` push packages native Wails desktop builds for Linux x64,
macOS Apple Silicon, macOS Intel, and Windows x64. Successful CI runs expose the Linux
binary as a `.tar.gz` artifact and the unsigned macOS `.app` bundles as `.zip` artifacts;
the macOS CI bundles are intended for validation and require release signing/notarization
before distribution.

The Wails v2 desktop shell keeps the external system tray on Windows and Linux. On
macOS it uses the standard application lifecycle (closing the window exits) because
the legacy tray library and Wails both install a native macOS application delegate.

Typical Windows desktop build:

```powershell
.\scripts\build-wails.ps1
```

Typical web/server build:

```powershell
.\scripts\build.ps1
```

---

## Validation

The current CI baseline validates:

- frontend checks and production dependency audit;
- Go tests and race tests;
- `go vet` and Staticcheck;
- Linux browser and Wails builds;
- binary vulnerability scanning;
- Windows frontend production build;
- Windows Wails `.exe` packaging;
- vulnerability scanning of the packaged Windows executable.

The Plex implementation is covered with fake/`httptest` PMS behavior and does not require a live Plex server in CI. Real-PMS smoke testing remains appropriate before a release candidate, especially for LAN discovery, authentication, large-library sync, codec coverage, and long-running direct playback.

---

## Browser smoke gate

After `npm ci` and `npm run build`, install the locked Chromium revision with
`npx playwright install chromium` (Linux CI uses `--with-deps`), then run
`npm run test:e2e`. Playwright starts a production preview at `127.0.0.1:4173`.
Keep this port free; CI always starts its own preview server.

The initial suite exercises browser-only empty-library navigation and the legacy
DJ redirect/reload, playlist creation/reload and deletion cancellation using fresh IndexedDB contexts and locally intercepted API
failures. External requests are blocked. No backend, accounts, credentials or
real music are needed. This does not qualify playback, native WebViews, import,
source repair or provider sessions. The synthetic recovery journey separately
fulfills only backup-list, diagnostics, validation and restore-stage endpoints.
It checks the Library Operations redirect, consequence disclosures, cancellation
without mutation and selected-archive staging/restart messaging. It does not
activate a database or verify recovery integrity. Add fixture-backed journeys
incrementally.

CI uses the frontend production artifact. Failures retain screenshots, traces
and an HTML report under `output/playwright/` for seven days. Use only synthetic
fixtures: traces can include network bodies and browser storage. View a report
with `npx playwright show-report output/playwright/report`.

CI publishes an npm CycloneDX SBOM from `package-lock.json` for 14 days.
Staticcheck `v0.8.1` and govulncheck `v1.6.0` are pinned across workflows;
update all install sites together after compatibility validation. The npm SBOM
is a dependency inventory, not a Go/native SBOM or license approval.

## TypeScript contract gate

`npm run typecheck` runs global strict TypeScript checking and the retained
focused boundary check. The global program explicitly includes repository
TS/TSX (including tests/configs) and excludes dependency, output, dist, build
and worktree directories. Imported generated Wails declarations remain available.

`allowJs: false` keeps JavaScript bodies out of this type gate; Vite still
bundles those runtime modules. The scratch worklet and Tailwind config have
concrete adjacent declarations rather than untyped module stubs. Their existing
runtime tests/build remain necessary. Existing explicit `any`, suppression
comments and `skipLibCheck` are not eliminated by enabling strict mode; migrate
those contracts separately without adding new escapes.
