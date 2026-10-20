/** Built-in device-scoped preset. Protocol facts, not copied Mixxx script code.
 * Reference: josefveiga/Numark-Mixtrack-Go-mapping-for-Mixxx/Numark-Mixtrack-Go.midi.xml
 */
import type { MidiAction, MidiMapping, MidiValueMode } from './djMidi';
import type { PadMode } from './djPadModes';

export function isMixtrackGo(name: string): boolean {
  return name.toLowerCase().replace(/[^a-z0-9]/g, '').includes('mixtrackgo');
}

export function mixtrackGoMappings(deviceId: string): MidiMapping[] {
  const mappings: MidiMapping[] = [];
  const add = (action: MidiAction, channel: number, note: number, valueMode: MidiValueMode, type: 'cc' | 'noteon' = 'noteon', extra: Partial<MidiMapping> = {}) => {
    mappings.push({ id: `mixtrack-go:${deviceId}:${action}:${extra.padMode ?? 'all'}`, deviceId, action, channel, note, type,
      valueMode, label: `MixTrack Go · Ch${channel + 1} ${type === 'cc' ? 'CC' : 'Note'} ${note}`, builtIn: true, ...extra });
  };
  add('masterVolume', 15, 0x0A, 'absolute', 'cc');
  add('headphoneVolume', 15, 0x0C, 'absolute', 'cc');
  add('crossfader', 15, 0x08, 'absolute', 'cc');
  add('library.browse', 15, 0x00, 'relative', 'cc');
  add('library.open', 15, 0x07, 'toggle');
  for (const [index, deck] of (['A', 'B'] as const).entries()) {
    const prefix = `deck${deck}` as const;
    add(`${prefix}.play`, index, 0x00, 'toggle');
    add(`${prefix}.cue`, index, 0x01, 'momentary');
    add(`${prefix}.sync`, index, 0x02, 'momentary');
    add(`${prefix}.headphoneCue`, index, 0x1B, 'toggle');
    add(`${prefix}.volume`, index, 0x16, 'absolute', 'cc');
    add(`${prefix}.filter`, index, 0x1A, 'absolute', 'cc');
    add(`${prefix}.tempo`, index, 0x09, 'absolute', 'cc', { ccLSB: 0x29 });
    add(`${prefix}.jogWheel`, index, 0x06, 'relative', 'cc');
    add(`${prefix}.jogTouch`, index, 0x50, 'momentary');
    add(`${prefix}.stemAcapella`, index, 0x46, 'toggle');
    add(`${prefix}.stemInstrumental`, index, 0x48, 'toggle');
    add(`${prefix}.load`, 15, 0x02 + index, 'toggle');
    add(`${prefix}.padMode`, 4 + index, 0x00, 'momentary');
    for (const mode of [0, 1, 2, 3] as PadMode[]) {
      for (const pad of [1, 2, 3, 4] as const) {
        add(`${prefix}.performancePad${pad}`, 4 + index, 0x13 + pad, 'momentary', 'noteon', { deck, padMode: mode });
      }
    }
  }
  return mappings;
}
