import { useEffect, useRef, useState } from 'react';
import { useStore } from '../store';
import { useAudioMetadataRevision } from '../hooks/useAudioMetadataRevision';
import { catalogFieldInventory, loadImportedCatalog, type ImportedCatalog } from '../services/importedCatalog';

const reasons: Record<string,string> = {
 direct_track_bundle_retained: 'Track and directly related material retained', track_album_artist_graph_retained: 'Reachable track, album and artist material retained',
 requested_observations_retained: 'Requested collection observations retained', requested_collection_unavailable: 'Some requested collection evidence was missing or could not be verified', no_requested_collections: 'No active playlist or library request origins', origin_limit: 'Request origin limit exceeded',
 snapshot_limit: 'Catalog snapshot limit exceeded', relation_limit: 'Catalog relation limit exceeded', no_eligible_catalog: 'No eligible catalog material at completion',
 root_track_unavailable: 'Track snapshot unavailable at completion', related_entity_unavailable: 'Related album or artist material missing',
};
const time = (value: string) => new Date(value).toLocaleString();
function Pages({ page, count, onPage, label }: { page: number; count: number; onPage: (page:number)=>void; label:string }) {
 return <div className="flex gap-3 items-center"><button className="rounded border border-surface-border px-2 py-1 disabled:opacity-50" type="button" disabled={page===0} onClick={()=>onPage(page-1)}>Previous {label}</button><span>{count ? `${page*25+1}–${Math.min(count,(page+1)*25)} of ${count}` : '0 entries'}</span><button className="rounded border border-surface-border px-2 py-1 disabled:opacity-50" type="button" disabled={(page+1)*25>=count} onClick={()=>onPage(page+1)}>Next {label}</button></div>;
}

export function SpotifyCatalogEvidence({ songId, fingerprint }: { songId:string; fingerprint?:string }) {
 const session=useStore(s=>s.spotifySessionGeneration);
 const {revision:metadataRevision,generation}=useAudioMetadataRevision(songId);
 const [open,setOpen]=useState(false),[retry,setRetry]=useState(0);
 const [snapshot,setSnapshot]=useState<{identity:string;data:ImportedCatalog|null;error?:boolean}>();
 const [entity,setEntity]=useState(0),[relations,setRelations]=useState(0),[origins,setOrigins]=useState(0);
 const identity=JSON.stringify([songId,fingerprint,session,retry,metadataRevision]);
 const live=useRef(identity);live.current=identity;
 useEffect(()=>{
  if(!open)return;
  const controller=new AbortController();let active=true;const expectedGeneration=generation.current;
  setSnapshot(undefined);setEntity(0);setRelations(0);setOrigins(0);
  void loadImportedCatalog(songId,fingerprint,controller.signal).then(data=>{
   if(active && live.current===identity && generation.current===expectedGeneration)setSnapshot({identity,data});
  },()=>{if(active && live.current===identity && generation.current===expectedGeneration)setSnapshot({identity,data:null,error:true});});
  return ()=>{active=false;controller.abort();};
 },[open,identity,songId,fingerprint,generation]);
 const current=snapshot?.identity===identity?snapshot:undefined;
 const data=current?.data;
 const selected=data?.snapshots[entity];
 const resourceLabels=new Map(data?.snapshots.map(s=>[JSON.stringify([s.entityType,s.spotifyId,s.resource]),`${s.capturedResource??s.resource}${s.captureRevision?` · Revision ${s.captureRevision}`:''}`]));
 return <details className="rounded-lg border border-surface-border p-3 space-y-3" onToggle={event=>{if(event.target===event.currentTarget && event.currentTarget.open)setOpen(true);}}>
  <summary className="cursor-pointer font-semibold text-text-main">Downloaded Spotify catalog</summary>
  <p>Retained with the current file and available offline. These observations do not establish current playlist membership or playability.</p>
  {open && !current && <p role="status">Loading retained catalog…</p>}
  {current && !data && <><p role="status">{current.error?'Retained catalog could not be loaded.':'No retained catalog for the current file.'}</p><button className="rounded border border-surface-border px-2 py-1 disabled:opacity-50" type="button" onClick={()=>setRetry(n=>n+1)}>Retry retained catalog</button></>}
  {data && <>
   <p>Spotify recording {data.recordingId}</p>
   <p>Latest completion: {data.catalogStatus ? `${data.catalogStatus.state.replaceAll('_',' ')} · ${reasons[data.catalogStatus.reason] ?? 'Outcome recorded'} · ${time(data.catalogStatus.checkedAt)}` : 'Outcome not recorded for this earlier import.'}</p>
   {data.catalogStatus && <p>Capture scope: {data.catalogStatus.scope==='track_album_artist_graph_v2'?'Reachable track, album and artist graph':'Track and directly related entities'}.</p>}
   {(data.catalogStatus?.state==='oversized' || data.catalogStatus?.state==='incomplete') && <p>Retained last-good entries remain available below. This outcome does not confirm a complete catalog.</p>}
   <p>Requested collection capture: {data.collectionStatus?`${data.collectionStatus.state.replaceAll('_',' ')} · ${reasons[data.collectionStatus.reason]??'Outcome recorded'} · ${time(data.collectionStatus.checkedAt)}`:'Outcome not recorded for this earlier import.'}</p>
   <p>Collection evidence covers captured playlist pages and selected saved-library pages at retrieval. It does not establish current membership or a complete saved library.</p>
   {(data.collectionStatus?.state==='oversized'||data.collectionStatus?.state==='incomplete')&&<p>Earlier retained collection observations remain available below.</p>}
   <button className="rounded border border-surface-border px-2 py-1 disabled:opacity-50" type="button" onClick={()=>setRetry(n=>n+1)}>Reload retained catalog</button>
   <h5 className="font-semibold">Catalog field inventory</h5>
   {data.snapshots.length ? <><label>Retained entity <select className="block w-full rounded border border-surface-border bg-surface-1 p-2 text-text-main" value={entity} onChange={event=>setEntity(Number(event.target.value))}>{data.snapshots.map((s,i)=><option className="bg-surface-1 text-text-main" key={`${s.entityType}:${s.spotifyId}:${s.resource}:${i}`} value={i}>{s.entityType} · {s.spotifyId} · {s.capturedResource??s.resource}{s.captureRevision?` · Revision ${s.captureRevision}`:''}{s.capturedResource?` · ${time(s.retrievedAt)}`:''}</option>)}</select></label>
    {selected && <>{selected.capturedResource&&<p className="[overflow-wrap:anywhere]">Original captured resource: {selected.capturedResource}{selected.captureRevision?` · Observed playlist revision ${selected.captureRevision}`:''}</p>}<p>{selected.stale?'Stale observation':'Fresh when inspected'} · Retrieved {time(selected.retrievedAt)} · Fresh until {time(selected.expiresAt)}</p>
     <dl className="grid grid-cols-1 sm:grid-cols-2 gap-2 break-words">{catalogFieldInventory(selected.payload).map(([label,value],i)=><div key={`${label}:${i}`}><dt className="text-text-subtle">{label}</dt><dd>{value}</dd></div>)}</dl>
     <p>Display inventory includes identity, names, release, duration, numbering, explicit/playability flags, popularity, genres, tags and selected collection fields when present. Additional domain fields may be retained without display; missing values are unknown.</p>
    </>}
   </>:<p>No catalog snapshots retained in this import.</p>}
   <h5 className="font-semibold">Ordered catalog relations</h5>
   <div className="overflow-x-auto" role="region" aria-label="Retained catalog relations" tabIndex={0}><table className="w-full text-left text-sm"><thead><tr><th className="px-3 py-2">Parent / resource</th><th className="px-3 py-2">Relation</th><th className="px-3 py-2">Position (zero-based)</th><th className="px-3 py-2">Child</th></tr></thead><tbody>{data.relations.slice(relations*25,(relations+1)*25).map((r,i)=><tr key={`${r.parentType}:${r.parentId}:${r.resource}:${r.kind}:${r.position}:${i}`}><td className="px-3 py-2 align-top whitespace-nowrap">{r.parentType} {r.parentId} · {resourceLabels.get(JSON.stringify([r.parentType,r.parentId,r.resource]))??r.resource}</td><td className="px-3 py-2 align-top whitespace-nowrap">{r.kind}</td><td className="px-3 py-2 align-top whitespace-nowrap">{r.position}</td><td className="px-3 py-2 align-top whitespace-nowrap">{r.unavailable?'Unavailable entry':r.childType} {r.childId || 'Unknown identity'}</td></tr>)}</tbody></table></div>
   <Pages page={relations} count={data.relations.length} onPage={setRelations} label="relations"/>
   <h5 className="font-semibold">Download request origins</h5>
   <p>{data.lineageStatus?.state==='available'?'Explicit request origins retained.':data.lineageStatus?.state==='oversized'?'Request-origin limit exceeded; prior retained origins remain.':data.lineageStatus?.state==='no_active_request_lineage'?'No active request origins retained for the latest completion. Earlier origins may remain.':'Origin outcome not recorded for this earlier import.'}</p>
   <ul>{data.origins.slice(origins*25,(origins+1)*25).map((o,i)=><li key={`${o.kind}:${o.id}:${o.revision}:${o.position}:${o.entityId}:${i}`}>{o.kind} · {o.id.replaceAll('_',' ')}{o.entityId?` · Collection ${o.entityId}`:''} · {o.position<0?'Position unknown':`Position ${o.position} (zero-based)`}{o.revision?` · Revision ${o.revision}`:''}</li>)}</ul>
   <Pages page={origins} count={data.origins.length} onPage={setOrigins} label="origins"/>
  </>}
 </details>;
}
