// @vitest-environment jsdom
import React, { act, useState } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';

const queue = vi.hoisted(() => vi.fn());
vi.mock('../services/api', () => ({ default: { downloadFromURL: queue } }));
vi.mock('../store', () => ({ useStore: (selector: (state: { spotifyConnected: boolean; spotifyAccountProduct: string }) => unknown) => selector({ spotifyConnected: true, spotifyAccountProduct: 'premium' }) }));
vi.mock('./SpotifyAccountNotice', () => ({ SpotifyAccountNotice: () => null }));
import { DirectDownloadDialog } from './DirectDownloadDialog';

let container: HTMLDivElement;
let root: Root;
let unmounted: boolean;
const result = { type: 'track', title: 'Test song' };

beforeEach(() => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
  vi.useFakeTimers();
  queue.mockReset().mockResolvedValue(result);
  container = document.createElement('div');
  document.body.append(container);
  root = createRoot(container);
  unmounted = false;
});

afterEach(async () => {
  if (!unmounted) await act(async () => root.unmount());
  container.remove();
  vi.useRealTimers();
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail; });
  return { promise, resolve, reject };
}

async function submit() {
  const input = container.querySelector('input')!;
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, 'spotify:track:abc123');
    input.dispatchEvent(new Event('input', { bubbles: true }));
  });
  await act(async () => {
    container.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  });
}

it('refreshes once after persistence, waits for refresh, then closes after two seconds', async () => {
  const response = deferred<typeof result>();
  const refresh = deferred<void>();
  queue.mockReturnValue(response.promise);
  const queued = vi.fn(() => refresh.promise);
  const close = vi.fn();
  await act(async () => root.render(<DirectDownloadDialog isOpen onClose={close} onQueued={queued} />));
  await submit();
  expect(queue).toHaveBeenCalledExactlyOnceWith('spotify:track:abc123');
  expect(queued).not.toHaveBeenCalled();
  await act(async () => response.resolve(result));
  expect(queued).toHaveBeenCalledOnce();
  await act(async () => vi.advanceTimersByTime(3000));
  expect(close).not.toHaveBeenCalled();
  await act(async () => refresh.resolve());
  expect(container.textContent).toContain('Queued "Test song"');
  await act(async () => vi.advanceTimersByTime(1999));
  expect(close).not.toHaveBeenCalled();
  await act(async () => vi.advanceTimersByTime(1));
  expect(close).toHaveBeenCalledOnce();
});

it('cancels the old success timer when closed and reopened', async () => {
  const close = vi.fn();
  function Harness() {
    const [open, setOpen] = useState(true);
    return <><button data-reopen onClick={() => setOpen(true)}>Reopen</button><DirectDownloadDialog isOpen={open} onClose={() => { close(); setOpen(false); }} /></>;
  }
  await act(async () => root.render(<Harness />));
  await submit();
  await act(async () => container.querySelector<HTMLButtonElement>('[aria-label="Close direct download dialog"]')!.click());
  await act(async () => container.querySelector<HTMLButtonElement>('[data-reopen]')!.click());
  await act(async () => vi.advanceTimersByTime(3000));
  expect(close).toHaveBeenCalledOnce();
  expect(container.querySelector('[role="dialog"]')).not.toBeNull();
  expect(container.textContent).not.toContain('Queued "Test song"');
});

it('does not refresh or schedule a close after an unmounted request succeeds', async () => {
  const response = deferred<typeof result>();
  queue.mockReturnValue(response.promise);
  const close = vi.fn();
  const queued = vi.fn();
  await act(async () => root.render(<DirectDownloadDialog isOpen onClose={close} onQueued={queued} />));
  await submit();
  await act(async () => root.unmount());
  unmounted = true;
  await act(async () => response.resolve(result));
  await act(async () => vi.advanceTimersByTime(3000));
  expect(queued).not.toHaveBeenCalled();
  expect(close).not.toHaveBeenCalled();
  expect(vi.getTimerCount()).toBe(0);
});

it('does not let an old API success or failure affect a reopened session', async () => {
  for (const fails of [false, true]) {
    const response = deferred<typeof result>();
    queue.mockReturnValue(response.promise);
    const close = vi.fn();
    const queued = vi.fn();
    await act(async () => root.render(<DirectDownloadDialog isOpen onClose={close} onQueued={queued} />));
    await submit();
    await act(async () => root.render(<DirectDownloadDialog isOpen={false} onClose={close} onQueued={queued} />));
    await act(async () => root.render(<DirectDownloadDialog isOpen onClose={close} onQueued={queued} />));
    await act(async () => { if (fails) response.reject(new Error('old failure')); else response.resolve(result); });
    await act(async () => vi.advanceTimersByTime(3000));
    expect(queued).not.toHaveBeenCalled();
    expect(close).not.toHaveBeenCalled();
    expect(container.textContent).not.toContain('old failure');
    expect(container.textContent).not.toContain('Queued "Test song"');
    expect(container.querySelector('input')!.disabled).toBe(false);
    await act(async () => root.render(<DirectDownloadDialog isOpen={false} onClose={close} />));
  }
});

it('does not close a replacement dialog when queue refresh leaves the empty state', async () => {
  const close = vi.fn();
  const refresh = deferred<void>();
  function Harness() {
    const [empty, setEmpty] = useState(true);
    const queued = async () => { setEmpty(false); await refresh.promise; };
    // Downloads renders these in separate empty/nonempty page branches.
    return empty
      ? <div><DirectDownloadDialog isOpen onClose={close} onQueued={queued} /></div>
      : <section><DirectDownloadDialog isOpen onClose={close} onQueued={queued} /></section>;
  }
  await act(async () => root.render(<Harness />));
  await submit();
  expect(container.querySelector('section [role="dialog"]')).not.toBeNull();
  await act(async () => refresh.resolve());
  await act(async () => vi.advanceTimersByTime(3000));
  expect(close).not.toHaveBeenCalled();
  expect(vi.getTimerCount()).toBe(0);
});
