// @vitest-environment jsdom
import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { beforeEach, expect, it, vi } from 'vitest';
import { SpotifyCatalogEvidence } from './SpotifyCatalogEvidence';
import type { ImportedCatalog } from '../services/importedCatalog';

const mocks=vi.hoisted(()=>({load:vi.fn(),session:0}));
vi.mock('../services/importedCatalog',async importOriginal=>({...await importOriginal<typeof import('../services/importedCatalog')>(),loadImportedCatalog:mocks.load}));
vi.mock('../store',()=>({useStore:(selector:any)=>selector({spotifySessionGeneration:mocks.session})}));
beforeEach(()=>{mocks.load.mockReset();mocks.session=0;(globalThis as any).IS_REACT_ACT_ENVIRONMENT=true;});
const recording='A'.repeat(22);
const fixture=():ImportedCatalog=>({recordingId:recording,sourceFingerprint:'fp',provenance:'spotify_download_import',snapshots:[{entityType:'track',spotifyId:recording,resource:'track',schemaVersion:1,adapterRevision:'fixture',payload:{name:'Retained track',popularity:0,explicit:false,release_date:null,unknown:'hidden domain data'},retrievedAt:'2026-10-01T00:00:00Z',expiresAt:'2026-10-02T00:00:00Z',stale:true}],relations:Array.from({length:26},(_,i)=>({parentType:'track',parentId:recording,resource:'track',kind:'artists',position:i,childType:'artist',childId:recording,unavailable:i===1,metadata:{}})),origins:[{kind:'playlist',id:recording,revision:'observed',position:0},{kind:'playlist',id:recording,revision:'observed',position:4},{kind:'library',id:'saved_playlists',entityId:recording,position:-1}],catalogStatus:{state:'incomplete',reason:'related_entity_unavailable',scope:'track_album_artist_graph_v2',checkedAt:'2026-10-01T00:00:00Z'},lineageStatus:{state:'available',checkedAt:'2026-10-01T00:00:00Z'}});
async function open(host:HTMLElement){await act(async()=>{const details=host.querySelector('details')!;details.open=true;details.dispatchEvent(new Event('toggle'));});}
async function click(host:HTMLElement,label:string){await act(async()=>Array.from(host.querySelectorAll('button')).find(b=>b.textContent===label)!.click());}
it('loads offline evidence on demand and pages ordered rows without dropping unavailable entries or origins',async()=>{
 mocks.load.mockResolvedValue(fixture());const host=document.createElement('div'),root=createRoot(host);
 try {
 await act(async()=>root.render(<SpotifyCatalogEvidence songId="song" fingerprint="fp"/>));expect(mocks.load).not.toHaveBeenCalled();await open(host);
 expect(host.textContent).toContain('Retained track');expect(host.textContent).toContain('Stale observation');expect(host.textContent).toContain('Related album or artist material missing');expect(host.textContent).toContain('last-good');
 expect(host.textContent).toContain('Popularity0');expect(host.textContent).toContain('ExplicitNo');expect(host.textContent).toContain('Release dateUnknown');expect(host.textContent).not.toContain('hidden domain data');
 expect(host.querySelectorAll('tbody tr')).toHaveLength(25);expect(host.textContent).toContain('Unavailable entry');expect(host.textContent).toContain('Position 4 (zero-based)');expect(host.textContent).toContain('Position unknown');expect(host.textContent).toContain(`Collection ${recording}`);
 await click(host,'Next relations');expect(host.querySelectorAll('tbody tr')).toHaveLength(1);expect(host.textContent).toContain('26–26 of 26');
 }finally{await act(async()=>root.unmount());}
});
it('keeps retry separate and accepts status-only successful responses',async()=>{
 mocks.load.mockRejectedValueOnce(new Error('temporary')).mockResolvedValue({...fixture(),snapshots:[],relations:[],origins:[],catalogStatus:{...fixture().catalogStatus!,state:'oversized',reason:'snapshot_limit'}});
 const host=document.createElement('div'),root=createRoot(host);
 try{await act(async()=>root.render(<SpotifyCatalogEvidence songId="song"/>));await open(host);expect(host.textContent).toContain('could not be loaded');await click(host,'Retry retained catalog');expect(host.textContent).toContain('Catalog snapshot limit exceeded');expect(host.textContent).toContain('No catalog snapshots retained');}
 finally{await act(async()=>root.unmount());}
});
it('rejects late song, source and account results',async()=>{
 let settle!:(data:ImportedCatalog)=>void;mocks.load.mockImplementationOnce(()=>new Promise(done=>{settle=done;})).mockResolvedValue(null);
 const host=document.createElement('div'),root=createRoot(host);
 try{await act(async()=>root.render(<SpotifyCatalogEvidence songId="old" fingerprint="fp"/>));await open(host);
 mocks.session=1;await act(async()=>root.render(<SpotifyCatalogEvidence songId="new" fingerprint="replacement"/>));await act(async()=>settle(fixture()));
 expect(host.textContent).not.toContain('Retained track');expect(host.textContent).toContain('No retained catalog');
 }finally{await act(async()=>root.unmount());}
});

it('shows original collection resources and observed revisions with separate incomplete outcome',async()=>{
 const data=fixture();data.collectionStatus={state:'incomplete',reason:'requested_collection_unavailable',scope:'requested_collection_observations_v1',checkedAt:'2026-10-01T00:00:00Z'};
 data.snapshots=[{...data.snapshots[0],entityType:'playlist',resource:'request:observation-key',capturedResource:'original-page',captureRevision:'v1'}];
 data.relations=[{...data.relations[0],parentType:'playlist',resource:'request:observation-key'}];
 mocks.load.mockResolvedValue(data);const host=document.createElement('div'),root=createRoot(host);
 try{
  await act(async()=>root.render(<SpotifyCatalogEvidence songId="song" fingerprint="fp"/>));await open(host);
  expect(host.textContent).toContain('Some requested collection evidence was missing or could not be verified');
  expect(host.textContent).toContain('Earlier retained collection observations remain');
  expect(host.textContent).toContain('Original captured resource: original-page · Observed playlist revision v1');
  expect(host.querySelector('tbody')!.textContent).toContain('original-page · Revision v1');
  expect(host.textContent).not.toContain('request:observation-key');
  expect(host.textContent).toContain('does not establish current membership or a complete saved library');
 }finally{await act(async()=>root.unmount());}
});
