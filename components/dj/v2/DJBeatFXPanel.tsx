import React, { memo, useCallback } from 'react';
import { Power } from 'lucide-react';
import { useStore } from '../../../store';
import type { BeatFXTarget, BeatFXType, BeatFraction } from '../../../slices/djMixerSlice';

const TARGETS: Array<{ value: BeatFXTarget; label: string }> = [
  { value: 'A', label: 'A' },
  { value: 'B', label: 'B' },
  { value: 'master', label: 'MST' },
];

const EFFECTS: Array<{ value: BeatFXType; label: string }> = [
  { value: 'delay', label: 'DLY' },
  { value: 'echo', label: 'ECHO' },
  { value: 'reverb', label: 'RVB' },
  { value: 'filter', label: 'FILT' },
  { value: 'flanger', label: 'FLG' },
];

const FRACTIONS: BeatFraction[] = ['1/4', '1/2', '1', '2', '4'];

interface DJBeatFXPanelProps {
  className?: string;
}

export const DJBeatFXPanel: React.FC<DJBeatFXPanelProps> = memo(({ className = '' }) => {
  const beatFX = useStore(state => state.djMixer.beatFX);
  const setBeatFXEnabled = useStore(state => state.setBeatFXEnabled);
  const setBeatFXTarget = useStore(state => state.setBeatFXTarget);
  const setBeatFXType = useStore(state => state.setBeatFXType);
  const setBeatFXFraction = useStore(state => state.setBeatFXFraction);
  const setBeatFXDepth = useStore(state => state.setBeatFXDepth);

  const handleToggle = useCallback(() => {
    setBeatFXEnabled(!beatFX.enabled);
  }, [beatFX.enabled, setBeatFXEnabled]);

  const activeColor = beatFX.target === 'A'
    ? 'blue'
    : beatFX.target === 'B'
      ? 'purple'
      : 'cyan';

  const onClass = activeColor === 'blue'
    ? 'bg-[var(--dj-deck-a)] text-white border-[var(--dj-deck-a-bright)] shadow-[color-mix(in_srgb,var(--dj-deck-a)_30%,transparent)]'
    : activeColor === 'purple'
      ? 'bg-[var(--dj-deck-b)] text-white border-[var(--dj-deck-b-bright)] shadow-[color-mix(in_srgb,var(--dj-deck-b)_30%,transparent)]'
      : 'bg-[var(--dj-info)] text-white border-[var(--dj-info)] shadow-[color-mix(in_srgb,var(--dj-info)_30%,transparent)]';

  return (
    <div className={`dj-beat-fx-panel w-[220px] rounded-md border border-[var(--dj-border)] bg-[var(--dj-bg)] p-2 ${className}`}>
      <div className='flex items-center justify-between gap-2'>
        <span className='text-[12px] font-bold text-[var(--dj-text-secondary)] uppercase tracking-widest'>Beat FX</span>
        <button
          type='button'
          onClick={handleToggle}
          aria-pressed={beatFX.enabled}
          className={`
            h-8 px-2 rounded border flex items-center gap-1.5 text-[12px] font-bold uppercase
            transition-all duration-100 shadow
            ${beatFX.enabled
              ? onClass
              : 'bg-[var(--dj-surface-3)] text-[var(--dj-text-secondary)] border-[var(--dj-border-light)] shadow-transparent hover:bg-[var(--dj-border)] hover:text-[var(--dj-text-primary)]'}
          `}
          title={`Beat FX ${beatFX.enabled ? 'ON' : 'OFF'}`}
        >
          <Power size={12} aria-hidden />
          {beatFX.enabled ? 'ON' : 'OFF'}
        </button>
      </div>

      <div className='mt-2 flex items-center gap-1'>
        {TARGETS.map(target => (
          <button
            key={target.value}
            type='button'
            onClick={() => setBeatFXTarget(target.value)}
            aria-pressed={beatFX.target === target.value}
            className={`
              flex-1 h-7 rounded border text-[12px] font-bold transition-colors
              ${beatFX.target === target.value
                ? target.value === 'A'
                  ? 'bg-[var(--dj-deck-a)] text-white border-[var(--dj-deck-a-bright)]'
                  : target.value === 'B'
                    ? 'bg-[var(--dj-deck-b)] text-white border-[var(--dj-deck-b-bright)]'
                    : 'bg-[var(--dj-info)] text-white border-[var(--dj-info)]'
                : 'bg-[var(--dj-surface-3)] text-[var(--dj-text-secondary)] border-[var(--dj-border)] hover:text-[var(--dj-text-primary)]'}
            `}
          >
            {target.label}
          </button>
        ))}
      </div>

      <div className='mt-2 grid grid-cols-5 gap-1'>
        {EFFECTS.map(effect => (
          <button
            key={effect.value}
            type='button'
            onClick={() => setBeatFXType(effect.value)}
            aria-pressed={beatFX.type === effect.value}
            className={`
              h-7 rounded border text-[12px] font-bold transition-colors
              ${beatFX.type === effect.value
                ? 'bg-[var(--dj-warning)] text-black border-[var(--dj-warning)]'
                : 'bg-[var(--dj-surface-3)] text-[var(--dj-text-secondary)] border-[var(--dj-border)] hover:text-[var(--dj-text-primary)]'}
            `}
          >
            {effect.label}
          </button>
        ))}
      </div>

      <div className='mt-2 grid grid-cols-5 gap-1'>
        {FRACTIONS.map(fraction => (
          <button
            key={fraction}
            type='button'
            onClick={() => setBeatFXFraction(fraction)}
            aria-pressed={beatFX.fraction === fraction}
            className={`
              h-7 rounded border text-[12px] font-mono font-bold transition-colors
              ${beatFX.fraction === fraction
                ? 'bg-[var(--dj-key-active)] text-white border-[var(--dj-key-active)]'
                : 'bg-[var(--dj-surface-3)] text-[var(--dj-text-secondary)] border-[var(--dj-border)] hover:text-[var(--dj-text-primary)]'}
            `}
          >
            {fraction}
          </button>
        ))}
      </div>

      <div className='mt-2 flex items-center gap-2'>
        <span className='w-9 text-[12px] font-bold text-[var(--dj-text-secondary)] uppercase tracking-wider'>Depth</span>
        <input
          type='range'
          min='0'
          max='1'
          step='0.01'
          value={beatFX.depth}
          onChange={event => setBeatFXDepth(Number(event.target.value))}
          className='flex-1 h-1 accent-amber-500'
          aria-label='Beat FX depth'
        />
        <span className='w-6 text-right text-[12px] font-mono text-[var(--dj-text-secondary)]'>
          {Math.round(beatFX.depth * 100)}
        </span>
      </div>
    </div>
  );
});

DJBeatFXPanel.displayName = 'DJBeatFXPanel';

export default DJBeatFXPanel;
