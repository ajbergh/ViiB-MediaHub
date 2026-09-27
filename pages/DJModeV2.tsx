/**
 * ViiB MediaHub - DJ Mode Page V2
 *
 * Page orchestration for the DJv2 workstation: canvas scaling, recording,
 * keyboard shortcuts, dialogs, drag-and-drop loading and callback wiring.
 * Layout lives in DJPerformanceWorkspace, DJDeckPanel and DJMixerPanel.
 *
 * Layout (docs/DJV2_UI_REFACTOR_ENGINEERING_PLAN.md §3):
 * ┌──────────────────────────────────────────────────────────────────────┐
 * │ Top bar: DJ · LIBRARY · BROWSE · FX · SCOPE      REC MIDI AUDIO ⚙ FS │
 * ├──────────────────────────────────┬───────────────────────────────────┤
 * │ Deck A waveform lane (50%)       │ Deck B waveform lane (50%)        │
 * ├──────────────────────┬───────────┴──────────┬────────────────────────┤
 * │ Deck A               │ Mixer                │ Deck B                 │
 * ├──────────────────────┴──────────────────────┴────────────────────────┤
 * │ Library affordance (drawer overlays the workspace)                   │
 * └──────────────────────────────────────────────────────────────────────┘
 *
 * @module pages/DJModeV2
 */

import React, { useEffect, useLayoutEffect, useState, useCallback, useRef } from 'react';
import { useStore } from '../store';
import { useDJAudioEngineActions, useDJAudioEngineSync } from '../hooks/useDJAudioEngine';
import { DJTopBar, type DJUpperView } from '../components/dj/v2/DJTopBar';
import { DJSplitWaveform } from '../components/dj/v2/DJSplitWaveform';
import { DJPerformanceWorkspace } from '../components/dj/v2/DJPerformanceWorkspace';
import { DJDeckPanel } from '../components/dj/v2/DJDeckPanel';
import { DJMixerPanel } from '../components/dj/v2/DJMixerPanel';
import { DJMidiMapping } from '../components/dj/v2/DJMidiMapping';
import { DJAudioSetup } from '../components/dj/v2/DJAudioSetup';
import { DJErrorBoundary } from '../components/dj/v2/DJErrorBoundary';
import { DJScopeView } from '../components/dj/v2/DJScopeView';
import { DJLibraryDrawer, type DJLibraryDrawerHandle } from '../components/dj/v2/DJLibraryDrawer';
import { useDJShortcuts } from '../components/dj/v2/hooks/useDJShortcuts';
import { createLogger } from '../services/loggerService';
import type { DeckId, DeckEQ, DJLayoutMode } from '../slices/djMixerSlice';
import { useIsDJReady } from '../hooks/useMediaQuery';
import { DJUnsupportedWidth } from '../components/dj/DJUnsupportedWidth';
import { DJFullscreenGate } from '../components/dj/DJFullscreenGate';
import { useAnalysisPlaybackPressure } from '../hooks/useAnalysisPlaybackPressure';
import { useDJMidiActions } from '../hooks/useDJMidiActions';

const logger = createLogger('DJModeV2');

// The complete workstation is authored against the usable canvas beside the
// collapsed sidebar at 1920×1080. Scaling the whole canvas from this single
// reference keeps control dimensions and every inter-control gap proportional.
const DJ_CANVAS_WIDTH = 1856;
const DJ_CANVAS_HEIGHT = 1090;

const SHORTCUTS: ReadonlyArray<[string, string | null]> = [
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
  ['← / →', 'Adjust crossfader ±2%'],
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
  ['Ctrl+Scroll', 'Zoom the waveform lane under the pointer'],
  ['?', 'Toggle This Overlay'],
  ['Esc', 'Close Overlay'],
];

const DJModeV2Inner: React.FC = () => {
  const [upperView, setUpperView] = useState<DJUpperView>('waveform');
  const [libraryOpen, setLibraryOpen] = useState(false);
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
  const workstationViewportRef = useRef<HTMLDivElement>(null);
  const [canvas, setCanvas] = useState({ scale: 1, left: 0 });

  useLayoutEffect(() => {
    const viewport = workstationViewportRef.current;
    if (!viewport) return;

    const updateCanvasScale = () => {
      const { width, height } = viewport.getBoundingClientRect();
      const nextScale = Math.min(width / DJ_CANVAS_WIDTH, height / DJ_CANVAS_HEIGHT);
      const nextLeft = Math.max(0, (width - DJ_CANVAS_WIDTH * nextScale) / 2);
      setCanvas(current =>
        Math.abs(current.scale - nextScale) < 0.0001 && Math.abs(current.left - nextLeft) < 0.1
          ? current
          : { scale: nextScale, left: nextLeft }
      );
    };

    updateCanvasScale();
    const observer = new ResizeObserver(updateCanvasScale);
    observer.observe(viewport);
    return () => observer.disconnect();
  }, []);

  // While a deck is playing, tell the backend to yield background track
  // analysis so library preparation does not compete with the set.
  useAnalysisPlaybackPressure();
  const midiEnabled = useDJMidiActions();

  // Actions — stable Zustand references, never trigger re-renders
  const setDJMixerEnabled = useStore(s => s.setDJMixerEnabled);
  const toggleActiveDeck = useStore(s => s.toggleActiveDeck);
  const setHotCue = useStore(s => s.setHotCue);
  const triggerHotCue = useStore(s => s.triggerHotCue);
  const setDJLayoutMode = useStore(s => s.setDJLayoutMode);
  const setDeckFilter = useStore(s => s.setDeckFilter);
  const storeSetMasterVolume = useStore(s => s.setMasterVolume);

  // Render-path state — granular selectors prevent position-driven re-renders
  const djActiveDeck = useStore(s => s.djActiveDeck);
  const songs = useStore(s => s.songs);
  const djLayoutMode = useStore(s => s.djMixer?.djLayoutMode || 'perf') as DJLayoutMode;

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
      // Keep state on unmount for quick return, but stop recording if active
      if (recordingTimerRef.current) clearInterval(recordingTimerRef.current);
      if (mediaRecorderRef.current && mediaRecorderRef.current.state !== 'inactive') mediaRecorderRef.current.stop();
    };
  }, [setDJMixerEnabled]);

  // VU meter level getters (stable callbacks for rAF-based meters)
  const getMasterLevels = useCallback(() => getVULevels().master, [getVULevels]);
  const getDeckALevels = useCallback(() => getVULevels().deckA, [getVULevels]);
  const getDeckBLevels = useCallback(() => getVULevels().deckB, [getVULevels]);

  const handleEQChange = useCallback((deck: DeckId, band: keyof DeckEQ, value: number) => {
    setEQ(deck, band, value);
  }, [setEQ]);

  const handleCrossfaderChange = useCallback((value: number) => {
    setCrossfader(value);
  }, [setCrossfader]);

  // Recording handler - capture master output via MediaRecorder
  const handleRecordToggle = useCallback(() => {
    if (isRecording) {
      if (mediaRecorderRef.current && mediaRecorderRef.current.state !== 'inactive') mediaRecorderRef.current.stop();
      if (recordingTimerRef.current) {
        clearInterval(recordingTimerRef.current);
        recordingTimerRef.current = null;
      }
      setIsRecording(false);
      return;
    }

    const stream = getMasterStream();
    if (!stream) {
      logger.warn('Cannot record: no master stream available');
      return;
    }

    recordedChunksRef.current = [];
    const mimeType = MediaRecorder.isTypeSupported('audio/webm;codecs=opus') ? 'audio/webm;codecs=opus' : 'audio/webm';
    const recorder = new MediaRecorder(stream, { mimeType });
    recorder.ondataavailable = (e) => {
      if (e.data.size > 0) recordedChunksRef.current.push(e.data);
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

    const startTime = Date.now();
    recordingTimerRef.current = window.setInterval(() => {
      setRecordingDuration(Math.floor((Date.now() - startTime) / 1000));
    }, 1000);
  }, [isRecording, getMasterStream]);

  const handleVolumeChange = useCallback((deck: DeckId, value: number) => {
    setVolume(deck, value);
  }, [setVolume]);

  const handleTempoChange = useCallback((deck: DeckId, value: number) => {
    setTempo(deck, value);
  }, [setTempo]);

  // Sync handler - syncs BPM to the other deck (reads from store snapshot to avoid position deps)
  const handleSync = useCallback((deck: DeckId) => {
    const state = useStore.getState();
    const thisDeck = deck === 'A' ? state.djDeckA : state.djDeckB;
    const otherDeck = deck === 'A' ? state.djDeckB : state.djDeckA;
    if (!thisDeck.originalBpm || !otherDeck.effectiveBpm) return;
    const clampedTempo = Math.max(0.5, Math.min(1.5, otherDeck.effectiveBpm / thisDeck.originalBpm));
    setTempo(deck, clampedTempo);
    if (state.djMixer.syncMode === 'beat-phase') syncBeatPhase(deck);
  }, [setTempo, syncBeatPhase]);

  const handleLayoutModeChange = useCallback((mode: DJLayoutMode) => {
    setDJLayoutMode(mode);
    if (mode === 'browse') libraryRef.current?.open();
  }, [setDJLayoutMode]);

  // BROWSE means "library open"; closing the library returns to the DJ layout.
  const handleLibraryOpenChange = useCallback((open: boolean) => {
    setLibraryOpen(open);
    if (!open && useStore.getState().djMixer?.djLayoutMode === 'browse') setDJLayoutMode('perf');
  }, [setDJLayoutMode]);

  const toggleLibrary = useCallback(() => libraryRef.current?.toggle(), []);
  const openMidi = useCallback(() => setShowMidiMapping(true), []);
  const openAudio = useCallback(() => setShowAudioSetup(true), []);
  const openShortcuts = useCallback(() => setShowShortcuts(true), []);

  // Keyboard shortcuts — single-mount listener via useDJShortcuts
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
    const normalized = (knobValue + 24) / 36;
    storeSetMasterVolume(Math.max(0, Math.min(1, normalized)));
  }, [storeSetMasterVolume]);

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
    if (![...event.dataTransfer.types].some(type => type === 'application/x-viib-dj-track' || type === 'text/plain')) return;
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

  const deckProps = {
    onTempoChange: handleTempoChange,
    onEQChange: handleEQChange,
    onVolumeChange: handleVolumeChange,
    onFilterChange: handleFilterChange,
    onNudge: nudgePosition,
    onDragOver: handleDeckDragOver,
    onDragLeave: handleDeckDragLeave,
    onDrop: handleDeckDrop,
  };

  return (
    <div ref={workstationViewportRef} className='dj-viewport relative h-full overflow-hidden'>
      <div
        className='dj-workstation absolute top-0'
        data-dj-mode={djLayoutMode}
        data-dj-canvas
        style={{
          width: DJ_CANVAS_WIDTH,
          height: DJ_CANVAS_HEIGHT,
          left: canvas.left,
          transform: `scale(${canvas.scale})`,
          transformOrigin: 'top left',
        }}
      >
        <h1 className='sr-only'>DJ Mode</h1>

        {showShortcuts && (
          <div className='dj-dialog-backdrop' onClick={() => setShowShortcuts(false)} role="dialog" aria-modal="true" aria-label="Keyboard shortcuts">
            <div className='dj-dialog' onClick={e => e.stopPropagation()}>
              <div className='flex items-center justify-between mb-4'>
                <h2 className='text-lg font-bold text-text-main'>Keyboard Shortcuts</h2>
                <button onClick={() => setShowShortcuts(false)} className='dj-btn dj-btn-icon dj-btn-ghost' aria-label="Close shortcuts overlay">✕</button>
              </div>
              <div className='dj-shortcuts-grid'>
                {SHORTCUTS.map(([key, action], i) => {
                  if (key === '' && action === '') return <div key={i} className='col-span-2 h-1' />;
                  if (action === null) return <div key={i} className='dj-shortcuts-heading'>{key}</div>;
                  return (
                    <React.Fragment key={i}>
                      <span className='text-right pr-2'><kbd className='dj-kbd'>{key}</kbd></span>
                      <span>{action}</span>
                    </React.Fragment>
                  );
                })}
              </div>
              <p className='dj-label mt-4 text-center'>Press ? to toggle • Esc to close</p>
            </div>
          </div>
        )}

        <DJTopBar
          layoutMode={djLayoutMode}
          onLayoutModeChange={handleLayoutModeChange}
          upperView={upperView}
          onUpperViewChange={setUpperView}
          libraryOpen={libraryOpen}
          onToggleLibrary={toggleLibrary}
          isRecording={isRecording}
          recordingDuration={recordingDuration}
          onRecordToggle={handleRecordToggle}
          midiEnabled={midiEnabled}
          onOpenMidi={openMidi}
          onOpenAudio={openAudio}
          onShowShortcuts={openShortcuts}
        />

        {showAudioSetup && <DJAudioSetup onClose={() => setShowAudioSetup(false)} />}
        {showMidiMapping && <DJMidiMapping onClose={() => setShowMidiMapping(false)} />}

        <DJPerformanceWorkspace
          upper={upperView === 'scope' ? (
            <div className='dj-upper-scope'><DJScopeView getVULevels={getVULevels} /></div>
          ) : (
            <DJErrorBoundary componentName='DJSplitWaveform'><DJSplitWaveform /></DJErrorBoundary>
          )}
          deckA={<DJDeckPanel deck='A' isActive={djActiveDeck === 'A'} isDragOver={dragOverDeck === 'A'} {...deckProps} />}
          mixer={(
            <DJMixerPanel
              getDeckALevels={getDeckALevels}
              getDeckBLevels={getDeckBLevels}
              getMasterLevels={getMasterLevels}
              onVolumeChange={handleVolumeChange}
              onMasterVolumeChange={handleMasterVolumeChange}
              onCrossfaderChange={handleCrossfaderChange}
            />
          )}
          deckB={<DJDeckPanel deck='B' isActive={djActiveDeck === 'B'} isDragOver={dragOverDeck === 'B'} {...deckProps} />}
        />

        <DJLibraryDrawer ref={libraryRef} onOpenChange={handleLibraryOpenChange} />
      </div>
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
