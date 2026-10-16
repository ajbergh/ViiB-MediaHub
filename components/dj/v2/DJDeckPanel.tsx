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

import React, { useCallback, useRef, useState } from 'react';
import { SlidersHorizontal } from 'lucide-react';
import { useStore } from '../../../store';
import type { DeckId, DeckEQ } from '../../../slices/djMixerSlice';
import { DeckTimeDisplay, DeckHasTrack } from './DJDeckComponents';
import { DeckFacts } from './DJDeckFacts';
import { DJDeckStatusBar } from './DJDeckStatusBar';
import { DJDeckInspector, type DJDeckInspectorTab } from './DJDeckInspector';
import { DJDeckOverview } from './DJDeckOverview';
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

const DeckVinylFallback = ({ deck }: { deck: DeckId }) => (
  <svg width='36' height='36' viewBox='0 0 36 36' fill='none' aria-hidden='true' className='dj-deck-art-disc'>
    <circle cx='18' cy='18' r='16' fill='#0b0e15' stroke='rgba(255,255,255,0.12)' strokeWidth='1' />
    <circle cx='18' cy='18' r='12.5' stroke='rgba(255,255,255,0.06)' strokeWidth='0.75' />
    <circle cx='18' cy='18' r='9.5' stroke='rgba(255,255,255,0.06)' strokeWidth='0.75' />
    <circle
      cx='18'
      cy='18'
      r='5.5'
      fill={deck === 'A' ? 'rgba(0, 179, 255, 0.16)' : 'rgba(255, 42, 109, 0.16)'}
      stroke={deck === 'A' ? 'var(--dj-deck-a-bright)' : 'var(--dj-deck-b-bright)'}
      strokeWidth='1.2'
    />
    <circle cx='18' cy='18' r='1.8' fill='#06080c' />
  </svg>
);

const DeckHeader = React.memo(function DeckHeader({ deck }: { deck: DeckId }) {
  const track = useStore(s => (deck === 'A' ? s.djDeckA : s.djDeckB).track);
  const analysisStatus = useStore(s => (deck === 'A' ? s.djDeckA : s.djDeckB).analysisStatus);
  const [inspectorOpen, setInspectorOpen] = useState(false);
  const [inspectorTab, setInspectorTab] = useState<DJDeckInspectorTab>('analysis');
  const triggerRef = useRef<HTMLButtonElement>(null);
  const closeInspector = useCallback(() => setInspectorOpen(false), []);
  const needsAttention = !!track && (analysisStatus === 'not_analyzed' || analysisStatus === 'unsupported' || analysisStatus === 'error');

  return (
    <header className='dj-deck-header'>
      <div className='dj-deck-info'>
        <span className='dj-deck-badge' data-deck={deck} aria-hidden='true'>{deck}</span>
        <div className='dj-deck-art' aria-hidden='true'>
          {track?.coverUrl ? <img src={track.coverUrl} alt='' /> : <DeckVinylFallback deck={deck} />}
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
            <div className='dj-deck-empty-sub'>Drag a track here or load from library below</div>
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
              title='Analysis, audio waveforms, key, BPM, cue and grid editing' onClick={() => setInspectorOpen(open => !open)}>
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
      <div className='dj-deck-header-overview'>
        <DJDeckOverview deck={deck} />
      </div>
      <DJDeckInspector deck={deck} open={inspectorOpen} tab={inspectorTab} onTabChange={setInspectorTab}
        onClose={closeInspector} triggerRef={triggerRef} />
    </header>
  );
});

export const DJDeckPanel = React.memo(function DJDeckPanel({
  deck, isActive, isDragOver, onTempoChange, onEQChange, onVolumeChange, onFilterChange, onNudge, onDragOver, onDragLeave, onDrop,
}: DJDeckPanelProps) {
  const hasTrack = useStore(s => Boolean((deck === 'A' ? s.djDeckA : s.djDeckB).track));
  const layoutMode = useStore(s => s.djMixer?.djLayoutMode || 'perf');
  const [fxDrawerOpen, setFxDrawerOpen] = useState(false);
  const fxOpen = layoutMode === 'fx' || fxDrawerOpen;
  const handleToggleFX = useCallback(() => setFxDrawerOpen(open => !open), []);
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
      <DJDeckToolbar deck={deck} fxOpen={fxOpen} onToggleFX={handleToggleFX} />
      <DJDeckFXRack deck={deck} open={fxOpen} />
      <div className='dj-deck-performance' data-deck={deck}>
        {deck === 'A' ? <>{tempo}{jog}{eq}</> : <>{eq}{jog}{tempo}</>}
      </div>
      <footer className='dj-deck-footer'>
        <div className='dj-deck-cues'>
          <DJHotCuePad deck={deck} console />
        </div>
        <div className='dj-deck-transport'>
          <DJTransportButtons deck={deck} console />
        </div>
      </footer>
    </section>
  );
});
