/**
 * ViiB MediaHub - DJ Audio Engine
 * 
 * Web Audio API-based dual-deck audio engine for DJ mixing.
 * Provides real-time audio processing including:
 * - Dual independent audio playback
 * - 3-band EQ per deck
 * - Crossfader mixing
 * - Master volume and limiter
 * - VU metering
 * 
 * @module lib/djAudio
 */

import { useStore } from '../store';
import type { DeckId, DeckState } from '../slices/djMixerSlice';
import type { Song } from '../types';

// ============================================================================
// Types
// ============================================================================

export interface DJAudioEngineConfig {
  /** Sample rate (default: 44100) */
  sampleRate?: number;
  /** Master limiter threshold in dB (default: -0.5) */
  limiterThreshold?: number;
  /** EQ frequency bands */
  eqFrequencies?: {
    low: number;   // Default: 200 Hz
    mid: number;   // Default: 1000 Hz
    high: number;  // Default: 3000 Hz
  };
}

export interface DeckAudioState {
  isLoaded: boolean;
  isPlaying: boolean;
  duration: number;
  currentTime: number;
  volume: number;
  tempo: number;
  eq: {
    low: number;
    mid: number;
    high: number;
  };
}

export interface VULevels {
  deckA: { left: number; right: number };
  deckB: { left: number; right: number };
  master: { left: number; right: number };
}

// ============================================================================
// DJ Audio Engine Class
// ============================================================================

export class DJAudioEngine {
  private audioContext: AudioContext | null = null;
  private isInitialized = false;

  // Audio elements (source)
  private audioElementA: HTMLAudioElement | null = null;
  private audioElementB: HTMLAudioElement | null = null;

  // Media element source nodes
  private sourceNodeA: MediaElementAudioSourceNode | null = null;
  private sourceNodeB: MediaElementAudioSourceNode | null = null;

  // Gain nodes for volume control
  private gainNodeA: GainNode | null = null;
  private gainNodeB: GainNode | null = null;

  // EQ filter nodes (3-band per deck)
  private eqLowA: BiquadFilterNode | null = null;
  private eqMidA: BiquadFilterNode | null = null;
  private eqHighA: BiquadFilterNode | null = null;
  private eqLowB: BiquadFilterNode | null = null;
  private eqMidB: BiquadFilterNode | null = null;
  private eqHighB: BiquadFilterNode | null = null;

  // FX nodes per deck (Phase 3)
  // Filter FX
  private filterFXA: BiquadFilterNode | null = null;
  private filterFXB: BiquadFilterNode | null = null;
  private filterFXGainA: GainNode | null = null;  // Dry/wet
  private filterFXGainB: GainNode | null = null;
  
  // Delay FX
  private delayNodeA: DelayNode | null = null;
  private delayNodeB: DelayNode | null = null;
  private delayFeedbackA: GainNode | null = null;
  private delayFeedbackB: GainNode | null = null;
  private delayWetGainA: GainNode | null = null;
  private delayWetGainB: GainNode | null = null;
  private delayDryGainA: GainNode | null = null;
  private delayDryGainB: GainNode | null = null;
  
  // Flanger FX (LFO-modulated delay)
  private flangerDelayA: DelayNode | null = null;
  private flangerDelayB: DelayNode | null = null;
  private flangerLfoA: OscillatorNode | null = null;
  private flangerLfoB: OscillatorNode | null = null;
  private flangerLfoGainA: GainNode | null = null;
  private flangerLfoGainB: GainNode | null = null;
  private flangerFeedbackA: GainNode | null = null;
  private flangerFeedbackB: GainNode | null = null;
  private flangerWetGainA: GainNode | null = null;
  private flangerWetGainB: GainNode | null = null;
  
  // Reverb FX (convolver-based)
  private reverbConvolverA: ConvolverNode | null = null;
  private reverbConvolverB: ConvolverNode | null = null;
  private reverbWetGainA: GainNode | null = null;
  private reverbWetGainB: GainNode | null = null;
  private reverbDryGainA: GainNode | null = null;
  private reverbDryGainB: GainNode | null = null;
  
  // FX bypass nodes (connect dry signal around FX)
  private fxSendA: GainNode | null = null;  // Input to FX chain
  private fxSendB: GainNode | null = null;
  private fxReturnA: GainNode | null = null;  // Output from FX chain
  private fxReturnB: GainNode | null = null;

  // Crossfader gain nodes
  private crossfaderGainA: GainNode | null = null;
  private crossfaderGainB: GainNode | null = null;

  // Master section
  private masterGain: GainNode | null = null;
  private limiter: DynamicsCompressorNode | null = null;

  // Analysers for VU meters
  private analyserA: AnalyserNode | null = null;
  private analyserB: AnalyserNode | null = null;
  private analyserMaster: AnalyserNode | null = null;

  // Configuration
  private config: Required<DJAudioEngineConfig>;

  // Animation frame for position updates
  private animationFrameId: number | null = null;
  private vuAnimationFrameId: number | null = null;

  // Callbacks
  private onPositionUpdate?: (deck: DeckId, position: number) => void;
  private onVUUpdate?: (levels: VULevels) => void;
  private onTrackEnd?: (deck: DeckId) => void;

  constructor(config?: DJAudioEngineConfig) {
    this.config = {
      sampleRate: config?.sampleRate ?? 44100,
      limiterThreshold: config?.limiterThreshold ?? -0.5,
      eqFrequencies: config?.eqFrequencies ?? {
        low: 200,
        mid: 1000,
        high: 3000,
      },
    };
  }

  // ============================================================================
  // Initialization
  // ============================================================================

  /**
   * Initialize the audio engine. Must be called after user interaction.
   */
  async initialize(): Promise<void> {
    if (this.isInitialized) return;

    try {
      // Create AudioContext
      this.audioContext = new AudioContext({
        sampleRate: this.config.sampleRate,
        latencyHint: 'interactive',
      });

      // Resume context if suspended (browser autoplay policy)
      if (this.audioContext.state === 'suspended') {
        await this.audioContext.resume();
      }

      // Create audio elements
      this.audioElementA = new Audio();
      this.audioElementB = new Audio();
      this.audioElementA.crossOrigin = 'anonymous';
      this.audioElementB.crossOrigin = 'anonymous';

      // Create source nodes from audio elements
      this.sourceNodeA = this.audioContext.createMediaElementSource(this.audioElementA);
      this.sourceNodeB = this.audioContext.createMediaElementSource(this.audioElementB);

      // Create the audio graph
      this.createAudioGraph();

      // Set up event listeners
      this.setupEventListeners();

      // Start animation loops
      this.startPositionTracking();
      this.startVUMetering();

      this.isInitialized = true;
      console.log('🎧 DJ Audio Engine initialized');
    } catch (error) {
      console.error('Failed to initialize DJ Audio Engine:', error);
      throw error;
    }
  }

  /**
   * Create the Web Audio API graph for mixing
   */
  private createAudioGraph(): void {
    if (!this.audioContext || !this.sourceNodeA || !this.sourceNodeB) return;

    const ctx = this.audioContext;

    // ========== Deck A Chain ==========
    // Gain (volume)
    this.gainNodeA = ctx.createGain();
    this.gainNodeA.gain.value = 1.0;

    // 3-Band EQ
    this.eqLowA = ctx.createBiquadFilter();
    this.eqLowA.type = 'lowshelf';
    this.eqLowA.frequency.value = this.config.eqFrequencies.low;
    this.eqLowA.gain.value = 0;

    this.eqMidA = ctx.createBiquadFilter();
    this.eqMidA.type = 'peaking';
    this.eqMidA.frequency.value = this.config.eqFrequencies.mid;
    this.eqMidA.Q.value = 1.0;
    this.eqMidA.gain.value = 0;

    this.eqHighA = ctx.createBiquadFilter();
    this.eqHighA.type = 'highshelf';
    this.eqHighA.frequency.value = this.config.eqFrequencies.high;
    this.eqHighA.gain.value = 0;

    // Crossfader gain for deck A
    this.crossfaderGainA = ctx.createGain();
    this.crossfaderGainA.gain.value = 1.0;

    // Analyser for VU meter
    this.analyserA = ctx.createAnalyser();
    this.analyserA.fftSize = 256;

    // FX Chain for Deck A
    this.createDeckFXChain('A', ctx);

    // Connect deck A chain: Source → Volume → EQ → FX → Crossfader → Analyser
    this.sourceNodeA
      .connect(this.gainNodeA)
      .connect(this.eqLowA)
      .connect(this.eqMidA)
      .connect(this.eqHighA);
    
    // Connect EQ to FX send, FX return to crossfader
    this.eqHighA.connect(this.fxSendA!);
    this.fxReturnA!.connect(this.crossfaderGainA).connect(this.analyserA);

    // ========== Deck B Chain ==========
    this.gainNodeB = ctx.createGain();
    this.gainNodeB.gain.value = 1.0;

    this.eqLowB = ctx.createBiquadFilter();
    this.eqLowB.type = 'lowshelf';
    this.eqLowB.frequency.value = this.config.eqFrequencies.low;
    this.eqLowB.gain.value = 0;

    this.eqMidB = ctx.createBiquadFilter();
    this.eqMidB.type = 'peaking';
    this.eqMidB.frequency.value = this.config.eqFrequencies.mid;
    this.eqMidB.Q.value = 1.0;
    this.eqMidB.gain.value = 0;

    this.eqHighB = ctx.createBiquadFilter();
    this.eqHighB.type = 'highshelf';
    this.eqHighB.frequency.value = this.config.eqFrequencies.high;
    this.eqHighB.gain.value = 0;

    this.crossfaderGainB = ctx.createGain();
    this.crossfaderGainB.gain.value = 1.0;

    this.analyserB = ctx.createAnalyser();
    this.analyserB.fftSize = 256;

    // FX Chain for Deck B
    this.createDeckFXChain('B', ctx);

    // Connect deck B chain: Source → Volume → EQ → FX → Crossfader → Analyser
    this.sourceNodeB
      .connect(this.gainNodeB)
      .connect(this.eqLowB)
      .connect(this.eqMidB)
      .connect(this.eqHighB);
    
    // Connect EQ to FX send, FX return to crossfader
    this.eqHighB.connect(this.fxSendB!);
    this.fxReturnB!.connect(this.crossfaderGainB).connect(this.analyserB);

    // ========== Master Section ==========
    this.masterGain = ctx.createGain();
    this.masterGain.gain.value = 1.0;

    // Limiter (dynamics compressor with extreme settings)
    this.limiter = ctx.createDynamicsCompressor();
    this.limiter.threshold.value = this.config.limiterThreshold;
    this.limiter.knee.value = 0;
    this.limiter.ratio.value = 20;
    this.limiter.attack.value = 0.003;
    this.limiter.release.value = 0.25;

    this.analyserMaster = ctx.createAnalyser();
    this.analyserMaster.fftSize = 256;

    // Connect to master
    this.analyserA.connect(this.masterGain);
    this.analyserB.connect(this.masterGain);
    this.masterGain
      .connect(this.limiter)
      .connect(this.analyserMaster)
      .connect(ctx.destination);

    console.log('🎧 Audio graph created');
  }

  /**
   * Create FX chain for a deck
   * FX are connected in parallel with dry signal for mix control
   */
  private createDeckFXChain(deck: DeckId, ctx: AudioContext): void {
    // FX Send (input to FX chain)
    const fxSend = ctx.createGain();
    fxSend.gain.value = 1.0;
    
    // FX Return (output from FX chain)
    const fxReturn = ctx.createGain();
    fxReturn.gain.value = 1.0;
    
    // Dry path (bypass)
    const dryGain = ctx.createGain();
    dryGain.gain.value = 1.0;
    fxSend.connect(dryGain).connect(fxReturn);
    
    // ========== Filter FX ==========
    const filterFX = ctx.createBiquadFilter();
    filterFX.type = 'lowpass';
    filterFX.frequency.value = 20000; // Full open by default
    filterFX.Q.value = 1;
    
    const filterGain = ctx.createGain();
    filterGain.gain.value = 0; // Disabled by default
    
    fxSend.connect(filterFX).connect(filterGain).connect(fxReturn);
    
    // ========== Delay FX ==========
    const delayNode = ctx.createDelay(2.0); // Max 2 second delay
    delayNode.delayTime.value = 0.375; // Default ~3/8 note at 120 BPM
    
    const delayFeedback = ctx.createGain();
    delayFeedback.gain.value = 0; // No feedback by default
    
    const delayWetGain = ctx.createGain();
    delayWetGain.gain.value = 0; // Disabled by default
    
    const delayDryGain = ctx.createGain();
    delayDryGain.gain.value = 1.0;
    
    // Delay with feedback loop
    fxSend.connect(delayNode).connect(delayWetGain).connect(fxReturn);
    delayNode.connect(delayFeedback).connect(delayNode); // Feedback loop
    fxSend.connect(delayDryGain).connect(fxReturn);
    
    // ========== Flanger FX ==========
    const flangerDelay = ctx.createDelay(0.02); // Max 20ms for flanger
    flangerDelay.delayTime.value = 0.003; // 3ms base delay
    
    const flangerLfo = ctx.createOscillator();
    flangerLfo.type = 'sine';
    flangerLfo.frequency.value = 0.5; // 0.5 Hz rate
    
    const flangerLfoGain = ctx.createGain();
    flangerLfoGain.gain.value = 0; // Disabled by default (modulation depth)
    
    const flangerFeedback = ctx.createGain();
    flangerFeedback.gain.value = 0;
    
    const flangerWetGain = ctx.createGain();
    flangerWetGain.gain.value = 0; // Disabled by default
    
    // Connect flanger
    flangerLfo.connect(flangerLfoGain).connect(flangerDelay.delayTime);
    fxSend.connect(flangerDelay).connect(flangerWetGain).connect(fxReturn);
    flangerDelay.connect(flangerFeedback).connect(flangerDelay);
    
    // Start LFO
    flangerLfo.start();
    
    // ========== Reverb FX (simplified - convolver) ==========
    // We'll create an impulse response programmatically for simplicity
    const reverbConvolver = ctx.createConvolver();
    this.createReverbImpulse(ctx, reverbConvolver, 0.5, 0.5);
    
    const reverbWetGain = ctx.createGain();
    reverbWetGain.gain.value = 0; // Disabled by default
    
    const reverbDryGain = ctx.createGain();
    reverbDryGain.gain.value = 1.0;
    
    fxSend.connect(reverbConvolver).connect(reverbWetGain).connect(fxReturn);
    fxSend.connect(reverbDryGain).connect(fxReturn);
    
    // Store references based on deck
    if (deck === 'A') {
      this.fxSendA = fxSend;
      this.fxReturnA = fxReturn;
      this.filterFXA = filterFX;
      this.filterFXGainA = filterGain;
      this.delayNodeA = delayNode;
      this.delayFeedbackA = delayFeedback;
      this.delayWetGainA = delayWetGain;
      this.delayDryGainA = delayDryGain;
      this.flangerDelayA = flangerDelay;
      this.flangerLfoA = flangerLfo;
      this.flangerLfoGainA = flangerLfoGain;
      this.flangerFeedbackA = flangerFeedback;
      this.flangerWetGainA = flangerWetGain;
      this.reverbConvolverA = reverbConvolver;
      this.reverbWetGainA = reverbWetGain;
      this.reverbDryGainA = reverbDryGain;
    } else {
      this.fxSendB = fxSend;
      this.fxReturnB = fxReturn;
      this.filterFXB = filterFX;
      this.filterFXGainB = filterGain;
      this.delayNodeB = delayNode;
      this.delayFeedbackB = delayFeedback;
      this.delayWetGainB = delayWetGain;
      this.delayDryGainB = delayDryGain;
      this.flangerDelayB = flangerDelay;
      this.flangerLfoB = flangerLfo;
      this.flangerLfoGainB = flangerLfoGain;
      this.flangerFeedbackB = flangerFeedback;
      this.flangerWetGainB = flangerWetGain;
      this.reverbConvolverB = reverbConvolver;
      this.reverbWetGainB = reverbWetGain;
      this.reverbDryGainB = reverbDryGain;
    }
    
    console.log(`🎧 FX chain created for Deck ${deck}`);
  }

  /**
   * Create a simple algorithmic reverb impulse response
   */
  private createReverbImpulse(
    ctx: AudioContext, 
    convolver: ConvolverNode, 
    roomSize: number, 
    damping: number
  ): void {
    const sampleRate = ctx.sampleRate;
    const length = sampleRate * (0.5 + roomSize * 2.5); // 0.5-3 seconds
    const impulse = ctx.createBuffer(2, length, sampleRate);
    
    for (let channel = 0; channel < 2; channel++) {
      const channelData = impulse.getChannelData(channel);
      for (let i = 0; i < length; i++) {
        // Exponential decay with noise
        const decay = Math.pow(1 - damping, i / sampleRate * 10);
        channelData[i] = (Math.random() * 2 - 1) * decay;
      }
    }
    
    convolver.buffer = impulse;
  }

  /**
   * Set up event listeners for audio elements
   */
  private setupEventListeners(): void {
    if (this.audioElementA) {
      this.audioElementA.addEventListener('ended', () => {
        this.onTrackEnd?.('A');
        useStore.getState().setDeckPlaying('A', false);
      });
      this.audioElementA.addEventListener('loadedmetadata', () => {
        const duration = this.audioElementA?.duration || 0;
        console.log(`🎧 djAudio: Deck A loadedmetadata, duration=${duration}`);
        useStore.getState().setDeckDuration('A', duration);
      });
    }

    if (this.audioElementB) {
      this.audioElementB.addEventListener('ended', () => {
        this.onTrackEnd?.('B');
        useStore.getState().setDeckPlaying('B', false);
      });
      this.audioElementB.addEventListener('loadedmetadata', () => {
        const duration = this.audioElementB?.duration || 0;
        console.log(`🎧 djAudio: Deck B loadedmetadata, duration=${duration}`);
        useStore.getState().setDeckDuration('B', duration);
      });
    }
  }

  // ============================================================================
  // Deck Operations
  // ============================================================================

  /**
   * Load a track to a deck
   */
  async loadTrack(deck: DeckId, track: Song): Promise<void> {
    const audioElement = deck === 'A' ? this.audioElementA : this.audioElementB;
    if (!audioElement) {
      console.error(`🎧 DJ Audio: Audio element for Deck ${deck} not available`);
      throw new Error('Audio engine not initialized');
    }

    // Stop current playback
    audioElement.pause();
    audioElement.currentTime = 0;

    // Set source - use the track's URL (should be /api/audio/{id})
    const audioUrl = track.url || `/api/audio/${track.id}`;
    console.log(`🎧 DJ Audio: Loading track to Deck ${deck}: ${track.title}, URL: ${audioUrl}`);
    audioElement.src = audioUrl;

    // Load the audio
    await new Promise<void>((resolve, reject) => {
      const timeoutId = setTimeout(() => {
        audioElement.removeEventListener('canplay', onCanPlay);
        audioElement.removeEventListener('error', onError);
        console.error(`🎧 DJ Audio: Timeout loading track to Deck ${deck}`);
        reject(new Error(`Timeout loading track: ${track.title}`));
      }, 30000); // 30 second timeout
      
      const onCanPlay = () => {
        clearTimeout(timeoutId);
        audioElement.removeEventListener('canplay', onCanPlay);
        audioElement.removeEventListener('error', onError);
        console.log(`🎧 DJ Audio: Track ready on Deck ${deck}`);
        resolve();
      };
      const onError = (e: Event) => {
        clearTimeout(timeoutId);
        audioElement.removeEventListener('canplay', onCanPlay);
        audioElement.removeEventListener('error', onError);
        const errorMsg = audioElement.error?.message || 'Unknown error';
        console.error(`🎧 DJ Audio: Error loading track to Deck ${deck}: ${errorMsg}`, e);
        reject(new Error(`Failed to load track: ${track.title} - ${errorMsg}`));
      };
      audioElement.addEventListener('canplay', onCanPlay);
      audioElement.addEventListener('error', onError);
      audioElement.load();
    });

    console.log(`🎧 Loaded track to Deck ${deck}: ${track.title}`);
  }

  /**
   * Unload a deck
   */
  unloadDeck(deck: DeckId): void {
    const audioElement = deck === 'A' ? this.audioElementA : this.audioElementB;
    if (audioElement) {
      audioElement.pause();
      audioElement.src = '';
      audioElement.currentTime = 0;
    }
  }

  /**
   * Play a deck
   */
  async play(deck: DeckId): Promise<void> {
    const audioElement = deck === 'A' ? this.audioElementA : this.audioElementB;
    if (audioElement && audioElement.src) {
      // Resume audio context if needed
      if (this.audioContext?.state === 'suspended') {
        await this.audioContext.resume();
      }
      await audioElement.play();
    }
  }

  /**
   * Pause a deck
   */
  pause(deck: DeckId): void {
    const audioElement = deck === 'A' ? this.audioElementA : this.audioElementB;
    if (audioElement) {
      audioElement.pause();
    }
  }

  /**
   * Toggle play/pause
   */
  async togglePlay(deck: DeckId): Promise<boolean> {
    const audioElement = deck === 'A' ? this.audioElementA : this.audioElementB;
    if (!audioElement || !audioElement.src) return false;

    if (audioElement.paused) {
      await this.play(deck);
      return true;
    } else {
      this.pause(deck);
      return false;
    }
  }

  /**
   * Seek to position (in seconds)
   */
  seek(deck: DeckId, position: number): void {
    const audioElement = deck === 'A' ? this.audioElementA : this.audioElementB;
    if (audioElement && audioElement.duration) {
      audioElement.currentTime = Math.max(0, Math.min(position, audioElement.duration));
    }
  }

  // ============================================================================
  // Vinyl Scratch / Jog Wheel
  // ============================================================================

  // Track scratch state per deck
  private scratchStateA: { active: boolean; baseTime: number; baseTempo: number; lastDragTime: number } | null = null;
  private scratchStateB: { active: boolean; baseTime: number; baseTempo: number; lastDragTime: number } | null = null;

  /**
   * Start scratch mode - call when user begins dragging waveform
   * Captures current state and prepares for vinyl-style scrubbing
   */
  startScratch(deck: DeckId): void {
    const audioElement = deck === 'A' ? this.audioElementA : this.audioElementB;
    if (!audioElement) return;

    const scratchState = {
      active: true,
      baseTime: audioElement.currentTime,
      baseTempo: audioElement.playbackRate,
      lastDragTime: Date.now(),
    };

    if (deck === 'A') {
      this.scratchStateA = scratchState;
    } else {
      this.scratchStateB = scratchState;
    }

    // During scratch, we control playbackRate directly based on drag velocity
    console.log(`🎛️ Scratch started on Deck ${deck} at ${scratchState.baseTime.toFixed(2)}s`);
  }

  /**
   * Update scratch position during drag
   * @param deck - Which deck
   * @param deltaTime - Time delta from drag movement (positive = forward, negative = backward)
   * @param velocity - Drag velocity (pixels per ms), used to set playback rate
   */
  updateScratch(deck: DeckId, deltaTime: number, velocity: number): void {
    const audioElement = deck === 'A' ? this.audioElementA : this.audioElementB;
    const scratchState = deck === 'A' ? this.scratchStateA : this.scratchStateB;
    
    if (!audioElement || !scratchState?.active) return;

    // Apply time delta to current position
    const newTime = Math.max(0, Math.min(
      audioElement.duration || 0,
      audioElement.currentTime + deltaTime
    ));
    audioElement.currentTime = newTime;

    // Set playback rate based on velocity
    // Positive velocity = forward scratch, negative = reverse
    // Scale velocity to reasonable playback rate range (-3 to 3)
    const scratchRate = Math.max(-3, Math.min(3, velocity * 0.1));
    
    if (Math.abs(scratchRate) > 0.05) {
      // Only change rate if there's significant movement
      audioElement.playbackRate = Math.abs(scratchRate);
      
      // Web Audio API doesn't support negative playback rates natively
      // For reverse scratch simulation, we manually seek backward on each update
      // The actual audio won't play backwards, but the position will move backwards
      if (scratchRate < 0 && !audioElement.paused) {
        audioElement.pause(); // Pause during reverse scratch to avoid forward audio
      } else if (scratchRate > 0 && audioElement.paused && scratchState.active) {
        // Resume if scratching forward
        audioElement.play().catch(() => {});
      }
    } else {
      // Minimal movement - pause audio for "holding" the record
      if (!audioElement.paused) {
        audioElement.pause();
      }
    }

    scratchState.lastDragTime = Date.now();
  }

  /**
   * End scratch mode - optionally apply momentum
   * @param deck - Which deck
   * @param finalVelocity - Final drag velocity (used for momentum)
   * @param resumePlayback - Whether to resume normal playback after scratch
   */
  endScratch(deck: DeckId, finalVelocity: number = 0, resumePlayback: boolean = true): void {
    const audioElement = deck === 'A' ? this.audioElementA : this.audioElementB;
    const scratchState = deck === 'A' ? this.scratchStateA : this.scratchStateB;
    
    if (!audioElement || !scratchState?.active) return;

    console.log(`🎛️ Scratch ended on Deck ${deck}, velocity: ${finalVelocity.toFixed(2)}, resume: ${resumePlayback}`);

    // Clear scratch state
    if (deck === 'A') {
      this.scratchStateA = null;
    } else {
      this.scratchStateB = null;
    }

    // Restore tempo to base rate (or current deck tempo setting)
    const deckState = useStore.getState();
    const targetTempo = deck === 'A' ? deckState.djDeckA.tempo : deckState.djDeckB.tempo;
    audioElement.playbackRate = targetTempo;

    if (resumePlayback) {
      // Apply momentum if there was significant final velocity
      if (Math.abs(finalVelocity) > 0.5) {
        this.applyMomentum(deck, finalVelocity);
      } else {
        // No momentum, just resume at normal tempo
        const wasPlaying = deck === 'A' ? deckState.djDeckA.isPlaying : deckState.djDeckB.isPlaying;
        if (wasPlaying) {
          audioElement.play().catch(() => {});
        }
      }
    }
  }

  /**
   * Apply momentum effect after scratch release
   * Gradually decelerates from scratch velocity to normal playback
   */
  private applyMomentum(deck: DeckId, initialVelocity: number): void {
    const audioElement = deck === 'A' ? this.audioElementA : this.audioElementB;
    if (!audioElement) return;

    const deckState = useStore.getState();
    const targetTempo = deck === 'A' ? deckState.djDeckA.tempo : deckState.djDeckB.tempo;
    const wasPlaying = deck === 'A' ? deckState.djDeckA.isPlaying : deckState.djDeckB.isPlaying;

    // Start with velocity-based rate, decay to target tempo
    let currentRate = Math.max(0.1, Math.min(2, Math.abs(initialVelocity) * 0.1));
    const decayDuration = 300; // ms
    const startTime = Date.now();

    // Resume playback for momentum effect
    if (wasPlaying && audioElement.paused) {
      audioElement.play().catch(() => {});
    }

    const decay = () => {
      const elapsed = Date.now() - startTime;
      const progress = Math.min(1, elapsed / decayDuration);
      
      // Ease-out decay from current rate to target tempo
      const easedProgress = 1 - Math.pow(1 - progress, 3);
      const newRate = currentRate + (targetTempo - currentRate) * easedProgress;
      
      audioElement.playbackRate = newRate;

      if (progress < 1) {
        requestAnimationFrame(decay);
      } else {
        audioElement.playbackRate = targetTempo;
      }
    };

    requestAnimationFrame(decay);
  }

  /**
   * Check if deck is currently in scratch mode
   */
  isScratching(deck: DeckId): boolean {
    const scratchState = deck === 'A' ? this.scratchStateA : this.scratchStateB;
    return scratchState?.active ?? false;
  }

  /**
   * Get current position (in seconds)
   */
  getPosition(deck: DeckId): number {
    const audioElement = deck === 'A' ? this.audioElementA : this.audioElementB;
    return audioElement?.currentTime || 0;
  }

  /**
   * Get duration (in seconds)
   */
  getDuration(deck: DeckId): number {
    const audioElement = deck === 'A' ? this.audioElementA : this.audioElementB;
    return audioElement?.duration || 0;
  }

  // ============================================================================
  // Volume & Mixing
  // ============================================================================

  /**
   * Set deck volume (0-1)
   */
  setVolume(deck: DeckId, volume: number): void {
    const gainNode = deck === 'A' ? this.gainNodeA : this.gainNodeB;
    if (gainNode) {
      gainNode.gain.setValueAtTime(
        Math.max(0, Math.min(1, volume)),
        this.audioContext?.currentTime || 0
      );
    }
  }

  /**
   * Set crossfader position (-1 = full A, 0 = center, 1 = full B)
   * Uses constant-power crossfade curve
   */
  setCrossfader(position: number): void {
    if (!this.crossfaderGainA || !this.crossfaderGainB || !this.audioContext) return;

    // Clamp position to -1 to 1
    const pos = Math.max(-1, Math.min(1, position));

    // Constant-power crossfade
    // At position -1: A=1, B=0
    // At position 0: A=0.707, B=0.707
    // At position 1: A=0, B=1
    const gainA = Math.cos((pos + 1) * Math.PI / 4);
    const gainB = Math.cos((1 - pos) * Math.PI / 4);

    const now = this.audioContext.currentTime;
    this.crossfaderGainA.gain.setValueAtTime(gainA, now);
    this.crossfaderGainB.gain.setValueAtTime(gainB, now);
  }

  /**
   * Set master volume (0-1)
   */
  setMasterVolume(volume: number): void {
    if (this.masterGain && this.audioContext) {
      this.masterGain.gain.setValueAtTime(
        Math.max(0, Math.min(1, volume)),
        this.audioContext.currentTime
      );
    }
  }

  // ============================================================================
  // EQ
  // ============================================================================

  /**
   * Set EQ band gain for a deck
   * @param deck - Deck ID
   * @param band - EQ band ('low', 'mid', 'high')
   * @param gain - Gain in dB (-24 to +12, -Infinity for kill)
   */
  setEQ(deck: DeckId, band: 'low' | 'mid' | 'high', gain: number): void {
    let eqNode: BiquadFilterNode | null = null;

    if (deck === 'A') {
      eqNode = band === 'low' ? this.eqLowA : band === 'mid' ? this.eqMidA : this.eqHighA;
    } else {
      eqNode = band === 'low' ? this.eqLowB : band === 'mid' ? this.eqMidB : this.eqHighB;
    }

    if (eqNode && this.audioContext) {
      // Clamp gain to reasonable range
      const clampedGain = Math.max(-24, Math.min(12, gain));
      eqNode.gain.setValueAtTime(clampedGain, this.audioContext.currentTime);
    }
  }

  // ============================================================================
  // Tempo (Playback Rate)
  // ============================================================================

  /**
   * Set tempo/playback rate (0.5 to 1.5)
   * Note: This changes pitch along with tempo (no time-stretching)
   */
  setTempo(deck: DeckId, tempo: number): void {
    const audioElement = deck === 'A' ? this.audioElementA : this.audioElementB;
    if (audioElement) {
      audioElement.playbackRate = Math.max(0.5, Math.min(1.5, tempo));
    }
  }

  // ============================================================================
  // Effects (Phase 3)
  // ============================================================================

  /**
   * Set Filter FX parameters
   */
  setFilterFX(
    deck: DeckId, 
    enabled: boolean, 
    type: 'lowpass' | 'highpass', 
    frequency: number, 
    resonance: number
  ): void {
    const filterFX = deck === 'A' ? this.filterFXA : this.filterFXB;
    const filterGain = deck === 'A' ? this.filterFXGainA : this.filterFXGainB;
    
    if (!filterFX || !filterGain || !this.audioContext) return;
    
    const now = this.audioContext.currentTime;
    
    filterFX.type = type;
    filterFX.frequency.setValueAtTime(Math.max(20, Math.min(20000, frequency)), now);
    filterFX.Q.setValueAtTime(Math.max(0.1, Math.min(30, resonance)), now);
    filterGain.gain.setValueAtTime(enabled ? 1 : 0, now);
    
    console.log(`🎛️ Filter FX ${deck}: ${enabled ? 'ON' : 'OFF'}, ${type}, freq=${frequency}Hz, Q=${resonance}`);
  }

  /**
   * Set Delay FX parameters
   */
  setDelayFX(
    deck: DeckId, 
    enabled: boolean, 
    time: number, 
    feedback: number, 
    mix: number
  ): void {
    const delayNode = deck === 'A' ? this.delayNodeA : this.delayNodeB;
    const delayFeedback = deck === 'A' ? this.delayFeedbackA : this.delayFeedbackB;
    const delayWetGain = deck === 'A' ? this.delayWetGainA : this.delayWetGainB;
    const delayDryGain = deck === 'A' ? this.delayDryGainA : this.delayDryGainB;
    
    if (!delayNode || !delayFeedback || !delayWetGain || !delayDryGain || !this.audioContext) return;
    
    const now = this.audioContext.currentTime;
    
    delayNode.delayTime.setValueAtTime(Math.max(0.01, Math.min(2, time)), now);
    delayFeedback.gain.setValueAtTime(Math.max(0, Math.min(0.95, feedback)), now);
    
    if (enabled) {
      delayWetGain.gain.setValueAtTime(mix, now);
      delayDryGain.gain.setValueAtTime(1 - mix * 0.5, now); // Don't fully cut dry signal
    } else {
      delayWetGain.gain.setValueAtTime(0, now);
      delayDryGain.gain.setValueAtTime(1, now);
    }
    
    console.log(`🎛️ Delay FX ${deck}: ${enabled ? 'ON' : 'OFF'}, time=${time}s, feedback=${feedback}, mix=${mix}`);
  }

  /**
   * Set Flanger FX parameters
   */
  setFlangerFX(
    deck: DeckId, 
    enabled: boolean, 
    rate: number, 
    depth: number, 
    feedback: number
  ): void {
    const flangerLfo = deck === 'A' ? this.flangerLfoA : this.flangerLfoB;
    const flangerLfoGain = deck === 'A' ? this.flangerLfoGainA : this.flangerLfoGainB;
    const flangerFeedback = deck === 'A' ? this.flangerFeedbackA : this.flangerFeedbackB;
    const flangerWetGain = deck === 'A' ? this.flangerWetGainA : this.flangerWetGainB;
    
    if (!flangerLfo || !flangerLfoGain || !flangerFeedback || !flangerWetGain || !this.audioContext) return;
    
    const now = this.audioContext.currentTime;
    
    flangerLfo.frequency.setValueAtTime(Math.max(0.1, Math.min(10, rate)), now);
    flangerLfoGain.gain.setValueAtTime(depth * 0.005, now); // Scale depth to delay time modulation
    flangerFeedback.gain.setValueAtTime(Math.max(0, Math.min(0.95, feedback)), now);
    flangerWetGain.gain.setValueAtTime(enabled ? 0.5 : 0, now);
    
    console.log(`🎛️ Flanger FX ${deck}: ${enabled ? 'ON' : 'OFF'}, rate=${rate}Hz, depth=${depth}, feedback=${feedback}`);
  }

  /**
   * Set Reverb FX parameters
   */
  setReverbFX(
    deck: DeckId, 
    enabled: boolean, 
    roomSize: number, 
    damping: number, 
    mix: number
  ): void {
    const reverbConvolver = deck === 'A' ? this.reverbConvolverA : this.reverbConvolverB;
    const reverbWetGain = deck === 'A' ? this.reverbWetGainA : this.reverbWetGainB;
    const reverbDryGain = deck === 'A' ? this.reverbDryGainA : this.reverbDryGainB;
    
    if (!reverbConvolver || !reverbWetGain || !reverbDryGain || !this.audioContext) return;
    
    const now = this.audioContext.currentTime;
    
    // Regenerate impulse response with new parameters
    this.createReverbImpulse(this.audioContext, reverbConvolver, roomSize, damping);
    
    if (enabled) {
      reverbWetGain.gain.setValueAtTime(mix, now);
      reverbDryGain.gain.setValueAtTime(1 - mix * 0.5, now);
    } else {
      reverbWetGain.gain.setValueAtTime(0, now);
      reverbDryGain.gain.setValueAtTime(1, now);
    }
    
    console.log(`🎛️ Reverb FX ${deck}: ${enabled ? 'ON' : 'OFF'}, room=${roomSize}, damp=${damping}, mix=${mix}`);
  }

  // ============================================================================
  // Loop Controls
  // ============================================================================

  /**
   * Set loop points for a deck
   */
  setLoop(deck: DeckId, startTime: number, endTime: number): void {
    const audioElement = deck === 'A' ? this.audioElementA : this.audioElementB;
    if (!audioElement) return;
    
    // Validate loop points
    if (startTime >= endTime || startTime < 0 || endTime > audioElement.duration) {
      console.warn(`⚠️ Invalid loop points for deck ${deck}: ${startTime} - ${endTime}`);
      return;
    }
    
    // Update store
    useStore.getState().setLoop(deck, startTime, endTime);
    console.log(`🔁 Loop set for deck ${deck}: ${startTime.toFixed(2)}s - ${endTime.toFixed(2)}s`);
  }

  /**
   * Toggle loop on/off for a deck
   */
  toggleLoop(deck: DeckId): void {
    useStore.getState().toggleLoop(deck);
    const deckState = deck === 'A' ? useStore.getState().djDeckA : useStore.getState().djDeckB;
    console.log(`🔁 Loop ${deck}: ${deckState.loop.enabled ? 'ON' : 'OFF'}`);
  }

  /**
   * Clear loop for a deck
   */
  clearLoop(deck: DeckId): void {
    useStore.getState().clearLoop(deck);
    console.log(`🔁 Loop cleared for deck ${deck}`);
  }

  /**
   * Set loop-in point (start of loop at current position)
   */
  setLoopIn(deck: DeckId): void {
    const audioElement = deck === 'A' ? this.audioElementA : this.audioElementB;
    if (!audioElement) return;
    
    const position = audioElement.currentTime;
    const deckState = deck === 'A' ? useStore.getState().djDeckA : useStore.getState().djDeckB;
    
    // If we already have a loop end point, validate
    if (deckState.loop.end > 0 && position < deckState.loop.end) {
      useStore.getState().setLoop(deck, position, deckState.loop.end);
    } else {
      // Just set the start point, keep end at 0 (will be set by setLoopOut)
      useStore.getState().setLoop(deck, position, deckState.loop.end || position + 4);
    }
    console.log(`🔁 Loop IN set for deck ${deck}: ${position.toFixed(2)}s`);
  }

  /**
   * Set loop-out point (end of loop at current position) and enable loop
   */
  setLoopOut(deck: DeckId): void {
    const audioElement = deck === 'A' ? this.audioElementA : this.audioElementB;
    if (!audioElement) return;
    
    const position = audioElement.currentTime;
    const deckState = deck === 'A' ? useStore.getState().djDeckA : useStore.getState().djDeckB;
    
    // Validate against start point
    if (position > deckState.loop.start) {
      useStore.getState().setLoop(deck, deckState.loop.start, position);
      // Enable loop when out point is set
      if (!deckState.loop.enabled) {
        useStore.getState().toggleLoop(deck);
      }
    } else {
      console.warn(`⚠️ Loop OUT must be after loop IN point`);
    }
    console.log(`🔁 Loop OUT set for deck ${deck}: ${position.toFixed(2)}s`);
  }

  /**
   * Set a beat-synced loop of specified length (in beats)
   */
  setLoopBeats(deck: DeckId, beats: number): void {
    const audioElement = deck === 'A' ? this.audioElementA : this.audioElementB;
    if (!audioElement) return;
    
    const deckState = deck === 'A' ? useStore.getState().djDeckA : useStore.getState().djDeckB;
    const bpm = deckState.effectiveBpm || deckState.originalBpm;
    
    if (!bpm) {
      // Fallback to time-based loop if no BPM
      const loopSeconds = beats * 0.5; // Assume 120 BPM
      const start = audioElement.currentTime;
      const end = Math.min(start + loopSeconds, audioElement.duration);
      this.setLoop(deck, start, end);
      return;
    }
    
    // Calculate loop length based on BPM
    const beatDuration = 60 / bpm;
    const loopDuration = beats * beatDuration;
    const start = audioElement.currentTime;
    const end = Math.min(start + loopDuration, audioElement.duration);
    
    this.setLoop(deck, start, end);
    
    // Enable the loop
    if (!deckState.loop.enabled) {
      useStore.getState().toggleLoop(deck);
    }
    console.log(`🔁 ${beats}-beat loop set for deck ${deck}: ${loopDuration.toFixed(2)}s @ ${bpm.toFixed(1)} BPM`);
  }

  /**
   * Double the current loop length
   */
  doubleLoop(deck: DeckId): void {
    const deckState = deck === 'A' ? useStore.getState().djDeckA : useStore.getState().djDeckB;
    const audioElement = deck === 'A' ? this.audioElementA : this.audioElementB;
    if (!audioElement || !deckState.loop.enabled) return;
    
    const currentLength = deckState.loop.end - deckState.loop.start;
    const newEnd = Math.min(deckState.loop.start + currentLength * 2, audioElement.duration);
    
    useStore.getState().setLoop(deck, deckState.loop.start, newEnd);
    console.log(`🔁 Loop doubled for deck ${deck}: ${(newEnd - deckState.loop.start).toFixed(2)}s`);
  }

  /**
   * Halve the current loop length
   */
  halveLoop(deck: DeckId): void {
    const deckState = deck === 'A' ? useStore.getState().djDeckA : useStore.getState().djDeckB;
    if (!deckState.loop.enabled) return;
    
    const currentLength = deckState.loop.end - deckState.loop.start;
    const minLength = 0.1; // Minimum 100ms loop
    const newEnd = deckState.loop.start + Math.max(currentLength / 2, minLength);
    
    useStore.getState().setLoop(deck, deckState.loop.start, newEnd);
    console.log(`🔁 Loop halved for deck ${deck}: ${(newEnd - deckState.loop.start).toFixed(2)}s`);
  }

  // ============================================================================
  // Position Tracking
  // ============================================================================

  private startPositionTracking(): void {
    const updatePositions = () => {
      const storeState = useStore.getState();
      
      // Deck A position and loop handling
      if (this.audioElementA && !this.audioElementA.paused) {
        const currentTime = this.audioElementA.currentTime;
        this.onPositionUpdate?.('A', currentTime);
        storeState.setDeckPosition('A', currentTime);
        
        // Check for loop
        const loopA = storeState.djDeckA.loop;
        if (loopA.enabled && loopA.end > loopA.start) {
          if (currentTime >= loopA.end) {
            this.audioElementA.currentTime = loopA.start;
          }
        }
      }
      
      // Deck B position and loop handling
      if (this.audioElementB && !this.audioElementB.paused) {
        const currentTime = this.audioElementB.currentTime;
        this.onPositionUpdate?.('B', currentTime);
        storeState.setDeckPosition('B', currentTime);
        
        // Check for loop
        const loopB = storeState.djDeckB.loop;
        if (loopB.enabled && loopB.end > loopB.start) {
          if (currentTime >= loopB.end) {
            this.audioElementB.currentTime = loopB.start;
          }
        }
      }
      
      this.animationFrameId = requestAnimationFrame(updatePositions);
    };
    this.animationFrameId = requestAnimationFrame(updatePositions);
  }

  // ============================================================================
  // VU Metering
  // ============================================================================

  private startVUMetering(): void {
    const bufferLengthA = this.analyserA?.frequencyBinCount || 128;
    const bufferLengthB = this.analyserB?.frequencyBinCount || 128;
    const bufferLengthMaster = this.analyserMaster?.frequencyBinCount || 128;
    
    const dataArrayA = new Uint8Array(bufferLengthA);
    const dataArrayB = new Uint8Array(bufferLengthB);
    const dataArrayMaster = new Uint8Array(bufferLengthMaster);

    const updateVU = () => {
      const levels: VULevels = {
        deckA: { left: 0, right: 0 },
        deckB: { left: 0, right: 0 },
        master: { left: 0, right: 0 },
      };

      // Get levels from analysers
      if (this.analyserA) {
        this.analyserA.getByteFrequencyData(dataArrayA);
        const avg = this.getAverageLevel(dataArrayA);
        levels.deckA = { left: avg, right: avg }; // Mono for now
      }

      if (this.analyserB) {
        this.analyserB.getByteFrequencyData(dataArrayB);
        const avg = this.getAverageLevel(dataArrayB);
        levels.deckB = { left: avg, right: avg };
      }

      if (this.analyserMaster) {
        this.analyserMaster.getByteFrequencyData(dataArrayMaster);
        const avg = this.getAverageLevel(dataArrayMaster);
        levels.master = { left: avg, right: avg };
      }

      this.onVUUpdate?.(levels);
      this.vuAnimationFrameId = requestAnimationFrame(updateVU);
    };

    this.vuAnimationFrameId = requestAnimationFrame(updateVU);
  }

  private getAverageLevel(dataArray: Uint8Array): number {
    let sum = 0;
    for (let i = 0; i < dataArray.length; i++) {
      sum += dataArray[i];
    }
    return (sum / dataArray.length) / 255; // Normalize to 0-1
  }

  // ============================================================================
  // Callbacks
  // ============================================================================

  setOnPositionUpdate(callback: (deck: DeckId, position: number) => void): void {
    this.onPositionUpdate = callback;
  }

  setOnVUUpdate(callback: (levels: VULevels) => void): void {
    this.onVUUpdate = callback;
  }

  setOnTrackEnd(callback: (deck: DeckId) => void): void {
    this.onTrackEnd = callback;
  }

  // ============================================================================
  // Cleanup
  // ============================================================================

  /**
   * Clean up and release resources
   */
  dispose(): void {
    // Stop animation loops
    if (this.animationFrameId) {
      cancelAnimationFrame(this.animationFrameId);
    }
    if (this.vuAnimationFrameId) {
      cancelAnimationFrame(this.vuAnimationFrameId);
    }

    // Stop and clean up audio elements
    if (this.audioElementA) {
      this.audioElementA.pause();
      this.audioElementA.src = '';
    }
    if (this.audioElementB) {
      this.audioElementB.pause();
      this.audioElementB.src = '';
    }

    // Close audio context
    if (this.audioContext) {
      this.audioContext.close();
    }

    this.isInitialized = false;
    console.log('🎧 DJ Audio Engine disposed');
  }

  // ============================================================================
  // State Getters
  // ============================================================================

  get initialized(): boolean {
    return this.isInitialized;
  }

  isPlaying(deck: DeckId): boolean {
    const audioElement = deck === 'A' ? this.audioElementA : this.audioElementB;
    return audioElement ? !audioElement.paused : false;
  }

  isLoaded(deck: DeckId): boolean {
    const audioElement = deck === 'A' ? this.audioElementA : this.audioElementB;
    return audioElement ? !!audioElement.src && audioElement.readyState >= 2 : false;
  }
}

// ============================================================================
// Singleton Instance
// ============================================================================

let djAudioEngine: DJAudioEngine | null = null;

/**
 * Get the singleton DJ Audio Engine instance
 */
export function getDJAudioEngine(): DJAudioEngine {
  if (!djAudioEngine) {
    djAudioEngine = new DJAudioEngine();
  }
  return djAudioEngine;
}

/**
 * Dispose the singleton DJ Audio Engine
 */
export function disposeDJAudioEngine(): void {
  if (djAudioEngine) {
    djAudioEngine.dispose();
    djAudioEngine = null;
  }
}

export default DJAudioEngine;
