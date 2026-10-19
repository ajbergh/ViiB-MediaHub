import { beforeEach, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({ initialize: vi.fn(), playing: vi.fn() }));
vi.mock('./djAudio', () => ({ getDJAudioEngine: () => ({ initialize: mocks.initialize }) }));
vi.mock('../store', () => ({ useStore: { getState: () => ({
  djSampler: Array.from({ length: 8 }, () => ({ volume: 0.8, mode: 'gate' })),
  setSamplerPadPlaying: mocks.playing,
}) } }));
import { DJSamplerEngine } from './djSampler';

function fixture() {
  const node = () => ({ gain: { value: 1 }, connect: vi.fn(), disconnect: vi.fn() });
  const bus = node();
  const source = { ...node(), start: vi.fn(), stop: vi.fn(), buffer: null, loop: false, onended: null };
  const context = { state: 'running', currentTime: 2, createGain: vi.fn(node),
    createBufferSource: vi.fn(() => source), close: vi.fn() } as unknown as AudioContext;
  const sampler = new DJSamplerEngine();
  Object.assign(sampler, { buffers: [{ duration: 1 }, ...Array(7).fill(null)] });
  return { sampler, context, bus, source };
}
beforeEach(() => { mocks.initialize.mockReset(); mocks.playing.mockClear(); });

it('uses the shared DJ context and master bus, without creating another destination path', async () => {
  const { sampler, context, bus, source } = fixture();
  sampler.attachOutput(context, bus as unknown as AudioNode);
  const master = (sampler as any).masterGain;
  expect(master.connect).toHaveBeenCalledExactlyOnceWith(bus);
  await sampler.triggerPad(0);
  expect(context.createBufferSource).toHaveBeenCalledOnce();
  expect(source.start).toHaveBeenCalledOnce();
  expect(mocks.initialize).not.toHaveBeenCalled();
  sampler.detachOutput();
  expect(source.stop).toHaveBeenCalledOnce();
  expect(master.disconnect).toHaveBeenCalledOnce();
  expect(context.close).not.toHaveBeenCalled();
  expect(sampler.isLoaded(0)).toBe(true);
});

it('initializes and attaches the DJ context before triggering a retained sample', async () => {
  const { sampler, context, bus, source } = fixture();
  mocks.initialize.mockImplementation(async () => { sampler.attachOutput(context, bus as unknown as AudioNode); });
  await sampler.triggerPad(0);
  expect(mocks.initialize).toHaveBeenCalledOnce();
  expect(source.start).toHaveBeenCalledOnce();
  sampler.destroy();
  expect(context.close).not.toHaveBeenCalled();
});

it('does not start a gated sample if released while the DJ context initializes', async () => {
  const { sampler, context, bus, source } = fixture();
  let finish: () => void;
  mocks.initialize.mockReturnValue(new Promise<void>(resolve => { finish = resolve; }));
  const pending = sampler.triggerPad(0);
  sampler.stopPad(0);
  sampler.attachOutput(context, bus as unknown as AudioNode);
  finish();
  await pending;
  expect(source.start).not.toHaveBeenCalled();
});
