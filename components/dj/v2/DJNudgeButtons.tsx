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
  const handleNudge = useCallback((offsetMs: number, fine: boolean) => {
    onNudge(deck, fine ? Math.sign(offsetMs) * 5 : offsetMs);
  }, [deck, onNudge]);

  return (
    <div className='flex items-center gap-1' role='group' aria-label={`Deck ${deck} nudge controls`}>
      <button
        type='button'
        disabled={disabled}
        onClick={(e) => handleNudge(-20, e.shiftKey)}
        className='dj-btn dj-btn-icon'
        data-deck-accent={deck}
        title={`Nudge Deck ${deck} backward 20ms (Shift-click: 5ms)`}
        aria-label={`Nudge Deck ${deck} backward`}
      >
        <Minus size={13} aria-hidden />
      </button>
      <button
        type='button'
        disabled={disabled}
        onClick={(e) => handleNudge(20, e.shiftKey)}
        className='dj-btn dj-btn-icon'
        data-deck-accent={deck}
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
