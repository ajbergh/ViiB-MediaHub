# Spotify BPM diagnostic

## Direct V2 retrieval and rollout

ViiB now requests `AUDIO_ATTRIBUTES_V2` (extension 222) directly by Spotify track URI through `POST https://spclient.wg.spotify.com/extended-metadata/v0/extended-metadata`. A live authenticated request for `5RIh3Tsq9U7CAvivB2iB7q` returned **70.4827 BPM / D# major / 5B**, matching the playlist-native metadata. No playlist membership is needed.

From `backend`, with the session environment variable set:

```powershell
go run -tags spotify_research ./cmd/spotify-track-metadata --v2 5RIh3Tsq9U7CAvivB2iB7q
```

To use the updated running backend's session instead:

```powershell
go run -tags spotify_research ./cmd/spotify-track-metadata --v2 --backend-url http://127.0.0.1:PORT 5RIh3Tsq9U7CAvivB2iB7q
```

Rebuild/restart ViiB before using that route against an older running version. The backend mode honors the reference cache and reports `fromCache`; cookie mode makes a direct upstream request. Without `--v2`, the CLI retains the legacy audio-features comparison mode.

Production preparation and download warmup fetch V2 through the existing account-fenced refresh service. V2 has a separate `audio_attributes_v2` cache/status identity and decoder revision. Fresh V2 BPM/key outrank fresh legacy scalars even if the legacy response is newer. Missing V2 dimensions retain the existing legacy/local fallback behavior; fallback provenance remains visible and does not establish parity with the Web Player. Other provider metadata and detailed artifacts continue to come from their existing resources. Manual overrides retain precedence.

After rebuilding, connect Spotify and run **Prepare library** to revisit all available local/Plex tracks, including already prepared tracks. Existing current local DSP artifacts are reused. Saved recording IDs are used where available; matching still applies to unlinked tracks. No startup bulk rewrite or audio retagging is performed. New downloads use the V2 scalars for tags and retain V2 fields with the final file for offline use. The schema upgrade preserves legacy cache rows. V2 availability for every Spotify recording is not guaranteed; failures use existing bounded retry/cooldown handling.

## Confirmed UI discrepancy — 2026-10-16

For `5RIh3Tsq9U7CAvivB2iB7q` (Bing Crosby, The First Snowfall), fresh `audio_features` and `audio_analysis` returned **137.98 BPM, key 8 major, Camelot 4B**. The authenticated current Web Player `fetchPlaylist` operation for playlist `37i9dQZF1DWZy48MuOV69W` returned **70.4827 BPM, D# major, Camelot 5B** under `itemV2.data.playlistAudioAttributes`, matching the screenshot's 70 / 5B. This establishes different provider metadata values; it does not independently measure musical accuracy.

The production playlist adapter uses an older persisted query and omits this field from its normalized track model. The new production scalar path requests V2 directly instead of relying on that playlist projection.

With `VIIB_SPOTIFY_SP_DC` set as described below, reproduce the native playlist value from `backend`:

```powershell
go run -tags spotify_research ./cmd/spotify-track-metadata --playlist-id 37i9dQZF1DWZy48MuOV69W 5RIh3Tsq9U7CAvivB2iB7q
```

This diagnostic-only mode uses the current desktop Web Player `fetchPlaylist` hash observed in its public bundle on October 16, 2026. It reads up to 1000 playlist entries and outputs only the exact matching track URI/name and its nullable native attribute payload, along with query provenance. It needs cookie mode and cannot be combined with `--backend-url`. It does not write to the database. Playlist content can vary by account; direct track queries do not currently supply these attributes. The standard cookie command below continues to use the production audio-features adapter.

## Running backend session

To reuse a running ViiB instance's Spotify session, pass its loopback URL from the app's startup log. No cookie setup is needed:

```powershell
go run -tags spotify_research ./cmd/spotify-track-metadata --backend-url http://127.0.0.1:34115 '1OwGYy9S2R0lzhoGn7vv6S'
```

Run from `backend`. The port varies between launches. This mode calls the backend's normal scalar refresh route and prints its response, including `fromCache`, `cache.observation`, retrieval time and expiration. It honors the backend's cache and cooldowns; it can populate the reference cache when a provider fetch is needed. It does not export authentication or change effective song BPM values. A cached result is not a new upstream measurement. Only literal loopback IP URLs are accepted, and redirects are rejected.

For a fresh fetch in a separate session, use the cookie mode below.

From the repository's `backend` directory:

```powershell
go run -tags spotify_research ./cmd/spotify-track-metadata 'spotify:track:11dFghVXANMlKmJXsNCbNl'
```

The command also accepts a bare 22-character track ID or an `https://open.spotify.com/track/...` link (including share parameters and `intl-*` links). `-h` prints help. It requires Go as specified by `backend/go.mod`.

Authenticate with the same Spotify account used in ViiB. Set `VIIB_SPOTIFY_SP_DC` to the value of that account's `sp_dc` cookie from the Spotify Web Player. In PowerShell, this prompt avoids placing the cookie in shell history:

```powershell
$spotifyCookie = Read-Host 'Spotify sp_dc cookie' -AsSecureString
$env:VIIB_SPOTIFY_SP_DC = [System.Net.NetworkCredential]::new('', $spotifyCookie).Password
try {
    go run -tags spotify_research ./cmd/spotify-track-metadata 'spotify:track:11dFghVXANMlKmJXsNCbNl'
} finally {
    Remove-Item Env:VIIB_SPOTIFY_SP_DC -ErrorAction SilentlyContinue
    Remove-Variable spotifyCookie
}
```

JSON on stdout contains normalized scalar metadata, including fractional `observation.bpm`, key/mode, duration, provider scores, rejected fields, endpoint, retrieval time, adapter revision and Spotify app version. Unknown fields remain null. Failures go to stderr and return a nonzero exit code. Cookies, tokens and account context are excluded from output. Save a result by appending `> track-metadata.json` to the command.

Without `--v2`, the CLI uses the production `auth.PinnedWebPlayerContract`, Web Player provider, auth manager and `analysis.Client.FetchFeatures` for legacy comparison:

```text
https://spclient.wg.spotify.com/audio-attributes/v1/audio-features/<track-id>?format=json
```

It fetches fresh metadata without opening or changing the ViiB database. `cacheBypassed: true` makes that distinction explicit: core ViiB can reuse a seven-day reference cache or durable download evidence. The CLI does not apply database field selection, manual overrides, automatic recording matching or local audio analysis.

For each discrepancy, retain the CLI JSON, ViiB's BPM and selected provider provenance (recording ID, endpoint, retrieval time and durable-import status), and the Spotify UI BPM plus exact track URI. First verify that the UI and database refer to the same recording. A `spotify` source label alone does not establish which endpoint or historical observation supplied the value.

To investigate endpoint disagreement, the existing broader research probe can fetch detailed analysis separately:

```powershell
go run -tags spotify_research ./cmd/spotify-analysis-probe --enable --track-id 'spotify:track:11dFghVXANMlKmJXsNCbNl'
```

Its default is `audio_analysis`; the new command always requests `audio_features`. Neither result independently verifies a UI-displayed BPM or establishes the cause of a discrepancy.
