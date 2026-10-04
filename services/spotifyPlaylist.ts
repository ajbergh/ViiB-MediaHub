import {backendSpotifyFetch,assertSpotifySession} from './spotifyBackend';
import {useStore} from '../store';

/** Preserve every playlist occurrence and fence all pages to one account. */
export async function fetchSpotifyPlaylist(id:string, initialResponse?:Response):Promise<any>{
 if(!/^[A-Za-z0-9]{22}$/.test(id))throw new Error('Invalid Spotify playlist');
 const generation=useStore.getState().spotifySessionGeneration;assertSpotifySession(generation);
 const response=initialResponse||await backendSpotifyFetch('/playlists/'+id);
 if(!response.ok)throw new Error('Failed to fetch Spotify playlist');
 const playlist=await response.json();const first=playlist.tracks;
 if(!Array.isArray(first?.items))throw new Error('Invalid Spotify playlist tracks');
 const items=[...first.items];let page=first;let previousOffset=Number.isInteger(first.offset)?first.offset:0;
 const path='/v1/playlists/'+id+'/tracks';
 while(page.next){
  const next=new URL(page.next);const offset=Number(next.searchParams.get('offset'));
  if(next.origin!=='https://api.spotify.com'||next.pathname!==path||!Number.isInteger(offset)||offset<=previousOffset)throw new Error('Invalid Spotify playlist pagination');
  assertSpotifySession(generation);const response=await backendSpotifyFetch(next.pathname.slice(3)+next.search);
  if(!response.ok)throw new Error('Failed to fetch Spotify playlist tracks');
  page=await response.json();if(!Array.isArray(page.items)||page.offset!==offset||page.total!==first.total||!page.items.length)throw new Error('Inconsistent Spotify playlist pagination');
  if(playlist.snapshot_id&&page.snapshot_id&&playlist.snapshot_id!==page.snapshot_id)throw new Error('Spotify playlist changed while loading');
  items.push(...page.items);previousOffset=offset;
 }
 if(Number.isInteger(first.total)&&items.length!==first.total)throw new Error('Incomplete Spotify playlist tracks');
 assertSpotifySession(generation);return {...playlist,tracks:{...first,items:items.filter(item=>item?.track?.id),next:null}};
}
