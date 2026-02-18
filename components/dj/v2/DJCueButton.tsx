import React, { useCallback } from 'react';
import { Headphones } from 'lucide-react';
import { useStore } from '../../../store';
import { getDJAudioEngine } from '../../../lib/djAudio';
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
  const { djDeckA, djDeckB, toggleDeckCue } = useStore();
  
  const deckState = deck === 'A' ? djDeckA : djDeckB;
  const isActive = deckState.cueEnabled;
  
  const activeColor = deck === 'A' ? '#f97316' : '#f59e0b';
  
  const handleClick = useCallback(() => {
    toggleDeckCue(deck);
    
    const engine = getDJAudioEngine();
    if (engine.initialized) {
      const newState = !isActive;
      engine.setCueEnabled(deck, newState);
    }
  }, [deck, isActive, toggleDeckCue]);

  const compactClasses = isActive
    ? 'bg-orange-500 text-white shadow-lg shadow-orange-500/40'
    : 'bg-[#2a2a2a] text-neutral-400 hover:bg-[#333] hover:text-white';

  const fullClasses = isActive
    ? 'text-white shadow-lg'
    : 'bg-[#2a2a2a] text-neutral-400 hover:bg-[#333] hover:text-white';

  if (compact) {
    return (
      <button
        onClick={handleClick}
        className={[
          'flex items-center justify-center',
          'w-8 h-8 rounded',
          'font-bold text-[10px] uppercase',
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
      onClick={handleClick}
      className={[
        'flex items-center gap-1.5 px-3 py-1.5 rounded',
        'font-bold text-xs uppercase',
        'transition-all duration-100',
        fullClasses,
        className
      ].join(' ')}
      style={isActive ? {
        backgroundColor: activeColor,
        boxShadow: '0 4px 14px ' + activeColor + '40',
      } : undefined}
      title={'Headphone Cue ' + deck + ' (PFL)'}
    >
      <Headphones size={12} />
      <span>CUE</span>
    </button>
  );
};

export default DJCueButton;
