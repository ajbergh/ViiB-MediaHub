/**
 * ViiB MediaHub - DJ Mode Page V2
 * 
 * Redesigned DJ Mode interface inspired by professional DJ software.
 * Features circular jog wheels, dual frequency-colored waveforms,
 * and an integrated mixer/library layout.
 * 
 * Layout:
 * ┌─────────────────────────────────────────────────────────────────────────┐
 * │                         TOP BAR (Tabs, Record, Track Info)              │
 * ├─────────────────────────────────────────────────────────────────────────┤
 * │                      DUAL WAVEFORM (Overview + Main)                    │
 * ├───────────────────┬─────────────────────────────────┬───────────────────┤
 * │                   │          MIXER STRIP            │                   │
 * │  DECK A CONTROLS  │ [ A ] [ MASTER/CROSS ] [ B ]    │  DECK B CONTROLS  │
 * │ (Jog, Loop, Cue)  │  EQs      Level        EQs      │ (Jog, Loop, Cue)  │
 * │                   │ Fader     CrFader     Fader     │                   │
 * ├───────────────────┴─────────────────────────────────┴───────────────────┤
 * │                  LIBRARY AFFORDANCE (overlay above workspace)           │
 * └─────────────────────────────────────────────────────────────────────────┘
 * 
 * @module pages/DJModeV2
 */

import React, { useEffect, useState, useCallback, useRef } from 'react';
import { useStore } from '../store';
import { useDJAudioEngineActions, useDJAudioEngineSync } from '../hooks/useDJAudioEngine';
import { DJTopBar } from '../components/dj/v2/DJTopBar';
import { DJDualWaveform } from '../components/dj/v2/DJDualWaveform';
import { DJWebGLWaveform } from '../components/dj/v2/webgl';
import { DJJogWheel } from '../components/dj/v2/DJJogWheel';
import { DJHotCuePad } from '../components/dj/v2/DJHotCuePad';
import { DJTransportButtons } from '../components/dj/v2/DJTransportButtons';
import { DJLoopSection } from '../components/dj/v2/DJLoopSection';
import { DJHeadphoneMix } from '../components/dj/v2/DJHeadphoneMix';
import { DJFXSection } from '../components/dj/v2/DJFXSection';
import { DJBeatJump } from '../components/dj/v2/DJBeatJump';
import { DJBeatGridEdit } from '../components/dj/v2/DJBeatGridEdit';
import { DJSamplerPads } from '../components/dj/v2/DJSamplerPads';
import { DJNudgeButtons } from '../components/dj/v2/DJNudgeButtons';
import { DJMidiMapping } from '../components/dj/v2/DJMidiMapping';
import { DJAudioSetup } from '../components/dj/v2/DJAudioSetup';
import { DJErrorBoundary } from '../components/dj/v2/DJErrorBoundary';
import { DJScopeView } from '../components/dj/v2/DJScopeView';
import { DJDeckStatusBar } from '../components/dj/v2/DJDeckStatusBar';
import { DJStereoVUMeter } from '../components/dj/v2/DJVUMeter';
import { DJLibraryDrawer, type DJLibraryDrawerHandle } from '../components/dj/v2/DJLibraryDrawer';
import { DeckTimeDisplay, DeckHasTrack, DeckBpmBadge, DeckHorizontalVU } from '../components/dj/v2/DJDeckComponents';
import { DJChannelStrip, DJMasterKnob, DJCrossfaderSelfSub, DJTempoSliderSelfSub, DJDeckEQStrip } from '../components/dj/v2/DJMixerComponents';
import { useDJShortcuts } from '../components/dj/v2/hooks/useDJShortcuts';
import { createLogger } from '../services/loggerService';
import { shouldUseAdvancedWebGL } from '../lib/webglSafety';
import type { DeckId, DeckEQ, DJLayoutMode } from '../slices/djMixerSlice';
import { useIsDJReady } from '../hooks/useMediaQuery';
import { DJUnsupportedWidth } from '../components/dj/DJUnsupportedWidth';
import { DJFullscreenGate } from '../components/dj/DJFullscreenGate';

const logger = createLogger('DJModeV2');

type ViewMode = 'timeline' | 'scope' | 'racks';

const DJModeV2Inner: React.FC = () => {
  const [viewMode, setViewMode] = useState<ViewMode>('timeline');
  // Respect the shared platform policy and renderer fallback chain.
  const advancedWebGLEnabled = shouldUseAdvancedWebGL();
  const [isRecording, setIsRecording] = useState(false);
  const [showShortcuts, setShowShortcuts] = useState(false);
  const [showMidiMapping, setShowMidiMapping] = useState(false);
  const [showAudioSetup, setShowAudioSetup] = useState(false);
  const [dragOverDeck, setDragOverDeck] = useState<DeckId | null>(null);
  const [recordingDuration, setRecordingDuration] = useState(0);
  const libraryRef = useRef<DJLibraryDrawerHandle>(null);
  const mediaRecorderRef = useRef<MediaRecorder | null>(null);
  const recordedChunksRef = useRef<Blob[]>([]);
  const recordingTimerRef = useRef<number | null>(null);
  
  // Actions — stable Zustand references, never trigger re-renders
  const setDJMixerEnabled = useStore(s => s.setDJMixerEnabled);
  const toggleActiveDeck = useStore(s => s.toggleActiveDeck);
  const setSyncMode = useStore(s => s.setSyncMode);
  const toggleQuantize = useStore(s => s.toggleQuantize);
  const setCrossfaderCurve = useStore(s => s.setCrossfaderCurve);
  const toggleWebGLWaveform = useStore(s => s.toggleWebGLWaveform);
  const setHotCue = useStore(s => s.setHotCue);
  const triggerHotCue = useStore(s => s.triggerHotCue);
  const setDJLayoutMode = useStore(s => s.setDJLayoutMode);
  const setDeckFilter = useStore(s => s.setDeckFilter);
  const storeSetMasterVolume = useStore(s => s.setMasterVolume);

  // Render-path state — granular selectors prevent position-driven re-renders (~15fps)
  const djActiveDeck = useStore(s => s.djActiveDeck);
  const songs = useStore(s => s.songs);

  // Deck A header info (excludes position/duration/hotCues — those update at 15fps)
  const deckATrack = useStore(s => s.djDeckA.track);
  const deckAKey = useStore(s => s.djDeckA.key);
  const deckACuePoint = useStore(s => s.djDeckA.cuePoint);
  const deckALoop = useStore(s => s.djDeckA.loop);

  // Deck B header info
  const deckBTrack = useStore(s => s.djDeckB.track);
  const deckBKey = useStore(s => s.djDeckB.key);
  const deckBCuePoint = useStore(s => s.djDeckB.cuePoint);
  const deckBLoop = useStore(s => s.djDeckB.loop);

  // Mixer render state (low-frequency values only — high-freq moved to self-subscribing wrappers)
  const djLayoutMode = useStore(s => s.djMixer?.djLayoutMode || 'perf') as DJLayoutMode;
  const useWebGLWaveform = useStore(s => s.djMixer?.useWebGLWaveform);
  const syncMode = useStore(s => s.djMixer?.syncMode);
  const quantize = useStore(s => s.djMixer?.quantize);
  const crossfaderCurve = useStore(s => s.djMixer?.crossfaderCurve);

  // Zero-rerender store→engine sync (crossfader, volumes, EQ, tempo, cue, headphone, keylock)
  useDJAudioEngineSync();

  // Stable action callbacks — no store subscriptions, no sync effects
  const { 
    loadTrack,
    togglePlay, 
    returnToCue, 
    setCrossfader, 
    seek, 
    setVolume,
    setEQ,
    setTempo,
    nudgePosition,
    syncBeatPhase,
    setFilterFX,
    getVULevels,
    getMasterStream,
  } = useDJAudioEngineActions();

  // Enable DJ mode when page mounts
  useEffect(() => {
    logger.info('DJ Mode V2 mounted');
    setDJMixerEnabled(true);
    return () => {
      logger.info('DJ Mode V2 unmounted');
      // Keep state on unmount for quick return
      // But stop recording if active
      if (recordingTimerRef.current) {
        clearInterval(recordingTimerRef.current);
      }
      if (mediaRecorderRef.current && mediaRecorderRef.current.state !== 'inactive') {
        mediaRecorderRef.current.stop();
      }
    };
  }, [setDJMixerEnabled]);

  // VU meter level getters (stable callbacks for rAF-based meters)
  const getMasterLevels = useCallback(() => {
    const levels = getVULevels();
    return levels.master;
  }, [getVULevels]);

  const getDeckALevels = useCallback(() => {
    const levels = getVULevels();
    return levels.deckA;
  }, [getVULevels]);

  const getDeckBLevels = useCallback(() => {
    const levels = getVULevels();
    return levels.deckB;
  }, [getVULevels]);

  // EQ change handler
  const handleEQChange = useCallback((deck: DeckId, band: keyof DeckEQ, value: number) => {
    setEQ(deck, band, value);
  }, [setEQ]);

  // Crossfader handler
  const handleCrossfaderChange = useCallback((value: number) => {
    setCrossfader(value);
  }, [setCrossfader]);

  // Recording handler - capture master output via MediaRecorder
  const handleRecordToggle = useCallback(() => {
    if (isRecording) {
      // Stop recording
      if (mediaRecorderRef.current && mediaRecorderRef.current.state !== 'inactive') {
        mediaRecorderRef.current.stop();
      }
      if (recordingTimerRef.current) {
        clearInterval(recordingTimerRef.current);
        recordingTimerRef.current = null;
      }
      setIsRecording(false);
    } else {
      // Start recording
      const stream = getMasterStream();
      if (!stream) {
        logger.warn('Cannot record: no master stream available');
        return;
      }

      recordedChunksRef.current = [];
      const mimeType = MediaRecorder.isTypeSupported('audio/webm;codecs=opus')
        ? 'audio/webm;codecs=opus'
        : 'audio/webm';
      
      const recorder = new MediaRecorder(stream, { mimeType });
      
      recorder.ondataavailable = (e) => {
        if (e.data.size > 0) {
          recordedChunksRef.current.push(e.data);
        }
      };
      
      recorder.onstop = () => {
        const blob = new Blob(recordedChunksRef.current, { type: mimeType });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
        a.href = url;
        a.download = `ViiB-DJ-Mix-${timestamp}.webm`;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(url);
        setRecordingDuration(0);
        logger.info('Recording saved');
      };
      
      recorder.start(1000); // Collect data every second
      mediaRecorderRef.current = recorder;
      setIsRecording(true);
      setRecordingDuration(0);
      
      // Start duration timer
      const startTime = Date.now();
      recordingTimerRef.current = window.setInterval(() => {
        setRecordingDuration(Math.floor((Date.now() - startTime) / 1000));
      }, 1000);
    }
  }, [isRecording, getMasterStream]);

  // Volume handlers
  const handleVolumeChange = useCallback((deck: DeckId, value: number) => {
    setVolume(deck, value);
  }, [setVolume]);

  // Tempo handler
  const handleTempoChange = useCallback((deck: DeckId, value: number) => {
    setTempo(deck, value);
  }, [setTempo]);

  // Sync handler - syncs BPM to the other deck (reads from store snapshot to avoid position deps)
  const handleSync = useCallback((deck: DeckId) => {
    const state = useStore.getState();
    const thisDeck = deck === 'A' ? state.djDeckA : state.djDeckB;
    const otherDeck = deck === 'A' ? state.djDeckB : state.djDeckA;
    
    if (!thisDeck.originalBpm || !otherDeck.effectiveBpm) return;
    
    // Calculate tempo to match other deck's BPM
    const targetBpm = otherDeck.effectiveBpm;
    const newTempo = targetBpm / thisDeck.originalBpm;
    const clampedTempo = Math.max(0.5, Math.min(1.5, newTempo));
    setTempo(deck, clampedTempo);
    
    // If sync mode includes beat-phase, also sync the phase
    if (state.djMixer.syncMode === 'beat-phase') {
      syncBeatPhase(deck);
    }
  }, [setTempo, syncBeatPhase]);

  const handleViewModeChange = useCallback((mode: ViewMode) => {
    setViewMode(mode);
    if (mode === 'racks') {
      setDJLayoutMode('fx');
    } else if (djLayoutMode === 'fx') {
      setDJLayoutMode('perf');
    }
  }, [djLayoutMode, setDJLayoutMode]);

  const handleLayoutModeChange = useCallback((mode: DJLayoutMode) => {
    setDJLayoutMode(mode);
    if (mode === 'fx') {
      setViewMode('racks');
    } else {
      if (viewMode === 'racks') setViewMode('timeline');
      if (mode === 'browse') libraryRef.current?.open();
    }
  }, [setDJLayoutMode, viewMode]);

  // Keyboard shortcuts — single-mount listener via useDJShortcuts (see §2.2)
  useDJShortcuts({
    togglePlay,
    returnToCue,
    setCrossfader,
    toggleActiveDeck,
    seek,
    setHotCue,
    triggerHotCue,
    handleSync,
    nudgePosition,
    setShowShortcuts,
    openLibrary: () => libraryRef.current?.open(),
    closeLibrary: () => libraryRef.current?.close() ?? false,
    showShortcuts,
  });

  // Filter knob handler — coalesce duplicate writes during a drag.
  // Knob already round-snaps, so most pointermove ticks deliver the SAME value;
  // skipping repeats avoids two no-op store/engine writes per tick.
  const lastFilterRef = useRef<{ A: number | null; B: number | null }>({ A: null, B: null });
  const handleFilterChange = useCallback((deck: DeckId, knobValue: number) => {
    const normalized = (knobValue + 24) / 36;          // 0..1
    const clamped = Math.max(-1, Math.min(1, normalized * 2 - 1));

    if (lastFilterRef.current[deck] === clamped) return;
    lastFilterRef.current[deck] = clamped;

    // Single store write — setDeckFilter sets both value and enabled (threshold 0.05)
    setDeckFilter(deck, clamped);

    // Update audio engine filter
    const isNeutral = Math.abs(clamped) < 0.05;
    if (isNeutral) {
      setFilterFX(deck, false, 'lowpass', 20000, 0.5);
    } else if (clamped < 0) {
      const freq = 200 * Math.pow(100, 1 + clamped);  // 200Hz..20kHz
      setFilterFX(deck, true, 'lowpass', freq, 2);
    } else {
      const freq = 20 + clamped * 7980;               // 20Hz..8kHz
      setFilterFX(deck, true, 'highpass', freq, 2);
    }
  }, [setDeckFilter, setFilterFX]);

  // Master volume handler - maps -24..+12 knob range to 0..1 volume
  const handleMasterVolumeChange = useCallback((knobValue: number) => {
    const normalized = (knobValue + 24) / 36; // 0 to 1
    const clamped = Math.max(0, Math.min(1, normalized));
    storeSetMasterVolume(clamped);
  }, [storeSetMasterVolume]);

  // Format seconds to MM:SS or -MM:SS
  const formatTime = useCallback((seconds: number, negative = false): string => {
    const abs = Math.abs(seconds);
    const m = Math.floor(abs / 60);
    const s = Math.floor(abs % 60);
    const prefix = negative ? '-' : '';
    return `${prefix}${m}:${s.toString().padStart(2, '0')}`;
  }, []);

  const getDraggedSong = useCallback((event: React.DragEvent): typeof songs[number] | null => {
    const typedPayload = event.dataTransfer.getData('application/x-viib-dj-track');
    if (typedPayload) {
      try {
        const parsed = JSON.parse(typedPayload) as { type?: string; songId?: string };
        if (parsed.type === 'viib-dj-track' && parsed.songId) {
          return songs.find(song => song.id === parsed.songId) ?? null;
        }
      } catch {
        return null;
      }
    }

    const fallbackId = event.dataTransfer.getData('text/plain');
    if (!fallbackId) return null;
    return songs.find(song => song.id === fallbackId) ?? null;
  }, [songs]);

  const handleDeckDragOver = useCallback((event: React.DragEvent, deck: DeckId) => {
    if (![...event.dataTransfer.types].some(type => type === 'application/x-viib-dj-track' || type === 'text/plain')) {
      return;
    }
    event.preventDefault();
    event.dataTransfer.dropEffect = 'copy';
    setDragOverDeck(deck);
  }, []);

  const handleDeckDragLeave = useCallback((event: React.DragEvent, deck: DeckId) => {
    const nextTarget = event.relatedTarget as Node | null;
    if (nextTarget && event.currentTarget.contains(nextTarget)) return;
    setDragOverDeck(current => current === deck ? null : current);
  }, []);

  const handleDeckDrop = useCallback(async (event: React.DragEvent, deck: DeckId) => {
    event.preventDefault();
    setDragOverDeck(null);
    const song = getDraggedSong(event);
    if (!song) return;

    try {
      await loadTrack(deck, song);
    } catch (error) {
      logger.warn(`Failed to load dragged track to Deck ${deck}`, error);
    }
  }, [getDraggedSong, loadTrack]);

  return (
    <div
      className='dj-workstation relative h-full flex flex-col overflow-hidden'
      data-dj-mode={djLayoutMode}
      style={{
        backgroundColor: 'var(--dj-bg)',
      }}
    >
      <h1 className='sr-only'>DJ Mode</h1>
      
      {/* Keyboard Shortcuts Overlay */}
      {showShortcuts && (
        <div className='fixed inset-0 z-[100] bg-black/80 backdrop-blur-sm flex items-center justify-center' onClick={() => setShowShortcuts(false)} role="dialog" aria-modal="true" aria-label="Keyboard shortcuts">
          <div className='bg-surface-2 border border-[#333] rounded-lg p-6 max-w-lg w-full max-h-[80vh] overflow-y-auto' onClick={e => e.stopPropagation()}>
            <div className='flex items-center justify-between mb-4'>
              <h2 className='text-lg font-bold text-text-main'>Keyboard Shortcuts</h2>
              <button onClick={() => setShowShortcuts(false)} className='text-text-subtle hover:text-text-main text-xl' aria-label="Close shortcuts overlay">✕</button>
            </div>
            <div className='grid grid-cols-2 gap-x-6 gap-y-0.5 text-xs text-text-secondary'>
              {[
                ['Transport', null],
                ['W', 'Play/Pause Deck A'],
                ['P', 'Play/Pause Deck B'],
                ['Space', 'Play/Pause Active Deck'],
                ['Q', 'Return to Cue (Deck A)'],
                ['O', 'Return to Cue (Deck B)'],
                ['Tab', 'Toggle Active Deck'],
                ['', ''],
                ['Crossfader', null],
                ['Z', 'Crossfader → A'],
                ['X', 'Crossfader → Center'],
                ['C', 'Crossfader → B'],
                ['', ''],
                ['Nudge', null],
                ['Shift+←', 'Nudge active deck -20ms'],
                ['Shift+→', 'Nudge active deck +20ms'],
                ['Alt+Shift+←', 'Fine nudge -5ms'],
                ['Alt+Shift+→', 'Fine nudge +5ms'],
                ['', ''],
                ['Sync', null],
                ['E', 'Sync Deck A'],
                ['[', 'Sync Deck B'],
                ['', ''],
                ['Hot Cues', null],
                ['1-8', 'Trigger Hot Cue'],
                ['Shift+1-8', 'Set Hot Cue'],
                ['', ''],
                ['Browser', null],
                ['/', 'Open Library / Focus Search'],
                ['', ''],
                ['View', null],
                ['F11', 'Toggle Fullscreen'],
                ['Ctrl+Scroll', 'Zoom Waveform'],
                ['?', 'Toggle This Overlay'],
                ['Esc', 'Close Overlay'],
              ].map(([key, action], i) => {
                if (key === '' && action === '') return <div key={i} className='col-span-2 h-1' />;
                if (action === null) return <div key={i} className='col-span-2 text-[10px] font-bold text-brand uppercase tracking-widest mt-2 mb-1 border-b border-surface-3 pb-1'>{key}</div>;
                return (
                  <React.Fragment key={i}>
                    <span className='text-right pr-2'>
                      <kbd className='px-1.5 py-0.5 bg-surface-3 rounded text-text-main font-mono text-[10px] border border-[#444]'>{key}</kbd>
                    </span>
                    <span className='text-text-secondary'>{action}</span>
                  </React.Fragment>
                );
              })}
            </div>
            <p className='text-[10px] text-neutral-600 mt-4 text-center'>Press ? to toggle • Esc to close</p>
          </div>
        </div>
      )}

      {/* 1. TOP BAR */}
      <div className='relative'>
        <DJTopBar 
          viewMode={viewMode}
          onViewModeChange={handleViewModeChange}
          isRecording={isRecording}
          onRecordToggle={handleRecordToggle}
          layoutMode={djLayoutMode}
          onLayoutModeChange={handleLayoutModeChange}
        />
        <div className='absolute right-12 top-1/2 -translate-y-1/2 flex items-center gap-1'>
          <button
            onClick={() => setShowAudioSetup(true)}
            className='flex items-center gap-1.5 px-2 min-h-[28px] text-[10px] font-bold uppercase tracking-wider
              bg-[#222] text-neutral-500 border border-[#333] rounded hover:bg-[#2a2a2a] hover:text-neutral-300 transition-colors'
            title='Audio Output Setup'
          >
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true">
              <path d="M3 9v6h4l5 4V5L7 9H3Z"/>
              <path d="M16 9.5a4 4 0 0 1 0 5"/>
              <path d="M18.5 7a8 8 0 0 1 0 10"/>
            </svg>
            AUDIO
          </button>
          {/* MIDI button — D1: icon-based button */}
          <button
            onClick={() => setShowMidiMapping(true)}
            className='flex items-center gap-1.5 px-2 min-h-[28px] text-[10px] font-bold uppercase tracking-wider
              bg-[#222] text-neutral-500 border border-[#333] rounded hover:bg-[#2a2a2a] hover:text-neutral-300 transition-colors'
            title='MIDI Controller Mapping'
          >
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true">
              <rect x="2" y="2" width="20" height="20" rx="2"/>
              <path d="M7 7v10M12 7v4M17 7v10M7 17h10"/>
            </svg>
            MIDI
          </button>
        </div>
      </div>

      {/* Audio Setup Dialog */}
      {showAudioSetup && <DJAudioSetup onClose={() => setShowAudioSetup(false)} />}

      {/* MIDI Mapping Dialog */}
      {showMidiMapping && <DJMidiMapping onClose={() => setShowMidiMapping(false)} />}

      {/* 2. WAVEFORM / SCOPE / FX VIEW */}
      {viewMode === 'timeline' && (
        <div className='flex-shrink-0 border-b border-[#2a2a2a] relative' style={{ height: 'var(--dj-waveform-h)', backgroundColor: 'var(--dj-bg)' }}>
          {/* WebGL / Canvas 2D toggle */}
          <button
            onClick={advancedWebGLEnabled ? toggleWebGLWaveform : undefined}
            disabled={!advancedWebGLEnabled}
            className={`absolute top-1 left-1 z-10 px-2 min-h-[28px] flex items-center rounded text-[10px] font-bold uppercase transition-colors ${
              advancedWebGLEnabled && useWebGLWaveform
                ? 'bg-green-600/30 text-green-400 border border-green-500/40'
                : 'bg-neutral-800/80 text-neutral-500 border border-neutral-700'
            }`}
            title={advancedWebGLEnabled
              ? (useWebGLWaveform ? 'WebGL waveform active — click for Canvas 2D' : 'Canvas 2D waveform — click for WebGL')
              : 'Canvas waveform is used in the macOS desktop app for stability'}
            aria-pressed={advancedWebGLEnabled && useWebGLWaveform}
          >
            {advancedWebGLEnabled && useWebGLWaveform ? 'WebGL' : '2D'}
          </button>
          <DJErrorBoundary componentName='DJWaveform'>
            {advancedWebGLEnabled && useWebGLWaveform ? (
              <DJWebGLWaveform height={-1} allowFallback />
            ) : (
              <DJDualWaveform height={-1} responsive />
            )}
          </DJErrorBoundary>
        </div>
      )}
      {viewMode === 'scope' && (
        <div className='flex-shrink-0 border-b border-[#2a2a2a] flex items-center justify-center' style={{ height: 'var(--dj-waveform-h)', backgroundColor: 'var(--dj-bg)' }}>
          <DJScopeView getVULevels={getVULevels} />
        </div>
      )}
      {viewMode === 'racks' && (
        <div className='flex-shrink-0 border-b border-[#2a2a2a] overflow-hidden' style={{ height: 'var(--dj-fx-h)', backgroundColor: 'var(--dj-bg)' }}>
          <div className='h-full flex flex-col'>
            <DJFXSection />
          </div>
        </div>
      )}

      {/* 3. FX SECTION (shown in timeline/scope mode, hidden in FX layout to avoid double-render) */}
      {viewMode !== 'racks' && djLayoutMode !== 'fx' && <DJFXSection />}

      {/* 4. MAIN CONTROL DECK (Decks + Mixer) */}
      <div data-dj-workspace className='flex-1 flex min-h-0 overflow-auto' style={{ backgroundColor: 'var(--dj-bg)' }}>
        
        {/* === DECK A === */}
        <div
          className={`dj-deck flex-1 flex flex-col min-w-0 border-r border-[#2a2a2a] relative ${djActiveDeck === 'A' ? 'ring-1 ring-inset ring-blue-500/40' : ''} ${dragOverDeck === 'A' ? 'ring-2 ring-inset ring-blue-300/80 bg-blue-500/5' : ''}`}
          onDragOver={(e) => handleDeckDragOver(e, 'A')}
          onDragLeave={(e) => handleDeckDragLeave(e, 'A')}
          onDrop={(e) => handleDeckDrop(e, 'A')}
        >
            {/* Active Deck Indicator Bar */}
            {djActiveDeck === 'A' && <div className='absolute top-0 left-0 right-0 h-[2px] bg-blue-500 z-30' />}
            
            {/* Deck Header: Track Info + Jumbo BPM/Time + VU strip + Controls
                (CUES moved to footer for performance reach) */}
            <div className='bg-[#161616] border-b border-[#222]'>
                  {/* Track Info Row — 64 px tall to fit jumbo BPM + jumbo time displays */}
                  <div className='dj-deck-info px-3 py-2 border-b border-[#1a1a1a]'>
                      <div className='flex items-center gap-2 min-w-0 flex-1'>
                        <span className='text-[10px] font-bold text-blue-400 uppercase tracking-wider flex-shrink-0 px-1.5 py-0.5 bg-blue-500/15 rounded'>DECK A</span>
                        {deckATrack ? (
                          <div className='min-w-0 flex-1'>
                            <div className='text-[14px] font-bold text-white truncate leading-tight'>
                              {deckATrack.title || 'Unknown'}
                            </div>
                            <div className='text-[11px] text-neutral-400 truncate leading-tight'>
                              {deckATrack.artist || 'Unknown Artist'}
                            </div>
                          </div>
                        ) : (
                          <span className='text-[11px] text-neutral-600 italic'>No track loaded</span>
                        )}
                      </div>
                      <div className='flex items-center gap-3 flex-shrink-0'>
                        {/* Per-deck horizontal stereo VU strip (real-time output level) */}
                        <DeckHorizontalVU getLevels={getDeckALevels} width={120} channelHeight={5} />
                        {deckAKey && (
                          <span className='text-[12px] font-bold text-emerald-400 bg-emerald-500/15 px-2.5 py-1 rounded'>
                            {deckAKey}
                          </span>
                        )}
                        <DJDeckStatusBar deck='A' />
                        {/* Jumbo BPM display */}
                        <DeckBpmBadge deck='A' large />
                        <DeckHasTrack deck='A'>
                          <div className='flex flex-col items-end font-mono leading-none'>
                            <DeckTimeDisplay deck='A' color='#93c5fd' sizeClass='text-[20px]' />
                            <DeckTimeDisplay deck='A' color='#737373' sizeClass='text-[12px] mt-0.5' showRemaining />
                          </div>
                        </DeckHasTrack>
                      </div>
                  </div>
                  {/* Controls Row: Loop + Beat Jump + Grid (icon-only, no labels — cleaner row) */}
                  <div className='flex items-center gap-3 px-3 py-1 overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden'>
                      <DJLoopSection deck='A' />
                      <DJBeatJump deck='A' compact />
                      <DJBeatGridEdit deck='A' />
                  </div>
            </div>

            {/* Deck Jog Area — EQ strip left of jog, jog right */}
            <div className='flex-1 relative bg-[#141414] flex items-stretch overflow-hidden'>
                {/* Tempo Slider — slim outer edge */}
                <div className='w-16 flex-shrink-0 flex flex-col items-center justify-center py-3 gap-2 z-10'>
                     <DJNudgeButtons deck='A' onNudge={nudgePosition} disabled={!deckATrack} />
                     <div className='flex-1 min-h-0 flex items-center justify-center'>
                     <DJTempoSliderSelfSub
                        deck='A'
                        onChange={(v) => handleTempoChange('A', v)}
                        disabled={!deckATrack}
                        height={-1}
                        responsive
                     />
                     </div>
                </div>

                {/* EQ Strip — TRIM / HIGH / MID / LOW / FILTER beside jog */}
                <div className='w-[72px] flex-shrink-0 border-r border-[#1e1e1e] bg-[#0d0d0d]'>
                    <DJDeckEQStrip
                      deckId='A'
                      onEQChange={handleEQChange}
                      onVolumeChange={handleVolumeChange}
                      onFilterChange={handleFilterChange}
                    />
                </div>

                {/* Jog Wheel — fills full remaining space */}
                <div className='flex-1 min-w-0 h-full flex items-center justify-center'>
                    <DJErrorBoundary componentName='DJJogWheel-A'>
                        <DJJogWheel deck='A' size={-1} responsive />
                    </DJErrorBoundary>
                </div>
            </div>

            {/* Deck Footer: Hot Cues row + Transport row (performance-reach layout) */}
            <div className='bg-[#161616] border-t border-[#222] flex flex-col flex-shrink-0 shadow-[inset_0_8px_24px_rgba(0,0,0,0.6)]'>
                {/* Hot Cues — always reachable in the prime thumb zone */}
                <div className='flex items-center justify-center px-3 py-2 border-b border-[#1a1a1a]'>
                    <DJHotCuePad deck='A' singleRow />
                </div>
                {/* Transport row — Cue / Play / Sync centered with status flanks */}
                <div className='h-[80px] flex items-center justify-between px-4'>
                    {/* CUE point */}
                    <div className='flex flex-col items-start gap-0.5 w-16 flex-shrink-0'>
                        <span className='text-[9px] text-neutral-500 uppercase tracking-wider'>CUE</span>
                        <span className='text-[11px] font-mono text-yellow-400'>
                            {deckACuePoint > 0 ? formatTime(deckACuePoint) : '--:--'}
                        </span>
                    </div>
                    {/* Transport controls */}
                    <DJTransportButtons deck='A' />
                    {/* Loop status */}
                    <div className='flex flex-col items-end gap-0.5 w-16 flex-shrink-0'>
                        <span className='text-[9px] text-neutral-500 uppercase tracking-wider'>LOOP</span>
                        <span className={`text-[11px] font-mono ${deckALoop.enabled ? 'text-green-400' : 'text-neutral-600'}`}>
                            {deckALoop.enabled ? `${(deckALoop.end - deckALoop.start).toFixed(2)}s` : 'OFF'}
                        </span>
                    </div>
                </div>
            </div>
        </div>

        {/* === MIXER CENTER (Fixed Width) === */}
        <div className='dj-mixer flex-shrink-0 flex flex-col border-x border-[#333] z-0 relative @container/mixer' style={{ width: 'var(--dj-mixer-w)', backgroundColor: 'var(--dj-surface-1, #181818)' }}>
             {/* Mixer Body */}
             <div className='flex-1 flex w-full relative min-h-0 overflow-y-auto overflow-x-hidden custom-scrollbar'>
                 {/* Channel A */}
                 <DJChannelStrip
                   deckId='A'
                   getDeckLevels={getDeckALevels}
                   onVolumeChange={handleVolumeChange}
                 />

                 {/* Center Master Strip */}
                 <div className='w-[88px] bg-[#131313] flex flex-col items-center py-2 border-x border-[#222]/50 overflow-hidden min-h-0'>
                      <div className='text-[10px] text-[#444] font-bold mb-1 tracking-widest'>MASTER</div>

                      {/* Real-time Master VU Meters — flex-1 to fill available space */}
                      <div className='flex-1 min-h-0 flex items-center justify-center mb-2'>
                           <DJStereoVUMeter
                             getLevels={getMasterLevels}
                             height={80}
                             channelWidth={6}
                             gap={3}
                             segments={14}
                             showPeak={true}
                           />
                      </div>

                      {/* Separator */}
                      <div className='w-10 h-px bg-[#333] mb-2' />

                      {/* Master Volume */}
                      <DJMasterKnob onChange={handleMasterVolumeChange} />
                 </div>

                 {/* Channel B */}
                 <DJChannelStrip
                   deckId='B'
                   getDeckLevels={getDeckBLevels}
                   onVolumeChange={handleVolumeChange}
                 />
             </div>

             {/* Sync Mode + Crossfader Section — crossfader is the hero, sits above CUE-MIX */}
             <div className='bg-[#1a1a1a] border-t border-[#333] flex flex-col items-center flex-shrink-0'>
                 {/* Sync Mode Selector + Quantize */}
                 <div className='flex gap-1 py-2 items-center'>
                   {(['off', 'bpm', 'beat-phase'] as const).map(mode => (
                     <button
                       key={mode}
                       onClick={() => setSyncMode(mode)}
                       aria-pressed={syncMode === mode}
                       className={`
                         px-3 min-h-[36px] flex items-center rounded text-[10px] font-bold uppercase tracking-wider
                         transition-all duration-100 border
                         ${syncMode === mode
                           ? mode === 'off'
                             ? 'bg-neutral-600 text-white border-neutral-500'
                             : mode === 'bpm'
                               ? 'bg-green-600 text-white border-green-500'
                               : 'bg-amber-600 text-white border-amber-500'
                           : 'bg-[#222] text-neutral-500 border-[#333] hover:bg-[#2a2a2a] hover:text-neutral-300'}
                       `}
                     >
                       {mode === 'beat-phase' ? 'PHASE' : mode.toUpperCase()}
                     </button>
                   ))}
                   <div className='w-px h-5 bg-[#333] mx-0.5' />
                   <button
                     onClick={toggleQuantize}
                     aria-pressed={quantize}
                     className={`
                       w-10 min-h-[36px] flex items-center justify-center rounded text-[12px] font-bold uppercase tracking-wider
                       transition-all duration-100 border
                       ${quantize
                         ? 'bg-cyan-600 text-white border-cyan-500'
                         : 'bg-[#222] text-neutral-500 border-[#333] hover:bg-[#2a2a2a] hover:text-neutral-300'}
                     `}
                     title={`Quantize ${quantize ? 'ON' : 'OFF'} - Snap actions to beat grid`}
                   >
                     Q
                   </button>
                 </div>

                 {/* Crossfader Curve Selector */}
                 <div className='flex gap-1 items-center pb-2'>
                   <span className='text-[10px] text-neutral-600 uppercase tracking-wider mr-1'>Curve</span>
                   {(['linear', 'constant-power', 'sharp'] as const).map(curve => (
                     <button
                       key={curve}
                       onClick={() => setCrossfaderCurve(curve)}
                       aria-pressed={crossfaderCurve === curve}
                       className={`
                         px-3 min-h-[32px] flex items-center rounded text-[10px] font-bold uppercase tracking-wider
                         transition-all duration-100 border
                         ${crossfaderCurve === curve
                           ? 'bg-orange-600/80 text-white border-orange-500/60'
                           : 'bg-[#222] text-neutral-600 border-[#333] hover:bg-[#2a2a2a] hover:text-neutral-400'}
                       `}
                       title={`Crossfader curve: ${curve === 'constant-power' ? 'Constant Power' : curve === 'linear' ? 'Linear' : 'Sharp / Cut'}`}
                     >
                       {curve === 'constant-power' ? 'CP' : curve === 'linear' ? 'LIN' : 'CUT'}
                     </button>
                   ))}
                 </div>

                 {/* Crossfader (hero) — sits above the headphone CUE-MIX so it's reached
                     before the lower-priority cue control. */}
                 <div className='py-2 flex items-center justify-center px-3 relative w-full'>
                     <DJCrossfaderSelfSub
                        onChange={handleCrossfaderChange}
                        width={-1}
                        responsive
                     />
                 </div>

                 {/* Headphone Mix (CUE/Master blend) */}
                 <div className='flex items-center justify-center py-2 border-t border-[#222]/60 w-full'>
                   <div className='flex flex-col items-center'>
                     <span className='text-[10px] text-[#555] font-bold tracking-widest mb-1'>CUE MIX</span>
                     <DJHeadphoneMix width={140} />
                   </div>
                 </div>

                 {/* Sampler Pads — always visible (the mixer column has been
                     widened + crossfader sits above so there's room) */}
                 <div className='px-2 pb-2 w-full'>
                   <DJSamplerPads />
                 </div>
             </div>
        </div>

        {/* === DECK B === */}
        <div
          className={`dj-deck flex-1 flex flex-col min-w-0 border-l border-[#2a2a2a] relative ${djActiveDeck === 'B' ? 'ring-1 ring-inset ring-purple-500/40' : ''} ${dragOverDeck === 'B' ? 'ring-2 ring-inset ring-purple-300/80 bg-purple-500/5' : ''}`}
          onDragOver={(e) => handleDeckDragOver(e, 'B')}
          onDragLeave={(e) => handleDeckDragLeave(e, 'B')}
          onDrop={(e) => handleDeckDrop(e, 'B')}
        >
            {/* Active Deck Indicator Bar */}
            {djActiveDeck === 'B' && <div className='absolute top-0 left-0 right-0 h-[2px] bg-purple-500 z-30' />}
            
             {/* Deck Header: mirror of A — jumbo BPM/Time on the LEFT, track info on the RIGHT */}
             <div className='bg-[#161616] border-b border-[#222]'>
                  {/* Track Info Row */}
                  <div className='dj-deck-info px-3 py-2 border-b border-[#1a1a1a]'>
                      <div className='flex items-center gap-3 flex-shrink-0'>
                        <DeckHasTrack deck='B'>
                          <div className='flex flex-col items-start font-mono leading-none'>
                            <DeckTimeDisplay deck='B' color='#d8b4fe' sizeClass='text-[20px]' />
                            <DeckTimeDisplay deck='B' color='#737373' sizeClass='text-[12px] mt-0.5' showRemaining />
                          </div>
                        </DeckHasTrack>
                        {/* Jumbo BPM display */}
                        <DeckBpmBadge deck='B' large />
                        <DJDeckStatusBar deck='B' />
                        {deckBKey && (
                          <span className='text-[12px] font-bold text-emerald-400 bg-emerald-500/15 px-2.5 py-1 rounded'>
                            {deckBKey}
                          </span>
                        )}
                        <DeckHorizontalVU getLevels={getDeckBLevels} width={120} channelHeight={5} />
                      </div>
                      <div className='flex items-center gap-2 min-w-0 flex-1 justify-end'>
                        {deckBTrack ? (
                          <div className='min-w-0 text-right'>
                            <div className='text-[14px] font-bold text-white truncate leading-tight'>
                              {deckBTrack.title || 'Unknown'}
                            </div>
                            <div className='text-[11px] text-neutral-400 truncate leading-tight'>
                              {deckBTrack.artist || 'Unknown Artist'}
                            </div>
                          </div>
                        ) : (
                          <span className='text-[11px] text-neutral-600 italic'>No track loaded</span>
                        )}
                        <span className='text-[10px] font-bold text-purple-400 uppercase tracking-wider flex-shrink-0 px-1.5 py-0.5 bg-purple-500/15 rounded'>DECK B</span>
                      </div>
                  </div>
                  {/* Controls Row: Grid + Beat Jump + Loop (mirrored, icon-only) */}
                  <div className='flex items-center gap-3 px-3 py-1 justify-end overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden'>
                      <DJBeatGridEdit deck='B' />
                      <DJBeatJump deck='B' compact />
                      <DJLoopSection deck='B' />
                  </div>
            </div>

            {/* Deck Jog Area — jog left, EQ strip right of jog (deck B mirrored) */}
            <div className='flex-1 relative bg-[#141414] flex items-stretch overflow-hidden'>
                
                {/* Jog Wheel — fills full remaining space */}
                <div className='flex-1 min-w-0 h-full flex items-center justify-center'>
                    <DJErrorBoundary componentName='DJJogWheel-B'>
                        <DJJogWheel deck='B' size={-1} responsive />
                    </DJErrorBoundary>
                </div>

                {/* EQ Strip — TRIM / HIGH / MID / LOW / FILTER beside jog */}
                <div className='w-[72px] flex-shrink-0 border-l border-[#1e1e1e] bg-[#0d0d0d]'>
                    <DJDeckEQStrip
                      deckId='B'
                      onEQChange={handleEQChange}
                      onVolumeChange={handleVolumeChange}
                      onFilterChange={handleFilterChange}
                    />
                </div>

                {/* Tempo Slider — slim outer edge */}
                <div className='w-16 flex-shrink-0 flex flex-col items-center justify-center py-3 gap-2 z-10'>
                     <DJNudgeButtons deck='B' onNudge={nudgePosition} disabled={!deckBTrack} />
                     <div className='flex-1 min-h-0 flex items-center justify-center'>
                     <DJTempoSliderSelfSub
                        deck='B'
                        onChange={(v) => handleTempoChange('B', v)}
                        disabled={!deckBTrack}
                        height={-1}
                        responsive
                     />
                     </div>
                </div>
            </div>

            {/* Deck Footer: Hot Cues row + Transport row */}
            <div className='bg-[#161616] border-t border-[#222] flex flex-col flex-shrink-0 shadow-[inset_0_8px_24px_rgba(0,0,0,0.6)]'>
                {/* Hot Cues — always reachable in the prime thumb zone */}
                <div className='flex items-center justify-center px-3 py-2 border-b border-[#1a1a1a]'>
                    <DJHotCuePad deck='B' singleRow />
                </div>
                {/* Transport row */}
                <div className='h-[80px] flex items-center justify-between px-4'>
                    {/* CUE point */}
                    <div className='flex flex-col items-start gap-0.5 w-16 flex-shrink-0'>
                        <span className='text-[9px] text-neutral-500 uppercase tracking-wider'>CUE</span>
                        <span className='text-[11px] font-mono text-yellow-400'>
                            {deckBCuePoint > 0 ? formatTime(deckBCuePoint) : '--:--'}
                        </span>
                    </div>
                    {/* Transport controls */}
                    <DJTransportButtons deck='B' />
                    {/* Loop status */}
                    <div className='flex flex-col items-end gap-0.5 w-16 flex-shrink-0'>
                        <span className='text-[9px] text-neutral-500 uppercase tracking-wider'>LOOP</span>
                        <span className={`text-[11px] font-mono ${deckBLoop.enabled ? 'text-green-400' : 'text-neutral-600'}`}>
                            {deckBLoop.enabled ? `${(deckBLoop.end - deckBLoop.start).toFixed(2)}s` : 'OFF'}
                        </span>
                    </div>
                </div>
            </div>
        </div>

      </div>

      <DJLibraryDrawer ref={libraryRef} />
    </div>
  );
};

// The application shell also renders on playback ticks. Preserve the DJ tree's
// narrow subscriptions across that parent boundary.
export const DJModeV2: React.FC = React.memo(() => {
  const ready = useIsDJReady();
  if (!ready) return <DJUnsupportedWidth minWidth={1440} variant="v2" />;
  return (
    <DJFullscreenGate>
      <DJModeV2Inner />
    </DJFullscreenGate>
  );
});

export default DJModeV2;
