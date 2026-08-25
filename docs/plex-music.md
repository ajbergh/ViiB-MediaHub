# Plex Media Server Music Support

ViiB MediaHub can use a Plex Media Server **music/audio library** as a first-class music source alongside local filesystem libraries.

Plex content synchronizes into ViiB's existing catalog, so Plex-hosted tracks use the normal Songs, Albums, Artists, Search, Queue, playlists, likes, play history, Smart Mixes, AI DJ, statistics, and player interfaces wherever those features operate on the ViiB `Song` catalog.

> Plex support in ViiB is **audio/music only**. Movie, TV, photo, music-video, and general Plex video playback are intentionally unsupported.

## Architecture

Plex is a remote read-only media source, not a separate Plex application inside ViiB.

- `songs` remains the canonical ViiB catalog table.
- Additive `plex_sources` and `plex_tracks` tables store remote source/library/PMS identity and playback keys.
- Plex song IDs are namespaced hashes derived from PMS `machineIdentifier` plus track `ratingKey`, preventing collisions with local IDs and leaving room for multiple Plex servers/libraries later.
- ViiB never needs filesystem access to the Plex server's underlying media paths.
- Plex media is not copied into local filesystem music folders.
- Browser playback remains `/api/audio/{songId}` and artwork remains `/api/cover/{songId}`. The Go backend dispatches Plex rows to authenticated proxies while local rows keep the existing filesystem behavior.
- Existing `songs` triggers continue to drive the normal revision log and search index, so Plex tracks participate in source-transparent library snapshots, deltas, and search.

## Plex API behavior

The integration follows the current Plex Media Server API model and Plex networking guidance.

- PMS identity is validated with `/identity` before a manual address is saved.
- Library discovery uses `/media/providers` and follows provider-returned content/library keys instead of assuming fixed legacy section URLs.
- For a music section, ViiB follows the returned `track` type pivot (Plex metadata type `10`) rather than blindly constructing `/all` URLs. Video/clip pivots such as music videos are not selected.
- PMS authentication is sent server-side with `X-Plex-Token`; credentials are never appended to browser-visible media URLs.
- New Plex account authentication uses Plex's JWT/PIN device flow with an ED25519 device key.

## Configure Plex

Plex configuration is available in **Settings → Library Health → Library Operations**.

### 1. Find a server on the LAN

Choose **Search Local Network**. Discovery runs in the Go backend, not in browser JavaScript.

For Plex Media Server discovery, ViiB sends the GDM `M-SEARCH` request to the Plex server multicast endpoint `239.0.0.250:32414`. It sends from each active, non-loopback IPv4 interface in parallel, so multi-NIC, VPN, link-local, corporate, and unusual lab addressing are not restricted to the OS default route or RFC1918-only interfaces.

Discovery is bounded (normally about 1.5 seconds and never allowed to run indefinitely), is only initiated when requested, does not run continuously, and does not block application startup.

Responses are parsed defensively and deduplicated by Plex resource/machine identifier when available, with host/port as a fallback identity. Malformed UDP responses are ignored rather than aborting the whole discovery operation.

A discovered server can include its friendly name, source address, port, resource identifier, and PMS version.

### 2. Add a server manually

Manual configuration works independently of GDM and is the fallback when local multicast is disabled or filtered.

Examples:

- `192.168.1.20`
- `plex-server.local`
- `192.168.1.20:32400`
- `http://192.168.1.20:32400`
- `https://plex.example.com`

A bare hostname/IP defaults to HTTP on Plex's normal port `32400`. A complete `http://` or `https://` URL is authoritative, including its normal scheme port when no explicit port is supplied. This supports reverse proxies such as `https://plex.example.com` on 443.

ViiB validates the endpoint as a Plex Media Server before persisting it. Connection errors distinguish common cases such as DNS failure, connection refusal/unreachability, timeout, TLS validation failure, non-Plex HTTP endpoints, and authentication requirements.

### 3. Authenticate

Claimed Plex servers normally require authentication. Select **Sign in / Reconnect** and complete the Plex-hosted sign-in flow.

ViiB uses the current Plex JWT/PIN device flow:

1. ViiB creates an ED25519 device key locally.
2. The backend requests a strong Plex PIN using the public JWK.
3. ViiB opens the official Plex authorization page.
4. The backend polls the pending PIN with a device-signed JWT.
5. After authorization, ViiB retains/refreshes the Plex account JWT and resolves the PMS-specific resource/access token when available.

Expired or rejected credentials surface as an authentication-required/reconnect state instead of silently breaking playback.

### 4. Select a music library

After connecting/authenticating, ViiB displays only Plex sections representing music/audio. Movies, TV shows, photos, clips/music videos, and other video libraries are filtered out and cannot be selected as ViiB sources.

Select the desired music library and choose **Synchronize**.

## Synchronization

A Plex synchronization maps available PMS track metadata into normal ViiB songs, including:

- title
- artist
- album
- album artist
- track number
- disc number
- genres
- year
- duration
- artwork identity
- date added
- PMS machine identifier
- Plex library identity
- track `ratingKey`
- metadata key
- media/part key used for playback

The initial import and every explicit resync are treated as authoritative **only after the complete Plex read succeeds**.

After a successful synchronization ViiB:

- adds newly discovered Plex tracks;
- updates changed Plex metadata;
- removes ViiB catalog rows for tracks a successful Plex synchronization confirms are no longer in the selected library.

If Plex is offline, times out, becomes unreachable, or authentication fails, ViiB does **not** interpret that as an empty library. Existing cached Plex catalog entries remain and the source is marked unavailable or authentication-required as appropriate.

Changing the selected Plex music library removes only ViiB's cached rows from the previously selected Plex library. Removing the Plex source removes only ViiB's cached source/catalog data and encrypted Plex credentials. Neither action sends a delete operation to PMS or modifies Plex media.

Local filesystem scanning also treats Plex synthetic source paths as remote records; filesystem diagnostics/reconciliation cannot classify an unavailable Plex source as deleted local media.

## Playback and seeking

Plex songs use the same ViiB player as local songs. The browser receives the normal ViiB URL:

```text
/api/audio/{songId}
```

For a Plex track, the Go backend:

- resolves the stored PMS media/part key;
- attaches Plex authentication server-side;
- forwards HTTP `Range` requests;
- preserves `206 Partial Content`, `Content-Range`, `Content-Length`, `Accept-Ranges`, and content type headers;
- streams the upstream response with request cancellation rather than buffering the complete song in memory;
- returns actionable authentication/unavailable errors through the existing player path;
- never places the Plex token in the browser URL.

This supports normal seeking for Plex media PMS can direct-play to the existing browser/Wails audio pipeline.

### Audio transcoding limitation

Direct play is preferred. Plex's documented universal audio-transcode APIs were reviewed for formats the current browser/Wails playback stack cannot decode directly, but PMS transcoding is session/timeline oriented rather than a drop-in replacement for ViiB's byte-range player contract. Automatic audio-transcode fallback is therefore not enabled in this initial implementation rather than shipping an unverified seeking path.

No video transcoding or Plex video playback is implemented.

## Artwork

Authenticated PMS artwork is exposed through ViiB's existing backend-controlled cover route. The browser never receives a PMS artwork URL containing a Plex token.

If a returned asset key resolves to another origin, ViiB strips `X-Plex-Token` before the backend request so PMS credentials cannot be disclosed cross-origin.

## Security

Plex credentials are sensitive application settings.

- Plex account/server tokens and the ED25519 private device key use ViiB's existing machine-bound AES-256-GCM encrypted settings store.
- `plex_credentials` is not exposed through the generic public settings allowlist.
- Credential structures are never returned by Plex configuration APIs.
- Tokens are sent to PMS in backend HTTP headers, not browser media URLs.
- Tokens are not stored in browser `localStorage` or Zustand state.
- Credential-bearing error strings are redacted before logging or API error responses.
- Cross-origin asset requests never carry a Plex token.

## Read-only source behavior

ViiB does not delete, move, rename, or rewrite Plex media; does not change PMS library configuration; and does not write ViiB metadata edits back to Plex.

ViiB's existing song metadata editing path is intentionally database-only and does not write source tags. A metadata edit to a Plex track is therefore a ViiB-side override. A later authoritative Plex synchronization may replace synchronized fields with current Plex metadata, but no edit is silently sent to PMS.

## Troubleshooting discovery

If **Search Local Network** returns no servers:

1. Verify Plex Media Server is running and reachable from the ViiB computer.
2. Verify **Enable local network discovery (GDM)** is enabled in Plex where applicable.
3. Check host/network firewalls for Plex GDM traffic, especially UDP `32414` for server discovery.
4. Check whether Wi-Fi isolation, VLAN policy, VPN policy, or multicast filtering prevents `239.0.0.250` traffic between ViiB and PMS.
5. Try the server's IP address, hostname, or HTTPS URL using manual configuration. Manual setup does not depend on GDM.

Multiple network interfaces are probed independently. A server can still remain undiscoverable when local network policy blocks multicast; this is expected and is why manual configuration is always available.

## Troubleshooting playback

If a synchronized Plex track is visible but will not play:

- Check the Plex source's connection/authentication status in Settings.
- Reconnect if ViiB reports authentication is required.
- Confirm PMS is reachable at the configured address.
- Try **Resynchronize** after connectivity returns.
- If only a particular codec fails while other Plex tracks play, the format may not be directly supported by the current ViiB/browser audio pipeline; automatic Plex audio-transcode fallback is the known limitation described above.

An outage does not delete the cached Plex catalog. Playback becomes available again when the source is reachable and authentication is valid.

## API endpoints

Plex configuration uses `/api/v2/plex`:

- `POST /discover`
- `POST /connect`
- `GET /config`
- `DELETE /config`
- `POST /auth/start`
- `GET /auth/status`
- `GET /libraries`
- `PUT /library`
- `POST /sync`
- `GET /sync/status`

Media remains on the source-transparent routes:

- `GET /api/audio/{songId}`
- `GET /api/cover/{songId}`

No Plex credential is accepted in or returned from these browser-facing media URLs.
