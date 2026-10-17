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
import { Lock } from 'lucide-react';
import { useStore } from '../../../store';
import type { DeckId } from '../../../slices/djMixerSlice';

interface DJDeckStatusBarProps {
  deck: DeckId;
}

interface ToggleItem {
  key: string;
  label: string;
  icon?: React.ReactNode;
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
      label: 'KEY',
      icon: <Lock size={10} strokeWidth={2.5} aria-hidden="true" />,
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
    emerald: { activeBg: 'bg-[color-mix(in_srgb,var(--dj-key-active)_22%,var(--dj-surface-2))]', activeText: 'text-[var(--dj-key-active)]', activeBorder: 'border-[color-mix(in_srgb,var(--dj-key-active)_55%,transparent)]' },
    orange: { activeBg: 'bg-[color-mix(in_srgb,var(--dj-hotcue)_22%,var(--dj-surface-2))]', activeText: 'text-orange-300', activeBorder: 'border-[color-mix(in_srgb,var(--dj-hotcue)_55%,transparent)]' },
    cyan: { activeBg: 'bg-[color-mix(in_srgb,var(--dj-info)_22%,var(--dj-surface-2))]', activeText: 'text-[var(--dj-info)]', activeBorder: 'border-[color-mix(in_srgb,var(--dj-info)_55%,transparent)]' },
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
              dj-status-toggle px-1.5 py-0.5 rounded border text-[10px] font-bold uppercase tracking-wider
              transition-all duration-100 flex items-center gap-1
              min-w-[32px] min-h-[24px] justify-center
              ${toggle.active
                ? `${colors.activeBg} ${colors.activeText} ${colors.activeBorder} font-extrabold shadow-[inset_0_1px_0_rgba(255,255,255,0.12)]`
                : 'bg-[var(--dj-surface-2)] text-[var(--dj-text-muted)] border-[var(--dj-border-light)] hover:text-[var(--dj-text-secondary)] hover:bg-[var(--dj-surface-3)]'}
            `}
            title={toggle.tooltip}
          >
            {toggle.icon}
            <span>{toggle.label}</span>
            <span
              className={`w-1.5 h-1.5 rounded-full transition-all ${
                toggle.active ? 'bg-current shadow-[0_0_5px_currentColor]' : 'bg-white/15'
              }`}
              aria-hidden='true'
            />
          </button>
        );
      })}
    </div>
  );
};

export default React.memo(DJDeckStatusBar);
