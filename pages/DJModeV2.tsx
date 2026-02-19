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
 * │                         LIBRARY BROWSER                                 │
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
import { DJEQKnob } from '../components/dj/v2/DJEQKnob';
import { DJVolumeFader } from '../components/dj/v2/DJVolumeFader';
import { DJCrossfader } from '../components/dj/v2/DJCrossfader';
import { DJTempoSlider } from '../components/dj/v2/DJTempoSlider';
import { DJCueButton } from '../components/dj/v2/DJCueButton';
import { DJHeadphoneMix } from '../components/dj/v2/DJHeadphoneMix';
import { DJLibraryBrowserV2 } from '../components/dj/v2/DJLibraryBrowserV2';
import { DJFXSection } from '../components/dj/v2/DJFXSection';
import { DJStereoVUMeter } from '../components/dj/v2/DJVUMeter';
import { DJBeatJump } from '../components/dj/v2/DJBeatJump';
import { DJBeatGridEdit } from '../components/dj/v2/DJBeatGridEdit';
import { DJSamplerPads } from '../components/dj/v2/DJSamplerPads';
import { DJMidiMapping } from '../components/dj/v2/DJMidiMapping';
import { DJErrorBoundary } from '../components/dj/v2/DJErrorBoundary';
import { DJScopeView } from '../components/dj/v2/DJScopeView';
import { DJDeckStatusBar } from '../components/dj/v2/DJDeckStatusBar';
import { createLogger } from '../services/loggerService';
import type { DeckId, DeckEQ, DJLayoutMode } from '../slices/djMixerSlice';

const logger = createLogger('DJModeV2');

type ViewMode = 'timeline' | 'scope' | 'fx';

// Self-subscribing time display to avoid parent re-renders from position updates
const DeckTimeDisplay = React.memo(({ deck, color, showRemaining = false }: { deck: DeckId; color: string; showRemaining?: boolean }) => {
  const containerRef = useRef<HTMLDivElement>(null);
  
  useEffect(() => {
    let animId: number;
    let timeoutId: ReturnType<typeof setTimeout>;
    let lastPos = -1;
    const update = () => {
      const state = useStore.getState();
      const d = deck === 'A' ? state.djDeckA : state.djDeckB;
      const el = containerRef.current;
      if (el && d.duration > 0) {
        const pos = d.position;
        const remaining = d.duration - pos;
        const formatT = (s: number, neg = false) => {
          const m = Math.floor(Math.abs(s) / 60);
          const sec = Math.floor(Math.abs(s) % 60);
          return `${neg ? '-' : ''}${m}:${sec.toString().padStart(2, '0')}`;
        };
        if (showRemaining) {
          el.textContent = formatT(remaining, true);
        } else {
          el.textContent = formatT(pos);
        }
        // Throttle to ~4fps when paused (position unchanging)
        if (!d.isPlaying && pos === lastPos) {
          timeoutId = setTimeout(() => { animId = requestAnimationFrame(update); }, 250);
        } else {
          animId = requestAnimationFrame(update);
        }
        lastPos = pos;
      } else {
        // No track loaded — throttle to ~2fps instead of 60fps
        timeoutId = setTimeout(() => { animId = requestAnimationFrame(update); }, 500);
      }
    };
    animId = requestAnimationFrame(update);
    return () => { cancelAnimationFrame(animId); clearTimeout(timeoutId); };
  }, [deck, showRemaining]);
  
  return <span ref={containerRef} className={`text-[10px] font-mono`} style={{ color }}>--:--</span>;
});
DeckTimeDisplay.displayName = 'DeckTimeDisplay';

// Self-subscribing deck duration check (avoids position subscription in parent)
const DeckHasTrack = React.memo(({ deck, children }: { deck: DeckId; children: React.ReactNode }) => {
  const hasTrack = useStore(state => deck === 'A' ? !!state.djDeckA.track : !!state.djDeckB.track);
  const hasDuration = useStore(state => deck === 'A' ? state.djDeckA.duration > 0 : state.djDeckB.duration > 0);
  if (!hasTrack || !hasDuration) return null;
  return <>{children}</>;
});
DeckHasTrack.displayName = 'DeckHasTrack';

// Self-subscribing Channel Strip — isolates EQ/filter/volume from parent re-renders
const DJChannelStrip = React.memo(({ deckId, getDeckLevels, onEQChange, onVolumeChange, onFilterChange }: {
  deckId: DeckId;
  getDeckLevels: () => { left: number; right: number };
  onEQChange: (deck: DeckId, band: keyof DeckEQ, value: number) => void;
  onVolumeChange: (deck: DeckId, value: number) => void;
  onFilterChange: (deck: DeckId, knobValue: number) => void;
}) => {
  const isA = deckId === 'A';
  const accentColor = isA ? '#3b82f6' : '#8b5cf6';

  // Granular selectors — only re-render when these specific fields change
  const eqHigh = useStore(s => isA ? s.djDeckA.eq.high : s.djDeckB.eq.high);
  const eqMid = useStore(s => isA ? s.djDeckA.eq.mid : s.djDeckB.eq.mid);
  const eqLow = useStore(s => isA ? s.djDeckA.eq.low : s.djDeckB.eq.low);
  const filterValue = useStore(s => isA ? s.djDeckA.filter.value : s.djDeckB.filter.value);
  const filterEnabled = useStore(s => isA ? s.djDeckA.filter.enabled : s.djDeckB.filter.enabled);
  const volume = useStore(s => isA ? s.djDeckA.volume : s.djDeckB.volume);
  const isPlaying = useStore(s => isA ? s.djDeckA.isPlaying : s.djDeckB.isPlaying);

  const filterKnobValue = ((filterValue + 1) / 2) * 36 - 24;

  return (
    <div className={`flex-1 flex flex-col items-center py-2 gap-3 min-w-[90px] border-[#2a2a2a]`}>
      {/* Gain / Trim */}
      <div className='relative pt-2'>
        <DJEQKnob
          label='TRIM'
          value={0}
          onChange={(v) => {
            const normalized = (v + 24) / 36;
            const trimmedVol = Math.max(0, Math.min(1, normalized * 1.5));
            onVolumeChange(deckId, trimmedVol);
          }}
          color='#aaaaaa'
          size={38}
        />
      </div>

      {/* EQ Section */}
      <div className='flex flex-col gap-2 p-1.5 bg-[#151515] rounded-md border border-[#222] shadow-inner'>
        <DJEQKnob
          label='HIGH'
          value={eqHigh}
          onChange={(v) => onEQChange(deckId, 'high', v)}
          color='#06b6d4'
          size={38}
        />
        <DJEQKnob
          label='MID'
          value={eqMid}
          onChange={(v) => onEQChange(deckId, 'mid', v)}
          color='#22c55e'
          size={38}
        />
        <DJEQKnob
          label='LOW'
          value={eqLow}
          onChange={(v) => onEQChange(deckId, 'low', v)}
          color='#f59e0b'
          size={38}
        />
      </div>

      {/* Separator */}
      <div className='h-px bg-[#333] mx-1' />

      {/* Filter Knob */}
      <div className='p-1.5 bg-[#151515] rounded-md border border-[#222] shadow-inner'>
        <DJEQKnob
          label='FILTER'
          value={filterKnobValue}
          onChange={(v) => onFilterChange(deckId, v)}
          color={filterEnabled ? '#ef4444' : '#666'}
          size={38}
        />
      </div>

      {/* Spacer */}
      <div className='flex-1'></div>

      {/* Headphone Cue */}
      <div className='mb-2'>
        <DJCueButton deck={deckId} />
      </div>

      {/* Channel Fader with VU */}
      <div className='w-full px-2 flex items-center justify-center gap-1'>
        <DJStereoVUMeter
          getLevels={getDeckLevels}
          height={140}
          channelWidth={4}
          gap={2}
          segments={18}
          showPeak={true}
        />
        <DJVolumeFader
          value={volume}
          onChange={(v) => onVolumeChange(deckId, v)}
          label=''
          height={160}
          isPlaying={isPlaying}
          accentColor={accentColor}
        />
      </div>
    </div>
  );
});
DJChannelStrip.displayName = 'DJChannelStrip';

// Self-subscribing BPM badge — avoids parent re-render on effectiveBpm change
const DeckBpmBadge = React.memo(({ deck }: { deck: DeckId }) => {
  const bpm = useStore(s => deck === 'A' ? s.djDeckA.effectiveBpm : s.djDeckB.effectiveBpm);
  if (!bpm) return null;
  return <span className='text-[9px] font-mono text-neutral-400'>{bpm.toFixed(1)}</span>;
});
DeckBpmBadge.displayName = 'DeckBpmBadge';

// Self-subscribing master volume knob — avoids parent re-render during volume drag
const DJMasterKnob = React.memo(({ onChange }: { onChange: (v: number) => void }) => {
  const masterVolume = useStore(s => s.djMixer?.masterVolume ?? 0.8);
  return (
    <DJEQKnob
      label='MAIN'
      value={((masterVolume) * 36 - 24)}
      onChange={onChange}
      size={40}
      color='#fff'
    />
  );
});
DJMasterKnob.displayName = 'DJMasterKnob';

// Self-subscribing crossfader — avoids parent re-render during crossfader drag
const DJCrossfaderSelfSub = React.memo(({ onChange, width, responsive }: {
  onChange: (v: number) => void;
  width: number;
  responsive?: boolean;
}) => {
  const crossfader = useStore(s => s.djMixer?.crossfader ?? 0);
  return <DJCrossfader value={crossfader} onChange={onChange} width={width} responsive={responsive} />;
});
DJCrossfaderSelfSub.displayName = 'DJCrossfaderSelfSub';

// Self-subscribing tempo slider — avoids parent re-render during tempo drag
const DJTempoSliderSelfSub = React.memo(({ deck, onChange, disabled, height, responsive }: {
  deck: DeckId;
  onChange: (v: number) => void;
  disabled?: boolean;
  height: number;
  responsive?: boolean;
}) => {
  const isA = deck === 'A';
  const tempo = useStore(s => isA ? s.djDeckA.tempo : s.djDeckB.tempo);
  const effectiveBpm = useStore(s => isA ? s.djDeckA.effectiveBpm : s.djDeckB.effectiveBpm);
  const originalBpm = useStore(s => isA ? s.djDeckA.originalBpm : s.djDeckB.originalBpm);
  return (
    <DJTempoSlider
      deck={deck}
      value={tempo}
      onChange={onChange}
      originalBpm={originalBpm}
      effectiveBpm={effectiveBpm}
      disabled={disabled}
      height={height}
      responsive={responsive}
    />
  );
});
DJTempoSliderSelfSub.displayName = 'DJTempoSliderSelfSub';

export const DJModeV2: React.FC = () => {
  const [viewMode, setViewMode] = useState<ViewMode>('timeline');
  const [isRecording, setIsRecording] = useState(false);
  const [libraryHeight, setLibraryHeight] = useState(250);
  const [showShortcuts, setShowShortcuts] = useState(false);
  const [showMidiMapping, setShowMidiMapping] = useState(false);
  const [libraryCollapsed, setLibraryCollapsed] = useState(false);
  const [recordingDuration, setRecordingDuration] = useState(0);
  const dragRef = useRef<{ startY: number; startHeight: number } | null>(null);
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

  // Deck A header info (excludes position/duration/hotCues — those update at 15fps)
  const deckATrack = useStore(s => s.djDeckA.track);
  const deckAKey = useStore(s => s.djDeckA.key);

  // Deck B header info
  const deckBTrack = useStore(s => s.djDeckB.track);
  const deckBKey = useStore(s => s.djDeckB.key);

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
    togglePlay, 
    returnToCue, 
    setCrossfader, 
    seek, 
    setVolume,
    setEQ,
    setTempo,
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

  // Adjust library height defaults when layout mode changes
  useEffect(() => {
    if (djLayoutMode === 'browse') {
      setLibraryHeight(prev => Math.max(prev, 350));
    } else if (djLayoutMode === 'perf') {
      setLibraryHeight(prev => Math.min(prev, 250));
    }
  }, [djLayoutMode]);

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

  // Keyboard shortcuts (reads state snapshot to avoid position-driven deps)
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Ignore if typing in input
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) {
        return;
      }

      // Read current state snapshot (not reactive - avoids position-driven re-binding)
      const state = useStore.getState();
      const activeDeck = state.djActiveDeck;

      // Hot cue keys 1-8 (use e.code to work with Shift held - e.key changes to !@# etc.)
      const digitMatch = e.code.match(/^Digit([1-8])$/);
      if (digitMatch) {
        const slot = parseInt(digitMatch[1]);
        const deck = activeDeck;
        const deckState = deck === 'A' ? state.djDeckA : state.djDeckB;
        if (!deckState.track) return;
        
        if (e.shiftKey) {
          // Set hot cue at current position
          const HOT_CUE_COLORS = ['#22c55e', '#22c55e', '#22c55e', '#eab308', '#f97316', '#3b82f6', '#8b5cf6', '#ec4899'];
          const color = HOT_CUE_COLORS[slot - 1] || '#22c55e';
          setHotCue(deck, slot, deckState.position, undefined, color);
        } else {
          // Trigger existing hot cue
          const hotCue = deckState.hotCues.find(hc => hc.slot === slot);
          if (hotCue) {
            seek(deck, hotCue.position);
            triggerHotCue(deck, slot);
          }
        }
        return;
      }

      switch (e.key.toLowerCase()) {
        case 'q': returnToCue('A'); break;
        case 'w': togglePlay('A'); break;
        case 'o': returnToCue('B'); break;
        case 'p': togglePlay('B'); break;
        case ' ':
          e.preventDefault();
          togglePlay(activeDeck);
          break;
        case 'tab':
          e.preventDefault();
          toggleActiveDeck();
          break;
        case 'z': setCrossfader(-1); break;
        case 'x': setCrossfader(0); break;
        case 'c': setCrossfader(1); break;
        case 'e': handleSync('A'); break;
        case '[': handleSync('B'); break;
        case 'arrowleft':
          if (e.shiftKey) {
            setCrossfader(Math.max(-1, state.djMixer.crossfader - 0.1));
          }
          break;
        case 'arrowright':
          if (e.shiftKey) {
            setCrossfader(Math.min(1, state.djMixer.crossfader + 0.1));
          }
          break;
        case 'f11':
          e.preventDefault();
          if (document.fullscreenElement) {
            document.exitFullscreen();
          } else {
            document.documentElement.requestFullscreen();
          }
          break;
        case '?':
          setShowShortcuts(prev => !prev);
          break;
        case '/':
          e.preventDefault();
          // Focus the library browser search input
          const searchInput = document.querySelector('[data-dj-mode] input[type="text"][placeholder*="Search"]') as HTMLInputElement;
          if (searchInput) searchInput.focus();
          break;
        case 'escape':
          setShowShortcuts(false);
          break;
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [togglePlay, returnToCue, setCrossfader, toggleActiveDeck, seek, setHotCue, triggerHotCue, handleSync]);

  // Filter knob handler - maps -24..+12 knob range to -1..+1 filter value
  const handleFilterChange = useCallback((deck: DeckId, knobValue: number) => {
    // Map knob range (-24 to +12) to filter range (-1 to +1)
    const normalized = (knobValue + 24) / 36; // 0 to 1
    const filterValue = normalized * 2 - 1;   // -1 to +1
    const clamped = Math.max(-1, Math.min(1, filterValue));
    
    // Single store write — setDeckFilter sets both value and enabled (threshold 0.05)
    setDeckFilter(deck, clamped);
    
    // Update audio engine filter
    const isNeutral = Math.abs(clamped) < 0.05;
    if (isNeutral) {
      setFilterFX(deck, false, 'lowpass', 20000, 0.5);
    } else {
      if (clamped < 0) {
        // Low-pass: map -1..0 to 200Hz..20000Hz
        const freq = 200 * Math.pow(100, 1 + clamped);
        setFilterFX(deck, true, 'lowpass', freq, 2);
      } else {
        // High-pass: map 0..+1 to 20Hz..8000Hz
        const freq = 20 + clamped * 7980;
        setFilterFX(deck, true, 'highpass', freq, 2);
      }
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

  return (
    <div
      className='h-full flex flex-col overflow-hidden min-w-[1024px] transition-[grid-template-rows] duration-200'
      data-dj-mode={djLayoutMode}
      style={{
        backgroundColor: 'var(--dj-bg)',
      }}
    >
      
      {/* Keyboard Shortcuts Overlay */}
      {showShortcuts && (
        <div className='fixed inset-0 z-[100] bg-black/80 backdrop-blur-sm flex items-center justify-center' onClick={() => setShowShortcuts(false)} role="dialog" aria-modal="true" aria-label="Keyboard shortcuts">
          <div className='bg-[#1a1a1a] border border-[#333] rounded-lg p-6 max-w-lg w-full max-h-[80vh] overflow-y-auto' onClick={e => e.stopPropagation()}>
            <div className='flex items-center justify-between mb-4'>
              <h2 className='text-lg font-bold text-white'>Keyboard Shortcuts</h2>
              <button onClick={() => setShowShortcuts(false)} className='text-neutral-500 hover:text-white text-xl' aria-label="Close shortcuts overlay">✕</button>
            </div>
            <div className='grid grid-cols-2 gap-x-6 gap-y-0.5 text-[11px]'>
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
                ['Shift+←', 'Nudge Left'],
                ['Shift+→', 'Nudge Right'],
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
                ['/', 'Focus Search Input'],
                ['', ''],
                ['View', null],
                ['F11', 'Toggle Fullscreen'],
                ['Ctrl+Scroll', 'Zoom Waveform'],
                ['?', 'Toggle This Overlay'],
                ['Esc', 'Close Overlay'],
              ].map(([key, action], i) => {
                if (key === '' && action === '') return <div key={i} className='col-span-2 h-1' />;
                if (action === null) return <div key={i} className='col-span-2 text-[10px] font-bold text-brand uppercase tracking-widest mt-2 mb-1 border-b border-[#333] pb-1'>{key}</div>;
                return (
                  <React.Fragment key={i}>
                    <span className='text-right pr-2'>
                      <kbd className='px-1.5 py-0.5 bg-[#333] rounded text-neutral-300 font-mono text-[10px] border border-[#444]'>{key}</kbd>
                    </span>
                    <span className='text-neutral-400'>{action}</span>
                  </React.Fragment>
                );
              })}
            </div>
            <p className='text-[9px] text-neutral-600 mt-4 text-center'>Press ? to toggle • Esc to close</p>
          </div>
        </div>
      )}

      {/* 1. TOP BAR */}
      <div className='relative'>
        <DJTopBar 
          viewMode={viewMode}
          onViewModeChange={setViewMode}
          isRecording={isRecording}
          onRecordToggle={handleRecordToggle}
          layoutMode={djLayoutMode}
          onLayoutModeChange={setDJLayoutMode}
        />
        {/* MIDI button */}
        <button
          onClick={() => setShowMidiMapping(true)}
          className='absolute right-12 top-1/2 -translate-y-1/2 px-2 py-1 text-[9px] font-bold uppercase tracking-wider
            bg-[#222] text-neutral-500 border border-[#333] rounded hover:bg-[#2a2a2a] hover:text-neutral-300 transition-colors'
          title='MIDI Controller Mapping'
        >
          🎹 MIDI
        </button>
      </div>

      {/* MIDI Mapping Dialog */}
      {showMidiMapping && <DJMidiMapping onClose={() => setShowMidiMapping(false)} />}

      {/* 2. WAVEFORM / SCOPE / FX VIEW */}
      {viewMode === 'timeline' && (
        <div className='flex-shrink-0 border-b border-[#2a2a2a] relative' style={{ height: 'var(--dj-waveform-h)', backgroundColor: 'var(--dj-bg)' }}>
          {/* WebGL / Canvas 2D toggle */}
          <button
            onClick={toggleWebGLWaveform}
            className={`absolute top-1 right-1 z-10 px-1.5 py-0.5 rounded text-[9px] font-bold uppercase transition-colors ${
              useWebGLWaveform
                ? 'bg-green-600/30 text-green-400 border border-green-500/40'
                : 'bg-neutral-800/80 text-neutral-500 border border-neutral-700 hover:text-neutral-300'
            }`}
            title={useWebGLWaveform ? 'WebGL waveform active — click for Canvas 2D' : 'Canvas 2D waveform — click for WebGL'}
          >
            {useWebGLWaveform ? 'WebGL' : '2D'}
          </button>
          <DJErrorBoundary componentName='DJWaveform'>
            {useWebGLWaveform ? (
              <DJWebGLWaveform height={180} allowFallback />
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
      {viewMode === 'fx' && (
        <div className='flex-shrink-0 border-b border-[#2a2a2a]' style={{ height: 'var(--dj-waveform-h)', backgroundColor: 'var(--dj-bg)' }}>
          <div className='h-full flex flex-col'>
            <DJFXSection />
            <div className='flex-1 flex items-center justify-center text-neutral-600 text-xs'>
              Expanded FX view — use knobs above to control effects
            </div>
          </div>
        </div>
      )}

      {/* 3. FX SECTION (always shown in timeline/scope mode) */}
      {viewMode !== 'fx' && <DJFXSection />}

      {/* 4. MAIN CONTROL DECK (Decks + Mixer) */}
      <div className='flex-1 flex min-h-0' style={{ backgroundColor: 'var(--dj-bg)' }}>
        
        {/* === DECK A === */}
        <div className={`flex-1 flex flex-col min-w-0 border-r border-[#2a2a2a] relative ${djActiveDeck === 'A' ? 'ring-1 ring-inset ring-blue-500/40' : ''}`}>
            {/* Active Deck Indicator Bar */}
            {djActiveDeck === 'A' && <div className='absolute top-0 left-0 right-0 h-[2px] bg-blue-500 z-30' />}
            
            {/* Deck Header: Track Info + Loops / Pads */}
            <div className='bg-[#161616] border-b border-[#222]'>
                  {/* Track Info Bar */}
                  <div className='flex items-center justify-between px-4 py-1.5 border-b border-[#1a1a1a]'>
                      <div className='flex items-center gap-2 min-w-0 flex-1'>
                        <span className='text-[9px] font-bold text-blue-400 uppercase tracking-wider flex-shrink-0'>DECK A</span>
                        {deckATrack ? (
                          <div className='min-w-0 flex-1'>
                            <div className='text-[11px] font-bold text-white truncate leading-tight'>
                              {deckATrack.title || 'Unknown'}
                            </div>
                            <div className='text-[9px] text-neutral-400 truncate leading-tight'>
                              {deckATrack.artist || 'Unknown Artist'}
                            </div>
                          </div>
                        ) : (
                          <span className='text-[9px] text-neutral-600 italic'>No track loaded</span>
                        )}
                      </div>
                      <div className='flex items-center gap-2 flex-shrink-0'>
                        {deckAKey && (
                          <span className='text-[9px] font-bold text-emerald-400 bg-emerald-500/15 px-1.5 py-0.5 rounded'>
                            {deckAKey}
                          </span>
                        )}
                        <DJDeckStatusBar deck='A' />
                        <DeckBpmBadge deck='A' />
                        <DeckHasTrack deck='A'>
                          <div className='flex items-center gap-1.5 font-mono'>
                            <DeckTimeDisplay deck='A' color='#93c5fd' />
                            <span className='text-[8px] text-neutral-600'>/</span>
                            <DeckTimeDisplay deck='A' color='#737373' showRemaining />
                          </div>
                        </DeckHasTrack>
                      </div>
                  </div>
                  {/* Controls Row */}
                  <div className='flex items-center justify-between px-4 py-1.5'>
                      <div className='flex flex-col gap-0.5'>
                          <span className='text-[8px] font-bold text-[#555] uppercase tracking-wider'>LOOP</span>
                          <DJLoopSection deck='A' />
                      </div>
                      <DJBeatJump deck='A' />
                      <DJBeatGridEdit deck='A' />
                      <div className='flex flex-col gap-0.5 items-end'>
                          <span className='text-[8px] font-bold text-[#555] uppercase tracking-wider'>HOT CUES</span>
                          <DJHotCuePad deck='A' />
                      </div>
                  </div>
            </div>

            {/* Deck Jog Area */}
            <div className='flex-1 relative bg-[#141414] flex items-center justify-center overflow-hidden'>
                {/* Tempo Slider (Left for Deck A to mirror outer edge if deck A is left) */}
                <div className='absolute left-4 top-4 bottom-4 w-12 z-10'>
                     <DJTempoSliderSelfSub
                        deck='A'
                        onChange={(v) => handleTempoChange('A', v)}
                        disabled={!deckATrack}
                        height={-1}
                        responsive
                     />
                </div>

                {/* Jog Wheel */}
                <div className='pl-16 pr-4 w-full h-full flex items-center justify-center'>
                    <DJErrorBoundary componentName='DJJogWheel-A'>
                        <DJJogWheel deck='A' size={-1} responsive />
                    </DJErrorBoundary>
                </div>
            </div>

            {/* Deck Footer: Transport */}
            <div className='h-[60px] bg-[#161616] border-t border-[#222] flex items-center justify-center gap-4 shadow-[-inset_0_10px_20px_rgba(0,0,0,0.5)]'>
                 <DJTransportButtons deck='A' />
            </div>
        </div>

        {/* === MIXER CENTER (Fixed Width) === */}
        <div className='flex-shrink-0 flex flex-col border-x border-[#333] shadow-2xl z-20 relative' style={{ width: 'var(--dj-mixer-w)', backgroundColor: 'var(--dj-surface-1, #181818)' }}>
             {/* Mixer Body */}
             <div className='flex-1 flex w-full relative'>
                 {/* Channel A */}
                 <DJChannelStrip
                   deckId='A'
                   getDeckLevels={getDeckALevels}
                   onEQChange={handleEQChange}
                   onVolumeChange={handleVolumeChange}
                   onFilterChange={handleFilterChange}
                 />

                 {/* Center Master Strip */}
                 <div className='w-[90px] bg-[#131313] flex flex-col items-center py-3 border-x border-[#222]/50'>
                      <div className='text-[9px] text-[#444] font-bold mb-2 tracking-widest'>MASTER</div>
                      
                      {/* OUTPUT Section */}
                      <div className='text-[7px] text-[#555] font-bold tracking-widest mb-1'>OUTPUT</div>

                      {/* Real-time Master VU Meters */}
                      <div className='flex-1 flex items-center justify-center mb-3'>
                           <DJStereoVUMeter
                             getLevels={getMasterLevels}
                             height={100}
                             channelWidth={5}
                             gap={2}
                             segments={20}
                             showPeak={true}
                           />
                      </div>

                      {/* Separator */}
                      <div className='w-8 h-px bg-[#333] mb-3' />

                      {/* MONITOR Section */}
                      <div className='text-[7px] text-[#555] font-bold tracking-widest mb-1'>MONITOR</div>

                      {/* Headphone Mix Knob */}
                      <div className='mb-4 flex flex-col items-center'>
                          <span className='text-[8px] text-[#666] mb-1 font-bold'>CUE MIX</span>
                          <div className='scale-75 origin-center'>
                            <DJHeadphoneMix width={60} />
                          </div>
                      </div>
                      
                      {/* Master Volume */}
                      <div className='mb-2'>
                           <DJMasterKnob onChange={handleMasterVolumeChange} />
                      </div>
                 </div>

                 {/* Channel B */}
                 <DJChannelStrip
                   deckId='B'
                   getDeckLevels={getDeckBLevels}
                   onEQChange={handleEQChange}
                   onVolumeChange={handleVolumeChange}
                   onFilterChange={handleFilterChange}
                 />
             </div>

             {/* Sync Mode + Crossfader Section */}
             <div className='bg-[#1a1a1a] border-t border-[#333] flex flex-col items-center'>
                 {/* Sync Mode Selector + Quantize */}
                 <div className='flex gap-1 py-2 items-center'>
                   {(['off', 'bpm', 'beat-phase'] as const).map(mode => (
                     <button
                       key={mode}
                       onClick={() => setSyncMode(mode)}
                       className={`
                         px-3 py-1 rounded text-[9px] font-bold uppercase tracking-wider
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
                     className={`
                       px-2.5 py-1 rounded text-[9px] font-bold uppercase tracking-wider
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
                 <div className='flex gap-1 items-center'>
                   <span className='text-[8px] text-neutral-600 uppercase tracking-wider mr-0.5'>Curve</span>
                   {(['linear', 'constant-power', 'sharp'] as const).map(curve => (
                     <button
                       key={curve}
                       onClick={() => setCrossfaderCurve(curve)}
                       className={`
                         px-1.5 py-0.5 rounded text-[8px] font-bold uppercase tracking-wider
                         transition-all duration-100 border
                         ${crossfaderCurve === curve
                           ? 'bg-orange-600/80 text-white border-orange-500/60'
                           : 'bg-[#222] text-neutral-600 border-[#333] hover:bg-[#2a2a2a] hover:text-neutral-400'}
                       `}
                       title={`Crossfader curve: ${curve}`}
                     >
                       {curve === 'constant-power' ? 'CP' : curve === 'linear' ? 'LIN' : 'CUT'}
                     </button>
                   ))}
                 </div>

                 {/* Crossfader */}
                 <div className='h-[50px] flex items-center justify-center px-8 relative w-full'>
                     <span className='absolute left-4 text-[10px] font-bold text-[#333]'>A</span>
                     <span className='absolute right-4 text-[10px] font-bold text-[#333]'>B</span>
                     
                     <DJCrossfaderSelfSub
                        onChange={handleCrossfaderChange}
                        width={-1}
                        responsive
                     />
                 </div>
                 
                 {/* Sampler Pads */}
                 <div className='px-2 pb-2'>
                   <DJSamplerPads />
                 </div>
             </div>
        </div>

        {/* === DECK B === */}
        <div className={`flex-1 flex flex-col min-w-0 border-l border-[#2a2a2a] relative ${djActiveDeck === 'B' ? 'ring-1 ring-inset ring-purple-500/40' : ''}`}>
            {/* Active Deck Indicator Bar */}
            {djActiveDeck === 'B' && <div className='absolute top-0 left-0 right-0 h-[2px] bg-purple-500 z-30' />}
            
             {/* Deck Header: Track Info + Loops / Pads */}
             <div className='bg-[#161616] border-b border-[#222]'>
                  {/* Track Info Bar */}
                  <div className='flex items-center justify-between px-4 py-1.5 border-b border-[#1a1a1a]'>
                      <div className='flex items-center gap-2 flex-shrink-0'>
                        <DeckHasTrack deck='B'>
                          <div className='flex items-center gap-1.5 font-mono'>
                            <DeckTimeDisplay deck='B' color='#737373' showRemaining />
                            <span className='text-[8px] text-neutral-600'>/</span>
                            <DeckTimeDisplay deck='B' color='#d8b4fe' />
                          </div>
                        </DeckHasTrack>
                        <DeckBpmBadge deck='B' />
                        {deckBKey && (
                          <span className='text-[9px] font-bold text-emerald-400 bg-emerald-500/15 px-1.5 py-0.5 rounded'>
                            {deckBKey}
                          </span>
                        )}
                        <DJDeckStatusBar deck='B' />
                      </div>
                      <div className='flex items-center gap-2 min-w-0 flex-1 justify-end'>
                        {deckBTrack ? (
                          <div className='min-w-0 text-right'>
                            <div className='text-[11px] font-bold text-white truncate leading-tight'>
                              {deckBTrack.title || 'Unknown'}
                            </div>
                            <div className='text-[9px] text-neutral-400 truncate leading-tight'>
                              {deckBTrack.artist || 'Unknown Artist'}
                            </div>
                          </div>
                        ) : (
                          <span className='text-[9px] text-neutral-600 italic'>No track loaded</span>
                        )}
                        <span className='text-[9px] font-bold text-purple-400 uppercase tracking-wider flex-shrink-0'>DECK B</span>
                      </div>
                  </div>
                  {/* Controls Row */}
                  <div className='flex items-center justify-between px-4 py-1.5'>
                      <div className='flex flex-col gap-0.5'>
                          <span className='text-[8px] font-bold text-[#555] uppercase tracking-wider'>HOT CUES</span>
                          <DJHotCuePad deck='B' />
                      </div>
                      <DJBeatJump deck='B' />
                      <DJBeatGridEdit deck='B' />
                      <div className='flex flex-col gap-0.5 items-end'>
                          <span className='text-[8px] font-bold text-[#555] uppercase tracking-wider'>LOOP</span>
                          <DJLoopSection deck='B' />
                      </div>
                  </div>
            </div>

            {/* Deck Jog Area */}
            <div className='flex-1 relative bg-[#141414] flex items-center justify-center overflow-hidden'>
                
                {/* Jog Wheel */}
                <div className='pr-16 pl-4 w-full h-full flex items-center justify-center'>
                    <DJErrorBoundary componentName='DJJogWheel-B'>
                        <DJJogWheel deck='B' size={-1} responsive />
                    </DJErrorBoundary>
                </div>

                {/* Tempo Slider - Right Edge for Deck B */}
                <div className='absolute right-4 top-4 bottom-4 w-12 z-10'>
                     <DJTempoSliderSelfSub
                        deck='B'
                        onChange={(v) => handleTempoChange('B', v)}
                        disabled={!deckBTrack}
                        height={-1}
                        responsive
                     />
                </div>
            </div>

            {/* Deck Footer: Transport */}
            <div className='h-[60px] bg-[#161616] border-t border-[#222] flex items-center justify-center gap-4 shadow-[-inset_0_10px_20px_rgba(0,0,0,0.5)]'>
                 <DJTransportButtons deck='B' />
            </div>
        </div>

      </div>

      {/* 5. LIBRARY BROWSER (Resizable Bottom Drawer) */}
      {/* Drag Handle */}
      <div 
        className='h-[6px] flex-shrink-0 bg-[#222] border-t border-[#333] cursor-row-resize flex items-center justify-center hover:bg-[#2a2a2a] transition-colors group'
        onMouseDown={(e) => {
          e.preventDefault();
          dragRef.current = { startY: e.clientY, startHeight: libraryHeight };
          const onMouseMove = (me: MouseEvent) => {
            if (!dragRef.current) return;
            const delta = dragRef.current.startY - me.clientY;
            const newHeight = Math.max(100, Math.min(600, dragRef.current.startHeight + delta));
            setLibraryHeight(newHeight);
            if (libraryCollapsed) setLibraryCollapsed(false);
          };
          const onMouseUp = () => {
            dragRef.current = null;
            document.removeEventListener('mousemove', onMouseMove);
            document.removeEventListener('mouseup', onMouseUp);
          };
          document.addEventListener('mousemove', onMouseMove);
          document.addEventListener('mouseup', onMouseUp);
        }}
        onDoubleClick={() => setLibraryCollapsed(!libraryCollapsed)}
      >
        <div className='w-8 h-[2px] bg-[#444] rounded-full group-hover:bg-[#666] transition-colors' />
      </div>
      
      {!libraryCollapsed && (
        <div className='flex-shrink-0 bg-[#000] flex flex-col' style={{ height: `${libraryHeight}px` }}>
            <DJErrorBoundary componentName='DJLibraryBrowserV2'>
               <DJLibraryBrowserV2 />
            </DJErrorBoundary>
        </div>
      )}
    </div>
  );
};

export default DJModeV2;
