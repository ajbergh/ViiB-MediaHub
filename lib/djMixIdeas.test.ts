import { describe, expect, it } from 'vitest';
import {
  deleteDJMixIdea,
  djMixIdeaKey,
  DJ_MIX_IDEAS_LIMIT,
  DJ_MIX_IDEAS_STORAGE_KEY,
  listDJMixIdeas,
  saveDJMixIdea,
  type MixIdeaStorage,
  type NewDJMixIdea,
} from './djMixIdeas';

class MemoryStorage implements MixIdeaStorage {
  values = new Map<string, string>();
  getItem(key: string) { return this.values.get(key) ?? null; }
  setItem(key: string, value: string) { this.values.set(key, value); }
}

function makeIdea(score = 0.82): NewDJMixIdea {
  return {
    source: { trackId: 'source-id', title: 'Source', artist: 'Source Artist' },
    candidate: { trackId: 'candidate-id', title: 'Candidate', artist: 'Candidate Artist' },
    intent: 'hold',
    algorithmVersion: 'transition-v2',
    score,
    vector: {
      outgoingTailEnergy: 0.7, incomingHeadEnergy: 0.72, energyDelta: 0.02, loudnessDeltaLu: -1,
      outgoingMixOutConfidence: 0.8, incomingMixInConfidence: 0.9, bpmDelta: 2,
      camelotRelation: 'same key', energyLevelDelta: 0,
    },
    components: [{ name: 'tempo', score: 0.9, weight: 0.3, rationale: 'Small tempo change' }],
    filters: { minBpm: 90, maxBpm: 130, playlistIds: ['playlist-a', 'playlist-b'], genre: 'House' },
    filterEvidence: { bpm: 122, energyLevel: 6, stemsAvailable: true, lastPlayed: 1700000000000 },
  };
}

describe('saved DJ Mix Next ideas', () => {
  it('saves, lists and deletes a bounded local snapshot', () => {
    const storage = new MemoryStorage();
    const saved = saveDJMixIdea(storage, makeIdea(), Date.UTC(2026, 8, 25));
    expect(saved).toHaveLength(1);
    expect(saved[0].createdAt).toBe('2026-09-25T00:00:00.000Z');
    expect(listDJMixIdeas(storage)).toEqual(saved);
    expect(deleteDJMixIdea(storage, djMixIdeaKey(saved[0]))).toEqual([]);
    expect(JSON.parse(storage.getItem(DJ_MIX_IDEAS_STORAGE_KEY)!).ideas).toEqual([]);
  });

  it('replaces a duplicate route snapshot and preserves the captured evidence', () => {
    const storage = new MemoryStorage();
    const live = makeIdea();
    saveDJMixIdea(storage, live, 1000);
    live.score = 0.1;
    live.vector.energyDelta = 4;
    live.components[0].rationale = 'Changed live recommendation';
    const loaded = listDJMixIdeas(storage);
    expect(loaded).toHaveLength(1);
    expect(loaded[0].score).toBe(0.82);
    expect(loaded[0].vector.energyDelta).toBe(0.02);
    expect(loaded[0].components[0].rationale).toBe('Small tempo change');

    saveDJMixIdea(storage, makeIdea(0.75), 2000);
    expect(listDJMixIdeas(storage)).toHaveLength(1);
    expect(listDJMixIdeas(storage)[0].score).toBe(0.75);
  });

  it('ignores malformed, unsupported-version and invalid ideas', () => {
    const storage = new MemoryStorage();
    storage.setItem(DJ_MIX_IDEAS_STORAGE_KEY, '{broken');
    expect(listDJMixIdeas(storage)).toEqual([]);
    storage.setItem(DJ_MIX_IDEAS_STORAGE_KEY, JSON.stringify({ schemaVersion: 999, ideas: [makeIdea()] }));
    expect(listDJMixIdeas(storage)).toEqual([]);
    storage.setItem(DJ_MIX_IDEAS_STORAGE_KEY, JSON.stringify({ schemaVersion: 1, ideas: [
      { ...makeIdea(), schemaVersion: 1, createdAt: new Date(1000).toISOString(), score: 7 },
      { ...makeIdea(), schemaVersion: 1, createdAt: new Date(1000).toISOString() },
    ] }));
    expect(listDJMixIdeas(storage)).toHaveLength(1);
  });

  it('keeps storage within the configured idea limit', () => {
    const storage = new MemoryStorage();
    for (let index = 0; index < DJ_MIX_IDEAS_LIMIT + 5; index++) {
      const idea = makeIdea();
      idea.source = { trackId: `source-${index}`, title: `Source ${index}`, artist: 'Artist' };
      idea.candidate = { trackId: `candidate-${index}`, title: `Candidate ${index}`, artist: 'Artist' };
      saveDJMixIdea(storage, idea, 1000 + index);
    }
    expect(listDJMixIdeas(storage)).toHaveLength(DJ_MIX_IDEAS_LIMIT);
    expect(listDJMixIdeas(storage)[0].source.trackId).toBe(`source-${DJ_MIX_IDEAS_LIMIT + 4}`);
  });
});
