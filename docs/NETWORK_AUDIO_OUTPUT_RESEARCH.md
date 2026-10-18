# AirPlay and Chromecast output research

Research date: 2026-10-17. Status: proposal; network playback is not implemented or hardware-validated by this research.

## Recommendation

Build a backend-owned network output service with one device picker and separate AirPlay and Google Cast adapters. Target every current native Wails release: Windows amd64, macOS amd64/arm64, and Linux amd64. Keep the design portable to additional architectures. The native targets come from [the release workflow](../.github/workflows/release.yml); its separate browser/server ARM64 builds are not additional native Wails releases.

Use **Chromecast URL playback first**, evaluating `vishen/go-chromecast` behind a ViiB adapter and a dedicated, restricted LAN media server. Use **application PCM streaming for AirPlay**, evaluating Twynn's sender work alongside Music Assistant's `cliairplay`. Twynn is particularly useful because it already addresses Windows, where the latter has no documented build. Neither establishes a complete, production-ready AirPlay solution across ViiB's platforms today.

Make cross-platform AirPlay feasibility the first engineering milestone, before building a finished picker. Prefer a portable audio-only helper extracted from Twynn if its protocol implementation passes the receiver matrix and its licensing can accommodate distribution. Benchmark against `cliairplay` on macOS/Linux. If portability requires disproportionate work, use platform-specific helpers behind the same PCM contract; that costs extra packaging and interoperability maintenance. Do not promise an all-platform release based on a successful Windows pairing or Linux build alone.

The user requirement is AirPlay **and** Chromecast on all Wails OS builds. A staged implementation can prove each transport separately, but that requirement is complete only when both pass on every native release target. Spotify Connect is outside this proposal; it is a different provider-controlled playback system.

## What ViiB already has

| Existing behavior | Consequence for casting |
| --- | --- |
| [Normal playback](../hooks/useAudioPlayer.ts) uses two HTML audio elements for transitions; [the audio engine](../lib/audio.ts) applies EQ, normalization, and master gain through Web Audio. | A post-processing PCM tap can preserve the rendered mix. Sending the original track URL bypasses that graph. |
| [Player state](../slices/playerSlice.ts) owns queue, current track, shuffle/repeat, and playback intent. | Introduce explicit remote-session state and translate controls; avoid two independent queues advancing the same track. |
| [Audio output settings](../components/AudioOutputSettings.tsx) select browser/OS output devices. | `setSinkId` is local output routing, not AirPlay/Cast discovery or transport. |
| [DJ audio](../lib/djAudio.ts) already uses stream destinations for routed output and recording. | Useful reference for a future capture seam; it is not an existing network sender. Headphone cue output must stay separate. |
| [Audio routes](../backend/internal/server/server.go) expose local/Plex tracks through `/api/audio/{songId}`; [Plex proxying](../backend/internal/api/plex.go) handles authentication and ranges. | Reuse source resolution internally. Do not send Plex credentials to receivers. |
| Both [desktop](../backend/cmd/wails/main.go) and [browser/server](../backend/cmd/viib/main.go) listeners bind loopback. | A receiver cannot fetch that host's `127.0.0.1` URL. Add a media-only LAN listener; keep the existing API private. |

There is no existing AirPlay/Cast sender or network-output abstraction in the inspected app. Browser file handles and blob URLs also cannot simply be passed to another device.

## Two playback modes with different behavior

| Mode | Where audio is rendered | Benefits | Limits |
| --- | --- | --- | --- |
| Track URL playback: Chromecast MVP | Receiver fetches and decodes the selected track | Native remote pause/seek/status; less sender processing; straightforward local/Plex integration | ViiB EQ, normalization, dual-player crossfade, visualizer audio analysis, and DJ effects do not follow automatically. Receiver codec support applies. |
| Processed mix: AirPlay target; later Cast extension | ViiB renders audio, then sends/encodes it | Preserves ViiB's processing and transitions | Requires PCM capture, pacing, bounded buffers, clock management, and explicit flush behavior. The app must keep rendering. Cast additionally needs a receiver-compatible live stream. |

These are architectural deductions from ViiB's graph and the receiver playback models. The UI should expose supported controls and explain when processing is unavailable. Do not silently leave EQ controls appearing effective during URL casting.

For the first release, scope normal-player local and Plex tracks, one selected destination, discovery, connection/pairing, play/pause/seek/next, volume, metadata, disconnect, and recovery. Keep Spotify streaming outside initial source qualification until its provider requirements and actual decoding path are checked. Supporting AirPlay/Cast receivers does not itself authorize relaying every provider's audio.

## AirPlay candidates and Twynn findings

Twynn was inspected at commit [`3716f1257d67cd27b35f1c1d28d3fd5ef891a936`](https://github.com/ajbergh/Twynn/tree/3716f1257d67cd27b35f1c1d28d3fd5ef891a936). Source links below pin that revision.

| Candidate | Evidence and fit | Remaining work / decision |
| --- | --- | --- |
| **Twynn audio-only sender** | [Audio CLI](https://github.com/ajbergh/Twynn/blob/3716f1257d67cd27b35f1c1d28d3fd5ef891a936/crates/twynn/src/bin/twynn-audio.rs) accepts receiver destinations and captures Windows loopback or a generated tone. Reusable protocol work includes discovery, pairing, session setup, ALAC/RTP, encryption, and PTP. | Windows-gated executable, no ViiB PCM input contract, and no Chromecast transport. Extract an audio-only helper without the video pipeline, add application PCM input and portable persistence/networking, then build on all targets. |
| **Music Assistant `cliairplay`** | [Upstream design](https://github.com/music-assistant/airplay-cli) fits persistent PCM input, stream flush/restart, metadata, remote events, and AirPlay 1/2 negotiation. Documented build targets cover Linux and macOS. | Strong reference and non-Windows prototype candidate. Windows support is unproven; POSIX command pipes/shared clock facilities need work. Upstream feature claims require our hardware tests. GPL distribution review is required. |
| **pyatv** | [Streaming documentation](https://pyatv.dev/development/stream/) offers discovery/pairing and audio streaming from supported files/buffers. Useful MIT-licensed protocol/receiver probe. | Not the preferred full player transport: current [RAOP implementation](https://github.com/postlund/pyatv/blob/master/pyatv/protocols/raop/__init__.py) implements pause by stopping the stream. Qualify resume/seek semantics rather than assuming a complete remote player. Adds Python packaging. |
| **OwnTone** | [Upstream](https://github.com/owntone/owntone-server) supports AirPlay outputs and [PCM pipe input](https://owntone.github.io/owntone-server/library/). | Full server brings separate library/queue ownership. Documented Linux/BSD/macOS deployment does not supply a Windows-native solution. Better as an optional external gateway. |
| **libraop** | [Upstream](https://github.com/philippe44/libraop) documents Windows/macOS/Linux RAOP and PCM input. | Useful legacy-receiver comparison. Its RAOP protocol “v2” wording is not proof of modern AirPlay 2/HAP compatibility. |

Twynn's [standalone audio documentation](https://github.com/ajbergh/Twynn/blob/3716f1257d67cd27b35f1c1d28d3fd5ef891a936/docs/AIRPLAY_AUDIO.md) explicitly leaves HomePod, groups, and third-party speaker compatibility unvalidated. Metadata/artwork, interactive volume, and authenticated live retransmission are not enabled. Explicit fan-out does not establish a synchronized stereo pair. Those are concrete gaps for a Spotify-like experience, not just UI work.

Use Twynn's deterministic protocol tests as a starting reference, then measure audible output on real receivers. Avoid system loopback as ViiB's normal integration: it can include unrelated apps and notifications, depends on OS capture facilities, and complicates local muting. Feeding only ViiB's processed PCM gives the app control over the source.

Twynn's [license file](https://github.com/ajbergh/Twynn/blob/3716f1257d67cd27b35f1c1d28d3fd5ef891a936/LICENSE.md) declares GPL-3.0-or-later and records unfinished dependency notices. ViiB is [MIT](../LICENSE). Before copying code or bundling a helper, establish redistribution terms, dependency notices, and any separately owned components. Repository ownership can help with permissions for original work; it does not resolve third-party obligations. A separate executable is not an automatic exemption.

On macOS, [system AirPlay output selection](https://support.apple.com/en-gb/guide/mac-help/mchlp2256/mac) can provide an interim manual route; it affects system output. Apple's [native route picker](https://developer.apple.com/documentation/avkit/avroutepickerview) integrates with native playback, including an associated AVPlayer. It does not automatically route ViiB's Web Audio graph or solve Windows/Linux. Keep it as an optional platform enhancement.

## Chromecast method

Google's [Web Sender SDK](https://developers.google.com/cast/docs/web_sender) targets Cast-enabled browsers and requires HTTPS. That is useful for a future supported-browser experience, but it is not evidence of support inside WebView2, WKWebView, or WebKitGTK. [Wails' platform dependencies](https://wails.io/docs/gettingstarted/installation/) include different webview runtimes. Therefore the desktop recommendation is a Go backend adapter, independent of the embedded browser's Cast support.

Evaluate [`vishen/go-chromecast`](https://github.com/vishen/go-chromecast) at commit `17b10a3630ac3fa8c04530a23ad8bd3637098610`. Its [release configuration](https://github.com/vishen/go-chromecast/blob/17b10a3630ac3fa8c04530a23ad8bd3637098610/.goreleaser.yaml) lists Windows, Darwin, and Linux with CGO disabled. Its [application API](https://github.com/vishen/go-chromecast/blob/17b10a3630ac3fa8c04530a23ad8bd3637098610/application/application.go) exposes load, status, pause/resume, seek, volume, and event callbacks. The [license](https://github.com/vishen/go-chromecast/blob/17b10a3630ac3fa8c04530a23ad8bd3637098610/LICENSE) is Apache-2.0. This is an unofficial implementation and a plausible integration candidate, not a Google-supported desktop SDK or a completed compatibility guarantee.

Wrap its control layer; serve ViiB-issued media URLs through our own scoped server rather than adopting its general file server or playlist manager. Qualify lifecycle/concurrency behavior and metadata extensibility before committing to the dependency. Transcoding paths invoke external FFmpeg; they are not a free consequence of a pure-Go build.

Start with the [Default Media Receiver](https://developers.google.com/cast/docs/web_receiver), which does not require a custom receiver application. Check Google's [media support](https://developers.google.com/cast/docs/media) against actual receiver models. Use correct MIME types and implement HEAD/ranges, content length, and required CORS behavior; adaptive streaming requires CORS. Unsupported input needs a deliberate transcoding path or an actionable error, not an assumed universal decoder.

A later processed-mix Cast mode would take the same PCM tap as AirPlay, encode a supported live format, and expose a receiver-fetchable stream. Validate container, buffering, pause/seek semantics, and whether a custom receiver is needed. This is additional work; a URL `LOAD` command alone cannot transmit an AudioContext.

## Proposed service boundary

```text
Wails UI: picker + queue intent + transport controls
                       |
Go network-output service: discovery, session state, capabilities, events
         |                                      |
Cast adapter                               AirPlay adapter/helper
receiver controls                          timed PCM + pairing + transport
         |                                      ^
scoped LAN media URL                       bounded PCM bridge
         |                                      |
local/Plex source resolver                 Web Audio post-processing tap
```

Suggested operations are `Discover`, `Connect`, `Disconnect`, `LoadTrack`, `StartPCM`, `Pause`, `Resume`, `Seek`, `SetVolume`, and session events. Devices declare capabilities such as URL/PCM input, seek, metadata, pairing, protocol version, and group support. Unsupported operations must return explicit outcomes. Keep helper control/PCM IPC local and authenticated; avoid high-frequency JSON/base64 Wails events for bulk samples. Benchmark a binary loopback bridge and AudioWorklet capture on each webview.

ViiB owns queue intent. In URL mode, reconcile receiver status/position and completion into that queue. In PCM mode, ViiB owns rendering and reports audible position separately from samples already rendered ahead. Associate commands/events with a session generation so stale completions cannot skip tracks after reconnect or seek. Pause/seek/next must flush old audio and re-anchor timing; merely pausing the source can leave buffered sound playing.

Use a separate local monitor gain so silencing speakers does not silence the PCM tap. Commit output handoff only after a valid remote start acknowledgement. Bound queues, expose connecting/buffering/error states, and stop stale sessions deterministically. Connection loss should pause and explain the failure; do not unexpectedly resume loudly through local speakers. Account for device volume separately from ViiB gain.

## Network and credentials

- Discover on the selected LAN interface with mDNS, expire stale records, and deduplicate stable identities. Validate advertised addresses/capabilities; discovery text is not trusted input. Provide clear blocked-multicast/VPN/network-isolation diagnostics.
- Cast media delivery needs a LAN-addressable listener active only for the session. Serve allowlisted media/artwork through random, expiring bearer URLs; permit repeated/range requests for their lifetime and revoke them on disconnect. Do not expose arbitrary files, proxy arbitrary URLs, or bind the existing management API to all interfaces. Restrict interface/firewall access where supported; plain LAN HTTP is not confidential.
- Keep Plex/Spotify secrets on the host. Persist AirPlay pairing records through OS-protected credentials on each platform; Twynn's Windows DPAPI store needs a macOS/Linux counterpart. Redact tokens, PINs, receiver identifiers, and pairing material from support logs.
- Qualify required ports per transport/profile. Twynn's current PTP path uses UDP 319/320 plus negotiated media/control ports; account for permission and existing clock-service conflicts without elevating the entire app or globally disabling firewalls.

## Implementation sequence and acceptance

1. **Transport feasibility:** pin dependency revisions; resolve distribution terms; build audio-only candidates on all four native targets. Compare Twynn PCM-input extraction against `cliairplay`; run Cast discovery, load, pause/resume, seek, volume, and status with the Go adapter. Record actual package/dependency sizes and launch requirements. Select the AirPlay helper strategy from results.
2. **Normal player integration:** introduce session/capability state and output handoff, scoped Cast media serving, AirPlay PCM bridge, pairing storage, queue reconciliation, and actionable errors. Prove local/Plex seeking and protected-source handling. Disable unsupported processing controls during URL playback.
3. **Release qualification:** run unit/protocol tests, adapter race/lifecycle tests, endpoint access/range tests, and packaged-app checks on each target. Test native webview PCM pacing, permission prompts, sleep/wake, receiver restart, Wi-Fi interruption, repeated handoffs, and helper crashes.
4. **Later scope:** processed-mix Cast streaming, group playback, and optional DJ master casting. Keep live DJ headphone cueing local. Measure latency before advertising any interactive DJ use; buffered network playback cannot be assumed suitable for scratching or beat monitoring.

| Physical receiver coverage | Required observations on each Wails OS |
| --- | --- |
| Legacy AirPlay/RAOP receiver | Discovery, pairing/password where applicable, audible stereo, pause/resume/seek, volume, clean teardown |
| HomePod or another native AirPlay 2 receiver; Apple TV | Actual protocol selected, pairing persistence, clock/UDP reachability, audible output, firmware recorded |
| Third-party AirPlay 2 speaker | Advertised capability vs working path; no inferred compatibility from Apple TV results |
| Chromecast/Google TV Cast receiver and Cast audio speaker | URL reachability, metadata, codec/ranges, remote status, queue completion, volume, interruption by another sender |
| Groups, if later advertised | Receiver-native topology, channel assignment, measured alignment and drift; independent fan-out is insufficient |

For every supported combination, run five connect/disconnect cycles, a 30-minute drift check, a one-hour stability run, rapid next/seek/pause operations, and a network interruption. Record startup delay, control-to-audible delay, dropouts, memory/CPU, and final teardown. Establish supported latency budgets from measurements rather than copying upstream buffer settings.

This research inspected ViiB and upstream source/documentation. No receiver was discovered, paired, or played to; no cross-platform sender build or end-to-end hardware matrix was completed. Those are the first implementation gates, not claims established by this document.
