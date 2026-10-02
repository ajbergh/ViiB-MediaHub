import {expect,it,vi} from 'vitest';
vi.mock('./httpClient',()=>({requestJSON:vi.fn().mockResolvedValue({})}));
import {requestJSON} from './httpClient';
import {libraryOperationsV2} from './libraryOperationsV2';
it('sends exactly the IDs confirmed in the repair preview',async()=>{
 await libraryOperationsV2.repair(true,undefined,['previewed']);
 const [,request]=vi.mocked(requestJSON).mock.calls[0];
 expect(JSON.parse(request!.body as string)).toEqual({removeMissing:true,confirmedSongIds:['previewed']});
});
