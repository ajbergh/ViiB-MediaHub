/**
 * ViiB MediaHub - DJ Top Bar (v2)
 *
 * Compact global navigation strip (Plan §10A.4). One mode group replaces the
 * former SCOPE/TIMELINE/RACKS + PERF/BROWSE/FX pair:
 *
 *   DJ      performance workspace (split waveforms, full decks)
 *   LIBRARY toggles the library drawer
 *   BROWSE  performance geometry with the library open
 *   FX      brings the mixer's FX pad (X-Y pad + Beat FX) forward
 *   SCOPE   swaps the waveform band for the diagnostic scope
 *
 * Utilities on the right: REC, MIDI, AUDIO, settings, fullscreen.
 *
 * @module components/dj/v2/DJTopBar
 */

import React, { useEffect, useRef, useState } from 'react';
import { Maximize2, Minimize2, Settings, Volume2, Cable } from 'lucide-react';
import { useStore } from '../../../store';
import { shouldUseAdvancedWebGL } from '../../../lib/webglSafety';
import type { DJLayoutMode } from '../../../slices/djMixerSlice';

export type DJUpperView = 'waveform' | 'scope';

interface DJTopBarProps {
  layoutMode: DJLayoutMode;
  onLayoutModeChange: (mode: DJLayoutMode) => void;
  upperView: DJUpperView;
  onUpperViewChange: (view: DJUpperView) => void;
  libraryOpen: boolean;
  onToggleLibrary: () => void;
  isRecording: boolean;
  recordingDuration: number;
  onRecordToggle: () => void;
  midiEnabled: boolean;
  onOpenMidi: () => void;
  onOpenAudio: () => void;
  onShowShortcuts: () => void;
}

/** Persistent fullscreen toggle shown in the top-bar utility group. */
const FullscreenButton: React.FC = () => {
  const [isFS, setIsFS] = useState(() => typeof document !== 'undefined' ? !!document.fullscreenElement : false);
  useEffect(() => {
    const onFSChange = () => setIsFS(!!document.fullscreenElement);
    document.addEventListener('fullscreenchange', onFSChange);
    return () => document.removeEventListener('fullscreenchange', onFSChange);
  }, []);
  const toggle = async () => {
    try {
      if (document.fullscreenElement) await document.exitFullscreen();
      else await document.documentElement.requestFullscreen();
    } catch {
      // Fullscreen not available (e.g. Wails WebView) — ignore
    }
  };
  return (
    <button type='button' onClick={toggle} className='dj-btn dj-btn-icon' aria-pressed={isFS}
      aria-label={isFS ? 'Exit fullscreen' : 'Enter fullscreen (F11)'} title={isFS ? 'Exit fullscreen' : 'Enter fullscreen (F11)'}>
      {isFS ? <Minimize2 size={15} aria-hidden='true' /> : <Maximize2 size={15} aria-hidden='true' />}
    </button>
  );
};

const formatDuration = (seconds: number) => `${Math.floor(seconds / 60)}:${(seconds % 60).toString().padStart(2, '0')}`;

/** DJ settings: waveform renderer and the shortcuts overlay. */
const SettingsMenu: React.FC<{ onShowShortcuts: () => void }> = ({ onShowShortcuts }) => {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const useWebGLWaveform = useStore(s => s.djMixer?.useWebGLWaveform);
  const toggleWebGLWaveform = useStore(s => s.toggleWebGLWaveform);
  const advancedWebGLEnabled = shouldUseAdvancedWebGL();

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (event: PointerEvent) => { if (!rootRef.current?.contains(event.target as Node)) setOpen(false); };
    document.addEventListener('pointerdown', onPointerDown, true);
    return () => document.removeEventListener('pointerdown', onPointerDown, true);
  }, [open]);

  return (
    <div ref={rootRef} className='relative' onKeyDown={event => { if (event.key === 'Escape' && open) { event.stopPropagation(); setOpen(false); } }}>
      <button type='button' className='dj-btn dj-btn-icon' aria-expanded={open} aria-haspopup='true' aria-label='DJ settings'
        title='DJ settings' onClick={() => setOpen(value => !value)}>
        <Settings size={15} aria-hidden='true' />
      </button>
      {open && (
        <div className='dj-popover' style={{ top: 'calc(100% + 6px)', right: 0 }} role='group' aria-label='DJ settings'>
          <p className='dj-label' style={{ marginBottom: 6 }}>Waveform renderer</p>
          <div className='dj-segmented' style={{ marginBottom: 10 }}>
            <button type='button' className='dj-btn dj-btn-xs' aria-pressed={advancedWebGLEnabled && !!useWebGLWaveform}
              disabled={!advancedWebGLEnabled} onClick={() => { if (!useWebGLWaveform) toggleWebGLWaveform(); }}
              title={advancedWebGLEnabled ? 'GPU-accelerated waveform' : 'Canvas waveform is used in the macOS desktop app for stability'}>WebGL</button>
            <button type='button' className='dj-btn dj-btn-xs' aria-pressed={!advancedWebGLEnabled || !useWebGLWaveform}
              onClick={() => { if (useWebGLWaveform) toggleWebGLWaveform(); }} title='Canvas 2D waveform'>Canvas 2D</button>
          </div>
          <button type='button' className='dj-btn dj-btn-xs dj-menu-item' onClick={() => { setOpen(false); onShowShortcuts(); }}>
            Keyboard shortcuts <kbd>?</kbd>
          </button>
        </div>
      )}
    </div>
  );
};

export const DJTopBar: React.FC<DJTopBarProps> = ({
  layoutMode, onLayoutModeChange, upperView, onUpperViewChange, libraryOpen, onToggleLibrary,
  isRecording, recordingDuration, onRecordToggle, midiEnabled, onOpenMidi, onOpenAudio, onShowShortcuts,
}) => {
  const modes: ReadonlyArray<{ mode: DJLayoutMode; label: string; title: string }> = [
    { mode: 'perf', label: 'DJ', title: 'Performance workspace — split waveforms and full decks' },
    { mode: 'browse', label: 'BROWSE', title: 'Browse — performance geometry with the library open' },
    { mode: 'fx', label: 'FX', title: 'FX — X-Y pad and Beat FX in the mixer' },
  ];
  const [dj, browse, fx] = modes;
  const modeButton = ({ mode, label, title }: typeof modes[number]) => (
    <button key={mode} type='button' className='dj-btn dj-topbar-mode' aria-pressed={layoutMode === mode} title={title}
      onClick={() => { onLayoutModeChange(mode); if (mode === 'perf') onUpperViewChange('waveform'); }}>{label}</button>
  );

  return (
    <header className='dj-topbar'>
      <nav className='dj-topbar-group' aria-label='DJ workspace modes'>
        {modeButton(dj)}
        <button type='button' className='dj-btn dj-topbar-mode' aria-expanded={libraryOpen} aria-controls='dj-library-drawer'
          title='Toggle the library (/)' onClick={onToggleLibrary}>LIBRARY</button>
        {modeButton(browse)}
        {modeButton(fx)}
        <button type='button' className='dj-btn dj-topbar-mode' aria-pressed={upperView === 'scope'}
          title='Swap the waveform band for the signal scope' onClick={() => onUpperViewChange(upperView === 'scope' ? 'waveform' : 'scope')}>SCOPE</button>
      </nav>
      <div className='dj-topbar-group'>
        <button type='button' className='dj-btn dj-btn-danger' onClick={onRecordToggle} aria-pressed={isRecording}
          aria-label={isRecording ? 'Stop recording' : 'Start recording'} title={isRecording ? 'Stop and save the recording' : 'Record the master output'}>
          <span className='dj-status-dot' data-state={isRecording ? 'live' : undefined} aria-hidden='true' />
          REC
          {isRecording && <span className='dj-rec-time'>{formatDuration(recordingDuration)}</span>}
        </button>
        <button type='button' className='dj-btn' onClick={onOpenMidi}
          title={midiEnabled ? 'MIDI enabled — open controller mappings' : 'MIDI is off — open this panel and enable MIDI to receive controller input'}
          aria-label={midiEnabled ? 'MIDI enabled, open controller mappings' : 'MIDI off, open controller mappings to enable'}>
          <Cable size={14} aria-hidden='true' /> MIDI <span className='dj-status-dot' data-state={midiEnabled ? 'ok' : 'warn'} aria-hidden='true' />
        </button>
        <button type='button' className='dj-btn' onClick={onOpenAudio} title='Audio output setup'>
          <Volume2 size={14} aria-hidden='true' /> AUDIO
        </button>
        <SettingsMenu onShowShortcuts={onShowShortcuts} />
        <FullscreenButton />
      </div>
    </header>
  );
};
