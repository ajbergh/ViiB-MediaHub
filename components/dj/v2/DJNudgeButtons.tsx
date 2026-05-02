import React, { memo, useCallback } from 'react';
import { Minus, Plus } from 'lucide-react';
import type { DeckId } from '../../../slices/djMixerSlice';

interface DJNudgeButtonsProps {
  deck: DeckId;
  onNudge: (deck: DeckId, offsetMs: number) => void;
  disabled?: boolean;
}

export const DJNudgeButtons: React.FC<DJNudgeButtonsProps> = memo(({
  deck,
  onNudge,
  disabled = false,
}) => {
  const isA = deck === 'A';
  const activeClass = isA
    ? 'hover:bg-blue-500/20 hover:text-blue-200 hover:border-blue-500/50 active:bg-blue-500/30'
    : 'hover:bg-purple-500/20 hover:text-purple-200 hover:border-purple-500/50 active:bg-purple-500/30';

  const handleNudge = useCallback((offsetMs: number, fine: boolean) => {
    onNudge(deck, fine ? Math.sign(offsetMs) * 5 : offsetMs);
  }, [deck, onNudge]);

  const buttonClass = `
    w-7 h-7 rounded border flex items-center justify-center transition-all duration-75
    ${disabled
      ? 'bg-[#1a1a1a] text-neutral-700 border-[#222] cursor-not-allowed'
      : `bg-[#222] text-neutral-400 border-[#333] ${activeClass}`}
  `;

  return (
    <div className='flex items-center gap-1' aria-label={`Deck ${deck} nudge controls`}>
      <button
        type='button'
        disabled={disabled}
        onClick={(e) => handleNudge(-20, e.shiftKey)}
        className={buttonClass}
        title={`Nudge Deck ${deck} backward 20ms (Shift-click: 5ms)`}
        aria-label={`Nudge Deck ${deck} backward`}
      >
        <Minus size={13} aria-hidden />
      </button>
      <button
        type='button'
        disabled={disabled}
        onClick={(e) => handleNudge(20, e.shiftKey)}
        className={buttonClass}
        title={`Nudge Deck ${deck} forward 20ms (Shift-click: 5ms)`}
        aria-label={`Nudge Deck ${deck} forward`}
      >
        <Plus size={13} aria-hidden />
      </button>
    </div>
  );
});

DJNudgeButtons.displayName = 'DJNudgeButtons';

export default DJNudgeButtons;
