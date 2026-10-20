# Single-device four-channel DJ audio

The live DJ workspace now offers **Audio Setup → Output routing → Single device · 4 channels (MixTrack Go)**. Select the MixTrack Go as the **Shared Output Device** and press **Apply**. Main uses channels 1–2 and Headphones uses channels 3–4 (zero-based Web Audio indices 0–3). A controller exposing one USB audio device is expected; its headphone channels are not another item in the device list. The routing mode/device selection is saved after a successful Apply.

The live `DJAudioEngine` uses `connectFourChannelOutput` in its existing context and shares that context/master bus with sampler playback. It bypasses the separate headphone AudioContext/MediaStream path in four-channel mode. Applying an unsupported device restores the previous route; initial unsupported configurations report an error and stay disconnected. Cue Mix directly controls the headphone blend in four-channel mode, and MST toggles between cue-only and master-only. The existing separate-device mode keeps its master-cue gate. No headphone gain boost is applied. Existing live deck cue tap placement is unchanged by this routing integration.

`DJFourChannelAudio` also remains available as a self-contained Web Audio manager, with the React lifecycle wrapper `useFourChannelDJAudio`. Its graph follows the supplied MixTrack Go channel layout:

| Source bus | Physical destination |
| --- | --- |
| Master left/right | Output channels 0/1 (Main) |
| Headphone left/right | Output channels 2/3 (Headphones) |

Each of `inputs.deckA`, `inputs.deckB`, and `inputs.sampler` feeds an independent source fader into the master bus. A parallel cue send taps each input **before its fader**. Cue toggles only affect headphones. The Cue Mix slider blends the sum of enabled cues with the post-volume master using an equal-power curve; Cue Volume changes the combined headphone level. Defaults are cue-only, all cue sends off, and unity source/master/headphone volumes. Multiple summed sources need appropriate level headroom to avoid clipping; equal-power blending correlated cue/master signals can increase the headphone level at intermediate positions.

```text
deckA   ─┬─ source fader ─┐
deckB   ─┼─ source fader ─┼─ master volume ─┬─ splitter L/R ── merger inputs 0/1
sampler ─┼─ source fader ─┘                └─ master blend ─┐
         └─ independent pre-fader cues ── cue blend ───────┼─ cue volume
                                                         └─ splitter L/R ── merger inputs 2/3
                                                                          └─ destination (4 channels)
```

The implementation uses `channelCount = 4`, `channelCountMode = 'explicit'`, and `channelInterpretation = 'discrete'` on the destination. `explicit` is not a valid interpretation value. See [channelCountMode](https://developer.mozilla.org/en-US/docs/Web/API/AudioNode/channelCountMode), [maxChannelCount](https://developer.mozilla.org/en-US/docs/Web/API/AudioDestinationNode/maxChannelCount), and [ChannelMergerNode](https://developer.mozilla.org/en-US/docs/Web/API/ChannelMergerNode).

## React binding

Start audio from a user gesture. Pass an `audiooutput` device ID from `navigator.mediaDevices.enumerateDevices()` to `start({ deviceId })`, or omit it to use the OS default. The example delegates track/sampler playback to `onReady`; those sources must be created in the returned engine's context and connected to its inputs.

```tsx
import { useEffect, useState } from 'react';
import { useFourChannelDJAudio } from '../hooks/useFourChannelDJAudio';
import type { DJAudioSource, DJFourChannelAudio } from '../lib/djFourChannelAudio';

export function HeadphoneControls({ deviceId, onReady }: {
  deviceId?: string;
  onReady: (engine: DJFourChannelAudio) => void;
}) {
  const { engine, start, stop, starting, error } = useFourChannelDJAudio();
  const [cue, setCue] = useState({ deckA: false, deckB: false, sampler: false });
  const [mix, setMix] = useState(0);
  const [volume, setVolume] = useState(1);

  useEffect(() => {
    if (!engine) return;
    for (const source of ['deckA', 'deckB', 'sampler'] as const) {
      engine.setCue(source, cue[source]);
    }
    engine.setCueMix(mix);
    engine.setCueVolume(volume);
  }, [engine, cue, mix, volume]);

  return <div>
    <button disabled={starting || !!engine} onClick={async () => {
      const ready = await start({ deviceId });
      if (ready) onReady(ready);
    }}>Enable audio</button>
    <button onClick={stop}>Stop audio</button>
    {(['deckA', 'deckB', 'sampler'] as DJAudioSource[]).map(source =>
      <label key={source}>
        <input type="checkbox" checked={cue[source]} onChange={event => {
          const checked = event.currentTarget.checked;
          setCue(previous => ({ ...previous, [source]: checked }));
        }} />{source} Cue
      </label>
    )}
    <label>Cue Mix (0: Cue, 1: Master)
      <input type="range" min={0} max={1} step={0.01} value={mix}
        onChange={event => setMix(Number(event.currentTarget.value))} />
    </label>
    <label>Cue Volume
      <input type="range" min={0} max={1} step={0.01} value={volume}
        onChange={event => setVolume(Number(event.currentTarget.value))} />
    </label>
    {error && <p role="alert">{error}</p>}
  </div>;
}
```

For example, decoded buffers can be played through a chosen source input:

```ts
function playBuffer(engine: DJFourChannelAudio, source: DJAudioSource, buffer: AudioBuffer) {
  const player = engine.context.createBufferSource();
  player.buffer = buffer;
  player.connect(engine.inputs[source]);
  player.onended = () => player.disconnect();
  player.start();
  return player; // caller owns transport, scheduling, looping and stopping
}

// Ordinary faders do not affect PFL:
engine.setSourceVolume('deckA', 0);
engine.setCue('deckA', true);
engine.setMasterVolume(0.8);
```

## Integration boundary and device requirements

The standalone manager/hook is an alternative to the live engine. Do not instantiate it alongside `DJAudioEngine` for the same sources. The workspace uses the channel-routing helper directly, preserving its existing EQ/FX, transport and master recording graph. AudioNodes cannot connect across AudioContexts. When using the standalone manager in another component, create all sources in `engine.context` and connect their pre-fader outputs to `engine.inputs`. If retaining EQ/FX, connect their stereo outputs to these inputs and apply channel faders through `setSourceVolume`, downstream of the cue taps. Crossfader gains also belong downstream of the cue taps.

Test Mix preview currently requires separate output devices; the four-channel route reports its shared device for both outputs so that feature remains disabled rather than relying on the old separate-device safety checks.

`start()` deduplicates concurrent starts and cleans up a pending initialization after Stop or unmount. An already started engine is reused; call `stop()` before `start({ deviceId: differentDevice })`. The manager owns its context; stop/disconnect caller-owned sources when appropriate, and dispose the manager when finished. There is no HTMLAudioElement/MediaStream fallback for multichannel output.

The selected device must expose at least four output channels to the browser/WebView; physical USB channel count alone does not establish runtime support. Device selection uses `AudioContext.setSinkId` when a specific ID is supplied. If unavailable, choose the controller as the OS default and omit the ID. A runtime reporting only two channels is rejected before the graph is connected, avoiding an accidental headphone downmix into Main. Four-channel destination assignment errors also propagate. The manager does not automatically identify an audio device from its MIDI port.

Verify channel 0/1 and 2/3 separation on each target OS/Wails build. Unit tests cover the signal graph and lifecycle using models/mocks. `scripts/dj-four-channel-render-check.mjs` also generates a Playwright CLI probe that renders the production routing helper in a real Chromium OfflineAudioContext at 44.1 and 48 kHz (master, headphone, and mixed stereo cases). Offline rendering does not validate the controller driver or physical headphone jack.

## Diagnosing a silent headphone jack

After applying four-channel routing, Audio Setup offers **Test physical outputs** buttons for Main left/right (1/2) and Headphone left/right (3/4). Pause tracks and start with a low physical volume. Each button plays a short, faded 440 Hz tone at −20 dBFS directly into the selected merger channel, bypassing track playback, cue selection, Cue Mix, and software volume. The controller's physical output controls still apply. Tests stop when the dialog closes or the audio route changes.

- If headphone tones work, the extra output channels reach the jack. Inspect the CUE flags, mix and volume under **Routing details**; cue-only with neither deck cued intentionally produces silence.
- If Main tones work but both headphone tones are silent, the next check is the runtime/device/physical output path, rather than increasing cue gain.
- If a headphone tone plays from Main instead, report which test button plays from which jack. Do not assume the device's advertised channel count proves its physical channel mapping.

Routing details show channel count, sample rate, context state, and cue controls. The application does not automatically change gain or force a new sample rate in response to silence.
