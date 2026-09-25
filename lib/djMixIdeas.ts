import type {
  TransitionComponent,
  TransitionIntent,
  TransitionRecommendationFilters,
  TransitionVector,
} from '../services/api';

export const DJ_MIX_IDEAS_STORAGE_KEY = 'viib.dj.mix-ideas.v1';
export const DJ_MIX_IDEAS_SCHEMA_VERSION = 1;
export const DJ_MIX_IDEAS_LIMIT = 50;
export const DJ_MIX_IDEAS_MAX_STORAGE_CHARS = 1_000_000;

export interface MixIdeaTrackSnapshot {
  trackId: string;
  title: string;
  artist: string;
}

export interface SavedDJMixIdea {
  schemaVersion: typeof DJ_MIX_IDEAS_SCHEMA_VERSION;
  createdAt: string;
  source: MixIdeaTrackSnapshot;
  candidate: MixIdeaTrackSnapshot;
  intent: TransitionIntent;
  algorithmVersion: string;
  score: number;
  vector: TransitionVector;
  components: TransitionComponent[];
  filters: TransitionRecommendationFilters;
  filterEvidence: {
    bpm?: number;
    energyLevel?: number;
    stemsAvailable?: boolean;
    lastPlayed?: number;
  };
}

export type NewDJMixIdea = Omit<SavedDJMixIdea, 'schemaVersion' | 'createdAt'>;

export interface MixIdeaStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
}

const intents = new Set<TransitionIntent>(['hold', 'lift', 'reset', 'harmonic']);

function boundedText(value: unknown, maxLength: number): string | null {
  if (typeof value !== 'string') return null;
  const text = value.trim();
  return text.length > 0 && text.length <= maxLength ? text : null;
}

function finite(value: unknown, min = -10000, max = 10000): number | undefined {
  return typeof value === 'number' && Number.isFinite(value) && value >= min && value <= max ? value : undefined;
}

function record(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown>
    : null;
}

function readTrack(value: unknown): MixIdeaTrackSnapshot | null {
  const item = record(value);
  if (!item) return null;
  const trackId = boundedText(item.trackId, 256);
  const title = boundedText(item.title, 512);
  const artist = boundedText(item.artist, 512);
  return trackId && title && artist ? { trackId, title, artist } : null;
}

function readFilters(value: unknown): TransitionRecommendationFilters | null {
  const item = record(value);
  if (!item) return null;
  const output: TransitionRecommendationFilters = {};
  for (const key of ['minBpm', 'maxBpm'] as const) {
    const number = finite(item[key], 0, 400);
    if (number !== undefined) output[key] = number;
    else if (item[key] !== undefined) return null;
  }
  for (const key of ['minEnergyLevel', 'maxEnergyLevel'] as const) {
    const number = finite(item[key], 1, 10);
    if (number !== undefined) output[key] = number;
    else if (item[key] !== undefined) return null;
  }
  for (const key of ['stemsAvailable', 'camelotCompatible'] as const) {
    if (item[key] !== undefined) {
      if (typeof item[key] !== 'boolean') return null;
      output[key] = item[key];
    }
  }
  if (item.genre !== undefined) {
    const genre = boundedText(item.genre, 128);
    if (!genre) return null;
    output.genre = genre;
  }
  if (item.notRecentlyPlayedHours !== undefined) {
    const hours = finite(item.notRecentlyPlayedHours, 1, 168);
    if (hours === undefined) return null;
    output.notRecentlyPlayedHours = hours;
  }
  const playlistIds = item.playlistIds ?? (item.playlistId ? [item.playlistId] : undefined);
  if (playlistIds !== undefined) {
    if (!Array.isArray(playlistIds) || playlistIds.length > 20
      || !playlistIds.every(id => typeof id === 'string' && id.length > 0 && id.length <= 256)) return null;
    output.playlistIds = [...new Set(playlistIds as string[])];
  }
  return output;
}

function readVector(value: unknown): TransitionVector | null {
  const item = record(value);
  if (!item) return null;
  const required = ['outgoingTailEnergy', 'incomingHeadEnergy', 'energyDelta', 'loudnessDeltaLu', 'outgoingMixOutConfidence', 'incomingMixInConfidence'] as const;
  const output: Record<string, number> = {};
  for (const key of required) {
    const number = finite(item[key]);
    if (number === undefined) return null;
    output[key] = number;
  }
  for (const key of ['bpmDelta', 'requiredTempoShiftPercent', 'energyLevelDelta'] as const) {
    const number = finite(item[key]);
    if (number !== undefined) output[key] = number;
    else if (item[key] !== undefined) return null;
  }
  if (item.camelotRelation !== undefined) {
    const relation = boundedText(item.camelotRelation, 64);
    if (!relation) return null;
    return { ...output, camelotRelation: relation } as TransitionVector;
  }
  return output as unknown as TransitionVector;
}

function readComponents(value: unknown): TransitionComponent[] | null {
  if (!Array.isArray(value) || value.length > 16) return null;
  const result: TransitionComponent[] = [];
  for (const raw of value) {
    const item = record(raw);
    const name = boundedText(item?.name, 64);
    const rationale = boundedText(item?.rationale, 512);
    const score = finite(item?.score, 0, 1);
    const weight = finite(item?.weight, 0, 1);
    if (!name || !rationale || score === undefined || weight === undefined) return null;
    result.push({ name, rationale, score, weight });
  }
  return result;
}

function readEvidence(value: unknown): SavedDJMixIdea['filterEvidence'] | null {
  const item = record(value);
  if (!item) return null;
  const result: SavedDJMixIdea['filterEvidence'] = {};
  if (item.bpm !== undefined) {
    const bpm = finite(item.bpm, 0, 400);
    if (bpm === undefined) return null;
    result.bpm = bpm;
  }
  if (item.energyLevel !== undefined) {
    const energyLevel = finite(item.energyLevel, 1, 10);
    if (energyLevel === undefined) return null;
    result.energyLevel = energyLevel;
  }
  if (item.stemsAvailable !== undefined) {
    if (typeof item.stemsAvailable !== 'boolean') return null;
    result.stemsAvailable = item.stemsAvailable;
  }
  if (item.lastPlayed !== undefined && item.lastPlayed !== null) {
    const lastPlayed = finite(item.lastPlayed, 0, Number.MAX_SAFE_INTEGER);
    if (lastPlayed === undefined) return null;
    result.lastPlayed = lastPlayed;
  }
  return result;
}

export function sanitizeDJMixIdea(value: unknown): SavedDJMixIdea | null {
  const item = record(value);
  if (!item || item.schemaVersion !== DJ_MIX_IDEAS_SCHEMA_VERSION) return null;
  const source = readTrack(item.source);
  const candidate = readTrack(item.candidate);
  const intent = item.intent as TransitionIntent;
  const algorithmVersion = boundedText(item.algorithmVersion, 128);
  const createdAt = boundedText(item.createdAt, 64);
  const score = finite(item.score, 0, 1);
  const vector = readVector(item.vector);
  const components = readComponents(item.components);
  const filters = readFilters(item.filters);
  const filterEvidence = readEvidence(item.filterEvidence);
  if (!source || !candidate || source.trackId === candidate.trackId || !intents.has(intent)
    || !algorithmVersion || !createdAt || !Number.isFinite(Date.parse(createdAt)) || score === undefined
    || !vector || !components || !filters || !filterEvidence) return null;
  return { schemaVersion: DJ_MIX_IDEAS_SCHEMA_VERSION, createdAt, source, candidate, intent,
    algorithmVersion, score, vector, components, filters, filterEvidence };
}

export function listDJMixIdeas(storage: MixIdeaStorage): SavedDJMixIdea[] {
  try {
    const raw = storage.getItem(DJ_MIX_IDEAS_STORAGE_KEY);
    if (!raw || raw.length > DJ_MIX_IDEAS_MAX_STORAGE_CHARS) return [];
    const parsed: unknown = JSON.parse(raw);
    const root = record(parsed);
    if (!root || root.schemaVersion !== DJ_MIX_IDEAS_SCHEMA_VERSION || !Array.isArray(root.ideas)) return [];
    return root.ideas.slice(0, DJ_MIX_IDEAS_LIMIT).map(sanitizeDJMixIdea).filter((idea): idea is SavedDJMixIdea => !!idea);
  } catch {
    return [];
  }
}

function writeDJMixIdeas(storage: MixIdeaStorage, ideas: SavedDJMixIdea[]): void {
  const serialized = JSON.stringify({ schemaVersion: DJ_MIX_IDEAS_SCHEMA_VERSION, ideas: ideas.slice(0, DJ_MIX_IDEAS_LIMIT) });
  if (serialized.length > DJ_MIX_IDEAS_MAX_STORAGE_CHARS) throw new RangeError('Saved mix ideas exceed the storage limit');
  storage.setItem(DJ_MIX_IDEAS_STORAGE_KEY, serialized);
}

function ideaKey(idea: Pick<SavedDJMixIdea, 'source' | 'candidate' | 'intent'>): string {
  return `${idea.source.trackId}\0${idea.candidate.trackId}\0${idea.intent}`;
}

export function saveDJMixIdea(storage: MixIdeaStorage, value: NewDJMixIdea, now = Date.now()): SavedDJMixIdea[] {
  const idea = sanitizeDJMixIdea({ ...value, schemaVersion: DJ_MIX_IDEAS_SCHEMA_VERSION, createdAt: new Date(now).toISOString() });
  if (!idea) return listDJMixIdeas(storage);
  const previous = listDJMixIdeas(storage).filter(item => ideaKey(item) !== ideaKey(idea));
  const next = [idea, ...previous].slice(0, DJ_MIX_IDEAS_LIMIT);
  writeDJMixIdeas(storage, next);
  return next;
}

export function deleteDJMixIdea(storage: MixIdeaStorage, key: string): SavedDJMixIdea[] {
  const next = listDJMixIdeas(storage).filter(item => ideaKey(item) !== key);
  writeDJMixIdeas(storage, next);
  return next;
}

export function djMixIdeaKey(idea: Pick<SavedDJMixIdea, 'source' | 'candidate' | 'intent'>): string {
  return ideaKey(idea);
}
