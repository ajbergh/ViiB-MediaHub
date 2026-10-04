import {backendSpotifyFetch, assertSpotifySession} from './spotifyBackend';
import {useStore} from '../store';

/** Load every track page before album actions use the playback context. */
export async function fetchSpotifyAlbum(id: string): Promise<any> {
 if (!/^[A-Za-z0-9]{22}$/.test(id)) throw new Error('Invalid Spotify album');
 const generation = useStore.getState().spotifySessionGeneration;
 assertSpotifySession(generation);
 const response = await backendSpotifyFetch('/albums/' + id);
 if (!response.ok) throw new Error('Failed to fetch Spotify album');
 const album = await response.json();
 const first = album.tracks;
 if (!Array.isArray(first?.items)) throw new Error('Invalid Spotify album tracks');
 const items = [...first.items];
 let page = first;
 let previousOffset = Number.isInteger(first.offset) ? first.offset : 0;
 const expectedPath = '/v1/albums/' + id + '/tracks';
 while (page.next) {
  const next = new URL(page.next);
  const offset = Number(next.searchParams.get('offset'));
  if (next.origin !== 'https://api.spotify.com' || next.pathname !== expectedPath || !Number.isInteger(offset) || offset <= previousOffset) throw new Error('Invalid Spotify album pagination');
  assertSpotifySession(generation);
  const response = await backendSpotifyFetch(next.pathname.slice(3) + next.search);
  if (!response.ok) throw new Error('Failed to fetch Spotify album tracks');
  page = await response.json();
  if (!Array.isArray(page.items) || page.offset !== offset || page.total !== first.total || !page.items.length) throw new Error('Inconsistent Spotify album pagination');
  items.push(...page.items);
  previousOffset = offset;
 }
 if (Number.isInteger(first.total) && items.length !== first.total) throw new Error('Incomplete Spotify album tracks');
 assertSpotifySession(generation);
 return {...album, tracks: {...first, items: items.filter(item => item?.id), next: null}};
}
