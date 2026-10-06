import { useEffect, useState } from 'react';
import { useStore } from '../../../store';

export function DJSaveResultsPlaylist({ songIds }: { songIds: string[] }) {
 const spotifySessionGeneration = useStore(state => state.spotifySessionGeneration);
 const createPlaylist = useStore(state => state.createPlaylist);
 const [selection,setSelection] = useState<{ songIds: string[]; generation: number } | null>(null);
 const currentSelection = selection?.generation === spotifySessionGeneration ? selection.songIds : null;
 const [name,setName] = useState('');
 const [busy,setBusy] = useState(false);
 const [message,setMessage] = useState('');
 const [error,setError] = useState('');
 useEffect(() => {
  setSelection(null);
  setName('');
  setBusy(false);
  setMessage('');
  setError('');
 }, [spotifySessionGeneration]);
 const save = async () => {
  if (!currentSelection?.length || !name.trim() || busy) return;
  const generation = useStore.getState().spotifySessionGeneration;
  if (selection?.generation !== generation) return;
  setBusy(true); setError('');
  try {
   await createPlaylist(name.trim(),[...currentSelection]);
   if (useStore.getState().spotifySessionGeneration !== generation) return;
   setMessage(`Saved ${currentSelection.length} tracks as “${name.trim()}”.`);
   setSelection(null); setName('');
  } catch {
   if (useStore.getState().spotifySessionGeneration === generation) setError('Playlist could not be saved. Your selection is retained; try again.');
  } finally {
   if (useStore.getState().spotifySessionGeneration === generation) setBusy(false);
  }
 };
 return <div className="px-3 py-2 text-xs text-[var(--dj-text-secondary)] border-b border-white/10">
  {currentSelection ? <form className="flex flex-wrap items-center gap-2" onSubmit={e => { e.preventDefault(); void save(); }}>
    <span>{currentSelection.length} tracks in the order captured when you opened this form.</span>
    <label>Playlist name <input autoFocus maxLength={200} value={name} onChange={e => setName(e.target.value)} disabled={busy} className="bg-surface-2 rounded p-1" /></label>
    <button className="dj-btn dj-btn-xs" type="submit" disabled={busy || !name.trim()}>{busy ? 'Saving…' : 'Create playlist'}</button>
    <button className="dj-btn dj-btn-xs" type="button" disabled={busy} onClick={() => { setSelection(null); setError(''); }}>Cancel</button>
  </form> : <button type="button" className="dj-btn dj-btn-xs" disabled={!songIds.length} onClick={() => { setSelection({ songIds: [...songIds], generation: spotifySessionGeneration }); setMessage(''); setError(''); }}>Save {songIds.length} results as playlist</button>}
  {message && <p role="status">{message}</p>}
  {error && <p role="alert">{error}</p>}
 </div>;
}
