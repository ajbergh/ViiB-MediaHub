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
import { useDJAudioEngine } from '../hooks/useDJAudioEngine';
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
import { createLogger } from '../services/loggerService';
import type { DeckId, DeckEQ } from '../slices/djMixerSlice';

const logger = createLogger('DJModeV2');

type ViewMode = 'timeline' | 'scope' | 'fx';

// Self-subscribing time display to avoid parent re-renders from position updates
const DeckTimeDisplay = React.memo(({ deck, color, showRemaining = false }: { deck: DeckId; color: string; showRemaining?: boolean }) => {
  const containerRef = useRef<HTMLDivElement>(null);
  
  useEffect(() => {
    let animId: number;
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
      }
      animId = requestAnimationFrame(update);
    };
    animId = requestAnimationFrame(update);
    return () => cancelAnimationFrame(animId);
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

// Scope view - real-time VU meter visualization as bars
const DJScopeView = React.memo(({ getVULevels }: { getVULevels: () => { deckA: { left: number; right: number }; deckB: { left: number; right: number }; master: { left: number; right: number } } }) => {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    
    let animId: number;
    const draw = () => {
      const ctx = canvas.getContext('2d');
      if (!ctx) { animId = requestAnimationFrame(draw); return; }
      
      const w = canvas.clientWidth;
      const h = canvas.clientHeight;
      if (canvas.width !== w * 2) { canvas.width = w * 2; canvas.height = h * 2; ctx.scale(2, 2); }
      
      ctx.fillStyle = '#0d0d0d';
      ctx.fillRect(0, 0, w, h);
      
      const levels = getVULevels();
      const bars = [
        { label: 'A-L', value: levels.deckA.left, color: '#3b82f6' },
        { label: 'A-R', value: levels.deckA.right, color: '#3b82f6' },
        { label: 'M-L', value: levels.master.left, color: '#22c55e' },
        { label: 'M-R', value: levels.master.right, color: '#22c55e' },
        { label: 'B-L', value: levels.deckB.left, color: '#8b5cf6' },
        { label: 'B-R', value: levels.deckB.right, color: '#8b5cf6' },
      ];
      
      const barWidth = Math.min(60, (w - 80) / bars.length);
      const gap = 8;
      const totalWidth = bars.length * (barWidth + gap) - gap;
      const startX = (w - totalWidth) / 2;
      const maxH = h - 30;
      
      bars.forEach((bar, i) => {
        const x = startX + i * (barWidth + gap);
        const barH = bar.value * maxH;
        
        // Bar
        const gradient = ctx.createLinearGradient(x, h - 15, x, h - 15 - maxH);
        gradient.addColorStop(0, bar.color);
        gradient.addColorStop(0.6, bar.color);
        gradient.addColorStop(0.85, '#eab308');
        gradient.addColorStop(1, '#ef4444');
        ctx.fillStyle = gradient;
        ctx.fillRect(x, h - 15 - barH, barWidth, barH);
        
        // Background
        ctx.fillStyle = '#1a1a1a';
        ctx.fillRect(x, h - 15 - maxH, barWidth, maxH - barH);
        
        // Label
        ctx.fillStyle = '#666';
        ctx.font = '9px system-ui';
        ctx.textAlign = 'center';
        ctx.fillText(bar.label, x + barWidth / 2, h - 3);
      });
      
      // Title
      ctx.fillStyle = '#444';
      ctx.font = 'bold 10px system-ui';
      ctx.textAlign = 'center';
      ctx.fillText('SCOPE VIEW — AUDIO LEVELS', w / 2, 14);
      
      animId = requestAnimationFrame(draw);
    };
    
    animId = requestAnimationFrame(draw);
    return () => cancelAnimationFrame(animId);
  }, [getVULevels]);
  
  return <canvas ref={canvasRef} className='w-full h-full' />;
});

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
  
  const {
    setDJMixerEnabled,
    djActiveDeck,
    toggleActiveDeck,
    djDeckA,
    djDeckB,
    djMixer,
    setDeckEQ,
    setDeckFilter,
    setDeckFilterEnabled,
    setMasterVolume: storeSetMasterVolume,
    setSyncMode,
    toggleQuantize,
    setCrossfaderCurve,
    toggleKeyLock,
    toggleSlipMode,
    toggleAutoGain,
    toggleWebGLWaveform,
    setHotCue,
    triggerHotCue,
  } = useStore();

  // Initialize DJ audio engine and get control functions
  const { 
    togglePlay, 
    returnToCue, 
    setCrossfader, 
    seek, 
    setVolume,
    setEQ,
    setTempo,
    setKeyLock,
    syncBeatPhase,
    setMasterVolume: engineSetMasterVolume,
    setFilterFX,
    getVULevels,
    getMasterStream,
  } = useDJAudioEngine();

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

  // Key lock handler - toggle store + sync engine
  const handleKeyLockToggle = useCallback((deck: DeckId) => {
    const current = deck === 'A' ? djMixer.keyLockA : djMixer.keyLockB;
    toggleKeyLock(deck);
    setKeyLock(deck, !current);
  }, [djMixer.keyLockA, djMixer.keyLockB, toggleKeyLock, setKeyLock]);

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
    
    setDeckFilter(deck, clamped);
    
    // Enable/disable filter in audio engine based on value
    const isNeutral = Math.abs(clamped) < 0.05;
    if (isNeutral) {
      setFilterFX(deck, false, 'lowpass', 20000, 0.5);
      setDeckFilterEnabled(deck, false);
    } else {
      setDeckFilterEnabled(deck, true);
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
  }, [setDeckFilter, setDeckFilterEnabled, setFilterFX]);

  // Master volume handler - maps -24..+12 knob range to 0..1 volume
  const handleMasterVolumeChange = useCallback((knobValue: number) => {
    const normalized = (knobValue + 24) / 36; // 0 to 1
    const clamped = Math.max(0, Math.min(1, normalized));
    storeSetMasterVolume(clamped);
    engineSetMasterVolume(clamped);
  }, [storeSetMasterVolume, engineSetMasterVolume]);

  // Format seconds to MM:SS or -MM:SS
  const formatTime = useCallback((seconds: number, negative = false): string => {
    const abs = Math.abs(seconds);
    const m = Math.floor(abs / 60);
    const s = Math.floor(abs % 60);
    const prefix = negative ? '-' : '';
    return `${prefix}${m}:${s.toString().padStart(2, '0')}`;
  }, []);

  // Helper for rendering channel strip
  const renderChannelStrip = (deckId: DeckId, deckState: any) => {
    const isA = deckId === 'A';
    const accentColor = isA ? '#3b82f6' : '#8b5cf6';
    
    // Convert filter value (-1..+1) to knob range (-24..+12)
    const filterKnobValue = ((deckState.filter.value + 1) / 2) * 36 - 24;
    const filterActive = deckState.filter.enabled;

    return (
      <div className={`flex-1 flex flex-col items-center py-2 gap-3 min-w-[90px] border-[#2a2a2a]`}>
        {/* Gain / Trim - currently maps to volume as pre-gain */}
        <div className='relative pt-2'>
            <DJEQKnob 
                label='TRIM' 
                value={deckState.eq.high !== undefined ? 0 : 0}
                onChange={(v) => {
                  // Trim acts as a master volume modifier
                  // Map -24..+12 to 0..1.5 volume range
                  const normalized = (v + 24) / 36;
                  const trimmedVol = Math.max(0, Math.min(1, normalized * 1.5));
                  handleVolumeChange(deckId, trimmedVol);
                }}
                color='#aaaaaa' 
                size={38} 
            />
        </div>
        
        {/* EQ Section - Grouped with background */}
        <div className='flex flex-col gap-2 p-1.5 bg-[#151515] rounded-md border border-[#222] shadow-inner'>
          <DJEQKnob 
            label='HIGH' 
            value={deckState.eq.high} 
            onChange={(v) => handleEQChange(deckId, 'high', v)}
            color='#06b6d4' // Cyan
            size={36}
          />
          <DJEQKnob 
            label='MID' 
            value={deckState.eq.mid} 
            onChange={(v) => handleEQChange(deckId, 'mid', v)}
            color='#22c55e' // Green
            size={36}
          />
          <DJEQKnob 
            label='LOW' 
            value={deckState.eq.low} 
            onChange={(v) => handleEQChange(deckId, 'low', v)}
            color='#f59e0b' // Orange
            size={36}
          />
        </div>

        {/* Filter Knob */}
        <div className='p-1.5 bg-[#151515] rounded-md border border-[#222] shadow-inner'>
          <DJEQKnob 
            label='FILTER' 
            value={filterKnobValue}
            onChange={(v) => handleFilterChange(deckId, v)}
            color={filterActive ? '#ef4444' : '#666'}
            size={36}
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
               getLevels={isA ? getDeckALevels : getDeckBLevels}
               height={140}
               channelWidth={3}
               gap={1}
               segments={18}
               showPeak={true}
             />
             <DJVolumeFader 
                value={deckState.volume}
                onChange={(v) => handleVolumeChange(deckId, v)}
                label=''
                height={160}
                isPlaying={deckState.isPlaying}
                accentColor={accentColor}
             />
        </div>
      </div>
    );
  };

  return (
    <div className='h-full flex flex-col bg-[#121212] overflow-hidden min-w-[1024px]'>
      
      {/* Keyboard Shortcuts Overlay */}
      {showShortcuts && (
        <div className='fixed inset-0 z-[100] bg-black/80 backdrop-blur-sm flex items-center justify-center' onClick={() => setShowShortcuts(false)}>
          <div className='bg-[#1a1a1a] border border-[#333] rounded-lg p-6 max-w-lg w-full max-h-[80vh] overflow-y-auto' onClick={e => e.stopPropagation()}>
            <div className='flex items-center justify-between mb-4'>
              <h2 className='text-lg font-bold text-white'>Keyboard Shortcuts</h2>
              <button onClick={() => setShowShortcuts(false)} className='text-neutral-500 hover:text-white text-xl'>✕</button>
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
        <div className='flex-shrink-0 bg-[#0d0d0d] border-b border-[#2a2a2a] relative' style={{ height: '180px' }}>
          {/* WebGL / Canvas 2D toggle */}
          <button
            onClick={toggleWebGLWaveform}
            className={`absolute top-1 right-1 z-10 px-1.5 py-0.5 rounded text-[9px] font-bold uppercase transition-colors ${
              djMixer.useWebGLWaveform
                ? 'bg-green-600/30 text-green-400 border border-green-500/40'
                : 'bg-neutral-800/80 text-neutral-500 border border-neutral-700 hover:text-neutral-300'
            }`}
            title={djMixer.useWebGLWaveform ? 'WebGL waveform active — click for Canvas 2D' : 'Canvas 2D waveform — click for WebGL'}
          >
            {djMixer.useWebGLWaveform ? 'WebGL' : '2D'}
          </button>
          <DJErrorBoundary componentName='DJWaveform'>
            {djMixer.useWebGLWaveform ? (
              <DJWebGLWaveform height={180} allowFallback />
            ) : (
              <DJDualWaveform height={-1} responsive />
            )}
          </DJErrorBoundary>
        </div>
      )}
      {viewMode === 'scope' && (
        <div className='flex-shrink-0 bg-[#0d0d0d] border-b border-[#2a2a2a] flex items-center justify-center' style={{ height: '180px' }}>
          <DJScopeView getVULevels={getVULevels} />
        </div>
      )}
      {viewMode === 'fx' && (
        <div className='flex-shrink-0 bg-[#0d0d0d] border-b border-[#2a2a2a]' style={{ height: '180px' }}>
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
      <div className='flex-1 flex min-h-0 bg-[#121212]'>
        
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
                        {djDeckA.track ? (
                          <div className='min-w-0 flex-1'>
                            <div className='text-[11px] font-bold text-white truncate leading-tight'>
                              {djDeckA.track.title || 'Unknown'}
                            </div>
                            <div className='text-[9px] text-neutral-400 truncate leading-tight'>
                              {djDeckA.track.artist || 'Unknown Artist'}
                            </div>
                          </div>
                        ) : (
                          <span className='text-[9px] text-neutral-600 italic'>No track loaded</span>
                        )}
                      </div>
                      <div className='flex items-center gap-2 flex-shrink-0'>
                        {djDeckA.key && (
                          <span className='text-[9px] font-bold text-emerald-400 bg-emerald-500/15 px-1.5 py-0.5 rounded'>
                            {djDeckA.key}
                          </span>
                        )}
                        <button
                          onClick={() => handleKeyLockToggle('A')}
                          className={`text-[8px] font-bold px-1 py-0.5 rounded border transition-all duration-100
                            ${djMixer.keyLockA
                              ? 'bg-emerald-600/40 text-emerald-300 border-emerald-500/50'
                              : 'bg-[#222] text-neutral-600 border-[#333] hover:text-neutral-400'}`}
                          title={`Key Lock ${djMixer.keyLockA ? 'ON' : 'OFF'} - ${djMixer.keyLockA ? 'Pitch preserved when tempo changes' : 'Pitch follows tempo'}`}
                        >
                          🔒
                        </button>
                        <button
                          onClick={() => toggleSlipMode('A')}
                          className={`text-[8px] font-bold px-1 py-0.5 rounded border transition-all duration-100
                            ${djMixer.slipModeA
                              ? 'bg-orange-600/40 text-orange-300 border-orange-500/50'
                              : 'bg-[#222] text-neutral-600 border-[#333] hover:text-neutral-400'}`}
                          title={`Slip Mode ${djMixer.slipModeA ? 'ON' : 'OFF'} - ${djMixer.slipModeA ? 'Playback continues in background during scratch' : 'Normal scratch behavior'}`}
                        >
                          SLIP
                        </button>
                        <button
                          onClick={() => toggleAutoGain('A')}
                          className={`text-[8px] font-bold px-1 py-0.5 rounded border transition-all duration-100
                            ${djMixer.autoGainA
                              ? 'bg-cyan-600/40 text-cyan-300 border-cyan-500/50'
                              : 'bg-[#222] text-neutral-600 border-[#333] hover:text-neutral-400'}`}
                          title={`Auto-Gain ${djMixer.autoGainA ? 'ON' : 'OFF'} - Normalizes track loudness`}
                        >
                          AG
                        </button>
                        {djDeckA.effectiveBpm && (
                          <span className='text-[9px] font-mono text-neutral-400'>
                            {djDeckA.effectiveBpm.toFixed(1)}
                          </span>
                        )}
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
                     <DJTempoSlider 
                        deck='A'
                        value={djDeckA.tempo}
                        onChange={(v) => handleTempoChange('A', v)}
                        originalBpm={djDeckA.originalBpm}
                        effectiveBpm={djDeckA.effectiveBpm}
                        disabled={!djDeckA.track}
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
            <div className='h-[70px] bg-[#161616] border-t border-[#222] flex items-center justify-center gap-6 shadow-[-inset_0_10px_20px_rgba(0,0,0,0.5)]'>
                 <DJTransportButtons deck='A' />
                 
                 <button
                    onClick={() => handleSync('A')}
                    disabled={!djDeckA.track || djMixer.syncMode === 'off'}
                    className={`
                      h-8 px-4 rounded text-[10px] font-bold uppercase tracking-wider
                      transition-all duration-100 border
                      ${!djDeckA.track || djMixer.syncMode === 'off'
                        ? 'bg-[#2a2a2a] text-neutral-600 border-[#333] cursor-not-allowed'
                        : djMixer.syncMode === 'beat-phase'
                          ? 'bg-amber-600 hover:bg-amber-500 text-white border-amber-500'
                          : 'bg-[#333] hover:bg-[#444] text-white border-[#555]'}
                    `}
                  >
                    {djMixer.syncMode === 'beat-phase' ? 'SYNC+' : 'SYNC'}
                  </button>
            </div>
        </div>

        {/* === MIXER CENTER (Fixed Width) === */}
        <div className='w-[300px] flex-shrink-0 bg-[#181818] flex flex-col border-x border-[#333] shadow-2xl z-20 relative'>
             {/* Mixer Body */}
             <div className='flex-1 flex w-full relative'>
                 {/* Channel A */}
                 {renderChannelStrip('A', djDeckA)}

                 {/* Center Master Strip */}
                 <div className='w-[80px] bg-[#131313] flex flex-col items-center py-3 border-x border-[#222]/50'>
                      <div className='text-[9px] text-[#444] font-bold mb-2 tracking-widest'>MASTER</div>
                      
                      {/* Real-time Master VU Meters */}
                      <div className='flex-1 flex items-center justify-center mb-4'>
                           <DJStereoVUMeter
                             getLevels={getMasterLevels}
                             height={100}
                             channelWidth={5}
                             gap={2}
                             segments={20}
                             showPeak={true}
                           />
                      </div>

                      {/* Headphone Mix Knob */}
                      <div className='mb-4 flex flex-col items-center'>
                          <span className='text-[8px] text-[#555] mb-1 font-bold'>CUE MIX</span>
                          <div className='scale-75 origin-center'>
                            <DJHeadphoneMix width={60} />
                          </div>
                      </div>
                      
                      {/* Master Volume */}
                      <div className='mb-2'>
                           <DJEQKnob 
                             label='MAIN' 
                             value={((djMixer.masterVolume) * 36 - 24)}
                             onChange={handleMasterVolumeChange}
                             size={40} 
                             color='#fff' 
                           />
                      </div>
                 </div>

                 {/* Channel B */}
                 {renderChannelStrip('B', djDeckB)}
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
                         ${djMixer.syncMode === mode
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
                       ${djMixer.quantize
                         ? 'bg-cyan-600 text-white border-cyan-500'
                         : 'bg-[#222] text-neutral-500 border-[#333] hover:bg-[#2a2a2a] hover:text-neutral-300'}
                     `}
                     title={`Quantize ${djMixer.quantize ? 'ON' : 'OFF'} - Snap actions to beat grid`}
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
                         ${djMixer.crossfaderCurve === curve
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
                     
                     <DJCrossfader 
                        value={djMixer.crossfader}
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
                        {djDeckB.effectiveBpm && (
                          <span className='text-[9px] font-mono text-neutral-400'>
                            {djDeckB.effectiveBpm.toFixed(1)}
                          </span>
                        )}
                        {djDeckB.key && (
                          <span className='text-[9px] font-bold text-emerald-400 bg-emerald-500/15 px-1.5 py-0.5 rounded'>
                            {djDeckB.key}
                          </span>
                        )}
                        <button
                          onClick={() => handleKeyLockToggle('B')}
                          className={`text-[8px] font-bold px-1 py-0.5 rounded border transition-all duration-100
                            ${djMixer.keyLockB
                              ? 'bg-emerald-600/40 text-emerald-300 border-emerald-500/50'
                              : 'bg-[#222] text-neutral-600 border-[#333] hover:text-neutral-400'}`}
                          title={`Key Lock ${djMixer.keyLockB ? 'ON' : 'OFF'} - ${djMixer.keyLockB ? 'Pitch preserved when tempo changes' : 'Pitch follows tempo'}`}
                        >
                          🔒
                        </button>
                        <button
                          onClick={() => toggleSlipMode('B')}
                          className={`text-[8px] font-bold px-1 py-0.5 rounded border transition-all duration-100
                            ${djMixer.slipModeB
                              ? 'bg-orange-600/40 text-orange-300 border-orange-500/50'
                              : 'bg-[#222] text-neutral-600 border-[#333] hover:text-neutral-400'}`}
                          title={`Slip Mode ${djMixer.slipModeB ? 'ON' : 'OFF'} - ${djMixer.slipModeB ? 'Playback continues in background during scratch' : 'Normal scratch behavior'}`}
                        >
                          SLIP
                        </button>
                        <button
                          onClick={() => toggleAutoGain('B')}
                          className={`text-[8px] font-bold px-1 py-0.5 rounded border transition-all duration-100
                            ${djMixer.autoGainB
                              ? 'bg-cyan-600/40 text-cyan-300 border-cyan-500/50'
                              : 'bg-[#222] text-neutral-600 border-[#333] hover:text-neutral-400'}`}
                          title={`Auto-Gain ${djMixer.autoGainB ? 'ON' : 'OFF'} - Normalizes track loudness`}
                        >
                          AG
                        </button>
                      </div>
                      <div className='flex items-center gap-2 min-w-0 flex-1 justify-end'>
                        {djDeckB.track ? (
                          <div className='min-w-0 text-right'>
                            <div className='text-[11px] font-bold text-white truncate leading-tight'>
                              {djDeckB.track.title || 'Unknown'}
                            </div>
                            <div className='text-[9px] text-neutral-400 truncate leading-tight'>
                              {djDeckB.track.artist || 'Unknown Artist'}
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
                     <DJTempoSlider 
                        deck='B'
                        value={djDeckB.tempo}
                        onChange={(v) => handleTempoChange('B', v)}
                        originalBpm={djDeckB.originalBpm}
                        effectiveBpm={djDeckB.effectiveBpm}
                        disabled={!djDeckB.track}
                        height={-1}
                        responsive
                     />
                </div>
            </div>

            {/* Deck Footer: Transport */}
            <div className='h-[70px] bg-[#161616] border-t border-[#222] flex items-center justify-center gap-6 shadow-[-inset_0_10px_20px_rgba(0,0,0,0.5)]'>
                 <button
                    onClick={() => handleSync('B')}
                    disabled={!djDeckB.track || djMixer.syncMode === 'off'}
                    className={`
                      h-8 px-4 rounded text-[10px] font-bold uppercase tracking-wider
                      transition-all duration-100 border
                      ${!djDeckB.track || djMixer.syncMode === 'off'
                        ? 'bg-[#2a2a2a] text-neutral-600 border-[#333] cursor-not-allowed'
                        : djMixer.syncMode === 'beat-phase'
                          ? 'bg-amber-600 hover:bg-amber-500 text-white border-amber-500'
                          : 'bg-[#333] hover:bg-[#444] text-white border-[#555]'}
                    `}
                  >
                    {djMixer.syncMode === 'beat-phase' ? 'SYNC+' : 'SYNC'}
                  </button>
                 
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
