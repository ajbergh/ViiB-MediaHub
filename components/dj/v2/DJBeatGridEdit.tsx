/**
 * ViiB MediaHub - DJ Beat Grid Edit Component
 * 
 * Compact beat grid alignment editor.
 * Allows nudging the beat grid offset in ±1ms and ±10ms increments.
 * 
 * @module components/dj/v2/DJBeatGridEdit
 */

import React from 'react';
import { ChevronsLeft, ChevronLeft, ChevronRight, ChevronsRight } from 'lucide-react';
import type { DeckId } from '../../../slices/djMixerSlice';
import { useStore } from '../../../store';

interface DJBeatGridEditProps {
  deck: DeckId;
}

const BTN = 'w-11 h-11 rounded bg-[#222] text-neutral-400 hover:text-white border border-[#333] hover:border-neutral-500 transition-colors flex items-center justify-center flex-shrink-0';

export const DJBeatGridEdit: React.FC<DJBeatGridEditProps> = ({ deck }) => {
  const beatGridOffset = useStore(state =>
    deck === 'A' ? state.djDeckA.beatGridOffset : state.djDeckB.beatGridOffset
  );
  const shiftBeatGrid = useStore(state => state.shiftBeatGrid);
  const resetBeatGridOffset = useStore(state => state.resetBeatGridOffset);

  const offsetMs = Math.round(beatGridOffset * 1000);

  return (
    <div className='flex flex-col items-center gap-1 px-2 flex-shrink-0'>
      <span className='text-[10px] text-neutral-500 font-bold uppercase tracking-wider'>GRID</span>
      <div className='flex items-center gap-1'>
        <button onClick={() => shiftBeatGrid(deck, -0.01)} className={BTN} aria-label='Shift beat grid -10ms' title='Shift beat grid -10ms'>
          <ChevronsLeft size={18} aria-hidden />
        </button>
        <button onClick={() => shiftBeatGrid(deck, -0.001)} className={BTN} aria-label='Shift beat grid -1ms' title='Shift beat grid -1ms'>
          <ChevronLeft size={18} aria-hidden />
        </button>
        <button
          onClick={() => resetBeatGridOffset(deck)}
          className={`w-12 h-11 rounded border transition-colors font-mono text-[11px] flex items-center justify-center flex-shrink-0
            ${offsetMs !== 0
              ? 'bg-amber-600/20 text-amber-300 border-amber-500/40 hover:bg-amber-600/30'
              : 'bg-[#222] text-neutral-500 border-[#333] hover:text-neutral-400'}`}
          title={`Beat grid offset: ${offsetMs}ms. Click to reset`}
          aria-label={`Beat grid offset ${offsetMs}ms — click to reset`}
        >
          {offsetMs >= 0 ? '+' : ''}{offsetMs}
        </button>
        <button onClick={() => shiftBeatGrid(deck, 0.001)} className={BTN} aria-label='Shift beat grid +1ms' title='Shift beat grid +1ms'>
          <ChevronRight size={18} aria-hidden />
        </button>
        <button onClick={() => shiftBeatGrid(deck, 0.01)} className={BTN} aria-label='Shift beat grid +10ms' title='Shift beat grid +10ms'>
          <ChevronsRight size={18} aria-hidden />
        </button>
      </div>
    </div>
  );
};

export default React.memo(DJBeatGridEdit);


