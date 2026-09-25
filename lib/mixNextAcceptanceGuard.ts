import type { DeckId, DeckState } from '../slices/djMixerSlice';
import { isPreviewDeckOffAir, isPristineEmptyPreviewDeck } from './testMixPreviewGuard';

export interface MixNextAcceptanceOwnership {
  targetDeck: DeckId;
  referenceTrackId: string;
  candidateId: string;
  crossfader: number;
  targetCueEnabled: boolean;
  masterCueEnabled: boolean;
}

/** True only for a genuinely empty target whose audio source is also unused. */
export function canAcceptMixNextCandidate(input: {
  targetDeck: DeckId;
  targetState: DeckState;
  loadedTrackId: string | null;
  engineLoading: boolean;
  engineLoaded: boolean;
  enginePlaying: boolean;
  engineCueEnabled: boolean;
  crossfader: number;
  masterCueEnabled: boolean;
}): boolean {
  return isPristineEmptyPreviewDeck(input.targetState)
    && input.loadedTrackId === null
    && !input.engineLoading
    && !input.engineLoaded
    && !input.enginePlaying
    && !input.engineCueEnabled
    && !input.targetState.cueEnabled
    && !input.masterCueEnabled
    && isPreviewDeckOffAir(input.targetDeck, input.crossfader);
}

/** Check that reference identity, route and cue state have not changed during load. */
export function stillOwnsMixNextAcceptance(input: {
  ownership: MixNextAcceptanceOwnership;
  referenceTrackId: string | null;
  candidateId: string | null;
  crossfader: number;
  targetCueEnabled: boolean;
  masterCueEnabled: boolean;
  targetIsStillEmpty: boolean;
}): boolean {
  const { ownership } = input;
  return input.referenceTrackId === ownership.referenceTrackId
    && input.candidateId === ownership.candidateId
    && input.crossfader === ownership.crossfader
    && isPreviewDeckOffAir(ownership.targetDeck, input.crossfader)
    && input.targetCueEnabled === ownership.targetCueEnabled
    && input.targetCueEnabled === false
    && input.masterCueEnabled === ownership.masterCueEnabled
    && input.masterCueEnabled === false
    && input.targetIsStillEmpty;
}
