/**
 * ViiB MediaHub - DJ Audio Engine Hook
 * 
 * React hook for integrating the DJ Audio Engine with components.
 * Handles initialization, state synchronization, and cleanup.
 * 
 * @module hooks/useDJAudioEngine
 */

import { useEffect, useRef, useCallback } from 'react';
import { useStore } from '../store';
import { getDJAudioEngine, disposeDJAudioEngine, VULevels } from '../lib/djAudio';
import { api } from '../services/api';
import { generateClientWaveform } from '../lib/clientWaveform';
import { detectBPM, normalizeBPM, generateBeatGrid } from '../lib/bpmDetection';
import { detectKey } from '../lib/keyDetection';
import { createLogger } from '../services/loggerService';
import type { BeatFXTarget, DeckId } from '../slices/djMixerSlice';
import type { Song } from '../types';

const logger = createLogger('DJAudioEngine');

function getBeatFXBpm(state: ReturnType<typeof useStore.getState>, target: BeatFXTarget): number {
  if (target === 'A') {
    return state.djDeckA.effectiveBpm || state.djDeckA.originalBpm || 120;
  }
  if (target === 'B') {
    return state.djDeckB.effectiveBpm || state.djDeckB.originalBpm || 120;
  }

  const activeDeck = state.djActiveDeck === 'A' ? state.djDeckA : state.djDeckB;
  if (activeDeck.effectiveBpm || activeDeck.originalBpm) {
    return activeDeck.effectiveBpm || activeDeck.originalBpm || 120;
  }
  if (state.djDeckA.isPlaying && (state.djDeckA.effectiveBpm || state.djDeckA.originalBpm)) {
    return state.djDeckA.effectiveBpm || state.djDeckA.originalBpm || 120;
  }
  if (state.djDeckB.isPlaying && (state.djDeckB.effectiveBpm || state.djDeckB.originalBpm)) {
    return state.djDeckB.effectiveBpm || state.djDeckB.originalBpm || 120;
  }
  return 120;
}

export interface UseDJAudioEngineReturn {
  /** Initialize the audio engine (must be called after user interaction) */
  initialize: () => Promise<void>;
  /** Whether the engine is initialized */
  isInitialized: boolean;
  /** Load a track to a deck */
  loadTrack: (deck: DeckId, track: Song) => Promise<void>;
  /** Toggle play/pause for a deck */
  togglePlay: (deck: DeckId) => Promise<void>;
  /** Seek to position in seconds */
  seek: (deck: DeckId, position: number) => void;
  /** Set cue point at current position */
  setCue: (deck: DeckId) => void;
  /** Return to cue point */
  returnToCue: (deck: DeckId) => void;
  /** Set deck volume (0-1) */
  setVolume: (deck: DeckId, volume: number) => void;
  /** Set crossfader position (-1 to 1) */
  setCrossfader: (position: number) => void;
  /** Set master volume (0-1) */
  setMasterVolume: (volume: number) => void;
  /** Set EQ band gain */
  setEQ: (deck: DeckId, band: 'low' | 'mid' | 'high', gain: number) => void;
  /** Set tempo/playback rate (0.5-1.5) */
  setTempo: (deck: DeckId, tempo: number) => void;
  /** Set key lock (preserve pitch when tempo changes) */
  setKeyLock: (deck: DeckId, enabled: boolean) => void;
  /** Start vinyl scratch mode */
  startScratch: (deck: DeckId) => void;
  /** Update scratch position during drag */
  updateScratch: (deck: DeckId, deltaTime: number, velocity: number) => void;
  /** End scratch mode */
  endScratch: (deck: DeckId, finalVelocity?: number, resumePlayback?: boolean) => void;
  /** Check if deck is scratching */
  isScratching: (deck: DeckId) => boolean;
  /** Set Filter FX parameters */
  setFilterFX: (deck: DeckId, enabled: boolean, type: 'lowpass' | 'highpass', frequency: number, resonance: number) => void;
  /** Set Delay FX parameters */
  setDelayFX: (deck: DeckId, enabled: boolean, time: number, feedback: number, mix: number) => void;
  /** Set Flanger FX parameters */
  setFlangerFX: (deck: DeckId, enabled: boolean, rate: number, depth: number, feedback: number) => void;
  /** Set Reverb FX parameters */
  setReverbFX: (deck: DeckId, enabled: boolean, roomSize: number, damping: number, mix: number) => void;
  /** Set loop points for a deck */
  setLoop: (deck: DeckId, start: number, end: number) => void;
  /** Toggle loop on/off */
  toggleLoop: (deck: DeckId) => void;
  /** Clear loop */
  clearLoop: (deck: DeckId) => void;
  /** Set loop-in point at current position */
  setLoopIn: (deck: DeckId) => void;
  /** Set loop-out point at current position and enable loop */
  setLoopOut: (deck: DeckId) => void;
  /** Set a beat-synced loop of specified length */
  setLoopBeats: (deck: DeckId, beats: number) => void;
  /** Double the current loop length */
  doubleLoop: (deck: DeckId) => void;
  /** Halve the current loop length */
  halveLoop: (deck: DeckId) => void;
  /** Enable/disable headphone cue for a deck (Phase 4) */
  setCueEnabled: (deck: DeckId, enabled: boolean) => void;
  /** Toggle headphone cue for a deck (Phase 4) */
  toggleCue: (deck: DeckId) => void;
  /** Set headphone volume (Phase 4) */
  setHeadphoneVolume: (volume: number) => void;
  /** Set headphone cue/master mix (0 = cue only, 1 = master only) (Phase 4) */
  setHeadphoneMix: (mix: number) => void;
  /** Enable/disable master monitoring in headphones (Phase 7) */
  setMasterCueEnabled: (enabled: boolean) => void;
  /** Toggle master monitoring in headphones (Phase 7) */
  toggleMasterCue: () => void;
  /** Nudge deck position for manual beat matching (Phase 4) */
  nudgePosition: (deck: DeckId, offsetMs: number) => void;
  /** Perform beat-phase sync (Phase 4) */
  syncBeatPhase: (targetDeck: DeckId) => void;
  /** Get current VU levels (call inside rAF loop for real-time meters) */
  getVULevels: () => VULevels;
  /** Get master audio stream for recording */
  getMasterStream: () => MediaStream | null;
  /** Cleanup the engine */
  dispose: () => void;
}

export function useDJAudioEngine(): UseDJAudioEngineReturn {
  const isInitializedRef = useRef(false);
  
  // Get store actions (these don't cause re-renders - they're stable function references)
  const loadTrackToDeck = useStore(state => state.loadTrackToDeck);
  const unloadDeck = useStore(state => state.unloadDeck);
  const togglePlayDeck = useStore(state => state.togglePlayDeck);
  const setDeckPosition = useStore(state => state.setDeckPosition);
  const setDeckDuration = useStore(state => state.setDeckDuration);
  const setDeckPlaying = useStore(state => state.setDeckPlaying);
  const cueDeck = useStore(state => state.cueDeck);
  const setCuePoint = useStore(state => state.setCuePoint);
  const setDeckVolume = useStore(state => state.setDeckVolume);
  const setStoreCrossfader = useStore(state => state.setCrossfader);
  const setStoreMasterVolume = useStore(state => state.setMasterVolume);
  const setDeckEQ = useStore(state => state.setDeckEQ);
  const setDeckTempo = useStore(state => state.setDeckTempo);
  const setDeckWaveform = useStore(state => state.setDeckWaveform);
  const setDeckAnalysis = useStore(state => state.setDeckAnalysis);
  const loadHotCues = useStore(state => state.loadHotCues);
  const setDeckCue = useStore(state => state.setDeckCue);
  const toggleDeckCue = useStore(state => state.toggleDeckCue);
  const setStoreHeadphoneVolume = useStore(state => state.setHeadphoneVolume);
  const setStoreHeadphoneMix = useStore(state => state.setHeadphoneMix);
  const setStoreMasterCueEnabled = useStore(state => state.setMasterCueEnabled);
  
  // Subscribe only to the specific values needed for sync effects
  // These are the only values that should trigger re-renders
  const djCrossfader = useStore(state => state.djMixer.crossfader);
  const djMasterVolume = useStore(state => state.djMixer.masterVolume);
  const djDeckAVolume = useStore(state => state.djDeckA.volume);
  const djDeckAEqLow = useStore(state => state.djDeckA.eq.low);
  const djDeckAEqMid = useStore(state => state.djDeckA.eq.mid);
  const djDeckAEqHigh = useStore(state => state.djDeckA.eq.high);
  const djDeckATempo = useStore(state => state.djDeckA.tempo);
  const djDeckBVolume = useStore(state => state.djDeckB.volume);
  const djDeckBEqLow = useStore(state => state.djDeckB.eq.low);
  const djDeckBEqMid = useStore(state => state.djDeckB.eq.mid);
  const djDeckBEqHigh = useStore(state => state.djDeckB.eq.high);
  const djDeckBTempo = useStore(state => state.djDeckB.tempo);
  
  // Headphone cue state (Phase 4)
  const djDeckACueEnabled = useStore(state => state.djDeckA.cueEnabled);
  const djDeckBCueEnabled = useStore(state => state.djDeckB.cueEnabled);
  const djHeadphoneVolume = useStore(state => state.djMixer.headphoneVolume);
  const djHeadphoneMix = useStore(state => state.djMixer.headphoneMix);
  const djMasterCueEnabled = useStore(state => state.djMixer.masterCueEnabled);
  const djKeyLockA = useStore(state => state.djMixer.keyLockA);
  const djKeyLockB = useStore(state => state.djMixer.keyLockB);

  // Initialize the engine
  const initialize = useCallback(async () => {
    logger.debug(`Initialize called, isInitializedRef: ${isInitializedRef.current}`);
    if (isInitializedRef.current) {
      logger.debug('Already initialized, skipping');
      return;
    }

    try {
      const engine = getDJAudioEngine();
      logger.info('Initializing audio engine...');
      await engine.initialize();
      isInitializedRef.current = true;
      logger.info('Engine initialized successfully');

      // Set up callbacks
      engine.setOnTrackEnd((deck) => {
        logger.info(`Track ended on Deck ${deck}`);
        setDeckPlaying(deck, false);
      });

      // Apply initial state from store
      engine.setCrossfader(djCrossfader);
      engine.setMasterVolume(djMasterVolume);
      engine.setHeadphoneVolume(djHeadphoneVolume);
      engine.setMasterCueEnabled(djMasterCueEnabled);
      engine.updateHeadphoneMix(djHeadphoneMix);
      const initialState = useStore.getState();
      const beatFX = initialState.djMixer.beatFX;
      engine.setBeatFX(
        beatFX.target,
        beatFX.type,
        beatFX.enabled,
        beatFX.fraction,
        beatFX.depth,
        getBeatFXBpm(initialState, beatFX.target),
      );
      logger.debug('Initial state applied');
    } catch (error) {
      logger.logError(error, 'Initialization failed');
      throw error;
    }
  }, [djCrossfader, djHeadphoneMix, djHeadphoneVolume, djMasterCueEnabled, djMasterVolume, setDeckPlaying]);

  // Load track to deck
  const loadTrack = useCallback(async (deck: DeckId, track: Song) => {
    logger.info(`Loading track to Deck ${deck}: ${track.title}`);
    const engine = getDJAudioEngine();
    if (!engine.initialized) {
      logger.info('Engine not initialized, initializing...');
      await initialize();
    }

    try {
      logger.debug(`Calling engine.loadTrack for Deck ${deck}...`);
      await engine.loadTrack(deck, track);
      logger.debug(`engine.loadTrack complete, updating store...`);
      loadTrackToDeck(deck, track);
      
      // Fetch waveform data from backend, with client-side fallback
      const loadWaveform = async () => {
        try {
          // Try server-side first
          const waveformResponse = await api.getDJWaveform(track.id);
          if (waveformResponse && waveformResponse.peaks) {
            setDeckWaveform(deck, waveformResponse.peaks);
            logger.debug(`Waveform loaded from server for Deck ${deck}: ${waveformResponse.peaks.length} peaks`);
            return;
          }
        } catch (serverErr) {
          logger.debug(`Server waveform unavailable for Deck ${deck}, trying client-side generation...`);
        }
        
        // Fall back to client-side generation
        try {
          const audioUrl = `/api/audio/${track.id}`;
          const peaks = await generateClientWaveform(audioUrl);
          setDeckWaveform(deck, peaks);
          logger.debug(`Waveform generated client-side for Deck ${deck}: ${peaks.length} peaks`);
        } catch (clientErr) {
          logger.warn(`Failed to generate waveform for Deck ${deck}`, clientErr);
          // Non-critical error - waveform display will show loading state
        }
      };
      
      // Run waveform loading asynchronously (non-blocking)
      loadWaveform();
      
      // Run BPM detection asynchronously (non-blocking)
      const detectAndSetBPM = async () => {
        try {
          const audioUrl = `/api/audio/${track.id}`;
          logger.debug(`Starting BPM detection for Deck ${deck}...`);
          const bpmResult = await detectBPM(audioUrl);
          
          // Normalize BPM to reasonable range (avoid half/double time errors)
          const normalizedBpm = normalizeBPM(bpmResult.bpm);
          
          // Generate beat grid from BPM
          const deckState = deck === 'A' ? useStore.getState().djDeckA : useStore.getState().djDeckB;
          const duration = deckState.duration || track.duration || 0;
          const beatGrid = duration > 0 ? generateBeatGrid(normalizedBpm, duration) : [];
          
          // Get current key (may be set by key detection)
          const currentKey = deckState.key;
          
          // Update store with analysis results
          setDeckAnalysis(deck, normalizedBpm, currentKey, beatGrid);
          logger.info(`BPM detected for Deck ${deck}: ${normalizedBpm.toFixed(1)} (confidence: ${(bpmResult.confidence * 100).toFixed(0)}%)`);
        } catch (bpmErr) {
          logger.warn(`BPM detection failed for Deck ${deck}`, bpmErr);
          // Non-critical error - BPM will show as unknown
        }
      };
      
      // Run key detection asynchronously (non-blocking)
      const detectAndSetKey = async () => {
        try {
          const audioUrl = `/api/audio/${track.id}`;
          console.log(`🎶 useDJAudioEngine: Starting key detection for Deck ${deck}...`);
          
          // Analyze first 30 seconds for faster results
          const keyResult = await detectKey(audioUrl, { duration: 30 });
          
          // Get current BPM (may be set by BPM detection)
          const deckState = deck === 'A' ? useStore.getState().djDeckA : useStore.getState().djDeckB;
          const currentBpm = deckState.effectiveBpm || deckState.originalBpm;
          
          // Update store with key
          setDeckAnalysis(deck, currentBpm, keyResult.key, deckState.beatGrid || []);
          logger.info(`Key detected for Deck ${deck}: ${keyResult.key} (${keyResult.camelot}) - confidence: ${(keyResult.confidence * 100).toFixed(0)}%`);
        } catch (keyErr) {
          logger.warn(`Key detection failed for Deck ${deck}`, keyErr);
          // Non-critical error - key will show as unknown
        }
      };
      
      // Load hot cues from backend
      const loadSavedHotCues = async () => {
        try {
          logger.debug(`Loading hot cues for track ${track.id}...`);
          const response = await api.getDJHotCues(track.id);
          if (response && response.hotCues && response.hotCues.length > 0) {
            loadHotCues(deck, response.hotCues.map(hc => ({
              slot: hc.slot,
              position: hc.position,
              label: hc.label,
              color: hc.color
            })));
            logger.debug(`Loaded ${response.hotCues.length} hot cues for Deck ${deck}`);
          } else {
            // Clear any previous hot cues when loading new track with no saved cues
            loadHotCues(deck, []);
            logger.debug(`No hot cues found for track ${track.id}`);
          }
        } catch (hotCueErr) {
          logger.warn(`Failed to load hot cues for Deck ${deck}`, hotCueErr);
          // Non-critical error - hot cues will be empty
          loadHotCues(deck, []);
        }
      };
      
      detectAndSetBPM();
      detectAndSetKey();
      loadSavedHotCues();
    } catch (error) {
      logger.logError(error, `Failed to load track to Deck ${deck}`);
      throw error;
    }
  }, [initialize, loadTrackToDeck, setDeckWaveform, setDeckAnalysis, loadHotCues]);

  // Toggle play/pause
  const togglePlay = useCallback(async (deck: DeckId) => {
    const engine = getDJAudioEngine();
    if (!engine.initialized) return;

    const isNowPlaying = await engine.togglePlay(deck);
    setDeckPlaying(deck, isNowPlaying);
  }, [setDeckPlaying]);

  // Seek
  const seek = useCallback((deck: DeckId, position: number) => {
    const engine = getDJAudioEngine();
    if (!engine.initialized) return;

    engine.seek(deck, position);
    setDeckPosition(deck, position);
  }, [setDeckPosition]);

  // Set cue point
  const setCue = useCallback((deck: DeckId) => {
    const engine = getDJAudioEngine();
    if (!engine.initialized) return;

    const position = engine.getPosition(deck);
    setCuePoint(deck, position);
  }, [setCuePoint]);

  // Return to cue
  const returnToCue = useCallback((deck: DeckId) => {
    const engine = getDJAudioEngine();
    if (!engine.initialized) return;

    // Get cue point from store directly to avoid stale closures
    const deckState = deck === 'A' ? useStore.getState().djDeckA : useStore.getState().djDeckB;
    const cuePosition = deckState.cuePoint;

    if (cuePosition !== null) {
      engine.seek(deck, cuePosition);
      setDeckPosition(deck, cuePosition);
    }
    
    // Pause if playing (classic cue behavior)
    if (engine.isPlaying(deck)) {
      engine.pause(deck);
      setDeckPlaying(deck, false);
    }
  }, [setDeckPosition, setDeckPlaying]);

  // Set volume
  const setVolume = useCallback((deck: DeckId, volume: number) => {
    const engine = getDJAudioEngine();
    engine.setVolume(deck, volume);
    setDeckVolume(deck, volume);
  }, [setDeckVolume]);

  // Set crossfader
  const setCrossfader = useCallback((position: number) => {
    const engine = getDJAudioEngine();
    engine.setCrossfader(position);
    setStoreCrossfader(position);
  }, [setStoreCrossfader]);

  // Set master volume
  const setMasterVolume = useCallback((volume: number) => {
    const engine = getDJAudioEngine();
    engine.setMasterVolume(volume);
    setStoreMasterVolume(volume);
  }, [setStoreMasterVolume]);

  // Set EQ
  const setEQ = useCallback((deck: DeckId, band: 'low' | 'mid' | 'high', gain: number) => {
    const engine = getDJAudioEngine();
    engine.setEQ(deck, band, gain);
    setDeckEQ(deck, band, gain);
  }, [setDeckEQ]);

  // Set tempo
  const setTempo = useCallback((deck: DeckId, tempo: number) => {
    const engine = getDJAudioEngine();
    engine.setTempo(deck, tempo);
    setDeckTempo(deck, tempo);
  }, [setDeckTempo]);

  // Set key lock (preservesPitch)
  const setKeyLock = useCallback((deck: DeckId, enabled: boolean) => {
    useStore.getState().setKeyLock(deck, enabled);
    const engine = getDJAudioEngine();
    engine.setKeyLock(deck, enabled);
  }, []);

  // Dispose
  const dispose = useCallback(() => {
    disposeDJAudioEngine();
    isInitializedRef.current = false;
  }, []);

  // Sync store state to engine when store changes
  useEffect(() => {
    const engine = getDJAudioEngine();
    if (!engine.initialized) return;

    // Sync crossfader
    engine.setCrossfader(djCrossfader);
  }, [djCrossfader]);

  useEffect(() => {
    const engine = getDJAudioEngine();
    if (!engine.initialized) return;

    // Sync master volume
    engine.setMasterVolume(djMasterVolume);
  }, [djMasterVolume]);

  // Sync deck A state
  useEffect(() => {
    const engine = getDJAudioEngine();
    if (!engine.initialized) return;

    engine.setVolume('A', djDeckAVolume);
    engine.setEQ('A', 'low', djDeckAEqLow);
    engine.setEQ('A', 'mid', djDeckAEqMid);
    engine.setEQ('A', 'high', djDeckAEqHigh);
    engine.setTempo('A', djDeckATempo);
  }, [djDeckAVolume, djDeckAEqLow, djDeckAEqMid, djDeckAEqHigh, djDeckATempo]);

  // Sync deck B state
  useEffect(() => {
    const engine = getDJAudioEngine();
    if (!engine.initialized) return;

    engine.setVolume('B', djDeckBVolume);
    engine.setEQ('B', 'low', djDeckBEqLow);
    engine.setEQ('B', 'mid', djDeckBEqMid);
    engine.setEQ('B', 'high', djDeckBEqHigh);
    engine.setTempo('B', djDeckBTempo);
  }, [djDeckBVolume, djDeckBEqLow, djDeckBEqMid, djDeckBEqHigh, djDeckBTempo]);

  // Sync headphone cue state (Phase 4)
  useEffect(() => {
    const engine = getDJAudioEngine();
    if (!engine.initialized) return;

    engine.setCueEnabled('A', djDeckACueEnabled);
    engine.setCueEnabled('B', djDeckBCueEnabled);
  }, [djDeckACueEnabled, djDeckBCueEnabled]);

  // Sync headphone volume, mix, and master cue (Phase 4/7)
  useEffect(() => {
    const engine = getDJAudioEngine();
    if (!engine.initialized) return;

    // Guard against NaN/Infinity values from corrupted state
    const volume = typeof djHeadphoneVolume === 'number' && isFinite(djHeadphoneVolume)
      ? djHeadphoneVolume
        : 1.0; // Default
    const mix = typeof djHeadphoneMix === 'number' && isFinite(djHeadphoneMix)
      ? djHeadphoneMix
      : 0.5; // Default
    
    engine.setHeadphoneVolume(volume);
    engine.setMasterCueEnabled(djMasterCueEnabled);
    engine.updateHeadphoneMix(mix);
  }, [djHeadphoneVolume, djHeadphoneMix, djMasterCueEnabled]);

  // Sync key lock state
  useEffect(() => {
    const engine = getDJAudioEngine();
    if (!engine.initialized) return;

    engine.setKeyLock('A', djKeyLockA);
    engine.setKeyLock('B', djKeyLockB);
  }, [djKeyLockA, djKeyLockB]);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      // Don't dispose on unmount to allow returning to DJ mode
      // The engine will be disposed when the app closes
    };
  }, []);

  // Scratch functions
  const startScratch = useCallback((deck: DeckId) => {
    const engine = getDJAudioEngine();
    if (!engine) return;
    engine.startScratch(deck);
  }, []);

  const updateScratch = useCallback((deck: DeckId, deltaTime: number, velocity: number) => {
    const engine = getDJAudioEngine();
    if (!engine) return;
    engine.updateScratch(deck, deltaTime, velocity);
  }, []);

  const endScratch = useCallback((deck: DeckId, finalVelocity: number = 0, resumePlayback: boolean = true) => {
    const engine = getDJAudioEngine();
    if (!engine) return;
    engine.endScratch(deck, finalVelocity, resumePlayback);
  }, []);

  const isScratching = useCallback((deck: DeckId): boolean => {
    const engine = getDJAudioEngine();
    if (!engine) return false;
    return engine.isScratching(deck);
  }, []);

  // FX control functions
  const setFilterFX = useCallback((
    deck: DeckId, 
    enabled: boolean, 
    type: 'lowpass' | 'highpass', 
    frequency: number, 
    resonance: number
  ) => {
    useStore.getState().setFilterFX(deck, { enabled, type, frequency, resonance });
    const engine = getDJAudioEngine();
    if (!engine) return;
    engine.setFilterFX(deck, enabled, type, frequency, resonance);
  }, []);

  const setDelayFX = useCallback((
    deck: DeckId, 
    enabled: boolean, 
    time: number, 
    feedback: number, 
    mix: number
  ) => {
    useStore.getState().setDelayFX(deck, { enabled, time, feedback, mix });
    const engine = getDJAudioEngine();
    if (!engine) return;
    engine.setDelayFX(deck, enabled, time, feedback, mix);
  }, []);

  const setFlangerFX = useCallback((
    deck: DeckId, 
    enabled: boolean, 
    rate: number, 
    depth: number, 
    feedback: number
  ) => {
    useStore.getState().setFlangerFX(deck, { enabled, rate, depth, feedback });
    const engine = getDJAudioEngine();
    if (!engine) return;
    engine.setFlangerFX(deck, enabled, rate, depth, feedback);
  }, []);

  const setReverbFX = useCallback((
    deck: DeckId, 
    enabled: boolean, 
    roomSize: number, 
    damping: number, 
    mix: number
  ) => {
    useStore.getState().setReverbFX(deck, { enabled, roomSize, damping, mix });
    const engine = getDJAudioEngine();
    if (!engine) return;
    engine.setReverbFX(deck, enabled, roomSize, damping, mix);
  }, []);

  // Loop controls
  const setLoop = useCallback((deck: DeckId, start: number, end: number) => {
    const engine = getDJAudioEngine();
    if (!engine) return;
    engine.setLoop(deck, start, end);
  }, []);

  const toggleLoop = useCallback((deck: DeckId) => {
    const engine = getDJAudioEngine();
    if (!engine) return;
    engine.toggleLoop(deck);
  }, []);

  const clearLoop = useCallback((deck: DeckId) => {
    const engine = getDJAudioEngine();
    if (!engine) return;
    engine.clearLoop(deck);
  }, []);

  const setLoopIn = useCallback((deck: DeckId) => {
    const engine = getDJAudioEngine();
    if (!engine) return;
    engine.setLoopIn(deck);
  }, []);

  const setLoopOut = useCallback((deck: DeckId) => {
    const engine = getDJAudioEngine();
    if (!engine) return;
    engine.setLoopOut(deck);
  }, []);

  const setLoopBeats = useCallback((deck: DeckId, beats: number) => {
    const engine = getDJAudioEngine();
    if (!engine) return;
    engine.setLoopBeats(deck, beats);
  }, []);

  const doubleLoop = useCallback((deck: DeckId) => {
    const engine = getDJAudioEngine();
    if (!engine) return;
    engine.doubleLoop(deck);
  }, []);

  const halveLoop = useCallback((deck: DeckId) => {
    const engine = getDJAudioEngine();
    if (!engine) return;
    engine.halveLoop(deck);
  }, []);

  // Headphone cue functions (Phase 4)
  const setCueEnabled = useCallback((deck: DeckId, enabled: boolean) => {
    const engine = getDJAudioEngine();
    if (!engine) return;
    engine.setCueEnabled(deck, enabled);
    setDeckCue(deck, enabled);
  }, [setDeckCue]);

  const toggleCue = useCallback((deck: DeckId) => {
    const engine = getDJAudioEngine();
    if (!engine) return;
    const currentState = engine.getCueEnabled(deck);
    engine.setCueEnabled(deck, !currentState);
    toggleDeckCue(deck);
  }, [toggleDeckCue]);

  const setHeadphoneVolume = useCallback((volume: number) => {
    const engine = getDJAudioEngine();
    if (!engine) return;
    engine.setHeadphoneVolume(volume);
    setStoreHeadphoneVolume(volume);
  }, [setStoreHeadphoneVolume]);

  const setHeadphoneMix = useCallback((mix: number) => {
    const engine = getDJAudioEngine();
    if (!engine) return;
    engine.updateHeadphoneMix(mix);
    setStoreHeadphoneMix(mix);
  }, [setStoreHeadphoneMix]);

  const setMasterCueEnabled = useCallback((enabled: boolean) => {
    const engine = getDJAudioEngine();
    if (!engine) return;
    engine.setMasterCueEnabled(enabled);
    setStoreMasterCueEnabled(enabled);
  }, [setStoreMasterCueEnabled]);

  const toggleMasterCue = useCallback(() => {
    const engine = getDJAudioEngine();
    if (!engine) return;
    const nextEnabled = !useStore.getState().djMixer.masterCueEnabled;
    engine.setMasterCueEnabled(nextEnabled);
    setStoreMasterCueEnabled(nextEnabled);
  }, [setStoreMasterCueEnabled]);

  // Beat sync functions (Phase 4)
  const nudgePosition = useCallback((deck: DeckId, offsetMs: number) => {
    const engine = getDJAudioEngine();
    if (!engine) return;
    engine.nudgePosition(deck, offsetMs);
    setDeckPosition(deck, engine.getPosition(deck));
  }, [setDeckPosition]);

  const syncBeatPhase = useCallback((targetDeck: DeckId) => {
    const engine = getDJAudioEngine();
    if (!engine) return;
    
    // Get beat grids from store
    const state = useStore.getState();
    const sourceDeck = targetDeck === 'A' ? state.djDeckB : state.djDeckA;
    const target = targetDeck === 'A' ? state.djDeckA : state.djDeckB;
    
    if (!sourceDeck.beatGrid?.length || !target.beatGrid?.length) {
      console.warn('Beat-phase sync requires beat grids on both decks');
      return;
    }
    
    engine.syncBeatPhase(
      targetDeck,
      target.beatGrid,
      sourceDeck.beatGrid,
      sourceDeck.position
    );
  }, []);

  const getVULevels = useCallback((): VULevels => {
    const engine = getDJAudioEngine();
    if (!engine.initialized) {
      return { deckA: { left: 0, right: 0 }, deckB: { left: 0, right: 0 }, master: { left: 0, right: 0 } };
    }
    return engine.getVULevels();
  }, []);

  const getMasterStream = useCallback((): MediaStream | null => {
    const engine = getDJAudioEngine();
    if (!engine.initialized) return null;
    return engine.getMasterStream();
  }, []);

  return {
    initialize,
    isInitialized: isInitializedRef.current,
    loadTrack,
    togglePlay,
    seek,
    setCue,
    returnToCue,
    setVolume,
    setCrossfader,
    setMasterVolume,
    setEQ,
    setTempo,
    setKeyLock,
    startScratch,
    updateScratch,
    endScratch,
    isScratching,
    setFilterFX,
    setDelayFX,
    setFlangerFX,
    setReverbFX,
    setLoop,
    toggleLoop,
    clearLoop,
    setLoopIn,
    setLoopOut,
    setLoopBeats,
    doubleLoop,
    halveLoop,
    setCueEnabled,
    toggleCue,
    setHeadphoneVolume,
    setHeadphoneMix,
    setMasterCueEnabled,
    toggleMasterCue,
    nudgePosition,
    syncBeatPhase,
    getVULevels,
    getMasterStream,
    dispose,
  };
}

/**
 * Zero-rerender store→engine synchronization hook.
 *
 * Uses useStore.subscribe() (outside React) with manual diffing so that
 * crossfader / volume / EQ / tempo / cue / headphone / keylock changes
 * are forwarded to the audio engine WITHOUT causing the host component
 * (DJModeV2) to re-render.
 *
 * Must be called exactly ONCE at the top level of the DJ page.
 */
export function useDJAudioEngineSync(): void {
  useEffect(() => {
    let prev = {
      crossfader: 0,
      masterVolume: 0,
      deckAVolume: 0,
      deckAEqLow: 0,
      deckAEqMid: 0,
      deckAEqHigh: 0,
      deckATempo: 0,
      deckBVolume: 0,
      deckBEqLow: 0,
      deckBEqMid: 0,
      deckBEqHigh: 0,
      deckBTempo: 0,
      deckACue: false as boolean,
      deckBCue: false as boolean,
      headphoneVolume: 1,
      headphoneMix: 0.5,
      masterCueEnabled: false,
      beatFXEnabled: false,
      beatFXTarget: 'A' as BeatFXTarget,
      beatFXType: 'delay',
      beatFXFraction: '1',
      beatFXDepth: 0.45,
      keyLockA: false as boolean,
      keyLockB: false as boolean,
    };

    const syncToEngine = (state: ReturnType<typeof useStore.getState>, prevState?: ReturnType<typeof useStore.getState>) => {
      // Fast path: skip if none of the synced sub-objects changed.
      // Position/isPlaying updates create new deck objects but don't touch
      // volume/eq/tempo/cue primitives or the mixer object, so these
      // reference/primitive checks bail out in ~9 comparisons on ~15fps
      // position ticks instead of building + diffing an 18-field snapshot.
      if (
        prevState &&
        state.djMixer === prevState.djMixer &&
        state.djDeckA.volume === prevState.djDeckA.volume &&
        state.djDeckA.eq === prevState.djDeckA.eq &&
        state.djDeckA.tempo === prevState.djDeckA.tempo &&
        state.djDeckA.cueEnabled === prevState.djDeckA.cueEnabled &&
        state.djDeckB.volume === prevState.djDeckB.volume &&
        state.djDeckB.eq === prevState.djDeckB.eq &&
        state.djDeckB.tempo === prevState.djDeckB.tempo &&
        state.djDeckB.cueEnabled === prevState.djDeckB.cueEnabled
      ) return;

      const engine = getDJAudioEngine();
      if (!engine.initialized) return;

      const next = {
        crossfader: state.djMixer.crossfader,
        masterVolume: state.djMixer.masterVolume,
        deckAVolume: state.djDeckA.volume,
        deckAEqLow: state.djDeckA.eq.low,
        deckAEqMid: state.djDeckA.eq.mid,
        deckAEqHigh: state.djDeckA.eq.high,
        deckATempo: state.djDeckA.tempo,
        deckBVolume: state.djDeckB.volume,
        deckBEqLow: state.djDeckB.eq.low,
        deckBEqMid: state.djDeckB.eq.mid,
        deckBEqHigh: state.djDeckB.eq.high,
        deckBTempo: state.djDeckB.tempo,
        deckACue: state.djDeckA.cueEnabled,
        deckBCue: state.djDeckB.cueEnabled,
        headphoneVolume: state.djMixer.headphoneVolume,
        headphoneMix: state.djMixer.headphoneMix,
        masterCueEnabled: state.djMixer.masterCueEnabled,
        beatFXEnabled: state.djMixer.beatFX.enabled,
        beatFXTarget: state.djMixer.beatFX.target,
        beatFXType: state.djMixer.beatFX.type,
        beatFXFraction: state.djMixer.beatFX.fraction,
        beatFXDepth: state.djMixer.beatFX.depth,
        keyLockA: state.djMixer.keyLockA,
        keyLockB: state.djMixer.keyLockB,
      };

      // Crossfader & master
      if (next.crossfader !== prev.crossfader) engine.setCrossfader(next.crossfader);
      if (next.masterVolume !== prev.masterVolume) engine.setMasterVolume(next.masterVolume);

      // Deck A
      if (next.deckAVolume !== prev.deckAVolume) engine.setVolume('A', next.deckAVolume);
      if (next.deckAEqLow !== prev.deckAEqLow) engine.setEQ('A', 'low', next.deckAEqLow);
      if (next.deckAEqMid !== prev.deckAEqMid) engine.setEQ('A', 'mid', next.deckAEqMid);
      if (next.deckAEqHigh !== prev.deckAEqHigh) engine.setEQ('A', 'high', next.deckAEqHigh);
      if (next.deckATempo !== prev.deckATempo) engine.setTempo('A', next.deckATempo);

      // Deck B
      if (next.deckBVolume !== prev.deckBVolume) engine.setVolume('B', next.deckBVolume);
      if (next.deckBEqLow !== prev.deckBEqLow) engine.setEQ('B', 'low', next.deckBEqLow);
      if (next.deckBEqMid !== prev.deckBEqMid) engine.setEQ('B', 'mid', next.deckBEqMid);
      if (next.deckBEqHigh !== prev.deckBEqHigh) engine.setEQ('B', 'high', next.deckBEqHigh);
      if (next.deckBTempo !== prev.deckBTempo) engine.setTempo('B', next.deckBTempo);

      // Headphone cue
      if (next.deckACue !== prev.deckACue) engine.setCueEnabled('A', next.deckACue);
      if (next.deckBCue !== prev.deckBCue) engine.setCueEnabled('B', next.deckBCue);

      // Headphone volume, mix, and master cue
      if (next.masterCueEnabled !== prev.masterCueEnabled) {
        engine.setMasterCueEnabled(next.masterCueEnabled);
      }
      if (
        next.headphoneVolume !== prev.headphoneVolume ||
        next.headphoneMix !== prev.headphoneMix ||
        next.masterCueEnabled !== prev.masterCueEnabled
      ) {
        const vol = typeof next.headphoneVolume === 'number' && isFinite(next.headphoneVolume) ? next.headphoneVolume : 1.0;
        const mix = typeof next.headphoneMix === 'number' && isFinite(next.headphoneMix) ? next.headphoneMix : 0.5;
        engine.setHeadphoneVolume(vol);
        engine.updateHeadphoneMix(mix);
      }

      // Key lock
      if (next.keyLockA !== prev.keyLockA) engine.setKeyLock('A', next.keyLockA);
      if (next.keyLockB !== prev.keyLockB) engine.setKeyLock('B', next.keyLockB);

      if (
        next.beatFXEnabled !== prev.beatFXEnabled ||
        next.beatFXTarget !== prev.beatFXTarget ||
        next.beatFXType !== prev.beatFXType ||
        next.beatFXFraction !== prev.beatFXFraction ||
        next.beatFXDepth !== prev.beatFXDepth
      ) {
        engine.setBeatFX(
          next.beatFXTarget,
          next.beatFXType,
          next.beatFXEnabled,
          next.beatFXFraction,
          next.beatFXDepth,
          getBeatFXBpm(state, next.beatFXTarget),
        );
      }

      prev = next;
    };

    // Initial sync
    syncToEngine(useStore.getState());

    // Subscribe — fires outside React, zero re-renders
    const unsub = useStore.subscribe(syncToEngine);
    return unsub;
  }, []);
}
/**
 * Lightweight version of useDJAudioEngine for leaf components.
 * Returns only stable action callbacks — ZERO store subscriptions, ZERO sync effects.
 * Each callback reads fresh store state via getState() at invocation time,
 * so references never go stale and never cause React re-renders.
 * 
 * ⚠️ The parent component (e.g. DJModeV2) MUST still mount useDJAudioEngine() once
 *    to handle store→engine synchronization effects.
 */
export function useDJAudioEngineActions(): UseDJAudioEngineReturn {
  const isInitializedRef = useRef(false);

  // ---- Core lifecycle ----

  const initialize = useCallback(async () => {
    if (isInitializedRef.current) return;
    const engine = getDJAudioEngine();
    await engine.initialize();
    isInitializedRef.current = true;
    const s = useStore.getState();
    engine.setOnTrackEnd((deck) => useStore.getState().setDeckPlaying(deck, false));
    engine.setCrossfader(s.djMixer.crossfader);
    engine.setMasterVolume(s.djMixer.masterVolume);
    engine.setHeadphoneVolume(s.djMixer.headphoneVolume);
    engine.setMasterCueEnabled(s.djMixer.masterCueEnabled);
    engine.updateHeadphoneMix(s.djMixer.headphoneMix);
    const beatFX = s.djMixer.beatFX;
    engine.setBeatFX(
      beatFX.target,
      beatFX.type,
      beatFX.enabled,
      beatFX.fraction,
      beatFX.depth,
      getBeatFXBpm(s, beatFX.target),
    );
  }, []);

  const loadTrack = useCallback(async (deck: DeckId, track: Song) => {
    const engine = getDJAudioEngine();
    if (!engine.initialized) await initialize();

    await engine.loadTrack(deck, track);
    useStore.getState().loadTrackToDeck(deck, track);

    // Waveform (async, non-blocking)
    (async () => {
      try {
        const resp = await api.getDJWaveform(track.id);
        if (resp?.peaks) { useStore.getState().setDeckWaveform(deck, resp.peaks); return; }
      } catch { /* server unavailable */ }
      try {
        const peaks = await generateClientWaveform(`/api/audio/${track.id}`);
        useStore.getState().setDeckWaveform(deck, peaks);
      } catch { /* non-critical */ }
    })();

    // BPM detection (async, non-blocking)
    (async () => {
      try {
        const result = await detectBPM(`/api/audio/${track.id}`);
        const bpm = normalizeBPM(result.bpm);
        const ds = deck === 'A' ? useStore.getState().djDeckA : useStore.getState().djDeckB;
        const duration = ds.duration || track.duration || 0;
        const beatGrid = duration > 0 ? generateBeatGrid(bpm, duration) : [];
        useStore.getState().setDeckAnalysis(deck, bpm, ds.key, beatGrid);
      } catch { /* non-critical */ }
    })();

    // Key detection (async, non-blocking)
    (async () => {
      try {
        const keyResult = await detectKey(`/api/audio/${track.id}`, { duration: 30 });
        const ds = deck === 'A' ? useStore.getState().djDeckA : useStore.getState().djDeckB;
        useStore.getState().setDeckAnalysis(deck, ds.effectiveBpm || ds.originalBpm, keyResult.key, ds.beatGrid || []);
      } catch { /* non-critical */ }
    })();

    // Hot cues (async, non-blocking)
    (async () => {
      try {
        const resp = await api.getDJHotCues(track.id);
        const cues = resp?.hotCues?.length
          ? resp.hotCues.map((hc: any) => ({ slot: hc.slot, position: hc.position, label: hc.label, color: hc.color }))
          : [];
        useStore.getState().loadHotCues(deck, cues);
      } catch { useStore.getState().loadHotCues(deck, []); }
    })();
  }, [initialize]);

  const dispose = useCallback(() => {
    disposeDJAudioEngine();
    isInitializedRef.current = false;
  }, []);

  // ---- Playback controls (engine + store write) ----

  const togglePlay = useCallback(async (deck: DeckId) => {
    const engine = getDJAudioEngine();
    if (!engine.initialized) return;
    const isNowPlaying = await engine.togglePlay(deck);
    useStore.getState().setDeckPlaying(deck, isNowPlaying);
  }, []);

  const seek = useCallback((deck: DeckId, position: number) => {
    const engine = getDJAudioEngine();
    if (!engine.initialized) return;
    engine.seek(deck, position);
    useStore.getState().setDeckPosition(deck, position);
  }, []);

  const setCue = useCallback((deck: DeckId) => {
    const engine = getDJAudioEngine();
    if (!engine.initialized) return;
    useStore.getState().setCuePoint(deck, engine.getPosition(deck));
  }, []);

  const returnToCue = useCallback((deck: DeckId) => {
    const engine = getDJAudioEngine();
    if (!engine.initialized) return;
    const ds = deck === 'A' ? useStore.getState().djDeckA : useStore.getState().djDeckB;
    if (ds.cuePoint !== null) {
      engine.seek(deck, ds.cuePoint);
      useStore.getState().setDeckPosition(deck, ds.cuePoint);
    }
    if (engine.isPlaying(deck)) {
      engine.pause(deck);
      useStore.getState().setDeckPlaying(deck, false);
    }
  }, []);

  const setVolume = useCallback((deck: DeckId, volume: number) => {
    useStore.getState().setDeckVolume(deck, volume);
  }, []);

  const setCrossfader = useCallback((position: number) => {
    useStore.getState().setCrossfader(position);
  }, []);

  const setMasterVolume = useCallback((volume: number) => {
    useStore.getState().setMasterVolume(volume);
  }, []);

  const setEQ = useCallback((deck: DeckId, band: 'low' | 'mid' | 'high', gain: number) => {
    useStore.getState().setDeckEQ(deck, band, gain);
  }, []);

  const setTempo = useCallback((deck: DeckId, tempo: number) => {
    useStore.getState().setDeckTempo(deck, tempo);
  }, []);

  const setKeyLock = useCallback((deck: DeckId, enabled: boolean) => {
    useStore.getState().setKeyLock(deck, enabled);
    getDJAudioEngine().setKeyLock(deck, enabled);
  }, []);

  // ---- Scratch ----

  const startScratch = useCallback((deck: DeckId) => { getDJAudioEngine()?.startScratch(deck); }, []);
  const updateScratch = useCallback((deck: DeckId, dt: number, v: number) => { getDJAudioEngine()?.updateScratch(deck, dt, v); }, []);
  const endScratch = useCallback((deck: DeckId, fv: number = 0, resume: boolean = true) => { getDJAudioEngine()?.endScratch(deck, fv, resume); }, []);
  const isScratching = useCallback((deck: DeckId): boolean => getDJAudioEngine()?.isScratching(deck) ?? false, []);

  // ---- FX ----

  const setFilterFX = useCallback((deck: DeckId, enabled: boolean, type: 'lowpass' | 'highpass', freq: number, res: number) => {
    useStore.getState().setFilterFX(deck, { enabled, type, frequency: freq, resonance: res });
    getDJAudioEngine()?.setFilterFX(deck, enabled, type, freq, res);
  }, []);
  const setDelayFX = useCallback((deck: DeckId, enabled: boolean, time: number, fb: number, mix: number) => {
    useStore.getState().setDelayFX(deck, { enabled, time, feedback: fb, mix });
    getDJAudioEngine()?.setDelayFX(deck, enabled, time, fb, mix);
  }, []);
  const setFlangerFX = useCallback((deck: DeckId, enabled: boolean, rate: number, depth: number, fb: number) => {
    useStore.getState().setFlangerFX(deck, { enabled, rate, depth, feedback: fb });
    getDJAudioEngine()?.setFlangerFX(deck, enabled, rate, depth, fb);
  }, []);
  const setReverbFX = useCallback((deck: DeckId, enabled: boolean, roomSize: number, damping: number, mix: number) => {
    useStore.getState().setReverbFX(deck, { enabled, roomSize, damping, mix });
    getDJAudioEngine()?.setReverbFX(deck, enabled, roomSize, damping, mix);
  }, []);

  // ---- Loop ----

  const setLoop = useCallback((deck: DeckId, start: number, end: number) => { getDJAudioEngine()?.setLoop(deck, start, end); }, []);
  const toggleLoop = useCallback((deck: DeckId) => { getDJAudioEngine()?.toggleLoop(deck); }, []);
  const clearLoop = useCallback((deck: DeckId) => { getDJAudioEngine()?.clearLoop(deck); }, []);
  const setLoopIn = useCallback((deck: DeckId) => { getDJAudioEngine()?.setLoopIn(deck); }, []);
  const setLoopOut = useCallback((deck: DeckId) => { getDJAudioEngine()?.setLoopOut(deck); }, []);
  const setLoopBeats = useCallback((deck: DeckId, beats: number) => { getDJAudioEngine()?.setLoopBeats(deck, beats); }, []);
  const doubleLoop = useCallback((deck: DeckId) => { getDJAudioEngine()?.doubleLoop(deck); }, []);
  const halveLoop = useCallback((deck: DeckId) => { getDJAudioEngine()?.halveLoop(deck); }, []);

  // ---- Headphone / Cue ----

  const setCueEnabled = useCallback((deck: DeckId, enabled: boolean) => {
    useStore.getState().setDeckCue(deck, enabled);
  }, []);
  const toggleCue = useCallback((deck: DeckId) => {
    useStore.getState().toggleDeckCue(deck);
  }, []);
  const setHeadphoneVolume = useCallback((volume: number) => {
    useStore.getState().setHeadphoneVolume(volume);
  }, []);
  const setHeadphoneMix = useCallback((mix: number) => {
    useStore.getState().setHeadphoneMix(mix);
  }, []);
  const setMasterCueEnabled = useCallback((enabled: boolean) => {
    useStore.getState().setMasterCueEnabled(enabled);
  }, []);
  const toggleMasterCue = useCallback(() => {
    useStore.getState().toggleMasterCue();
  }, []);

  // ---- Sync / Nudge ----

  const nudgePosition = useCallback((deck: DeckId, offsetMs: number) => {
    const engine = getDJAudioEngine();
    engine.nudgePosition(deck, offsetMs);
    useStore.getState().setDeckPosition(deck, engine.getPosition(deck));
  }, []);

  const syncBeatPhase = useCallback((targetDeck: DeckId) => {
    const engine = getDJAudioEngine();
    if (!engine) return;
    const s = useStore.getState();
    const src = targetDeck === 'A' ? s.djDeckB : s.djDeckA;
    const tgt = targetDeck === 'A' ? s.djDeckA : s.djDeckB;
    if (!src.beatGrid?.length || !tgt.beatGrid?.length) return;
    engine.syncBeatPhase(targetDeck, tgt.beatGrid, src.beatGrid, src.position);
  }, []);

  // ---- Meters / Stream ----

  const getVULevels = useCallback((): VULevels => {
    const engine = getDJAudioEngine();
    if (!engine.initialized) return { deckA: { left: 0, right: 0 }, deckB: { left: 0, right: 0 }, master: { left: 0, right: 0 } };
    return engine.getVULevels();
  }, []);

  const getMasterStream = useCallback((): MediaStream | null => {
    const engine = getDJAudioEngine();
    return engine.initialized ? engine.getMasterStream() : null;
  }, []);

  return {
    initialize,
    isInitialized: isInitializedRef.current,
    loadTrack,
    togglePlay,
    seek,
    setCue,
    returnToCue,
    setVolume,
    setCrossfader,
    setMasterVolume,
    setEQ,
    setTempo,
    setKeyLock,
    startScratch,
    updateScratch,
    endScratch,
    isScratching,
    setFilterFX,
    setDelayFX,
    setFlangerFX,
    setReverbFX,
    setLoop,
    toggleLoop,
    clearLoop,
    setLoopIn,
    setLoopOut,
    setLoopBeats,
    doubleLoop,
    halveLoop,
    setCueEnabled,
    toggleCue,
    setHeadphoneVolume,
    setHeadphoneMix,
    setMasterCueEnabled,
    toggleMasterCue,
    nudgePosition,
    syncBeatPhase,
    getVULevels,
    getMasterStream,
    dispose,
  };
}
