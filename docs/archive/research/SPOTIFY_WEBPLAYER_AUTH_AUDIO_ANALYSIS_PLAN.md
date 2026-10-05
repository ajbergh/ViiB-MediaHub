# Spotify WebPlayer Authentication and Internal Audio Analysis Plan

> Historical record archived on 2026-10-04. Status, branch names, and validation results below describe the recorded checkpoint; unresolved runtime/quality checks are not closed by archiving. See the [current documentation](../../index.md) and [archive index](../README.md). Superseded by the [implementation plan](../../SPOTIFY_WEBPLAYER_AUTH_AUDIO_ANALYSIS_IMPLEMENTATION_PLAN.md), [parity audit](../../SPOTIFY_COOKIE_AUTH_PARITY_AUDIT.md), and [validation log](../../SPOTIFY_WEBPLAYER_VALIDATION.md).

**Status:** Planning / research document — implementation not yet authorized by this document  
**Research date:** 2026-09-30  
**Repository verification snapshot:** 9240afdb39cdf7d5539efa80988885d7a4259732 (main)  
**Repository:** ajbergh/ViiB-MediaHub  
**Primary scope:** Spotify authentication architecture, internal Spotify audio-analysis access, DJv2 BPM/key enrichment, and a possible future path away from mandatory Spotify Developer app setup  
**Related roadmap:** [DJV2_PROFESSIONAL_TRACK_ANALYSIS_ROADMAP.md](../../DJV2_PROFESSIONAL_TRACK_ANALYSIS_ROADMAP.md)

> This document preserves the September 30, 2026 design/research session around Spotify BPM/key access and WebPlayer authentication. It intentionally records the reasoning, alternatives, risks, and open questions, not only the preferred implementation. Undocumented Spotify interfaces are not stable contracts and may change without notice.

---

## 1. Executive summary

Spotify removed general developer access to the public Audio Features and Audio Analysis Web API endpoints for newer/development-mode applications beginning in late 2024. That blocks the simple historical ViiB approach of asking the normal Spotify Web API for BPM, musical key, mode, beat data, and related analysis fields.

Research performed in September 2026 shows that Spotify first-party clients and current third-party projects still use an internal Spotify service under:

    https://spclient.wg.spotify.com/audio-attributes/v1/

The most important endpoint for ViiB is:

    GET /audio-analysis/{spotify_track_id}

The response is the familiar detailed Spotify analysis payload and includes:

- track.tempo
- track.tempo_confidence
- track.key
- track.key_confidence
- track.mode
- track.mode_confidence
- track.time_signature
- track.time_signature_confidence
- track.loudness
- bars
- beats
- sections
- segments
- tatums

This is significantly more valuable than just restoring BPM and key. It may provide an external reference for ViiB tempo/key quality, beat-grid validation, cue-point research, section/phrase analysis, and DJ workflow enrichment.

The internal service does not use ViiB's current Spotify Developer OAuth flow in the same way as the public Web API. Current first-party WebPlayer behavior uses a Spotify-authenticated web session, a time-based one-time-password mechanism, and:

    https://open.spotify.com/api/token

to obtain a short-lived WebPlayer bearer token. That bearer can authenticate internal spclient requests.

The recommended architecture is therefore **not an immediate global replacement of ViiB's current OAuth PKCE flow**. Instead, introduce a second, backend-owned WebPlayer authentication provider, use it first for internal audio-analysis requests, then systematically test whether that token can safely replace other existing Spotify token consumers.

Longer term, if WebPlayer authentication proves sufficient for the required browse, playback, download, and metadata surfaces, ViiB may be able to remove the current requirement that each user create and configure a Spotify Developer application. That is a strategic possibility, not a current assumption.

---

## 2. Why this matters to ViiB

ViiB's DJv2 roadmap already includes a substantial local analysis engine for:

- tempo/BPM;
- key and mode;
- Camelot mapping;
- beatgrid;
- energy/structure;
- cue suggestions;
- harmonic-mixing support.

Local analysis should remain a first-class capability because it works with non-Spotify media and does not depend on an undocumented remote service.

Spotify analysis is nevertheless valuable for four reasons.

### 2.1 External reference data

Spotify's own tempo/key values can act as one external comparator for ViiB's pure-Go analyzer. A sufficiently large matched corpus can answer questions such as:

- How often does ViiB agree with Spotify within ±0.5 BPM?
- Where does ViiB choose half/double tempo relative to Spotify?
- What key disagreements are exact, relative-major/minor, adjacent Camelot, or completely divergent?
- Does ViiB confidence correlate with Spotify confidence?
- Which genres or production styles produce the largest divergence?

Spotify should not automatically be treated as ground truth, but it is a highly useful independent reference.

### 2.2 Better Spotify-native DJ metadata

For a Spotify track, ViiB could show or cache Spotify-derived values alongside local values:

    BPM        124.02
    Key        8A
    Source     Spotify

or:

    BPM        123.97
    Key        8A
    Source     ViiB Analysis

Provenance is essential. ViiB should never silently mix Spotify-provided facts with locally measured facts.

### 2.3 Rich structural analysis

The internal audio-analysis response includes bars, beats, sections, segments, and tatums. These could later contribute to:

- beat-grid validation;
- phrase boundary research;
- automatic cue candidates;
- transition preparation;
- waveform markers;
- section-aware recommendations;
- quality checks against ViiB's own structure engine.

### 2.4 Better Spotify onboarding may be possible

The current ViiB documentation requires a user to:

1. create a Spotify Developer application;
2. configure a Client ID;
3. register ViiB's redirect URI;
4. complete OAuth PKCE;
5. maintain access/refresh tokens.

If a WebPlayer-session architecture can eventually satisfy the required ViiB Spotify capabilities, the UX could potentially become:

    Connect Spotify
          ↓
    Sign in to Spotify
          ↓
    ViiB retains a local authenticated Spotify web session
          ↓
    ViiB derives short-lived tokens as needed

That would materially reduce setup complexity.

---

## 3. Verified current ViiB architecture

At repository snapshot 9240afdb39cdf7d5539efa80988885d7a4259732, ViiB uses Spotify OAuth 2.0 with PKCE.

Relevant code includes:

- services/spotifyService.ts
- pages/SpotifyCallback.tsx
- backend/internal/api/spotify_token.go
- backend/internal/api/spotify.go
- backend/internal/spotify/session.go
- slices/spotifySlice.ts
- docs/spotify.md

### 3.1 Current authentication flow

The frontend generates a PKCE verifier/challenge and sends the user to:

    https://accounts.spotify.com/authorize

The authorization code is exchanged at:

    https://accounts.spotify.com/api/token

ViiB stores:

- Client ID;
- access token;
- refresh token;
- token expiry;
- legacy/client-secret-compatible fields where still represented by the settings shape.

The frontend intentionally does not persist access/refresh tokens in renderer localStorage. The Go backend owns persistent Spotify credentials and refreshes them.

### 3.2 One token currently serves multiple roles

The current architecture effectively treats the Spotify OAuth access token as a universal credential.

It is used for normal Spotify Web API operations such as:

    https://api.spotify.com/v1/me
    https://api.spotify.com/v1/search
    https://api.spotify.com/v1/albums/...
    https://api.spotify.com/v1/playlists/...

It is also handed to the backend librespot/respot session manager:

    ctx := respot.DefaultSessionContext("ViiB MediaHub")
    ctx.Login.AuthToken = sm.accessToken
    sess, err := respot.StartNewSession(ctx)
    err = sess.Login()

That session supports Spotify streaming/download behavior.

This coupling is important: changing the token-generation method globally could affect catalog browsing, user-library access, playback, and downloads simultaneously.

---

## 4. 2026 Spotify API situation

### 4.1 Public Audio Features / Audio Analysis

Spotify restricted access to the historical public endpoints for newer/development-mode applications. The affected surfaces include the traditional:

    GET https://api.spotify.com/v1/audio-features/{id}

and:

    GET https://api.spotify.com/v1/audio-analysis/{id}

Grandfathered applications with appropriate legacy/extended access may still have different behavior, but ViiB should not design around obtaining such access.

### 4.2 Internal audio-attributes service

Current code in the Spotify customization ecosystem shows active use of:

    https://spclient.wg.spotify.com/audio-attributes/v1/audio-analysis/{id}

Current Spicetify-related projects also reference:

    https://spclient.wg.spotify.com/audio-attributes/v1/audio-features/{id}?format=json

and batch forms resembling:

    https://spclient.wg.spotify.com/audio-attributes/v1/audio-features?ids=id1,id2,...

The strongest 2026 evidence for ViiB's immediate use case is the audio-analysis route. Current implementations have live-verified that it returns track-level tempo, key, mode, loudness and confidence fields, plus the detailed structural arrays.

The audio-features route appears more client/context dependent and should be treated as optional enrichment rather than the primary dependency.

---

## 5. Spotify audio-analysis data model

A representative response shape is conceptually:

~~~json
{
  "track": {
    "tempo": 124.017,
    "tempo_confidence": 0.91,
    "key": 9,
    "key_confidence": 0.74,
    "mode": 1,
    "mode_confidence": 0.65,
    "time_signature": 4,
    "time_signature_confidence": 0.97,
    "loudness": -6.42
  },
  "bars": [],
  "beats": [],
  "sections": [],
  "segments": [],
  "tatums": []
}
~~~

Spotify key numbering follows pitch class:

| Value | Key |
|---:|---|
| -1 | unknown |
| 0 | C |
| 1 | C#/Db |
| 2 | D |
| 3 | D#/Eb |
| 4 | E |
| 5 | F |
| 6 | F#/Gb |
| 7 | G |
| 8 | G#/Ab |
| 9 | A |
| 10 | A#/Bb |
| 11 | B |

Mode is generally:

| Value | Mode |
|---:|---|
| 0 | minor |
| 1 | major |

ViiB can map key + mode through its existing Camelot model.

---

## 6. WebPlayer authentication research

The current first-party WebPlayer token flow is distinct from ViiB's normal accounts.spotify.com PKCE flow.

A current observed architecture is approximately:

    authenticated Spotify web session
                  │
                  │ sp_dc / related session cookies
                  ▼
       open.spotify.com/api/server-time
                  │
                  ▼
        Spotify WebPlayer TOTP
                  │
                  ▼
          open.spotify.com/api/token
                  │
                  ▼
          WebPlayer bearer token
                  │
                  ▼
         spclient.wg.spotify.com

Current requests use parameters resembling:

    reason=transport
    productType=web-player
    totp=<current generated value>
    totpServer=<server-time generated value>
    totpVer=<current WebPlayer TOTP version>

and headers such as:

    App-Platform: WebPlayer

The resulting token is then used with internal endpoints, commonly with headers resembling:

    Authorization: Bearer <webplayer token>
    Spotify-App-Version: <current Spotify web client version>
    App-Platform: WebPlayer
    Accept: application/json

Both the TOTP version/secret derivation and Spotify-App-Version are implementation details, not stable public API contracts. ViiB must therefore isolate these values behind a provider implementation rather than spread them through the application.

---

## 7. Why WebPlayer auth should not immediately replace PKCE globally

The tempting architecture is:

    sp_dc
      ↓
    WebPlayer TOTP
      ↓
    one bearer token
      ├── public Web API
      ├── internal spclient
      └── librespot/respot

That may ultimately be possible for some or all surfaces, but it is unsafe to assume.

The current ViiB OAuth token is known to work for:

- /v1/me;
- search;
- album detail;
- playlist detail;
- user library surfaces;
- current Spotify frontend/backend routes;
- the existing respot login path.

Research around WebPlayer tokens shows that some public Web API endpoints and token classes do not behave identically. A token that successfully calls spclient is not automatically a guaranteed substitute for every api.spotify.com endpoint.

Therefore the initial architecture should support **multiple token classes**.

---

## 8. Recommended target architecture

### 8.1 Dual-provider model

Recommended first implementation:

    ┌────────────────────────────────────────────┐
    │          ViiB Spotify Auth Manager         │
    └──────────────────────┬─────────────────────┘
                           │
              ┌────────────┴─────────────┐
              │                          │
              ▼                          ▼
      Official OAuth PKCE         WebPlayer Session
              │                          │
         OAuth token                sp_dc/session
              │                          │
              ▼                          ▼
      api.spotify.com           TOTP token refresh
              │                          │
              │                          ▼
              │                spclient.wg.spotify.com
              │                          │
              │                 audio-attributes
              │                          │
              └──────────────┬───────────┘
                             ▼
                    normalized Spotify data

This preserves all known-good ViiB behavior while unlocking internal analysis.

### 8.2 Token purpose must become explicit

Code should stop asking for a generic "Spotify access token" when the caller really requires a specific credential class.

Conceptual API:

~~~go
type TokenPurpose int

const (
    TokenPurposeWebAPI TokenPurpose = iota
    TokenPurposeInternal
    TokenPurposePlayback
)

type SpotifyToken struct {
    AccessToken string
    ExpiresAt   time.Time
    Kind        string
}

type AuthManager interface {
    Token(ctx context.Context, purpose TokenPurpose) (SpotifyToken, error)
}
~~~

Initially:

- WebAPI → OAuth PKCE provider;
- Internal → WebPlayer provider;
- Playback → existing OAuth token until tested otherwise.

This prevents accidental substitution of the wrong token type.

---

## 9. Proposed credential model

The existing logical credential shape is centered on OAuth:

~~~go
type SpotifyCredentials struct {
    ClientId     string
    ClientSecret string
    AccessToken  string
    RefreshToken string
    Expiry       int64
}
~~~

A future-compatible model should separate auth systems:

~~~go
type SpotifyCredentials struct {
    OAuth     *SpotifyOAuthCredentials
    WebPlayer *SpotifyWebPlayerCredentials
}

type SpotifyOAuthCredentials struct {
    ClientID     string
    AccessToken  string
    RefreshToken string
    Expiry       int64
}

type SpotifyWebPlayerCredentials struct {
    SPDC string
}
~~~

A cached WebPlayer bearer may exist in memory:

~~~go
type CachedWebPlayerToken struct {
    AccessToken string
    Expiry      time.Time
}
~~~

The short-lived derived bearer should not need durable persistence. The durable secret is the authenticated session material used to obtain replacement tokens.

---

## 10. Security model

WebPlayer authentication changes the credential sensitivity model.

### 10.1 sp_dc is sensitive session material

Treat sp_dc at least as carefully as an OAuth refresh token.

Required invariants:

- never write it to renderer localStorage;
- never expose it through public settings APIs;
- never include it in logs;
- never include it in crash diagnostics;
- never send it to ViiB-hosted infrastructure;
- encrypt it using the backend's existing sensitive-setting mechanism or a future OS credential store;
- keep token derivation in the Go backend;
- redact Authorization and Cookie headers from diagnostics.

### 10.2 Derived bearer tokens

Derived WebPlayer bearer tokens should preferably be:

- held in backend memory;
- refreshed shortly before expiry;
- discarded on logout;
- refreshed on 401/403 at most once per request path;
- never logged.

### 10.3 Logout

"Log out of Spotify" should eventually clear both authentication domains:

- OAuth access/refresh credentials;
- WebPlayer session material;
- cached WebPlayer bearer;
- active respot session.

During the dual-auth transition, UI copy should be explicit if disconnecting one provider does not yet disconnect the other.

---

## 11. Obtaining the WebPlayer session

This is the largest product/UX question.

A developer-only prototype can manually supply sp_dc. That is acceptable only to prove the internal endpoint.

A production ViiB experience should **not** require instructions such as:

    Open browser developer tools
    Find Spotify cookies
    Copy sp_dc
    Paste into ViiB

That is fragile and teaches users to handle a powerful session credential manually.

### 11.1 Preferred production direction

ViiB should launch a controlled Spotify login experience and capture the resulting authenticated session locally.

ViiB already has useful building blocks:

- Wails desktop runtime;
- Go backend;
- chromedp dependency;
- browser launching code;
- encrypted backend settings.

Candidate approaches should be tested for Spotify compatibility and cross-platform behavior:

1. dedicated browser profile controlled by the backend;
2. a Wails/webview login surface where cookie access is safely available to native code;
3. chromedp-based browser session with a persistent local profile;
4. external-browser login plus a carefully designed local handoff if direct cookie access is not available.

The implementation must not scrape user passwords or intercept form credentials. The goal is to retain Spotify's resulting authenticated session after Spotify itself performs login.

### 11.2 Prototype shortcut

For Phase 0/1 engineering validation, support a developer-only setting or environment variable for sp_dc. Keep it explicitly unavailable from normal production UI.

This allows the token-generation and audio-analysis path to be proven independently from the eventual login UX.

---

## 12. Proposed backend package structure

One possible structure:

    backend/internal/spotify/
        auth/
            manager.go
            oauth.go
            webplayer.go
            webplayer_totp.go
            credentials.go
        analysis/
            client.go
            models.go
            normalize.go
        session.go
        streamer.go
        downloader.go

Responsibilities:

### auth/manager.go

- chooses provider by token purpose;
- owns refresh mutexes;
- exposes token provenance;
- centralizes redaction rules.

### auth/oauth.go

- moves/encapsulates current PKCE refresh behavior;
- remains responsible for normal Web API tokens until migration evidence says otherwise.

### auth/webplayer.go

- reads encrypted WebPlayer session material;
- obtains Spotify server time;
- requests/refreshes WebPlayer bearer;
- caches expiry;
- provides internal-api token.

### auth/webplayer_totp.go

- isolates current TOTP algorithm and version;
- includes deterministic test vectors;
- makes future Spotify changes local to one implementation.

### analysis/client.go

- sends audio-attributes requests;
- handles 401/403 refresh;
- validates response shape;
- supports context cancellation/timeouts;
- never leaks auth headers.

### analysis/models.go

- typed Spotify analysis response;
- no direct dependency from DJ UI on raw remote JSON.

### analysis/normalize.go

- maps Spotify key/mode to ViiB's normalized key representation and Camelot;
- records provenance and confidence.

---

## 13. Example internal audio-analysis client

Conceptual Go shape:

~~~go
type SpotifyAudioAnalysis struct {
    Track struct {
        Tempo                   float64 `json:"tempo"`
        TempoConfidence         float64 `json:"tempo_confidence"`
        Key                     int     `json:"key"`
        KeyConfidence           float64 `json:"key_confidence"`
        Mode                    int     `json:"mode"`
        ModeConfidence          float64 `json:"mode_confidence"`
        TimeSignature           int     `json:"time_signature"`
        TimeSignatureConfidence float64 `json:"time_signature_confidence"`
        Loudness                float64 `json:"loudness"`
    } `json:"track"`

    Bars     []SpotifyInterval `json:"bars"`
    Beats    []SpotifyInterval `json:"beats"`
    Sections []SpotifySection  `json:"sections"`
    Segments []SpotifySegment  `json:"segments"`
    Tatums   []SpotifyInterval `json:"tatums"`
}
~~~

Request:

    GET https://spclient.wg.spotify.com/audio-attributes/v1/audio-analysis/{id}

Headers:

    Authorization: Bearer <WebPlayer bearer>
    Spotify-App-Version: <provider-managed current version>
    App-Platform: WebPlayer
    Accept: application/json

Implementation requirements:

- bare Spotify track ID only;
- URL-escape track ID;
- 15–30 second timeout;
- one auth refresh retry on 401/403;
- no blind infinite retry;
- distinguish not-found/unsupported from authentication failure;
- preserve raw confidence values;
- validate tempo/key ranges before persistence.

---

## 14. Track-analysis provenance model

Do not overwrite local analysis as though Spotify and ViiB are the same measurement source.

A normalized external result should resemble:

~~~go
type AnalysisSource string

const (
    AnalysisSourceViiB            AnalysisSource = "viib"
    AnalysisSourceSpotifyInternal AnalysisSource = "spotify_internal"
    AnalysisSourceReccoBeats      AnalysisSource = "reccobeats"
)

type ExternalTrackAnalysis struct {
    SpotifyTrackID string

    BPM           *float64
    BPMConfidence *float64

    Key           *int
    Mode          *int
    KeyConfidence *float64

    Camelot       *string
    LoudnessDB    *float64
    TimeSignature *int

    Source        AnalysisSource
    RetrievedAt   time.Time
    SourceVersion string
}
~~~

Important rule:

> ViiB-local measured facts and remote provider facts are separate observations. An "effective" value may choose one according to product policy, but storage should retain provenance.

---

## 15. Suggested provider/fallback hierarchy

For Spotify-linked tracks:

    Spotify internal analysis
            │
            ├── success → retain Spotify result
            │
            └── unavailable
                    ↓
              ReccoBeats (optional)
                    │
                    └── unavailable
                            ↓
                     ViiB local analysis

However, the product may choose a different **effective-value policy**.

For example, ViiB DJ could prefer local analysis because it is computed from the exact local/downloaded bytes, while still displaying Spotify as a comparator. That question should be decided independently from retrieval order.

Recommended initial policy:

- always keep ViiB analysis;
- retrieve Spotify analysis when a Spotify ID is known;
- display source/provenance in advanced/debug views;
- use Spotify primarily as validation/reference until quality policy is explicitly decided.

---

## 16. ReccoBeats role

ReccoBeats can remain a useful fallback/enrichment service because it exposes Spotify-ID-oriented audio features without ViiB owning Spotify internal authentication.

But its values should not be labeled as Spotify values.

Use explicit provenance:

    spotify_internal
    reccobeats
    viib

This prevents future benchmarking from accidentally treating independently computed ReccoBeats values as Spotify ground truth.

---

## 17. Relationship to DJv2 quality work

Spotify analysis should augment, not bypass, the existing DJv2 professional-quality gate.

Useful benchmark categories:

### Tempo

- strict absolute error;
- ±0.5 BPM;
- ±1.0 BPM;
- metrical match;
- half/double rate;
- unknown/refusal rate;
- confidence calibration.

### Key

- exact key + mode;
- pitch-class-only;
- relative-major/minor;
- Camelot-compatible neighbor;
- total disagreement;
- unknown/refusal.

### Structural analysis

Future experiments can compare:

- ViiB beat positions vs Spotify beats;
- ViiB downbeat/phrase inference vs Spotify bars/sections;
- cue recommendations vs section boundaries.

Do not silently tune ViiB to reproduce Spotify if that reduces accuracy against a better labeled corpus. Spotify is a comparator, not the sole target.

---

## 18. Phased implementation plan

### Phase 0 — document and protect current behavior

Goal: create seams before changing authentication.

Tasks:

- introduce explicit token-purpose abstractions;
- add tests covering current OAuth refresh behavior;
- enumerate every current consumer of Spotify accessToken;
- ensure secrets are absent from logs;
- document current API surface by consumer.

Exit criteria:

- no behavior change;
- all current Spotify browse/stream/download tests remain green;
- token consumers are classified as WebAPI, Internal, or Playback.

### Phase 1 — developer-only WebPlayer provider

Goal: prove token generation and internal audio analysis without changing normal login UX.

Tasks:

- implement server-time retrieval;
- implement current WebPlayer TOTP algorithm in Go;
- add deterministic TOTP tests;
- accept sp_dc only through a developer-only secure configuration path;
- implement open.spotify.com/api/token refresh;
- cache derived token in memory;
- implement audio-analysis request;
- create a diagnostic command/test path for a known Spotify track ID.

Exit criteria:

- repeated token refresh works;
- known track returns tempo/key/mode;
- 401/403 refresh path works;
- no sensitive data appears in logs;
- existing PKCE behavior untouched.

### Phase 2 — normalized Spotify analysis provider

Goal: make remote analysis consumable by DJv2/analysisbench.

Tasks:

- typed response structs;
- normalize key/mode/Camelot;
- persist source/provenance;
- add optional Spotify external-analysis table or appropriate existing evidence store;
- expose retrieval through backend API;
- add cache age/version semantics.

Exit criteria:

- Spotify analysis survives restart if persistence is desired;
- ViiB local result remains independently available;
- UI can distinguish source.

### Phase 3 — benchmark integration

Goal: use Spotify as an external comparator for ViiB's analyzer.

Tasks:

- import/match Spotify IDs for lawful benchmark corpus entries;
- batch retrieval with rate controls;
- compare ViiB vs Spotify;
- produce tempo/key disagreement reports;
- retain confidence values;
- do not modify production selection automatically.

Exit criteria:

- reproducible comparison report;
- no provider values mislabeled as ground truth;
- no benchmark dependency on manual browser work after auth is configured.

### Phase 4 — production WebPlayer login UX

Goal: eliminate manual cookie handling.

Tasks:

- spike login approaches on Windows/macOS/Linux;
- select controlled-browser/WebView architecture;
- retain resulting session locally;
- persist sensitive session data encrypted;
- implement disconnect/logout;
- add expiry/re-auth UX.

Exit criteria:

- no DevTools/cookie copy required;
- no Spotify password processed by ViiB code;
- login works across supported desktop OSes;
- credential lifecycle is understandable to the user.

### Phase 5 — playback-token compatibility experiment

Goal: determine whether WebPlayer bearer can replace OAuth token for respot.

Test exact ViiB dependency:

    github.com/art-media-platform/librespot-go
    revision 0b9301b09744

Matrix:

| Test | OAuth token | WebPlayer token |
|---|---|---|
| StartNewSession | baseline | test |
| Login | baseline | test |
| PinTrack | baseline | test |
| audio-key request | baseline | test |
| sustained stream | baseline | test |
| concurrent downloads | baseline | test |
| session reset/recovery | baseline | test |

Do not migrate playback until this passes real-account testing.

### Phase 6 — Web API compatibility experiment

Goal: determine whether any official Web API calls can safely move away from PKCE.

Test individually:

- /v1/me;
- /v1/search;
- album detail;
- playlist detail;
- saved albums/library;
- recently played;
- every route currently proxied by backend/internal/api/spotify.go;
- frontend direct calls that remain.

Record HTTP status, required headers/scopes, and behavior over token refresh.

Do not infer capability from one successful endpoint.

### Phase 7 — optional OAuth retirement

Only consider removing the Spotify Developer application requirement if WebPlayer auth provides all required production capabilities or ViiB has stable alternate internal implementations for missing Web API features.

Decision gate:

- browse parity;
- library parity;
- playback parity;
- download parity;
- reliable login UX;
- cross-platform session lifecycle;
- acceptable maintenance risk;
- legal/terms review;
- graceful failure/fallback story.

Until this gate passes, dual-auth remains the correct architecture.

---

## 19. Test matrix

### Authentication

- valid sp_dc;
- expired/revoked sp_dc;
- malformed session value;
- TOTP clock skew;
- Spotify server-time unavailable;
- TOTP version changed;
- token endpoint 401/403;
- token endpoint 429;
- transient 5xx;
- offline startup;
- logout during request;
- concurrent refresh callers.

### Internal analysis

- valid track;
- invalid track ID;
- unavailable track;
- regional availability mismatch;
- podcast/episode ID accidentally supplied;
- local track with no Spotify ID;
- key = -1;
- low confidence;
- no structural arrays;
- very large analysis payload;
- rate limiting.

### Security

- logs contain no Cookie header;
- logs contain no Authorization bearer;
- settings API never returns sp_dc;
- renderer cannot read sp_dc;
- database value is encrypted where expected;
- logout deletes session material;
- crash report redaction.

### Regression

Existing ViiB functionality must continue to work:

- Spotify login;
- search;
- album detail;
- playlists;
- saved albums;
- recently played;
- Spotify streaming;
- downloads;
- token refresh;
- startup session restoration.

---

## 20. Failure-handling policy

Undocumented Spotify services will break eventually. The feature must degrade safely.

Recommended states:

- available;
- authentication_required;
- temporarily_unavailable;
- rate_limited;
- unsupported_track;
- provider_changed;
- disabled.

DJ/local analysis must continue working even when Spotify internal analysis is unavailable.

No user-visible core ViiB music functionality should depend exclusively on undocumented audio analysis.

---

## 21. Observability

Safe metrics/logging should capture:

- provider name;
- endpoint category, not full URL with secrets;
- HTTP status;
- request duration;
- token refresh count;
- cache hit/miss;
- analysis success/failure reason;
- Spotify track ID only if existing logging policy permits it.

Never log:

- sp_dc;
- Cookie headers;
- Authorization headers;
- raw bearer token;
- full credential JSON.

A developer diagnostic could expose:

    WebPlayer session: configured
    WebPlayer bearer: valid until <time>
    Audio-analysis probe: HTTP 200
    Track tempo present: yes
    Track key present: yes

without exposing secret values.

---

## 22. Product UX concepts

### Current UX

    Settings
      Spotify Client ID
      Redirect URI registration
      Connect with Spotify

### Possible future UX

    Spotify
    ─────────────────────────────
    Status: Not connected

          [ Connect Spotify ]

    Sign in with Spotify to browse,
    stream, download, and enrich tracks.

Advanced diagnostics can separately show:

    Public Web API      Connected
    Internal analysis  Connected
    Playback session   Connected

During migration, avoid presenting the implementation distinction to normal users unless an error requires it.

---

## 23. Important implementation questions still open

1. What is the most robust way for Wails/Go to obtain and retain the authenticated Spotify WebPlayer session on Windows, macOS, and Linux without manual cookie copying?

2. Does the current art-media-platform/librespot-go revision accept a WebPlayer bearer reliably for the exact ViiB login/audio-key/download path?

3. Which api.spotify.com routes, if any, accept the derived WebPlayer token consistently in September 2026?

4. Is the internal audio-features endpoint useful enough to support, or should ViiB depend only on audio-analysis for Spotify-derived DJ facts?

5. What cache TTL should ViiB use for Spotify analysis results? The underlying analysis is effectively track-version metadata and likely changes rarely, but undocumented responses provide no compatibility promise.

6. Should external Spotify results live in a new provider-observation table, the analysis benchmark evidence system, or an extension of track_analysis with strict provenance columns?

7. When Spotify and ViiB disagree, what should the default DJ UI display? This requires an explicit product policy rather than "Spotify always wins."

8. How should Spotify IDs be matched to local/Plex media? Existing source metadata, ISRC, downloaded provenance, and conservative title/artist matching may all contribute, but false matches are unacceptable for benchmarking.

9. Does shipping a production feature based on these internal interfaces fit Spotify's then-current terms and ViiB's distribution goals? This must be reviewed before release, not assumed from technical feasibility.

---

## 24. Decision log from the planning session

### Decision A — do not abandon local analysis

**Decision:** Keep the ViiB local analyzer as a core capability.

**Reasoning:** ViiB supports local/Plex/non-Spotify media and needs deterministic functionality independent of a private Spotify service.

### Decision B — Spotify internal audio-analysis is worth pursuing

**Decision:** Build a research/prototype provider for audio-analysis.

**Reasoning:** It exposes Spotify's own tempo/key/mode confidences and rich beat/section structure, providing both DJ metadata and valuable comparison evidence.

### Decision C — do not scrape Spotify HTML for BPM/key

**Decision:** Prefer spclient audio-attributes over rendered-page scraping.

**Reasoning:** Current ecosystem code shows a direct internal data service. HTML scraping adds brittleness without improving data fidelity.

### Decision D — WebPlayer auth should initially coexist with OAuth PKCE

**Decision:** Adopt a dual-provider architecture first.

**Reasoning:** Current OAuth is known to support ViiB's public Web API and respot consumers. WebPlayer tokens are known to support internal spclient, but are not proven as universal substitutes.

### Decision E — token purpose must become explicit

**Decision:** Replace the conceptual "one Spotify access token" dependency with purpose-aware credential retrieval.

**Reasoning:** It prevents future internal/public/playback token mix-ups and makes gradual migration possible.

### Decision F — never make cookie copying the production user experience

**Decision:** Manual sp_dc input is acceptable only for a developer prototype.

**Reasoning:** sp_dc is sensitive session material and a normal user should not be taught to extract and paste it.

### Decision G — explore eventual removal of Spotify Developer app setup

**Decision:** Treat this as a strategic future goal, not an initial requirement.

**Reasoning:** If WebPlayer auth covers all required surfaces, ViiB could offer much easier onboarding. But browse/playback compatibility and maintenance risk must first be proven.

---

## 25. Useful 2026 research references

These are research references, not contractual APIs.

### Spotify developer change background

- Spotify developer blog, November 2024: changes to Web API access for new apps  
  https://developer.spotify.com/blog/2024-11-27-changes-to-the-web-api

### Internal audio analysis usage

- Spicetify CLI helper code referencing audio-attributes/audio-analysis  
  https://github.com/spicetify/cli

- Pithaya/spicetify-apps internal spclient audio feature/analysis helpers  
  https://github.com/Pithaya/spicetify-apps

- L3-N0X/spicetify-dj-info — DJ metadata using internal Spotify APIs  
  https://github.com/L3-N0X/spicetify-dj-info

- stephancill/stupid-social — 2026 WebPlayer token/probe implementation and live audio-analysis verification  
  https://github.com/stephancill/stupid-social

### Related client research

- jpochyla/psst  
  https://github.com/jpochyla/psst

- librespot  
  https://github.com/librespot-org/librespot

### Alternate audio-feature source

- ReccoBeats  
  https://reccobeats.com/

---

## 26. Suggested first engineering branch

Suggested branch:

    spotify/webplayer-auth-analysis-provider

Recommended first slice:

1. Add purpose-aware auth interface without changing any caller behavior.
2. Wrap current OAuth behavior as WebAPI/Playback provider.
3. Add developer-only WebPlayer credentials structure.
4. Implement server time + TOTP + token request.
5. Implement one audio-analysis call.
6. Add deterministic unit tests.
7. Add an integration probe disabled by default.
8. Record results in this document.
9. Do **not** yet alter Spotify UI login.
10. Do **not** yet alter respot authentication.

This slice creates maximum evidence with minimum risk.

---

## 27. Success criteria for the overall initiative

The initiative is successful if ViiB can:

- retrieve Spotify's internal tempo/key/mode analysis for a known Spotify track;
- preserve strict provenance between Spotify, ReccoBeats, and ViiB analysis;
- use Spotify analysis to improve benchmarking and DJ tooling;
- keep current Spotify browse/playback/download behavior stable during migration;
- maintain a secure local credential boundary;
- avoid manual-cookie UX in production;
- degrade gracefully when undocumented endpoints change;
- determine empirically whether WebPlayer auth can replace PKCE for playback and/or Web API calls;
- potentially simplify Spotify onboarding in a later release without coupling core ViiB functionality to a brittle private API.

---

## 28. Non-goals

This plan does not authorize:

- bypassing Spotify DRM;
- changing ViiB download behavior;
- removing PKCE immediately;
- treating Spotify analysis as objective ground truth;
- storing Spotify passwords;
- exposing browser session cookies to the renderer;
- making undocumented Spotify services mandatory for local DJ analysis;
- shipping a production feature before applicable Spotify terms and distribution implications are reviewed.

---

## 29. Short version for future maintainers

If you are returning to this months later:

**What we found:** Spotify's public audio-analysis/features APIs are restricted, but Spotify's internal WebPlayer ecosystem still exposes audio analysis through spclient.wg.spotify.com/audio-attributes/v1/audio-analysis/{trackID}.

**What is special about auth:** the internal path uses WebPlayer session-derived tokens from open.spotify.com/api/token and a Spotify TOTP mechanism, not simply ViiB's normal Developer OAuth PKCE token.

**What we should build first:** a backend WebPlayerAuthProvider beside the existing OAuth provider, then use it only for internal audio analysis.

**What we should not do first:** replace the existing PKCE token globally. ViiB currently uses that token for public Web API calls and librespot/respot login, and WebPlayer-token compatibility is not yet proven for all of them.

**Long-term opportunity:** if compatibility testing succeeds, ViiB may eventually eliminate the requirement for users to create a Spotify Developer application.

**Critical rule:** keep local analysis independent and keep provenance for every remote value.
