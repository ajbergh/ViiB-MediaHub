/**
 * ViiB MediaHub - DJ Deck Status Bar Component
 * 
 * Compact horizontal strip consolidating deck toggle indicators:
 * KEY LOCK, SLIP, AUTO-GAIN. Each toggle has click interaction,
 * clear active/inactive states (not color-only), and tooltips.
 * 
 * @module components/dj/v2/DJDeckStatusBar
 */

import React from 'react';
import { useStore } from '../../../store';
import type { DeckId } from '../../../slices/djMixerSlice';

interface DJDeckStatusBarProps {
  deck: DeckId;
}

interface ToggleItem {
  key: string;
  label: string;
  activeLabel?: string;
  active: boolean;
  onClick: () => void;
  activeColor: string;
  tooltip: string;
}

export const DJDeckStatusBar: React.FC<DJDeckStatusBarProps> = ({ deck }) => {
  const keyLock = useStore(state => deck === 'A' ? state.djMixer.keyLockA : state.djMixer.keyLockB);
  const slipMode = useStore(state => deck === 'A' ? state.djMixer.slipModeA : state.djMixer.slipModeB);
  const autoGain = useStore(state => deck === 'A' ? state.djMixer.autoGainA : state.djMixer.autoGainB);
  const toggleKeyLock = useStore(state => state.toggleKeyLock);
  const toggleSlipMode = useStore(state => state.toggleSlipMode);
  const toggleAutoGain = useStore(state => state.toggleAutoGain);

  const handleKeyLock = () => {
    toggleKeyLock(deck);
  };
  const handleSlip = () => toggleSlipMode(deck);
  const handleAutoGain = () => toggleAutoGain(deck);

  const toggles: ToggleItem[] = [
    {
      key: 'keylock',
      label: '🔒 KEY',
      active: keyLock,
      onClick: handleKeyLock,
      activeColor: 'emerald',
      tooltip: `Key Lock ${keyLock ? 'ON' : 'OFF'} — ${keyLock ? 'Pitch preserved when tempo changes' : 'Pitch follows tempo'}`,
    },
    {
      key: 'slip',
      label: 'SLIP',
      active: slipMode,
      onClick: handleSlip,
      activeColor: 'orange',
      tooltip: `Slip Mode ${slipMode ? 'ON' : 'OFF'} — ${slipMode ? 'Playback continues in background' : 'Normal scratch behavior'}`,
    },
    {
      key: 'autogain',
      label: 'AG',
      active: autoGain,
      onClick: handleAutoGain,
      activeColor: 'cyan',
      tooltip: `Auto-Gain ${autoGain ? 'ON' : 'OFF'} — Normalizes track loudness`,
    },
  ];

  const colorMap: Record<string, { activeBg: string; activeText: string; activeBorder: string }> = {
    emerald: { activeBg: 'bg-[color-mix(in_srgb,var(--dj-key-active)_30%,transparent)]', activeText: 'text-[var(--dj-key-active)]', activeBorder: 'border-[color-mix(in_srgb,var(--dj-key-active)_50%,transparent)]' },
    orange: { activeBg: 'bg-[color-mix(in_srgb,var(--dj-hotcue)_30%,transparent)]', activeText: 'text-orange-300', activeBorder: 'border-[color-mix(in_srgb,var(--dj-hotcue)_50%,transparent)]' },
    cyan: { activeBg: 'bg-[color-mix(in_srgb,var(--dj-info)_30%,transparent)]', activeText: 'text-[var(--dj-info)]', activeBorder: 'border-[color-mix(in_srgb,var(--dj-info)_50%,transparent)]' },
  };

  return (
    <div className='flex items-center gap-1'>
      {toggles.map(toggle => {
        const colors = colorMap[toggle.activeColor];
        return (
          <button
            key={toggle.key}
            onClick={toggle.onClick}
            type='button'
            aria-pressed={toggle.active}
            className={`
              px-1.5 py-0.5 rounded border text-[12px] font-bold uppercase tracking-wider
              transition-all duration-100 flex items-center gap-0.5
              min-w-[32px] min-h-[32px] justify-center
              ${toggle.active
                ? `${colors.activeBg} ${colors.activeText} ${colors.activeBorder} font-extrabold`
                : 'bg-[var(--dj-surface-3)] text-[var(--dj-text-secondary)] border-[var(--dj-border-light)] hover:text-[var(--dj-text-secondary)] hover:bg-[var(--dj-border)]'}
            `}
            title={toggle.tooltip}
          >
            {toggle.label}
            {toggle.active && <span className='w-1 h-1 rounded-full bg-current' />}
          </button>
        );
      })}
    </div>
  );
};

export default React.memo(DJDeckStatusBar);
