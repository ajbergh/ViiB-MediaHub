# Spotify Free media compatibility probe

## Result — 2026-10-16

The authorized live session authenticated as Web Player and product state returned **free**. ViiB's current librespot transport then failed at session login with the provider protocol error **PremiumAccountRequired**, before stream creation, audio-format selection or any audio bytes were read. The probe reports this as `media_premium_required`. The initial generic failure was rerun after adding safe stage/error classification; the classified result is recorded in [redacted evidence](evidence/spotify-free-account-probe-2026-10-16.json).

This establishes a provider-side Premium requirement for the current media transport on this account. It does not establish that Spotify's first-party Free Web Player cannot stream, nor universal behavior for every account or market. Changing ViiB's quality setting or removing its local Premium gate would not resolve the observed login rejection. Production media gates are unchanged.

Spotify documents Free Web Player playback as AAC 128 kbit/s and Premium Web Player playback as AAC 256 kbit/s. These differ from ViiB's current Ogg Vorbis transport and its 96/160/320 quality choices. Spotify's third-party Web Playback SDK requires Premium. Music offline downloads are Premium-only; Free accounts can download podcasts. A permitted streaming response would not independently establish music-download entitlement or complete-file support.

- [Spotify audio quality](https://support.spotify.com/ag/article/audio-quality/)
- [Spotify Web Playback SDK requirements](https://developer.spotify.com/documentation/web-playback-sdk/howtos/web-app-player)
- [Spotify offline downloads](https://www.spotify.com/us/premium/features/offline-downloads/)

## Reproduce

From `backend`, with an authorized Free account's `VIIB_SPOTIFY_SP_DC` set locally:

```powershell
go run -tags spotify_research ./cmd/spotify-analysis-probe --enable --free-account-test --track-id 5RIh3Tsq9U7CAvivB2iB7q --renew=false
```

The command accepts a track ID, URI or Spotify track URL. The new flag verifies product state before initializing the media session; a Premium/unknown product cannot produce a successful Free-account result. It defaults to a 4096-byte application read at **96 kbps only**, and allows `--stream-read-bytes 4096..16384`. It rejects combining the Free-account mode with download, analysis, catalog or Web API modes. `--renew=true` additionally tests a fresh media session after token renewal if the initial pass succeeds.

The existing isolated worker has a 75-second watchdog, discards dependency output and cleans its temporary report/cache directory. No music file or audio bytes are retained. The dependency may prefetch internally, so the application read limit is not a network-transfer limit. Credentials are supplied only through the environment and excluded from reports.

Successful short reads report the Vorbis identification header's **nominal bitrate**, not measured average VBR bandwidth. An Ogg marker alone does not prove selected bitrate, sustained playback or complete decoding. A Free-account success requires verified product state and the expected 96 kbps identification header. Session/authentication/key rejections are reported without retrying alternate identities, entitlements or formats.

JSON reports distinguish account verification, media session initialization, stream creation, application bytes read, Ogg verification and nominal bitrate. Failures exit nonzero. `downloadAssessment: not_tested_music_requires_premium` is an explicit untested music-download outcome, not an experimental download failure.

## Implication for ViiB

The existing transport cannot be enabled for this Free account by selecting a lower bitrate. Supporting first-party Free playback would require a separate player integration compatible with Spotify's Free playback flow, including its playback and advertising requirements; this test does not implement or validate such an integration. Opening Spotify's own Web Player remains available. Existing local-file playback and metadata browsing remain usable in ViiB.

## Follow-up: full playback through ViiB's own player

The requested target is full-track Free playback through ViiB's player, without an embedded Spotify player. The temporary embed prototype was removed. No successful direct Free playback has been demonstrated.

ViiB currently assigns media URLs to two HTML audio elements in `components/Player.tsx` and `hooks/useAudioPlayer.ts`. `lib/audio.ts` connects those elements to ViiB's Web Audio processing graph. The Windows desktop renderer is WebView2. These are integration points for a separate transport, but the existing player does not implement Media Source Extensions (MSE), Encrypted Media Extensions (EME), or Spotify's protected Web Player playback session.

The remaining direct-player investigation needs to distinguish three independent questions:

1. Can the actual Wails/WebView2 renderer decode and continuously play AAC in MP4 through MSE? A normal Edge browser result does not establish the desktop app's behavior.
2. Can that renderer obtain the media key-system access and protected playback configuration required by Spotify? A successful EME capability query alone does not establish a usable Spotify license.
3. Does the authorized Free session receive playable media and licenses in this integration, with advertisements and service-reported playback restrictions handled? Only an audible, sustained full-track test establishes playback. Metadata, a manifest, codec support, or a short encrypted response is insufficient.

Neither the actual renderer's protected-media capabilities nor a Spotify Free license exchange has been validated. Spotify's public Web Playback SDK explicitly requires a Premium user's access token; it cannot provide the Free transport simply by using different bitrate settings. This is a limitation of the documented integration, not proof that every possible first-party-compatible implementation is technically impossible.

- [Web Playback SDK token and account requirements](https://developer.spotify.com/documentation/web-playback-sdk/reference)
- [Encrypted Media Extensions overview](https://developer.mozilla.org/en-US/docs/Web/API/Encrypted_Media_Extensions_API)

### Separate official lead: Spotify Soloist

Spotify Soloist is an official headless playback engine. Its README states that setup requires Premium, after which both Free and Premium users can connect. It currently runs on Linux and outputs audio through PipeWire or PulseAudio. Its optional WebSocket API exposes playback commands, queue information, metadata, position, and available actions.

A ViiB adapter could therefore use ViiB's controls to operate a Soloist engine. This is a design possibility, not a tested integration. The documented API does not expose an audio URL or PCM stream for feeding ViiB's HTML audio elements, EQ, analyser, or crossfade graph. Soloist consequently does not satisfy direct playback through the existing Windows audio engine. Linux-helper audio output, including any WSL audio configuration, would need a separate experiment and must not be described as native Windows playback.

An adapter must honor Soloist's `available_actions` rather than assuming that Free playback permits seeking, skipping, or arbitrary track selection. Each user must obtain their own API key and official binary; binaries must not be redistributed, and builds expire after 90 days. The WebSocket API has no built-in authentication or Origin checks, so any experimental helper should bind to loopback and be mediated by ViiB's backend.

- [Spotify Soloist requirements](https://github.com/spotify/soloist/blob/main/README.md)
- [Spotify Soloist overview and distribution requirements](https://developer.spotify.com/documentation/soloist)
- [WebSocket commands, events, and available actions](https://developer.spotify.com/documentation/soloist/reference/websocket-api)
- [Linux downloads and build lifetime](https://developer.spotify.com/documentation/soloist/reference/downloads-and-updates)

No Soloist binary was installed, no API key was generated, and no Soloist Free playback was tested during this investigation. The existing probe's `PremiumAccountRequired` result remains the only live media result. Streaming research does not establish music-download capability.
