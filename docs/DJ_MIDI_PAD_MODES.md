# DJ MIDI performance pad modes

MixTrack Go now has a built-in device preset. After MIDI access is enabled, a recognized connected controller gets its default mappings automatically; no MIDI Learn step is required. The Mode button and four pads are pre-mapped independently for each deck and pad bank.

## Automatic controller preset

The [preset](../lib/mixtrackGoProfile.ts) uses the [MixTrack Go MIDI XML](https://github.com/josefveiga/Numark-Mixtrack-Go-mapping-for-Mixxx/blob/main/Numark-Mixtrack-Go.midi.xml) as a protocol reference. Defaults are generated for the actual connected input ID and are not written over saved user bindings.

| Control | Default behavior |
| --- | --- |
| Play, Cue, Sync, headphone/PFL | Corresponding deck transport and cue controls |
| Level, Main Level, Cue Level, crossfader | Deck volume, master volume, headphone volume, crossfader |
| Filter/Low | Bipolar filter matching the UI filter response; center bypasses the filter |
| Pitch | Paired 14-bit CC input, ±8% tempo range; center is normal speed |
| Jog rim / touch | Position nudge; touching the platter enables scratch movement, release ends scratch |
| Mode / pads | Independent Hot Cue, Loops, Sampler, Stems banks on each deck |
| Acapel / Instru | Toggle vocal-only / instrumental mute presets when prepared stems are playing |
| Browse / Browse push | Dial opens the library, highlights the selection and scrolls it into view; push toggles the drawer open/closed |
| Load 1 / Load 2 | Load the highlighted library track into A/B, matching the on-screen load buttons, including replacement of a playing track |

Fade FX and Shift-layer controls are not included in this preset yet. ViiB has no equivalent crossfader-driven Fade FX mode, and Shift combinations require a separate modifier model. They are not silently assigned unrelated functions. Stems still require a prepared stem package and enabled stem playback; sample pads require loaded samples. Jog sensitivity and pitch direction need a physical controller check.

Other controller input presets and their limitations are listed in [Experimental DJ controller presets](EXPERIMENTAL_DJ_CONTROLLERS.md). MixTrack Go's output handshake and LEDs remain specific to that device.

## Per-mode overrides

Use **Mapping bank** in the MIDI mapping panel to choose Deck A/B plus Hot Cue, Loops, Sampler, or Stems. Then select any action category and Learn the pad for that action. The same MIDI note can be learned as different actions in all four banks. Selecting a configuration bank does not change the hardware's active mode; the panel also shows the actual active modes. Mode-button mappings are always global.

For example, learn Pad 1 as Deck A Hot Cue 2 in `Deck A · Hot Cue`, and as Sampler Pad 3 in `Deck A · Sampler`. Those mappings coexist and only the active bank fires. An explicit bank override takes precedence over generic bindings and built-in defaults. Legacy unscoped custom mappings remain global; older device-specific Hot Cue/Sampler bindings on the known MixTrack pad notes migrate into their corresponding banks so they do not steal other modes.

Remapped pad lights follow known target cue/sample slots, performance-pad slots, Play, PFL, or loop state. Other arbitrary mapped actions show a dim assigned indicator because they have no persistent pad-state contract.

Defaults are identified with a `preset` label. Removing a preset binding disables that default; removing a user override exposes the underlying default again. **Restore preset** removes user overrides and re-enables defaults. Overrides and disabled defaults survive restart. Clearing all mappings also disables the connected device's defaults until restored.

Held pads retain the action that received their press. Changing mode releases scoped momentary actions, then waits for the physical button to be released before reusing it in the new bank. MIDI Learn consumes presses but still delivers pending releases and permits the mapped Mode button to change banks while learning other controls.

[DJPadModeController](../lib/djPadModes.ts) owns ephemeral, independent mode state for each deck, initially `0`. Each Mode button press advances `(mode + 1) % 4`. Repeated positive values while held do not advance again. Releases, Note On velocity zero, and CC zero release the button. Modes reset to Hot Cue when the DJ MIDI action hook is remounted; they are not stored as track metadata.

| Mode | Pads 1–4 |
| --- | --- |
| 0: Hot Cue | Seek to cue slots 1–4; set an empty slot at the current position |
| 1: Auto Loop | Set 1-, 2-, 4-, or 8-beat loops through the existing audio-engine API |
| 2: Sampler | Deck A uses sampler slots 1–4; Deck B uses slots 5–8. Gate samples stop on release. |
| 3: Stems | Toggle mute for vocals, music/melody, bass, and drums; requires an available stem package and active stem playback. |

The cycle matches the four printed hardware indicators: Hot Cue → Loops → Sampler → Stems. Changing mode, disconnecting the input, or leaving the workspace releases held sampler gates. Enable stem playback through the existing Stems controls before using the Stems pads. Selecting the pad bank alone does not start separation or asynchronously change a track's playback source.

## MIDI input addresses

[djMidi.ts](../lib/djMidi.ts) exports Mode note `00`. The preset uses MIDI channels 5/6 for the two decks' mode/pad messages and notes `14`–`17` for Pads 1–4. Custom mappings carry optional `deck` and `padMode` fields. To construct an override manually:

```ts
midi.addMapping({
  id: 'mixtrack-a-sampler-pad-1', deviceId: connectedInputId,
  type: 'noteon', channel: 4, note: 0x14,
  action: 'sampler.pad3', valueMode: 'momentary',
  deck: 'A', padMode: 2,
});
midi.saveMappings();
```

The existing input parser and learn workflow remain the entry points. Performance actions are intercepted in `routeMessage` to preserve both button edges, regardless of a saved mapping's value mode. [useDJMidiActions](../hooks/useDJMidiActions.ts) forwards those edges to `handleButton` and supplies the audio callbacks. The state machine's `switch` is the central place to select pad behavior.

## Mode LED feedback

Mode transitions call `sendPadModeFeedback(inputDeviceId, deck, mode)`. This uses the input/output association established by the MixTrack connection handshake and waits for the output to be ready. Inactive mode indicators are explicitly cleared before the selected indicator is lit.

The [MixTrack Go mapping author's protocol implementation](https://github.com/josefveiga/Numark-Mixtrack-Go-mapping-for-Mixxx/blob/main/Numark-Mixtrack-Go.scripts.js) identifies mode LED notes `01`–`04`, using status `94` for Deck A and `95` for Deck B. Velocity `00` is off and `7F` is bright. ViiB uses these protocol values; it does not incorporate the mapping's implementation. Hardware confirmation in ViiB remains necessary.

Immediately after the identity SysEx is queued on an opened output, ViiB sets both decks' indicators. With default modes this ends with `94 01 7F` and `95 01 7F`, lighting Hot Cue on both decks. No loaded track or learned pad mapping is needed for initialization. The hook supplies a live mode reader, so reconnect restores each deck's active software mode. If the DJ workspace mounts after MIDI is already connected, registering that reader also resynchronizes the indicators. Modes reset to Hot Cue when the workspace remounts.

## Transport and performance-pad feedback

The same controller protocol now drives Play/Pause, Cue, Sync, headphone/PFL, and the four performance pads on both decks:

| Control / pad bank | Feedback |
| --- | --- |
| Play/Pause | Bright while the deck is playing; dim with a loaded paused track; off when empty |
| Cue / Sync | Bright while the mapped button is held; dim with a loaded track on release. Sync is a momentary action, not a latched sync-lock indicator. |
| Headphone/PFL | Bright while headphone cue is enabled, dim otherwise |
| ACAPEL / INSTRU | Steady bright for the deck's vocals-only / complete instrumental mix; off otherwise, in every pad bank |
| Hot Cue pads | Bright for saved cue slots; off for empty slots |
| Loop pads | Bright for the matching enabled 1/2/4/8-beat loop; off otherwise |
| Sampler pads | Off when empty, dim when loaded, bright while playing; return when the sample ends |
| Stems pads | Bright for audible buses, dim for muted/zero-gain/non-soloed buses; off without active stem playback |

In Loops mode, pressing the matching active loop pad again disables that loop. Pressing a different pad sets and enables its 1/2/4/8-beat length; pressing a disabled loop's pad starts it again. Releases and repeated button-down messages do not toggle. Beat loops retain their requested preset length for pad toggling and LED feedback, even when a reviewed grid produces a different duration or the track end clips a loop. Manual boundary edits clear that preset identity; manual loops use original track BPM to estimate a matching pad. Loading a new track clears loop identity.

[Feedback generation](../lib/djMidiFeedback.ts) reads software state. Mouse/touch transport, cue, and sampler changes therefore update the controller too. Feedback is refreshed every 50 ms while either deck has a loaded track, plays, or has a Stems bank selected (stem controls live in the audio engine). Only changed LED values are transmitted. Connection replays the current snapshot, and workspace cleanup clears control/pad lights. Jog wheels and faders do not get invented LED messages.

Dedicated ACAPEL and INSTRU indicators use status `90`/`91` for Deck A/B and notes `46`/`48`, respectively, from the same protocol reference. They follow actual stem audibility, including mute, solo, and gain edits from the software. ACAPEL requires audible vocals and no audible instrumental buses; INSTRU requires inaudible vocals and all three instrumental buses audible. Both stay off for silence, partial/custom mixes, unavailable stems, full/fallback playback, or an unloaded deck. These indicators stay steady during pad beat pulses, and update even while paused outside the Stems bank.

Loading a track with a ready, playable stem package automatically selects Stems audio playback on either deck. The source validates and buffers initial stem frames before completing the load. Missing, incompatible, or failed packages retain full-track playback. Full mode remains available as a manual choice for the loaded track; each new load rechecks stem availability. This audio playback choice is independent of the controller's selected pad bank.

While a deck plays, its bright performance pads pulse bright for the first quarter of each beat and dim between beats. Empty pads stay off, and loaded/inactive dim pads stay dim, preserving each bank's status. Deck A and B follow their own track positions, including seeks and loop wraps. Custom pad mappings retain their target's status but pulse to the physical deck's clock. Pausing or losing beat timing restores steady status feedback.

Timing uses a verified, locked manual grid with its offset when available. Otherwise it estimates phase from the original BPM and track position, including the offset; BPM-only timing may need grid alignment to match the audible beat. Playback position already accounts for tempo, so the pulse does not apply pitch changes twice. Tracks without usable BPM or a reviewed grid keep steady feedback.

### Hardware retest

1. Connect or reconnect: verify Hot Cue is lit on both decks and pads match each deck's saved cues.
2. Start and pause each deck using hardware and the UI; verify Play follows actual playback and turns back down at track end.
3. Hold/release Cue and Sync; verify their lights release. Toggle headphone cue and verify it stays lit only while enabled.
4. Select every pad bank on each deck. Verify loop selection, sample end/gate release, and stem mute/unmute update the correct pads without stale lights from the previous bank.
5. Disconnect/reconnect while playing or while a pad is held; verify state is restored and no held indicator stays stuck. Leave the DJ workspace and confirm control/pad feedback clears.
6. Play different BPM tracks on A and B with saved cues. Confirm each deck's active pads pulse to its own beat. Change pitch, seek, loop, and pause one deck; verify the other deck stays independent and empty pads remain dark. Check audible alignment using a verified grid.
7. With active stem playback, press ACAPEL then INSTRU on each deck in Hot Cue mode; verify only the matching dedicated light is bright. Toggle off, edit stem mutes/solos/gains in software while paused, switch banks, unload, and reconnect; verify the lights always match the current mix.

## Automated validation

Automated tests cover per-deck cycling and wraparound, duplicate-down suppression, Note/CC releases, mode dispatch, gate cleanup, startup output ordering, one active LED per deck, reconnect synchronization, and integration with sampler/stem APIs. Rebuild and physically confirm both Hot Cue indicators at connection, then cycle each deck separately and confirm the old indicator goes dark. Stems pad tests should use a prepared track with stem playback enabled.
