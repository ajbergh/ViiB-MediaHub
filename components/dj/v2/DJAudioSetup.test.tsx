// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  mode: 'separate',
  route: vi.fn(), persist: vi.fn(), headphone: vi.fn(), persistHeadphone: vi.fn(),
  testChannel: vi.fn(), stopTest: vi.fn(),
}));
vi.mock('../../../store', () => ({ useStore: { getState: () => ({
  audioSettings: { djOutputMode: mocks.mode, mainOutputDevice: 'speakers', headphoneOutputDevice: 'controller' },
  setDJOutputRouting: mocks.persist, setHeadphoneOutputDevice: mocks.persistHeadphone,
}) } }));
vi.mock('../../../lib/djAudio', () => ({ getDJAudioEngine: () => engine }));
const engine = {
  initialized: true,
  getOutputMode: () => mocks.mode,
  getMainOutputDeviceId: () => 'speakers', getHeadphoneOutputDeviceId: () => 'controller',
  setOutputRouting: mocks.route, setHeadphoneOutputDevice: mocks.headphone,
  getOutputDiagnostics: () => ({ channelCount: 4, maxChannelCount: 4, sampleRate: 48000, contextState: 'running', cueA: false, cueB: false, cueMix: 0, headphoneVolume: 1 }),
  testOutputChannel: mocks.testChannel, stopOutputTest: mocks.stopTest,
};
import { DJAudioSetup } from './DJAudioSetup';

let root: Root, container: HTMLDivElement;
let previous: PropertyDescriptor | undefined;
const select = async (label: string, value: string) => {
  const element = container.querySelector<HTMLSelectElement>(label)!;
  await act(async () => { element.value = value; element.dispatchEvent(new Event('change', { bubbles: true })); });
};
const applyMain = async () => {
  const button = [...container.querySelectorAll('button')].find(button => button.textContent === 'Apply')!;
  await act(async () => button.click());
};
beforeEach(async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  vi.clearAllMocks(); mocks.mode = 'separate'; mocks.route.mockReset();
  previous = Object.getOwnPropertyDescriptor(navigator, 'mediaDevices');
  Object.defineProperty(navigator, 'mediaDevices', { configurable: true, value: {
    enumerateDevices: vi.fn(async () => [
      { kind: 'audiooutput', deviceId: 'speakers', label: 'Desktop speakers' },
      { kind: 'audiooutput', deviceId: 'controller', label: 'Speakers (Numark MixTrack Go Audio)' },
    ]),
  } });
  container = document.createElement('div'); root = createRoot(container);
  await act(async () => root.render(<DJAudioSetup onClose={() => {}} />));
});
afterEach(async () => {
  await act(async () => root.unmount());
  if (previous) Object.defineProperty(navigator, 'mediaDevices', previous);
  else delete (navigator as any).mediaDevices;
});

it('offers single-device four-channel routing and applies the selected MixTrack sink to the live engine', async () => {
  await select('#dj-output-mode', 'four-channel');
  expect(container.textContent).toContain('Headphones: channels 3–4');
  expect(container.querySelector('[aria-label="Headphone output device"]')).toBeNull();
  await select('[aria-label="Master output device"]', 'controller');
  await applyMain();
  expect(mocks.route).toHaveBeenCalledExactlyOnceWith('four-channel', 'controller');
  expect(mocks.persist).toHaveBeenCalledExactlyOnceWith('four-channel', 'controller', 'controller');
  expect(container.textContent).toContain('Four-channel routing applied');
});

it('shows channel-capacity errors and does not persist a rejected route', async () => {
  mocks.route.mockRejectedValueOnce(new Error('This device exposes 2 output channels'));
  await select('#dj-output-mode', 'four-channel');
  await applyMain();
  expect(container.textContent).toContain('This device exposes 2 output channels');
  expect(mocks.persist).not.toHaveBeenCalled();
});

it('does not save an unapplied headphone dropdown change when only master is applied', async () => {
  await select('[aria-label="Headphone output device"]', 'speakers');
  await applyMain();
  expect(mocks.persist).toHaveBeenCalledExactlyOnceWith('separate', 'speakers', 'controller');
  expect(mocks.headphone).not.toHaveBeenCalled();
});

it('tests a headphone channel directly and shows the cue-only silence hint', async () => {
  mocks.mode = 'four-channel';
  await select('#dj-output-mode', 'four-channel');
  expect(container.textContent).toContain('neither deck cued');
  const button = [...container.querySelectorAll('button')].find(button => button.textContent === 'Test Headphone left · 3')!;
  await act(async () => button.click());
  expect(mocks.testChannel).toHaveBeenCalledWith(2);
  expect(container.textContent).toContain('Channel 3 test finished');
});
