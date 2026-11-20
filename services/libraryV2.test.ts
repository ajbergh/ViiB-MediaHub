/** Tests and fixtures for library V2 behavior. */

import {afterEach,expect,it,vi} from 'vitest';
vi.mock('./backendService',()=>({apiSongToSong:(song:unknown)=>song}));
import {libraryV2,LibraryResnapshotRequired} from './libraryV2';
afterEach(()=>vi.unstubAllGlobals());
const setup=()=>vi.stubGlobal('window',{setTimeout,clearTimeout});
it('exposes a typed expired-cursor signal for snapshot recovery',async()=>{
 setup();vi.stubGlobal('fetch',vi.fn().mockResolvedValue({status:410,ok:false}));
 await expect(libraryV2.getChanges(1)).rejects.toBeInstanceOf(LibraryResnapshotRequired);
});
it('does not retain an earlier page payload after a later invisible upsert',async()=>{
 setup();const song={id:'song',title:'Song'};
 vi.stubGlobal('fetch',vi.fn().mockResolvedValueOnce({status:200,ok:true,json:async()=>({toRevision:1,hasMore:true,changes:[{revision:1,songId:'song',operation:'upsert'}],songs:[song]})}).mockResolvedValueOnce({status:200,ok:true,json:async()=>({toRevision:2,hasMore:false,changes:[{revision:2,songId:'song',operation:'upsert'}],songs:[]})}));
 const result=await libraryV2.getChanges(0);expect(result.revision).toBe(2);expect(result.songs).toEqual([]);
});

it('previews the first page while keeping its revision for catch-up after later pages', async () => {
 setup();
 const first = { id: 'first', title: 'First' };
 const second = { id: 'second', title: 'Second' };
 let resolvePage!: (page: unknown) => void;
 const laterPage = new Promise(resolve => { resolvePage = resolve; });
 vi.stubGlobal('fetch', vi.fn()
   .mockResolvedValueOnce({ ok: true, json: async () => ({ revision: 4, songs: [first], hasMore: true, nextCursor: 'first' }) })
   .mockResolvedValueOnce({ ok: true, json: () => laterPage }));
 const preview = vi.fn();
 const result = libraryV2.getSnapshot(undefined, preview);
 await vi.waitFor(() => expect(preview).toHaveBeenCalledWith([first]));
 resolvePage({ revision: 5, songs: [second], hasMore: false });
 await expect(result).resolves.toEqual({ revision: 4, songs: [first, second] });
 expect(preview).toHaveBeenCalledTimes(1);
});
