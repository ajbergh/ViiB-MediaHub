# Spotify waveform research — 2026-10-04

The Web Player waveform column has a separate source from scalar features and detailed audio analysis. Authenticated live reads using the existing WebPlayer provider succeeded for two recordings. Production waveform integration is not implemented.

## Source and request

Inspected Spotify's published [desktop bundle](https://open.spotifycdn.com/cdn/build/web-player/web-player.a6d2a638.js). Module 53170 requests `THREEBAND_WAVEFORMS` for the waveform column. Module 92826 assigns extension kind **237**; module 64070 defines its protobuf decoder. The transport builds this request:

```text
POST https://spclient.wg.spotify.com/extended-metadata/v0/extended-metadata
Authorization: Bearer <existing WebPlayer token>
Content-Type: application/json
Accept: application/protobuf
App-Platform: WebPlayer
Spotify-App-Version: <pinned application contract>
```

```json
{"entityRequest":[{"entityUri":"spotify:track:5XVjNRubJUW0iPhhSWpLCj","query":[{"extensionKind":237,"etag":""}]}]}
```

The response contains a protobuf Any with type URL `type.googleapis.com/spotify.playlistmixing.extensions.mixthreebandwaveforms.ThreeBandWaveforms`. Fields are `sampleRate` (1), `sampleWindowSizeMs` (2), and packed int32 arrays `lows` (3), `mids` (4), `highs` (5). The player requires a positive window and equal, nonempty band arrays. These are frequency-band overview samples, not raw PCM.

## Live evidence

The temporary probe opened the installed library read-only, decrypted the existing saved session in memory using the application's executable-bound encryption context, and reused `auth.NewWebPlayerProvider` with `PinnedWebPlayerContract`. Credentials, bearer tokens, request headers and raw response bodies were not printed or saved. No library writes or playback actions occurred. The temporary probe was removed after the experiment.

| Recording | Features | Waveform HTTP / entity status | Samples per band | Window / rate | Detailed analysis |
| --- | --- | --- | --- | --- | --- |
| Better Off Alone — `5XVjNRubJUW0iPhhSWpLCj` | 200; 136.955 BPM, Ab minor, -6.149 dB, time signature 4, 214.883 s | 200 / 200 | 10,745 | 20 ms / 44,100 Hz | 200 |
| Previously tested recording — `5r9W9MJLvHk83fcZSPQ8SE` | 200; 108.022 BPM, F# major, -13.212 dB, time signature 4, 382.4 s | 200 / 200 | 19,120 | 20 ms / 44,100 Hz | 404 |

Better Off Alone detailed analysis contained 121 bars, 488 beats, 976 tatums, 10 sections and 880 segments. Segment fields included start/duration/confidence, loudness start/max/end and peak time, pitches and timbre. The existing adapter validates parts of this payload but retains only normalized scalars; it does not retain arrays or segment loudness envelopes. Earlier detailed-analysis 404 evidence remains valid for the previously tested recording and does not establish universal endpoint unavailability.

## Integration implications

Existing cookie-to-token authentication retrieves this waveform independently of detailed-analysis availability. A production adapter needs bounded protobuf decoding, extension/entity status handling, sample validation, separate cache/provenance, cancellation and rate-limit handling. Displaying the three-band overview requires a renderer and a distinction from the existing amplitude-only local waveform cache. Local fallback remains necessary; two successful recordings do not establish account/market/catalog-wide availability. Waveform retrieval does not replace local energy, loudness or cue analysis.
