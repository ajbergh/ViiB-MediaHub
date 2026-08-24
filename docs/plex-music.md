# Plex Media Server Music Support

ViiB MediaHub can use a Plex Media Server **music/audio library** as a first-class music source alongside local filesystem libraries.

Plex content is synchronized into ViiB's existing local catalog, so Plex-hosted tracks use the normal Songs, Albums, Artists, Search, Queue, playlists, likes, play history, Smart Mixes, AI DJ, statistics, and player interfaces wherever those features operate on the ViiB `Song` catalog.

> Plex support in ViiB is **audio/music only**. Movie, TV, photo, music-video, and general Plex video playback are intentionally unsupported.

## Architecture

Plex is implemented as a remote read-only media source, not as a separate Plex application inside ViiB.

- `songs` remains the canonical ViiB catalog table.
- Plex-specific source and playback identity is stored in additive `plex_sources` and `plex_tracks` tables.
- Plex song IDs are namespaced hashes derived from the PMS `machineIdentifier` and track `ratingKey`, preventing collisions with local IDs and allowing the model to support multiple Plex servers/libraries in the future.
- ViiB never requires filesystem access to the Plex server's media paths.
- Plex media is not copied into local filesystem music folders.
- Browser playback continues to use `/api/audio/{songId}` and artwork continues to use `/api/cover/{songId}`. The Go backend dispatches Plex rows to authenticated proxies while local rows keep the existing filesystem behavior.

This design means normal ViiB album/artist aggregation and indexed search do not need Plex-specific variants.

## Plex API behavior

The integration follows the current official Plex Media Server API documentation.

- PMS identity is validated using the documented `/identity` endpoint.
- Library discovery uses `/media/providers` and follows the returned content/library keys rather than assuming fixed legacy section URLs.
- For a music section ViiB follows the section's returned `track` type pivot (Plex metadata type `10`) rather than blindly constructing an `/all` URL. This deliberately avoids video/clip pivots such as music videos.
- PMS authentication is sent server-side with `X-Plex-Token` and is not appended to browser-visible media URLs.
- New Plex account authentication uses Plex's JWT/PIN device flow with an ED25519 device key.

## Configure Plex

Plex configuration is available in **Settings → Library Health → Library Operations**.

### 1. Find a server on the LAN

Choose **Search Local Network**. ViiB's Go backend performs a bounded Plex GDM UDP discovery request and returns discovered servers.

Discovery is only initiated when requested. ViiB does not continuously broadcast discovery traffic and does not block application startup on GDM.

A discovered server includes the information Plex makes available through GDM, such as its friendly name, address/port, resource identifier, and PMS version.

### 2. Add a server manually

If GDM is disabled or blocked, enter a server address manually.

Examples:

- `192.168.1.20`
- `plex-server.local`
- `192.168.1.20:32400`
- `http://192.168.1.20:32400`
- `https://plex.example.com`

Bare hostnames/IP addresses default to HTTP on Plex's normal port `32400`. A complete `http://` or `https://` URL is used as entered, including the normal scheme port when no explicit port is present. This supports reverse-proxy deployments such as Plex on `https://plex.example.com` port 443.

ViiB validates the endpoint as a real Plex Media Server before saving it. Connection errors distinguish common cases such as DNS failure, connection refusal/unreachability, timeout, TLS certificate failure, non-Plex HTTP endpoints, and authentication requirements.

### 3. Authenticate

Claimed Plex servers normally require authentication. Select **Sign in / Reconnect** and complete the Plex-hosted sign-in flow.

ViiB uses Plex's current JWT/PIN authentication flow:

1. ViiB creates an ED25519 device key locally.
2. The backend requests a strong Plex PIN using the device's public JWK.
3. The user's browser opens the official Plex authorization page.
4. ViiB polls the PIN with a device-signed JWT.
5. After authorization, the backend obtains the Plex account JWT and resolves the server resource/access token when available.

Authentication state that expires or becomes invalid is surfaced as an authentication-required/reconnect state rather than silently breaking playback.

### 4. Select a music library

After connecting/authenticating, ViiB displays only Plex sections that represent music/audio.

Movies, TV shows, photos, clips/music videos, and other video libraries are filtered out and cannot be selected as ViiB sources.

Select the desired music library and choose **Synchronize**.

## Synchronization

A Plex synchronization reads the selected music library and maps available metadata into normal ViiB songs, including:

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
- library identity
- track `ratingKey`
- metadata key
- media/part key used for playback

The initial import and every explicit resync are complete authoritative reads **only when the entire Plex request succeeds**.

On a successful synchronization ViiB:

- adds newly discovered Plex tracks;
- updates changed Plex metadata;
- removes ViiB catalog records for tracks that a successful Plex synchronization confirms are no longer in the selected library.

If Plex is temporarily offline, times out, becomes unreachable, or authentication fails, ViiB does **not** interpret that as an empty library. Existing cached Plex catalog entries are retained and the source is marked unavailable/authentication-required as appropriate.

Changing the selected Plex music library clears only ViiB's cached catalog records from the previously selected Plex library before synchronizing the new selection.

Removing the Plex source removes ViiB's cached source/catalog records and stored credentials. It sends no delete operation to Plex and does not delete or modify media on the Plex server.

## Playback and seeking

Plex songs use the same ViiB player as local songs.

The browser receives the normal ViiB URL:

```text
/api/audio/{songId}
```

For a Plex track, the Go backend:

- resolves the PMS media/part key;
- attaches Plex authentication server-side;
- forwards HTTP `Range` requests;
- preserves `206 Partial Content`, `Content-Range`, `Content-Length`, `Accept-Ranges`, and content type headers;
- streams the response with request cancellation instead of buffering the complete song into memory;
- returns actionable authentication/unavailable errors to the existing player path;
- never places the Plex token in the browser URL.

This supports normal seeking for Plex media that PMS can direct-play to the existing browser/Wails audio pipeline.

### Audio transcoding limitation

Direct play is preferred. Plex's documented universal audio-transcode APIs were reviewed for formats the browser cannot decode directly, but the PMS transcoder is session/timeline oriented rather than a direct replacement for ViiB's current byte-range player contract. Automatic audio-transcode fallback is therefore not enabled in this first implementation rather than adding an unverified seeking path.

No video transcoding or Plex video playback is implemented.

## Artwork

Authenticated PMS artwork is exposed through ViiB's existing backend-controlled cover URL. The browser never receives a PMS artwork URL containing a Plex token.

If a Plex artwork key points to another origin, ViiB deliberately removes the Plex authentication header before following that URL so a PMS token is not disclosed cross-origin.

## Security

Plex credentials are handled as sensitive application settings.

- Plex account/server tokens and the ED25519 private device key are stored in the existing machine-bound AES-256-GCM encrypted settings store.
- `plex_credentials` is not exposed through the generic settings allowlist.
- Plex credential structures are never returned by the Plex configuration APIs.
- Tokens are sent to PMS as backend HTTP headers, not embedded in browser media URLs.
- Tokens are not stored in browser `localStorage` or Zustand state.
- Credential-bearing error text is redacted before logging or API error responses.
- Cross-origin media requests never carry the Plex token.

## Read-only source behavior

ViiB treats Plex as remote read-only media storage.

ViiB does not:

- delete Plex media;
- move or rename Plex media;
- modify PMS library configuration;
- write Plex metadata changes back to PMS;
- invoke destructive Plex operations.

ViiB's existing metadata editing path remains a ViiB database operation. A later successful Plex synchronization may replace fields with authoritative Plex metadata; no metadata edit is silently written back to PMS.

## Troubleshooting discovery

If **Search Local Network** returns no servers:

1. Verify Plex Media Server is running and reachable from the ViiB computer.
2. Verify local network discovery/GDM is enabled where applicable.
3. Check host and network firewalls for Plex/GDM UDP traffic, including UDP port `32414`.
4. Confirm the ViiB and Plex machines are on LANs/VLANs where broadcast discovery is permitted.
5. Try the server's IP address or hostname with manual configuration. Manual configuration works independently of GDM.

Multiple active network interfaces/VPNs can change which broadcasts are routable. ViiB broadcasts on usable private IPv4 interfaces and deduplicates repeated responses, but network policy can still block UDP discovery.

## Troubleshooting playback

If a previously synchronized Plex track is visible but will not play:

- Check the Plex source's connection/authentication status in Settings.
- Reconnect if ViiB reports authentication is required.
- Confirm PMS is reachable at the configured address.
- Try **Resynchronize** after connectivity returns.
- If only a particular codec fails while other Plex tracks play, the format may not be directly supported by the current ViiB/browser audio pipeline. Automatic Plex audio-transcode fallback is a known limitation described above.

An outage does not delete the cached Plex catalog. Playback becomes available again when the server is reachable and authentication is valid.

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

Media itself remains on the existing source-transparent routes:

- `GET /api/audio/{songId}`
- `GET /api/cover/{songId}`

No Plex credential is accepted in or returned from these browser-facing media URLs.
