import React, { useCallback } from 'react';
import { Headphones } from 'lucide-react';
import { useStore } from '../../../store';
import type { DeckId } from '../../../slices/djMixerSlice';

interface DJCueButtonProps {
  deck: DeckId;
  compact?: boolean;
  className?: string;
}

export const DJCueButton: React.FC<DJCueButtonProps> = ({
  deck,
  compact = false,
  className = '',
}) => {
  const isActive = useStore(state => deck === 'A' ? state.djDeckA.cueEnabled : state.djDeckB.cueEnabled);
  const toggleDeckCue = useStore(state => state.toggleDeckCue);
  
  const activeColor = deck === 'A' ? 'var(--dj-hotcue)' : 'var(--dj-warning)';
  
  const handleClick = useCallback(() => {
    toggleDeckCue(deck);
  }, [deck, toggleDeckCue]);

  const compactClasses = isActive
    ? 'bg-[var(--dj-hotcue)] text-white shadow-lg shadow-[color-mix(in_srgb,var(--dj-hotcue)_40%,transparent)]'
    : 'bg-[var(--dj-border)] text-[var(--dj-text-secondary)] hover:bg-[var(--dj-border-light)] hover:text-white';

  const fullClasses = isActive
    ? 'text-white shadow-lg'
    : 'bg-[var(--dj-border)] text-[var(--dj-text-secondary)] hover:bg-[var(--dj-border-light)] hover:text-white';

  if (compact) {
    return (
      <button
        type='button'
        onClick={handleClick}
        aria-pressed={isActive}
        aria-label={`Headphone cue Deck ${deck}`}
        className={[
          'flex items-center justify-center',
          'w-8 h-8 rounded',
          'font-bold text-[12px] uppercase',
          'transition-all duration-100',
          compactClasses,
          className
        ].join(' ')}
        title={'Headphone Cue ' + deck + ' (PFL)'}
      >
        <Headphones size={14} />
      </button>
    );
  }

  return (
    <button
      type='button'
      onClick={handleClick}
      aria-pressed={isActive}
      aria-label={`Headphone cue Deck ${deck}`}
      className={[
        'flex items-center gap-1.5 px-3 min-h-8 rounded',
        'font-bold text-xs uppercase',
        'transition-all duration-100',
        fullClasses,
        className
      ].join(' ')}
      style={isActive ? {
        backgroundColor: activeColor,
        boxShadow: `0 4px 14px color-mix(in srgb, ${activeColor} 25%, transparent)`,
      } : undefined}
      title={'Headphone Cue ' + deck + ' (PFL)'}
    >
      <Headphones size={12} />
      <span>CUE</span>
    </button>
  );
};

export default DJCueButton;
