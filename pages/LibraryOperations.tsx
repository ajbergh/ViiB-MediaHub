/**
 * LibraryOperations is the local maintenance UI for diagnostics, database
 * repair, validated backup/restore staging, and continuous monitoring.
 * Restore activation stays outside the running app and is performed by
 * viib-restore after ViiB has exited.
 */
import React, { useCallback, useEffect, useState } from 'react';
import { Activity, Archive, CheckCircle2, Database, Eye, Play, RefreshCw, ShieldCheck, Square, Wrench } from 'lucide-react';
import { Page, PageHeader } from '../components/ui/Page';
import { BackupInfo, LibraryDiagnostics, WatcherStatus, libraryOperationsV2 } from '../services/libraryOperationsV2';
import { MetadataHealthWidget } from '../components/MetadataHealthWidget';

const formatBytes = (bytes: number) => {
  if (bytes < 1024) return `${bytes} B`;
  const units = ['KB', 'MB', 'GB', 'TB'];
  let value = bytes / 1024;
  let index = 0;
  while (value >= 1024 && index < units.length - 1) { value /= 1024; index += 1; }
  return `${value.toFixed(value >= 10 ? 1 : 2)} ${units[index]}`;
};

const actionClass = 'inline-flex items-center gap-2 rounded-lg bg-brand px-4 py-2 text-sm font-semibold text-black hover:bg-brand-hover disabled:cursor-not-allowed disabled:opacity-50';
const secondaryClass = 'inline-flex items-center gap-2 rounded-lg bg-surface-2 px-4 py-2 text-sm font-semibold text-text-main hover:bg-surface-3 disabled:cursor-not-allowed disabled:opacity-50';

export const LibraryOperations: React.FC = () => {
  const [diagnostics, setDiagnostics] = useState<LibraryDiagnostics | null>(null);
  const [backups, setBackups] = useState<BackupInfo[]>([]);
  const [watcher, setWatcher] = useState<WatcherStatus>({ running: false, intervalMs: 15000, lastChanges: 0, checks: 0 });
  const [busy, setBusy] = useState<string | null>(null);
  const [message, setMessage] = useState('');
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    const controller = new AbortController();
    try {
      const [backupResult, watcherResult] = await Promise.all([
        libraryOperationsV2.listBackups(controller.signal),
        libraryOperationsV2.watcherStatus(controller.signal),
      ]);
      setBackups(backupResult.backups);
      setWatcher(watcherResult);
    } catch (loadError) {
      setError(loadError instanceof Error ? loadError.message : 'Unable to load library operations');
    }
    return () => controller.abort();
  }, []);

  useEffect(() => { void load(); }, [load]);

  // run serializes user-triggered maintenance actions so an expensive repair,
  // backup, or watcher transition cannot overlap another operation in this UI.
  const run = async (label: string, operation: () => Promise<void>) => {
    setBusy(label); setError(''); setMessage('');
    try { await operation(); }
    catch (operationError) { setError(operationError instanceof Error ? operationError.message : `${label} failed`); }
    finally { setBusy(null); }
  };

  const runDiagnostics = () => run('diagnostics', async () => {
    const result = await libraryOperationsV2.diagnostics();
    setDiagnostics(result);
    setMessage(`Diagnostics completed. Integrity: ${result.integrity}.`);
  });

  const repair = (removeMissing: boolean) => run('repair', async () => {
    const result = await libraryOperationsV2.repair(removeMissing);
    setMessage(`Repair complete: ${result.removedMissing || 0} missing tracks removed and ${result.removedPlaylistReferences || 0} broken playlist references repaired.`);
    setDiagnostics(await libraryOperationsV2.diagnostics());
  });

  const createBackup = () => run('backup', async () => {
    const created = await libraryOperationsV2.createBackup();
    setMessage(`Backup ${created.name} created and validated.`);
    setBackups((await libraryOperationsV2.listBackups()).backups);
  });

  const previewRestore = (backup: BackupInfo) => run(`preview-${backup.name}`, async () => {
    const preview = await libraryOperationsV2.previewRestore(backup.name);
    setMessage(`Backup ${backup.name} is valid. Restore requires an application restart.`);
    if (!preview.valid) throw new Error('Backup validation did not complete');
  });

  const stageRestore = (backup: BackupInfo) => run(`restore-${backup.name}`, async () => {
    const confirmed = window.confirm(`Stage ${backup.name} for restore? The current library database will be replaced after applying the staged restore during maintenance.`);
    if (!confirmed) return;
    const result = await libraryOperationsV2.stageRestore(backup.name);
    setMessage(`Restore staged in ${result.path}. Close ViiB before applying the staged database. A restart is required.`);
  });

  const toggleWatcher = () => run('watcher', async () => {
    const status = watcher.running
      ? await libraryOperationsV2.stopWatcher()
      : await libraryOperationsV2.startWatcher(watcher.intervalMs || 15000);
    setWatcher(status);
    setMessage(status.running ? 'Continuous library monitoring started.' : 'Continuous library monitoring stopped.');
  });

  return (
    <Page withPlayerPadding={false}>
      <PageHeader heading="Library Operations" />
      <p className="mb-6 max-w-3xl text-text-secondary">Diagnose and repair library consistency, create validated backups, stage recovery, and continuously monitor configured folders.</p>

      {(message || error) && (
        <div className={`mb-6 rounded-lg border p-4 text-sm ${error ? 'border-error/40 bg-error/10 text-error' : 'border-accent-green/30 bg-accent-green/10 text-text-main'}`} role="status">
          {error || message}
        </div>
      )}

      {/* Metadata Health Dashboard */}
      <div className="mb-6">
        <MetadataHealthWidget />
      </div>

      <div className="grid gap-6 xl:grid-cols-2">
        <section className="rounded-xl border border-surface-highlight bg-surface-1 p-5">
          <div className="mb-4 flex items-center gap-3"><ShieldCheck className="text-brand" /><h2 className="text-xl font-semibold">Diagnostics & Repair</h2></div>
          <div className="mb-4 flex flex-wrap gap-3">
            <button className={actionClass} disabled={busy !== null} onClick={runDiagnostics}><Activity size={17} />Run diagnostics</button>
            <button className={secondaryClass} disabled={busy !== null} onClick={() => repair(false)}><Wrench size={17} />Repair indexes</button>
            <button className={secondaryClass} disabled={busy !== null} onClick={() => repair(true)}><RefreshCw size={17} />Remove missing files</button>
          </div>
          {diagnostics ? (
            <dl className="grid grid-cols-2 gap-3 text-sm">
              <div className="rounded-lg bg-surface-2 p-3"><dt className="text-text-secondary">Database integrity</dt><dd className="mt-1 font-semibold">{diagnostics.integrity}</dd></div>
              <div className="rounded-lg bg-surface-2 p-3"><dt className="text-text-secondary">Songs / search rows</dt><dd className="mt-1 font-semibold">{diagnostics.songCount} / {diagnostics.searchIndexCount}</dd></div>
              <div className="rounded-lg bg-surface-2 p-3"><dt className="text-text-secondary">Missing media</dt><dd className="mt-1 font-semibold">{diagnostics.missingMedia.length}</dd></div>
              <div className="rounded-lg bg-surface-2 p-3"><dt className="text-text-secondary">Broken playlist refs</dt><dd className="mt-1 font-semibold">{diagnostics.brokenPlaylistReferences.length}</dd></div>
              <div className="rounded-lg bg-surface-2 p-3"><dt className="text-text-secondary">Quarantined media</dt><dd className="mt-1 font-semibold">{diagnostics.scannerFailures.length}</dd></div>
              <div className="rounded-lg bg-surface-2 p-3"><dt className="text-text-secondary">Library revision</dt><dd className="mt-1 font-semibold">{diagnostics.revision}</dd></div>
            </dl>
          ) : <p className="text-sm text-text-secondary">Run diagnostics to inspect database integrity, file availability, playlists, search indexes, and scanner quarantine.</p>}
        </section>

        <section className="rounded-xl border border-surface-highlight bg-surface-1 p-5">
          <div className="mb-4 flex items-center gap-3"><Archive className="text-brand" /><h2 className="text-xl font-semibold">Backup & Recovery</h2></div>
          <button className={actionClass} disabled={busy !== null} onClick={createBackup}><Database size={17} />Create validated backup</button>
          <div className="mt-4 max-h-72 space-y-2 overflow-y-auto">
            {backups.length === 0 && <p className="text-sm text-text-secondary">No backups have been created.</p>}
            {backups.map(backup => (
              <div key={backup.name} className="flex flex-wrap items-center justify-between gap-3 rounded-lg bg-surface-2 p-3">
                <div className="min-w-0"><div className="truncate font-medium">{backup.name}</div><div className="text-xs text-text-secondary">{formatBytes(backup.size)} · {new Date(backup.createdAt).toLocaleString()}</div></div>
                <div className="flex gap-2">
                  <button className={secondaryClass} disabled={busy !== null} onClick={() => previewRestore(backup)}><Eye size={15} />Validate</button>
                  <button className={secondaryClass} disabled={busy !== null} onClick={() => stageRestore(backup)}><CheckCircle2 size={15} />Stage</button>
                </div>
              </div>
            ))}
          </div>
        </section>

        <section className="rounded-xl border border-surface-highlight bg-surface-1 p-5 xl:col-span-2">
          <div className="mb-4 flex items-center gap-3"><Activity className="text-brand" /><h2 className="text-xl font-semibold">Continuous Monitoring</h2></div>
          <div className="flex flex-wrap items-center gap-4">
            <label className="text-sm text-text-secondary">Interval
              <select className="ml-2 rounded-lg bg-surface-2 px-3 py-2 text-text-main" value={watcher.intervalMs || 15000} disabled={watcher.running} onChange={event => setWatcher(current => ({ ...current, intervalMs: Number(event.target.value) }))}>
                <option value={5000}>5 seconds</option><option value={15000}>15 seconds</option><option value={30000}>30 seconds</option><option value={60000}>1 minute</option><option value={300000}>5 minutes</option>
              </select>
            </label>
            <button className={watcher.running ? secondaryClass : actionClass} disabled={busy !== null} onClick={toggleWatcher}>
              {watcher.running ? <><Square size={16} />Stop watcher</> : <><Play size={16} />Start watcher</>}
            </button>
            <span className="text-sm text-text-secondary">Checks: {watcher.checks} · Last changes: {watcher.lastChanges}{watcher.lastError ? ` · Error: ${watcher.lastError}` : ''}</span>
          </div>
        </section>
      </div>
    </Page>
  );
};
