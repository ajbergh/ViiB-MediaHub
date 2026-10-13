import { useEffect, useState } from 'react';
import { loadImportedAnalysis, type ImportedAnalysisKind, type ImportedAnalysisPage } from '../services/importedAnalysis';

export function ImportedAnalysisPreview({ songId, fingerprint }: { songId: string; fingerprint: string }) {
 const [open,setOpen] = useState(false);
 const [kind,setKind] = useState<ImportedAnalysisKind>('beats');
 const [offset,setOffset] = useState(0);
 const identity = `${songId}:${fingerprint}:${kind}:${offset}`;
 const [snapshot,setSnapshot] = useState<{identity:string;data:ImportedAnalysisPage|null;error?:boolean}>();
 useEffect(()=>{
  let active=true;
  if(open) {
   setSnapshot(undefined);
   void loadImportedAnalysis(songId,kind,offset).then(data=>{
   if(active) setSnapshot({identity,data:data?.sourceFingerprint===fingerprint?data:null});
  },()=>{if(active) setSnapshot({identity,data:null,error:true});});
  }
  return ()=>{active=false;};
 },[open,songId,fingerprint,kind,offset,identity]);
 const current=snapshot?.identity===identity?snapshot:undefined;
 const data=current?.data;
 const number=(value:unknown)=>typeof value==='number'&&Number.isFinite(value)?value.toFixed(2):'Unknown';
 return <details onToggle={event=>{if(event.target===event.currentTarget)setOpen(event.currentTarget.open);}} className="rounded-lg border border-surface-border p-3">
  <summary>Retained Spotify timing and segments</summary>
  <p className="mt-2">Times use the Spotify recording timeline; duration compatibility does not establish beat alignment. Review the local grid before mixing.</p>
  <label className="block mt-2">Detailed array <select value={kind} onChange={event=>{setKind(event.target.value as ImportedAnalysisKind);setOffset(0);}} className="bg-surface-2 rounded p-1">
   {(['bars','beats','tatums','sections','segments'] as const).map(value=><option key={value} value={value}>{value}</option>)}
  </select></label>
  {open && (!current ? <p role="status">Loading retained analysis...</p> : !data ? <p role="status">{current.error?'Retained analysis could not be loaded.':'This array was not retained for the current file.'}</p> : <>
   <p>{data.provenance==='spotify_durable_import'?'Retained with this downloaded file.':'Cached Spotify observation for the current recording.'}{data.unverified?' Account confirmation pending; viewing only.':''}</p>
   <p>{data.totalItems} {kind}{data.stale?' - Stale observation':''} - Retrieved {new Date(data.retrievedAt).toLocaleString()}</p>
   <div className="overflow-x-auto"><table className="w-full text-left border-separate border-spacing-x-2"><caption>{kind} on the recording timeline</caption><thead><tr><th>Start (s)</th><th>Duration (s)</th><th>Confidence</th>{(kind==='segments'||kind==='sections')&&<th>Observation</th>}</tr></thead><tbody>
    {data.items.map((item,index)=><tr key={offset+index} className="align-top"><td>{number(item.start)}</td><td>{number(item.duration)}</td><td>{typeof item.confidence==='number'&&Number.isFinite(item.confidence)?`${Math.round(item.confidence*100)}%`:'Unknown'}</td>{(kind==='segments'||kind==='sections')&&<td><details><summary>Inspect {kind==='segments'?'segment':'section'} {offset+index+1}</summary>
      <dl>{(kind==='sections'?['tempo','tempo_confidence','key','key_confidence','mode','mode_confidence','time_signature','time_signature_confidence','loudness']:['loudness_start','loudness_max','loudness_max_time','loudness_end']).map(field=><div key={field}><dt>{field.replaceAll('_',' ')}</dt><dd>{number(item[field])}</dd></div>)}</dl>
      {kind==='sections'&&<p>Tempo in BPM; loudness in provider dB. Key codes 0-11 mean C-B; mode 0 is minor and 1 is major. Time signature is beats per bar, without an inferred denominator. Confidence uses 0-1.</p>}
      {kind==='segments'&&<p>Loudness uses provider dB; loudness max time uses seconds within this segment.</p>}
      {kind==='segments'&&<><p>Pitch classes C through B: {item.pitches?.map(value=>number(value)).join(', ')??'Unknown'}</p><p>Timbre coefficients 1-12: {item.timbre?.map(value=>number(value)).join(', ')??'Unknown'}</p><p>Provider coefficients describe this segment; they are not local key or mood estimates.</p></>}
     </details></td>}</tr>)}
   </tbody></table></div>
   <div className="flex items-center gap-2 mt-2"><button disabled={offset===0} onClick={()=>setOffset(Math.max(0,offset-25))}>Previous</button><span>{data.totalItems?offset+1:0}-{offset+data.items.length} of {data.totalItems}</span><button disabled={offset+data.items.length>=data.totalItems} onClick={()=>setOffset(offset+25)}>Next</button></div>
  </>)}
 </details>;
}
