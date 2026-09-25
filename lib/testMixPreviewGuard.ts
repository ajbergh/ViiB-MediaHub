import type { DeckId, DeckState } from '../slices/djMixerSlice';

export interface TestMixPreviewBaseline {
  volume: number;
  eq: DeckState['eq'];
}

export interface TestMixPreviewRouteOwnership {
  deck: DeckId;
  crossfader: number;
  startedAtCrossfader: number;
  masterCueEnabled: boolean;
  autoGainEnabled: boolean;
  keyLockEnabled: boolean;
  startedAtKeyLock: boolean;
  headphoneDeviceId: string;
  startedAtHeadphoneDeviceId: string;
  masterDeviceId: string;
  startedAtMasterDeviceId: string;
}

function hasDefaultFX(deck: DeckState): boolean {
  const fx = deck.fx;
  return !fx.filter.enabled && fx.filter.type === 'lowpass' && fx.filter.frequency === 1000 && fx.filter.resonance === 1
    && !fx.delay.enabled && fx.delay.time === 0.375 && fx.delay.feedback === 0.3 && fx.delay.mix === 0.3
    && !fx.reverb.enabled && fx.reverb.roomSize === 0.5 && fx.reverb.damping === 0.5 && fx.reverb.mix === 0.3
    && !fx.flanger.enabled && fx.flanger.rate === 0.5 && fx.flanger.depth === 0.5 && fx.flanger.feedback === 0.3;
}

/** Only an empty, default-state deck can be safely cleared after audition. */
export function isPristineEmptyPreviewDeck(deck: DeckState): boolean {
  return deck.track === null && !deck.isPlaying && deck.position === 0 && deck.duration === 0 && deck.cuePoint === 0
    && deck.analysisStatus === 'not_analyzed' && deck.originalBpm === null && deck.effectiveBpm === null && deck.key === null
    && deck.waveformPeaks === null && deck.beatGrid === null && deck.downbeatIndices === null && !deck.beatGridLocked
    && deck.beatGridSource === 'unknown' && deck.bpmConfidence === null && deck.tempoEvidence === null && deck.beatGridOffset === 0
    && deck.tempo === 1 && !deck.filter.enabled && deck.filter.value === 0
    && !deck.loop.enabled && deck.loop.start === 0 && deck.loop.end === 0 && deck.loop.pendingIn == null
    && deck.hotCues.length === 0 && !deck.cueEnabled && hasDefaultFX(deck);
}

/** Occupied preview is limited to a loaded, un-cued deck with a stable track identity. */
export function isRestorableOccupiedPreviewDeck(deck: DeckState, loadedTrackId: string | null): boolean {
  return !!deck.track && deck.track.id === loadedTrackId && !deck.cueEnabled && deck.analysisStatus !== 'loading';
}

/** Detect changes to user-owned mixer controls while the original deck is snapshotted. */
export function stillOwnsOccupiedPreviewBaseline(deck: DeckState, baseline: DeckState): boolean {
  return deck.track === baseline.track
    && deck.volume === baseline.volume
    && deck.eq.low === baseline.eq.low && deck.eq.mid === baseline.eq.mid && deck.eq.high === baseline.eq.high
    && deck.cuePoint === baseline.cuePoint && deck.tempo === baseline.tempo
    && deck.filter.enabled === baseline.filter.enabled && deck.filter.value === baseline.filter.value
    && deck.loop.enabled === baseline.loop.enabled && deck.loop.start === baseline.loop.start
    && deck.loop.end === baseline.loop.end && deck.loop.pendingIn === baseline.loop.pendingIn
    && JSON.stringify(deck.hotCues) === JSON.stringify(baseline.hotCues)
    && JSON.stringify(deck.fx) === JSON.stringify(baseline.fx)
    && deck.cueEnabled === baseline.cueEnabled;
}

export function isPreviewDeckOffAir(deck: DeckId, crossfader: number): boolean {
  return deck === 'A' ? crossfader === 1 : crossfader === -1;
}

export function hasSeparateHeadphoneRoute(headphoneDeviceId: string, masterDeviceId: string): boolean {
  return !!headphoneDeviceId && headphoneDeviceId !== 'default' && headphoneDeviceId !== masterDeviceId;
}

/** Keep destructive preview cleanup tied to the exact off-air route it started with. */
export function stillOwnsPreviewRoute(route: TestMixPreviewRouteOwnership): boolean {
  return route.crossfader === route.startedAtCrossfader
    && isPreviewDeckOffAir(route.deck, route.crossfader)
    && !route.masterCueEnabled && !route.autoGainEnabled
    && route.keyLockEnabled === route.startedAtKeyLock
    && route.headphoneDeviceId === route.startedAtHeadphoneDeviceId
    && route.masterDeviceId === route.startedAtMasterDeviceId
    && hasSeparateHeadphoneRoute(route.headphoneDeviceId, route.masterDeviceId);
}

/** Detect user-owned control changes before clearing the temporary candidate. */
export function isPreparedPreviewDeck(deck: DeckState, candidateId: string, baseline: TestMixPreviewBaseline): boolean {
  return deck.track?.id === candidateId && deck.cueEnabled
    && deck.volume === baseline.volume
    && deck.eq.low === baseline.eq.low && deck.eq.mid === baseline.eq.mid && deck.eq.high === baseline.eq.high
    && deck.cuePoint === 0 && deck.tempo === 1 && !deck.filter.enabled && deck.filter.value === 0
    && !deck.loop.enabled && deck.loop.start === 0 && deck.loop.end === 0 && deck.loop.pendingIn == null
    && deck.hotCues.length === 0 && hasDefaultFX(deck);
}

export function stillOwnsPreviewDeck(deck: DeckState, candidateId: string, baseline: TestMixPreviewBaseline): boolean {
  return deck.isPlaying && isPreparedPreviewDeck(deck, candidateId, baseline);
}
