/**
 * ViiB MediaHub - DJ Mixer Panel (v2)
 *
 * Neutral center anchor between the decks (Plan §9, §10A.13). Order, top to
 * bottom: active-deck header, channel strips + master, headphone cue mix,
 * crossfader, sync mode + quantize, crossfader curve, then a tools panel
 * with Sampler, FX Pad (X-Y pad) and Beat FX tabs. The panel height is
 * budgeted for the tallest tool (Beat FX); the FX layout mode opens FX Pad.
 *
 * Leaf controls keep their own narrow store subscriptions.
 *
 * @module components/dj/v2/DJMixerPanel
 */

import React, { useEffect, useState } from 'react';
import { useStore } from '../../../store';
import type { DeckId, DJLayoutMode } from '../../../slices/djMixerSlice';
import { DJChannelStrip, DJMasterKnob, DJCrossfaderSelfSub } from './DJMixerComponents';
import { DJStereoVUMeter } from './DJVUMeter';
import { DJHeadphoneMix } from './DJHeadphoneMix';
import { DJSamplerPads } from './DJSamplerPads';
import { DJBeatFXPanel } from './DJBeatFXPanel';
import { DJFXPad } from './DJFXPad';

interface DJMixerPanelProps {
  getDeckALevels: () => { left: number; right: number };
  getDeckBLevels: () => { left: number; right: number };
  getMasterLevels: () => { left: number; right: number };
  onVolumeChange: (deck: DeckId, value: number) => void;
  onMasterVolumeChange: (knobValue: number) => void;
  onCrossfaderChange: (value: number) => void;
}

const SYNC_MODES = [
  { mode: 'off', label: 'OFF', title: 'Sync off' },
  { mode: 'bpm', label: 'BPM', title: 'Sync matches BPM only' },
  { mode: 'beat-phase', label: 'PHASE', title: 'Sync matches BPM and beat phase (needs verified grids)' },
] as const;

const CURVES = [
  { curve: 'linear', label: 'LIN', title: 'Crossfader curve: Linear' },
  { curve: 'constant-power', label: 'CP', title: 'Crossfader curve: Constant Power' },
  { curve: 'sharp', label: 'CUT', title: 'Crossfader curve: Sharp / Cut' },
] as const;

const MixerHeader = React.memo(function MixerHeader() {
  const activeDeck = useStore(s => s.djActiveDeck);
  const toggleActiveDeck = useStore(s => s.toggleActiveDeck);
  const select = (deck: DeckId) => { if (activeDeck !== deck) toggleActiveDeck(); };
  return (
    <div className='dj-mixer-head' role='group' aria-label='Active deck for keyboard shortcuts'>
      <button type='button' className='dj-btn' data-deck-accent='A' aria-pressed={activeDeck === 'A'}
        aria-label='Make Deck A the active deck' title='Active deck receives Space, nudge and hot-cue shortcuts' onClick={() => select('A')}>A</button>
      <span className='dj-mixer-title'>MIXER</span>
      <button type='button' className='dj-btn' data-deck-accent='B' aria-pressed={activeDeck === 'B'}
        aria-label='Make Deck B the active deck' title='Active deck receives Space, nudge and hot-cue shortcuts' onClick={() => select('B')}>B</button>
    </div>
  );
});

const SyncRow = React.memo(function SyncRow() {
  const syncMode = useStore(s => s.djMixer?.syncMode);
  const quantize = useStore(s => s.djMixer?.quantize);
  const setSyncMode = useStore(s => s.setSyncMode);
  const toggleQuantize = useStore(s => s.toggleQuantize);
  return (
    <div className='dj-mixer-row' role='group' aria-label='Sync mode'>
      {SYNC_MODES.map(({ mode, label, title }) => (
        <button key={mode} type='button' className={`dj-btn ${mode === 'bpm' ? 'dj-btn-key' : mode === 'beat-phase' ? 'dj-btn-warn' : ''}`}
          aria-pressed={syncMode === mode} title={title} onClick={() => setSyncMode(mode)}>{label}</button>
      ))}
      <span className='dj-toolbar-gap' aria-hidden='true' />
      <button type='button' className='dj-btn dj-btn-icon' aria-pressed={!!quantize} onClick={toggleQuantize}
        title={`Quantize ${quantize ? 'ON' : 'OFF'} - Snap actions to beat grid`} aria-label='Quantize'>Q</button>
    </div>
  );
});

const CurveRow = React.memo(function CurveRow() {
  const crossfaderCurve = useStore(s => s.djMixer?.crossfaderCurve);
  const setCrossfaderCurve = useStore(s => s.setCrossfaderCurve);
  return (
    <div className='dj-mixer-row' role='group' aria-label='Crossfader curve'>
      <span className='dj-label'>Curve</span>
      {CURVES.map(({ curve, label, title }) => (
        <button key={curve} type='button' className='dj-btn' aria-pressed={crossfaderCurve === curve} title={title}
          onClick={() => setCrossfaderCurve(curve)}>{label}</button>
      ))}
    </div>
  );
});

type MixerTool = 'sampler' | 'fxpad' | 'beatfx';
const MIXER_TOOLS: ReadonlyArray<{ id: MixerTool; label: string }> = [
  { id: 'sampler', label: 'Sampler' },
  { id: 'fxpad', label: 'FX Pad' },
  { id: 'beatfx', label: 'Beat FX' },
];

const MixerBottom = React.memo(function MixerBottom() {
  const layoutMode = useStore(s => s.djMixer?.djLayoutMode || 'perf') as DJLayoutMode;
  const [tool, setTool] = useState<MixerTool>(layoutMode === 'fx' ? 'fxpad' : 'sampler');
  // The FX layout mode brings the FX pad forward; other modes return to the sampler.
  useEffect(() => { setTool(layoutMode === 'fx' ? 'fxpad' : 'sampler'); }, [layoutMode]);
  const active = MIXER_TOOLS.find(entry => entry.id === tool)!;
  return (
    <div className='dj-mixer-bottom'>
      <div className='dj-mixer-bottom-tabs' role='tablist' aria-label='Mixer tools'>
        {MIXER_TOOLS.map(({ id, label }) => (
          <button key={id} type='button' role='tab' className='dj-btn dj-btn-xs' aria-selected={tool === id} aria-pressed={tool === id}
            onClick={() => setTool(id)}>{label}</button>
        ))}
      </div>
      <div className='dj-mixer-bottom-body' role='tabpanel' aria-label={active.label} data-dj-mixer-tool={tool}>
        {tool === 'sampler' && <DJSamplerPads fill />}
        {tool === 'fxpad' && <div className='dj-mixer-fxpad'><DJFXPad size={130} /></div>}
        {tool === 'beatfx' && <DJBeatFXPanel />}
      </div>
    </div>
  );
});

export const DJMixerPanel = React.memo(function DJMixerPanel({
  getDeckALevels, getDeckBLevels, getMasterLevels, onVolumeChange, onMasterVolumeChange, onCrossfaderChange,
}: DJMixerPanelProps) {
  return (
    <section className='dj-mixer' data-dj-mixer aria-label='Mixer'>
      <MixerHeader />
      <div className='dj-mixer-channels'>
        <DJChannelStrip deckId='A' getDeckLevels={getDeckALevels} onVolumeChange={onVolumeChange} />
        <div className='dj-mixer-master'>
          <span className='dj-label'>Master</span>
          <DJStereoVUMeter getLevels={getMasterLevels} height={64} channelWidth={6} gap={3} segments={14} showPeak />
          <DJMasterKnob onChange={onMasterVolumeChange} />
        </div>
        <DJChannelStrip deckId='B' getDeckLevels={getDeckBLevels} onVolumeChange={onVolumeChange} />
      </div>
      <div className='dj-mixer-section' aria-label='Headphone cue mix' role='group'>
        <DJHeadphoneMix width={200} />
      </div>
      <div className='dj-mixer-section'>
        <div className='w-full px-2'>
          <DJCrossfaderSelfSub onChange={onCrossfaderChange} width={-1} responsive />
        </div>
      </div>
      <div className='dj-mixer-section'>
        <SyncRow />
        <CurveRow />
      </div>
      <MixerBottom />
    </section>
  );
});
