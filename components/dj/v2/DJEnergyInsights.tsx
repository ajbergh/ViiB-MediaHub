import React, { useEffect, useMemo, useRef, useState } from 'react';
import { useStore } from '../../../store';
import { getDJAudioEngine, type PreparedDeckTrack, type RetainedDeckSource } from '../../../lib/djAudio';
import { hasSeparateHeadphoneRoute, isPreviewDeckOffAir, isPreparedPreviewDeck, isPristineEmptyPreviewDeck, isRestorableOccupiedPreviewDeck, stillOwnsOccupiedPreviewBaseline, stillOwnsPreviewDeck, stillOwnsPreviewRoute, stillOwnsPreviewTransport, type TestMixPreviewBaseline } from '../../../lib/testMixPreviewGuard';
import { api, type TrackEnergyFeatures, type TrackTransitionRecommendations, type TransitionIntent, type TransitionRecommendationFilters } from '../../../services/api';
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
  baseline: TestMixPreviewBaseline;
  occupied: boolean;
  originalDeck: DeckState;
  originalPosition: number;
  originalKeyLock: boolean;
  originalStemMode: ReturnType<ReturnType<typeof getDJAudioEngine>['getStemStatus']>['mode'];
  originalStemState: ReturnType<ReturnType<typeof getDJAudioEngine>['getStemState']>;
  previewStemMode: ReturnType<ReturnType<typeof getDJAudioEngine>['getStemStatus']>['mode'] | null;
  previewStemState: ReturnType<ReturnType<typeof getDJAudioEngine>['getStemState']> | null;
  previewStemControlGeneration: number | null;
  originalTransportControlGeneration: number;
  previewTransportControlGeneration: number | null;
  originalStemControlGeneration: number;
  generationBeforeLoad: number;
  loadGeneration: number | null;
  preparedCandidate: PreparedDeckTrack | null;
  retainedSource: RetainedDeckSource | null;
  phase: 'preparing' | 'loading' | 'ready' | 'playing';
  startedAtCrossfader: number;
  startedAtKeyLock: boolean;
  startedAtHeadphoneDeviceId: string;
  startedAtMasterDeviceId: string;
  timeout?: number;
  interval?: number;
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
  const [intent, setIntent] = useState<TransitionIntent>('hold');
  const [minBpm, setMinBpm] = useState('');
  const [maxBpm, setMaxBpm] = useState('');
  const [minEnergy, setMinEnergy] = useState('');
  const [maxEnergy, setMaxEnergy] = useState('');
  const [stemsOnly, setStemsOnly] = useState(false);
  const [camelotOnly, setCamelotOnly] = useState(false);
  const [playlistId, setPlaylistId] = useState('');
  const [genre, setGenre] = useState('');
  const [notRecentlyPlayedHours, setNotRecentlyPlayedHours] = useState('');
  const [previewMessage, setPreviewMessage] = useState('');
  const previewRef = useRef<TestMixPreviewSession | null>(null);
  const previewTokenRef = useRef(0);
  const hotCues = useStore(state => deck === 'A' ? state.djDeckA.hotCues : state.djDeckB.hotCues);
  const analysisStatus = useStore(state => deck === 'A' ? state.djDeckA.analysisStatus : state.djDeckB.analysisStatus);
  const setHotCue = useStore(state => state.setHotCue);
  const librarySongs = useStore(state => state.songs);
  const libraryPlaylists = useStore(state => state.playlists);
  const genreOptions = useMemo(() => Array.from(new Map(librarySongs.flatMap(song => (song.genre ?? [])
    .map(value => value.trim().replace(/\s+/g, ' ')).filter(Boolean).map(value => [value.toLocaleLowerCase(), value] as const))).values()).sort((a, b) => a.localeCompare(b)), [librarySongs]);
  const previewDeckID = deck === 'A' ? 'B' : 'A';
  const previewDeckState = useStore(state => previewDeckID === 'A' ? state.djDeckA : state.djDeckB);
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
    ...(playlistId ? { playlistId } : {}),
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
      api.getTrackEnergyFeatures(trackID).then(value => live && setFeatures(value)).catch(() => {});
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
  }, [trackID, analysisStatus, intent, minBpm, maxBpm, minEnergy, maxEnergy, stemsOnly, camelotOnly, playlistId, genre, notRecentlyPlayedHours, filtersValid]);

  if (analysisStatus === 'not_analyzed' || analysisStatus === 'error') return <div className="px-2 py-1 text-[10px] text-amber-400">{analysisStatus === 'not_analyzed' ? 'Track not analysed yet.' : 'Track analysis is unavailable.'} Energy insights and recommendations are unavailable.</div>;
  if (!features) return null;
  const acceptCue = (position: number, kind: string) => {
    if (!deck) return;
    const slot = Array.from({ length: 8 }, (_, index) => index + 1).find(candidate => !hotCues.some(cue => cue.slot === candidate));
    if (!slot) return;
    setHotCue(deck, slot, position, `Suggested ${kind}`);
  };
  const top = recommendations?.recommendations[0];
  const candidateTrack = top ? librarySongs.find(song => song.id === top.songId) : undefined;
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

  const discardPreviewSession = (session: TestMixPreviewSession, preserveRetained = false) => {
    if (session.timeout !== undefined) window.clearTimeout(session.timeout);
    if (session.interval !== undefined) window.clearInterval(session.interval);
    const audio = getDJAudioEngine();
    if (session.preparedCandidate?.state === 'prepared') audio.discardPreparedTrack(session.preparedCandidate);
    session.preparedCandidate = null;
    if (!preserveRetained && session.retainedSource?.state === 'retained') audio.discardRetainedDeckSource(session.retainedSource);
    if (!preserveRetained) session.retainedSource = null;
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

  const ownsOccupiedPreviewControls = (session: TestMixPreviewSession, deckState: DeckState, audio: ReturnType<typeof getDJAudioEngine>) =>
    deckState.track === session.candidateTrack
    && isPreparedPreviewDeck(deckState, session.candidateId, session.baseline)
    && session.previewStemMode !== null && session.previewStemState !== null
    && audio.getStemStatus(session.targetDeck).mode === session.previewStemMode
    && JSON.stringify(audio.getStemState(session.targetDeck)) === JSON.stringify(session.previewStemState)
    && session.previewStemControlGeneration === audio.getStemControlGeneration(session.targetDeck)
    && session.previewTransportControlGeneration !== null
    && stillOwnsPreviewTransport(session.previewTransportControlGeneration, audio.getDeckTransportControlGeneration(session.targetDeck));

  const restoreOccupiedPreview = (session: TestMixPreviewSession, audio: ReturnType<typeof getDJAudioEngine>, message: string) => {
    const target = session.targetDeck;
    const stateBeforeRestore = useStore.getState();
    const targetBeforeRestore = target === 'A' ? stateBeforeRestore.djDeckA : stateBeforeRestore.djDeckB;
    const retained = session.retainedSource;
    const baselineStillOwned = session.phase === 'loading'
      ? stillOwnsOccupiedPreviewBaseline(targetBeforeRestore, session.originalDeck)
      : (session.occupied ? ownsOccupiedPreviewControls(session, targetBeforeRestore, audio)
        : targetBeforeRestore.track === session.candidateTrack && isPreparedPreviewDeck(targetBeforeRestore, session.candidateId, session.baseline));
    if (!retained || !baselineStillOwned || audio.getDeckLoadGeneration(target) !== session.loadGeneration
      || audio.getDeckLoadedTrackId(target) !== session.candidateId
      || (session.phase === 'loading' && !stillOwnsPreviewTransport(session.originalTransportControlGeneration, audio.getDeckTransportControlGeneration(target)))
      || (session.phase === 'loading' && audio.getStemControlGeneration(target) !== session.originalStemControlGeneration)
      || !ownsCurrentPreviewRoute(session, stateBeforeRestore, audio)) {
      if (retained) audio.discardRetainedDeckSource(retained);
      session.retainedSource = null;
      setPreviewMessage('Test Mix handed off: deck, source, or cue routing changed; the current state was left untouched.');
      return;
    }
    const expectedTransport = session.phase === 'loading' ? session.originalTransportControlGeneration : session.previewTransportControlGeneration;
    const expectedStem = session.phase === 'loading' ? session.originalStemControlGeneration : session.previewStemControlGeneration;
    const stillOwned = () => {
      const current = useStore.getState();
      const currentDeck = target === 'A' ? current.djDeckA : current.djDeckB;
      return (session.phase === 'loading'
        ? stillOwnsOccupiedPreviewBaseline(currentDeck, session.originalDeck)
        : session.occupied ? ownsOccupiedPreviewControls(session, currentDeck, audio)
          : currentDeck.track === session.candidateTrack && isPreparedPreviewDeck(currentDeck, session.candidateId, session.baseline))
        && ownsCurrentPreviewRoute(session, current, audio)
        && audio.getDeckLoadGeneration(target) === session.loadGeneration
        && audio.getDeckLoadedTrackId(target) === session.candidateId
        && expectedStem !== null && audio.getStemControlGeneration(target) === expectedStem
        && expectedTransport !== null && stillOwnsPreviewTransport(expectedTransport, audio.getDeckTransportControlGeneration(target));
    };
    const restored = audio.restoreRetainedDeckSource(retained, stillOwned, () => {
      audio.applyDeckMixSnapshot(target, session.originalDeck, session.originalKeyLock, session.originalStemState);
      const restoredDeck = copyDeckSnapshot(session.originalDeck);
      restoredDeck.duration = audio.getDuration(target);
      restoredDeck.position = retained.position;
      restoredDeck.isPlaying = retained.wasPlaying;
      useStore.getState().restoreDeckSnapshot(target, restoredDeck, session.originalKeyLock);
    });
    session.retainedSource = null;
    setPreviewMessage(restored ? message : 'Test Mix handed off during restore; user changes were kept.');
  };

  const finishPreview = (token: number, message: string) => {
    const session = previewRef.current;
    if (!session || session.token !== token) return;
    discardPreviewSession(session, !!session.retainedSource);
    const currentEngine = getDJAudioEngine();
    const state = useStore.getState();
    const currentDeck = session.targetDeck === 'A' ? state.djDeckA : state.djDeckB;
    const generationMatches = session.loadGeneration !== null
      ? currentEngine.getDeckLoadGeneration(session.targetDeck) === session.loadGeneration
      : (session.phase === 'preparing' || session.phase === 'loading') && currentEngine.getDeckLoadGeneration(session.targetDeck) === session.generationBeforeLoad;

    if (!generationMatches) {
      if (session.retainedSource) currentEngine.discardRetainedDeckSource(session.retainedSource);
      session.retainedSource = null;
      setPreviewMessage('Test Mix handed off: the preview deck was loaded or unloaded elsewhere.'); return;
    }
    if (session.phase === 'preparing') { setPreviewMessage(message); return; }
    if (!ownsCurrentPreviewRoute(session, state, currentEngine)) {
      if (session.retainedSource) currentEngine.discardRetainedDeckSource(session.retainedSource);
      session.retainedSource = null;
      setPreviewMessage('Test Mix handed off: cue routing changed, so the preview deck was left untouched.');
      return;
    }
    if (session.retainedSource) {
      discardPreviewSession(session, true);
      restoreOccupiedPreview(session, currentEngine, message);
      return;
    }
    if (session.phase === 'loading') { setPreviewMessage(message); return; }
    const baselineDeck = session.targetDeck === 'A' ? state.djDeckA : state.djDeckB;
    const ownsPrepared = isPreparedPreviewDeck(baselineDeck, session.candidateId, session.baseline);
    const ownsPlaying = stillOwnsPreviewDeck(baselineDeck, session.candidateId, session.baseline);
    const endedNaturally = session.phase === 'playing' && !currentEngine.isPlaying(session.targetDeck)
      && currentEngine.getPosition(session.targetDeck) >= Math.max(0, currentEngine.getDuration(session.targetDeck) - 0.15);
    if (!ownsPrepared || currentEngine.getDeckLoadedTrackId(session.targetDeck) !== session.candidateId
      || (session.phase === 'playing' && !ownsPlaying && !endedNaturally)) {
      setPreviewMessage('Test Mix handed off: the preview deck changed, so its state was left untouched.');
      return;
    }
    currentEngine.pause(session.targetDeck);
    currentEngine.setCueEnabled(session.targetDeck, false);
    currentEngine.unloadDeck(session.targetDeck);
    state.setDeckCue(session.targetDeck, false);
    state.unloadDeck(session.targetDeck);
    setPreviewMessage(message);
  };

  const finishPreviewRef = useRef(finishPreview);
  finishPreviewRef.current = finishPreview;

  useEffect(() => () => {
    const session = previewRef.current;
    if (session) finishPreviewRef.current(session.token, 'Test Mix stopped.');
  }, []);

  const testCandidate = async () => {
    if (!deck || !candidateTrack || !previewAllowed || previewRef.current) {
      setPreviewMessage(previewRef.current ? 'A Test Mix preview is already active.' : previewReason);
      return;
    }
    const state = useStore.getState();
    const target = previewDeckID;
    const targetState = target === 'A' ? state.djDeckA : state.djDeckB;
    const audio = getDJAudioEngine();
    const occupied = !!targetState.track;
    if ((!isPristineEmptyPreviewDeck(targetState) && !isRestorableOccupiedPreviewDeck(targetState, audio.getDeckLoadedTrackId(target)))
      || (occupied && (!audio.isLoaded(target) || targetState.isPlaying !== audio.isPlaying(target)))) {
      setPreviewMessage('Test Mix needs an empty deck or a loaded, uncued opposite deck with a verified source.'); return;
    }
    const originalDeck = copyDeckSnapshot(targetState);
    const originalKeyLock = target === 'A' ? state.djMixer.keyLockA : state.djMixer.keyLockB;
    const baseline: TestMixPreviewBaseline = { volume: targetState.volume, eq: { ...targetState.eq } };
    const startedAtHeadphoneDeviceId = audio.getHeadphoneOutputDeviceId();
    const startedAtMasterDeviceId = audio.getMainOutputDeviceId();
    const session: TestMixPreviewSession = {
      token: ++previewTokenRef.current, targetDeck: target, candidateId: candidateTrack.id, candidateTrack, baseline,
      occupied, originalDeck, originalPosition: occupied ? audio.getPosition(target) : targetState.position,
      originalKeyLock, originalStemMode: audio.getStemStatus(target).mode, originalStemState: audio.getStemState(target),
      previewStemMode: null, previewStemState: null,
      previewStemControlGeneration: null,
      originalTransportControlGeneration: audio.getDeckTransportControlGeneration(target), previewTransportControlGeneration: null,
      originalStemControlGeneration: audio.getStemControlGeneration(target),
      generationBeforeLoad: audio.getDeckLoadGeneration(target), loadGeneration: null, phase: 'preparing',
      preparedCandidate: null, retainedSource: null,
      startedAtCrossfader: state.djMixer.crossfader, startedAtKeyLock: originalKeyLock,
      startedAtHeadphoneDeviceId, startedAtMasterDeviceId,
    };
    previewRef.current = session;
    setPreviewMessage('Preparing off-air headphone preview…');
    try {
      const candidateFeatures = await api.getTrackEnergyFeatures(candidateTrack.id).catch(() => null);
      if (previewRef.current?.token !== session.token) return;
      const current = useStore.getState();
      const currentTarget = target === 'A' ? current.djDeckA : current.djDeckB;
      const mixerStillSafe = ownsCurrentPreviewRoute(session, current, audio);
      const deckStillOwned = occupied
        ? stillOwnsOccupiedPreviewBaseline(currentTarget, originalDeck) && audio.getDeckLoadedTrackId(target) === originalDeck.track?.id
          && stillOwnsPreviewTransport(session.originalTransportControlGeneration, audio.getDeckTransportControlGeneration(target))
          && audio.getStemControlGeneration(target) === session.originalStemControlGeneration
        : isPristineEmptyPreviewDeck(currentTarget) && currentTarget.volume === baseline.volume
          && currentTarget.eq.low === baseline.eq.low && currentTarget.eq.mid === baseline.eq.mid && currentTarget.eq.high === baseline.eq.high;
      if (!mixerStillSafe || !deckStillOwned || audio.getDeckLoadGeneration(target) !== session.generationBeforeLoad) {
        discardPreviewSession(session); setPreviewMessage('Test Mix stopped: deck or cue routing changed before loading.'); return;
      }

      session.phase = 'loading';
      const prepared = await audio.prepareTrack(target, candidateTrack);
      session.preparedCandidate = prepared;
      if (previewRef.current?.token !== session.token) { audio.discardPreparedTrack(prepared); return; }
      await audio.configurePreparedTrack(prepared, originalDeck, originalKeyLock, session.originalStemMode, session.originalStemState);
      if (previewRef.current?.token !== session.token) { audio.discardPreparedTrack(prepared); return; }
      const beforeCommit = useStore.getState();
      const beforeCommitDeck = target === 'A' ? beforeCommit.djDeckA : beforeCommit.djDeckB;
      const candidateCommitOwned = (occupied
        ? stillOwnsOccupiedPreviewBaseline(beforeCommitDeck, originalDeck)
          && audio.getDeckLoadedTrackId(target) === originalDeck.track?.id
          && stillOwnsPreviewTransport(session.originalTransportControlGeneration, audio.getDeckTransportControlGeneration(target))
          && audio.getStemControlGeneration(target) === session.originalStemControlGeneration
        : isPristineEmptyPreviewDeck(beforeCommitDeck) && beforeCommitDeck.volume === baseline.volume
          && beforeCommitDeck.eq.low === baseline.eq.low && beforeCommitDeck.eq.mid === baseline.eq.mid && beforeCommitDeck.eq.high === baseline.eq.high)
        && audio.getDeckLoadGeneration(target) === session.generationBeforeLoad
        && ownsCurrentPreviewRoute(session, beforeCommit, audio);
      const retained = audio.commitPreparedTrack(prepared, () => previewRef.current?.token === session.token && candidateCommitOwned, {
        position: audio.getPosition(target), wasPlaying: audio.isPlaying(target),
      });
      session.preparedCandidate = null;
      if (!retained) {
        discardPreviewSession(session); setPreviewMessage('Test Mix stopped: deck or cue routing changed during load.'); return;
      }
      session.retainedSource = retained;
      session.originalPosition = retained.position;
      session.originalDeck.position = retained.position;
      session.originalDeck.isPlaying = retained.wasPlaying;
      session.loadGeneration = audio.getDeckLoadGeneration(target);
      const afterLoad = useStore.getState();
      const afterTarget = target === 'A' ? afterLoad.djDeckA : afterLoad.djDeckB;
      afterLoad.loadTrackToDeck(target, candidateTrack);
      afterLoad.setDeckDuration(target, audio.getDuration(target));
      session.previewStemMode = audio.getStemStatus(target).mode;
      session.previewStemState = audio.getStemState(target);
      session.previewStemControlGeneration = audio.getStemControlGeneration(target);
      const mixIn = candidateFeatures?.cueSuggestions.find(cue => cue.kind === 'mix-in');
      const startAt = Math.max(0, mixIn?.position ?? 0);
      audio.setCueEnabled(target, true);
      afterLoad.setDeckCue(target, true);
      audio.seek(target, startAt);
      afterLoad.setDeckPosition(target, startAt);
      session.phase = 'ready';
      const previewPlay = audio.play(target);
      session.previewTransportControlGeneration = audio.getDeckTransportControlGeneration(target);
      await previewPlay;
      if (previewRef.current?.token !== session.token || audio.getDeckLoadGeneration(target) !== session.loadGeneration) return;
      if (occupied && audio.getDeckTransportControlGeneration(target) !== session.previewTransportControlGeneration) {
        finishPreviewRef.current(session.token, 'Test Mix handed off: transport controls changed.'); return;
      }
      if (audio.getDeckLoadedTrackId(target) !== candidateTrack.id) {
        finishPreviewRef.current(session.token, 'Test Mix handed off: the preview source changed.'); return;
      }
      const started = useStore.getState();
      const startedDeck = target === 'A' ? started.djDeckA : started.djDeckB;
      if (occupied ? !ownsOccupiedPreviewControls(session, startedDeck, audio)
        : startedDeck.track !== candidateTrack || !isPreparedPreviewDeck(startedDeck, candidateTrack.id, baseline)) {
        finishPreviewRef.current(session.token, 'Test Mix stopped because the preview deck changed.'); return;
      }
      started.setDeckPlaying(target, true);
      session.phase = 'playing';
      session.interval = window.setInterval(() => {
        if (previewRef.current?.token !== session.token) return;
        const currentState = useStore.getState();
        const currentTarget = target === 'A' ? currentState.djDeckA : currentState.djDeckB;
        const mixerChanged = !ownsCurrentPreviewRoute(session, currentState, audio);
        if (!mixerChanged && audio.getDeckLoadGeneration(target) === session.loadGeneration
          && currentTarget.track === candidateTrack && audio.getDeckLoadedTrackId(target) === candidateTrack.id && !audio.isPlaying(target)
          && audio.getPosition(target) >= Math.max(0, audio.getDuration(target) - 0.15)) {
          finishPreviewRef.current(session.token, 'Test Mix complete; the preview deck was restored.');
          return;
        }
        if (mixerChanged || audio.getDeckLoadGeneration(target) !== session.loadGeneration
          || currentTarget.track !== candidateTrack || audio.getDeckLoadedTrackId(target) !== candidateTrack.id
          || !stillOwnsPreviewDeck(currentTarget, candidateTrack.id, baseline)
          || (occupied && !ownsOccupiedPreviewControls(session, currentTarget, audio))) {
          discardPreviewSession(session);
          setPreviewMessage('Test Mix handed off: the preview deck changed, so its state was left untouched.');
          return;
        }
      }, 100);
      session.timeout = window.setTimeout(() => finishPreviewRef.current(session.token, 'Test Mix complete; the preview deck was restored.'), 10000);
      setPreviewMessage(`Testing ${candidateTrack.title} in headphones…`);
    } catch {
      finishPreviewRef.current(session.token, 'Test Mix failed and the preview deck was restored when still owned.');
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
      <label className="inline-flex items-center gap-1" title="Optionally limit candidates to one saved playlist">
        <span>Playlist</span>
        <select aria-label="Mix Next playlist" value={playlistId} onChange={event => setPlaylistId(event.target.value)}
          className="max-w-32 rounded border border-neutral-700 bg-neutral-950 px-1 text-neutral-200">
          <option value="">All playlists</option>
          {libraryPlaylists.map(playlist => <option key={playlist.id} value={playlist.id}>{playlist.name}</option>)}
        </select>
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
        {previewRef.current ? <button type="button" onClick={() => finishPreview(previewRef.current!.token, 'Test Mix stopped; the preview deck was restored.')}
          className="rounded border border-amber-500/40 px-2 py-1 text-amber-200">Stop Test Mix</button>
          : <button type="button" disabled={!previewAllowed || !!previewRef.current} onClick={() => void testCandidate()}
            title={previewRef.current ? 'Another Test Mix preview is active.' : previewDeckOccupied ? 'The off-air deck’s track, transport, mixer, FX, and stem controls will be restored after the audition if they remain under Test Mix ownership.' : previewReason}
            className="rounded border border-violet-500/40 px-2 py-1 text-violet-200 disabled:cursor-not-allowed disabled:opacity-50">Test Mix in headphones</button>}
        {previewMessage && <span role="status">{previewMessage}</span>}
      </div>
      <ul className="mt-1 space-y-0.5 pl-3">
        {top.filterEvidence.lastPlayed !== undefined && <li>Last completed play: {top.filterEvidence.lastPlayed ? new Date(top.filterEvidence.lastPlayed).toLocaleString() : 'never recorded'}</li>}
        {top.components.map(component => <li key={component.name} title={component.rationale}>
          {component.name}: {Math.round(component.score * 100)}% — {component.rationale}
        </li>)}
      </ul>
    </details>}
    {recommendations && recommendations.candidatesAfterFilters === 0 && <p role="status" className="mt-1 text-neutral-500">
      No analyzed candidates match these filters ({recommendations.candidatesBeforeFilters} checked).
    </p>}
  </section>;
}
