/**
 * ViiB MediaHub - DJ Deck Panel (v2)
 *
 * One mirrored implementation for Deck A and Deck B (Plan §4.2):
 * track header + position bar, performance toolbar, deck FX rack,
 * tempo | EQ | jog body (physically mirrored on B), hot cues and transport.
 * Editing and analysis tools open in the anchored deck inspector.
 *
 * The panel itself subscribes to nothing position-driven; every live readout
 * is a self-subscribing leaf.
 *
 * @module components/dj/v2/DJDeckPanel
 */

import React, { useCallback, useEffect, useRef, useState } from 'react';
import { SlidersHorizontal } from 'lucide-react';
import { useStore } from '../../../store';
import { describeKey } from '../../../lib/keyDetection';
import type { DeckId, DeckEQ } from '../../../slices/djMixerSlice';
import { DeckTimeDisplay, DeckHasTrack } from './DJDeckComponents';
import { DJDeckStatusBar } from './DJDeckStatusBar';
import { DJDeckInspector, type DJDeckInspectorTab } from './DJDeckInspector';
import { DJDeckToolbar } from './DJDeckToolbar';
import { DJDeckFXRack } from './DJDeckFXRack';
import { DJNudgeButtons } from './DJNudgeButtons';
import { DJTempoSliderSelfSub, DJDeckEQStrip } from './DJMixerComponents';
import { DJJogWheel } from './DJJogWheel';
import { DJHotCuePad } from './DJHotCuePad';
import { DJTransportButtons } from './DJTransportButtons';
import { DJErrorBoundary } from './DJErrorBoundary';

export interface DJDeckPanelProps {
  deck: DeckId;
  isActive: boolean;
  isDragOver: boolean;
  onTempoChange: (deck: DeckId, value: number) => void;
  onEQChange: (deck: DeckId, band: keyof DeckEQ, value: number) => void;
  onVolumeChange: (deck: DeckId, value: number) => void;
  onFilterChange: (deck: DeckId, knobValue: number) => void;
  onNudge: (deck: DeckId, offsetMs: number) => void;
  onDragOver: (event: React.DragEvent, deck: DeckId) => void;
  onDragLeave: (event: React.DragEvent, deck: DeckId) => void;
  onDrop: (event: React.DragEvent, deck: DeckId) => void;
}

const formatClock = (seconds: number) => `${Math.floor(seconds / 60)}:${Math.floor(seconds % 60).toString().padStart(2, '0')}`;

/** BPM · Camelot · key secondary line. Isolated so tempo drags only re-render it. */
const DeckFacts = React.memo(function DeckFacts({ deck }: { deck: DeckId }) {
  const bpm = useStore(s => (deck === 'A' ? s.djDeckA : s.djDeckB).effectiveBpm);
  const key = useStore(s => (deck === 'A' ? s.djDeckA : s.djDeckB).key);
  const analysisStatus = useStore(s => (deck === 'A' ? s.djDeckA : s.djDeckB).analysisStatus);
  const described = describeKey(key);
  if (!bpm && (analysisStatus === 'not_analyzed' || analysisStatus === 'error')) {
    return (
      <div className='dj-deck-facts dj-deck-facts-warn' title='Analyze this track to enable measured BPM and beat-grid features'>
        {analysisStatus === 'error' ? 'Analysis unavailable' : 'Not analysed'}
      </div>
    );
  }
  return (
    <div className='dj-deck-facts'>
      <span>{bpm ? `${bpm.toFixed(1)} BPM` : analysisStatus === 'loading' ? 'Analysing…' : '--.- BPM'}</span>
      {described?.camelot && <span>{described.camelot}</span>}
      {described && <span>{described.label}</span>}
    </div>
  );
});

/** Thin deck-colored position bar driven by its own timer, never React state. */
const DeckProgress = React.memo(function DeckProgress({ deck }: { deck: DeckId }) {
  const fillRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const update = () => {
      const d = deck === 'A' ? useStore.getState().djDeckA : useStore.getState().djDeckB;
      if (fillRef.current) fillRef.current.style.width = d.duration > 0 ? `${Math.min(100, d.position / d.duration * 100)}%` : '0';
    };
    update();
    const timer = window.setInterval(update, 100);
    return () => window.clearInterval(timer);
  }, [deck]);
  return (
    <div className='dj-deck-progress' aria-hidden='true'>
      <div ref={fillRef} className='dj-deck-progress-fill' />
    </div>
  );
});

const DeckHeader = React.memo(function DeckHeader({ deck }: { deck: DeckId }) {
  const track = useStore(s => (deck === 'A' ? s.djDeckA : s.djDeckB).track);
  const analysisStatus = useStore(s => (deck === 'A' ? s.djDeckA : s.djDeckB).analysisStatus);
  const [inspectorOpen, setInspectorOpen] = useState(false);
  const [inspectorTab, setInspectorTab] = useState<DJDeckInspectorTab>('analysis');
  const triggerRef = useRef<HTMLButtonElement>(null);
  const closeInspector = useCallback(() => setInspectorOpen(false), []);
  const needsAttention = !!track && (analysisStatus === 'not_analyzed' || analysisStatus === 'error');

  return (
    <header className='dj-deck-header'>
      <div className='dj-deck-info'>
        <span className='dj-deck-badge' data-deck={deck} aria-hidden='true'>{deck}</span>
        <div className='dj-deck-art' aria-hidden='true'>
          {track?.coverUrl ? <img src={track.coverUrl} alt='' /> : '♫'}
        </div>
        {track ? (
          <div className='dj-deck-meta'>
            <div className='dj-deck-title' title={track.title || 'Unknown'}>{track.title || 'Unknown'}</div>
            <div className='dj-deck-artist' title={track.artist || 'Unknown Artist'}>{track.artist || 'Unknown Artist'}</div>
            <DeckFacts deck={deck} />
          </div>
        ) : (
          <div className='dj-deck-meta'>
            <div className='dj-deck-empty'>{`Deck ${deck} · No track loaded`}</div>
          </div>
        )}
        <div className='dj-deck-side'>
          <div className='dj-deck-time-row'>
            <DeckHasTrack deck={deck}>
              <DeckTimeDisplay deck={deck} color='var(--dj-text-primary)' sizeClass='dj-time-remaining' showRemaining />
            </DeckHasTrack>
            <button ref={triggerRef} type='button' className='dj-btn dj-btn-icon dj-deck-inspector-trigger' data-deck-accent={deck}
              aria-expanded={inspectorOpen} aria-pressed={inspectorOpen}
              aria-label={`Deck ${deck} analysis and editing${needsAttention ? ' (track not analysed)' : ''}`}
              title='Analysis, key, BPM, cue and grid editing' onClick={() => setInspectorOpen(open => !open)}>
              <SlidersHorizontal size={15} aria-hidden='true' />
              {needsAttention && <span className='dj-status-dot dj-trigger-dot' data-state='warn' aria-hidden='true' />}
            </button>
          </div>
          <DeckHasTrack deck={deck}>
            <DeckTimeDisplay deck={deck} color='var(--dj-text-secondary)' sizeClass='dj-time-elapsed' />
          </DeckHasTrack>
          <div className='dj-deck-actions'>
            <DJDeckStatusBar deck={deck} />
          </div>
        </div>
      </div>
      <DeckProgress deck={deck} />
      <DJDeckInspector deck={deck} open={inspectorOpen} tab={inspectorTab} onTabChange={setInspectorTab}
        onClose={closeInspector} triggerRef={triggerRef} />
    </header>
  );
});

const DeckCueReadout = React.memo(function DeckCueReadout({ deck }: { deck: DeckId }) {
  const cuePoint = useStore(s => (deck === 'A' ? s.djDeckA : s.djDeckB).cuePoint);
  return (
    <div className='dj-deck-readout'>
      <span className='dj-label'>Cue</span>
      <span className='dj-deck-readout-value' data-cue={cuePoint > 0}>{cuePoint > 0 ? formatClock(cuePoint) : '--:--'}</span>
    </div>
  );
});

const DeckLoopReadout = React.memo(function DeckLoopReadout({ deck }: { deck: DeckId }) {
  const loop = useStore(s => (deck === 'A' ? s.djDeckA : s.djDeckB).loop);
  const bpm = useStore(s => {
    const d = deck === 'A' ? s.djDeckA : s.djDeckB;
    return d.effectiveBpm || d.originalBpm || 120;
  });
  const active = loop.enabled && loop.end > loop.start;
  return (
    <div className='dj-deck-readout'>
      <span className='dj-label'>Loop</span>
      <span className='dj-deck-readout-value' data-active={active}>
        {active ? `${((loop.end - loop.start) * bpm / 60).toFixed(2)} beats` : 'OFF'}
      </span>
    </div>
  );
});

export const DJDeckPanel = React.memo(function DJDeckPanel({
  deck, isActive, isDragOver, onTempoChange, onEQChange, onVolumeChange, onFilterChange, onNudge, onDragOver, onDragLeave, onDrop,
}: DJDeckPanelProps) {
  const hasTrack = useStore(s => Boolean((deck === 'A' ? s.djDeckA : s.djDeckB).track));
  const handleTempo = useCallback((value: number) => onTempoChange(deck, value), [deck, onTempoChange]);

  const tempo = (
    <div className='dj-deck-tempo'>
      <DJNudgeButtons deck={deck} onNudge={onNudge} disabled={!hasTrack} />
      <div className='flex-1 min-h-0 flex items-center justify-center'>
        <DJTempoSliderSelfSub deck={deck} onChange={handleTempo} disabled={!hasTrack} height={-1} responsive />
      </div>
    </div>
  );
  const eq = (
    <div className='dj-deck-eq'>
      <DJDeckEQStrip deckId={deck} onEQChange={onEQChange} onVolumeChange={onVolumeChange} onFilterChange={onFilterChange} />
    </div>
  );
  const jog = (
    <div className='dj-deck-jog'>
      <div className='dj-deck-jog-size'>
        <DJErrorBoundary componentName={`DJJogWheel-${deck}`}>
          <DJJogWheel deck={deck} size={-1} responsive />
        </DJErrorBoundary>
      </div>
    </div>
  );

  return (
    <section
      className='dj-deck'
      data-dj-deck={deck}
      data-active={isActive}
      data-drag-over={isDragOver}
      aria-label={`Deck ${deck}`}
      onDragOver={event => onDragOver(event, deck)}
      onDragLeave={event => onDragLeave(event, deck)}
      onDrop={event => onDrop(event, deck)}
    >
      <DeckHeader deck={deck} />
      <DJDeckToolbar deck={deck} />
      <DJDeckFXRack deck={deck} />
      <div className='dj-deck-performance' data-deck={deck}>
        {deck === 'A' ? <>{tempo}{eq}{jog}</> : <>{jog}{eq}{tempo}</>}
      </div>
      <footer className='dj-deck-footer'>
        <div className='dj-deck-cues'>
          <DeckCueReadout deck={deck} />
          <DJHotCuePad deck={deck} console />
          <DeckLoopReadout deck={deck} />
        </div>
        <div className='dj-deck-transport'>
          <DJTransportButtons deck={deck} console />
        </div>
      </footer>
    </section>
  );
});
