// @vitest-environment jsdom
import { deferred } from '../testUtils/deferred';
import React, { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({ create: vi.fn() }));
vi.mock('../lib/djFourChannelAudio', () => ({ DJFourChannelAudio: { create: mocks.create } }));
import { useFourChannelDJAudio } from './useFourChannelDJAudio';

let root: Root;
let audio: ReturnType<typeof useFourChannelDJAudio>;
function Host() { audio = useFourChannelDJAudio(); return null; }
beforeEach(async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  mocks.create.mockReset();
  root = createRoot(document.createElement('div'));
  await act(async () => root.render(<Host />));
});
afterEach(async () => { await act(async () => root.unmount()); });

it('creates one engine for concurrent starts and disposes it on stop', async () => {
  const created = { dispose: vi.fn(async () => {}) };
  const request = deferred<typeof created>();
  mocks.create.mockReturnValue(request.promise);
  let first: ReturnType<typeof audio.start> | undefined;
  await act(async () => {
    first = audio.start({ deviceId: 'mixtrack' });
    expect(audio.start()).toBe(first);
  });
  if (!first) throw new Error('Missing start promise');
  expect(audio.starting).toBe(true);
  await act(async () => { request.resolve(created); await first; });
  expect(mocks.create).toHaveBeenCalledTimes(1);
  expect(audio.engine).toBe(created);
  expect(audio.starting).toBe(false);
  await act(async () => audio.stop());
  expect(created.dispose).toHaveBeenCalledOnce();
  expect(audio.engine).toBeNull();
});

it('disposes a late initialization after stop instead of publishing it', async () => {
  const created = { dispose: vi.fn(async () => {}) };
  const request = deferred<typeof created>();
  mocks.create.mockReturnValue(request.promise);
  let operation: ReturnType<typeof audio.start> | undefined;
  await act(async () => { operation = audio.start(); });
  if (!operation) throw new Error('Missing start promise');
  await act(async () => audio.stop());
  expect((mocks.create.mock.calls[0][1] as AbortSignal).aborted).toBe(true);
  await act(async () => { request.resolve(created); expect(await operation).toBeNull(); });
  expect(audio.engine).toBeNull();
  expect(created.dispose).toHaveBeenCalledOnce();
});

it('disposes a late initialization after unmount', async () => {
  const created = { dispose: vi.fn(async () => {}) };
  const request = deferred<typeof created>();
  mocks.create.mockReturnValue(request.promise);
  let operation: ReturnType<typeof audio.start> | undefined;
  await act(async () => { operation = audio.start(); });
  if (!operation) throw new Error('Missing start promise');
  await act(async () => root.unmount());
  request.resolve(created);
  if (!operation) throw new Error('Missing start promise');
  expect(await operation).toBeNull();
  expect(created.dispose).toHaveBeenCalledOnce();
});

it('reports device errors and allows retrying with another device', async () => {
  mocks.create.mockRejectedValueOnce(new Error('Only two channels available'));
  await act(async () => { expect(await audio.start()).toBeNull(); });
  expect(audio.error).toBe('Only two channels available');
  expect(audio.starting).toBe(false);
  const created = { dispose: vi.fn(async () => {}) };
  mocks.create.mockResolvedValueOnce(created);
  await act(async () => { await audio.start({ deviceId: 'four-channel' }); });
  expect(audio.error).toBeNull();
  expect(audio.engine).toBe(created);
});

it('does not let a cancelled start overwrite a replacement engine', async () => {
  const old = { dispose: vi.fn(async () => {}) };
  const replacement = { dispose: vi.fn(async () => {}) };
  const request = deferred<typeof old>();
  mocks.create.mockReturnValueOnce(request.promise).mockResolvedValueOnce(replacement);
  let previous: ReturnType<typeof audio.start> | undefined;
  await act(async () => { previous = audio.start(); });
  if (!previous) throw new Error('Missing start promise');
  await act(async () => audio.stop());
  await act(async () => { await audio.start({ deviceId: 'replacement' }); });
  await act(async () => { request.resolve(old); await previous; });
  expect(audio.engine).toBe(replacement);
  expect(old.dispose).toHaveBeenCalledOnce();
  expect(replacement.dispose).not.toHaveBeenCalled();
});
