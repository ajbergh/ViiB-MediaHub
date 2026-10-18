/** Dispatches mapped MIDI controller actions into DJ store and transport operations. */

import { useEffect, useRef, useState } from 'react';
import { useDJAudioEngineActions } from './useDJAudioEngine';
import { getDJMidiService, type MidiAction, type MidiMapping } from '../lib/djMidi';
import { getDJSamplerEngine } from '../lib/djSampler';
import { syncUnavailableReason } from '../lib/djMixerControls';
import { DJPadModeController, type PerformancePad } from '../lib/djPadModes';
import { useStore } from '../store';
import type { DeckId } from '../slices/djMixerSlice';
import type { StemBus } from '../lib/stemDeckSource';
import { buildDeckMidiFeedback, mappedPadLEDValue, pulseDeckMidiFeedback, type DeckFeedback } from '../lib/djMidiFeedback';
import { browseDJMidiLibrary, loadDJMidiLibrary, toggleDJMidiLibrary } from '../lib/djMidiLibrary';

const DECK_ACTION = /^(deck[AB])\.(.+)$/;

/** Connect persisted MIDI mappings to the active DJ audio, mixer and sampler APIs. */
export function useDJMidiActions(): boolean {
  const actions = useDJAudioEngineActions();
  const actionsRef = useRef(actions);
  actionsRef.current = actions;
  const [midiEnabled, setMidiEnabled] = useState(() => getDJMidiService().isEnabled());

  useEffect(() => {
    const midi = getDJMidiService();
    midi.loadMappings();
    const heldButtons = new Map<string, { deviceId: string; deck: DeckId; kind: string }>();
    const touchedJogs = new Map<DeckId, string>();
    const stemPresets = new Map<DeckId, { track: unknown; preset: string; restore: Record<StemBus, boolean> }>();
    const toggleBeatLoop = (deck: DeckId, beats: number) => {
      const state = useStore.getState();
      const target = deck === 'A' ? state.djDeckA : state.djDeckB;
      if (!target.track) return;
      const bpm = target.originalBpm || 120;
      const loopBeats = target.loop.presetBeats ?? (target.loop.end - target.loop.start) * bpm / 60;
      const matches = target.loop.presetBeats !== undefined ? loopBeats === beats
        : Math.abs(loopBeats - beats) < Math.min(0.15, beats / 4);
      if (target.loop.enabled && matches) actionsRef.current.toggleLoop(deck);
      else actionsRef.current.setLoopBeats(deck, beats);
    };
    const padModes = new DJPadModeController({
      hotCue: (deck, slot) => {
        const state = useStore.getState();
        const target = deck === 'A' ? state.djDeckA : state.djDeckB;
        if (!target.track) return;
        const cue = target.hotCues.find(item => item.slot === slot);
        if (cue) actionsRef.current.seek(deck, cue.position);
        else state.setHotCue(deck, slot, target.position);
      },
      autoLoop: toggleBeatLoop,
      sampler: (deck, pad) => {
        const index = (deck === 'A' ? 0 : 4) + pad - 1;
        const sampler = getDJSamplerEngine();
        const gate = useStore.getState().djSampler[index]?.mode === 'gate';
        sampler.triggerPad(index);
        return gate ? () => sampler.stopPad(index) : undefined;
      },
      stems: (deck, pad) => {
        const current = actionsRef.current;
        const status = current.getStemStatus(deck);
        // Keep input synchronous: enable stem playback with the existing Stems
        // controls first; never enqueue a mute against a later-loaded track.
        if (!status.available || status.mode !== 'stems') return;
        const buses: StemBus[] = ['vocals', 'music', 'bass', 'drums'];
        const bus = buses[pad - 1];
        current.setStemMuted(deck, bus, !current.getStemState(deck)[bus].muted);
      },
      modeLED: (deck, mode, deviceId) => {
        midi.sendPadModeFeedback(deviceId, deck, mode);
        midi.refreshFeedback();
      },
    });
    midi.setPadModeProvider(deck => padModes.getMode(deck));
    midi.setFeedbackProvider(deviceId => {
      const state = useStore.getState();
      const decks = {} as Record<DeckId, DeckFeedback>;
      for (const deck of ['A', 'B'] as const) {
        const mode = padModes.getMode(deck);
        let stems: DeckFeedback['stems'];
        try {
          // Dedicated ACAPEL/INSTRU indicators work in every pad bank.
          stems = { status: actionsRef.current.getStemStatus(deck), state: actionsRef.current.getStemState(deck) };
        } catch { /* Engine not ready: stem indicators stay dark. */ }
        const held = Array.from(heldButtons.values()).filter(button => button.deck === deck);
        decks[deck] = {
          deck: deck === 'A' ? state.djDeckA : state.djDeckB, mode, stems,
          samples: state.djSampler.slice(deck === 'A' ? 0 : 4, deck === 'A' ? 4 : 8),
          cuePressed: held.some(button => button.kind === 'cue'),
          syncPressed: held.some(button => button.kind === 'sync'),
        };
      }
      const custom = deviceId ? midi.getMappings().filter(mapping => !mapping.builtIn
        && (mapping.deviceId === deviceId || mapping.deviceId === '*')) : [];
      return (['A', 'B'] as const).flatMap((deck, index) => {
        const messages = buildDeckMidiFeedback(deck, decks[deck]);
        for (let pad = 0; pad < 4; pad++) {
          const candidates = custom.filter(mapping => (mapping.type === 'noteon' || mapping.type === 'noteoff')
            && (mapping.channel === -1 || mapping.channel === 4 + index) && mapping.note === 0x14 + pad
            && (mapping.padMode === undefined || (mapping.deck === deck && mapping.padMode === decks[deck].mode)));
          const mapping = candidates.find(mapping => mapping.padMode !== undefined) ?? candidates[0];
          if (mapping) messages[4 + pad] = [messages[4 + pad][0], messages[4 + pad][1], mappedPadLEDValue(mapping.action, decks)];
        }
        return pulseDeckMidiFeedback(deck, deck === 'A' ? state.djDeckA : state.djDeckB, messages);
      });
    });
    const unsubscribeFeedback = useStore.subscribe((state, previous) => {
      const changed = state.djSampler !== previous.djSampler || (['djDeckA', 'djDeckB'] as const).some(key => {
        const a = state[key]; const b = previous[key];
        return a.track !== b.track || a.isPlaying !== b.isPlaying || a.cueEnabled !== b.cueEnabled
          || a.hotCues !== b.hotCues || a.loop !== b.loop || a.effectiveBpm !== b.effectiveBpm || a.originalBpm !== b.originalBpm
          || a.beatGrid !== b.beatGrid || a.beatGridOffset !== b.beatGridOffset
          || a.beatGridLocked !== b.beatGridLocked || a.beatGridSource !== b.beatGridSource || a.tempo !== b.tempo;
      });
      if (changed) midi.refreshFeedback();
    });
    // Read independent playback positions, never accumulate a free-running BPM
    // timer. Poll loaded, paused decks too: UI stem controls live in the engine.
    // The MIDI output cache transmits only brightness changes.
    const feedbackTimer = window.setInterval(() => {
      const state = useStore.getState();
      if (state.djDeckA.isPlaying || state.djDeckB.isPlaying
        || state.djDeckA.track || state.djDeckB.track
        || padModes.getMode('A') === 3 || padModes.getMode('B') === 3) midi.refreshFeedback();
    }, 50);
    const updateMidiEnabled = () => {
      setMidiEnabled(midi.isEnabled());
      for (const device of midi.getDevices()) {
        if (!device.connected) {
          padModes.disconnectDevice(device.id);
          for (const [key, held] of heldButtons) if (held.deviceId === device.id) heldButtons.delete(key);
          for (const [deck, deviceId] of touchedJogs) if (deviceId === device.id) {
            actionsRef.current.endScratch(deck);
            touchedJogs.delete(deck);
          }
        }
      }
      midi.refreshFeedback();
    };
    const unsubscribe = midi.subscribe(updateMidiEnabled);
    updateMidiEnabled();
    const handler = (action: MidiAction, value: number, trigger: 'press' | 'release' | 'value', mapping: MidiMapping) => {
      const match = DECK_ACTION.exec(action);
      const deck = match ? (match[1] === 'deckA' ? 'A' : 'B') as DeckId : null;
      const kind = match?.[2];
      const pressed = trigger === 'press';
      const normalized = Math.max(0, Math.min(1, value));

      const currentActions = actionsRef.current;
      if (action === 'library.open' && pressed) { toggleDJMidiLibrary(); return; }
      if (action === 'headphoneVolume' && trigger === 'value') { currentActions.setHeadphoneVolume(normalized); return; }
      if (action === 'library.browse' && trigger === 'value') {
        const steps = value === 0 ? 0 : Math.sign(value) * Math.max(1, Math.round(Math.abs(value) * 63));
        if (steps) browseDJMidiLibrary(steps);
        return;
      }
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
      if (kind === 'load' && pressed) { loadDJMidiLibrary(deck); return; }
      if (kind === 'jogTouch') {
        if (pressed) { touchedJogs.set(deck, mapping.deviceId); currentActions.startScratch(deck); }
        else if (trigger === 'release') { touchedJogs.delete(deck); currentActions.endScratch(deck); }
        return;
      }
      if ((kind === 'stemAcapella' || kind === 'stemInstrumental') && pressed) {
        if (!currentActions.getStemStatus(deck).available || currentActions.getStemStatus(deck).mode !== 'stems') return;
        const state = useStore.getState();
        const track = (deck === 'A' ? state.djDeckA : state.djDeckB).track;
        const buses: StemBus[] = ['vocals', 'music', 'bass', 'drums'];
        const current = currentActions.getStemState(deck);
        const previous = stemPresets.get(deck);
        const restore = previous?.track === track ? previous.restore : Object.fromEntries(buses.map(bus => [bus, current[bus].muted])) as Record<StemBus, boolean>;
        const toggleOff = previous?.track === track && previous.preset === kind;
        for (const bus of buses) currentActions.setStemMuted(deck, bus, toggleOff ? restore[bus] : kind === 'stemAcapella' ? bus !== 'vocals' : bus === 'vocals');
        if (toggleOff) stemPresets.delete(deck); else stemPresets.set(deck, { track, preset: kind, restore });
        midi.refreshFeedback();
        return;
      }
      if (kind === 'filter' && trigger === 'value') {
        const bipolar = normalized * 2 - 1;
        useStore.getState().setDeckFilter(deck, bipolar);
        const enabled = Math.abs(bipolar) >= 0.05;
        currentActions.setFilterFX(deck, enabled, bipolar < 0 || !enabled ? 'lowpass' : 'highpass',
          !enabled ? 20000 : bipolar < 0 ? 200 * Math.pow(100, 1 + bipolar) : 20 + bipolar * 7980, enabled ? 2 : 0.5);
        return;
      }

      if ((kind === 'cue' || kind === 'sync') && trigger !== 'value') {
        const key = `${mapping.deviceId}:${deck}:${kind}`;
        const alreadyPressed = heldButtons.has(key);
        if (pressed) heldButtons.set(key, { deviceId: mapping.deviceId, deck, kind });
        else heldButtons.delete(key);
        midi.refreshFeedback();
        if (pressed && alreadyPressed) return;
      }

      if (kind === 'padMode') {
        if (trigger !== 'value') padModes.handleButton(deck, 'mode', pressed, mapping.deviceId);
        return;
      }
      const performancePad = /^performancePad([1-4])$/.exec(kind);
      if (performancePad) {
        if (trigger !== 'value') padModes.handleButton(deck, Number(performancePad[1]) as PerformancePad, pressed, mapping.deviceId);
        midi.refreshFeedback();
        return;
      }

      if (kind === 'play' && pressed) { void currentActions.togglePlay(deck); return; }
      if (kind === 'cue') {
        if (pressed) { currentActions.returnToCue(deck); void currentActions.togglePlay(deck); }
        else if (trigger === 'release') currentActions.returnToCue(deck);
        return;
      }
      if (kind === 'sync' && pressed) {
        const state = useStore.getState();
        const target = deck === 'A' ? state.djDeckA : state.djDeckB;
        const source = deck === 'A' ? state.djDeckB : state.djDeckA;
        const syncMode = state.djMixer.syncMode;
        const sourceBpm = source.effectiveBpm || source.originalBpm;
        if (!syncUnavailableReason(state, deck) && target.originalBpm && sourceBpm) {
          currentActions.setTempo(deck, Math.max(0.5, Math.min(1.5, sourceBpm / target.originalBpm)));
          if (syncMode === 'beat-phase') currentActions.syncBeatPhase(deck);
        }
        return;
      }
      if (kind === 'loopIn' && pressed) { currentActions.setLoopIn(deck); return; }
      if (kind === 'loopOut' && pressed) { currentActions.setLoopOut(deck); return; }
      if (kind === 'loopToggle' && pressed) {
        if (mapping.loopBeats !== undefined) toggleBeatLoop(deck, mapping.loopBeats);
        else currentActions.toggleLoop(deck);
        return;
      }
      if (kind === 'headphoneCue') {
        if (mapping.valueMode === 'momentary') {
          if (trigger !== 'value') currentActions.setCueEnabled(deck, pressed);
        } else if (trigger === 'value') {
          currentActions.setCueEnabled(deck, normalized >= 0.5);
        } else if (pressed) {
          currentActions.toggleCue(deck);
        }
        return;
      }
      if (kind === 'volume' && trigger === 'value') { currentActions.setVolume(deck, normalized); return; }
      if (kind === 'tempo' && trigger === 'value') {
        currentActions.setTempo(deck, mapping.builtIn ? 1 + (normalized * 2 - 1) * (mapping.tempoRange ?? 0.08) : 0.5 + normalized);
        return;
      }
      if (kind === 'jogWheel' && trigger === 'value') {
        if (touchedJogs.has(deck)) currentActions.updateScratch(deck, value * 0.1, value * 10);
        else currentActions.nudgePosition(deck, value * 100);
        return;
      }
      if (kind === 'fxWet' && trigger === 'value') {
        const state = deck === 'A' ? useStore.getState().djDeckA : useStore.getState().djDeckB;
        currentActions.setDelayFX(deck, state.fx.delay.enabled, state.fx.delay.time, state.fx.delay.feedback, normalized);
        return;
      }
      const eqBand = kind === 'eqHigh' ? 'high' : kind === 'eqMid' ? 'mid' : kind === 'eqLow' ? 'low' : null;
      if (eqBand && trigger === 'value') {
        const gain = mapping.builtIn ? (normalized <= 0.5 ? -24 + normalized * 48 : (normalized - 0.5) * 24) : -24 + normalized * 36;
        currentActions.setEQ(deck, eqBand, gain);
        return;
      }
      const cueSlotMatch = /^hotCue([1-4])$/.exec(kind);
      if (cueSlotMatch && pressed) {
        const slot = Number(cueSlotMatch[1]);
        const state = useStore.getState();
        const target = deck === 'A' ? state.djDeckA : state.djDeckB;
        const cue = target.hotCues.find(item => item.slot === slot);
        if (!target.track) return;
        if (cue) currentActions.seek(deck, cue.position);
        else state.setHotCue(deck, slot, target.position);
      }
    };

    midi.setActionHandler(handler);
    return () => {
      unsubscribe();
      unsubscribeFeedback();
      window.clearInterval(feedbackTimer);
      padModes.dispose();
      for (const deck of touchedJogs.keys()) actionsRef.current.endScratch(deck);
      heldButtons.clear();
      midi.setFeedbackProvider(null);
      midi.setPadModeProvider(null);
      midi.setActionHandler(null);
    };
  }, []);

  return midiEnabled;
}
