import type { SpotifyDownloadOrigin } from './api';

export interface CatalogSnapshot {
 capturedResource?: string; captureRevision?: string;
 entityType: string; spotifyId: string; resource: string; schemaVersion: number; adapterRevision: string;
 payload: Record<string, unknown>; retrievedAt: string; expiresAt: string; stale: boolean;
}
export interface CatalogRelation {
 parentType: string; parentId: string; resource: string; kind: string; position: number;
 childType: string; childId: string; unavailable: boolean; metadata: unknown;
}
export interface ImportedCatalog {
 recordingId: string; sourceFingerprint: string; provenance: 'spotify_download_import';
 snapshots: CatalogSnapshot[]; relations: CatalogRelation[]; origins: SpotifyDownloadOrigin[];
 catalogStatus?: { state: 'available' | 'incomplete' | 'oversized' | 'not_available'; reason: string; scope: string; checkedAt: string };
 collectionStatus?: { state: 'available' | 'incomplete' | 'oversized' | 'not_available'; reason: string; scope: 'requested_collection_observations_v1'; checkedAt: string };
 lineageStatus?: { state: 'available' | 'oversized' | 'no_active_request_lineage'; checkedAt: string };
}
const text = (value: unknown, max = 256): value is string => typeof value === 'string' && value.length > 0 && value.length <= max;
const id = (value: unknown) => typeof value === 'string' && /^[A-Za-z0-9]{22}$/.test(value);
const date = (value: unknown) => typeof value === 'string' && Number.isFinite(Date.parse(value));
const object = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);

export async function loadImportedCatalog(songId: string, fingerprint?: string, signal?: AbortSignal): Promise<ImportedCatalog | null> {
 const response = await fetch(`/api/v2/analysis/${encodeURIComponent(songId)}/imported-catalog`, { cache: 'no-store', signal });
 if (response.status === 404) return null;
 if (!response.ok) throw new Error('Retained catalog unavailable');
 const body = await response.text();
 if (body.length > 16 * 1024 * 1024) throw new Error('Retained catalog exceeds display limit');
 const data = JSON.parse(body) as ImportedCatalog;
 if (!object(data) || !id(data.recordingId) || !text(data.sourceFingerprint) || (fingerprint && data.sourceFingerprint !== fingerprint) ||
     data.provenance !== 'spotify_download_import' || !Array.isArray(data.snapshots) || data.snapshots.length > 256 ||
     !Array.isArray(data.relations) || data.relations.length > 20000 || !Array.isArray(data.origins) || data.origins.length > 64) throw new Error('Invalid retained catalog');
 const etag = response.headers.get('ETag');
 if (etag && etag !== JSON.stringify(data.sourceFingerprint)) throw new Error('Catalog source changed');
 const parents = new Set<string>();
 for (const s of data.snapshots) {
  if (!object(s) || (s.capturedResource != null && (!text(s.capturedResource,128) || !['playlist','library'].includes(s.entityType))) || (s.captureRevision != null && (!text(s.captureRevision) || !s.capturedResource || s.entityType !== 'playlist')) || !['track','album','artist','playlist','library'].includes(s.entityType) || !text(s.spotifyId) || !text(s.resource) || s.schemaVersion !== 1 || !text(s.adapterRevision) || !object(s.payload) || !date(s.retrievedAt) || !date(s.expiresAt) || typeof s.stale !== 'boolean') throw new Error('Invalid catalog snapshot');
  parents.add(JSON.stringify([s.entityType,s.spotifyId,s.resource]));
 }
 for (const r of data.relations) {
  if (!object(r) || !parents.has(JSON.stringify([r.parentType,r.parentId,r.resource])) || !text(r.kind,128) || !Number.isInteger(r.position) || r.position < 0 || typeof r.childType !== 'string' || r.childType.length > 128 || (r.childId !== '' && !id(r.childId)) || typeof r.unavailable !== 'boolean') throw new Error('Invalid catalog relation');
 }
 for (const o of data.origins) {
  if (!object(o) || (o.entityId != null && (!id(o.entityId) || o.kind !== 'library')) || !['album','playlist','library'].includes(o.kind) || (o.kind === 'library' ? !['saved_tracks','saved_albums','saved_playlists'].includes(o.id) : !id(o.id)) || !Number.isInteger(o.position) || o.position < -1 || o.position > 1000000 || (o.revision != null && (typeof o.revision !== 'string' || o.revision.length > 256))) throw new Error('Invalid request origin');
 }
 if (data.catalogStatus && (!object(data.catalogStatus) || !['available','incomplete','oversized','not_available'].includes(data.catalogStatus.state) || !text(data.catalogStatus.reason) || !['track_and_direct_relations_v1','track_album_artist_graph_v2'].includes(data.catalogStatus.scope) || !date(data.catalogStatus.checkedAt))) throw new Error('Invalid catalog outcome');
 if (data.collectionStatus && (!object(data.collectionStatus) || !['available','incomplete','oversized','not_available'].includes(data.collectionStatus.state) || !text(data.collectionStatus.reason) || data.collectionStatus.scope !== 'requested_collection_observations_v1' || !date(data.collectionStatus.checkedAt))) throw new Error('Invalid collection outcome');
 if (data.lineageStatus && (!object(data.lineageStatus) || !['available','oversized','no_active_request_lineage'].includes(data.lineageStatus.state) || !date(data.lineageStatus.checkedAt))) throw new Error('Invalid origin outcome');
 return data;
}

// The display inventory is deliberately explicit. Additional retained domain
// fields remain stored; they are never recursively rendered into the panel.
export function catalogFieldInventory(payload: Record<string, unknown>): Array<[string, string]> {
 const result: Array<[string,string]> = [];
 const own = (value: Record<string,unknown>, key: string) => Object.prototype.hasOwnProperty.call(value,key);
 const display = (value: unknown, limit = 256): string => value === null ? 'Unknown' : typeof value === 'boolean' ? (value ? 'Yes' : 'No') : typeof value === 'number' ? String(value) : typeof value === 'string' ? (value.length > limit ? `${value.slice(0,limit)}…` : value || '(empty)') : 'Retained structured value';
 const scalar = (name: string, value: unknown) => {
  if (value !== undefined && result.length < 96) result.push([name,display(value)]);
 };
 const list = (name: string, value: unknown) => {
  if (!Array.isArray(value)) { scalar(name,value); return; }
  if (!value.length) { scalar(name,'(empty list)'); return; }
  const shown = value.slice(0,20).map(entry=>display(entry,64)).join(' · ');
  scalar(name,shown + (value.length > 20 ? ` · … (${value.length-20} more retained)` : ''));
 };
 const field = (parent: Record<string,unknown>, key: string, label: string) => { if(own(parent,key)) scalar(label,parent[key]); };
 const release = (parent: Record<string,unknown>, prefix: string) => {
  field(parent,'release_date',`${prefix}release date`);field(parent,'release_date_precision',`${prefix}date precision`);
  if(own(parent,'date')) {
   const date=parent.date;
   const wire = prefix.includes('(Web Player)') ? '' : ' (Web Player)';
   const dateLabel = prefix ? `${prefix}release date${wire}` : `Release date${wire}`;
   const yearLabel = prefix ? `${prefix}release year${wire}` : `Release year${wire}`;
   if(object(date)) {
    // A present null ISO date remains unknown; a year is never expanded into an invented month/day.
    if(own(date,'isoString')) scalar(dateLabel,date.isoString);
    else field(date,'year',yearLabel);
   } else scalar(dateLabel,date);
  }
 };
 const artistList = (value: unknown, prefix: string) => {
  const entries=Array.isArray(value)?value:object(value)&&Array.isArray(value.items)?value.items:undefined;
  if(!entries) { scalar(prefix,value);return; }
  if(!entries.length) { scalar(prefix,'(empty list)');return; }
  entries.slice(0,10).forEach((artist,index)=>{
   const name=`${prefix} ${index+1}`;
   if(!object(artist)) { scalar(name,artist);return; }
   if(own(artist,'name')) scalar(name,artist.name);
   else if(object(artist.profile)&&own(artist.profile,'name')) scalar(name,artist.profile.name);
   else if(own(artist,'profile')&&artist.profile===null) scalar(name,null);
   else if(own(artist,'id')) scalar(name,artist.id);
   else if(own(artist,'uri')) scalar(name,artist.uri);
   else scalar(name,null);
  });
  if(entries.length>10) scalar(`${prefix} display limit`,`${entries.length-10} more retained`);
 };
 for (const [key,label] of Object.entries({name:'Name',id:'ID',uri:'Spotify URI',_uri:'Spotify URI (alternate)',type:'Type',__typename:'Provider type',description:'Description',release_date:'Release date',release_date_precision:'Date precision',duration_ms:'Duration (milliseconds)',track_number:'Track number',disc_number:'Disc number',explicit:'Explicit',is_playable:'Playable when captured',popularity:'Popularity',total_tracks:'Track count',snapshot_id:'Playlist revision',label:'Label'})) field(payload,key,label);
 for(const key of ['genres','tags']) if(own(payload,key)) list(key==='genres'?'Genres':'Tags',payload[key]);
 // Provider aliases are displayed separately so conflicting retained facts are not silently merged.
 if(own(payload,'date')) release({date:payload.date},'');
 for(const key of ['duration','trackDuration']) if(own(payload,key)) {
  const value=payload[key];
  if(object(value)) field(value,'totalMilliseconds',`Duration (milliseconds, ${key})`);
  else scalar(`Duration (${key})`,value);
 }
 if(own(payload,'contentRating')) {
  const rating=payload.contentRating;
  if(object(rating)) field(rating,'label','Content rating (Web Player)');else scalar('Content rating (Web Player)',rating);
 }
 if(own(payload,'playability')) {
  const playability=payload.playability;
  if(object(playability)) field(playability,'playable','Playable when captured (Web Player)');else scalar('Playable when captured (Web Player)',playability);
 }
 for(const [key,prefix] of [['album','Album '],['albumOfTrack','Album (Web Player) '],['owner','Owner '],['profile','Profile ']] as const) {
  if(!own(payload,key)) continue;
  const nested=payload[key];
  if(!object(nested)) { scalar(prefix.trim(),nested);continue; }
  for(const name of ['name','id','display_name']) field(nested,name,`${prefix}${name.replaceAll('_',' ')}`);
  if(key==='album'||key==='albumOfTrack') {
   release(nested,prefix);
   for(const tag of ['genres','tags']) if(own(nested,tag)) list(`${prefix}${tag}`,nested[tag]);
   if(own(nested,'artists')) artistList(nested.artists,`${prefix}artist`);
  }
 }
 if(own(payload,'ownerV2')) {
  const owner=payload.ownerV2;
  if(object(owner)&&object(owner.data)) for(const key of ['name','uri']) field(owner.data,key,`Owner (Web Player) ${key}`);
  else scalar('Owner (Web Player)',object(owner)?owner.data:owner);
 }
 for(const [key,label] of [['artists','Artist'],['firstArtist','Primary artist'],['otherArtists','Other artist']] as const) if(own(payload,key)) artistList(payload[key],label);
 return result;
}
