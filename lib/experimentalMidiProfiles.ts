/** Protocol addresses from Mixxx 2.5 presets; no Mixxx script implementation is bundled.
 * Sources, supported subsets and exclusions: docs/EXPERIMENTAL_DJ_CONTROLLERS.md.
 */
import type { MidiAction, MidiMapping, MidiValueMode } from './djMidi';

export type ExperimentalControllerId = 'numark-dj2go' | 'behringer-cmd-micro' | 'hercules-djcontrol-compact' | 'pioneer-ddj-400';

export function experimentalMidiMappings(id: ExperimentalControllerId, label: string, deviceId: string): MidiMapping[] {
  const mappings: MidiMapping[] = [];
  const add = (action: MidiAction, channel: number, note: number, valueMode: MidiValueMode = 'toggle',
    type: 'cc' | 'noteon' = 'noteon', extra: Partial<MidiMapping> = {}) => {
    mappings.push({ id: `${id}:${deviceId}:${type}:${channel}:${note}:${action}`, deviceId, type, channel, note,
      action, valueMode, builtIn: true, label: `${label} · experimental`, ...extra });
  };
  const cc = (action: MidiAction, channel: number, note: number, extra: Partial<MidiMapping> = {}) => add(action, channel, note, 'absolute', 'cc', extra);
  const relative = (action: MidiAction, channel: number, note: number, offset = false) => add(action, channel, note, 'relative', 'cc',
    offset ? { relativeEncoding: 'offset-binary' } : {});

  if (id === 'numark-dj2go') {
    cc('crossfader', 0, 0x0A); cc('masterVolume', 0, 0x17); cc('headphoneVolume', 0, 0x0B);
    relative('library.browse', 0, 0x1A); add('library.open', 0, 0x5A);
    for (const [index, deck] of (['A', 'B'] as const).entries()) {
      const action = `deck${deck}` as const;
      add(`${action}.play`, 0, [0x3B, 0x42][index]);
      add(`${action}.cue`, 0, [0x33, 0x3C][index], 'momentary');
      add(`${action}.sync`, 0, [0x40, 0x47][index], 'momentary');
      add(`${action}.headphoneCue`, 0, [0x65, 0x66][index]);
      add(`${action}.load`, 0, [0x4B, 0x34][index]);
      cc(`${action}.volume`, 0, 0x08 + index);
      cc(`${action}.tempo`, 0, 0x0D + index, { invert: true, tempoRange: 0.08 });
      relative(`${action}.jogWheel`, 0, [0x19, 0x18][index]);
    }
  } else if (id === 'behringer-cmd-micro') {
    cc('crossfader', 0, 0x40); cc('masterVolume', 0, 0x30); cc('headphoneVolume', 0, 0x31);
    relative('library.browse', 0, 0x32, true);
    for (const [index, deck] of (['A', 'B'] as const).entries()) {
      const action = `deck${deck}` as const;
      const base = index * 0x10;
      add(`${action}.play`, 0, base + 0x17);
      add(`${action}.cue`, 0, base + 0x18, 'momentary');
      add(`${action}.sync`, 0, base + 0x16, 'momentary');
      add(`${action}.headphoneCue`, 0, base + 0x19);
      add(`${action}.load`, 0, [0x14, 0x25][index]);
      add(`${action}.jogTouch`, 0, index + 1, 'momentary');
      relative(`${action}.jogWheel`, 0, base + 0x11, true);
      cc(`${action}.volume`, 0, base + 0x12);
      cc(`${action}.tempo`, 0, base + 0x10, { invert: true, tempoRange: 0.08 });
      add(`${action}.loopToggle`, 0, base + 0x12, 'toggle', 'noteon', { loopBeats: 4 });
      add(`${action}.loopToggle`, 0, [0x13, 0x24][index], 'toggle', 'noteon', { loopBeats: 8 });
    }
  } else if (id === 'hercules-djcontrol-compact') {
    cc('crossfader', 0, 0x36);
    for (const [index, deck] of (['A', 'B'] as const).entries()) {
      const action = `deck${deck}` as const;
      const base = index * 0x30;
      add(`${action}.play`, 0, base + 0x21);
      add(`${action}.cue`, 0, base + 0x22, 'momentary');
      add(`${action}.sync`, 0, base + 0x23, 'momentary');
      relative(`${action}.jogWheel`, 0, 0x30 + index);
      cc(`${action}.volume`, 0, [0x39, 0x3D][index]); // Physical trim controls ViiB deck level.
      cc(`${action}.eqMid`, 0, [0x3B, 0x3F][index]);
      cc(`${action}.eqLow`, 0, [0x3C, 0x40][index]);
      add(`${action}.loopToggle`, 0, base + 0x19);
      add(`${action}.loopToggle`, 0, base + 0x1A, 'toggle', 'noteon', { loopBeats: 8 });
      for (const pad of [1, 2, 3, 4] as const) {
        add(`${action}.hotCue${pad}`, 0, base + pad);
        add(`sampler.pad${index * 4 + pad}` as MidiAction, 0, base + 0x10 + pad, 'momentary');
      }
    }
  } else {
    relative('library.browse', 6, 0x40); add('library.open', 6, 0x41);
    cc('crossfader', 6, 0x1F, { ccLSB: 0x3F });
    cc('headphoneVolume', 6, 0x0D, { ccLSB: 0x2D });
    for (const [index, deck] of (['A', 'B'] as const).entries()) {
      const action = `deck${deck}` as const;
      add(`${action}.play`, index, 0x0B);
      add(`${action}.cue`, index, 0x0C, 'momentary');
      add(`${action}.sync`, index, 0x58, 'momentary');
      add(`${action}.headphoneCue`, index, 0x54);
      add(`${action}.load`, 6, 0x46 + index);
      add(`${action}.jogTouch`, index, 0x36, 'momentary');
      for (const note of [0x21, 0x22, 0x23]) relative(`${action}.jogWheel`, index, note, true);
      cc(`${action}.volume`, index, 0x13, { ccLSB: 0x33 });
      cc(`${action}.tempo`, index, 0, { ccLSB: 0x20, invert: true, tempoRange: 0.06 });
      cc(`${action}.eqHigh`, index, 0x07, { ccLSB: 0x27 });
      cc(`${action}.eqMid`, index, 0x0B, { ccLSB: 0x2B });
      cc(`${action}.eqLow`, index, 0x0F, { ccLSB: 0x2F });
      cc(`${action}.filter`, 6, 0x17 + index, { ccLSB: 0x37 + index });
      add(`${action}.loopIn`, index, 0x10); add(`${action}.loopOut`, index, 0x11);
      add(`${action}.loopToggle`, index, 0x4D);
      const padChannel = 7 + index * 2;
      for (const pad of [1, 2, 3, 4] as const) {
        add(`${action}.hotCue${pad}`, padChannel, pad - 1);
        add(`sampler.pad${index * 4 + pad}` as MidiAction, padChannel, 0x2F + pad, 'momentary');
      }
      [0.25, 0.5, 1, 2, 4, 8, 16, 32].forEach((beats, pad) => add(`${action}.loopToggle`, padChannel, 0x60 + pad,
        'toggle', 'noteon', { loopBeats: beats }));
    }
  }
  return mappings;
}
