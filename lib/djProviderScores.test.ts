import { expect,it } from 'vitest';
import { usableProviderScore,providerScoreMatches,compareProviderScores } from './djProviderScores';
it('preserves zero and excludes stale or unverified scores',()=>{
 const feature:any={providerScores:{spotify_energy_score:{value:0,stale:false,expiresAt:'2100-01-01T00:00:00Z'}}};
 expect(usableProviderScore(feature,'energy')).toBe(0);
 feature.providerScoresUnverified=true; expect(usableProviderScore(feature,'energy')).toBeUndefined();
 feature.providerScoresUnverified=false; feature.providerScores.spotify_energy_score.stale=true; expect(usableProviderScore(feature,'energy')).toBeUndefined();
});
it('filters inclusively and sorts unknown last in both directions',()=>{
 expect(providerScoreMatches(0,'0','0')).toBe(true);
 expect(providerScoreMatches(undefined,'','')).toBe(true);
 expect(providerScoreMatches(undefined,'0','1')).toBe(false);
 expect(providerScoreMatches(0.5,'0.8','0.2')).toBe(false);
 expect(compareProviderScores(undefined,0,'desc')).toBe(1);
 expect(compareProviderScores(undefined,0,'asc')).toBe(1);
 expect(compareProviderScores(0,1,'desc')).toBe(1);
});

it('expires an initially fresh score at its deadline and rejects missing expiry',()=>{
 const deadline=Date.parse('2026-10-05T12:00:00Z');
 const feature:any={providerScores:{spotify_energy_score:{value:0,stale:false,expiresAt:new Date(deadline).toISOString()}}};
 expect(usableProviderScore(feature,'energy',deadline-1)).toBe(0);
 expect(usableProviderScore(feature,'energy',deadline)).toBeUndefined();
 delete feature.providerScores.spotify_energy_score.expiresAt;
 expect(usableProviderScore(feature,'energy',deadline-1)).toBeUndefined();
});
