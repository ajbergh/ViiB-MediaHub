import {afterEach,expect,it,vi} from 'vitest';
const mocks=vi.hoisted(()=>({put:vi.fn().mockResolvedValue(undefined)}));
vi.mock('./db',()=>({getDB:async()=>({transaction:()=>({store:{put:mocks.put},done:Promise.resolve()})}),closeDB:vi.fn()}));
import {libraryService} from './libraryService';
afterEach(()=>vi.clearAllMocks());
it('stores durable song data while dropping ephemeral audio and artwork URLs',async()=>{
 const song={id:'song',title:'Song',artist:'Artist',album:'Album',duration:120,url:'blob:audio',coverUrl:'blob:cover',coverData:new Blob(['art']),addedAt:1};
 await libraryService.saveSongs([song]);const durable=mocks.put.mock.calls[0][0];
 expect(durable.url).toBe('');expect(durable.coverUrl).toBeUndefined();expect(durable.coverData).toBe(song.coverData);expect(song.url).toBe('blob:audio');
});
