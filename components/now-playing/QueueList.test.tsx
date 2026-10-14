// @vitest-environment jsdom
/** Covers queue indices and one reorder per drop under React Strict Mode. */
import React, { act, StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
import type { Song } from '../../types';

const mocks = vi.hoisted(() => ({ play: vi.fn(), remove: vi.fn(), reorder: vi.fn(), context: vi.fn(), toast: vi.fn(), state: {} as any }));
vi.mock('../../store', () => ({
  useStore: (select: any) => select(mocks.state),
  useAlbumCovers: () => ({}),
}));
vi.mock('react-virtuoso', () => ({ Virtuoso: ({ data, itemContent }: any) => <>{data.map((song: Song, index: number) => <React.Fragment key={index}>{itemContent(index, song)}</React.Fragment>)}</> }));
import { QueueList } from './QueueList';
import { Queue } from '../Queue';

it.each([['Now Playing', QueueList], ['Popover', Queue]] as const)('%s reorders once, clears canceled drags, and preserves duplicate-song indices', async (_name, Component) => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  vi.clearAllMocks();
  const host = document.createElement('div');
  const root = createRoot(host);
  const song = { id: 'same-song', title: 'Track', artist: 'Artist', album: 'Album', duration: 60, coverUrl: '/broken.jpg' } as Song;
  mocks.state = { queue: [song, song], currentSongIndex: 0, isQueueOpen: true, playQueueItem: mocks.play, removeFromQueue: mocks.remove, reorderQueue: mocks.reorder, openContextMenu: mocks.context, showToast: mocks.toast };
  const drag = (element: Element, type: string) => {
    const event = new Event(type, { bubbles: true, cancelable: true });
    Object.defineProperty(event, 'dataTransfer', { value: { effectAllowed: '' } });
    element.dispatchEvent(event);
  };
  try {
    await act(async () => root.render(<StrictMode><Component queue={[song, song]} currentSongIndex={0}/></StrictMode>));
    const rows = host.querySelectorAll('[draggable=true]');
    await act(async () => drag(rows[0], 'dragstart'));
    await act(async () => drag(rows[1], 'drop'));
    expect(mocks.reorder).toHaveBeenCalledTimes(1);
    expect(mocks.reorder).toHaveBeenLastCalledWith(0, 1);
    await act(async () => drag(rows[0], 'dragstart'));
    await act(async () => drag(rows[0], 'dragend'));
    await act(async () => drag(rows[1], 'drop'));
    expect(mocks.reorder).toHaveBeenCalledTimes(1);
    await act(async () => rows[1].querySelector<HTMLButtonElement>('[aria-label^="Remove"]')!.click());
    expect(mocks.remove).toHaveBeenCalledWith(1);
    await act(async () => rows[1].querySelector<HTMLButtonElement>('[aria-label="Play Track"]')!.click());
    expect(mocks.play).toHaveBeenCalledWith(1);
    await act(async () => rows[1].dispatchEvent(new MouseEvent('contextmenu', { bubbles: true })));
    expect(mocks.context.mock.lastCall?.[2]).toEqual({ song, index: 1 });
    await act(async () => rows[1].querySelector('img')!.dispatchEvent(new Event('error')));
    expect(rows[1].querySelector('img')).toBeNull();
  } finally { await act(async () => root.unmount()); }
});
