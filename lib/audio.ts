/**
 * ViiB MediaHub - Audio Engine
 * 
 * Web Audio API abstraction for audio processing and effects.
 * Manages the audio graph including EQ, volume, and visualization.
 * 
 * Audio Graph Architecture:
 * [Audio Elements] -> [Input Gains] -> [10-Band EQ] -> [Analyser] -> [Master Gain] -> [Destination]
 * 
 * Features:
 * - 10-band parametric equalizer (32Hz - 16kHz)
 * - Smooth crossfade transitions between tracks
 * - Real-time frequency analysis for visualizer
 * - Volume control with scheduled value changes
 * - Multiple audio source registration (for dual-player crossfade)
 * 
 * Note: Requires user interaction before AudioContext can be resumed
 * due to browser autoplay policies.
 * 
 * @module audio
 */

import { EQ_FREQUENCIES } from "../utils";

class AudioEngine {
  private context: AudioContext | null = null;
  private masterGain: GainNode | null = null;
  private analyser: AnalyserNode | null = null;
  private eqNodes: BiquadFilterNode[] = [];
  
  // Track connected sources and their individual gain nodes (for crossfading)
  private sources: Map<HTMLAudioElement, { source: MediaElementAudioSourceNode, inputGain: GainNode }> = new Map();

  constructor() {}

  init() {
    if (this.context) return;

    const AudioContextClass = window.AudioContext || (window as any).webkitAudioContext;
    this.context = new AudioContextClass();

    this.masterGain = this.context.createGain();
    this.masterGain.gain.value = 1;

    this.analyser = this.context.createAnalyser();
    this.analyser.fftSize = 2048;
    this.analyser.smoothingTimeConstant = 0.8;

    // Create EQ Bands
    this.eqNodes = EQ_FREQUENCIES.map((freq) => {
      const filter = this.context!.createBiquadFilter();
      filter.type = 'peaking';
      filter.frequency.value = freq;
      filter.Q.value = 1.0;
      filter.gain.value = 0;
      return filter;
    });

    // Connect Chain: InputNodes -> EQ -> Analyser -> Master -> Dest
    
    // 1. Link EQ Nodes
    for (let i = 0; i < this.eqNodes.length - 1; i++) {
        this.eqNodes[i].connect(this.eqNodes[i + 1]);
    }

    // 2. Link EQ to Analyser
    const lastEq = this.eqNodes[this.eqNodes.length - 1];
    if (lastEq) {
        lastEq.connect(this.analyser);
    } else {
        // Fallback if no EQ (shouldn't happen given constant)
    }

    // 3. Link Analyser to Master
    this.analyser.connect(this.masterGain);

    // 4. Link Master to Dest
    this.masterGain.connect(this.context.destination);
  }

  // Register audio element to the graph
  register(element: HTMLAudioElement) {
      if (!this.context) this.init();
      if (this.sources.has(element)) return;

      const source = this.context!.createMediaElementSource(element);
      const inputGain = this.context!.createGain();
      
      source.connect(inputGain);
      
      // Connect to start of EQ chain
      if (this.eqNodes.length > 0) {
          inputGain.connect(this.eqNodes[0]);
      } else {
          inputGain.connect(this.analyser!);
      }

      this.sources.set(element, { source, inputGain });
  }

  setVolume(vol: number) {
      if (this.masterGain && this.context) {
          try {
            this.masterGain.gain.cancelScheduledValues(this.context.currentTime);
            this.masterGain.gain.setValueAtTime(vol, this.context.currentTime);
          } catch(e) {
            this.masterGain.gain.value = vol;
          }
      }
  }

  setEqBands(gains: number[]) {
    if (!this.context || this.eqNodes.length === 0) return;
    const now = this.context.currentTime;

    this.eqNodes.forEach((node, index) => {
      const gainValue = gains[index];
      if (typeof gainValue === 'number' && isFinite(gainValue)) {
        // Use setTargetAtTime for smoother EQ transitions
        node.gain.setTargetAtTime(gainValue, now, 0.1);
      }
    });
  }

  resume() {
      if (this.context && this.context.state === 'suspended') {
          this.context.resume();
      }
  }

  // Crossfade Transition
  async transition(from: HTMLAudioElement | null, to: HTMLAudioElement, duration: number) {
      if (!this.context) this.init();
      this.resume();
      this.register(to);
      if (from) this.register(from);

      const now = this.context!.currentTime;
      const toNode = this.sources.get(to);
      const fromNode = from ? this.sources.get(from) : null;

      // 1. Fade In New Track
      if (toNode) {
          // Reset gain to 0 instantly
          toNode.inputGain.gain.cancelScheduledValues(now);
          toNode.inputGain.gain.setValueAtTime(0, now);
          
          try {
             const playPromise = to.play();
             if (playPromise) {
                 playPromise.catch(e => console.warn("AudioEngine: Play blocked", e));
             }
          } catch(e) {
              console.warn("AudioEngine: Sync play failed", e);
          }

          // Ramp to 1
          toNode.inputGain.gain.linearRampToValueAtTime(1, now + duration);
      }

      // 2. Fade Out Old Track
      if (fromNode && from) {
          fromNode.inputGain.gain.cancelScheduledValues(now);
          fromNode.inputGain.gain.setValueAtTime(1, now); 
          fromNode.inputGain.gain.linearRampToValueAtTime(0, now + duration);
          
          // Cleanup after fade
          setTimeout(() => {
              from.pause();
              from.currentTime = 0;
          }, duration * 1000 + 100); 
      }
  }
  
  // Method to check frequency data for visualizer
  getAnalyser() {
      return this.analyser;
  }

  /**
   * Get the master gain node for external audio connections.
   * Used by Butterchurn/Milkdrop to receive audio for analysis.
   * 
   * @returns {GainNode | null} The master gain node, or null if not initialized
   */
  getMasterGainNode(): GainNode | null {
      return this.masterGain;
  }

  /**
   * Get the audio context for external use.
   * Required by Butterchurn visualizer initialization.
   * 
   * @returns {AudioContext | null} The audio context, or null if not initialized
   */
  getAudioContext(): AudioContext | null {
      return this.context;
  }
}

export const audioEngine = new AudioEngine();