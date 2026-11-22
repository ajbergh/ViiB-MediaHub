import { useId, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { Loader2, X } from 'lucide-react';
import { useFocusTrap } from '../hooks/useFocusTrap';
import type { AlbumMetadataPatch, Song } from '../types';
import { Button } from './ui/Button';
import { VIIB } from './ui/tokens';

const fields = [
  { key: 'album', label: 'Album title', read: (song: Song) => song.album },
  { key: 'albumArtist', label: 'Album artist', read: (song: Song) => song.albumArtist ?? '' },
  { key: 'artist', label: 'Track artist', read: (song: Song) => song.artist },
  { key: 'year', label: 'Release year', read: (song: Song) => song.year ? String(song.year) : '' },
  { key: 'genre', label: 'Genres', read: (song: Song) => song.genre?.join(', ') ?? '' },
] as const;

type Field = (typeof fields)[number]['key'];

interface Props {
  album: string;
  artist: string;
  tracks: Song[];
  onClose: () => void;
  onSave: (ids: string[], patch: AlbumMetadataPatch) => Promise<void>;
}

export function AlbumMetadataDialog({ album, artist, tracks, onClose, onSave }: Props) {
  const id = useId();
  const [initial] = useState(() => fields.map(({ read }) => {
    const first = read(tracks[0]);
    return tracks.every(track => read(track) === first) ? first : null;
  }));
  const [changes, setChanges] = useState<Partial<Record<Field, string>>>({});
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);
  const pending = useRef(false);
  const close = () => { if (!pending.current) onClose(); };
  const dialogRef = useFocusTrap<HTMLDivElement>(true, close);

  const submit = async () => {
    if (pending.current || !Object.keys(changes).length) return;
    setError('');
    const patch: AlbumMetadataPatch = {};
    for (const { key, label } of fields) {
      const value = changes[key]?.trim();
      if (value === undefined) continue;
      if (key === 'year') {
        const year = Number(value);
        if (!Number.isInteger(year) || year < 1 || year > 3000) {
          setError('Enter a release year between 1 and 3000.');
          return;
        }
        patch.year = year;
      } else if (key === 'genre') {
        const genres = new Map<string, string>();
        for (const entry of value.split(',')) {
          const genre = entry.trim();
          if (genre && !genres.has(genre.toLowerCase())) genres.set(genre.toLowerCase(), genre);
        }
        patch.genre = [...genres.values()];
      } else {
        if (!value && key !== 'albumArtist') {
          setError(`${label} is required.`);
          return;
        }
        patch[key] = value;
      }
    }
    const albumArtists = new Set(tracks.map(track => (patch.albumArtist ?? track.albumArtist) || (patch.artist ?? track.artist)));
    if (albumArtists.size > 1) {
      setError('Set a shared album artist to keep these tracks together in one album.');
      return;
    }
    pending.current = true;
    setSaving(true);
    try {
      await onSave(tracks.map(track => track.id), patch);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to update album metadata.');
    } finally {
      pending.current = false;
      setSaving(false);
    }
  };

  return createPortal(
    <div
      className={`fixed inset-0 ${VIIB.zIndex.confirmDialog} flex items-center justify-center bg-black/75 p-4 backdrop-blur-sm`}
      onClick={event => { if (event.target === event.currentTarget) close(); }}
    >
      <div ref={dialogRef} tabIndex={-1} role="dialog" aria-modal="true" aria-labelledby={`${id}-title`} aria-describedby={`${id}-details`} className="flex max-h-[90dvh] w-full max-w-xl flex-col overflow-hidden rounded-2xl border border-surface-border bg-surface-1 shadow-2xl motion-safe:animate-scale-in">
        <div className="flex items-start justify-between gap-4 border-b border-surface-border p-5">
          <div className="min-w-0">
            <h2 id={`${id}-title`} className="text-section text-text-main">Edit album metadata</h2>
            <p id={`${id}-details`} className="mt-1 break-words text-sm text-text-secondary">{album} · {artist} · {tracks.length} {tracks.length === 1 ? 'track' : 'tracks'}</p>
          </div>
          <Button variant="ghost" size="icon" aria-label="Close album metadata editor" disabled={saving} onClick={close}><X size={18} /></Button>
        </div>
        <form className="flex min-h-0 flex-1 flex-col" onSubmit={event => { event.preventDefault(); void submit(); }}>
          <div className="min-h-0 overflow-y-auto p-5">
            <p className="mb-4 text-sm text-text-secondary">Select the fields to update on every track in this album. Changes are saved in the ViiB catalog.</p>
            <fieldset disabled={saving} className="space-y-4">
              {fields.map(({ key, label }, index) => {
                const saved = initial[index];
                const selected = changes[key] !== undefined;
                const inputId = `${id}-${key}`;
                return (
                  <div key={key}>
                    <div className="mb-1.5 flex items-center justify-between gap-2">
                      <label htmlFor={inputId} className="text-sm font-semibold text-text-main">{label}</label>
                      <label className="flex items-center gap-2 text-xs text-text-secondary">
                        <input
                          type="checkbox"
                          checked={selected}
                          aria-label={`Change ${label.toLowerCase()}`}
                          onChange={event => {
                            const enabled = event.target.checked;
                            setChanges(previous => {
                              const next = { ...previous };
                              if (enabled) next[key] = saved ?? '';
                              else delete next[key];
                              return next;
                            });
                            setError('');
                          }}
                          className="size-4 accent-brand"
                        />
                        Change
                      </label>
                    </div>
                    <input
                      id={inputId}
                      value={changes[key] ?? saved ?? ''}
                      disabled={!selected}
                      inputMode={key === 'year' ? 'numeric' : 'text'}
                      placeholder={!selected && saved === null ? 'Mixed across tracks' : key === 'genre' ? 'Comma-separated genres' : key === 'albumArtist' ? 'Use track artist when blank' : undefined}
                      onChange={event => {
                        const value = event.target.value;
                        setChanges(previous => ({ ...previous, [key]: value }));
                        setError('');
                      }}
                      className="h-10 w-full rounded-lg border border-surface-border bg-surface-0 px-3 text-sm text-text-main placeholder:text-text-subtle focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand disabled:bg-surface-2 disabled:text-text-secondary"
                    />
                  </div>
                );
              })}
            </fieldset>
            {error && <p role="alert" className="mt-4 text-sm text-error">{error}</p>}
          </div>
          <div className="flex flex-wrap items-center justify-end gap-2 border-t border-surface-border p-4">
            <Button variant="ghost" onClick={close} disabled={saving}>Cancel</Button>
            <Button type="submit" variant="primary" disabled={saving || !Object.keys(changes).length}>
              {saving && <Loader2 size={16} className="motion-safe:animate-spin" aria-hidden="true" />}
              {saving ? 'Saving changes…' : `Apply to ${tracks.length} ${tracks.length === 1 ? 'track' : 'tracks'}`}
            </Button>
          </div>
        </form>
      </div>
    </div>,
    document.body,
  );
}
