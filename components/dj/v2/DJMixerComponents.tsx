/**
 * ViiB MediaHub - DJ Mode V2 Self-Subscribing Mixer Components
 *
 * These wrappers were previously inlined in DJModeV2.tsx (lines ~113–285).
 * Each component subscribes only to the precise store slice it needs, so a
 * tempo drag on Deck A will NOT re-render the Deck B channel strip, the master
 * knob, or the crossfader.
 *
 * Components:
 *  - DJChannelStrip     — EQ knobs, filter, volume fader, VU meter for one deck
 *  - DJMasterKnob       — Self-subscribing master volume knob
 *  - DJCrossfaderSelfSub — Self-subscribing crossfader wrapper
 *  - DJTempoSliderSelfSub — Self-subscribing tempo slider wrapper
 *
 * @module components/dj/v2/DJMixerComponents
 */

import React from 'react';
import { useStore } from '../../../store';
import { DJEQKnob } from './DJEQKnob';
import { DJVolumeFader } from './DJVolumeFader';
import { DJCrossfader } from './DJCrossfader';
import { DJTempoSlider } from './DJTempoSlider';
import { DJCueButton } from './DJCueButton';
import { DJStereoVUMeter } from './DJVUMeter';
import type { DeckId, DeckEQ } from '../../../slices/djMixerSlice';

// ---------------------------------------------------------------------------
// DJChannelStrip
// ---------------------------------------------------------------------------

interface DJChannelStripProps {
  deckId: DeckId;
  getDeckLevels: () => { left: number; right: number };
  onVolumeChange: (deck: DeckId, value: number) => void;
}

/**
 * Self-subscribing channel strip — fader + VU only.
 * EQ, TRIM, and Filter are now in DJDeckEQStrip (placed beside the jog wheel).
 */
export const DJChannelStrip = React.memo<DJChannelStripProps>(({
  deckId,
  getDeckLevels,
  onVolumeChange,
}) => {
  const isA = deckId === 'A';
  const accentColor = isA ? '#3b82f6' : '#8b5cf6';
  const volume    = useStore(s => isA ? s.djDeckA.volume    : s.djDeckB.volume);
  const isPlaying = useStore(s => isA ? s.djDeckA.isPlaying : s.djDeckB.isPlaying);

  return (
    <div className='flex-1 flex flex-col items-center py-1 gap-2 min-w-[90px] min-h-0 border-[#2a2a2a]'>
      {/* Spacer — pushes fader towards bottom */}
      <div className='flex-1' />

      {/* Headphone Cue button */}
      <div className='mb-2'>
        <DJCueButton deck={deckId} />
      </div>

      {/* Channel Fader with VU */}
      <div className='w-full px-2 flex items-center justify-center gap-1'>
        <DJStereoVUMeter
          getLevels={getDeckLevels}
          height={110}
          channelWidth={4}
          gap={2}
          segments={16}
          showPeak={true}
        />
        <DJVolumeFader
          value={volume}
          onChange={(v) => onVolumeChange(deckId, v)}
          label=''
          height={160}
          isPlaying={isPlaying}
          accentColor={accentColor}
        />
      </div>
    </div>
  );
});
DJChannelStrip.displayName = 'DJChannelStrip';

// ---------------------------------------------------------------------------
// DJDeckEQStrip — compact EQ panel placed beside the jog wheel
// ---------------------------------------------------------------------------

interface DJDeckEQStripProps {
  deckId: DeckId;
  onEQChange: (deck: DeckId, band: keyof DeckEQ, value: number) => void;
  onVolumeChange: (deck: DeckId, value: number) => void;
  onFilterChange: (deck: DeckId, knobValue: number) => void;
}

/**
 * Compact vertical EQ strip (TRIM + HIGH + MID + LOW + FILTER) at size=28.
 * Placed in the blank space beside each deck's jog wheel.
 */
export const DJDeckEQStrip = React.memo<DJDeckEQStripProps>(({
  deckId,
  onEQChange,
  onVolumeChange,
  onFilterChange,
}) => {
  const isA = deckId === 'A';
  const accentColor   = isA ? '#3b82f6' : '#8b5cf6';
  const eqHigh        = useStore(s => isA ? s.djDeckA.eq.high       : s.djDeckB.eq.high);
  const eqMid         = useStore(s => isA ? s.djDeckA.eq.mid        : s.djDeckB.eq.mid);
  const eqLow         = useStore(s => isA ? s.djDeckA.eq.low        : s.djDeckB.eq.low);
  const filterValue   = useStore(s => isA ? s.djDeckA.filter.value  : s.djDeckB.filter.value);
  const filterEnabled = useStore(s => isA ? s.djDeckA.filter.enabled: s.djDeckB.filter.enabled);
  const volume        = useStore(s => isA ? s.djDeckA.volume        : s.djDeckB.volume);

  const filterKnobValue = ((filterValue + 1) / 2) * 36 - 24;

  // Compact mode hides each knob's value bar + numeric readout, saving ~22px
  // per knob (×5) so the column fits in the available deck height even at 1080p.
  return (
    <div className='flex flex-col items-center justify-evenly h-full py-2 px-1 gap-0.5'>
      <DJEQKnob
        label='TRIM'
        value={parseFloat(((volume / 1.5) * 36 - 24).toFixed(1))}
        onChange={(v) => {
          const normalized = (v + 24) / 36;
          onVolumeChange(deckId, Math.max(0, Math.min(1, normalized * 1.5)));
        }}
        color={accentColor}
        size={40}
        compact
      />
      <DJEQKnob label='HIGH'   value={eqHigh}          onChange={v => onEQChange(deckId, 'high', v)}   color='#06b6d4' size={40} compact />
      <DJEQKnob label='MID'    value={eqMid}           onChange={v => onEQChange(deckId, 'mid', v)}    color='#22c55e' size={40} compact />
      <DJEQKnob label='LOW'    value={eqLow}           onChange={v => onEQChange(deckId, 'low', v)}    color='#f59e0b' size={40} compact />
      <DJEQKnob
        label='FILT'
        value={filterKnobValue}
        onChange={v => onFilterChange(deckId, v)}
        color={filterEnabled ? '#ef4444' : '#666'}
        size={40}
        compact
      />
    </div>
  );
});
DJDeckEQStrip.displayName = 'DJDeckEQStrip';

// ---------------------------------------------------------------------------
// DJMasterKnob
// ---------------------------------------------------------------------------

interface DJMasterKnobProps {
  onChange: (v: number) => void;
}

/** Self-subscribing master volume knob — isolates masterVolume subscription. */
export const DJMasterKnob = React.memo<DJMasterKnobProps>(({ onChange }) => {
  const masterVolume = useStore(s => s.djMixer?.masterVolume ?? 0.8);
  return (
    <DJEQKnob
      label='MAIN'
      value={parseFloat((masterVolume * 36 - 24).toFixed(1))}
      onChange={onChange}
      size={48}
      color='#fff'
    />
  );
});
DJMasterKnob.displayName = 'DJMasterKnob';

// ---------------------------------------------------------------------------
// DJCrossfaderSelfSub
// ---------------------------------------------------------------------------

interface DJCrossfaderSelfSubProps {
  onChange: (v: number) => void;
  width: number;
  responsive?: boolean;
}

/** Self-subscribing crossfader — prevents parent re-renders during fader drag. */
export const DJCrossfaderSelfSub = React.memo<DJCrossfaderSelfSubProps>(
  ({ onChange, width, responsive }) => {
    const crossfader = useStore(s => s.djMixer?.crossfader ?? 0);
    return (
      <DJCrossfader
        value={crossfader}
        onChange={onChange}
        width={width}
        responsive={responsive}
      />
    );
  },
);
DJCrossfaderSelfSub.displayName = 'DJCrossfaderSelfSub';

// ---------------------------------------------------------------------------
// DJTempoSliderSelfSub
// ---------------------------------------------------------------------------

interface DJTempoSliderSelfSubProps {
  deck: DeckId;
  onChange: (v: number) => void;
  disabled?: boolean;
  height: number;
  responsive?: boolean;
}

/** Self-subscribing tempo slider — prevents parent re-renders during tempo drag. */
export const DJTempoSliderSelfSub = React.memo<DJTempoSliderSelfSubProps>(
  ({ deck, onChange, disabled, height, responsive }) => {
    const isA = deck === 'A';
    const tempo       = useStore(s => isA ? s.djDeckA.tempo        : s.djDeckB.tempo);
    const effectiveBpm = useStore(s => isA ? s.djDeckA.effectiveBpm : s.djDeckB.effectiveBpm);
    const originalBpm  = useStore(s => isA ? s.djDeckA.originalBpm  : s.djDeckB.originalBpm);

    return (
      <DJTempoSlider
        deck={deck}
        value={tempo}
        onChange={onChange}
        originalBpm={originalBpm}
        effectiveBpm={effectiveBpm}
        disabled={disabled}
        height={height}
        responsive={responsive}
      />
    );
  },
);
DJTempoSliderSelfSub.displayName = 'DJTempoSliderSelfSub';
