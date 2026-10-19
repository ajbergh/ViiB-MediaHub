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
import { getDJAudioEngine } from './djAudio';

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
  private triggerGeneration = new Array<number>(8).fill(0);

  // ========================================================================
  // Initialization
  // ========================================================================

  /** The DJ engine owns the context and physical output routing. */
  attachOutput(context: AudioContext, destination: AudioNode): void {
    if (this.audioContext === context && this.masterGain) return;
    if (this.audioContext || this.masterGain) this.detachOutput();
    this.audioContext = context;
    this.masterGain = context.createGain();
    this.masterGain.gain.value = 1.0;
    this.masterGain.connect(destination);

    for (let i = 0; i < 8; i++) {
      this.gains[i] = context.createGain();
      this.gains[i]!.gain.value = 0.8;
      this.gains[i]!.connect(this.masterGain);
    }
  }

  detachOutput(): void {
    for (let i = 0; i < 8; i++) {
      ++this.triggerGeneration[i];
      if (this.sources[i]) this.stopPad(i);
      this.gains[i]?.disconnect();
    }
    this.masterGain?.disconnect();
    this.masterGain = null;
    this.audioContext = null;
    this.gains.fill(null);
  }

  private ensureContext(): AudioContext {
    if (!this.audioContext || this.audioContext.state === 'closed') {
      throw new Error('DJ audio must be initialized before sampler playback.');
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
      if (!this.audioContext || this.audioContext.state === 'closed') await getDJAudioEngine().initialize();
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

  async triggerPad(padId: number): Promise<void> {
    if (padId < 0 || padId >= 8) return;
    
    const buffer = this.buffers[padId];
    if (!buffer) return;
    const generation = ++this.triggerGeneration[padId];
    if (!this.audioContext || this.audioContext.state === 'closed') {
      try { await getDJAudioEngine().initialize(); }
      catch (error) { console.error('[DJSampler] DJ audio initialization failed:', error); return; }
      // A gate release or another press while initializing cancels this trigger.
      if (this.triggerGeneration[padId] !== generation) return;
    }
    
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
    ++this.triggerGeneration[padId];
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
    this.detachOutput();
    for (let i = 0; i < 8; i++) {
      this.stopSource(i);
      this.buffers[i] = null;
      this.loadedUrls[i] = null;
      this.startedAt[i] = null;
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
