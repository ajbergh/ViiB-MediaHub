import { readFileSync } from 'node:fs';
import { afterEach, expect, it, vi } from 'vitest';
import { catalogFieldInventory, loadImportedCatalog, type ImportedCatalog } from './importedCatalog';

const recording='A'.repeat(22);
const fixture=():ImportedCatalog=>({recordingId:recording,sourceFingerprint:'fp',provenance:'spotify_download_import',snapshots:[],relations:[],origins:[],catalogStatus:{state:'not_available',reason:'no_eligible_catalog',scope:'track_album_artist_graph_v2',checkedAt:'2026-10-01T00:00:00Z'}});
afterEach(()=>vi.unstubAllGlobals());
it('reads a source-qualified empty outcome without provider requests and treats absence separately',async()=>{
 const fetchMock=vi.fn(async()=>new Response(JSON.stringify(fixture()),{headers:{ETag:'"fp"'}}));vi.stubGlobal('fetch',fetchMock);
 expect((await loadImportedCatalog('song /','fp'))?.catalogStatus?.state).toBe('not_available');
 expect(fetchMock).toHaveBeenCalledWith('/api/v2/analysis/song%20%2F/imported-catalog',expect.objectContaining({cache:'no-store'}));
 fetchMock.mockResolvedValue(new Response(null,{status:404}));expect(await loadImportedCatalog('song')).toBeNull();
 fetchMock.mockResolvedValue(new Response(null,{status:500}));await expect(loadImportedCatalog('song')).rejects.toThrow('unavailable');
});
it('rejects mismatched source, etag, orphan relations and over-bound shapes',async()=>{
 const data=fixture();
 const fetchMock=vi.fn(async()=>new Response(JSON.stringify(data),{headers:{ETag:'"fp"'}}));vi.stubGlobal('fetch',fetchMock);
 await expect(loadImportedCatalog('song','different')).rejects.toThrow();
 fetchMock.mockResolvedValue(new Response(JSON.stringify(data),{headers:{ETag:'"changed"'}}));await expect(loadImportedCatalog('song')).rejects.toThrow();
 data.relations=[{parentType:'track',parentId:recording,resource:'track',kind:'artist',position:0,childType:'artist',childId:recording,unavailable:false,metadata:{}}];
 fetchMock.mockImplementation(async()=>new Response(JSON.stringify(data)));await expect(loadImportedCatalog('song')).rejects.toThrow('relation');
 data.relations=[];data.origins=Array.from({length:65},()=>({kind:'playlist',id:recording,position:0}));await expect(loadImportedCatalog('song')).rejects.toThrow();
});
it('uses an explicit bounded inventory and preserves zero, false and null',()=>{
 const inventory=catalogFieldInventory({name:'Track',popularity:0,explicit:false,release_date:null,private_unknown:'never render',artists:Array.from({length:20},(_,i)=>({name:`Artist ${i}`}))});
 expect(inventory).toContainEqual(['Popularity','0']);expect(inventory).toContainEqual(['Explicit','No']);expect(inventory).toContainEqual(['Release date','Unknown']);
 expect(JSON.stringify(inventory)).not.toContain('never render');expect(inventory.filter(([key])=>/^Artist \d+$/.test(key))).toHaveLength(10);
});

it('displays REST genre/tag lists without replacing missing or explicitly unknown values',()=>{
 expect(catalogFieldInventory({})).toEqual([]);
 const inventory=catalogFieldInventory({genres:['house','ambient'],tags:[0,false,null,''],album:{release_date:'2020',release_date_precision:'year',genres:[],artists:[{name:null,id:'not-a-name'}]}});
 expect(inventory).toContainEqual(['Genres','house · ambient']);
 expect(inventory).toContainEqual(['Tags','0 · No · Unknown · (empty)']);
 expect(inventory).toContainEqual(['Album genres','(empty list)']);
 expect(inventory).toContainEqual(['Album release date','2020']);expect(inventory).toContainEqual(['Album date precision','year']);
 expect(inventory).toContainEqual(['Album artist 1','Unknown']);expect(JSON.stringify(inventory)).not.toContain('not-a-name');
 expect(catalogFieldInventory({genres:null,tags:[]})).toEqual([['Genres','Unknown'],['Tags','(empty list)']]);
});
it('reads original Web Player paths and preserves partial dates, false playability and content rating labels',()=>{
 const inventory=catalogFieldInventory({uri:'spotify:track:retained',duration:{totalMilliseconds:0},contentRating:{label:'NONE'},playability:{playable:false},albumOfTrack:{name:'Album',date:{year:2020},artists:{items:[{profile:{name:'Album artist'}}]}},firstArtist:{items:[{profile:{name:null},uri:'do-not-substitute'}]},otherArtists:{items:[{profile:{name:'Guest'}}]},ownerV2:{data:{name:'Owner',uri:'spotify:user:owner'}},future:{name:'hidden nested value'}});
 expect(inventory).toContainEqual(['Duration (milliseconds, duration)','0']);expect(inventory).toContainEqual(['Content rating (Web Player)','NONE']);expect(inventory).toContainEqual(['Playable when captured (Web Player)','No']);
 expect(inventory).toContainEqual(['Album (Web Player) release year','2020']);expect(JSON.stringify(inventory)).not.toContain('2020-01-01');
 expect(inventory).toContainEqual(['Album (Web Player) artist 1','Album artist']);expect(inventory).toContainEqual(['Primary artist 1','Unknown']);expect(inventory).toContainEqual(['Other artist 1','Guest']);expect(inventory).toContainEqual(['Owner (Web Player) name','Owner']);
 expect(JSON.stringify(inventory)).not.toContain('hidden nested value');expect(JSON.stringify(inventory)).not.toContain('do-not-substitute');
 expect(catalogFieldInventory({date:{isoString:null,year:2020}})).toContainEqual(['Release date (Web Player)','Unknown']);
});
it('bounds long tag collections and arbitrary structured values without recursively displaying them',()=>{
 const inventory=catalogFieldInventory({genres:Array.from({length:100},(_,i)=>`g${i}`),tags:[{secret:'hidden object'},'x'.repeat(1000)],artists:Array.from({length:100},(_,i)=>({profile:{name:`a${i}`}}))});
 expect(inventory).toContainEqual(['Artist display limit','90 more retained']);
 const genres=inventory.find(([key])=>key==='Genres')![1];expect(genres).toContain('80 more retained');expect(genres).not.toContain('g20');
 expect(inventory.every(([,value])=>value.length<=257)).toBe(true);expect(JSON.stringify(inventory)).not.toContain('hidden object');
});

it('displays the same REST and Web Player fields exercised by backend capture fixtures',()=>{
 const fixtures=JSON.parse(readFileSync(new URL('../testdata/spotify/catalog-inventory-v1.json',import.meta.url),'utf8'));
 for(const fixture of fixtures) {
  const payload=fixture.transport==='rest'?fixture.payload:fixture.payload.data.trackUnion;
  const inventory=catalogFieldInventory(payload);
  for(const expected of fixture.inventory) expect(inventory,fixture.name).toContainEqual(expected);
  expect(JSON.stringify(inventory)).not.toContain('synthetic-must-remove');
  expect(JSON.stringify(inventory)).not.toContain('future_field');
 }
});

it('retains a qualified library collection identity and rejects invalid identity claims',async()=>{
 const data=fixture();data.origins=[{kind:'library',id:'saved_albums',entityId:recording,position:-1}];
 vi.stubGlobal('fetch',vi.fn(async()=>new Response(JSON.stringify(data))));
 expect((await loadImportedCatalog('song'))?.origins[0].entityId).toBe(recording);
 data.origins[0].entityId='invalid';await expect(loadImportedCatalog('song')).rejects.toThrow('origin');
 data.origins=[{kind:'playlist',id:recording,entityId:recording,position:0}];await expect(loadImportedCatalog('song')).rejects.toThrow('origin');
});

it('reads requested collection provenance and rejects malformed collection status',async()=>{
 const data=fixture();data.collectionStatus={state:'available',reason:'requested_observations_retained',scope:'requested_collection_observations_v1',checkedAt:'2026-10-01T00:00:00Z'};
 data.snapshots=[{entityType:'playlist',spotifyId:recording,resource:'request:'+recording,capturedResource:'original-page',captureRevision:'v1',schemaVersion:1,adapterRevision:'fixture',payload:{name:'Playlist'},retrievedAt:'2026-10-01T00:00:00Z',expiresAt:'2026-10-02T00:00:00Z',stale:true}];
 vi.stubGlobal('fetch',vi.fn(async()=>new Response(JSON.stringify(data))));
 expect((await loadImportedCatalog('song'))?.snapshots[0].captureRevision).toBe('v1');
 data.snapshots[0].entityType='track';await expect(loadImportedCatalog('song')).rejects.toThrow('snapshot');
 data.snapshots[0].entityType='playlist';data.snapshots[0].capturedResource=undefined;await expect(loadImportedCatalog('song')).rejects.toThrow('snapshot');
 data.snapshots=[];(data.collectionStatus as any).scope='all_provider_data';await expect(loadImportedCatalog('song')).rejects.toThrow('collection');
});
