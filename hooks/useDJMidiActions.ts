import { useEffect, useRef } from 'react';
import { useDJAudioEngineActions } from './useDJAudioEngine';
import { getDJMidiService, type MidiAction } from '../lib/djMidi';
import { getDJSamplerEngine } from '../lib/djSampler';
import { useStore } from '../store';
import type { DeckId } from '../slices/djMixerSlice';

const DECK_ACTION = /^(deck[AB])\.(.+)$/;

/** Connect persisted MIDI mappings to the active DJ audio, mixer and sampler APIs. */
export function useDJMidiActions(): void {
  const actions = useDJAudioEngineActions();
  const actionsRef = useRef(actions);
  actionsRef.current = actions;

  useEffect(() => {
    const midi = getDJMidiService();
    midi.loadMappings();
    const handler = (action: MidiAction, value: number, trigger: 'press' | 'release' | 'value') => {
      const match = DECK_ACTION.exec(action);
      const deck = match ? (match[1] === 'deckA' ? 'A' : 'B') as DeckId : null;
      const kind = match?.[2];
      const pressed = trigger === 'press';
      const normalized = Math.max(0, Math.min(1, value));

      const currentActions = actionsRef.current;
      if (action === 'crossfader' && trigger === 'value') { currentActions.setCrossfader(value * 2 - 1); return; }
      if (action === 'masterVolume' && trigger === 'value') { currentActions.setMasterVolume(normalized); return; }
      if (action.startsWith('sampler.pad')) {
        const pad = Number(action.slice('sampler.pad'.length)) - 1;
        if (pad < 0 || pad > 7) return;
        const sampler = getDJSamplerEngine();
        const padState = useStore.getState().djSampler[pad];
        if (trigger === 'press') sampler.triggerPad(pad);
        else if (trigger === 'release' && padState?.mode === 'gate') sampler.stopPad(pad);
        else if (trigger === 'value' && value > 0) sampler.triggerPad(pad);
        return;
      }
      if (!deck || !kind) return;

      if (kind === 'play' && pressed) { void currentActions.togglePlay(deck); return; }
      if (kind === 'cue') {
        if (pressed) { currentActions.returnToCue(deck); void currentActions.togglePlay(deck); }
        else if (trigger === 'release') currentActions.returnToCue(deck);
        return;
      }
      if (kind === 'sync' && pressed) { currentActions.syncBeatPhase(deck); return; }
      if (kind === 'loopIn' && pressed) { currentActions.setLoopIn(deck); return; }
      if (kind === 'loopOut' && pressed) { currentActions.setLoopOut(deck); return; }
      if (kind === 'loopToggle' && pressed) { currentActions.toggleLoop(deck); return; }
      if (kind === 'headphoneCue') { currentActions.setCueEnabled(deck, pressed); return; }
      if (kind === 'volume' && trigger === 'value') { currentActions.setVolume(deck, normalized); return; }
      if (kind === 'tempo' && trigger === 'value') { currentActions.setTempo(deck, 0.5 + normalized); return; }
      if (kind === 'jogWheel' && trigger === 'value') { currentActions.nudgePosition(deck, value * 100); return; }
      if (kind === 'fxWet' && trigger === 'value') {
        const state = deck === 'A' ? useStore.getState().djDeckA : useStore.getState().djDeckB;
        currentActions.setDelayFX(deck, state.fx.delay.enabled, state.fx.delay.time, state.fx.delay.feedback, normalized);
        return;
      }
      const eqBand = kind === 'eqHigh' ? 'high' : kind === 'eqMid' ? 'mid' : kind === 'eqLow' ? 'low' : null;
      if (eqBand && trigger === 'value') { currentActions.setEQ(deck, eqBand, -24 + normalized * 36); return; }
      const cueSlotMatch = /^hotCue([1-4])$/.exec(kind);
      if (cueSlotMatch && pressed) {
        const slot = Number(cueSlotMatch[1]);
        const state = deck === 'A' ? useStore.getState().djDeckA : useStore.getState().djDeckB;
        const cue = state.hotCues.find(item => item.slot === slot);
        if (cue) currentActions.seek(deck, cue.position);
      }
    };

    midi.setActionHandler(handler);
    return () => midi.setActionHandler(null);
  }, []);
}
