# Experimental DJ controller presets

ViiB now automatically installs input presets for four additional MIDI controllers. They are documentation-derived and **have not been tested on physical hardware**. The MIDI mapping panel identifies the detected preset and its experimental status. Enable MIDI once; supported device names receive defaults without using MIDI Learn. Custom learned mappings still take precedence.

This is a selected subset of the [Mixxx 2.5 hardware manuals](https://manual.mixxx.org/2.5/en/hardware/manuals), also maintained in the [manual source repository](https://github.com/mixxxdj/manual/tree/main/source/hardware/controllers). It does not imply support for every device listed by Mixxx or import Mixxx XML/scripts into ViiB.

## Included presets

| Controller | Implemented input subset | Current limits |
| --- | --- | --- |
| Numark DJ2GO (original) | Play, Cue, Sync, PFL, Load A/B, deck level, master/headphone level, crossfader, pitch faders, jog nudge, Browse dial, Enter to toggle the library | Fixed ±8% pitch range. No shift layers, scratch-mode latch or LEDs. DJ2GO2/Touch are different models and are not matched. |
| Behringer CMD Micro | Transport, load, PFL, levels, crossfader, pitch faders, touch/jog, Browse dial, dedicated 4/8-beat loop toggles | Fixed ±8% pitch range. No pitch-bend buttons, playlist navigation or LEDs. Browse rotation opens the library; use its on-screen close control. |
| Hercules DJControl Compact | Transport, crossfader, jog nudge, physical trim knobs as ViiB deck level, mid/low EQ, hot cues 1–4 per deck, eight sampler slots, loop exit and 8-beat loop toggle | Firmware sends distinct notes for its cue/sampler/loop banks. No software Mode cycling, global Scratch latch, shifted tempo adjustment, rolls, loop halve/double or FX bank. No PFL/load controls added where the documented mapping has none. No LEDs. |
| Pioneer DDJ-400 | Transport, Browse press/dial, loading, PFL, jog touch/nudge, high-resolution pitch/level/crossfader/EQ/filter/headphone controls, hot cues 1–4 per deck, sampler pads 1–4 per side, all eight beat-loop lengths | Fixed ±6% pitch range; no shift tempo-range cycling. Native pad banks use distinct notes. Hot-cue pads 5–8, remaining sampler pads, beat jumps, FX, advanced jog/loop-adjust behavior and LEDs are not implemented. |

Loop buttons retain the requested beat length and toggle off on a second press, including on reviewed grids. Experimental EQ knobs use a centered response: center is 0 dB, with ViiB's −24/+12 dB endpoints. Sampler buttons use ViiB's configured one-shot/gate behavior. These are ViiB action mappings, not a promise to reproduce every Mixxx gesture.

The [DDJ-400 manual](https://manual.mixxx.org/2.5/en/hardware/controllers/pioneer_ddj_400) documents a firmware issue that can produce unsolicited tempo-slider messages. Follow its firmware guidance when validating that controller. Its hardware master/headphone mixing controls are not invented as software MIDI messages here. Audio device/channel selection remains separate from MIDI mapping.

## Protocol references

The implementations use control addresses and value encodings from the official Mixxx `2.5` controller sources, inspected on October 18, 2026. No Mixxx script code is bundled or executed.

| Device | Manual | MIDI address source | Encoding reference |
| --- | --- | --- | --- |
| DJ2GO | [Manual](https://manual.mixxx.org/2.5/en/hardware/controllers/numark_dj2go) | [XML](https://github.com/mixxxdj/mixxx/blob/2.5/res/controllers/Numark%20DJ2Go.midi.xml) | [Script](https://github.com/mixxxdj/mixxx/blob/2.5/res/controllers/Numark-DJ2Go-scripts.js): single-channel, distinct deck notes; 1/127 relative directions |
| CMD Micro | [Manual](https://manual.mixxx.org/2.5/en/hardware/controllers/behringer_cmd_micro) | [XML](https://github.com/mixxxdj/mixxx/blob/2.5/res/controllers/Behringer%20CMD%20Micro.midi.xml) | [Script](https://github.com/mixxxdj/mixxx/blob/2.5/res/controllers/Behringer-CMD-Micro-scripts.js): jog center 64; XML Browse uses spread64 |
| DJControl Compact | [Manual](https://manual.mixxx.org/2.5/en/hardware/controllers/hercules_djcontrol_compact) | [XML](https://github.com/mixxxdj/mixxx/blob/2.5/res/controllers/Hercules%20DJControl%20Compact.midi.xml) | [Script](https://github.com/mixxxdj/mixxx/blob/2.5/res/controllers/Hercules-DJControl-Compact-scripts.js): signed 7-bit jog values |
| DDJ-400 | [Manual](https://manual.mixxx.org/2.5/en/hardware/controllers/pioneer_ddj_400) | [XML](https://github.com/mixxxdj/mixxx/blob/2.5/res/controllers/Pioneer-DDJ-400.midi.xml) | [Script](https://github.com/mixxxdj/mixxx/blob/2.5/res/controllers/Pioneer-DDJ-400-script.js): jog center 64 and paired tempo CCs |

## Recognition and output boundaries

[The registry](../lib/djControllerProfiles.ts) recognizes specific model names after case/punctuation normalization. Similar models such as DDJ-FLX4, DJ2GO2 Touch, and a generic `CMD`/`DJControl` port are deliberately left on MIDI Learn rather than assigned guessed addresses. Additional OS-specific port aliases can be added after hardware confirmation.

All experimental profiles are **input-only**. They do not open output ports, send a handshake, or receive MixTrack Go LED/pad-mode messages. The existing MixTrack Go preset retains its protocol and mapping IDs. Legacy MixTrack mapping migration stays specific to that model. Unknown MIDI devices retain manual mapping; HID-only hardware needs a separate transport implementation.

Implementation: [preset data](../lib/experimentalMidiProfiles.ts), [MIDI service](../lib/djMidi.ts), [action dispatch](../hooks/useDJMidiActions.ts). Generated IDs include the profile, device and control, so disabled defaults remain stable and simultaneous devices retain separate input mappings. ViiB's audio decks are shared across connected controllers; this does not create additional decks.

## Validation and promotion checklist

Automated tests cover model recognition/rejection, stable IDs, unique control addresses, transport for both decks, signed/offset-binary encoders, DDJ-400 paired CCs, user overrides, simultaneous profiles, disabled-default persistence/restore, experimental UI status, and suppression of outbound traffic. Existing MixTrack handshake/LED regressions remain enabled.

Before promoting a profile out of experimental status, record its OS, firmware, input/output port names and raw press/release messages. Confirm fader direction/center/endpoints, jog direction/sensitivity, touch release, cue hold/release, native pad banks, loop toggling, sampler gate releases, hot-plug, and custom overrides. Validate audio routing separately. Output feedback requires a separately documented, tested implementation for that controller.
