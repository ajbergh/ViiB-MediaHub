/**
 * ViiB MediaHub - DJ Sampler Audio Engine
 * 
 * Lightweight audio engine for 8 sampler pads using Web Audio API.
 * Each pad pre-loads an AudioBuffer for instant trigger with minimal latency.
 * 
 * Features:
 * - One-shot, loop, and gate playback modes
 * - Per-pad volume control
 * - Pre-loaded AudioBuffers for instant playback
 * - Routes through master output
 * 
 * @module lib/djSampler
 */

import { useStore } from '../store';

// ============================================================================
// Sampler Engine Singleton
// ============================================================================

class DJSamplerEngine {
  private audioContext: AudioContext | null = null;
  private masterGain: GainNode | null = null;
  
  // Per-pad state
  private buffers: (AudioBuffer | null)[] = new Array(8).fill(null);
  private sources: (AudioBufferSourceNode | null)[] = new Array(8).fill(null);
  private gains: (GainNode | null)[] = new Array(8).fill(null);
  private loadedUrls: (string | null)[] = new Array(8).fill(null);
  private startedAt: (number | null)[] = new Array(8).fill(null);

  // ========================================================================
  // Initialization
  // ========================================================================

  private ensureContext(): AudioContext {
    if (!this.audioContext || this.audioContext.state === 'closed') {
      this.audioContext = new AudioContext();
      this.masterGain = this.audioContext.createGain();
      this.masterGain.gain.value = 1.0;
      this.masterGain.connect(this.audioContext.destination);
      
      // Create per-pad gain nodes
      for (let i = 0; i < 8; i++) {
        this.gains[i] = this.audioContext.createGain();
        this.gains[i]!.gain.value = 0.8;
        this.gains[i]!.connect(this.masterGain!);
      }
    }
    
    if (this.audioContext.state === 'suspended') {
      this.audioContext.resume();
    }
    
    return this.audioContext;
  }

  // ========================================================================
  // Loading
  // ========================================================================

  async loadSample(padId: number, url: string): Promise<boolean> {
    if (padId < 0 || padId >= 8) return false;
    
    try {
      const ctx = this.ensureContext();
      
      // Skip if already loaded
      if (this.loadedUrls[padId] === url && this.buffers[padId]) return true;
      
      // Stop any current playback
      this.stopPad(padId);
      
      const response = await fetch(url);
      const arrayBuffer = await response.arrayBuffer();
      const audioBuffer = await ctx.decodeAudioData(arrayBuffer);
      
      this.buffers[padId] = audioBuffer;
      this.loadedUrls[padId] = url;
      
      return true;
    } catch (err) {
      console.error(`[DJSampler] Failed to load sample for pad ${padId}:`, err);
      return false;
    }
  }

  clearSample(padId: number): void {
    if (padId < 0 || padId >= 8) return;
    this.stopPad(padId);
    this.buffers[padId] = null;
    this.loadedUrls[padId] = null;
  }

  // ========================================================================
  // Playback
  // ========================================================================

  triggerPad(padId: number): void {
    if (padId < 0 || padId >= 8) return;
    
    const buffer = this.buffers[padId];
    if (!buffer) return;
    
    const ctx = this.ensureContext();
    const store = useStore.getState();
    const pad = store.djSampler[padId];
    if (!pad) return;
    
    // Stop current playback first
    this.stopSource(padId);
    
    // Create new source
    const source = ctx.createBufferSource();
    source.buffer = buffer;
    source.loop = pad.mode === 'loop';
    
    // Connect through per-pad gain
    if (this.gains[padId]) {
      this.gains[padId]!.gain.value = pad.volume;
      source.connect(this.gains[padId]!);
    }
    
    // Track end of playback for oneshot/loop
    source.onended = () => {
      if (this.sources[padId] === source) {
        this.sources[padId] = null;
        this.startedAt[padId] = null;
        store.setSamplerPadPlaying(padId, false);
      }
    };
    
    source.start(0);
    this.sources[padId] = source;
    this.startedAt[padId] = ctx.currentTime;
    store.setSamplerPadPlaying(padId, true);
  }

  stopPad(padId: number): void {
    if (padId < 0 || padId >= 8) return;
    this.stopSource(padId);
    useStore.getState().setSamplerPadPlaying(padId, false);
  }

  private stopSource(padId: number): void {
    const source = this.sources[padId];
    if (source) {
      try {
        source.stop();
      } catch {
        // Already stopped
      }
      source.disconnect();
      this.sources[padId] = null;
      this.startedAt[padId] = null;
    }
  }

  // ========================================================================
  // Volume
  // ========================================================================

  setVolume(padId: number, volume: number): void {
    if (padId < 0 || padId >= 8) return;
    const gain = this.gains[padId];
    if (gain) {
      gain.gain.value = Math.max(0, Math.min(1, volume));
    }
  }

  // ========================================================================
  // Utility
  // ========================================================================

  isLoaded(padId: number): boolean {
    return this.buffers[padId] !== null;
  }

  getDuration(padId: number): number {
    return this.buffers[padId]?.duration ?? 0;
  }

  getProgress(padId: number): number {
    if (padId < 0 || padId >= 8 || !this.audioContext) return 0;
    const source = this.sources[padId];
    const buffer = this.buffers[padId];
    const startedAt = this.startedAt[padId];
    if (!source || !buffer || startedAt === null || buffer.duration <= 0) return 0;

    const elapsed = Math.max(0, this.audioContext.currentTime - startedAt);
    const pad = useStore.getState().djSampler[padId];
    if (pad?.mode === 'loop') {
      return (elapsed % buffer.duration) / buffer.duration;
    }
    return Math.max(0, Math.min(1, elapsed / buffer.duration));
  }

  destroy(): void {
    for (let i = 0; i < 8; i++) {
      this.stopSource(i);
      this.buffers[i] = null;
      this.loadedUrls[i] = null;
      this.startedAt[i] = null;
    }
    
    if (this.audioContext && this.audioContext.state !== 'closed') {
      this.audioContext.close();
    }
    this.audioContext = null;
    this.masterGain = null;
    this.gains.fill(null);
  }
}

// Singleton instance
let samplerInstance: DJSamplerEngine | null = null;

export function getDJSamplerEngine(): DJSamplerEngine {
  if (!samplerInstance) {
    samplerInstance = new DJSamplerEngine();
  }
  return samplerInstance;
}

export { DJSamplerEngine };
