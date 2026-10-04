/** Tests resource validation, backend errors, and session-generation fences. */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { backendSpotifyFetch, SpotifySessionChangedError } from './spotifyBackend';
import { SpotifyService } from './spotifyService';
import { api } from './api';
import { SpotifyAuthError, SpotifyRateLimitError } from '../lib/spotifyErrors';

import { useStore } from '../store';
beforeEach(() => {useStore.getState().logoutSpotify();useStore.getState().setSpotifyConnected(true);});
afterEach(() => {vi.useRealTimers();vi.unstubAllGlobals();vi.restoreAllMocks();});
describe('backend Spotify session boundary', () => {
    it('forwards paths and query parameters without renderer credentials', async () => {
        const fetchMock=vi.fn(async () => new Response('{}',{status:200}));
        vi.stubGlobal('fetch',fetchMock);
        await backendSpotifyFetch('/playlists/recording/tracks?limit=100&offset=100');
        const [target,options]=fetchMock.mock.calls[0] as unknown as [string,unknown];
        const url=new URL(target,'http://local');
        expect(url.pathname).toBe('/api/spotify/proxy');
        expect(url.searchParams.get('path')).toBe('playlists/recording/tracks');
        expect(url.searchParams.get('offset')).toBe('100');
        expect(options).toMatchObject({signal: expect.any(AbortSignal)});
        expect((options as RequestInit).headers).toBeUndefined();
        await expect(backendSpotifyFetch('https://untrusted.invalid')).rejects.toThrow();
        expect(fetchMock).toHaveBeenCalledTimes(1);
    });
    it('preserves reconnect, rate-limit and playlist fallback outcomes', async () => {
        const mock=vi.fn().mockResolvedValueOnce(new Response('{}',{status:401})).mockResolvedValueOnce(new Response('{}',{status:429,headers:{'Retry-After':'28'}})).mockResolvedValueOnce(new Response('{}',{status:403}));
        vi.stubGlobal('fetch',mock);
        await expect(backendSpotifyFetch('/me')).rejects.toBeInstanceOf(SpotifyAuthError);
        expect(useStore.getState().spotifyAuthRequired).toBe(true);
        expect(useStore.getState().spotifyConnected).toBe(false);
        useStore.getState().setSpotifyConnected(true);
        await expect(backendSpotifyFetch('/search?q=music')).rejects.toMatchObject({retryAfter:28});
        const denied=await backendSpotifyFetch('/playlists/id');
        expect(denied.status).toBe(403);
        expect(mock).toHaveBeenCalledTimes(3);
    });
    it('connects, probes status and disconnects only through the backend',async()=>{
        const mock=vi.fn(async(_target: string, _init?: RequestInit)=>new Response(JSON.stringify({provider:'webplayer',connected:true,authRequired:false,message:''}),{status:200}));
        vi.stubGlobal('fetch',mock);
        await api.connectSpotifySession('fixture-session');
        await api.getSpotifyAuthStatus();
        await api.disconnectSpotifySession();
        expect(mock.mock.calls.map(call=>call[0])).toEqual(['/api/spotify/auth/session','/api/spotify/auth/status','/api/spotify/auth/session']);
        const connectOptions=mock.mock.calls[0][1] as RequestInit;
        expect(JSON.parse(connectOptions.body as string)).toEqual({spDC:'fixture-session'});
        expect(connectOptions.headers).toEqual({'Content-Type':'application/json'});
        expect((mock.mock.calls[2][1] as RequestInit).method).toBe('DELETE');
    });
    it('runs catalog and library functions without renderer OAuth credentials',async()=>{
        vi.useFakeTimers();
        const paths:string[]=[];
        vi.stubGlobal('fetch',vi.fn(async(target:string)=>{
            paths.push(target);
            return new Response(JSON.stringify({id:'profile',display_name:'fixture',items:[],artists:{items:[]},albums:{items:[]},tracks:{items:[]},playlists:{items:[]}}),{status:200});
        }));
        const work=[
            SpotifyService.getUserProfile(),
            SpotifyService.search('music'),
            SpotifyService.searchArtist('artist'),
            SpotifyService.searchAlbum('album','artist'),
            SpotifyService.getRecentlyPlayed(50),
            SpotifyService.getSavedAlbums(20,20),
            SpotifyService.getSavedPlaylists(20,20),
            SpotifyService.getArtistTopTracks('artist','US'),
            SpotifyService.getArtist('artist')
        ];
        await vi.runAllTimersAsync();
        await Promise.all(work);
        expect(paths).toHaveLength(10);
        expect(paths.every(path=>path.startsWith('/api/spotify/'))).toBe(true);
        expect(paths.filter(path=>path.startsWith('/api/spotify/proxy?'))).toHaveLength(9);
        const resources=paths.map(path=>new URL(path,'http://local').searchParams.get('path'));
        expect(resources).toContain('me/albums');
        expect(resources).toContain('me/playlists');
        expect(resources).toContain('me/player/recently-played');
    });
});

describe('Spotify account response fencing',()=>{
 it('rejects a late 401 without disconnecting the replacement account',async()=>{
  let complete!:(response:Response)=>void;
  vi.stubGlobal('fetch',vi.fn(()=>new Promise<Response>(resolve=>{complete=resolve;})));
  const pending=backendSpotifyFetch('/me');
  const rejected=expect(pending).rejects.toBeInstanceOf(SpotifySessionChangedError);
  useStore.getState().logoutSpotify();useStore.getState().setSpotifyConnected(true);
  complete(new Response('{}',{status:401}));
  await rejected;
  expect(useStore.getState().spotifyConnected).toBe(true);
  expect(useStore.getState().spotifyAuthRequired).toBe(false);
 });
 it('rejects a body completed after account replacement',async()=>{
  vi.stubGlobal('fetch',vi.fn(async()=>new Response('{"id":"old-user"}')));
  const response=await backendSpotifyFetch('/me');
  useStore.getState().logoutSpotify();useStore.getState().setSpotifyConnected(true);
  await expect(response.json()).rejects.toBeInstanceOf(SpotifySessionChangedError);
 });
 it('rejects old queued work before another network request',async()=>{
  vi.useFakeTimers();
  vi.stubGlobal('fetch',vi.fn(async()=>new Response('{"id":"old-user"}')));
  const first=SpotifyService.getUserProfile();
  const second=SpotifyService.getSavedAlbums();
  const settled=Promise.allSettled([first,second]);
  useStore.getState().logoutSpotify();useStore.getState().setSpotifyConnected(true);
  await vi.runAllTimersAsync();
  expect((await settled).every(result=>result.status==='rejected')).toBe(true);
  expect(fetch).toHaveBeenCalledTimes(1);
 });
 it('suppresses requests while reconnect is required',async()=>{
  vi.stubGlobal('fetch',vi.fn());
  useStore.getState().markSpotifyAuthRequired();
  await expect(backendSpotifyFetch('/me')).rejects.toBeInstanceOf(SpotifyAuthError);
  expect(fetch).not.toHaveBeenCalled();
 });
});

describe('interactive Spotify search latency', () => {
 it('dispatches search while the startup profile is still pending', async () => {
  vi.useFakeTimers();
  let finishProfile!: (response: Response) => void;
  const mock = vi.fn((target: string) => {
   const path = new URL(target, 'http://local').searchParams.get('path');
   if (path === 'me') return new Promise<Response>(resolve => {finishProfile = resolve;});
   return Promise.resolve(new Response('{"tracks":{"items":[]}}'));
  });
  vi.stubGlobal('fetch', mock);
  const profile = SpotifyService.getUserProfile();
  const result = await SpotifyService.search('music', ['track']);
  expect(result.tracks.items).toEqual([]);
  expect(mock).toHaveBeenCalledTimes(2);
  finishProfile(new Response('{"id":"profile"}'));
  await profile;
  await vi.runAllTimersAsync();
 });
 it('publishes catalog results before playlist fallback and preserves merged results', async () => {
  let finishFallback!: (value: any) => void;
  vi.spyOn(api, 'searchPlaylistsFallback').mockImplementation(() => new Promise(resolve => {finishFallback = resolve;}));
  vi.stubGlobal('fetch', vi.fn(async () => new Response('{"tracks":{"items":[{"id":"track"}]},"playlists":{"items":[{"id":"existing"}],"total":1}}')));
  let publish!: () => void;
  const published = new Promise<void>(resolve => {publish = resolve;});
    const onCatalogResults = vi.fn((_results: any) => publish());
  let finished = false;
  const work = SpotifyService.search('music', ['track', 'playlist'], 20, 0, {onCatalogResults}).then(result => {finished = true; return result;});
  await published;
  expect(onCatalogResults.mock.calls[0][0]).toMatchObject({tracks: {items: [{id:'track'}]}});
  expect(finished).toBe(false);
  // Allow the dynamic API import to enqueue the fallback without real timers.
  await vi.waitFor(() => expect(finishFallback).toBeTypeOf('function'));
  finishFallback({playlists: {items: [{id:'existing'}, {id:'first-party'}], total:2}});
  const result = await work;
  expect(result.playlists.items.map((item: any) => item.id)).toEqual(['first-party', 'existing']);
 });
 it('aborts superseded search work without auth failure or error logging', async () => {
  const controller = new AbortController();
  const addLog = vi.spyOn(useStore.getState(), 'addLog');
  vi.stubGlobal('fetch', vi.fn((_target: string, init: RequestInit) => new Promise<Response>((_resolve, reject) => {
   init.signal!.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')), {once:true});
  })));
  const work = SpotifyService.search('old', ['track'], 20, 0, {signal:controller.signal});
  const rejected = expect(work).rejects.toMatchObject({name:'AbortError'});
  controller.abort();
  await rejected;
  expect(useStore.getState().spotifyConnected).toBe(true);
  expect(addLog).not.toHaveBeenCalled();
  addLog.mockRestore();
 });
 it('cancels response body work after headers have arrived', async () => {
  const controller = new AbortController();
  let transportSignal!: AbortSignal;
  vi.stubGlobal('fetch', vi.fn(async (_target: string, init: RequestInit) => {
   transportSignal = init.signal as AbortSignal;
   return new Response('{}');
  }));
  const response = await backendSpotifyFetch('/search?q=old', controller.signal);
  controller.abort();
  expect(transportSignal.aborted).toBe(true);
  await expect(response.json()).rejects.toMatchObject({name:'AbortError'});
 });
});
