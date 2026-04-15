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
    <div className='flex flex-col items-center gap-1 w-full px-2'>
      <span className='text-[9px] text-neutral-500 font-bold uppercase tracking-wider'>GRID</span>
      <div className='flex items-center gap-1 w-full'>
        <button
          onClick={() => shiftBeatGrid(deck, -0.01)}
          className='flex-1 text-[9px] h-7 rounded bg-[#222] text-neutral-400 hover:text-white border border-[#333] hover:border-neutral-500 transition-colors flex items-center justify-center'
          title='Shift beat grid -10ms'
        >
          ◀◀
        </button>
        <button
          onClick={() => shiftBeatGrid(deck, -0.001)}
          className='flex-1 text-[9px] h-7 rounded bg-[#222] text-neutral-400 hover:text-white border border-[#333] hover:border-neutral-500 transition-colors flex items-center justify-center'
          title='Shift beat grid -1ms'
        >
          ◀
        </button>
        <button
          onClick={() => resetBeatGridOffset(deck)}
          className={`w-10 flex-shrink-0 text-[9px] h-7 rounded border transition-colors font-mono flex items-center justify-center
            ${offsetMs !== 0 
              ? 'bg-amber-600/20 text-amber-300 border-amber-500/40 hover:bg-amber-600/30' 
              : 'bg-[#222] text-neutral-500 border-[#333] hover:text-neutral-400'}`}
          title={`Beat grid offset: ${offsetMs}ms. Click to reset`}
        >
          {offsetMs >= 0 ? '+' : ''}{offsetMs}
        </button>
        <button
          onClick={() => shiftBeatGrid(deck, 0.001)}
          className='flex-1 text-[9px] h-7 rounded bg-[#222] text-neutral-400 hover:text-white border border-[#333] hover:border-neutral-500 transition-colors flex items-center justify-center'
          title='Shift beat grid +1ms'
        >
          ▶
        </button>
        <button
          onClick={() => shiftBeatGrid(deck, 0.01)}
          className='flex-1 text-[9px] h-7 rounded bg-[#222] text-neutral-400 hover:text-white border border-[#333] hover:border-neutral-500 transition-colors flex items-center justify-center'
          title='Shift beat grid +10ms'
        >
          ▶▶
        </button>
      </div>
    </div>
  );
};

export default React.memo(DJBeatGridEdit);
