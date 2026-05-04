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
    ? 'bg-blue-600 text-white border-blue-400 shadow-blue-500/30'
    : activeColor === 'purple'
      ? 'bg-purple-600 text-white border-purple-400 shadow-purple-500/30'
      : 'bg-cyan-600 text-white border-cyan-400 shadow-cyan-500/30';

  return (
    <div className={`w-[196px] rounded-md border border-[#282828] bg-[#101010] p-2 ${className}`}>
      <div className='flex items-center justify-between gap-2'>
        <span className='text-[10px] font-bold text-neutral-500 uppercase tracking-widest'>Beat FX</span>
        <button
          type='button'
          onClick={handleToggle}
          aria-pressed={beatFX.enabled}
          className={`
            h-8 px-2 rounded border flex items-center gap-1.5 text-[10px] font-bold uppercase
            transition-all duration-100 shadow
            ${beatFX.enabled
              ? onClass
              : 'bg-[#222] text-neutral-500 border-[#333] shadow-transparent hover:bg-[#2a2a2a] hover:text-neutral-200'}
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
              flex-1 h-7 rounded border text-[10px] font-bold transition-colors
              ${beatFX.target === target.value
                ? target.value === 'A'
                  ? 'bg-blue-600 text-white border-blue-400'
                  : target.value === 'B'
                    ? 'bg-purple-600 text-white border-purple-400'
                    : 'bg-cyan-600 text-white border-cyan-400'
                : 'bg-[#1d1d1d] text-neutral-500 border-[#2f2f2f] hover:text-neutral-200'}
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
              h-7 rounded border text-[9px] font-bold transition-colors
              ${beatFX.type === effect.value
                ? 'bg-amber-500 text-black border-amber-300'
                : 'bg-[#1d1d1d] text-neutral-500 border-[#2f2f2f] hover:text-neutral-200'}
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
              h-7 rounded border text-[10px] font-mono font-bold transition-colors
              ${beatFX.fraction === fraction
                ? 'bg-emerald-600 text-white border-emerald-400'
                : 'bg-[#1d1d1d] text-neutral-500 border-[#2f2f2f] hover:text-neutral-200'}
            `}
          >
            {fraction}
          </button>
        ))}
      </div>

      <div className='mt-2 flex items-center gap-2'>
        <span className='w-9 text-[9px] font-bold text-neutral-500 uppercase tracking-wider'>Depth</span>
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
        <span className='w-6 text-right text-[10px] font-mono text-neutral-400'>
          {Math.round(beatFX.depth * 100)}
        </span>
      </div>
    </div>
  );
});

DJBeatFXPanel.displayName = 'DJBeatFXPanel';

export default DJBeatFXPanel;
