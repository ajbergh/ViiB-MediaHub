/**
 * ViiB MediaHub - DJ Mixer State Slice
 * 
 * Zustand slice managing the DJ Mode (two-deck mixer) feature state.
 * This is distinct from aiDjSlice which handles AI-powered playlist generation.
 * 
 * State:
 * - deckA/deckB: Individual deck state (loaded track, playing, position, volume, EQ)
 * - mixer: Crossfader position, master volume
 * - activeDeck: Which deck is currently "active" for keyboard shortcuts
 * - isEnabled: Whether DJ mode UI is currently active
 * 
 * This slice does NOT handle audio playback directly - that's managed by
 * the DJAudioEngine class in lib/djAudio.ts which uses Web Audio API.
 * 
 * @module slices/djMixerSlice
 */

import { StateCreator } from 'zustand';
import { Song } from '../types';
import { api } from '../services/api';

// ============================================================================
// Types
// ============================================================================

export type DeckId = 'A' | 'B';

export interface DeckEQ {
  high: number;  // -24 to +12 dB
  mid: number;   // -24 to +12 dB
  low: number;   // -24 to +12 dB
}

export interface DeckFilter {
  enabled: boolean;
  value: number;  // -1 (LP) to +1 (HP), 0 = off
}

export interface HotCue {
  slot: number;      // 1-8
  position: number;  // seconds
  label?: string;
  color: string;     // hex color
}

export interface Loop {
  enabled: boolean;
  start: number;     // seconds
  end: number;       // seconds
}

// Phase 3: Effect types
export type EffectType = 'filter' | 'delay' | 'reverb' | 'flanger';
export type BeatFXTarget = 'A' | 'B' | 'master';
export type BeatFXType = 'delay' | 'echo' | 'reverb' | 'filter' | 'flanger';
export type BeatFraction = '1/4' | '1/2' | '1' | '2' | '4';

export interface DJBeatFXState {
  enabled: boolean;
  target: BeatFXTarget;
  type: BeatFXType;
  fraction: BeatFraction;
  depth: number; // 0..1
}

export interface FilterFX {
  enabled: boolean;
  type: 'lowpass' | 'highpass';
  frequency: number;  // 20-20000 Hz
  resonance: number;  // 0-30
}

export interface DelayFX {
  enabled: boolean;
  time: number;       // Delay time in seconds (0.01-2)
  feedback: number;   // 0-0.95
  mix: number;        // 0-1 (dry/wet)
}

export interface ReverbFX {
  enabled: boolean;
  roomSize: number;   // 0-1
  damping: number;    // 0-1
  mix: number;        // 0-1 (dry/wet)
}

export interface FlangerFX {
  enabled: boolean;
  rate: number;       // LFO rate in Hz (0.1-10)
  depth: number;      // 0-1
  feedback: number;   // 0-0.95
}

export interface DeckFX {
  filter: FilterFX;
  delay: DelayFX;
  reverb: ReverbFX;
  flanger: FlangerFX;
}

export interface DeckState {
  // Track
  track: Song | null;
  
  // Transport
  isPlaying: boolean;
  position: number;        // Current playback position in seconds
  duration: number;        // Track duration in seconds
  cuePoint: number;        // Cue point position in seconds
  
  // Volume & Mixing
  volume: number;          // 0-1
  eq: DeckEQ;
  filter: DeckFilter;
  
  // Tempo (Phase 2+)
  tempo: number;           // 0.5-1.5 (1.0 = original)
  originalBpm: number | null;
  effectiveBpm: number | null;
  
  // Analysis (Phase 2+)
  key: string | null;      // e.g., "Am", "C#m"
  waveformPeaks: number[] | null;
  beatGrid: number[] | null;
  beatGridOffset: number;  // Manual beat grid offset in seconds (for alignment editing)
  
  // Loop (Phase 3+)
  loop: Loop;
  
  // Hot cues (Phase 3+)
  hotCues: HotCue[];
  
  // Effects (Phase 3+)
  fx: DeckFX;
  
  // Headphone Cue (Phase 4) - whether this deck is routed to headphones
  cueEnabled: boolean;
}

// ============================================================================
// Sampler Types
// ============================================================================

export interface SamplerPad {
  id: number;              // 0-7 pad index
  name: string;            // Display name (custom or filename)
  url: string | null;      // Audio URL (blob or file URL)
  needsRelink?: boolean;   // Metadata restored but the original local audio URL is not durable
  isPlaying: boolean;      // Currently playing
  volume: number;          // 0-1
  mode: 'oneshot' | 'loop' | 'gate'; // oneshot: play once, loop: repeat, gate: play while held
  color: string;           // Pad color for UI
}

export const DEFAULT_PAD_COLORS = [
  '#ef4444', '#f97316', '#eab308', '#22c55e', 
  '#3b82f6', '#a855f7', '#ec4899', '#06b6d4'
] as const;

export function createDefaultSamplerPad(id: number): SamplerPad {
  return {
    id,
    name: `Pad ${id + 1}`,
    url: null,
    isPlaying: false,
    volume: 0.8,
    mode: 'oneshot',
    color: DEFAULT_PAD_COLORS[id] || '#888',
  };
}

// Sync mode types
export type SyncMode = 'off' | 'bpm' | 'beat-phase';

// Layout mode for DJ V2 UI
export type DJLayoutMode = 'perf' | 'browse' | 'fx';

export interface MixerState {
  crossfader: number;      // -1 (full A) to +1 (full B)
  masterVolume: number;    // 0-1
  crossfaderCurve: 'linear' | 'constant-power' | 'sharp';
  
  // Headphone Cue (Phase 4)
  headphoneVolume: number;  // 0-1
  headphoneMix: number;     // 0 = cue only, 1 = master only, 0.5 = 50/50
  masterCueEnabled: boolean; // Route master mix to headphone monitoring

  // Beat FX (Phase 6)
  beatFX: DJBeatFXState;
  
  // Sync Mode (Phase 4)
  syncMode: SyncMode;       // off, bpm only, or beat-phase sync
  
  // Quantize
  quantize: boolean;         // Snap actions to beat grid
  
  // Key Lock
  keyLockA: boolean;         // Preserve pitch when changing tempo (Deck A)
  keyLockB: boolean;         // Preserve pitch when changing tempo (Deck B)
  
  // Slip Mode
  slipModeA: boolean;        // Slip mode Deck A (playback continues in background during scratch)
  slipModeB: boolean;        // Slip mode Deck B
  
  // Auto-Gain
  autoGainA: boolean;        // Auto-gain normalization Deck A
  autoGainB: boolean;        // Auto-gain normalization Deck B
  
  // Rendering
  useWebGLWaveform: boolean; // Use WebGL2 waveform renderer (vs Canvas 2D fallback)

  // UI Layout
  djLayoutMode: DJLayoutMode; // perf (default), browse (expanded library), fx (expanded FX)
}

// ============================================================================
// Slice Interface
// ============================================================================

export interface DJMixerSlice {
  // State
  djMixerEnabled: boolean;
  djActiveDeck: DeckId;
  djDeckA: DeckState;
  djDeckB: DeckState;
  djMixer: MixerState;
  djSampler: SamplerPad[];
  
  // DJ Mode toggle
  setDJMixerEnabled: (enabled: boolean) => void;
  
  // Active deck
  setActiveDeck: (deck: DeckId) => void;
  toggleActiveDeck: () => void;
  
  // Deck loading
  loadTrackToDeck: (deck: DeckId, track: Song) => void;
  unloadDeck: (deck: DeckId) => void;
  
  // Transport
  playDeck: (deck: DeckId) => void;
  pauseDeck: (deck: DeckId) => void;
  togglePlayDeck: (deck: DeckId) => void;
  seekDeck: (deck: DeckId, position: number) => void;
  cueDeck: (deck: DeckId) => void;
  setCuePoint: (deck: DeckId, position: number) => void;
  
  // Position updates (called from audio engine)
  updateDeckPosition: (deck: DeckId, position: number) => void;
  setDeckPosition: (deck: DeckId, position: number) => void; // Alias for updateDeckPosition
  setDeckDuration: (deck: DeckId, duration: number) => void;
  setDeckPlaying: (deck: DeckId, isPlaying: boolean) => void;
  
  // Volume & EQ
  setDeckVolume: (deck: DeckId, volume: number) => void;
  setDeckEQ: (deck: DeckId, band: keyof DeckEQ, value: number) => void;
  resetDeckEQ: (deck: DeckId) => void;
  setDeckFilter: (deck: DeckId, value: number) => void;
  setDeckFilterEnabled: (deck: DeckId, enabled: boolean) => void;
  
  // Mixer
  setCrossfader: (position: number) => void;
  setMasterVolume: (volume: number) => void;
  setCrossfaderCurve: (curve: MixerState['crossfaderCurve']) => void;
  
  // Tempo (Phase 2+)
  setDeckTempo: (deck: DeckId, tempo: number) => void;
  syncDeck: (deck: DeckId) => void;
  
  // Analysis data (from backend)
  setDeckWaveform: (deck: DeckId, peaks: number[]) => void;
  setDeckAnalysis: (deck: DeckId, bpm: number | null, key: string | null, beatGrid?: number[]) => void;
  
  // Beat grid editing (Phase 3)
  shiftBeatGrid: (deck: DeckId, offsetDelta: number) => void;  // Shift by ±ms
  resetBeatGridOffset: (deck: DeckId) => void;
  
  // Loop (Phase 3+)
  setLoop: (deck: DeckId, start: number, end: number) => void;
  toggleLoop: (deck: DeckId) => void;
  clearLoop: (deck: DeckId) => void;
  
  // Hot cues (Phase 3+)
  setHotCue: (deck: DeckId, slot: number, position: number, label?: string, color?: string) => void;
  triggerHotCue: (deck: DeckId, slot: number) => void;
  clearHotCue: (deck: DeckId, slot: number) => void;
  loadHotCues: (deck: DeckId, hotCues: HotCue[]) => void;
  saveHotCuesToBackend: (deck: DeckId) => Promise<void>;
  
  // Effects (Phase 3+)
  setFilterFX: (deck: DeckId, params: Partial<FilterFX>) => void;
  setDelayFX: (deck: DeckId, params: Partial<DelayFX>) => void;
  setReverbFX: (deck: DeckId, params: Partial<ReverbFX>) => void;
  setFlangerFX: (deck: DeckId, params: Partial<FlangerFX>) => void;
  toggleFX: (deck: DeckId, fxType: EffectType) => void;

  // Beat FX (Phase 6)
  setBeatFXEnabled: (enabled: boolean) => void;
  setBeatFXTarget: (target: BeatFXTarget) => void;
  setBeatFXType: (type: BeatFXType) => void;
  setBeatFXFraction: (fraction: BeatFraction) => void;
  setBeatFXDepth: (depth: number) => void;
  
  // Headphone Cue (Phase 4)
  setDeckCue: (deck: DeckId, enabled: boolean) => void;
  toggleDeckCue: (deck: DeckId) => void;
  setHeadphoneVolume: (volume: number) => void;
  setHeadphoneMix: (mix: number) => void;
  setMasterCueEnabled: (enabled: boolean) => void;
  toggleMasterCue: () => void;
  
  // Beat-Phase Sync (Phase 4)
  setSyncMode: (mode: SyncMode) => void;
  syncBeatPhase: (targetDeck: DeckId) => void;
  toggleQuantize: () => void;
  toggleKeyLock: (deck: DeckId) => void;
  setKeyLock: (deck: DeckId, enabled: boolean) => void;
  toggleSlipMode: (deck: DeckId) => void;
  toggleAutoGain: (deck: DeckId) => void;
  toggleWebGLWaveform: () => void;
  setDJLayoutMode: (mode: DJLayoutMode) => void;
  
  // Sampler
  loadSamplerPad: (padId: number, name: string, url: string) => void;
  restoreSamplerPadMetadata: (padId: number, metadata: Pick<SamplerPad, 'name' | 'volume' | 'mode' | 'color'>) => void;
  clearSamplerPad: (padId: number) => void;
  setSamplerPadPlaying: (padId: number, playing: boolean) => void;
  setSamplerPadVolume: (padId: number, volume: number) => void;
  setSamplerPadMode: (padId: number, mode: SamplerPad['mode']) => void;
  
  // Bulk state (for initialization)
  getDeckState: (deck: DeckId) => DeckState;
  
  // Reset
  resetDJMixer: () => void;
}

// ============================================================================
// Default State
// ============================================================================

const createDefaultFX = (): DeckFX => ({
  filter: {
    enabled: false,
    type: 'lowpass',
    frequency: 1000,
    resonance: 1,
  },
  delay: {
    enabled: false,
    time: 0.375, // 3/8 note at 120 BPM
    feedback: 0.3,
    mix: 0.3,
  },
  reverb: {
    enabled: false,
    roomSize: 0.5,
    damping: 0.5,
    mix: 0.3,
  },
  flanger: {
    enabled: false,
    rate: 0.5,
    depth: 0.5,
    feedback: 0.3,
  },
});

const createDefaultDeckState = (): DeckState => ({
  track: null,
  isPlaying: false,
  position: 0,
  duration: 0,
  cuePoint: 0,
  volume: 0.75,
  eq: { high: 0, mid: 0, low: 0 },
  filter: { enabled: false, value: 0 },
  tempo: 1.0,
  originalBpm: null,
  effectiveBpm: null,
  key: null,
  waveformPeaks: null,
  beatGrid: null,
  beatGridOffset: 0,     // No offset by default
  loop: { enabled: false, start: 0, end: 0 },
  hotCues: [],
  fx: createDefaultFX(),
  cueEnabled: false,  // Headphone cue (Phase 4)
});

const createDefaultMixerState = (): MixerState => ({
  crossfader: 0,         // Center position
  masterVolume: 0.8,
  crossfaderCurve: 'constant-power',
  headphoneVolume: 1.0,  // Phase 4: Headphone cue volume
  headphoneMix: 0.5,     // Phase 4: 0 = cue only, 1 = master only
  masterCueEnabled: false, // Phase 7: Master cue off until explicitly monitored
  beatFX: {
    enabled: false,
    target: 'A',
    type: 'delay',
    fraction: '1',
    depth: 0.45,
  },
  syncMode: 'bpm',       // Phase 4: Default to BPM sync
  quantize: true,          // Default quantize on
  keyLockA: true,          // Default key lock ON (preserve pitch)
  keyLockB: true,          // Default key lock ON (preserve pitch)
  slipModeA: false,        // Default slip mode OFF
  slipModeB: false,        // Default slip mode OFF
  autoGainA: false,        // Default auto-gain OFF
  autoGainB: false,        // Default auto-gain OFF
  useWebGLWaveform: false, // Default Canvas 2D (safer fallback)
  djLayoutMode: 'perf',   // Default layout mode: Performance
});

// ============================================================================
// Slice Creator
// ============================================================================

export const createDJMixerSlice: StateCreator<DJMixerSlice, [], [], DJMixerSlice> = (set, get) => ({
  // Initial state
  djMixerEnabled: false,
  djActiveDeck: 'A',
  djDeckA: createDefaultDeckState(),
  djDeckB: createDefaultDeckState(),
  djMixer: createDefaultMixerState(),
  djSampler: Array.from({ length: 8 }, (_, i) => createDefaultSamplerPad(i)),
  
  // DJ Mode toggle
  setDJMixerEnabled: (enabled) => set({ djMixerEnabled: enabled }),
  
  // Active deck
  setActiveDeck: (deck) => set({ djActiveDeck: deck }),
  toggleActiveDeck: () => set((state) => ({ 
    djActiveDeck: state.djActiveDeck === 'A' ? 'B' : 'A' 
  })),
  
  // Deck loading
  loadTrackToDeck: (deck, track) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    set((state) => ({
      [deckKey]: {
        ...createDefaultDeckState(),
        track,
        duration: track.duration || 0, // Initialize from track metadata
        volume: state[deckKey].volume, // Preserve volume setting
        eq: state[deckKey].eq,         // Preserve EQ setting
      }
    }));
  },
  
  unloadDeck: (deck) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    set((state) => ({
      [deckKey]: {
        ...createDefaultDeckState(),
        volume: state[deckKey].volume,
        eq: state[deckKey].eq,
      }
    }));
  },
  
  // Transport
  playDeck: (deck) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    set((state) => ({
      [deckKey]: { ...state[deckKey], isPlaying: true }
    }));
  },
  
  pauseDeck: (deck) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    set((state) => ({
      [deckKey]: { ...state[deckKey], isPlaying: false }
    }));
  },
  
  togglePlayDeck: (deck) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    set((state) => ({
      [deckKey]: { ...state[deckKey], isPlaying: !state[deckKey].isPlaying }
    }));
  },
  
  seekDeck: (deck, position) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    set((state) => ({
      [deckKey]: { ...state[deckKey], position: Math.max(0, Math.min(position, state[deckKey].duration)) }
    }));
  },
  
  cueDeck: (deck) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    set((state) => ({
      [deckKey]: { 
        ...state[deckKey], 
        position: state[deckKey].cuePoint,
        isPlaying: false 
      }
    }));
  },
  
  setCuePoint: (deck, position) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    set((state) => ({
      [deckKey]: { ...state[deckKey], cuePoint: position }
    }));
  },
  
  // Position updates
  updateDeckPosition: (deck, position) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    set((state) => ({
      [deckKey]: { ...state[deckKey], position }
    }));
  },
  
  // Alias for updateDeckPosition
  setDeckPosition: (deck, position) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    set((state) => ({
      [deckKey]: { ...state[deckKey], position }
    }));
  },
  
  setDeckDuration: (deck, duration) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    set((state) => ({
      [deckKey]: { ...state[deckKey], duration }
    }));
  },
  
  setDeckPlaying: (deck, isPlaying) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    set((state) => ({
      [deckKey]: { ...state[deckKey], isPlaying }
    }));
  },
  
  // Volume & EQ
  setDeckVolume: (deck, volume) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    set((state) => ({
      [deckKey]: { ...state[deckKey], volume: Math.max(0, Math.min(1, volume)) }
    }));
  },
  
  setDeckEQ: (deck, band, value) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    // Clamp to -24 to +12 dB range
    const clampedValue = Math.max(-24, Math.min(12, value));
    set((state) => ({
      [deckKey]: { 
        ...state[deckKey], 
        eq: { ...state[deckKey].eq, [band]: clampedValue }
      }
    }));
  },
  
  resetDeckEQ: (deck) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    set((state) => ({
      [deckKey]: { 
        ...state[deckKey], 
        eq: { high: 0, mid: 0, low: 0 }
      }
    }));
  },
  
  setDeckFilter: (deck, value) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    const clampedValue = Math.max(-1, Math.min(1, value));
    set((state) => ({
      [deckKey]: { 
        ...state[deckKey], 
        filter: { 
          ...state[deckKey].filter, 
          value: clampedValue,
          enabled: Math.abs(clampedValue) >= 0.05
        }
      }
    }));
  },
  
  setDeckFilterEnabled: (deck, enabled) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    set((state) => ({
      [deckKey]: { 
        ...state[deckKey], 
        filter: { ...state[deckKey].filter, enabled }
      }
    }));
  },
  
  // Mixer
  setCrossfader: (position) => {
    set((state) => ({
      djMixer: { 
        ...state.djMixer, 
        crossfader: Math.max(-1, Math.min(1, position)) 
      }
    }));
  },
  
  setMasterVolume: (volume) => {
    set((state) => ({
      djMixer: { 
        ...state.djMixer, 
        masterVolume: Math.max(0, Math.min(1, volume)) 
      }
    }));
  },
  
  setCrossfaderCurve: (curve) => {
    set((state) => ({
      djMixer: { ...state.djMixer, crossfaderCurve: curve }
    }));
  },
  
  // Tempo
  setDeckTempo: (deck, tempo) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    const clampedTempo = Math.max(0.5, Math.min(1.5, tempo));
    set((state) => {
      const originalBpm = state[deckKey].originalBpm;
      return {
        [deckKey]: { 
          ...state[deckKey], 
          tempo: clampedTempo,
          effectiveBpm: originalBpm ? Math.round(originalBpm * clampedTempo * 10) / 10 : null
        }
      };
    });
  },
  
  syncDeck: (deck) => {
    const state = get();
    const sourceDeck = deck === 'A' ? state.djDeckB : state.djDeckA;
    const targetDeckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    
    if (sourceDeck.effectiveBpm && state[targetDeckKey].originalBpm) {
      const newTempo = sourceDeck.effectiveBpm / state[targetDeckKey].originalBpm;
      const clampedTempo = Math.max(0.5, Math.min(1.5, newTempo));
      
      set({
        [targetDeckKey]: {
          ...state[targetDeckKey],
          tempo: clampedTempo,
          effectiveBpm: Math.round(state[targetDeckKey].originalBpm! * clampedTempo * 10) / 10
        }
      });
    }
  },
  
  // Analysis data
  setDeckWaveform: (deck, peaks) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    set((state) => ({
      [deckKey]: { ...state[deckKey], waveformPeaks: peaks }
    }));
  },
  
  setDeckAnalysis: (deck, bpm, key, beatGrid) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    set((state) => ({
      [deckKey]: { 
        ...state[deckKey], 
        originalBpm: bpm,
        effectiveBpm: bpm ? Math.round(bpm * state[deckKey].tempo * 10) / 10 : null,
        key,
        beatGrid: beatGrid || null,
        beatGridOffset: 0  // Reset offset when new analysis arrives
      }
    }));
  },
  
  // Beat grid editing
  shiftBeatGrid: (deck, offsetDelta) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    set((state) => ({
      [deckKey]: {
        ...state[deckKey],
        beatGridOffset: state[deckKey].beatGridOffset + offsetDelta,
      }
    }));
  },
  
  resetBeatGridOffset: (deck) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    set((state) => ({
      [deckKey]: {
        ...state[deckKey],
        beatGridOffset: 0,
      }
    }));
  },
  
  // Loop
  setLoop: (deck, start, end) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    set((state) => ({
      [deckKey]: { 
        ...state[deckKey], 
        loop: { enabled: true, start, end }
      }
    }));
  },
  
  toggleLoop: (deck) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    set((state) => ({
      [deckKey]: { 
        ...state[deckKey], 
        loop: { ...state[deckKey].loop, enabled: !state[deckKey].loop.enabled }
      }
    }));
  },
  
  clearLoop: (deck) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    set((state) => ({
      [deckKey]: { 
        ...state[deckKey], 
        loop: { enabled: false, start: 0, end: 0 }
      }
    }));
  },
  
  // Hot cues
  setHotCue: (deck, slot, position, label, color) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    set((state) => {
      const hotCues = [...state[deckKey].hotCues];
      const existingIndex = hotCues.findIndex(hc => hc.slot === slot);
      const newHotCue: HotCue = {
        slot,
        position,
        label,
        color: color || '#FF5500'
      };
      
      if (existingIndex >= 0) {
        hotCues[existingIndex] = newHotCue;
      } else {
        hotCues.push(newHotCue);
      }
      
      return {
        [deckKey]: { ...state[deckKey], hotCues }
      };
    });
    // Auto-save to backend after setting
    get().saveHotCuesToBackend(deck);
  },
  
  triggerHotCue: (deck, slot) => {
    const state = get();
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    const hotCue = state[deckKey].hotCues.find(hc => hc.slot === slot);
    
    if (hotCue) {
      set({
        [deckKey]: { 
          ...state[deckKey], 
          position: hotCue.position,
          isPlaying: true
        }
      });
    }
  },
  
  clearHotCue: (deck, slot) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    set((state) => ({
      [deckKey]: { 
        ...state[deckKey], 
        hotCues: state[deckKey].hotCues.filter(hc => hc.slot !== slot)
      }
    }));
    // Auto-save to backend after clearing
    get().saveHotCuesToBackend(deck);
  },
  
  loadHotCues: (deck, hotCues) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    set((state) => ({
      [deckKey]: { 
        ...state[deckKey], 
        hotCues 
      }
    }));
    console.log(`🎯 loadHotCues: Loaded ${hotCues.length} hot cues for Deck ${deck}`);
  },
  
  saveHotCuesToBackend: async (deck) => {
    const state = get();
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    const deckState = state[deckKey];
    
    if (!deckState.track?.id) {
      console.log(`🎯 saveHotCuesToBackend: No track loaded on Deck ${deck}, skipping`);
      return;
    }
    
    try {
      const hotCuesToSave = deckState.hotCues.map(hc => ({
        slot: hc.slot,
        position: hc.position,
        label: hc.label || '',
        color: hc.color || '#FF5500'
      }));
      
      await api.saveDJHotCues(deckState.track.id, hotCuesToSave);
      console.log(`🎯 saveHotCuesToBackend: Saved ${hotCuesToSave.length} hot cues for track ${deckState.track.id}`);
    } catch (error) {
      console.error(`🎯 saveHotCuesToBackend: Failed to save hot cues for Deck ${deck}:`, error);
    }
  },
  
  // ============================================================================
  // Effects (Phase 3+)
  // ============================================================================
  
  setFilterFX: (deck, params) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    set((state) => ({
      [deckKey]: {
        ...state[deckKey],
        fx: {
          ...state[deckKey].fx,
          filter: { ...state[deckKey].fx.filter, ...params },
        },
      },
    }));
  },
  
  setDelayFX: (deck, params) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    set((state) => ({
      [deckKey]: {
        ...state[deckKey],
        fx: {
          ...state[deckKey].fx,
          delay: { ...state[deckKey].fx.delay, ...params },
        },
      },
    }));
  },
  
  setReverbFX: (deck, params) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    set((state) => ({
      [deckKey]: {
        ...state[deckKey],
        fx: {
          ...state[deckKey].fx,
          reverb: { ...state[deckKey].fx.reverb, ...params },
        },
      },
    }));
  },
  
  setFlangerFX: (deck, params) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    set((state) => ({
      [deckKey]: {
        ...state[deckKey],
        fx: {
          ...state[deckKey].fx,
          flanger: { ...state[deckKey].fx.flanger, ...params },
        },
      },
    }));
  },
  
  toggleFX: (deck, fxType) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    set((state) => ({
      [deckKey]: {
        ...state[deckKey],
        fx: {
          ...state[deckKey].fx,
          [fxType]: {
            ...state[deckKey].fx[fxType],
            enabled: !state[deckKey].fx[fxType].enabled,
          },
        },
      },
    }));
  },

  // Beat FX (Phase 6)
  setBeatFXEnabled: (enabled) => {
    set((state) => ({
      djMixer: {
        ...state.djMixer,
        beatFX: {
          ...state.djMixer.beatFX,
          enabled,
        },
      },
    }));
  },

  setBeatFXTarget: (target) => {
    set((state) => ({
      djMixer: {
        ...state.djMixer,
        beatFX: {
          ...state.djMixer.beatFX,
          target,
        },
      },
    }));
  },

  setBeatFXType: (type) => {
    set((state) => ({
      djMixer: {
        ...state.djMixer,
        beatFX: {
          ...state.djMixer.beatFX,
          type,
        },
      },
    }));
  },

  setBeatFXFraction: (fraction) => {
    set((state) => ({
      djMixer: {
        ...state.djMixer,
        beatFX: {
          ...state.djMixer.beatFX,
          fraction,
        },
      },
    }));
  },

  setBeatFXDepth: (depth) => {
    const safeDepth = (typeof depth === 'number' && isFinite(depth))
      ? Math.max(0, Math.min(1, depth))
      : 0.45;
    set((state) => ({
      djMixer: {
        ...state.djMixer,
        beatFX: {
          ...state.djMixer.beatFX,
          depth: safeDepth,
        },
      },
    }));
  },
  
  // Headphone Cue (Phase 4)
  setDeckCue: (deck, enabled) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    set((state) => ({
      [deckKey]: {
        ...state[deckKey],
        cueEnabled: enabled,
      },
    }));
  },
  
  toggleDeckCue: (deck) => {
    const deckKey = deck === 'A' ? 'djDeckA' : 'djDeckB';
    set((state) => ({
      [deckKey]: {
        ...state[deckKey],
        cueEnabled: !state[deckKey].cueEnabled,
      },
    }));
  },
  
  setHeadphoneVolume: (volume) => {
    // Guard against NaN/undefined - use default if invalid
    const safeVolume = (typeof volume === 'number' && isFinite(volume))
      ? Math.max(0, Math.min(1, volume))
      : 1.0;
    set((state) => ({
      djMixer: {
        ...state.djMixer,
        headphoneVolume: safeVolume,
      },
    }));
  },
  
  setHeadphoneMix: (mix) => {
    // Guard against NaN/undefined - use default if invalid
    const safeMix = (typeof mix === 'number' && isFinite(mix))
      ? Math.max(0, Math.min(1, mix))
      : 0.5;
    set((state) => ({
      djMixer: {
        ...state.djMixer,
        headphoneMix: safeMix,
      },
    }));
  },

  setMasterCueEnabled: (enabled) => {
    set((state) => ({
      djMixer: {
        ...state.djMixer,
        masterCueEnabled: enabled,
      },
    }));
  },

  toggleMasterCue: () => {
    set((state) => ({
      djMixer: {
        ...state.djMixer,
        masterCueEnabled: !state.djMixer.masterCueEnabled,
      },
    }));
  },
  
  // Beat-Phase Sync (Phase 4)
  setSyncMode: (mode) => {
    set((state) => ({
      djMixer: {
        ...state.djMixer,
        syncMode: mode,
      },
    }));
  },
  
  syncBeatPhase: (targetDeck) => {
    // Beat-phase sync: Align the target deck's beat phase with the other deck
    // This is called when user presses SYNC and syncMode is 'beat-phase'
    const state = get();
    const sourceDeck = targetDeck === 'A' ? state.djDeckB : state.djDeckA;
    const target = targetDeck === 'A' ? state.djDeckA : state.djDeckB;
    
    // Both decks need beat grids for phase sync
    if (!sourceDeck.beatGrid?.length || !target.beatGrid?.length) {
      console.warn('Beat-phase sync requires beat grids on both decks');
      return;
    }
    
    // Calculate current beat position in source deck
    const sourcePosition = sourceDeck.position;
    const sourceBeatGrid = sourceDeck.beatGrid;
    
    // Find the current beat index in source
    let sourceBeatIndex = 0;
    for (let i = 0; i < sourceBeatGrid.length - 1; i++) {
      if (sourcePosition >= sourceBeatGrid[i] && sourcePosition < sourceBeatGrid[i + 1]) {
        sourceBeatIndex = i;
        break;
      }
    }
    
    // Calculate source's phase within current beat (0-1)
    const sourceBeatStart = sourceBeatGrid[sourceBeatIndex];
    const sourceBeatEnd = sourceBeatGrid[sourceBeatIndex + 1] || sourceBeatStart + 0.5;
    const sourceBeatDuration = sourceBeatEnd - sourceBeatStart;
    const sourcePhase = (sourcePosition - sourceBeatStart) / sourceBeatDuration;
    
    // Find target's current beat position
    const targetPosition = target.position;
    const targetBeatGrid = target.beatGrid;
    
    let targetBeatIndex = 0;
    for (let i = 0; i < targetBeatGrid.length - 1; i++) {
      if (targetPosition >= targetBeatGrid[i] && targetPosition < targetBeatGrid[i + 1]) {
        targetBeatIndex = i;
        break;
      }
    }
    
    // Calculate where target should be to match source phase
    const targetBeatStart = targetBeatGrid[targetBeatIndex];
    const targetBeatEnd = targetBeatGrid[targetBeatIndex + 1] || targetBeatStart + 0.5;
    const targetBeatDuration = targetBeatEnd - targetBeatStart;
    const targetIdealPosition = targetBeatStart + (sourcePhase * targetBeatDuration);
    
    // Calculate offset needed
    const phaseOffset = targetIdealPosition - targetPosition;
    
    // Store the offset for the audio engine to apply
    // The actual seek will be handled by the audio engine
    const deckKey = targetDeck === 'A' ? 'djDeckA' : 'djDeckB';
    set((state) => ({
      [deckKey]: {
        ...state[deckKey],
        // Store a small nudge offset - audio engine will pick this up
        position: Math.max(0, targetPosition + phaseOffset),
      },
    }));
    
    console.log(`Beat-phase sync: Deck ${targetDeck} nudged by ${(phaseOffset * 1000).toFixed(1)}ms`);
  },
  
  toggleQuantize: () => {
    set((state) => ({
      djMixer: {
        ...state.djMixer,
        quantize: !state.djMixer.quantize,
      },
    }));
  },
  
  toggleKeyLock: (deck) => {
    set((state) => ({
      djMixer: {
        ...state.djMixer,
        ...(deck === 'A'
          ? { keyLockA: !state.djMixer.keyLockA }
          : { keyLockB: !state.djMixer.keyLockB }),
      },
    }));
  },

  setKeyLock: (deck, enabled) => {
    set((state) => ({
      djMixer: {
        ...state.djMixer,
        ...(deck === 'A'
          ? { keyLockA: enabled }
          : { keyLockB: enabled }),
      },
    }));
  },
  
  toggleSlipMode: (deck) => {
    set((state) => ({
      djMixer: {
        ...state.djMixer,
        ...(deck === 'A'
          ? { slipModeA: !state.djMixer.slipModeA }
          : { slipModeB: !state.djMixer.slipModeB }),
      },
    }));
  },
  
  toggleAutoGain: (deck) => {
    set((state) => ({
      djMixer: {
        ...state.djMixer,
        ...(deck === 'A'
          ? { autoGainA: !state.djMixer.autoGainA }
          : { autoGainB: !state.djMixer.autoGainB }),
      },
    }));
  },
  
  toggleWebGLWaveform: () => {
    set((state) => ({
      djMixer: {
        ...state.djMixer,
        useWebGLWaveform: !state.djMixer.useWebGLWaveform,
      },
    }));
  },

  setDJLayoutMode: (mode) => {
    set((state) => ({
      djMixer: {
        ...state.djMixer,
        djLayoutMode: mode,
      },
    }));
  },
  
  // Sampler pad actions
  loadSamplerPad: (padId, name, url) => {
    set((state) => ({
      djSampler: state.djSampler.map(pad =>
        pad.id === padId ? { ...pad, name, url, needsRelink: false, isPlaying: false } : pad
      ),
    }));
  },

  restoreSamplerPadMetadata: (padId, metadata) => {
    set((state) => ({
      djSampler: state.djSampler.map(pad =>
        pad.id === padId
          ? {
              ...pad,
              ...metadata,
              url: null,
              needsRelink: true,
              isPlaying: false,
            }
          : pad
      ),
    }));
  },
  
  clearSamplerPad: (padId) => {
    set((state) => ({
      djSampler: state.djSampler.map(pad =>
        pad.id === padId ? createDefaultSamplerPad(padId) : pad
      ),
    }));
  },
  
  setSamplerPadPlaying: (padId, playing) => {
    set((state) => ({
      djSampler: state.djSampler.map(pad =>
        pad.id === padId ? { ...pad, isPlaying: playing } : pad
      ),
    }));
  },
  
  setSamplerPadVolume: (padId, volume) => {
    set((state) => ({
      djSampler: state.djSampler.map(pad =>
        pad.id === padId ? { ...pad, volume: Math.max(0, Math.min(1, volume)) } : pad
      ),
    }));
  },
  
  setSamplerPadMode: (padId, mode) => {
    set((state) => ({
      djSampler: state.djSampler.map(pad =>
        pad.id === padId ? { ...pad, mode } : pad
      ),
    }));
  },
  
  // Utility
  getDeckState: (deck) => {
    const state = get();
    return deck === 'A' ? state.djDeckA : state.djDeckB;
  },
  
  // Reset
  resetDJMixer: () => set({
    djMixerEnabled: false,
    djActiveDeck: 'A',
    djDeckA: createDefaultDeckState(),
    djDeckB: createDefaultDeckState(),
    djMixer: createDefaultMixerState(),
    djSampler: Array.from({ length: 8 }, (_, i) => createDefaultSamplerPad(i)),
  }),
});
