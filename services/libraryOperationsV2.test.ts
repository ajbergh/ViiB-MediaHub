/** Tests and fixtures for library Operations V2 behavior. */

import {expect,it,vi} from 'vitest';
vi.mock('./httpClient', async importOriginal => ({
  ...await importOriginal<typeof import('./httpClient')>(),
  requestJSON: vi.fn().mockResolvedValue({}),
}));
import {APIError,requestJSON} from './httpClient';
import {libraryOperationsV2} from './libraryOperationsV2';
it('sends exactly the IDs confirmed in the repair preview',async()=>{
 await libraryOperationsV2.repair(true,undefined,['previewed']);
 const [,request]=vi.mocked(requestJSON).mock.calls[0];
 expect(JSON.parse(request!.body as string)).toEqual({removeMissing:true,confirmedSongIds:['previewed']});
});

it('explains when the backend does not have the bulk album route', async () => {
  vi.mocked(requestJSON).mockRejectedValueOnce(new APIError(404, {
    code: 'http_404', message: 'Not Found', retryable: false,
  }));
  await expect(libraryOperationsV2.updateAlbumMetadata(['track-a'], { artist: 'Michael Jackson' }))
    .rejects.toThrow('The running ViiB backend does not support bulk album edits.');
});

it('preserves the backend explanation when a track is missing', async () => {
  const error = new APIError(404, {
    code: 'song_not_found', message: 'A track is no longer in the library.', retryable: false,
  });
  vi.mocked(requestJSON).mockRejectedValueOnce(error);
  await expect(libraryOperationsV2.updateAlbumMetadata(['track-a'], { artist: 'Michael Jackson' })).rejects.toBe(error);
});
