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
import type { DeckId } from '../slices/djMixerSlice';
import type { Song } from '../types';

const logger = createLogger('DJAudioEngine');

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
      logger.debug('Initial state applied');
    } catch (error) {
      logger.logError(error, 'Initialization failed');
      throw error;
    }
  }, [djCrossfader, djMasterVolume, setDeckPlaying]);

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

  // Sync headphone volume and mix (Phase 4)
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
    engine.updateHeadphoneMix(mix);
  }, [djHeadphoneVolume, djHeadphoneMix]);

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

  // Beat sync functions (Phase 4)
  const nudgePosition = useCallback((deck: DeckId, offsetMs: number) => {
    const engine = getDJAudioEngine();
    if (!engine) return;
    engine.nudgePosition(deck, offsetMs);
  }, []);

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
    nudgePosition,
    syncBeatPhase,
    getVULevels,
    getMasterStream,
    dispose,
  };
}
