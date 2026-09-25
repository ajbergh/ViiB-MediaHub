import React, { useEffect, useMemo, useRef, useState } from 'react';
import { useStore } from '../../../store';
import { getDJAudioEngine, type SynchronizedPreviewSources } from '../../../lib/djAudio';
import { hasSeparateHeadphoneRoute, isPreviewDeckOffAir, isPristineEmptyPreviewDeck, isRestorableOccupiedPreviewDeck, stillOwnsDeckSnapshot, stillOwnsPreviewRoute } from '../../../lib/testMixPreviewGuard';
import { canAcceptMixNextCandidate, stillOwnsMixNextAcceptance, type MixNextAcceptanceOwnership } from '../../../lib/mixNextAcceptanceGuard';
import { useDJAudioEngineActions } from '../../../hooks/useDJAudioEngine';
import { api, normalizeTrackEnergyFeatures, type TrackBeatGrid, type TrackEnergyFeatures, type TrackTransitionRecommendations, type TransitionIntent, type TransitionRecommendationFilters } from '../../../services/api';
import { describeTestMixPhaseEvidence } from '../../../lib/testMixPhaseReadiness';
import { DJSavedMixIdeas } from './DJSavedMixIdeas';
import type { DeckState } from '../../../slices/djMixerSlice';
import type { Song } from '../../../types';

interface DJEnergyInsightsProps {
  trackID?: string;
  deck?: 'A' | 'B';
}

interface TestMixPreviewSession {
  token: number;
  targetDeck: 'A' | 'B';
  candidateId: string;
  candidateTrack: Song;
  phase: 'preparing' | 'loading' | 'playing';
  startedAtCrossfader: number;
  startedAtKeyLock: boolean;
  startedAtHeadphoneDeviceId: string;
  startedAtMasterDeviceId: string;
  timeout?: number;
  interval?: number;
  syncOwnership?: SynchronizedPreviewOwnership;
  synchronizedSources?: SynchronizedPreviewSources;
  abortController?: AbortController;
}

interface SynchronizedPreviewDeckBaseline {
  deck: 'A' | 'B';
  state: DeckState;
  trackId: string | null;
  loaded: boolean;
  loadGeneration: number;
  transportGeneration: number;
  stemGeneration: number;
  stemMode: ReturnType<ReturnType<typeof getDJAudioEngine>['getStemStatus']>['mode'];
  stemState: ReturnType<ReturnType<typeof getDJAudioEngine>['getStemState']>;
  keyLock: boolean;
  autoGain: boolean;
}

interface SynchronizedPreviewOwnership {
  reference: SynchronizedPreviewDeckBaseline;
  candidate: SynchronizedPreviewDeckBaseline;
}

function copyDeckSnapshot(deck: DeckState): DeckState {
  return {
    ...deck,
    eq: { ...deck.eq },
    filter: { ...deck.filter },
    waveformPeaks: deck.waveformPeaks ? [...deck.waveformPeaks] : null,
    beatGrid: deck.beatGrid ? [...deck.beatGrid] : null,
    downbeatIndices: deck.downbeatIndices ? [...deck.downbeatIndices] : null,
    tempoEvidence: deck.tempoEvidence ? { ...deck.tempoEvidence } : null,
    loop: { ...deck.loop },
    hotCues: deck.hotCues.map(cue => ({ ...cue })),
    fx: {
      filter: { ...deck.fx.filter }, delay: { ...deck.fx.delay },
      reverb: { ...deck.fx.reverb }, flanger: { ...deck.fx.flanger },
    },
  };
}

// This deliberately displays suggestions as opt-in actions. It never writes
// analysis output into a DJ's hot cues until the DJ accepts a specific cue.
export function DJEnergyInsights({ trackID, deck }: DJEnergyInsightsProps) {
  const [features, setFeatures] = useState<TrackEnergyFeatures | null>(null);
  const [recommendations, setRecommendations] = useState<TrackTransitionRecommendations | null>(null);
  const [candidatePhaseGrid, setCandidatePhaseGrid] = useState<{ trackId: string; grid: TrackBeatGrid | null; loaded: boolean } | null>(null);
  const [intent, setIntent] = useState<TransitionIntent>('hold');
  const [minBpm, setMinBpm] = useState('');
  const [maxBpm, setMaxBpm] = useState('');
  const [minEnergy, setMinEnergy] = useState('');
  const [maxEnergy, setMaxEnergy] = useState('');
  const [stemsOnly, setStemsOnly] = useState(false);
  const [camelotOnly, setCamelotOnly] = useState(false);
  const [playlistIds, setPlaylistIds] = useState<string[]>([]);
  const [genre, setGenre] = useState('');
  const [notRecentlyPlayedHours, setNotRecentlyPlayedHours] = useState('');
  const [previewMessage, setPreviewMessage] = useState('');
  const [acceptanceMessage, setAcceptanceMessage] = useState('');
  const [acceptanceBusy, setAcceptanceBusy] = useState(false);
  const acceptanceBusyRef = useRef(false);
  const previewRef = useRef<TestMixPreviewSession | null>(null);
  const previewTokenRef = useRef(0);
  const acceptanceTokenRef = useRef(0);
  const acceptanceMountedRef = useRef(false);
  const acceptanceViewRef = useRef<{ deck?: 'A' | 'B'; trackID?: string; candidateId: string | null }>({ candidateId: null });
  const loadTrack = useDJAudioEngineActions().loadTrack;
  const hotCues = useStore(state => deck === 'A' ? state.djDeckA.hotCues : state.djDeckB.hotCues);
  const analysisStatus = useStore(state => deck === 'A' ? state.djDeckA.analysisStatus : state.djDeckB.analysisStatus);
  const setHotCue = useStore(state => state.setHotCue);
  const librarySongs = useStore(state => state.songs);
  const libraryPlaylists = useStore(state => state.playlists);
  const genreOptions = useMemo(() => Array.from(new Map(librarySongs.flatMap(song => (song.genre ?? [])
    .map(value => value.trim().replace(/\s+/g, ' ')).filter(Boolean).map(value => [value.toLocaleLowerCase(), value] as const))).values()).sort((a, b) => a.localeCompare(b)), [librarySongs]);
  const previewDeckID = deck === 'A' ? 'B' : 'A';
  const previewDeckState = useStore(state => previewDeckID === 'A' ? state.djDeckA : state.djDeckB);
  const phaseReferenceState = useStore(state => deck === 'B' ? state.djDeckB : state.djDeckA);
  const crossfader = useStore(state => state.djMixer.crossfader);
  const masterCueEnabled = useStore(state => state.djMixer.masterCueEnabled);
  const autoGainEnabled = useStore(state => previewDeckID === 'A' ? state.djMixer.autoGainA : state.djMixer.autoGainB);
  const progressRef = useRef<HTMLDivElement>(null);
  const filters: TransitionRecommendationFilters = {
    ...(minBpm !== '' ? { minBpm: Number(minBpm) } : {}),
    ...(maxBpm !== '' ? { maxBpm: Number(maxBpm) } : {}),
    ...(minEnergy !== '' ? { minEnergyLevel: Number(minEnergy) } : {}),
    ...(maxEnergy !== '' ? { maxEnergyLevel: Number(maxEnergy) } : {}),
    ...(stemsOnly ? { stemsAvailable: true } : {}),
    ...(camelotOnly ? { camelotCompatible: true } : {}),
    ...(playlistIds.length ? { playlistIds } : {}),
    ...(genre ? { genre } : {}),
    ...(notRecentlyPlayedHours !== '' ? { notRecentlyPlayedHours: Number(notRecentlyPlayedHours) } : {}),
  };
  const bpmValuesValid = [minBpm, maxBpm].every(value => value === '' || (Number.isFinite(Number(value)) && Number(value) >= 60 && Number(value) <= 190));
  const energyValuesValid = [minEnergy, maxEnergy].every(value => value === '' || (Number.isInteger(Number(value)) && Number(value) >= 1 && Number(value) <= 10));
  const minBpmValid = minBpm === '' || maxBpm === '' || Number(minBpm) <= Number(maxBpm);
  const minEnergyValid = minEnergy === '' || maxEnergy === '' || Number(minEnergy) <= Number(maxEnergy);
  const filtersValid = bpmValuesValid && energyValuesValid && minBpmValid && minEnergyValid;

  useEffect(() => {
    if (!features || !deck) return;
    const updatePosition = () => {
      const progress = progressRef.current;
      if (!progress) return;
      const state = useStore.getState();
      const currentDeck = deck === 'A' ? state.djDeckA : state.djDeckB;
      const engine = getDJAudioEngine();
      const valid = currentDeck.track?.id === trackID && currentDeck.duration > 0;
      progress.hidden = !valid;
      if (!valid) return;
      const position = engine.initialized && (currentDeck.isPlaying || engine.isScratching(deck))
        ? engine.getPosition(deck) : currentDeck.position;
      const percent = Math.max(0, Math.min(100, position / currentDeck.duration * 100));
      progress.style.width = `${percent}%`;
      progress.setAttribute('aria-valuenow', String(Math.round(percent)));
      progress.setAttribute('aria-valuetext', `${Math.floor(position)} of ${Math.floor(currentDeck.duration)} seconds`);
    };
    updatePosition();
    const timer = window.setInterval(updatePosition, 50);
    return () => window.clearInterval(timer);
  }, [features, deck, trackID]);

  useEffect(() => {
    let live = true;
    setFeatures(null);
    setRecommendations(null);
    if (trackID && analysisStatus === 'available') {
      api.getTrackEnergyFeatures(trackID).then(value => live && setFeatures(normalizeTrackEnergyFeatures(value))).catch(() => {});
    }
    return () => { live = false; };
  }, [trackID, analysisStatus]);

  useEffect(() => {
    let live = true;
    setRecommendations(null);
    if (trackID && analysisStatus === 'available' && filtersValid) {
      api.getTrackTransitionRecommendations(trackID, 3, intent, filters).then(value => live && setRecommendations(value)).catch(() => {});
    }
    return () => { live = false; };
  }, [trackID, analysisStatus, intent, minBpm, maxBpm, minEnergy, maxEnergy, stemsOnly, camelotOnly, playlistIds, genre, notRecentlyPlayedHours, filtersValid]);

  const phaseCandidateId = recommendations?.recommendations[0]?.songId ?? null;
  useEffect(() => {
    let live = true;
    if (!phaseCandidateId) {
      setCandidatePhaseGrid(null);
      return () => { live = false; };
    }
    setCandidatePhaseGrid({ trackId: phaseCandidateId, grid: null, loaded: false });
    api.getTrackBeatGrid(phaseCandidateId).then(grid => {
      if (live) setCandidatePhaseGrid({ trackId: phaseCandidateId, grid, loaded: true });
    }).catch(() => {
      if (live) setCandidatePhaseGrid({ trackId: phaseCandidateId, grid: null, loaded: true });
    });
    return () => { live = false; };
  }, [phaseCandidateId]);

  const acceptCue = (position: number, kind: string) => {
    if (!deck) return;
    const slot = Array.from({ length: 8 }, (_, index) => index + 1).find(candidate => !hotCues.some(cue => cue.slot === candidate));
    if (!slot) return;
    setHotCue(deck, slot, position, `Suggested ${kind}`);
  };
  const top = recommendations?.recommendations[0];
  const candidateTrack = top ? librarySongs.find(song => song.id === top.songId) : undefined;
  const sourceTrack = trackID ? librarySongs.find(song => song.id === trackID) : undefined;
  const currentMixIdea = top && recommendations && sourceTrack ? {
    source: { trackId: sourceTrack.id, title: sourceTrack.title, artist: sourceTrack.artist ?? 'Unknown artist' },
    candidate: { trackId: top.songId, title: top.title, artist: top.artist },
    intent: recommendations.intent,
    algorithmVersion: recommendations.algorithmVersion,
    score: top.score,
    vector: top.vector,
    components: top.components,
    filters: recommendations.filters,
    filterEvidence: top.filterEvidence,
  } : null;
  const phaseGridState = candidateTrack && candidatePhaseGrid?.trackId === candidateTrack.id ? candidatePhaseGrid : undefined;
  const phaseGrid = !phaseGridState || !phaseGridState.loaded ? undefined
    : phaseGridState.grid?.songId === candidateTrack.id ? phaseGridState.grid : null;
  const phaseEvidenceStatus = !deck || phaseReferenceState.track?.id !== trackID
    ? 'Downbeat evidence unavailable: selected reference is not loaded on its deck.'
    : !candidateTrack
    ? 'Downbeat evidence unavailable: no current candidate.'
    : phaseGrid === undefined
      ? 'Checking manual downbeat evidence…'
      : describeTestMixPhaseEvidence({
        beats: phaseReferenceState.beatGrid,
        downbeatIndices: phaseReferenceState.downbeatIndices,
        locked: phaseReferenceState.beatGridLocked,
        source: phaseReferenceState.beatGridSource,
      }, phaseGrid ? {
        beats: phaseGrid.beats,
        downbeatIndices: phaseGrid.downbeatIndices,
        locked: phaseGrid.locked,
        source: phaseGrid.provenance ?? phaseGrid.source ?? 'unknown',
      } : null);
  acceptanceViewRef.current = { deck, trackID, candidateId: candidateTrack?.id ?? null };
  const engine = getDJAudioEngine();
  const headphoneDeviceId = engine.getHeadphoneOutputDeviceId();
  const mainDeviceId = engine.getMainOutputDeviceId();
  const previewDeckEmpty = isPristineEmptyPreviewDeck(previewDeckState);
  const previewDeckOccupied = isRestorableOccupiedPreviewDeck(previewDeckState, engine.getDeckLoadedTrackId(previewDeckID))
    && engine.isLoaded(previewDeckID) && !engine.isScratching(previewDeckID)
    && previewDeckState.isPlaying === engine.isPlaying(previewDeckID);
  const previewAllowed = !!deck && !!candidateTrack && engine.initialized
    && (previewDeckEmpty || previewDeckOccupied)
    && isPreviewDeckOffAir(previewDeckID, crossfader)
    && !masterCueEnabled && !engine.getMasterCueEnabled() && !autoGainEnabled
    && hasSeparateHeadphoneRoute(headphoneDeviceId, mainDeviceId);
  const previewReason = !deck ? 'A loaded reference deck is required.'
    : !candidateTrack ? 'Candidate is no longer in the local library.'
    : !engine.initialized ? 'Initialize audio before testing a mix.'
      : !previewDeckEmpty && !previewDeckOccupied ? 'Test Mix needs an empty deck or a loaded, uncued opposite deck with a verified source.'
        : !isPreviewDeckOffAir(previewDeckID, crossfader) ? 'Move the opposite deck fully off the master crossfader.'
          : masterCueEnabled || engine.getMasterCueEnabled() ? 'Turn off master monitoring in headphones first.'
            : autoGainEnabled ? 'Turn off auto-gain for the preview deck first.'
              : !hasSeparateHeadphoneRoute(headphoneDeviceId, mainDeviceId) ? 'Select a separate headphone output device first.' : '';

  const acceptanceTargetState = previewDeckState;
  const acceptanceAllowed = !!deck && !!candidateTrack && engine.initialized
    && canAcceptMixNextCandidate({
      targetDeck: previewDeckID, targetState: acceptanceTargetState,
      loadedTrackId: engine.getDeckLoadedTrackId(previewDeckID), engineLoading: engine.isDeckLoading(previewDeckID),
      engineLoaded: engine.isLoaded(previewDeckID),
      enginePlaying: engine.isPlaying(previewDeckID), engineCueEnabled: engine.getCueEnabled(previewDeckID),
      crossfader, masterCueEnabled: masterCueEnabled || engine.getMasterCueEnabled(),
    });
  const acceptanceReason = !deck ? 'A loaded reference deck is required.'
    : !candidateTrack ? 'Candidate is no longer in the local library.'
      : !engine.initialized ? 'Initialize audio before loading a candidate.'
        : engine.isDeckLoading(previewDeckID) ? 'Another load is already in progress on the opposite deck.'
          : !isPristineEmptyPreviewDeck(acceptanceTargetState) || engine.isLoaded(previewDeckID)
          || engine.getDeckLoadedTrackId(previewDeckID) !== null ? 'The opposite deck must be empty and unused.'
          : !isPreviewDeckOffAir(previewDeckID, crossfader) ? 'Move the opposite deck fully off the master crossfader.'
            : acceptanceTargetState.cueEnabled || engine.getCueEnabled(previewDeckID) ? 'Turn off headphone cue on the opposite deck.'
              : masterCueEnabled || engine.getMasterCueEnabled() ? 'Turn off master monitoring before loading.' : '';

  const discardPreviewSession = (session: TestMixPreviewSession) => {
    if (session.timeout !== undefined) window.clearTimeout(session.timeout);
    if (session.interval !== undefined) window.clearInterval(session.interval);
    session.abortController?.abort();
    session.abortController = undefined;
    session.synchronizedSources?.dispose();
    session.synchronizedSources = undefined;
    if (previewRef.current?.token === session.token) previewRef.current = null;
  };

  const ownsCurrentPreviewRoute = (session: TestMixPreviewSession, state: ReturnType<typeof useStore.getState>, audio: ReturnType<typeof getDJAudioEngine>) => {
    const targetIsA = session.targetDeck === 'A';
    return stillOwnsPreviewRoute({
      deck: session.targetDeck,
      crossfader: state.djMixer.crossfader,
      startedAtCrossfader: session.startedAtCrossfader,
      masterCueEnabled: state.djMixer.masterCueEnabled || audio.getMasterCueEnabled(),
      autoGainEnabled: targetIsA ? state.djMixer.autoGainA : state.djMixer.autoGainB,
      keyLockEnabled: targetIsA ? state.djMixer.keyLockA : state.djMixer.keyLockB,
      startedAtKeyLock: session.startedAtKeyLock,
      headphoneDeviceId: audio.getHeadphoneOutputDeviceId(),
      startedAtHeadphoneDeviceId: session.startedAtHeadphoneDeviceId,
      masterDeviceId: audio.getMainOutputDeviceId(),
      startedAtMasterDeviceId: session.startedAtMasterDeviceId,
    });
  };

  const ownsSynchronizedPreview = (session: TestMixPreviewSession, state: ReturnType<typeof useStore.getState>, audio: ReturnType<typeof getDJAudioEngine>) => {
    const ownership = session.syncOwnership;
    if (!ownership || previewRef.current?.token !== session.token) return false;
    const deckStillOwned = (baseline: SynchronizedPreviewDeckBaseline) => {
      const currentDeck = baseline.deck === 'A' ? state.djDeckA : state.djDeckB;
      const keyLock = baseline.deck === 'A' ? state.djMixer.keyLockA : state.djMixer.keyLockB;
      const autoGain = baseline.deck === 'A' ? state.djMixer.autoGainA : state.djMixer.autoGainB;
      return stillOwnsDeckSnapshot(currentDeck, baseline.state)
        && audio.getDeckLoadedTrackId(baseline.deck) === baseline.trackId
        && audio.isLoaded(baseline.deck) === baseline.loaded
        && audio.getDeckLoadGeneration(baseline.deck) === baseline.loadGeneration
        && audio.getDeckTransportControlGeneration(baseline.deck) === baseline.transportGeneration
        && audio.getStemControlGeneration(baseline.deck) === baseline.stemGeneration
        && audio.getStemStatus(baseline.deck).mode === baseline.stemMode
        && JSON.stringify(audio.getStemState(baseline.deck)) === JSON.stringify(baseline.stemState)
        && keyLock === baseline.keyLock && autoGain === baseline.autoGain;
    };
    return deckStillOwned(ownership.reference) && deckStillOwned(ownership.candidate)
      && ownsCurrentPreviewRoute(session, state, audio);
  };

  const finishPreview = (token: number, message: string) => {
    const session = previewRef.current;
    if (!session || session.token !== token) return;
    discardPreviewSession(session);
    setPreviewMessage(message);
  };
  const finishPreviewRef = useRef(finishPreview);
  finishPreviewRef.current = finishPreview;

  useEffect(() => {
    acceptanceMountedRef.current = true;
    return () => {
      acceptanceMountedRef.current = false;
      ++acceptanceTokenRef.current;
    };
  }, []);

  useEffect(() => () => {
    const session = previewRef.current;
    if (session) finishPreviewRef.current(session.token, 'Test Mix stopped.');
  }, []);

  if (analysisStatus === 'not_analyzed' || analysisStatus === 'error') return <div className="px-2 py-1 text-[10px] text-amber-400">{analysisStatus === 'not_analyzed' ? 'Track not analysed yet.' : 'Track analysis is unavailable.'} Energy insights and recommendations are unavailable.</div>;
  if (!features) return null;

  const acceptCandidate = async () => {
    if (acceptanceBusyRef.current) return;
    if (previewRef.current) { setAcceptanceMessage('Stop Test Mix before loading a candidate.'); return; }
    const audio = getDJAudioEngine();
    const state = useStore.getState();
    const latest = acceptanceViewRef.current;
    const target = previewDeckID;
    const targetState = target === 'A' ? state.djDeckA : state.djDeckB;
    const referenceState = deck === 'A' ? state.djDeckA : deck === 'B' ? state.djDeckB : null;
    const candidate = candidateTrack && state.songs.find(song => song.id === candidateTrack.id);
    if (!deck || !trackID || !candidate || latest.deck !== deck || latest.trackID !== trackID || latest.candidateId !== candidate.id) {
      setAcceptanceMessage('Candidate or reference changed. Refresh recommendations and choose the current top result.');
      return;
    }
    if (!audio.initialized || !canAcceptMixNextCandidate({
      targetDeck: target, targetState,
      loadedTrackId: audio.getDeckLoadedTrackId(target), engineLoading: audio.isDeckLoading(target),
      engineLoaded: audio.isLoaded(target),
      enginePlaying: audio.isPlaying(target), engineCueEnabled: audio.getCueEnabled(target),
      crossfader: state.djMixer.crossfader,
      masterCueEnabled: state.djMixer.masterCueEnabled || audio.getMasterCueEnabled(),
    })) {
      setAcceptanceMessage(acceptanceReason || 'The opposite deck is no longer available for acceptance.');
      return;
    }
    if (!referenceState?.track || referenceState.track.id !== trackID || !audio.isLoaded(deck)
      || audio.getDeckLoadedTrackId(deck) !== trackID) {
      setAcceptanceMessage('The selected reference track is no longer loaded on its deck.');
      return;
    }

    const ownership: MixNextAcceptanceOwnership = {
      targetDeck: target, referenceTrackId: trackID, candidateId: candidate.id,
      crossfader: state.djMixer.crossfader,
      targetCueEnabled: audio.getCueEnabled(target),
      masterCueEnabled: state.djMixer.masterCueEnabled || audio.getMasterCueEnabled(),
    };
    const generationBeforeLoad = audio.getDeckLoadGeneration(target);
    const targetBaseline = copyDeckSnapshot(targetState);
    const token = ++acceptanceTokenRef.current;
    acceptanceBusyRef.current = true;
    setAcceptanceBusy(true);
    setAcceptanceMessage(`Loading ${candidate.title} to Deck ${target}…`);

    const stillOwned = () => {
      if (!acceptanceMountedRef.current || acceptanceTokenRef.current !== token) return false;
      const current = useStore.getState();
      const currentReference = deck === 'A' ? current.djDeckA : current.djDeckB;
      const currentTarget = target === 'A' ? current.djDeckA : current.djDeckB;
      const currentMixerCue = current.djMixer.masterCueEnabled || audio.getMasterCueEnabled();
      const live = acceptanceViewRef.current;
      const routeAndIdentityStillOwned = live.deck === deck && live.trackID === trackID && live.candidateId === candidate.id
        && current.songs.some(song => song.id === candidate.id)
        && currentReference.track?.id === ownership.referenceTrackId
        && audio.isLoaded(deck) && audio.getDeckLoadedTrackId(deck) === ownership.referenceTrackId
        && stillOwnsMixNextAcceptance({
          ownership,
          referenceTrackId: currentReference.track?.id ?? null,
          candidateId: live.candidateId,
          crossfader: current.djMixer.crossfader,
          targetCueEnabled: audio.getCueEnabled(target) || currentTarget.cueEnabled,
          masterCueEnabled: currentMixerCue,
          targetIsStillEmpty: currentTarget.track === null && !currentTarget.isPlaying && !currentTarget.cueEnabled
            && stillOwnsDeckSnapshot(currentTarget, targetBaseline),
        });
      return routeAndIdentityStillOwned;
    };

    try {
      await loadTrack(target, candidate, { expectedLoadGeneration: generationBeforeLoad, shouldCommit: stillOwned });
      const after = useStore.getState();
      const loadedTarget = target === 'A' ? after.djDeckA : after.djDeckB;
      if (loadedTarget.track?.id === candidate.id && audio.getDeckLoadedTrackId(target) === candidate.id) {
        if (acceptanceMountedRef.current && acceptanceTokenRef.current === token) {
          setAcceptanceMessage(`Loaded ${candidate.title} on Deck ${target}. It remains stopped and ready when you are.`);
        }
      } else if (acceptanceMountedRef.current && acceptanceTokenRef.current === token) {
        setAcceptanceMessage('Load canceled: the candidate, route, or deck ownership changed before it could be committed.');
      }
    } catch {
      if (acceptanceMountedRef.current && acceptanceTokenRef.current === token) {
        setAcceptanceMessage('Could not load the candidate. The opposite deck was left in its current state.');
      }
    } finally {
      if (acceptanceTokenRef.current === token) {
        acceptanceBusyRef.current = false;
        if (acceptanceMountedRef.current) setAcceptanceBusy(false);
      }
    }
  };

  const testCandidate = async () => {
    if (!deck || !candidateTrack || !previewAllowed || previewRef.current) {
      setPreviewMessage(previewRef.current ? 'A Test Mix preview is already active.' : previewReason);
      return;
    }
    const audio = getDJAudioEngine();
    const state = useStore.getState();
    const referenceDeck = deck;
    const candidateDeck = previewDeckID;
    const referenceState = referenceDeck === 'A' ? state.djDeckA : state.djDeckB;
    const candidateState = candidateDeck === 'A' ? state.djDeckA : state.djDeckB;
    if (!referenceState.track || referenceState.track.id !== trackID || !audio.isLoaded(referenceDeck)
      || audio.getDeckLoadedTrackId(referenceDeck) !== referenceState.track.id) {
      setPreviewMessage('Test Mix needs the selected reference track loaded on its deck.');
      return;
    }
    const candidateEmpty = isPristineEmptyPreviewDeck(candidateState)
      && !audio.isLoaded(candidateDeck) && audio.getDeckLoadedTrackId(candidateDeck) === null;
    const candidateOccupied = isRestorableOccupiedPreviewDeck(candidateState, audio.getDeckLoadedTrackId(candidateDeck))
      && audio.isLoaded(candidateDeck) && candidateState.isPlaying === audio.isPlaying(candidateDeck);
    if (!candidateEmpty && !candidateOccupied) {
      setPreviewMessage('Test Mix needs an empty deck or a loaded, uncued opposite deck with a verified source.');
      return;
    }
    const referenceBpm = referenceState.effectiveBpm
      ?? (referenceState.originalBpm ? referenceState.originalBpm * referenceState.tempo : null);
    const candidateBpm = top?.filterEvidence.bpm;
    const requiredTempoShiftPercent = top?.vector.requiredTempoShiftPercent;
    if (!referenceBpm || !candidateBpm || requiredTempoShiftPercent === undefined
      || !Number.isFinite(requiredTempoShiftPercent) || !Number.isFinite(referenceBpm) || !Number.isFinite(candidateBpm)
      || referenceBpm <= 0 || candidateBpm <= 0) {
      setPreviewMessage('Test Mix needs confidence-qualified BPM evidence for both tracks to calculate tempo-only beatmatch.');
      return;
    }
    const candidateTempo = referenceBpm / candidateBpm;
    if (!Number.isFinite(candidateTempo) || candidateTempo < 0.5 || candidateTempo > 1.5) {
      setPreviewMessage('The recommended tempo match is outside the supported 50%–150% playback range.');
      return;
    }
    const baselineFor = (id: 'A' | 'B', deckState: DeckState): SynchronizedPreviewDeckBaseline => ({
      deck: id,
      state: copyDeckSnapshot(deckState),
      trackId: audio.getDeckLoadedTrackId(id),
      loaded: audio.isLoaded(id),
      loadGeneration: audio.getDeckLoadGeneration(id),
      transportGeneration: audio.getDeckTransportControlGeneration(id),
      stemGeneration: audio.getStemControlGeneration(id),
      stemMode: audio.getStemStatus(id).mode,
      stemState: audio.getStemState(id),
      keyLock: id === 'A' ? state.djMixer.keyLockA : state.djMixer.keyLockB,
      autoGain: id === 'A' ? state.djMixer.autoGainA : state.djMixer.autoGainB,
    });
    const baselineReference = baselineFor(referenceDeck, referenceState);
    const baselineCandidate = baselineFor(candidateDeck, candidateState);
    const startedAtHeadphoneDeviceId = audio.getHeadphoneOutputDeviceId();
    const startedAtMasterDeviceId = audio.getMainOutputDeviceId();
    const abortController = new AbortController();
    const session: TestMixPreviewSession = {
      token: ++previewTokenRef.current, targetDeck: candidateDeck, candidateId: candidateTrack.id, candidateTrack,
      phase: 'preparing',
      startedAtCrossfader: state.djMixer.crossfader, startedAtKeyLock: baselineCandidate.keyLock,
      startedAtHeadphoneDeviceId, startedAtMasterDeviceId,
      syncOwnership: { reference: baselineReference, candidate: baselineCandidate },
      abortController,
    };
    previewRef.current = session;
    setPreviewMessage('Preparing isolated headphone sources…');
    const stillOwned = () => ownsSynchronizedPreview(session, useStore.getState(), audio);
    try {
      const candidateFeatures = await api.getTrackEnergyFeatures(candidateTrack.id).catch(() => null);
      if (previewRef.current?.token !== session.token) return;
      if (!stillOwned()) {
        finishPreviewRef.current(session.token, 'Test Mix handed off: a deck, control, or route changed during preparation.');
        return;
      }
      const mixOut = features.cueSuggestions.find(cue => cue.kind === 'mix-out')?.position;
      const mixIn = candidateFeatures && normalizeTrackEnergyFeatures(candidateFeatures).cueSuggestions.find(cue => cue.kind === 'mix-in')?.position;
      if (mixOut === undefined || !Number.isFinite(mixOut) || mixOut < 0) {
        finishPreviewRef.current(session.token, 'Test Mix needs a valid recommended mix-out cue on the reference track.');
        return;
      }
      if (mixIn === undefined || !Number.isFinite(mixIn) || mixIn < 0) {
        finishPreviewRef.current(session.token, 'Test Mix needs a valid recommended mix-in cue on the candidate track.');
        return;
      }
      const referenceDuration = referenceState.duration || referenceState.track.duration;
      if (referenceDuration > 0 && mixOut >= referenceDuration) {
        finishPreviewRef.current(session.token, 'The reference mix-out cue is outside the track duration.');
        return;
      }
      if (candidateTrack.duration > 0 && mixIn >= candidateTrack.duration) {
        finishPreviewRef.current(session.token, 'The candidate mix-in cue is outside the track duration.');
        return;
      }
      session.phase = 'loading';
      const synchronizedSources = await audio.startSynchronizedPreview(referenceState.track, candidateTrack, {
        referencePosition: mixOut,
        candidatePosition: Math.max(0, Math.min(mixIn, candidateTrack.duration || mixIn)),
        referenceTempo: referenceState.tempo,
        candidateTempo,
        stillOwned,
        signal: abortController.signal,
      });
      if (previewRef.current?.token !== session.token) { synchronizedSources.dispose(); return; }
      session.synchronizedSources = synchronizedSources;
      if (!stillOwned()) {
        finishPreviewRef.current(session.token, 'Test Mix handed off: a deck, control, or route changed as playback began.');
        return;
      }
      session.phase = 'playing';
      session.interval = window.setInterval(() => {
        if (previewRef.current?.token !== session.token) return;
        if (!stillOwned()) {
          finishPreviewRef.current(session.token, 'Test Mix handed off: deck, source, controls, or headphone route changed; both decks were left untouched.');
          return;
        }
        const sources = session.synchronizedSources;
        if (!sources || !sources.reference.isPlaying() || !sources.candidate.isPlaying()) {
          finishPreviewRef.current(session.token, 'Test Mix complete; both original decks remain unchanged.');
        }
      }, 100);
      session.timeout = window.setTimeout(() => finishPreviewRef.current(session.token,
        'Test Mix complete; both original decks remain unchanged.'), 10000);
      setPreviewMessage(`Testing ${candidateTrack.title} against ${referenceState.track.title} in headphones; coordinated browser start is not sample-accurate…`);
    } catch {
      finishPreviewRef.current(session.token, stillOwned()
        ? 'Test Mix failed; both original decks remain unchanged.'
        : 'Test Mix handed off: deck, source, controls, or route changed; both decks were left untouched.');
    }
  };

  return <section aria-label="Measured track energy" className="px-2 py-1 text-[10px] text-neutral-400">
    <div className="relative flex h-5 items-end gap-px overflow-hidden" title="Track energy · Highlight shows playback position">
      {features.energy.map((point, index) => <i key={index} className="w-1 bg-cyan-400/70" style={{ height: `${Math.max(2, point.value * 100)}%` }} />)}
      {deck && <div ref={progressRef} hidden role="progressbar" aria-label={`Deck ${deck} track position`}
        aria-valuemin={0} aria-valuemax={100} aria-valuenow={0}
        className="pointer-events-none absolute inset-y-0 left-0 bg-white/15" style={{ width: '0%' }}>
        <span className="absolute inset-y-0 right-0 w-2 translate-x-1/2 bg-white/20" />
        <span className="absolute inset-y-0 right-0 w-0.5 bg-white" />
        <span className="absolute right-0 top-0 h-1.5 w-1.5 translate-x-1/2 rotate-45 bg-white" />
      </div>}
    </div>
    <div className="mt-1 flex flex-wrap items-center gap-2">
      <span title="Unweighted RMS-based estimate; not BS.1770 LUFS">Loudness proxy: {features.integratedLufs.toFixed(1)} dB</span>
      <span>{features.cueSuggestions.length} advisory cues</span>
      <label className="inline-flex items-center gap-1">
        <span>Mix Next</span>
        <select aria-label="Mix Next direction" value={intent} onChange={event => setIntent(event.target.value as TransitionIntent)}
          className="rounded border border-violet-500/30 bg-neutral-950 px-1 text-violet-200">
          <option value="hold">Hold</option>
          <option value="lift">Lift (+1 energy)</option>
          <option value="reset">Reset (-1 energy)</option>
          <option value="harmonic">Harmonic</option>
        </select>
      </label>
      <label className="inline-flex items-center gap-1" title="Inclusive BPM range; candidates without a BPM value are excluded">
        <span>BPM</span>
        <input aria-label="Minimum BPM" type="number" min={60} max={190} step="0.1" value={minBpm} onChange={event => setMinBpm(event.target.value)} placeholder="min"
          className="w-12 rounded border border-neutral-700 bg-neutral-950 px-1 text-neutral-200" />
        <span>–</span>
        <input aria-label="Maximum BPM" type="number" min={60} max={190} step="0.1" value={maxBpm} onChange={event => setMaxBpm(event.target.value)} placeholder="max"
          className="w-12 rounded border border-neutral-700 bg-neutral-950 px-1 text-neutral-200" />
      </label>
      <label className="inline-flex items-center gap-1" title="Inclusive Energy Level range (1–10); candidates without a score are excluded">
        <span>Energy</span>
        <input aria-label="Minimum Energy Level" type="number" min={1} max={10} step={1} value={minEnergy} onChange={event => setMinEnergy(event.target.value)} placeholder="min"
          className="w-9 rounded border border-neutral-700 bg-neutral-950 px-1 text-neutral-200" />
        <span>–</span>
        <input aria-label="Maximum Energy Level" type="number" min={1} max={10} step={1} value={maxEnergy} onChange={event => setMaxEnergy(event.target.value)} placeholder="max"
          className="w-9 rounded border border-neutral-700 bg-neutral-950 px-1 text-neutral-200" />
      </label>
      <label className="inline-flex items-center gap-1" title="Only include candidates with a registered ready stem set">
        <input aria-label="Stems available only" type="checkbox" checked={stemsOnly} onChange={event => setStemsOnly(event.target.checked)} />
        <span>Stems</span>
      </label>
      <label className="inline-flex items-center gap-1" title="Only include candidates with trusted keys in the same, adjacent, or relative Camelot relation">
        <input aria-label="Compatible Camelot only" type="checkbox" checked={camelotOnly} onChange={event => setCamelotOnly(event.target.checked)} />
        <span>Compatible Camelot only</span>
      </label>
      <label className="inline-flex items-center gap-1" title="Include candidates that belong to any selected playlist; this filter combines with the other filters using AND.">
        <span>Playlists</span>
        <select aria-label="Mix Next playlists (any selected)" multiple size={3} value={playlistIds}
          onChange={event => setPlaylistIds(Array.from(event.currentTarget.selectedOptions, option => option.value))}
          className="max-w-36 rounded border border-neutral-700 bg-neutral-950 px-1 text-neutral-200">
          {libraryPlaylists.map(playlist => <option key={playlist.id} value={playlist.id}>{playlist.name}</option>)}
        </select>
        {playlistIds.length > 0 && <button type="button" onClick={() => setPlaylistIds([])} className="rounded border border-neutral-700 px-1">All</button>}
      </label>
      <label className="inline-flex items-center gap-1" title="Exact normalized genre membership; candidates without this genre are excluded">
        <span>Genre</span>
        <select aria-label="Mix Next genre" value={genre} onChange={event => setGenre(event.target.value)}
          className="max-w-28 rounded border border-neutral-700 bg-neutral-950 px-1 text-neutral-200">
          <option value="">All genres</option>
          {genreOptions.map(option => <option key={option} value={option}>{option}</option>)}
        </select>
      </label>
      <label className="inline-flex items-center gap-1" title="Exclude tracks with a completed play in this period. Skips and listening events do not update completed-play history.">
        <span>Exclude tracks completed in the last</span>
        <select aria-label="Exclude tracks completed in the last" value={notRecentlyPlayedHours} onChange={event => setNotRecentlyPlayedHours(event.target.value)}
          className="rounded border border-neutral-700 bg-neutral-950 px-1 text-neutral-200">
          <option value="">Off</option>
          {[1, 3, 6, 12, 24, 48, 72, 168].map(hours => <option key={hours} value={hours}>{hours}</option>)}
        </select>
        <span>hours</span>
      </label>
      {!filtersValid && <span role="status" className="text-amber-400">Check ranges: BPM 60–190; Energy Level 1–10; minimum must not exceed maximum.</span>}
      {features.cueSuggestions.slice(0, 3).map((cue, index) => {
        const accepted = hotCues.some(hotCue => Math.abs(hotCue.position - cue.position) < .01);
        return <button key={`${cue.kind}-${index}`} disabled={!deck || accepted} onClick={() => acceptCue(cue.position, cue.kind)} title={cue.rationale}
          className="rounded border border-cyan-500/30 px-1 text-cyan-300 disabled:border-neutral-700 disabled:text-neutral-600">
          {accepted ? `${cue.kind} added` : `Add ${cue.kind}`}
        </button>;
      })}
    </div>
    {top && <details className="mt-1 text-neutral-500">
      <summary className="cursor-pointer text-violet-300">Recommended next: {top.title} — {top.artist} ({Math.round(top.score * 100)}%)</summary>
      <div className="mt-2 flex items-center gap-2 text-neutral-300">
        <button type="button" disabled={!acceptanceAllowed || acceptanceBusy || !!previewRef.current}
          onClick={() => void acceptCandidate()}
          title={acceptanceBusy ? 'Candidate load is in progress.' : !acceptanceAllowed ? acceptanceReason : 'Load this candidate onto the empty, off-air opposite deck. It will remain stopped.'}
          className="rounded border border-emerald-500/40 px-2 py-1 text-emerald-200 disabled:cursor-not-allowed disabled:opacity-50">
          {acceptanceBusy ? 'Loading candidate…' : `Load to Deck ${previewDeckID}`}
        </button>
        {previewRef.current ? <button type="button" onClick={() => finishPreview(previewRef.current!.token, 'Test Mix stopped; both original decks remain unchanged.')}
          className="rounded border border-amber-500/40 px-2 py-1 text-amber-200">Stop Test Mix</button>
          : <button type="button" disabled={!previewAllowed || !!previewRef.current} onClick={() => void testCandidate()}
            title={previewRef.current ? 'Another Test Mix preview is active.' : !previewAllowed ? previewReason : 'Detached headphone copies audition both cue regions; browser-coordinated start is not sample-accurate, and both live deck states remain untouched.'}
            className="rounded border border-violet-500/40 px-2 py-1 text-violet-200 disabled:cursor-not-allowed disabled:opacity-50">Test Mix in headphones</button>}
        <span role="status" title="This checks only reviewed downbeat evidence. Current HTML-media sources cannot share a scheduled start, and phrase length is not part of the grid metadata."
          className="text-neutral-500">{phaseEvidenceStatus}</span>
        {acceptanceMessage && <span role="status">{acceptanceMessage}</span>}
        {previewMessage && <span role="status">{previewMessage}</span>}
      </div>
      <ul className="mt-1 space-y-0.5 pl-3">
        {top.filterEvidence.lastPlayed !== undefined && <li>Last completed play: {top.filterEvidence.lastPlayed ? new Date(top.filterEvidence.lastPlayed).toLocaleString() : 'never recorded'}</li>}
        {top.components.map(component => <li key={component.name} title={component.rationale}>
          {component.name}: {Math.round(component.score * 100)}% — {component.rationale}
        </li>)}
      </ul>
    </details>}
    <DJSavedMixIdeas currentIdea={currentMixIdea} />
    {recommendations && recommendations.candidatesAfterFilters === 0 && <p role="status" className="mt-1 text-neutral-500">
      No analyzed candidates match these filters ({recommendations.candidatesBeforeFilters} checked).
    </p>}
  </section>;
}
