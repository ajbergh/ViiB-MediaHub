import { afterEach, expect, it, vi } from 'vitest';
import { loadImportedAnalysis } from './importedAnalysis';
afterEach(()=>vi.unstubAllGlobals());
it('preserves zero confidence and rejects malformed intervals and page identity',async()=>{
 const page={songId:'song',sourceFingerprint:'fp',recordingId:'recording',kind:'beats',provenance:'spotify_durable_import',retrievedAt:'2026-10-01T00:00:00Z',stale:false,offset:0,limit:25,totalItems:1,items:[{start:0,duration:1,confidence:0}]};
 const fetch=vi.fn();vi.stubGlobal('fetch',fetch);
 fetch.mockResolvedValue({ok:true,status:200,json:async()=>page});
 expect((await loadImportedAnalysis('song','beats',0))?.items[0].confidence).toBe(0);
 fetch.mockResolvedValue({ok:true,status:200,json:async()=>({...page,provenance:'spotify_private_cache',unverified:true})});
 expect((await loadImportedAnalysis('song','beats',0))?.unverified).toBe(true);
 fetch.mockResolvedValue({ok:true,status:200,json:async()=>({...page,items:[null]})});
 await expect(loadImportedAnalysis('song','beats',0)).rejects.toThrow();
 fetch.mockResolvedValue({ok:true,status:200,json:async()=>({...page,offset:25})});
 await expect(loadImportedAnalysis('song','beats',0)).rejects.toThrow();
 fetch.mockResolvedValue({ok:false,status:404});expect(await loadImportedAnalysis('song','beats',0)).toBeNull();
});
