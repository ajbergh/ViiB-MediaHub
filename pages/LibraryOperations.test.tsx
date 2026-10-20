// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
const service = vi.hoisted(() => ({ listBackups: vi.fn(), diagnostics: vi.fn(), repair: vi.fn(), stageRestore: vi.fn(), createBackup: vi.fn() }));
vi.mock('../services/libraryOperationsV2', () => ({ libraryOperationsV2: service }));
vi.mock('../components/MetadataHealthWidget', () => ({ MetadataHealthWidget: () => null }));
vi.mock('../components/LibraryAnalysisPanel', () => ({ LibraryAnalysisPanel: () => null }));
import { LibraryOperationsPanel } from './LibraryOperations';

let container: HTMLDivElement;
let root: ReturnType<typeof createRoot>;
const click = async (name: string) => {
  const button = [...container.querySelectorAll('button')].find(item => item.textContent?.trim() === name);
  if (!button) throw new Error(`Missing action: ${name}`);
  await act(async () => button.click());
};
beforeEach(async () => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
  vi.resetAllMocks();
  service.listBackups.mockResolvedValue({ backups: [{ name: 'fixture.zip', size: 1024, createdAt: 1 }] });
  service.diagnostics.mockResolvedValue({ integrity: 'ok', songCount: 3, searchIndexCount: 3, revision: 1,
    missingMedia: [{ songId: 'missing-a', title: 'A', filePath: '/fixture/a.mp3' }, { songId: 'missing-b', title: 'B', filePath: '/fixture/b.mp3' }],
    unavailableMedia: [{ songId: 'offline-c', title: 'C', reason: 'offline', filePath: '/fixture/c.mp3' }],
    brokenPlaylistReferences: [], scannerFailures: [] });
  service.repair.mockResolvedValue({ removedMissing: 2, removedPlaylistReferences: 2 });
  service.stageRestore.mockResolvedValue({ staged: true, restartRequired: true, path: '/fixture/pending' });
  container = document.createElement('div'); document.body.appendChild(container);
  root = createRoot(container);
  await act(async () => root.render(<LibraryOperationsPanel />));
});
afterEach(async () => {
  await act(async () => root.unmount()); container.remove(); vi.restoreAllMocks();
});

it('discloses missing-track data loss, makes no mutation on cancel, and sends only reviewed missing IDs after approval', async () => {
  const confirmation = vi.spyOn(window, 'confirm').mockReturnValueOnce(false).mockReturnValueOnce(true);
  await click('Run diagnostics');
  await click('Remove missing files');
  expect(service.repair).not.toHaveBeenCalled();
  expect(confirmation.mock.calls[0][0]).toContain('likes and playlist entries');
  expect(confirmation.mock.calls[0][0]).toContain('Create a backup');
  await click('Remove missing files');
  expect(service.repair).toHaveBeenCalledExactlyOnceWith(true, expect.any(AbortSignal), ['missing-a', 'missing-b']);
});

it('discloses restore replacement scope and leaves the restore unstaged until confirmation', async () => {
  const confirmation = vi.spyOn(window, 'confirm').mockReturnValueOnce(false).mockReturnValueOnce(true);
  await click('Stage');
  expect(service.stageRestore).not.toHaveBeenCalled();
  expect(confirmation.mock.calls[0][0]).toContain('catalog, playlists, likes, listening history and settings');
  expect(confirmation.mock.calls[0][0]).toContain('Changes made after the backup');
  expect(confirmation.mock.calls[0][0]).toContain('Audio files are unaffected');
  await click('Stage');
  expect(service.stageRestore).toHaveBeenCalledExactlyOnceWith('fixture.zip', expect.any(AbortSignal));
  expect(container.textContent).toContain('A restart is required');
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(done => { resolve = done; });
  return { promise, resolve };
}

async function leavePanel() {
  await act(async () => root.unmount());
  // Keep teardown valid and permit an unrelated replacement UI to mount.
  root = createRoot(container);
}

it('aborts a pending backup-list read on unmount', async () => {
  await leavePanel();
  const pending = deferred<{ backups: [] }>();
  service.listBackups.mockReturnValueOnce(pending.promise);
  await act(async () => root.render(<LibraryOperationsPanel />));
  const signal = service.listBackups.mock.lastCall![0] as AbortSignal;
  expect(signal.aborted).toBe(false);
  await leavePanel();
  expect(signal.aborted).toBe(true);
  await act(async () => pending.resolve({ backups: [] }));
});

it('does not start a diagnostics refresh when a repair finishes after unmount', async () => {
  const pending = deferred<{ removedMissing: number }>();
  service.repair.mockReturnValueOnce(pending.promise);
  await click('Repair indexes');
  const signal = service.repair.mock.lastCall![1] as AbortSignal;
  expect(signal.aborted).toBe(false);
  await leavePanel();
  expect(signal.aborted).toBe(true);
  await act(async () => pending.resolve({ removedMissing: 0 }));
  expect(service.diagnostics).not.toHaveBeenCalled();
});

it('does not refresh backups when creation finishes after unmount', async () => {
  const pending = deferred<{ name: string }>();
  service.createBackup.mockReturnValueOnce(pending.promise);
  await click('Create validated backup');
  const signal = service.createBackup.mock.lastCall![1] as AbortSignal;
  await leavePanel();
  expect(signal.aborted).toBe(true);
  await act(async () => pending.resolve({ name: 'later.zip' }));
  expect(service.listBackups).toHaveBeenCalledOnce();
});

it('serializes maintenance requests even when clicks arrive in one render batch', async () => {
  const pending = deferred<{ removedMissing: number }>();
  service.repair.mockReturnValue(pending.promise);
  const button = [...container.querySelectorAll('button')].find(item => item.textContent?.trim() === 'Repair indexes')!;
  await act(async () => { button.click(); button.click(); });
  expect(service.repair).toHaveBeenCalledOnce();
  await act(async () => pending.resolve({ removedMissing: 0 }));
});
