/** Defines analysis, beat-grid, energy, cue, and recommendation DTOs with payload normalization. */

import type { DJHotCue } from './api';

// Resolved server-side analysis for DJ display and timing. `bpm` deliberately
// excludes legacy AI estimates, and `syncAllowed` is true only for manual or
// audio-measured tempo. Spotify scalar tempo is displayable but supplies no beat phase.
export interface SpotifyScalarField {
  key: string;
  metric: string;
  units: string;
  value: number | { tonic: number; mode: number; modeConfidence?: number };
  confidence?: number;
  endpoint: 'audio_features' | 'audio_analysis';
  schemaVersion: number;
  adapterRevision: string;
  retrievedAt: string;
  expiresAt: string;
  stale: boolean;
  durableImport?: boolean;
}

/** Source-bound candidates; stale provider evidence is inspection-only here. */
export interface EffectiveScalarCandidate extends Omit<SpotifyScalarField, 'endpoint'> {
  endpoint: 'audio_features' | 'audio_analysis' | '';
  source: 'manual' | 'local' | 'spotify_private' | 'spotify_download_import';
  sourceFingerprint: string;
  locked?: boolean;
}

export interface EffectiveScalar {
  key: string;
  state: 'available' | 'unknown';
  selected?: EffectiveScalarCandidate;
  lastGood?: EffectiveScalarCandidate;
}

export interface TrackAnalysisFeature {
  effectiveFields?: EffectiveScalar[];
  providerScores?: Record<string, { value: number; stale: boolean; retrievedAt: string; expiresAt: string; endpoint: 'audio_features' | 'audio_analysis' }>;
  providerScoresUnverified?: boolean;
  providerScalars?: {
    readOnly?: boolean;
    unverified?: boolean;
    provenance?: 'spotify_private_cache' | 'spotify_download_import' | 'spotify_mixed_private_and_download_import';
    recordingId: string;
    sourceFingerprint: string;
    fields: SpotifyScalarField[];
    attempts: Array<{
      key: string;
      endpoint: 'audio_features' | 'audio_analysis';
      state: 'available' | 'not_returned' | 'invalid_field';
      reason?: string;
      checkedAt: string;
      adapterRevision: string;
      durableImport?: boolean;
    }>;
    selected: SpotifyScalarField[];
    selectionPolicy: 'fresh_then_newest_v1';
  };
  songId: string;
  status: 'pending' | 'running' | 'complete' | 'partial' | 'failed' | 'unsupported';
  analyzedAt?: number;
  bpm?: number;
  bpmConfidence?: number;
  measuredBpm?: number;
  measuredBpmConfidence?: number;
  measuredKeyConfidence?: number;
  localAlgorithmVersion?: string;
  bpmAltCandidate?: number;
  tempoStability?: number;
  tempoKind?: string;
  bpmSource: 'unknown' | 'manual' | 'measured' | 'spotify';
  syncAllowed: boolean;
  key?: string;
  camelotKey?: string;
  openKey?: string;
  keyTonic?: number;
  keyMode?: 'major' | 'minor';
  measuredKeyTonic?: number;
  measuredKeyMode?: 'major' | 'minor';
  keyConfidence?: number;
  keySource: 'unknown' | 'manual' | 'measured' | 'spotify';
  measuredEnergyLevel?: number;
  energyLevelSource?: string;
  energyLevel?: number;
  energyLevelConfidence?: number;
  energyAlgorithmVersion?: string;
  structureAvailable?: boolean;
  /** Current measured ITU-R BS.1770-5 integrated loudness, if available. */
  integratedLufsBs1770?: number;
  /** Current measured ITU-R BS.1770-5 Annex 2 true peak in dBTP, if available. */
  truePeakDbtp?: number;
  /** Opaque fingerprint for the source revision currently available to BPM edits. */
  sourceFingerprint?: string;
}

// Persisted phase-aligned timing data. It is distinct from the scalar BPM so
// deck Sync can use measured beat positions without recreating a zero-offset
// browser grid.
export interface TrackBeatGrid {
  sourceFingerprint?: string;
  resolution?: 'available' | 'unavailable';
  reason?: string;
  songId: string;
  beats: number[];
  downbeatIndices: number[];
  locked: boolean;
  algorithmVersion: string;
  /** Legacy alias retained for clients that predate explicit provenance. */
  source?: 'unknown' | 'measured' | 'inferred-from-meter' | 'manual';
  provenance?: 'unknown' | 'measured' | 'inferred-from-meter' | 'manual';
}

export interface TrackBeatGridUpdate {
  bpm?: number;
  beats: number[];
  downbeatIndices: number[];
  locked: boolean;
}

export interface EnergyPoint { time: number; value: number; }
export interface EnergySection {
  start: number;
  end: number;
  energy: number;
  label?: 'intro' | 'build' | 'drop' | 'breakdown' | 'outro' | 'unknown';
  confidence?: number;
  timingProvenance?: 'energy-windows' | 'energy-windows+downbeat-grid';
  downbeatStart?: number;
  downbeatEnd?: number;
}
export interface CueSuggestion { position: number; kind: 'mix-in' | 'mix-out' | 'section'; confidence: number; rationale: string; }
export interface TrackEnergyFeatures {
  songId: string;
  integratedLufs: number;
  truePeakDbfs: number;
  integratedLufsBs1770?: number;
  truePeakDbtp?: number;
  loudnessStandard?: string;
  loudnessAlgorithmVersion?: string;
  truePeakAlgorithmVersion?: string;
  loudnessChannelLayout?: string;
  loudnessChannelWeighting?: string;
  loudnessStatus?: string;
  truePeakStatus?: string;
  energy: EnergyPoint[];
  sections: EnergySection[];
  cueSuggestions: CueSuggestion[];
  algorithmVersion: string;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function finiteNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value);
}

export function normalizeTrackEnergyFeatures(value: TrackEnergyFeatures | unknown): TrackEnergyFeatures {
  const item = isRecord(value) ? value : {};
  const energy = Array.isArray(item.energy) ? item.energy.filter((point): point is EnergyPoint =>
    isRecord(point) && finiteNumber(point.time) && finiteNumber(point.value)) : [];
  const sections = Array.isArray(item.sections) ? item.sections.filter((section): section is EnergySection =>
    isRecord(section) && finiteNumber(section.start) && finiteNumber(section.end) && finiteNumber(section.energy)) : [];
  const cueSuggestions = Array.isArray(item.cueSuggestions) ? item.cueSuggestions.filter((cue): cue is CueSuggestion =>
    isRecord(cue) && finiteNumber(cue.position) && typeof cue.kind === 'string' && finiteNumber(cue.confidence)
      && typeof cue.rationale === 'string') : [];
  return {
    ...(item as unknown as TrackEnergyFeatures),
    songId: typeof item.songId === 'string' ? item.songId : '',
    integratedLufs: finiteNumber(item.integratedLufs) ? item.integratedLufs : Number.NaN,
    truePeakDbfs: finiteNumber(item.truePeakDbfs) ? item.truePeakDbfs : Number.NaN,
    algorithmVersion: typeof item.algorithmVersion === 'string' ? item.algorithmVersion : '',
    energy,
    sections,
    cueSuggestions,
  };
}

export type AnalysisCueApplyMode = 'fill-empty' | 'replace-generated' | 'selected-only';

export interface AnalysisCueCandidate extends DJHotCue {
  origin: 'analysis';
  generatorVersion: string;
  confidence: number;
  kind: string;
  rationale: string;
  sourceFingerprint: string;
  downbeatAligned: boolean;
}

export interface AnalysisCueSuppression {
  slot: number;
  kind: string;
}

export interface AnalysisCueList {
  songId: string;
  generatorVersion: string;
  sourceFingerprint: string;
  defaultApplyMode: AnalysisCueApplyMode;
  hotCues: DJHotCue[];
  generatedCandidates: AnalysisCueCandidate[];
  suppressions: AnalysisCueSuppression[];
}

export function normalizeAnalysisCueList(value: AnalysisCueList | unknown): AnalysisCueList {
  const item = isRecord(value) ? value : {};
  const hotCues = Array.isArray(item.hotCues) ? item.hotCues.filter((cue): cue is DJHotCue =>
    isRecord(cue) && Number.isInteger(cue.slot) && finiteNumber(cue.position) && typeof cue.color === 'string') : [];
  const generatedCandidates = Array.isArray(item.generatedCandidates) ? item.generatedCandidates.filter((cue): cue is AnalysisCueCandidate =>
    isRecord(cue) && Number.isInteger(cue.slot) && finiteNumber(cue.position) && typeof cue.color === 'string'
      && typeof cue.generatorVersion === 'string' && finiteNumber(cue.confidence) && typeof cue.kind === 'string'
      && typeof cue.rationale === 'string' && typeof cue.sourceFingerprint === 'string' && cue.origin === 'analysis') : [];
  const suppressions = Array.isArray(item.suppressions) ? item.suppressions.filter((suppression): suppression is AnalysisCueSuppression =>
    isRecord(suppression) && Number.isInteger(suppression.slot) && typeof suppression.kind === 'string') : [];
  return {
    ...(item as unknown as AnalysisCueList),
    songId: typeof item.songId === 'string' ? item.songId : '',
    generatorVersion: typeof item.generatorVersion === 'string' ? item.generatorVersion : 'unknown',
    sourceFingerprint: typeof item.sourceFingerprint === 'string' ? item.sourceFingerprint : '',
    defaultApplyMode: item.defaultApplyMode === 'replace-generated' || item.defaultApplyMode === 'selected-only'
      ? item.defaultApplyMode : 'fill-empty',
    hotCues,
    generatedCandidates,
    suppressions,
  };
}

export interface AnalysisCueApplyResult extends AnalysisCueList {
  appliedSlots: number[];
  blockedSlots: number[];
}

export interface TransitionVector {
  outgoingTailEnergy: number;
  incomingHeadEnergy: number;
  energyDelta: number;
  loudnessDeltaLu: number;
  outgoingMixOutConfidence: number;
  incomingMixInConfidence: number;
  bpmDelta?: number;
  requiredTempoShiftPercent?: number;
  camelotRelation?: string;
  energyLevelDelta?: number;
}

export type TransitionIntent = 'hold' | 'lift' | 'reset' | 'harmonic';

export interface TransitionComponent {
  name: string;
  score: number;
  weight: number;
  rationale: string;
}

export interface TransitionRecommendationFilters {
  spotifyScoreMetric?: string;
  minSpotifyScore?: number;
  maxSpotifyScore?: number;
  minBpm?: number;
  maxBpm?: number;
  minEnergyLevel?: number;
  maxEnergyLevel?: number;
  stemsAvailable?: boolean;
  camelotCompatible?: boolean;
  playlistId?: string;
  playlistIds?: string[];
  genre?: string;
  notRecentlyPlayedHours?: number;
}

export interface TransitionCandidateFilterEvidence {
  spotifyScoreMetric?: string;
  spotifyScore?: { value: number; retrievedAt: string; expiresAt: string; endpoint: string; stale: boolean };
  bpm?: number;
  energyLevel?: number;
  stemsAvailable?: boolean;
  lastPlayed?: number;
}

export interface TransitionRecommendation {
  songId: string;
  title: string;
  artist: string;
  score: number;
  intent: TransitionIntent;
  vector: TransitionVector;
  components: TransitionComponent[];
  filterEvidence: TransitionCandidateFilterEvidence;
}

export interface TrackTransitionRecommendations {
  songId: string;
  intent: TransitionIntent;
  algorithmVersion: string;
  filters: TransitionRecommendationFilters;
  candidatesBeforeFilters: number;
  candidatesAfterFilters: number;
  recommendations: TransitionRecommendation[];
}

export function normalizeTrackTransitionRecommendations(value: TrackTransitionRecommendations | unknown): TrackTransitionRecommendations {
  const item = isRecord(value) ? value : {};
  const recommendations = Array.isArray(item.recommendations) ? item.recommendations.flatMap((raw): TransitionRecommendation[] => {
    if (!isRecord(raw) || typeof raw.songId !== 'string' || typeof raw.title !== 'string' || typeof raw.artist !== 'string'
      || !finiteNumber(raw.score) || !isRecord(raw.vector)) return [];
    const vector = raw.vector;
    const components = Array.isArray(raw.components) ? raw.components.filter((component): component is TransitionComponent =>
      isRecord(component) && typeof component.name === 'string' && finiteNumber(component.score)
        && finiteNumber(component.weight) && typeof component.rationale === 'string') : [];
    const filterEvidence = isRecord(raw.filterEvidence) ? raw.filterEvidence as TransitionCandidateFilterEvidence : {};
    return [{
      ...(raw as unknown as TransitionRecommendation),
      intent: raw.intent === 'lift' || raw.intent === 'reset' || raw.intent === 'harmonic' ? raw.intent : 'hold',
      vector: vector as unknown as TransitionVector,
      components,
      filterEvidence,
    }];
  }) : [];
  return {
    ...(item as unknown as TrackTransitionRecommendations),
    songId: typeof item.songId === 'string' ? item.songId : '',
    intent: item.intent === 'lift' || item.intent === 'reset' || item.intent === 'harmonic' ? item.intent : 'hold',
    algorithmVersion: typeof item.algorithmVersion === 'string' ? item.algorithmVersion : 'unknown',
    filters: isRecord(item.filters) ? item.filters as TransitionRecommendationFilters : {},
    candidatesBeforeFilters: finiteNumber(item.candidatesBeforeFilters) ? item.candidatesBeforeFilters : recommendations.length,
    candidatesAfterFilters: finiteNumber(item.candidatesAfterFilters) ? item.candidatesAfterFilters : recommendations.length,
    recommendations,
  };
}
