import { afterEach, expect, it, vi } from 'vitest';
import { DJVisualScheduler } from './djVisualScheduler';

afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals(); });
it('uses one RAF for all jobs and cancels it when the last meter unmounts', () => {
  const request = vi.fn(() => 1), cancel = vi.fn();
  vi.stubGlobal('requestAnimationFrame', request); vi.stubGlobal('cancelAnimationFrame', cancel);
  const scheduler = new DJVisualScheduler();
  const a = scheduler.subscribe(() => true), b = scheduler.subscribe(() => true), c = scheduler.subscribe(() => true);
  expect(request).toHaveBeenCalledTimes(1);
  a(); b(); expect(cancel).not.toHaveBeenCalled();
  c(); expect(cancel).toHaveBeenCalledWith(1);
});
it('sleeps at idle and wakes on a new subscription without adding independent clocks', () => {
  vi.useFakeTimers();
  const queue: FrameRequestCallback[] = [];
  vi.stubGlobal('requestAnimationFrame', vi.fn((callback: FrameRequestCallback) => { queue.push(callback); return queue.length; }));
  vi.stubGlobal('cancelAnimationFrame', vi.fn());
  const scheduler = new DJVisualScheduler();
  const idle = vi.fn(() => false);
  const off = scheduler.subscribe(idle);
  queue.shift()!(16);
  expect(idle).toHaveBeenCalledTimes(1);
  expect(queue).toHaveLength(0);
  vi.advanceTimersByTime(249); expect(queue).toHaveLength(0);
  vi.advanceTimersByTime(1); expect(queue).toHaveLength(1);
  off();
  expect(vi.getTimerCount()).toBe(0);
});
