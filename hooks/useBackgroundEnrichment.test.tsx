// @vitest-environment jsdom
import React, {act} from 'react';
import {createRoot, Root} from 'react-dom/client';
import {afterEach, expect, it, vi} from 'vitest';
import {useStore} from '../store';
import api from '../services/api';
import {useBackgroundEnrichment} from './useBackgroundEnrichment';

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
 vi.spyOn(api,'getUncheckedAlbumMetadata').mockResolvedValue([{albumName:'Album',artistName:'Artist'}] as any);
 vi.spyOn(api,'getExpiredAlbumMetadata').mockResolvedValue([]);
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
 expect(api.getUncheckedAlbumMetadata).toHaveBeenCalledTimes(1);
});
