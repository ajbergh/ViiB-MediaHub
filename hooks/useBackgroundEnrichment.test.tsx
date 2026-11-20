// @vitest-environment jsdom
/** Tests background metadata enrichment scheduling and Spotify connection/session guards. */

import React, {act} from 'react';
import {createRoot, Root} from 'react-dom/client';
import {afterEach, expect, it, vi} from 'vitest';
import {useStore} from '../store';
import api from '../services/api';
import {useBackgroundEnrichment, useVisibleAlbumEnrichment} from './useBackgroundEnrichment';

let root:Root|undefined;
const original=useStore.getState().fetchAlbumMetadata;
afterEach(async()=>{
 if(root){await act(async()=>root!.unmount());root=undefined;}
 useStore.setState({fetchAlbumMetadata:original,backendAvailable:false});
 useStore.getState().logoutSpotify();
 vi.restoreAllMocks();vi.useRealTimers();
});
function Worker(){useBackgroundEnrichment();return null;}

it('does not schedule another album after rejection during pending enrichment',async()=>{
 (globalThis as any).IS_REACT_ACT_ENVIRONMENT=true;
 vi.useFakeTimers();
 vi.spyOn(api,'getNextAlbumMetadata').mockResolvedValue({albumName:'Album',artistName:'Artist'} as any);
 let finish!:()=>void;
 const enrichment=vi.fn(()=>new Promise<void>(resolve=>{finish=resolve;}));
 useStore.setState({backendAvailable:true,fetchAlbumMetadata:enrichment});
 useStore.getState().setSpotifyConnected(true);
 root=createRoot(document.createElement('div'));
 await act(async()=>root!.render(<Worker/>));
 await act(async()=>vi.advanceTimersByTimeAsync(10000));
 expect(enrichment).toHaveBeenCalledTimes(1);
 await act(async()=>{useStore.getState().markSpotifyAuthRequired();finish();});
 await act(async()=>vi.advanceTimersByTimeAsync(120000));
 expect(enrichment).toHaveBeenCalledTimes(1);
 expect(api.getNextAlbumMetadata).toHaveBeenCalledTimes(1);
});

it('prioritizes visible albums and leaves one worker to process the backlog', async () => {
 (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
 vi.useFakeTimers();
 const backlog = vi.spyOn(api, 'getNextAlbumMetadata').mockResolvedValue(null);
 const enrich = vi.fn(async () => {});
 const albums = [
   { name: 'Plex cover', artist: 'Artist', songCount: 1, plexBacked: true },
   { name: 'Visible', artist: 'Artist', songCount: 1 },
 ];
 function Visible() { useVisibleAlbumEnrichment(albums); return null; }
 useStore.setState({ backendAvailable: true, fetchAlbumMetadata: enrich });
 useStore.getState().setSpotifyConnected(true);
 root = createRoot(document.createElement('div'));
 await act(async () => root!.render(<><Worker/><Visible/></>));
 await act(async () => vi.advanceTimersByTimeAsync(300));
 expect(enrich).toHaveBeenCalledWith('Visible', 'Artist');
 expect(backlog).not.toHaveBeenCalled();
 await act(async () => vi.advanceTimersByTimeAsync(5000));
 expect(enrich).toHaveBeenCalledTimes(1);
 expect(backlog).toHaveBeenCalledTimes(1);
});
