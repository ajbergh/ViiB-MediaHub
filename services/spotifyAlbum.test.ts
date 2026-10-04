import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {fetchSpotifyAlbum} from './spotifyAlbum';
import {useStore} from '../store';
const id='A'.repeat(22);
beforeEach(()=>{useStore.getState().logoutSpotify();useStore.getState().setSpotifyConnected(true)});
afterEach(()=>vi.unstubAllGlobals());
it('collects all pages and removes unavailable rows without losing duplicate occurrences',async()=>{
 const first={offset:0,total:4,items:[{id:'track1'},null],next:'https://api.spotify.com/v1/albums/'+id+'/tracks?limit=2&offset=2'};
 const mock=vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({id,tracks:first}))).mockResolvedValueOnce(new Response(JSON.stringify({offset:2,total:4,items:[{id:'track2'},{id:'track1'}],next:null})));
 vi.stubGlobal('fetch',mock);const album=await fetchSpotifyAlbum(id);expect(album.tracks.items.map((t:any)=>t.id)).toEqual(['track1','track2','track1']);expect(mock.mock.calls[1][0]).toContain('offset=2');expect(album.tracks.next).toBeNull();
});
it.each(['https://untrusted.invalid/v1/albums/'+id+'/tracks?offset=2','https://api.spotify.com/v1/albums/'+id+'/tracks?offset=0'])('rejects unsafe or nonadvancing pages %s',async(next)=>{
 const mock=vi.fn().mockResolvedValue(new Response(JSON.stringify({tracks:{offset:0,total:2,items:[{id:'track'}],next}})));vi.stubGlobal('fetch',mock);await expect(fetchSpotifyAlbum(id)).rejects.toThrow('pagination');expect(mock).toHaveBeenCalledTimes(1);
});
it('rejects a page completed after account replacement',async()=>{
 const mock=vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({tracks:{offset:0,total:2,items:[{id:'old'}],next:'https://api.spotify.com/v1/albums/'+id+'/tracks?offset=1'}}))).mockImplementationOnce(async()=>{useStore.getState().logoutSpotify();useStore.getState().setSpotifyConnected(true);return new Response(JSON.stringify({offset:1,total:2,items:[{id:'old2'}],next:null}))});vi.stubGlobal('fetch',mock);await expect(fetchSpotifyAlbum(id)).rejects.toThrow();
});

it('rejects a truncated terminal album rather than silently playing a subset',async()=>{vi.stubGlobal('fetch',vi.fn().mockResolvedValue(new Response(JSON.stringify({tracks:{offset:0,total:2,items:[{id:'track'}],next:null}}))));await expect(fetchSpotifyAlbum(id)).rejects.toThrow('Incomplete');});
