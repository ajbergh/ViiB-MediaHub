// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, expect, it, vi } from 'vitest';
const mocks = vi.hoisted(() => ({ state: {} as any, analysis: vi.fn(), play: vi.fn(), context: vi.fn(), info: vi.fn(), like: vi.fn() }));
vi.mock('../store', () => ({ useStore: (selector?: any) => selector ? selector(mocks.state) : mocks.state, useAlbumCovers: () => ({}) }));
vi.mock('../services/api', () => ({ api: { getTrackAnalysisFeatures: mocks.analysis } }));
vi.mock('react-router', () => ({ useNavigate: () => vi.fn() }));
vi.mock('../components/LikeButton', () => ({ LikeButton: () => <button aria-label="Like" onClick={event => { event.stopPropagation(); mocks.like(); }}>Like</button> }));
vi.mock('react-virtuoso', () => ({ Virtuoso: ({ data, itemContent, components, context }: any) => <><components.Header context={context}/>{data.map((song: any, index: number) => <React.Fragment key={song.id}>{itemContent(index, song)}</React.Fragment>)}</> }));
import { Songs } from './Songs';
import { DEFAULT_SONG_COLUMNS, SONG_COLUMNS_STORAGE_KEY } from '../lib/songColumns';
afterEach(() => { localStorage.clear(); vi.clearAllMocks(); });
const setup = () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  mocks.state = { songs: [
    { id: 'a', title: 'Alpha', artist: 'Artist', album: 'Album', bpm: 110, genre: ['Rock'], originalYear: 1995, duration: 180, addedAt: 1 },
    { id: 'b', title: 'Beta', artist: 'Artist', album: 'Album', duration: 240, addedAt: 0 },
  ], playSong: mocks.play, openContextMenu: mocks.context, openSongInfoModal: mocks.info };
  const container = document.createElement('div'); document.body.append(container);
  return { container, root: createRoot(container) };
};
const toggle = async (label: string) => act(async () => [...document.querySelectorAll<HTMLButtonElement>('[role=menuitemcheckbox]')].find(button => button.textContent?.trim() === label)!.click());
const openChooser = async (container: HTMLElement) => act(async () => container.querySelector<HTMLButtonElement>('[aria-label="Choose track columns"]')!.click());

it('opens from the header, aligns selected columns, preserves actions and preferences, and resets defaults', async () => {
  mocks.analysis.mockResolvedValue([{ songId: 'a', bpm: 123.45, key: 'C minor', camelotKey: '5A', energyLevel: 0 }]);
  const fixture = setup(); let root = fixture.root; const { container } = fixture;
  const render = () => act(async () => root.render(<Songs/>));
  const cells = (element: Element) => [...element.querySelectorAll('[data-column]')].map(cell => [cell.getAttribute('data-column'), cell.textContent]);
  try {
    await render(); expect(mocks.analysis).not.toHaveBeenCalled();
    const header = container.querySelector<HTMLElement>('[aria-label="Track column header"]')!;
    const event = new MouseEvent('contextmenu', { bubbles: true, cancelable: true, clientX: 900, clientY: 500 });
    await act(async () => header.dispatchEvent(event));
    expect(event.defaultPrevented).toBe(true);
    expect(document.querySelector('[aria-label="Track columns"]')).not.toBeNull();
    const menu = document.querySelector('[aria-label="Track columns"]')!;
    expect(document.activeElement?.textContent?.trim()).toBe('Album');
    await act(async () => menu.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true })));
    expect(document.activeElement?.textContent?.trim()).toBe('Artist');
    await toggle('BPM'); await toggle('Key'); await toggle('Genre'); await toggle('Year'); await toggle('Energy');
    const row = container.querySelector<HTMLElement>('[data-song-id="a"]')!;
    expect(cells(header).map(cell => cell[0])).toEqual(cells(row).map(cell => cell[0]));
    expect(cells(row)).toEqual(expect.arrayContaining([['bpm', '123.5'], ['key', 'C minor'], ['genre', 'Rock'], ['year', '1995'], ['energy', '0']]));
    expect(cells(container.querySelector('[data-song-id="b"]')!)).toContainEqual(['key', '—']);
    expect(container.querySelectorAll('[style]')[0].getAttribute('style')).toContain('--song-columns:');
    expect(JSON.parse(localStorage.getItem(SONG_COLUMNS_STORAGE_KEY)!)).toContain('key');
    await act(async () => document.querySelector('[aria-label="Track columns"]')!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })));
    expect(document.querySelector('[aria-label="Track columns"]')).toBeNull();
    expect(document.activeElement).toBe(header);
    await act(async () => row.querySelector<HTMLButtonElement>('[aria-label="Track Info"]')!.click());
    await act(async () => row.querySelector<HTMLButtonElement>('[aria-label="More options"]')!.click());
    await act(async () => row.querySelector<HTMLButtonElement>('[aria-label="Like"]')!.click());
    expect(mocks.info).toHaveBeenCalledWith(mocks.state.songs[0]); expect(mocks.context).toHaveBeenCalled(); expect(mocks.like).toHaveBeenCalled(); expect(mocks.play).not.toHaveBeenCalled();
    await act(async () => row.click()); expect(mocks.play).toHaveBeenCalledWith(mocks.state.songs[0], mocks.state.songs);
    await act(async () => root.unmount()); root = createRoot(container); await render();
    expect(cells(container.querySelector('[data-song-id="a"]')!)).toContainEqual(['key', 'C minor']);
    await openChooser(container);
    await act(async () => [...document.querySelectorAll<HTMLButtonElement>('[role=menuitem]')].find(button => button.textContent === 'Reset to default')!.click());
    expect(cells(container.querySelector('[data-song-id="a"]')!).map(cell => cell[0])).toEqual(DEFAULT_SONG_COLUMNS);
  } finally { await act(async () => root.unmount()); container.remove(); }
});

it('uses metadata BPM and unknown Key if analysis fails, then refreshes when the library updates', async () => {
  localStorage.setItem(SONG_COLUMNS_STORAGE_KEY, JSON.stringify(['bpm', 'key']));
  mocks.analysis.mockRejectedValueOnce(new Error('offline')).mockResolvedValue([{ songId: 'a', bpm: 120, key: 'D major' }]);
  const { root, container } = setup();
  try {
    await act(async () => root.render(<Songs/>));
    expect(container.querySelector('[data-song-id="a"] [data-column=bpm]')?.textContent).toBe('110');
    expect(container.querySelector('[data-song-id="a"] [data-column=key]')?.textContent).toBe('—');
    await act(async () => window.dispatchEvent(new Event('library_updated')));
    expect(container.querySelector('[data-song-id="a"] [data-column=key]')?.textContent).toBe('D major');
  } finally { await act(async () => root.unmount()); container.remove(); }
});
