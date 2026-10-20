/** MixTrack Go LED protocol, separated from MIDI I/O and React subscriptions. */
import type { DeckId, DeckState, SamplerPad } from '../slices/djMixerSlice';
import type { PadMode } from './djPadModes';
import { PAD_LOOP_BEATS } from './djPadModes';
import type { StemDeckState, StemDeckStatus } from './stemDeckSource';
import type { MidiAction } from './djMidi';
import { canSyncBeatGrid } from './beatGridConfidence';

export type MidiLEDMessage = readonly [status: number, note: number, value: number];
export const MIDI_LED = { off: 0x00, dim: 0x01, bright: 0x7F } as const;

type DeckBeatClock = Pick<DeckState, 'track' | 'isPlaying' | 'position' | 'originalBpm' | 'effectiveBpm'
  | 'tempo' | 'beatGrid' | 'beatGridOffset' | 'beatGridSource' | 'beatGridLocked'>;

/** null means no usable playback clock; phase always follows source-track time. */
export function deckBeatPulse(deck: DeckBeatClock): boolean | null {
  if (!deck.track || !deck.isPlaying || !Number.isFinite(deck.position) || deck.position < 0) return null;
  const offset = Number.isFinite(deck.beatGridOffset) ? deck.beatGridOffset : 0;
  let beats: number;
  if (canSyncBeatGrid(deck)) {
    const grid = deck.beatGrid!;
    const time = deck.position - offset;
    let lo = 0, hi = grid.length - 1;
    while (lo < hi) {
      const mid = Math.ceil((lo + hi) / 2);
      if (grid[mid] <= time) lo = mid; else hi = mid - 1;
    }
    const index = Math.min(lo, grid.length - 2);
    beats = (time - grid[index]) / (grid[index + 1] - grid[index]);
  } else {
    // Playback position already advances at the selected tempo. Applying the
    // effective BPM directly would count the pitch adjustment a second time.
    const bpm = deck.originalBpm && Number.isFinite(deck.originalBpm) && deck.originalBpm > 0
      ? deck.originalBpm : (deck.tempo > 0 ? (deck.effectiveBpm ?? 0) / deck.tempo : 0);
    if (!Number.isFinite(bpm) || bpm <= 0) return null;
    beats = (deck.position - offset) * bpm / 60;
  }
  const phase = ((beats % 1) + 1) % 1;
  return phase < 0.25;
}

/** Apply after custom mappings: physical pads follow their own deck's clock. */
export function pulseDeckMidiFeedback(deck: DeckId, clock: DeckBeatClock, messages: MidiLEDMessage[]): MidiLEDMessage[] {
  if (deckBeatPulse(clock) !== false) return messages;
  const status = deck === 'A' ? 0x94 : 0x95;
  return messages.map(message => message[0] === status && message[1] >= 0x14 && message[1] <= 0x17
    && message[2] === MIDI_LED.bright ? [message[0], message[1], MIDI_LED.dim] : message);
}

export interface DeckFeedback {
  deck: Pick<DeckState, 'track' | 'isPlaying' | 'cueEnabled' | 'hotCues' | 'loop' | 'effectiveBpm' | 'originalBpm'>;
  mode: PadMode;
  samples: Pick<SamplerPad, 'url' | 'isPlaying'>[];
  cuePressed: boolean;
  syncPressed: boolean;
  stems?: { status: StemDeckStatus; state: StemDeckState };
}

export function buildDeckMidiFeedback(deck: DeckId, feedback: DeckFeedback): MidiLEDMessage[] {
  const { deck: state, mode, samples, cuePressed, syncPressed, stems } = feedback;
  const loaded = Boolean(state.track);
  const idle = loaded ? MIDI_LED.dim : MIDI_LED.off;
  const transport = deck === 'A' ? 0x90 : 0x91;
  const padStatus = deck === 'A' ? 0x94 : 0x95;
  const messages: MidiLEDMessage[] = [
    [transport, 0x00, state.isPlaying ? MIDI_LED.bright : idle],
    [transport, 0x01, cuePressed ? MIDI_LED.bright : idle],
    [transport, 0x02, syncPressed ? MIDI_LED.bright : idle],
    // The controller's PFL LED uses a Note Off status with a brightness value.
    [deck === 'A' ? 0x80 : 0x81, 0x1B, state.cueEnabled ? MIDI_LED.bright : MIDI_LED.dim],
  ];
  const bpm = state.originalBpm || 120;
  const loopBeats = state.loop.enabled ? state.loop.presetBeats ?? (state.loop.end - state.loop.start) * bpm / 60 : 0;
  const buses = ['vocals', 'music', 'bass', 'drums'] as const;
  const soloed = stems && buses.some(bus => stems.state[bus].solo);
  const audible = (bus: typeof buses[number]) => {
    const state = stems?.state[bus];
    return Boolean(state && !state.muted && state.gain > 0 && (!soloed || state.solo));
  };
  for (let index = 0; index < 4; index++) {
    let value: number = MIDI_LED.off;
    switch (mode) {
      case 0:
        if (loaded && state.hotCues.some(cue => cue.slot === index + 1)) value = MIDI_LED.bright;
        break;
      case 1:
        if (loaded && loopBeats > 0 && Math.abs(loopBeats - PAD_LOOP_BEATS[index]) < 0.15) value = MIDI_LED.bright;
        break;
      case 2:
        value = samples[index]?.isPlaying ? MIDI_LED.bright : samples[index]?.url ? MIDI_LED.dim : MIDI_LED.off;
        break;
      case 3:
        if (loaded && stems?.status.available && stems.status.mode === 'stems') {
          value = audible(buses[index]) ? MIDI_LED.bright : MIDI_LED.dim;
        }
        break;
    }
    messages.push([padStatus, 0x14 + index, value]);
  }
  const activeStems = loaded && stems?.status.available && stems.status.mode === 'stems';
  const instruments = ['music', 'bass', 'drums'] as const;
  // Read the actual mix, including UI mute/solo/gain edits, rather than the last
  // MIDI preset pressed. Protocol: MixTrack Go led.acapel / led.instru.
  messages.push(
    [transport, 0x46, activeStems && audible('vocals') && instruments.every(bus => !audible(bus)) ? MIDI_LED.bright : MIDI_LED.off],
    [transport, 0x48, activeStems && !audible('vocals') && instruments.every(bus => audible(bus)) ? MIDI_LED.bright : MIDI_LED.off],
  );
  return messages;
}

/** Feedback for an overridden physical pad follows its mapped target slot. */
export function mappedPadLEDValue(action: MidiAction, decks: Record<DeckId, DeckFeedback>): number {
  const sampler = /^sampler\.pad([1-8])$/.exec(action);
  if (sampler) {
    const index = Number(sampler[1]) - 1;
    const sample = (index < 4 ? decks.A : decks.B).samples[index % 4];
    return sample?.isPlaying ? MIDI_LED.bright : sample?.url ? MIDI_LED.dim : MIDI_LED.off;
  }
  const match = /^deck([AB])\.(.+)$/.exec(action);
  if (!match) return MIDI_LED.dim;
  const deck = match[1] as DeckId;
  const target = decks[deck];
  const cue = /^hotCue([1-4])$/.exec(match[2]);
  if (cue) return target.deck.track && target.deck.hotCues.some(item => item.slot === Number(cue[1])) ? MIDI_LED.bright : MIDI_LED.off;
  const pad = /^performancePad([1-4])$/.exec(match[2]);
  if (pad) return buildDeckMidiFeedback(deck, target)[3 + Number(pad[1])][2];
  if (match[2] === 'play') return target.deck.isPlaying ? MIDI_LED.bright : MIDI_LED.dim;
  if (match[2] === 'headphoneCue') return target.deck.cueEnabled ? MIDI_LED.bright : MIDI_LED.dim;
  if (match[2] === 'loopToggle') return target.deck.loop.enabled ? MIDI_LED.bright : MIDI_LED.dim;
  return MIDI_LED.dim; // Other custom actions have no persistent pad-state contract.
}
