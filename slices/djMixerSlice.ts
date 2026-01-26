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
  
  // Loop (Phase 3+)
  loop: Loop;
  
  // Hot cues (Phase 3+)
  hotCues: HotCue[];
  
  // Effects (Phase 3+)
  fx: DeckFX;
}

export interface MixerState {
  crossfader: number;      // -1 (full A) to +1 (full B)
  masterVolume: number;    // 0-1
  crossfaderCurve: 'linear' | 'constant-power' | 'sharp';
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
  
  // Loop (Phase 3+)
  setLoop: (deck: DeckId, start: number, end: number) => void;
  toggleLoop: (deck: DeckId) => void;
  clearLoop: (deck: DeckId) => void;
  
  // Hot cues (Phase 3+)
  setHotCue: (deck: DeckId, slot: number, position: number, label?: string, color?: string) => void;
  triggerHotCue: (deck: DeckId, slot: number) => void;
  clearHotCue: (deck: DeckId, slot: number) => void;
  
  // Effects (Phase 3+)
  setFilterFX: (deck: DeckId, params: Partial<FilterFX>) => void;
  setDelayFX: (deck: DeckId, params: Partial<DelayFX>) => void;
  setReverbFX: (deck: DeckId, params: Partial<ReverbFX>) => void;
  setFlangerFX: (deck: DeckId, params: Partial<FlangerFX>) => void;
  toggleFX: (deck: DeckId, fxType: EffectType) => void;
  
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
  loop: { enabled: false, start: 0, end: 0 },
  hotCues: [],
  fx: createDefaultFX(),
});

const createDefaultMixerState = (): MixerState => ({
  crossfader: 0,         // Center position
  masterVolume: 0.8,
  crossfaderCurve: 'constant-power',
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
          enabled: clampedValue !== 0
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
        beatGrid: beatGrid || null
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
  }),
});
