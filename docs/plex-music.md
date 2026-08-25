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
- Track hierarchy follows Plex metadata semantics: the parent is the album and `grandparentTitle` is the artist hierarchy value. `originalTitle` is not treated as an artist fallback.
- PMS pagination is driven by the response's actual offset/count/total state. ViiB does not assume that receiving fewer objects than the requested page size means the library is exhausted.
- PMS authentication is sent server-side with `X-Plex-Token`; credentials are never appended to browser-visible media URLs.
- New Plex account authentication uses Plex's JWT/PIN device flow with an ED25519 device key.

## Configure Plex

Plex can be configured directly from the **first-launch wizard** or later from **Settings → Library Health → Library Operations**.

On first launch, Plex and local folders are peer music-source choices. A Plex-only installation does **not** need to add a dummy local folder. The first-launch Plex flow can discover or manually connect to a server, complete Plex sign-in when required, select a music library, and start the initial synchronization. The existing local-folder, Spotify, AI, and Last.fm onboarding remains available from the same first-run source-selection screen.

### 1. Find a server on the LAN

Choose **Search Local Network**. First launch also performs this search automatically when no Plex source is configured. Discovery runs in the Go backend, not in browser JavaScript.

ViiB first performs Plex GDM discovery from each active, non-loopback IPv4 interface. It sends the GDM `M-SEARCH` request in two small waves to:

- Plex's multicast server endpoint `239.0.0.250:32414`; and
- the directed broadcast address for that interface's IPv4 subnet when one is available.

Using both targets improves reliability on Windows/home networks where multicast delivery is inconsistent while still retaining Plex's normal GDM behavior. Multi-NIC, VPN, link-local, corporate, and unusual lab addressing are not restricted to the OS default route or RFC1918-only interfaces for GDM.

If GDM returns no Plex server, ViiB performs a second, bounded fallback by validating `/identity` on Plex's standard TCP port `32400`. The fallback always checks loopback/current-host addresses and may check the local private/link-local subnet. Broad networks are reduced to the `/24` containing the ViiB host, candidate count and concurrency are capped, public networks are never swept, and the overall operation remains time-bounded. This is specifically intended for machines where PMS is reachable normally but UDP multicast/broadcast discovery is filtered by Windows or network policy.

The default first-run/search window is approximately 3.5 seconds. Discovery is user- or onboarding-initiated, does not run continuously, and does not block normal application startup.

Responses are parsed defensively and deduplicated by Plex resource/machine identifier when available, with host/port as a fallback identity. Malformed UDP responses are ignored rather than aborting the whole discovery operation.

A discovered server can include its friendly name, source address, port, resource identifier, and PMS version.

### 2. Add a server manually

Manual configuration is always available and works independently of GDM or subnet probing. It is the recommended fallback for VLANs, filtered networks, nonstandard PMS ports, or reverse proxies.

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
- updates changed Plex metadata, media keys, codec/container information, and artwork identity;
- removes ViiB catalog rows for tracks a successful Plex synchronization confirms are no longer in the selected library.

Plex metadata presence and immediate playability are treated separately. If a complete successful PMS snapshot still reports a track `ratingKey` but temporarily omits a usable `Media/Part`, ViiB does not interpret that track as deleted. An existing cached track and its last known playable media key are retained; a newly discovered track without a usable part is deferred until PMS reports one on a later synchronization.

If Plex is offline, times out, becomes unreachable, or authentication fails, ViiB does **not** interpret that as an empty library. Existing cached Plex catalog entries remain and the source is marked unavailable or authentication-required as appropriate.

Changing the selected Plex music library updates the desired library selection but intentionally retains the previously synchronized Plex cache until a **complete successful synchronization** of the new library is available. That successful snapshot then reconciles old rows. This prevents a library switch followed by an outage from erasing the last usable catalog.

Removing the Plex source removes only ViiB's cached source/catalog data and encrypted Plex credentials. It never sends a delete operation to PMS or modifies Plex media.

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

## Artwork fidelity

Plex-backed **track/album artwork is authoritative** in the ViiB music UI.

- For a Plex track, ViiB prefers the Plex parent/album thumbnail and falls back to the track thumbnail only when PMS does not provide parent artwork.
- The exact PMS artwork key is persisted with the Plex track and fetched through ViiB's backend-controlled `/api/cover/{songId}` proxy.
- Browser-facing Plex cover URLs include a source-derived version query. When PMS changes the artwork key or relevant source update state, the ViiB URL changes so browser/Wails caches do not keep displaying the old cover.
- Spotify or other enrichment artwork may still enhance local filesystem albums, but it cannot replace the authoritative PMS artwork for a Plex-backed album.
- **No Plex artwork is also authoritative.** If PMS supplies no artwork for a Plex-backed track/album, ViiB keeps the normal no-art/gradient state instead of borrowing Spotify artwork or a cover from a same-named local album. The rule is propagated through Albums, Album Detail, Liked Albums, artist-discography album cards, and derived album-cover fallback used by Queue/Now Playing/track surfaces.
- The browser never receives a PMS artwork URL containing a Plex token.

Plex may return either PMS-relative artwork keys or absolute publicly accessible artwork URLs. Authentication is attached only to same-server requests. If an asset key is already cross-origin, ViiB strips `X-Plex-Token`. Redirect handling also strips the token **before** following any redirect away from the configured PMS origin, for both JSON API requests and media/artwork streams.

### Artist portrait scope

The circular artist portrait shown on ViiB's Artists/Artist Detail pages is a separate artist-metadata enrichment feature and currently remains Spotify-backed when enrichment is available. It is intentionally **not** populated with a Plex album cover merely to appear Plex-native.

If Plex artist-portrait parity is added, it should browse Plex's documented artist/type-8 metadata and use each artist item's own `thumb`. Album/track artwork fidelity does not depend on that future enhancement.

## Security

Plex credentials are sensitive application settings.

- Plex account/server tokens and the ED25519 private device key use ViiB's existing machine-bound AES-256-GCM encrypted settings store.
- `plex_credentials` is not exposed through the generic public settings allowlist.
- Credential structures are never returned by Plex configuration APIs.
- Tokens are sent to PMS in backend HTTP headers, not browser media URLs.
- Tokens are not stored in browser `localStorage` or Zustand state.
- Credential-bearing error strings are redacted before logging or API error responses.
- Absolute cross-origin asset requests never carry a Plex token.
- Cross-origin redirects are allowed only after `X-Plex-Token` has been removed from the redirected request.

## Read-only source behavior

ViiB does not delete, move, rename, or rewrite Plex media; does not change PMS library configuration; and does not write ViiB metadata edits back to Plex.

ViiB's existing song metadata editing path is intentionally database-only and does not write source tags. A metadata edit to a Plex track is therefore a ViiB-side override. A later authoritative Plex synchronization may replace synchronized fields with current Plex metadata, but no edit is silently sent to PMS.

## Troubleshooting discovery

If **Search Local Network** returns no servers:

1. Verify Plex Media Server is running and reachable from the ViiB computer.
2. Verify **Enable local network discovery (GDM)** is enabled in Plex where applicable.
3. Check host/network firewalls for Plex GDM traffic, especially UDP `32414` for server discovery.
4. Confirm normal PMS access on TCP `32400`. ViiB's fallback validates that port when safe to do so, but a firewall can still block it.
5. Check whether Wi-Fi isolation, VLAN policy, VPN policy, or multicast/broadcast filtering prevents local discovery traffic between ViiB and PMS.
6. Try the server's IP address, hostname, explicit `:32400`, or HTTPS URL using manual configuration. Manual setup does not depend on discovery traffic.

Multiple network interfaces are probed independently. GDM discovery can still be blocked by network policy, PMS can use a nonstandard port, and routed/VLAN environments may intentionally hide broadcast domains; manual configuration therefore remains a first-class setup method.

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
