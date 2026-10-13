import type { TrackAnalysisFeature } from '../services/trackAnalysisContracts';
export const PROVIDER_SCORE_OPTIONS = ['energy','danceability','acousticness','instrumentalness','liveness','speechiness','valence'] as const;
export function usableProviderScore(feature: TrackAnalysisFeature | undefined, metric: string, now = Date.now()): number | undefined {
 const score = feature?.providerScores?.[`spotify_${metric}_score`];
 const expiry = Date.parse(score?.expiresAt ?? '');
 if (!Number.isFinite(expiry) || expiry <= now) return undefined;
 if (feature?.providerScoresUnverified || !score || score.stale || !Number.isFinite(score.value) || score.value < 0 || score.value > 1) return undefined;
 return score.value;
}
export function providerScoreMatches(value: number | undefined, min: string, max: string): boolean {
 if (min === '' && max === '') return true;
 const low = min === '' ? 0 : Number(min), high = max === '' ? 1 : Number(max);
 return value !== undefined && Number.isFinite(low) && Number.isFinite(high) && low >= 0 && high <= 1 && low <= high && value >= low && value <= high;
}
export function compareProviderScores(a: number | undefined,b: number | undefined,direction: 'asc' | 'desc'): number {
 if (a === undefined || b === undefined) return a === b ? 0 : a === undefined ? 1 : -1;
 return direction === 'asc' ? a-b : b-a;
}
