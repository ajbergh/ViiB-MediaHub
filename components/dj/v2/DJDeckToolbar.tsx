/**
 * ViiB MediaHub - DJ Deck Toolbar (v2)
 *
 * The one continuously needed controls row beneath the track header
 * (Plan §6.2): beat jump with a jump-size select, beat loops with
 * IN / OUT / RELOOP, and the compact stem switch. Everything fits the
 * authored deck width without horizontal scrolling.
 *
 * @module components/dj/v2/DJDeckToolbar
 */

import React, { useCallback, useState } from 'react';
import { ChevronLeft, ChevronRight } from 'lucide-react';
import { useStore } from '../../../store';
import { useDJAudioEngineActions } from '../../../hooks/useDJAudioEngine';
import { calculateBeatJumpTimestamp } from '../../../lib/beatJump';
import type { DeckId } from '../../../slices/djMixerSlice';
import { DJStemControls } from './DJStemControls';

const JUMP_SIZES = [1, 2, 4, 8, 16, 32] as const;
const LOOP_SIZES = [0.25, 0.5, 1, 2, 4, 8] as const;

const formatBeats = (beats: number) => (beats < 1 ? `1/${Math.round(1 / beats)}` : String(beats));

const DJBeatJumpStrip = React.memo(function DJBeatJumpStrip({ deck }: { deck: DeckId }) {
  const hasTrack = useStore(s => Boolean((deck === 'A' ? s.djDeckA : s.djDeckB).track));
  const [size, setSize] = useState<number>(4);
  const { seek } = useDJAudioEngineActions();

  const jump = useCallback((direction: 1 | -1) => {
    const d = deck === 'A' ? useStore.getState().djDeckA : useStore.getState().djDeckB;
    if (!d.track || !d.duration) return;
    const target = calculateBeatJumpTimestamp({
      position: d.position,
      beats: direction * size,
      duration: d.duration,
      beatGrid: d.beatGrid,
      beatGridOffset: d.beatGridOffset,
      bpm: d.effectiveBpm || d.originalBpm,
    });
    if (target !== null) seek(deck, target);
  }, [deck, seek, size]);

  const unit = size === 1 ? 'beat' : 'beats';
  return (
    <div className='dj-toolbar-cluster' role='group' aria-label={`Deck ${deck} beat jump`}>
      <button type='button' className='dj-btn dj-btn-icon' disabled={!hasTrack} onClick={() => jump(-1)}
        aria-label={`Jump back ${size} ${unit}`} title={`Jump back ${size} ${unit}; uses the stored beat grid when available, otherwise BPM`}>
        <ChevronLeft size={16} aria-hidden='true' />
      </button>
      <button type='button' className='dj-btn dj-btn-icon' disabled={!hasTrack} onClick={() => jump(1)}
        aria-label={`Jump forward ${size} ${unit}`} title={`Jump forward ${size} ${unit}; uses the stored beat grid when available, otherwise BPM`}>
        <ChevronRight size={16} aria-hidden='true' />
      </button>
      <select className='dj-select' value={size} onChange={event => setSize(Number(event.currentTarget.value))}
        aria-label={`Deck ${deck} beat jump size`}>
        {JUMP_SIZES.map(beats => <option key={beats} value={beats}>{beats}</option>)}
      </select>
    </div>
  );
});

const DJLoopStrip = React.memo(function DJLoopStrip({ deck }: { deck: DeckId }) {
  const hasTrack = useStore(s => Boolean((deck === 'A' ? s.djDeckA : s.djDeckB).track));
  const loop = useStore(s => (deck === 'A' ? s.djDeckA : s.djDeckB).loop);
  const bpm = useStore(s => {
    const d = deck === 'A' ? s.djDeckA : s.djDeckB;
    return d.effectiveBpm || d.originalBpm || 120;
  });
  const { setLoopBeats, setLoopIn, setLoopOut, toggleLoop } = useDJAudioEngineActions();
  const validLoop = loop.end > loop.start;
  const loopBeats = validLoop ? (loop.end - loop.start) / (60 / bpm) : 0;

  return (
    <div className='dj-toolbar-cluster' role='group' aria-label={`Deck ${deck} loop`}>
      {LOOP_SIZES.map(beats => (
        <button key={beats} type='button' className='dj-btn' data-deck-accent={deck} disabled={!hasTrack}
          aria-pressed={loop.enabled && validLoop && Math.abs(loopBeats - beats) < 0.05}
          aria-label={`Set ${formatBeats(beats)} beat loop`} title={`Set ${formatBeats(beats)} beat loop`}
          onClick={() => setLoopBeats(deck, beats)}>{formatBeats(beats)}</button>
      ))}
      <span className='dj-toolbar-gap' aria-hidden='true' />
      <button type='button' className='dj-btn' disabled={!hasTrack} onClick={() => setLoopIn(deck)} title='Set loop in point'>IN</button>
      <button type='button' className='dj-btn' disabled={!hasTrack} onClick={() => setLoopOut(deck)} title='Set loop out point'>OUT</button>
      <button type='button' className='dj-btn' data-deck-accent={deck} disabled={!hasTrack || !validLoop}
        aria-pressed={loop.enabled && validLoop} onClick={() => toggleLoop(deck)}
        title={validLoop ? (loop.enabled ? 'Exit loop' : 'Re-enter the last loop') : 'No loop set'}>RELOOP</button>
    </div>
  );
});

export const DJDeckToolbar = React.memo(function DJDeckToolbar({ deck }: { deck: DeckId }) {
  return (
    <div className='dj-deck-toolbar' data-dj-deck-toolbar={deck}>
      <DJBeatJumpStrip deck={deck} />
      <DJLoopStrip deck={deck} />
      <DJStemControls deck={deck} compact />
    </div>
  );
});
