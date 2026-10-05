/** Tests and fixtures for metadata behavior. */

import {afterEach,expect,it,vi} from 'vitest';
import {managedObjectUrls} from './lib/playbackLifecycle';
import {parseSong} from './metadata';
afterEach(()=>{managedObjectUrls.releaseAll();vi.restoreAllMocks();vi.unstubAllGlobals()});
it('registers actual browser-parser URLs and releases the worker bootstrap URL',async()=>{
 let index=0;vi.spyOn(URL,'createObjectURL').mockImplementation(()=>`blob:parser-${++index}`);
 const revoke=vi.spyOn(URL,'revokeObjectURL').mockImplementation(()=>{});
 class WorkerFixture {onmessage:any;onerror:any;postMessage(payload:any){queueMicrotask(()=>this.onmessage({data:{id:payload.id,metadata:{title:'Song',artist:'Artist',album:'Album',duration:1,coverData:new Blob(['art'])}}}))}}
 vi.stubGlobal('Worker',WorkerFixture);
 const parsed=await parseSong(new File(['audio'],'song.mp3'),new Map());
 expect(managedObjectUrls.owns(parsed.url)).toBe(true);expect(managedObjectUrls.owns(parsed.coverUrl)).toBe(true);
 expect(revoke).toHaveBeenCalledWith('blob:parser-1');managedObjectUrls.release(parsed.url);expect(revoke).toHaveBeenCalledWith(parsed.url);
});
