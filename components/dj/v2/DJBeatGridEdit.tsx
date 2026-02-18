/**
 * ViiB MediaHub - DJ Beat Grid Edit Component
 * 
 * Compact beat grid alignment editor.
 * Allows nudging the beat grid offset in ±1ms and ±10ms increments.
 * 
 * @module components/dj/v2/DJBeatGridEdit
 */

import React from 'react';
import type { DeckId } from '../../../slices/djMixerSlice';
import { useStore } from '../../../store';

interface DJBeatGridEditProps {
  deck: DeckId;
}

export const DJBeatGridEdit: React.FC<DJBeatGridEditProps> = ({ deck }) => {
  const beatGridOffset = useStore(state => 
    deck === 'A' ? state.djDeckA.beatGridOffset : state.djDeckB.beatGridOffset
  );
  const shiftBeatGrid = useStore(state => state.shiftBeatGrid);
  const resetBeatGridOffset = useStore(state => state.resetBeatGridOffset);

  const offsetMs = Math.round(beatGridOffset * 1000);

  return (
    <div className='flex items-center gap-0.5'>
      <span className='text-[8px] text-neutral-500 mr-0.5'>GRID</span>
      <button
        onClick={() => shiftBeatGrid(deck, -0.01)}
        className='text-[8px] px-1 py-0.5 rounded bg-[#222] text-neutral-400 hover:text-white border border-[#333] hover:border-neutral-500 transition-colors'
        title='Shift beat grid -10ms'
      >
        ◀◀
      </button>
      <button
        onClick={() => shiftBeatGrid(deck, -0.001)}
        className='text-[8px] px-1 py-0.5 rounded bg-[#222] text-neutral-400 hover:text-white border border-[#333] hover:border-neutral-500 transition-colors'
        title='Shift beat grid -1ms'
      >
        ◀
      </button>
      <button
        onClick={() => resetBeatGridOffset(deck)}
        className={`text-[8px] px-1.5 py-0.5 rounded border transition-colors font-mono min-w-[3rem] text-center
          ${offsetMs !== 0 
            ? 'bg-amber-600/20 text-amber-300 border-amber-500/40 hover:bg-amber-600/30' 
            : 'bg-[#222] text-neutral-500 border-[#333] hover:text-neutral-400'}`}
        title={`Beat grid offset: ${offsetMs}ms. Click to reset`}
      >
        {offsetMs >= 0 ? '+' : ''}{offsetMs}ms
      </button>
      <button
        onClick={() => shiftBeatGrid(deck, 0.001)}
        className='text-[8px] px-1 py-0.5 rounded bg-[#222] text-neutral-400 hover:text-white border border-[#333] hover:border-neutral-500 transition-colors'
        title='Shift beat grid +1ms'
      >
        ▶
      </button>
      <button
        onClick={() => shiftBeatGrid(deck, 0.01)}
        className='text-[8px] px-1 py-0.5 rounded bg-[#222] text-neutral-400 hover:text-white border border-[#333] hover:border-neutral-500 transition-colors'
        title='Shift beat grid +10ms'
      >
        ▶▶
      </button>
    </div>
  );
};

export default React.memo(DJBeatGridEdit);
