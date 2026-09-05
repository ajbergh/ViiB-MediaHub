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
import type { BeatFXTarget, BeatFXType, BeatFraction, DeckId, DeckState } from '../slices/djMixerSlice';
import type { Song } from '../types';

// Debug flag - set to false for production to reduce console overhead
const DJ_DEBUG = false;

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
  private filterFXGainA: GainNode | null = null;  // Wet gain (filtered signal)
  private filterFXGainB: GainNode | null = null;
  private filterDryGainA: GainNode | null = null;  // Dry gain (unfiltered signal)
  private filterDryGainB: GainNode | null = null;
  
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
  
  // Reverb parameter cache (to avoid regenerating impulse response unnecessarily)
  private reverbParamsA: { roomSize: number; damping: number } = { roomSize: -1, damping: -1 };
  private reverbParamsB: { roomSize: number; damping: number } = { roomSize: -1, damping: -1 };
  
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

  // Central Beat FX chain for master-targeted effects (Phase 6)
  private masterBeatFXInput: GainNode | null = null;
  private masterBeatFXDryGain: GainNode | null = null;
  private masterBeatFXOutput: GainNode | null = null;
  private masterBeatFXDelay: DelayNode | null = null;
  private masterBeatFXDelayFeedback: GainNode | null = null;
  private masterBeatFXDelayWetGain: GainNode | null = null;
  private masterBeatFXFilter: BiquadFilterNode | null = null;
  private masterBeatFXFilterWetGain: GainNode | null = null;
  private masterBeatFXFlangerDelay: DelayNode | null = null;
  private masterBeatFXFlangerLfo: OscillatorNode | null = null;
  private masterBeatFXFlangerLfoGain: GainNode | null = null;
  private masterBeatFXFlangerFeedback: GainNode | null = null;
  private masterBeatFXFlangerWetGain: GainNode | null = null;
  private masterBeatFXReverb: ConvolverNode | null = null;
  private masterBeatFXReverbWetGain: GainNode | null = null;
  private lastBeatFXTarget: BeatFXTarget | null = null;
  private lastBeatFXType: BeatFXType | null = null;

  // Analysers for VU meters
  private analyserA: AnalyserNode | null = null;
  private analyserB: AnalyserNode | null = null;
  private analyserMaster: AnalyserNode | null = null;

  // Headphone cue nodes (Phase 4)
  private headphoneCueGainA: GainNode | null = null;  // Deck A cue send
  private headphoneCueGainB: GainNode | null = null;  // Deck B cue send
  private headphoneCueMix: GainNode | null = null;    // Mixed cue signal
  private headphoneMasterMix: GainNode | null = null; // Master signal to headphones
  private headphoneMixer: GainNode | null = null;     // Final headphone output
  private cueEnabledA = false;
  private cueEnabledB = false;
  private masterCueEnabled = false;
  private headphoneMixValue = 0.5;  // 0 = cue only, 1 = master only

  // Output device routing (Phase 4)
  private mainOutputDeviceId: string = '';       // Empty = default
  private headphoneOutputDeviceId: string = '';  // Empty = default
  private headphoneOutputActive = false;         // Whether headphone stream is currently playing
  private mainOutputStreamDestination: MediaStreamAudioDestinationNode | null = null;
  private mainOutputAudioElement: HTMLAudioElement | null = null;
  private mainOutputElementRoutingEnabled = false;
  private masterDirectOutputConnected = false;
  private headphoneStreamDestination: MediaStreamAudioDestinationNode | null = null;
  private headphoneContext: AudioContext | null = null;
  private headphoneStreamSource: MediaStreamAudioSourceNode | null = null;
  private headphoneAudioElement: HTMLAudioElement | null = null;  // Fallback for routing

  // Configuration
  private config: Required<DJAudioEngineConfig>;

  // Animation frame for position updates
  private animationFrameId: number | null = null;
  private vuAnimationFrameId: number | null = null;
  
  private trackLoadGenerationA = 0;
  private trackLoadGenerationB = 0;

  // Callbacks
  private onTrackEnd: ((deck: DeckId) => void) | null = null;
  private onPositionUpdate: ((deck: DeckId, position: number) => void) | null = null;
  private onVUUpdate: ((levels: VULevels) => void) | null = null;

  // Position tracking state
  private readonly POSITION_UPDATE_INTERVAL = 50; // ms between position callbacks
  private lastPositionUpdateA = 0;
  private lastPositionUpdateB = 0;
  private positionIdleTimer: ReturnType<typeof setTimeout> | null = null;

  // VU metering state
  private vuLevels: VULevels = { deckA: { left: 0, right: 0 }, deckB: { left: 0, right: 0 }, master: { left: 0, right: 0 } };
  private vuIdleTimer: ReturnType<typeof setTimeout> | null = null;

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
      // WebKit historically exposed this as webkitAudioContext. Modern
      // WKWebView uses AudioContext, but keep the native desktop fallback.
      const AudioContextCtor = window.AudioContext
        || (window as typeof window & { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
      if (!AudioContextCtor) {
        throw new Error('Web Audio is not supported by this browser or WebView.');
      }
      this.audioContext = new AudioContextCtor({
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

      // Apply saved master device after the graph exists so media-element
      // fallback routing can attach to the master analyser if needed.
      if (this.mainOutputDeviceId) {
        try {
          await this.setMainOutputDevice(this.mainOutputDeviceId);
        } catch (error) {
          console.warn('Failed to apply saved main output device:', error);
        }
      }

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

    // Connect to master. Keep the live master bus direct; master Beat FX is
    // mapped to both decks until the dedicated post-mix chain is verified.
    this.analyserA.connect(this.masterGain);
    this.analyserB.connect(this.masterGain);
    this.masterGain
      .connect(this.limiter)
      .connect(this.analyserMaster)
      .connect(ctx.destination);
    this.masterDirectOutputConnected = true;

    // ========== Headphone Cue Section (Phase 4) ==========
    // Create cue send gains for each deck (taps signal after EQ, before crossfader)
    this.headphoneCueGainA = ctx.createGain();
    this.headphoneCueGainA.gain.value = 0;  // CUE disabled by default
    
    this.headphoneCueGainB = ctx.createGain();
    this.headphoneCueGainB.gain.value = 0;  // CUE disabled by default
    
    // Mix node for cue signals
    this.headphoneCueMix = ctx.createGain();
    this.headphoneCueMix.gain.value = 1.0;
    
    // Master signal to headphones. Gated by masterCueEnabled.
    this.headphoneMasterMix = ctx.createGain();
    this.headphoneMasterMix.gain.value = 0;
    
    // Final headphone output
    this.headphoneMixer = ctx.createGain();
    this.headphoneMixer.gain.value = 1.0;  // Headphone volume
    
    // Connect deck signals to cue sends (tap from after FX return, before crossfader)
    this.fxReturnA!.connect(this.headphoneCueGainA);
    this.fxReturnB!.connect(this.headphoneCueGainB);
    
    // Mix cue signals
    this.headphoneCueGainA.connect(this.headphoneCueMix);
    this.headphoneCueGainB.connect(this.headphoneCueMix);
    
    // Connect master to headphone master mix
    this.analyserMaster.connect(this.headphoneMasterMix);
    
    // Mix cue and master into headphone output
    this.headphoneCueMix.connect(this.headphoneMixer);
    this.headphoneMasterMix.connect(this.headphoneMixer);
    
    // ========== Headphone Output Device Routing ==========
    // Create a MediaStreamDestination to capture headphone audio
    // This allows routing to a separate audio device via HTMLAudioElement.setSinkId()
    this.headphoneStreamDestination = ctx.createMediaStreamDestination();
    this.headphoneMixer.connect(this.headphoneStreamDestination);
    
    // We use a secondary AudioContext to route the headphone stream.
    // This bypasses HTMLMediaElement quirks with setSinkId on MediaStreams,
    // especially when the primary AudioContext is routed to a different device.
    if (typeof (AudioContext.prototype as AudioContext & { setSinkId?: unknown }).setSinkId === 'function') {
      this.headphoneContext = new AudioContext({
        sampleRate: this.config.sampleRate,
        latencyHint: 'interactive',
      });
      // Context will be suspended initially; we resume it in ensureHeadphoneOutputPlaying
      this.headphoneStreamSource = this.headphoneContext.createMediaStreamSource(this.headphoneStreamDestination.stream);
      this.headphoneStreamSource.connect(this.headphoneContext.destination);
    } else {
      // Fallback to HTMLAudioElement if AudioContext.setSinkId is not available
      this.headphoneAudioElement = new Audio();
      this.headphoneAudioElement.style.display = 'none';
      this.headphoneAudioElement.id = 'dj-headphone-audio';
      if (!document.getElementById('dj-headphone-audio')) {
        document.body.appendChild(this.headphoneAudioElement);
      }
      this.headphoneAudioElement.srcObject = this.headphoneStreamDestination.stream;
      this.headphoneAudioElement.volume = 1.0;
      this.headphoneAudioElement.autoplay = true;
    }
    
    // Start the headphone output stream. With the corrected updateHeadphoneMix logic,
    // master bleed is prevented by default (masterCueEnabled gates the master send).
    // Starting the stream unconditionally is safe: cue sends start at gain=0 and the
    // master send is at 0 unless masterCueEnabled is explicitly turned on.
    void this.ensureHeadphoneOutputPlaying();
    
    // Apply saved headphone device if set
    if (this.headphoneOutputDeviceId) {
      void this.setHeadphoneOutputDevice(this.headphoneOutputDeviceId).catch(error => {
        console.warn('Failed to apply saved headphone output device:', error);
      });
    }
    
    // Set initial mix to 0 (cue-only). Cue deck gains start at 0 so headphone
    // output is silent until the user enables CUE on a deck.
    this.updateHeadphoneMix(0);

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
    
    // ========== Filter FX ==========
    // Filter replaces the dry signal when enabled (not additive)
    const filterFX = ctx.createBiquadFilter();
    filterFX.type = 'lowpass';
    filterFX.frequency.value = 1000; // Start at midpoint
    filterFX.Q.value = 1;
    
    // Wet path (filtered signal)
    const filterGain = ctx.createGain();
    filterGain.gain.value = 0; // Disabled by default (wet = 0)
    
    // Dry path (unfiltered signal) - this IS the main signal path
    const filterDryGain = ctx.createGain();
    filterDryGain.gain.value = 1.0; // Enabled by default (dry = 1)
    
    // Both filter wet and dry go to fxReturn
    fxSend.connect(filterFX).connect(filterGain).connect(fxReturn);
    fxSend.connect(filterDryGain).connect(fxReturn);
    
    // ========== Delay FX ==========
    // Delay is additive (adds echoes to the signal)
    const delayNode = ctx.createDelay(2.0); // Max 2 second delay
    delayNode.delayTime.value = 0.375; // Default ~3/8 note at 120 BPM
    
    const delayFeedback = ctx.createGain();
    delayFeedback.gain.value = 0; // No feedback by default
    
    const delayWetGain = ctx.createGain();
    delayWetGain.gain.value = 0; // Disabled by default
    
    const delayDryGain = ctx.createGain();
    delayDryGain.gain.value = 1.0;
    
    // Delay with feedback loop - signal goes from fxSend, wet output added to fxReturn
    fxSend.connect(delayNode).connect(delayWetGain).connect(fxReturn);
    delayNode.connect(delayFeedback).connect(delayNode); // Feedback loop
    // Note: delayDryGain controls the original signal level, managed in setDelayFX
    
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
    // Reverb is additive (only adds wet signal)
    const reverbConvolver = ctx.createConvolver();
    this.createReverbImpulse(ctx, reverbConvolver, 0.5, 0.5);
    
    const reverbWetGain = ctx.createGain();
    reverbWetGain.gain.value = 0; // Disabled by default
    
    const reverbDryGain = ctx.createGain();
    reverbDryGain.gain.value = 1.0; // Kept for compatibility but not used
    
    // Reverb wet goes to fxReturn (additive)
    fxSend.connect(reverbConvolver).connect(reverbWetGain).connect(fxReturn);
    // Note: no separate dry path for reverb - filter handles the main signal path
    
    // Store references based on deck
    if (deck === 'A') {
      this.fxSendA = fxSend;
      this.fxReturnA = fxReturn;
      this.filterFXA = filterFX;
      this.filterFXGainA = filterGain;
      this.filterDryGainA = filterDryGain;
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
      this.filterDryGainB = filterDryGain;
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
   * Create the centralized Beat FX chain for master-targeted effects.
   * Deck A/B Beat FX can reuse deck FX nodes, but master needs its own
   * post-mix, pre-limiter processing path.
   */
  private createMasterBeatFXChain(ctx: AudioContext): void {
    this.masterBeatFXInput = ctx.createGain();
    this.masterBeatFXDryGain = ctx.createGain();
    this.masterBeatFXOutput = ctx.createGain();
    this.masterBeatFXDryGain.gain.value = 1;

    this.masterBeatFXInput.connect(this.masterBeatFXDryGain).connect(this.masterBeatFXOutput);

    this.masterBeatFXDelay = ctx.createDelay(2.0);
    this.masterBeatFXDelay.delayTime.value = 0.5;
    this.masterBeatFXDelayFeedback = ctx.createGain();
    this.masterBeatFXDelayFeedback.gain.value = 0;
    this.masterBeatFXDelayWetGain = ctx.createGain();
    this.masterBeatFXDelayWetGain.gain.value = 0;
    this.masterBeatFXInput
      .connect(this.masterBeatFXDelay)
      .connect(this.masterBeatFXDelayWetGain)
      .connect(this.masterBeatFXOutput);
    this.masterBeatFXDelay
      .connect(this.masterBeatFXDelayFeedback)
      .connect(this.masterBeatFXDelay);

    this.masterBeatFXFilter = ctx.createBiquadFilter();
    this.masterBeatFXFilter.type = 'lowpass';
    this.masterBeatFXFilter.frequency.value = 20000;
    this.masterBeatFXFilter.Q.value = 1;
    this.masterBeatFXFilterWetGain = ctx.createGain();
    this.masterBeatFXFilterWetGain.gain.value = 0;
    this.masterBeatFXInput
      .connect(this.masterBeatFXFilter)
      .connect(this.masterBeatFXFilterWetGain)
      .connect(this.masterBeatFXOutput);

    this.masterBeatFXFlangerDelay = ctx.createDelay(0.02);
    this.masterBeatFXFlangerDelay.delayTime.value = 0.003;
    this.masterBeatFXFlangerLfo = ctx.createOscillator();
    this.masterBeatFXFlangerLfo.type = 'sine';
    this.masterBeatFXFlangerLfo.frequency.value = 0.5;
    this.masterBeatFXFlangerLfoGain = ctx.createGain();
    this.masterBeatFXFlangerLfoGain.gain.value = 0;
    this.masterBeatFXFlangerFeedback = ctx.createGain();
    this.masterBeatFXFlangerFeedback.gain.value = 0;
    this.masterBeatFXFlangerWetGain = ctx.createGain();
    this.masterBeatFXFlangerWetGain.gain.value = 0;
    this.masterBeatFXFlangerLfo
      .connect(this.masterBeatFXFlangerLfoGain)
      .connect(this.masterBeatFXFlangerDelay.delayTime);
    this.masterBeatFXInput
      .connect(this.masterBeatFXFlangerDelay)
      .connect(this.masterBeatFXFlangerWetGain)
      .connect(this.masterBeatFXOutput);
    this.masterBeatFXFlangerDelay
      .connect(this.masterBeatFXFlangerFeedback)
      .connect(this.masterBeatFXFlangerDelay);
    this.masterBeatFXFlangerLfo.start();

    this.masterBeatFXReverb = ctx.createConvolver();
    this.createReverbImpulse(ctx, this.masterBeatFXReverb, 0.5, 0.5);
    this.masterBeatFXReverbWetGain = ctx.createGain();
    this.masterBeatFXReverbWetGain.gain.value = 0;
    this.masterBeatFXInput
      .connect(this.masterBeatFXReverb)
      .connect(this.masterBeatFXReverbWetGain)
      .connect(this.masterBeatFXOutput);
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

    const generation = deck === 'A' ? ++this.trackLoadGenerationA : ++this.trackLoadGenerationB;

    // Stop current playback
    audioElement.pause();
    audioElement.currentTime = 0;

    // Set source - use the track's URL (should be /api/audio/{id})
    const audioUrl = track.url || `/api/audio/${track.id}`;
    console.log(`🎧 DJ Audio: Loading track to Deck ${deck}: ${track.title}, URL: ${audioUrl}`);
    audioElement.src = audioUrl;

    // Load the audio
    await new Promise<void>((resolve, reject) => {
      let timeoutId: ReturnType<typeof setTimeout>;
      const cleanup = () => {
        clearTimeout(timeoutId);
        audioElement.removeEventListener('canplay', onCanPlay);
        audioElement.removeEventListener('error', onError);
      };

      const checkGeneration = () => {
        const currentGen = deck === 'A' ? this.trackLoadGenerationA : this.trackLoadGenerationB;
        return currentGen === generation;
      };

      timeoutId = setTimeout(() => {
        cleanup();
        if (checkGeneration()) {
          console.error(`🎧 DJ Audio: Timeout loading track to Deck ${deck}`);
          reject(new Error(`Timeout loading track: ${track.title}`));
        }
      }, 30000); // 30 second timeout
      
      const onCanPlay = () => {
        if (!checkGeneration()) return;
        cleanup();
        console.log(`🎧 DJ Audio: Track ready on Deck ${deck}`);
        resolve();
      };
      
      const onError = (e: Event) => {
        if (!checkGeneration()) return;
        cleanup();
        const errorMsg = audioElement.error?.message || 'Unknown error';
        console.error(`🎧 DJ Audio: Error loading track to Deck ${deck}: ${errorMsg}`, e);
        reject(new Error(`Failed to load track: ${track.title} - ${errorMsg}`));
      };
      
      audioElement.addEventListener('canplay', onCanPlay);
      audioElement.addEventListener('error', onError);
      
      // If readyState is already enough, resolve immediately
      if (audioElement.readyState >= 3) { // HAVE_FUTURE_DATA
        onCanPlay();
      } else {
        audioElement.load();
      }
    });

    console.log(`🎧 Loaded track to Deck ${deck}: ${track.title}`);

    // Auto-gain: analyze track loudness and compute normalization factor
    const storeState = useStore.getState();
    const autoGainEnabled = deck === 'A' ? storeState.djMixer.autoGainA : storeState.djMixer.autoGainB;
    if (autoGainEnabled) {
      this.analyzeAutoGain(deck, audioUrl).catch(err => {
        console.warn(`🎧 Auto-gain analysis failed for Deck ${deck}:`, err);
      });
    } else {
      // Reset auto-gain factor when disabled
      if (deck === 'A') this.autoGainFactorA = 1.0;
      else this.autoGainFactorB = 1.0;
    }
  }

  /**
   * Analyze track loudness for auto-gain normalization.
   * Fetches audio data, decodes it, and computes peak/RMS to determine gain correction.
   * Target: -3 dBFS peak (~0.71 linear) for consistent loudness across tracks.
   */
  private async analyzeAutoGain(deck: DeckId, audioUrl: string): Promise<void> {
    if (!this.audioContext) return;
    
    try {
      const response = await fetch(audioUrl);
      const arrayBuffer = await response.arrayBuffer();
      const audioBuffer = await this.audioContext.decodeAudioData(arrayBuffer);
      
      // Find peak amplitude across all channels
      let peak = 0;
      for (let ch = 0; ch < audioBuffer.numberOfChannels; ch++) {
        const channelData = audioBuffer.getChannelData(ch);
        for (let i = 0; i < channelData.length; i++) {
          const abs = Math.abs(channelData[i]);
          if (abs > peak) peak = abs;
        }
      }
      
      // Compute gain factor to normalize peak to target (-3 dBFS = 0.707)
      const targetPeak = 0.707;
      const gainFactor = peak > 0.01 ? Math.min(3.0, targetPeak / peak) : 1.0; // Cap at +9.5dB
      
      if (deck === 'A') {
        this.autoGainFactorA = gainFactor;
      } else {
        this.autoGainFactorB = gainFactor;
      }
      
      const gainDb = 20 * Math.log10(gainFactor);
      console.log(`🎧 Auto-gain Deck ${deck}: peak=${peak.toFixed(3)}, factor=${gainFactor.toFixed(2)} (${gainDb.toFixed(1)}dB)`);
      
      // Apply auto-gain by adjusting the channel gain node
      this.applyAutoGain(deck);
    } catch (err) {
      console.warn(`🎧 Auto-gain analysis error for Deck ${deck}:`, err);
      if (deck === 'A') this.autoGainFactorA = 1.0;
      else this.autoGainFactorB = 1.0;
    }
  }

  /**
   * Apply the auto-gain factor to the deck's gain node.
   * Multiplies auto-gain factor with the user's volume setting.
   */
  private applyAutoGain(deck: DeckId): void {
    const gainNode = deck === 'A' ? this.gainNodeA : this.gainNodeB;
    if (!gainNode || !this.audioContext) return;
    
    const storeState = useStore.getState();
    const deckState = deck === 'A' ? storeState.djDeckA : storeState.djDeckB;
    const autoGainFactor = deck === 'A' ? this.autoGainFactorA : this.autoGainFactorB;
    
    const effectiveGain = deckState.volume * autoGainFactor;
    gainNode.gain.setValueAtTime(
      Math.max(0, Math.min(3, effectiveGain)),
      this.audioContext.currentTime
    );
  }

  /**
   * Get the current auto-gain factor for a deck
   */
  getAutoGainFactor(deck: DeckId): number {
    return deck === 'A' ? this.autoGainFactorA : this.autoGainFactorB;
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

  // Slip mode: shadow position tracks where playback would be if scratch hadn't happened
  private slipShadowA: { startRealTime: number; startPosition: number; tempo: number } | null = null;
  private slipShadowB: { startRealTime: number; startPosition: number; tempo: number } | null = null;

  // Auto-gain: normalization gain factor per deck (1.0 = no adjustment)
  private autoGainFactorA: number = 1.0;
  private autoGainFactorB: number = 1.0;

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

    // If slip mode is enabled, capture shadow position for background playback tracking
    const storeState = useStore.getState();
    const slipEnabled = deck === 'A' ? storeState.djMixer.slipModeA : storeState.djMixer.slipModeB;
    if (slipEnabled) {
      const shadow = {
        startRealTime: Date.now(),
        startPosition: audioElement.currentTime,
        tempo: audioElement.playbackRate,
      };
      if (deck === 'A') {
        this.slipShadowA = shadow;
      } else {
        this.slipShadowB = shadow;
      }
      console.log(`🔀 Slip shadow started on Deck ${deck} at ${shadow.startPosition.toFixed(2)}s`);
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

    // Slip mode: seek to shadow position (where playback would have been)
    const slipShadow = deck === 'A' ? this.slipShadowA : this.slipShadowB;
    if (slipShadow) {
      const elapsedRealMs = Date.now() - slipShadow.startRealTime;
      const elapsedAudioSec = (elapsedRealMs / 1000) * slipShadow.tempo;
      const shadowPosition = slipShadow.startPosition + elapsedAudioSec;
      const clampedPosition = Math.max(0, Math.min(audioElement.duration || 0, shadowPosition));
      
      console.log(`🔀 Slip resume on Deck ${deck}: shadow=${clampedPosition.toFixed(2)}s (elapsed ${elapsedRealMs}ms)`);
      audioElement.currentTime = clampedPosition;
      
      // Clear shadow
      if (deck === 'A') {
        this.slipShadowA = null;
      } else {
        this.slipShadowB = null;
      }
      
      // Resume playback (skip momentum in slip mode for instant resume)
      const wasPlaying = deck === 'A' ? deckState.djDeckA.isPlaying : deckState.djDeckB.isPlaying;
      if (wasPlaying && resumePlayback) {
        audioElement.play().catch(() => {});
      }
      return;
    }

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
   * Set deck volume (0-1), accounting for auto-gain factor when enabled
   */
  setVolume(deck: DeckId, volume: number): void {
    const gainNode = deck === 'A' ? this.gainNodeA : this.gainNodeB;
    if (gainNode && this.audioContext) {
      // Guard against NaN/Infinity
      const safeVolume = (typeof volume === 'number' && isFinite(volume))
        ? Math.max(0, Math.min(1, volume))
        : 1.0;
      
      // Apply auto-gain factor if enabled
      const storeState = useStore.getState();
      const autoGainEnabled = deck === 'A' ? storeState.djMixer.autoGainA : storeState.djMixer.autoGainB;
      const autoGainFactor = autoGainEnabled
        ? (deck === 'A' ? this.autoGainFactorA : this.autoGainFactorB)
        : 1.0;
      
      const effectiveVolume = Math.max(0, Math.min(3, safeVolume * autoGainFactor));
      gainNode.gain.setValueAtTime(effectiveVolume, this.audioContext.currentTime);
    }
  }

  /**
   * Set crossfader position (-1 = full A, 0 = center, 1 = full B)
   * Uses constant-power crossfade curve
   */
  setCrossfader(position: number): void {
    if (!this.crossfaderGainA || !this.crossfaderGainB || !this.audioContext) return;

    // Guard against NaN/Infinity - clamp position to -1 to 1
    const pos = (typeof position === 'number' && isFinite(position))
      ? Math.max(-1, Math.min(1, position))
      : 0;

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
      // Guard against NaN/Infinity
      const safeVolume = (typeof volume === 'number' && isFinite(volume))
        ? Math.max(0, Math.min(1, volume))
        : 0.8;
      this.masterGain.gain.setValueAtTime(safeVolume, this.audioContext.currentTime);
    }
  }

  // ============================================================================
  // Headphone Cue (Phase 4)
  // ============================================================================

  /**
   * Enable/disable cue for a deck
   * When enabled, deck's post-EQ signal is sent to headphones
   */
  setCueEnabled(deck: DeckId, enabled: boolean): void {
    if (!this.audioContext) return;
    
    const cueGain = deck === 'A' ? this.headphoneCueGainA : this.headphoneCueGainB;
    if (!cueGain) return;
    
    if (deck === 'A') {
      this.cueEnabledA = enabled;
    } else {
      this.cueEnabledB = enabled;
    }
    
    // Set cue send gain (1 = enabled, 0 = disabled)
    const now = this.audioContext.currentTime;
    cueGain.gain.setTargetAtTime(enabled ? 1.0 : 0, now, 0.01);
    
    if (DJ_DEBUG) console.log(`🎧 Deck ${deck} CUE ${enabled ? 'enabled' : 'disabled'}`);
  }

  /**
   * Update headphone mix (cue vs master balance)
   * @param mix - 0 = cue only, 1 = master only
   *
   * Design intent:
   *   - The master signal ONLY enters the headphone path when masterCueEnabled = true.
   *     This prevents the master from bleeding into the headphone output just because
   *     the mix knob is at center, which caused both outputs to sound identical.
   *   - When masterCueEnabled = false, cued decks play at full gain in headphones,
   *     and the mix knob has no effect (it is only relevant when master monitoring is on).
   *   - When masterCueEnabled = true, the mix knob provides a constant-power crossfade
   *     between cue-only (mix=0) and master-only (mix=1).
   */
  updateHeadphoneMix(mix: number): void {
    if (!this.audioContext || !this.headphoneCueMix || !this.headphoneMasterMix) return;
    
    // Guard against NaN/Infinity - use safe default if invalid
    const safeMix = (typeof mix === 'number' && isFinite(mix))
      ? Math.max(0, Math.min(1, mix))
      : 0;
    
    this.headphoneMixValue = safeMix;

    let cueGain: number;
    let masterGain: number;

    if (this.masterCueEnabled) {
      // Master monitoring on: mix knob blends between cue (0) and master (1)
      cueGain = Math.cos(safeMix * Math.PI / 2);   // 1 at mix=0, 0 at mix=1
      masterGain = Math.sin(safeMix * Math.PI / 2); // 0 at mix=0, 1 at mix=1
    } else {
      // Master monitoring off: only cued decks play in headphones at full gain.
      // Master never bleeds in, regardless of the mix knob position.
      cueGain = 1.0;
      masterGain = 0;
    }
    
    const now = this.audioContext.currentTime;
    this.headphoneCueMix.gain.setTargetAtTime(cueGain, now, 0.01);
    this.headphoneMasterMix.gain.setTargetAtTime(masterGain, now, 0.01);
    
    if (DJ_DEBUG) console.log(`🎧 Headphone mix: cue=${cueGain.toFixed(2)} master=${masterGain.toFixed(2)}, masterCue=${this.masterCueEnabled}`);
  }

  /**
   * Enable/disable master monitoring in the headphone output.
   * This only gates the headphone master send and never affects live master output.
   */
  setMasterCueEnabled(enabled: boolean): void {
    this.masterCueEnabled = enabled;
    this.updateHeadphoneMix(this.headphoneMixValue);
  }

  getMasterCueEnabled(): boolean {
    return this.masterCueEnabled;
  }

  /**
   * Set headphone output volume
   */
  setHeadphoneVolume(volume: number): void {
    if (!this.audioContext || !this.headphoneMixer) return;
    
    // Guard against NaN/Infinity - use safe default if invalid
    const safeVolume = (typeof volume === 'number' && isFinite(volume))
      ? Math.max(0, Math.min(1, volume))
      : 1.0;
    
    const now = this.audioContext.currentTime;
    this.headphoneMixer.gain.setTargetAtTime(safeVolume, now, 0.01);
    
    if (DJ_DEBUG) console.log(`🎧 Headphone volume: ${(safeVolume * 100).toFixed(0)}%`);
  }

  /**
   * Get current cue state for a deck
   */
  getCueEnabled(deck: DeckId): boolean {
    return deck === 'A' ? this.cueEnabledA : this.cueEnabledB;
  }

  // ============================================================================
  // Output Device Routing
  // ============================================================================

  /**
   * Returns true when the headphone output is configured to the same physical
   * device as the master output.  In that case the headphone stream must be
   * kept silent to avoid doubling the master signal on the same speakers.
   */
  private headphoneAndMasterSameDevice(): boolean {
    const headIsDefault = !this.headphoneOutputDeviceId || this.headphoneOutputDeviceId === 'default';
    const mainIsDefault = !this.mainOutputDeviceId || this.mainOutputDeviceId === 'default';
    if (headIsDefault && mainIsDefault) return true;
    if (this.headphoneOutputDeviceId && this.headphoneOutputDeviceId === this.mainOutputDeviceId) return true;
    return false;
  }

  private async ensureHeadphoneOutputPlaying(): Promise<void> {
    if (this.headphoneContext) {
      if (this.headphoneContext.state === 'suspended') {
        try {
          await this.headphoneContext.resume();
          this.headphoneOutputActive = true;
          console.log('🎧 Headphone secondary AudioContext resumed');
        } catch (error) {
          console.error('🎧 Headphone secondary AudioContext could not resume:', error);
        }
      }
      return;
    }

    const audio = this.headphoneAudioElement;
    if (!audio) return;

    audio.muted = false;
    audio.volume = 1.0;

    if (!audio.paused) return;

    try {
      await audio.play();
      this.headphoneOutputActive = true;
      console.log('🎧 Headphone audio playback started');
    } catch (error) {
      console.error('🎧 Headphone audio playback could not start:', error);
    }
  }

  /**
   * Suspend/stop the headphone output stream without destroying the graph.
   * Called when the headphone device is reset to default (same as master) to
   * prevent the signal from doubling on the same physical output.
   */
  private async suspendHeadphoneOutput(): Promise<void> {
    this.headphoneOutputActive = false;
    if (this.headphoneContext && this.headphoneContext.state === 'running') {
      try {
        await this.headphoneContext.suspend();
        console.log('🎧 Headphone secondary AudioContext suspended (same-device guard)');
      } catch (error) {
        console.warn('🎧 Headphone context suspend failed:', error);
      }
      return;
    }
    const audio = this.headphoneAudioElement;
    if (audio && !audio.paused) {
      audio.pause();
      console.log('🎧 Headphone audio element paused (same-device guard)');
    }
  }

  private async ensureMainOutputElementPlaying(): Promise<void> {
    const audio = this.mainOutputAudioElement;
    if (!audio) return;

    audio.muted = false;
    audio.volume = 1.0;

    if (!audio.paused) return;

    try {
      await audio.play();
      console.log('🔊 Main routed audio playback started');
    } catch (error) {
      console.error('🔊 Main routed audio playback could not start:', error);
    }
  }

  private disconnectMasterDirectOutput(): void {
    if (!this.audioContext || !this.analyserMaster || !this.masterDirectOutputConnected) return;

    try {
      this.analyserMaster.disconnect(this.audioContext.destination);
    } catch (error) {
      if (DJ_DEBUG) console.warn('Master direct output disconnect failed:', error);
    } finally {
      this.masterDirectOutputConnected = false;
    }
  }

  private connectMasterDirectOutput(): void {
    if (!this.audioContext || !this.analyserMaster || this.masterDirectOutputConnected) return;

    this.analyserMaster.connect(this.audioContext.destination);
    this.masterDirectOutputConnected = true;
  }

  private async enableMainOutputElementRouting(deviceId: string): Promise<void> {
    if (!this.audioContext || !this.analyserMaster) {
      console.warn('Audio graph not initialized, main output device will be set on next init');
      return;
    }

    if (!this.mainOutputStreamDestination) {
      this.mainOutputStreamDestination = this.audioContext.createMediaStreamDestination();
    }

    if (!this.mainOutputAudioElement) {
      this.mainOutputAudioElement = new Audio();
      this.mainOutputAudioElement.style.display = 'none';
      this.mainOutputAudioElement.id = 'dj-main-audio';
      if (!document.getElementById('dj-main-audio')) {
        document.body.appendChild(this.mainOutputAudioElement);
      }
      this.mainOutputAudioElement.autoplay = true;
      this.mainOutputAudioElement.volume = 1.0;
    }

    this.mainOutputAudioElement.srcObject = this.mainOutputStreamDestination.stream;

    const audio = this.mainOutputAudioElement as HTMLAudioElement & { setSinkId?: (deviceId: string) => Promise<void> };
    if (typeof audio.setSinkId !== 'function') {
      throw new Error('AudioContext.setSinkId() and HTMLMediaElement.setSinkId() are not supported by this browser or WebView. Master output routing cannot be changed here.');
    }

    if (!this.mainOutputElementRoutingEnabled) {
      this.analyserMaster.connect(this.mainOutputStreamDestination);
      this.mainOutputElementRoutingEnabled = true;
    }

    await audio.setSinkId(deviceId);
    await this.ensureMainOutputElementPlaying();
    this.disconnectMasterDirectOutput();
  }

  private disableMainOutputElementRouting(): void {
    if (this.mainOutputElementRoutingEnabled && this.analyserMaster && this.mainOutputStreamDestination) {
      try {
        this.analyserMaster.disconnect(this.mainOutputStreamDestination);
      } catch (error) {
        if (DJ_DEBUG) console.warn('Main routed output disconnect failed:', error);
      }
    }

    this.mainOutputElementRoutingEnabled = false;
    this.mainOutputAudioElement?.pause();
    this.connectMasterDirectOutput();
  }

  /**
   * Set the main/live audio output device
   * Uses AudioContext.setSinkId() when available, otherwise falls back to a
   * sink-routed media element for non-default devices.
   * @param deviceId - Device ID from navigator.mediaDevices.enumerateDevices(), or empty for default
   */
  async setMainOutputDevice(deviceId: string): Promise<void> {
    this.mainOutputDeviceId = deviceId;
    
    if (!this.audioContext) {
      console.warn('Audio context not initialized, main output device will be set on next init');
      return;
    }
    
    // Check if setSinkId is supported on AudioContext
    const ctx = this.audioContext as AudioContext & { setSinkId?: (deviceId: string) => Promise<void> };
    if (typeof ctx.setSinkId === 'function') {
      try {
        this.disableMainOutputElementRouting();
        await ctx.setSinkId(deviceId || '');
        console.log(`🔊 Main output device set to: ${deviceId || 'default'}`);
      } catch (error) {
        console.error('Failed to set main output device:', error);
        throw error;
      }
      return;
    }

    if (!deviceId) {
      this.disableMainOutputElementRouting();
      console.log('🔊 Main output device set to: default');
      return;
    }

    try {
      await this.enableMainOutputElementRouting(deviceId);
      console.log(`🔊 Main output device set via media element fallback: ${deviceId}`);
    } catch (error) {
      console.error('Failed to set main output device:', error);
      throw error;
    }
  }

  /**
   * Set the headphone/cue audio output device
   * Uses HTMLAudioElement.setSinkId() for routing headphone mix to separate device
   * @param deviceId - Device ID from navigator.mediaDevices.enumerateDevices(), or empty for default
   */
  async setHeadphoneOutputDevice(deviceId: string): Promise<void> {
    this.headphoneOutputDeviceId = deviceId;

    // Primary approach: use secondary AudioContext
    if (this.headphoneContext && typeof (this.headphoneContext as any).setSinkId === 'function') {
      try {
        await (this.headphoneContext as any).setSinkId(deviceId || '');
        await this.ensureHeadphoneOutputPlaying();
        console.log(`🎧 Headphone output device set (via AudioContext) to: ${deviceId || 'default'}`);
        return;
      } catch (error) {
        console.error('Failed to set headphone output device via AudioContext:', error);
        throw error;
      }
    }

    if (!this.headphoneAudioElement) {
      console.warn('Headphone audio element not initialized, device will be set on next init');
      return;
    }
    
    // Fallback approach: use HTMLAudioElement
    const audio = this.headphoneAudioElement as HTMLAudioElement & { setSinkId?: (deviceId: string) => Promise<void> };
    if (typeof audio.setSinkId === 'function') {
      try {
        await audio.setSinkId(deviceId || '');
        await this.ensureHeadphoneOutputPlaying();
        console.log(`🎧 Headphone output device set to: ${deviceId || 'default'}`);
      } catch (error) {
        console.error('Failed to set headphone output device:', error);
        throw error;
      }
    } else {
      throw new Error('HTMLMediaElement.setSinkId() is not supported by this browser or WebView. Headphone output routing cannot be changed here.');
    }
  }

  /**
   * Get current main output device ID
   */
  getMainOutputDeviceId(): string {
    return this.mainOutputDeviceId;
  }

  /**
   * Get current headphone output device ID
   */
  getHeadphoneOutputDeviceId(): string {
    return this.headphoneOutputDeviceId;
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
      // Guard against NaN/Infinity and clamp gain to reasonable range
      const safeGain = (typeof gain === 'number' && isFinite(gain))
        ? Math.max(-24, Math.min(12, gain))
        : 0;
      eqNode.gain.setValueAtTime(safeGain, this.audioContext.currentTime);
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

  /**
   * Set key lock (preservesPitch) for a deck
   * When ON, tempo changes don't affect pitch. When OFF, pitch follows tempo.
   */
  setKeyLock(deck: DeckId, enabled: boolean): void {
    const audioElement = deck === 'A' ? this.audioElementA : this.audioElementB;
    if (audioElement) {
      audioElement.preservesPitch = enabled;
    }
  }

  /**
   * Nudge deck position slightly for beat-phase sync
   * @param deck - Deck to nudge
   * @param offsetMs - Offset in milliseconds (positive = forward, negative = backward)
   */
  nudgePosition(deck: DeckId, offsetMs: number): void {
    const audioElement = deck === 'A' ? this.audioElementA : this.audioElementB;
    if (!audioElement) return;
    
    const currentTime = audioElement.currentTime;
    const offsetSeconds = offsetMs / 1000;
    const newTime = Math.max(0, currentTime + offsetSeconds);
    
    audioElement.currentTime = newTime;
    
    if (DJ_DEBUG) console.log(`🎯 Deck ${deck} nudged by ${offsetMs.toFixed(1)}ms`);
  }

  /**
   * Perform beat-phase sync: align target deck's beat phase with source deck
   * @param targetDeck - Deck to sync
   * @param targetBeatGrid - Beat grid timestamps for target deck
   * @param sourceBeatGrid - Beat grid timestamps for source deck
   * @param sourcePosition - Current position of source deck (seconds)
   */
  syncBeatPhase(
    targetDeck: DeckId,
    targetBeatGrid: number[],
    sourceBeatGrid: number[],
    sourcePosition: number
  ): void {
    const targetElement = targetDeck === 'A' ? this.audioElementA : this.audioElementB;
    if (!targetElement || !sourceBeatGrid.length || !targetBeatGrid.length) {
      console.warn('Beat-phase sync: Missing audio element or beat grids');
      return;
    }
    
    const targetPosition = targetElement.currentTime;
    
    // Find current beat in source
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
    
    // Find current beat in target
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
    
    // Only nudge if offset is significant but not too large (avoid jumping beats)
    const maxOffset = targetBeatDuration / 2;  // Max half a beat
    if (Math.abs(phaseOffset) > 0.005 && Math.abs(phaseOffset) < maxOffset) {
      this.nudgePosition(targetDeck, phaseOffset * 1000);
      console.log(`🎯 Beat-phase sync: Deck ${targetDeck} nudged by ${(phaseOffset * 1000).toFixed(1)}ms`);
    } else if (Math.abs(phaseOffset) >= maxOffset) {
      console.log(`🎯 Beat-phase sync: Offset too large (${(phaseOffset * 1000).toFixed(1)}ms), skipping nudge`);
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
    const filterDryGain = deck === 'A' ? this.filterDryGainA : this.filterDryGainB;
    
    if (!filterFX || !filterGain || !filterDryGain || !this.audioContext) return;
    
    const now = this.audioContext.currentTime;
    
    // Guard against NaN/Infinity
    const safeFrequency = (typeof frequency === 'number' && isFinite(frequency))
      ? Math.max(20, Math.min(20000, frequency))
      : 1000;
    const safeResonance = (typeof resonance === 'number' && isFinite(resonance))
      ? Math.max(0.1, Math.min(30, resonance))
      : 1;
    
    filterFX.type = type;
    filterFX.frequency.setValueAtTime(safeFrequency, now);
    filterFX.Q.setValueAtTime(safeResonance, now);
    
    // When enabled: wet=1, dry=0 (only filtered signal)
    // When disabled: wet=0, dry=1 (only dry signal)
    filterGain.gain.setValueAtTime(enabled ? 1 : 0, now);
    filterDryGain.gain.setValueAtTime(enabled ? 0 : 1, now);
    
    if (DJ_DEBUG) console.log(`🎛️ Filter FX ${deck}: ${enabled ? 'ON' : 'OFF'}, ${type}, freq=${frequency}Hz, Q=${resonance}`);
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
    
    if (!delayNode || !delayFeedback || !delayWetGain || !this.audioContext) return;
    
    const now = this.audioContext.currentTime;
    
    // Guard against NaN/Infinity
    const safeTime = (typeof time === 'number' && isFinite(time))
      ? Math.max(0.01, Math.min(2, time))
      : 0.3;
    const safeFeedback = (typeof feedback === 'number' && isFinite(feedback))
      ? Math.max(0, Math.min(0.9, feedback))
      : 0.3;
    const safeMix = (typeof mix === 'number' && isFinite(mix))
      ? Math.max(0, Math.min(1, mix))
      : 0.5;
    
    // Set delay time and feedback
    delayNode.delayTime.setValueAtTime(safeTime, now);
    
    if (enabled) {
      delayWetGain.gain.setValueAtTime(safeMix, now);
      delayFeedback.gain.setValueAtTime(safeFeedback, now);
    } else {
      // When disabled: no wet signal, no feedback (stop echoes)
      delayWetGain.gain.setValueAtTime(0, now);
      delayFeedback.gain.setValueAtTime(0, now);
    }
    
    if (DJ_DEBUG) console.log(`🎛️ Delay FX ${deck}: ${enabled ? 'ON' : 'OFF'}, time=${time}s, feedback=${feedback}, mix=${mix}`);
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
    const flangerDelay = deck === 'A' ? this.flangerDelayA : this.flangerDelayB;
    const flangerLfo = deck === 'A' ? this.flangerLfoA : this.flangerLfoB;
    const flangerLfoGain = deck === 'A' ? this.flangerLfoGainA : this.flangerLfoGainB;
    const flangerFeedback = deck === 'A' ? this.flangerFeedbackA : this.flangerFeedbackB;
    const flangerWetGain = deck === 'A' ? this.flangerWetGainA : this.flangerWetGainB;
    
    if (!flangerDelay || !flangerLfo || !flangerLfoGain || !flangerFeedback || !flangerWetGain || !this.audioContext) return;
    
    const now = this.audioContext.currentTime;
    
    // Guard against NaN/Infinity
    const safeRate = (typeof rate === 'number' && isFinite(rate))
      ? Math.max(0.1, Math.min(8, rate))
      : 0.5;
    const safeDepth = (typeof depth === 'number' && isFinite(depth))
      ? Math.max(0, Math.min(1, depth))
      : 0.5;
    const safeFeedback = (typeof feedback === 'number' && isFinite(feedback))
      ? Math.max(0, Math.min(0.9, feedback))
      : 0.3;
    
    if (enabled) {
      flangerLfo.frequency.setValueAtTime(safeRate, now);
      flangerLfoGain.gain.setValueAtTime(safeDepth * 0.005, now); // Scale depth to delay time modulation
      flangerFeedback.gain.setValueAtTime(safeFeedback, now);
      flangerWetGain.gain.setValueAtTime(0.5, now);
    } else {
      // When disabled: stop modulation, no feedback, no wet signal
      flangerLfoGain.gain.setValueAtTime(0, now);
      flangerFeedback.gain.setValueAtTime(0, now);
      flangerWetGain.gain.setValueAtTime(0, now);
    }
    
    if (DJ_DEBUG) console.log(`🎛️ Flanger FX ${deck}: ${enabled ? 'ON' : 'OFF'}, rate=${rate}Hz, depth=${depth}, feedback=${feedback}`);
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
    const cachedParams = deck === 'A' ? this.reverbParamsA : this.reverbParamsB;
    
    if (!reverbConvolver || !reverbWetGain || !this.audioContext) return;
    
    const now = this.audioContext.currentTime;
    
    // Only regenerate impulse response when roomSize or damping changes
    // This is expensive, so we cache and compare
    if (cachedParams.roomSize !== roomSize || cachedParams.damping !== damping) {
      this.createReverbImpulse(this.audioContext, reverbConvolver, roomSize, damping);
      cachedParams.roomSize = roomSize;
      cachedParams.damping = damping;
    }
    
    if (enabled) {
      reverbWetGain.gain.setValueAtTime(mix, now);
    } else {
      reverbWetGain.gain.setValueAtTime(0, now);
    }
    
    if (DJ_DEBUG) console.log(`🎛️ Reverb FX ${deck}: ${enabled ? 'ON' : 'OFF'}, room=${roomSize}, damp=${damping}, mix=${mix}`);
  }

  // ============================================================================
  // Beat FX (Phase 6)
  // ============================================================================

  private beatFractionToMultiplier(fraction: BeatFraction): number {
    switch (fraction) {
      case '1/4':
        return 0.25;
      case '1/2':
        return 0.5;
      case '2':
        return 2;
      case '4':
        return 4;
      case '1':
      default:
        return 1;
    }
  }

  private getBeatFXDelayTime(fraction: BeatFraction, bpm: number): number {
    const safeBpm = (typeof bpm === 'number' && isFinite(bpm))
      ? Math.max(40, Math.min(240, bpm))
      : 120;
    const beatSeconds = 60 / safeBpm;
    return Math.max(0.01, Math.min(2, beatSeconds * this.beatFractionToMultiplier(fraction)));
  }

  private clearPreviousBeatFX(bpm: number): void {
    if (!this.lastBeatFXTarget || !this.lastBeatFXType) return;
    this.applyBeatFXToTarget(
      this.lastBeatFXTarget,
      this.lastBeatFXType,
      false,
      '1',
      0,
      bpm,
    );
    this.lastBeatFXTarget = null;
    this.lastBeatFXType = null;
  }

  setBeatFX(
    target: BeatFXTarget,
    type: BeatFXType,
    enabled: boolean,
    fraction: BeatFraction,
    depth: number,
    bpm: number = 120,
  ): void {
    const safeDepth = (typeof depth === 'number' && isFinite(depth))
      ? Math.max(0, Math.min(1, depth))
      : 0.45;

    const changedTargetOrType =
      this.lastBeatFXTarget !== null &&
      this.lastBeatFXType !== null &&
      (this.lastBeatFXTarget !== target || this.lastBeatFXType !== type);

    if (!enabled) {
      if (this.lastBeatFXTarget || this.lastBeatFXType) {
        this.clearPreviousBeatFX(bpm);
      } else {
        this.applyBeatFXToTarget(target, type, false, fraction, safeDepth, bpm);
      }
      return;
    }

    if (changedTargetOrType) {
      this.clearPreviousBeatFX(bpm);
    }

    this.applyBeatFXToTarget(target, type, true, fraction, safeDepth, bpm);
    this.lastBeatFXTarget = target;
    this.lastBeatFXType = type;
  }

  private applyBeatFXToTarget(
    target: BeatFXTarget,
    type: BeatFXType,
    enabled: boolean,
    fraction: BeatFraction,
    depth: number,
    bpm: number,
  ): void {
    if (target === 'master') {
      this.applyDeckBeatFX('A', type, enabled, fraction, depth, bpm);
      this.applyDeckBeatFX('B', type, enabled, fraction, depth, bpm);
      return;
    }

    this.applyDeckBeatFX(target, type, enabled, fraction, depth, bpm);
  }

  private applyDeckBeatFX(
    deck: DeckId,
    type: BeatFXType,
    enabled: boolean,
    fraction: BeatFraction,
    depth: number,
    bpm: number,
  ): void {
    const delayTime = this.getBeatFXDelayTime(fraction, bpm);
    const safeDepth = Math.max(0, Math.min(1, depth));

    switch (type) {
      case 'delay':
        this.setDelayFX(deck, enabled, delayTime, 0.12 + safeDepth * 0.48, 0.15 + safeDepth * 0.55);
        break;
      case 'echo':
        this.setDelayFX(deck, enabled, delayTime, 0.35 + safeDepth * 0.5, 0.2 + safeDepth * 0.65);
        break;
      case 'reverb':
        this.setReverbFX(deck, enabled, 0.25 + safeDepth * 0.7, 0.35, 0.12 + safeDepth * 0.65);
        break;
      case 'filter': {
        const frequency = enabled ? Math.max(180, 20000 * Math.pow(0.035, safeDepth)) : 20000;
        this.setFilterFX(deck, enabled, 'lowpass', frequency, 0.7 + safeDepth * 12);
        break;
      }
      case 'flanger': {
        const rate = Math.max(0.1, Math.min(8, 1 / delayTime));
        this.setFlangerFX(deck, enabled, rate, 0.2 + safeDepth * 0.8, 0.1 + safeDepth * 0.65);
        break;
      }
    }
  }

  private resetMasterBeatFX(now: number): void {
    this.masterBeatFXDryGain?.gain.setValueAtTime(1, now);
    this.masterBeatFXDelayWetGain?.gain.setValueAtTime(0, now);
    this.masterBeatFXDelayFeedback?.gain.setValueAtTime(0, now);
    this.masterBeatFXFilterWetGain?.gain.setValueAtTime(0, now);
    this.masterBeatFXFlangerWetGain?.gain.setValueAtTime(0, now);
    this.masterBeatFXFlangerFeedback?.gain.setValueAtTime(0, now);
    this.masterBeatFXFlangerLfoGain?.gain.setValueAtTime(0, now);
    this.masterBeatFXReverbWetGain?.gain.setValueAtTime(0, now);
  }

  private applyMasterBeatFX(
    type: BeatFXType,
    enabled: boolean,
    fraction: BeatFraction,
    depth: number,
    bpm: number,
  ): void {
    if (
      !this.audioContext ||
      !this.masterBeatFXDryGain ||
      !this.masterBeatFXDelay ||
      !this.masterBeatFXDelayFeedback ||
      !this.masterBeatFXDelayWetGain ||
      !this.masterBeatFXFilter ||
      !this.masterBeatFXFilterWetGain ||
      !this.masterBeatFXFlangerDelay ||
      !this.masterBeatFXFlangerLfo ||
      !this.masterBeatFXFlangerLfoGain ||
      !this.masterBeatFXFlangerFeedback ||
      !this.masterBeatFXFlangerWetGain ||
      !this.masterBeatFXReverb ||
      !this.masterBeatFXReverbWetGain
    ) return;

    const now = this.audioContext.currentTime;
    this.resetMasterBeatFX(now);
    if (!enabled) return;

    const safeDepth = Math.max(0, Math.min(1, depth));
    const delayTime = this.getBeatFXDelayTime(fraction, bpm);

    switch (type) {
      case 'delay':
        this.masterBeatFXDelay.delayTime.setValueAtTime(delayTime, now);
        this.masterBeatFXDelayFeedback.gain.setValueAtTime(0.12 + safeDepth * 0.48, now);
        this.masterBeatFXDelayWetGain.gain.setValueAtTime(0.15 + safeDepth * 0.55, now);
        break;
      case 'echo':
        this.masterBeatFXDelay.delayTime.setValueAtTime(delayTime, now);
        this.masterBeatFXDelayFeedback.gain.setValueAtTime(0.35 + safeDepth * 0.5, now);
        this.masterBeatFXDelayWetGain.gain.setValueAtTime(0.2 + safeDepth * 0.65, now);
        break;
      case 'reverb':
        this.createReverbImpulse(this.audioContext, this.masterBeatFXReverb, 0.25 + safeDepth * 0.7, 0.35);
        this.masterBeatFXReverbWetGain.gain.setValueAtTime(0.12 + safeDepth * 0.65, now);
        break;
      case 'filter':
        this.masterBeatFXDryGain.gain.setValueAtTime(0, now);
        this.masterBeatFXFilter.type = 'lowpass';
        this.masterBeatFXFilter.frequency.setValueAtTime(Math.max(180, 20000 * Math.pow(0.035, safeDepth)), now);
        this.masterBeatFXFilter.Q.setValueAtTime(0.7 + safeDepth * 12, now);
        this.masterBeatFXFilterWetGain.gain.setValueAtTime(1, now);
        break;
      case 'flanger':
        this.masterBeatFXFlangerLfo.frequency.setValueAtTime(Math.max(0.1, Math.min(8, 1 / delayTime)), now);
        this.masterBeatFXFlangerLfoGain.gain.setValueAtTime((0.2 + safeDepth * 0.8) * 0.005, now);
        this.masterBeatFXFlangerFeedback.gain.setValueAtTime(0.1 + safeDepth * 0.65, now);
        this.masterBeatFXFlangerWetGain.gain.setValueAtTime(0.45, now);
        break;
    }
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
    if (DJ_DEBUG) console.log(`🔁 Loop set for deck ${deck}: ${startTime.toFixed(2)}s - ${endTime.toFixed(2)}s`);
  }

  /**
   * Toggle loop on/off for a deck
   */
  toggleLoop(deck: DeckId): void {
    useStore.getState().toggleLoop(deck);
    const deckState = deck === 'A' ? useStore.getState().djDeckA : useStore.getState().djDeckB;
    if (DJ_DEBUG) console.log(`🔁 Loop ${deck}: ${deckState.loop.enabled ? 'ON' : 'OFF'}`);
  }

  /**
   * Clear loop for a deck
   */
  clearLoop(deck: DeckId): void {
    useStore.getState().clearLoop(deck);
    if (DJ_DEBUG) console.log(`🔁 Loop cleared for deck ${deck}`);
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
    if (DJ_DEBUG) console.log(`🔁 Loop IN set for deck ${deck}: ${position.toFixed(2)}s`);
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
    if (DJ_DEBUG) console.log(`🔁 Loop OUT set for deck ${deck}: ${position.toFixed(2)}s`);
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
    if (DJ_DEBUG) console.log(`🔁 ${beats}-beat loop set for deck ${deck}: ${loopDuration.toFixed(2)}s @ ${bpm.toFixed(1)} BPM`);
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
    if (DJ_DEBUG) console.log(`🔁 Loop doubled for deck ${deck}: ${(newEnd - deckState.loop.start).toFixed(2)}s`);
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
    if (DJ_DEBUG) console.log(`🔁 Loop halved for deck ${deck}: ${(newEnd - deckState.loop.start).toFixed(2)}s`);
  }

  // ============================================================================
  // Position Tracking
  // ============================================================================

  private startPositionTracking(): void {
    const updatePositions = () => {
      const storeState = useStore.getState();
      const now = performance.now();
      const aPaused = !this.audioElementA || this.audioElementA.paused;
      const bPaused = !this.audioElementB || this.audioElementB.paused;
      const aScratch = this.scratchStateA?.active ?? false;
      const bScratch = this.scratchStateB?.active ?? false;
      
      // Deck A position and loop handling — also update during scratch even when paused
      if (this.audioElementA && (!aPaused || aScratch)) {
        const currentTime = this.audioElementA.currentTime;
        
        // Check for loop at full frame rate (critical for tight loops)
        const loopA = storeState.djDeckA.loop;
        if (loopA.enabled && loopA.end > loopA.start) {
          if (currentTime >= loopA.end) {
            this.audioElementA.currentTime = loopA.start;
          }
        }
        
        // Throttle state updates to reduce React re-renders (~15 fps)
        if (now - this.lastPositionUpdateA >= this.POSITION_UPDATE_INTERVAL) {
          this.lastPositionUpdateA = now;
          this.onPositionUpdate?.('A', currentTime);
          storeState.setDeckPosition('A', currentTime);
        }
      }
      
      // Deck B position and loop handling — also update during scratch even when paused
      if (this.audioElementB && (!this.audioElementB.paused || bScratch)) {
        const currentTime = this.audioElementB.currentTime;
        
        // Check for loop at full frame rate (critical for tight loops)
        const loopB = storeState.djDeckB.loop;
        if (loopB.enabled && loopB.end > loopB.start) {
          if (currentTime >= loopB.end) {
            this.audioElementB.currentTime = loopB.start;
          }
        }
        
        // Throttle state updates to reduce React re-renders (~15 fps)
        if (now - this.lastPositionUpdateB >= this.POSITION_UPDATE_INTERVAL) {
          this.lastPositionUpdateB = now;
          this.onPositionUpdate?.('B', currentTime);
          storeState.setDeckPosition('B', currentTime);
        }
      }
      
      // Throttle to ~4fps when both decks paused AND not scratching
      if ((aPaused && !aScratch) && (bPaused && !bScratch)) {
        if (this.positionIdleTimer) clearTimeout(this.positionIdleTimer);
        this.positionIdleTimer = setTimeout(() => {
          this.animationFrameId = requestAnimationFrame(updatePositions);
        }, 250);
      } else {
        if (this.positionIdleTimer) {
          clearTimeout(this.positionIdleTimer);
          this.positionIdleTimer = null;
        }
        this.animationFrameId = requestAnimationFrame(updatePositions);
      }
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
      // When both decks are paused, skip expensive FFT and throttle to ~4fps
      const aPaused = !this.audioElementA || this.audioElementA.paused;
      const bPaused = !this.audioElementB || this.audioElementB.paused;
      if (aPaused && bPaused) {
        this.vuLevels = {
          deckA: { left: 0, right: 0 },
          deckB: { left: 0, right: 0 },
          master: { left: 0, right: 0 },
        };
        this.onVUUpdate?.(this.vuLevels);
        if (this.vuIdleTimer) clearTimeout(this.vuIdleTimer);
        this.vuIdleTimer = setTimeout(() => {
          this.vuAnimationFrameId = requestAnimationFrame(updateVU);
        }, 250);
        return;
      }
      if (this.vuIdleTimer) {
        clearTimeout(this.vuIdleTimer);
        this.vuIdleTimer = null;
      }

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

      this.vuLevels = levels;
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
    if (this.positionIdleTimer) {
      clearTimeout(this.positionIdleTimer);
      this.positionIdleTimer = null;
    }
    if (this.vuIdleTimer) {
      clearTimeout(this.vuIdleTimer);
      this.vuIdleTimer = null;
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

    // Clean up routed main output audio element
    if (this.mainOutputAudioElement) {
      this.mainOutputAudioElement.pause();
      this.mainOutputAudioElement.srcObject = null;
      this.mainOutputAudioElement = null;
    }
    this.mainOutputStreamDestination = null;
    this.mainOutputElementRoutingEnabled = false;
    this.masterDirectOutputConnected = false;

    // Clean up headphone audio element
    if (this.headphoneAudioElement) {
      this.headphoneAudioElement.pause();
      this.headphoneAudioElement.srcObject = null;
      this.headphoneAudioElement = null;
    }
    this.headphoneStreamDestination = null;

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

  /**
   * Get cached VU levels (updated by startVUMetering loop).
   */
  getVULevels(): VULevels {
    return this.vuLevels;
  }
  /**
   * Get a MediaStream from the master output for recording.
   * Creates a MediaStreamDestinationNode connected to the master gain.
   */
  getMasterStream(): MediaStream | null {
    if (!this.audioContext || !this.masterGain) return null;
    
    const dest = this.audioContext.createMediaStreamDestination();
    // Tap after masterGain (before limiter) to get clean signal
    this.masterGain.connect(dest);
    return dest.stream;
  }

  /**
   * Get the current slip shadow position for a deck (null if not in slip).
   * This is where normal playback would be if scratch hadn't happened.
   */
  getSlipShadowPosition(deck: DeckId): number | null {
    const shadow = deck === 'A' ? this.slipShadowA : this.slipShadowB;
    if (!shadow) return null;
    const elapsedRealMs = Date.now() - shadow.startRealTime;
    const elapsedAudioSec = (elapsedRealMs / 1000) * shadow.tempo;
    return shadow.startPosition + elapsedAudioSec;
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
