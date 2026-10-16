# Web Player audio attributes V2

Observed and authenticated against the desktop Web Player contract on 2026-10-16. This is a private protocol and can change independently of the public Spotify API.

- Endpoint: `POST https://spclient.wg.spotify.com/extended-metadata/v0/extended-metadata`.
- JSON request: `entityRequest: [{entityUri: "spotify:track:<id>", query: [{extensionKind: 222, etag: ""}]}]`.
- Bearer authentication from the backend Web Player provider; no session cookie is forwarded to the metadata endpoint.
- Response: protobuf; top-level field 2 repeats extension groups. Group field 2 is extension kind; field 3 repeats entities. Entity field 1 is status header, field 2 URI, field 3 protobuf Any. Header field 1 is status.
- Verified Any type: `type.googleapis.com/spotify.playlistmixing.extensions.audio_attributes.v2.AudioAttributes`.
- Domain field 1: BPM double. Field 2: key message, with field 1 tonic string, field 2 mode enum, field 3 Camelot message. Camelot field 1 is value and field 2 color.
- Provider mode: 1 minor, 2 major, 0 unknown. Normalized ViiB mode: 0 minor, 1 major, nil unknown.

Only BPM/key are supported by this resource. Confidence, timing, duration and perceptual scores are not inferred. Validate exact requested URI, extension kind, Any type, finite positive BPM and tonic/mode/Camelot consistency; reject duplicate payloads and bounded malformed messages. An unconditioned request does not reuse a 304 response. The shared refresh service handles successful cache TTL, per-resource failures, session lifetime, account fencing, coalescing and global rate-limit cooldowns.

Known comparison: `5RIh3Tsq9U7CAvivB2iB7q` returns 70.4827 BPM, D# major, 5B here; legacy audio-features and audio-analysis return 137.98 BPM, G# major, 4B. This demonstrates source disagreement, not an independent measurement of audio accuracy.
