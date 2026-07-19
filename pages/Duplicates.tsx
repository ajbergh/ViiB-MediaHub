import React, { useCallback, useEffect, useState } from 'react';
import { Copy, Eye, EyeOff, RefreshCw } from 'lucide-react';
import { api, DuplicateGroup } from '../services/api';
import { useStore } from '../store';
import { Page, PageHeader } from '../components/ui/Page';
import { Button } from '../components/ui/Button';

export const Duplicates: React.FC = () => {
  const refreshLibrary = useStore(state => state.refreshLibrary);
  const [groups, setGroups] = useState<DuplicateGroup[]>([]);
  const [ignored, setIgnored] = useState<DuplicateGroup['songs']>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [duplicateGroups, ignoredSongs] = await Promise.all([
        api.getDuplicateGroups(),
        api.getIgnoredSongs(),
      ]);
      setGroups(duplicateGroups);
      setIgnored(ignoredSongs);
    } catch (loadError) {
      setError(loadError instanceof Error ? loadError.message : 'Failed to load duplicate information');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void load(); }, [load]);

  const setIgnoredState = async (songId: string, value: boolean) => {
    await api.setDuplicateIgnored(songId, value);
    await Promise.all([load(), refreshLibrary()]);
  };

  return (
    <Page>
      <PageHeader
        heading="Duplicate Manager"
        actions={<Button variant="secondary" onClick={() => void load()}><RefreshCw size={16} /> Refresh</Button>}
      />
      <p className="mb-6 text-sm text-text-secondary">
        Hide redundant library copies without deleting source files or losing their persisted metadata.
      </p>

      {error ? (
        <div className="rounded-xl border border-error/30 bg-error/10 p-4 text-sm text-error">{error}</div>
      ) : loading ? (
        <div className="text-text-secondary">Scanning duplicate fingerprints…</div>
      ) : groups.length === 0 ? (
        <div className="rounded-xl border border-surface-border bg-surface-2 p-8 text-center">
          <Copy className="mx-auto mb-3 text-brand" size={36} />
          <h2 className="font-semibold text-text-main">No active duplicates found</h2>
          <p className="mt-2 text-sm text-text-secondary">Copies are grouped by their stable media fingerprint.</p>
        </div>
      ) : (
        <div className="space-y-6">
          {groups.map(group => (
            <section key={group.fileHash} className="rounded-xl border border-surface-border bg-surface-2 p-5">
              <h2 className="mb-4 font-semibold text-text-main">{group.songs.length} identical copies</h2>
              <div className="space-y-3">
                {group.songs.map((song, index) => (
                  <div key={song.id} className="flex items-center gap-4 rounded-lg bg-surface-1 p-3">
                    <div className="min-w-0 flex-1">
                      <div className="truncate font-medium text-text-main">{song.title}</div>
                      <div className="truncate text-sm text-text-secondary">{song.artist} · {song.album}</div>
                      <div className="truncate text-xs text-text-subtle" title={song.filePath}>{song.filePath}</div>
                    </div>
                    {index === 0 ? (
                      <span className="rounded-full bg-brand/15 px-3 py-1 text-xs font-semibold text-brand">Suggested keep</span>
                    ) : (
                      <Button variant="secondary" onClick={() => void setIgnoredState(song.id, true)}>
                        <EyeOff size={15} /> Hide copy
                      </Button>
                    )}
                  </div>
                ))}
              </div>
            </section>
          ))}
        </div>
      )}

      {ignored.length > 0 && (
        <section className="mt-8 rounded-xl border border-surface-border bg-surface-2 p-5">
          <h2 className="mb-4 font-semibold text-text-main">Hidden duplicate copies</h2>
          <div className="space-y-3">
            {ignored.map(song => (
              <div key={song.id} className="flex items-center gap-4 rounded-lg bg-surface-1 p-3">
                <div className="min-w-0 flex-1">
                  <div className="truncate font-medium text-text-main">{song.title}</div>
                  <div className="truncate text-sm text-text-secondary">{song.artist} · {song.album}</div>
                </div>
                <Button variant="secondary" onClick={() => void setIgnoredState(song.id, false)}>
                  <Eye size={15} /> Restore
                </Button>
              </div>
            ))}
          </div>
        </section>
      )}
    </Page>
  );
};
