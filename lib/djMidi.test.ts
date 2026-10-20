import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { DJMidiService } from './djMidi';

function fixture() {
  const input = {
    id: 'input-1', type: 'input', name: 'Numark MixTrack Go', manufacturer: 'Numark',
    state: 'connected', onmidimessage: null,
  } as unknown as MIDIInput;
  const output = {
    id: 'output-1', type: 'output', name: 'Numark MixTrack Go', manufacturer: 'Numark',
    state: 'connected', connection: 'closed', open: vi.fn(), close: vi.fn().mockResolvedValue(undefined), send: vi.fn(),
  } as unknown as MIDIOutput;
  const access = {
    inputs: new Map([[input.id, input]]), outputs: new Map([[output.id, output]]),
    sysexEnabled: true, onstatechange: null,
  } as unknown as MIDIAccess;
  const emit = (port: MIDIPort, state: MIDIPortDeviceState = port.state) => {
    Object.assign(port, { state });
    access.onstatechange?.({ port } as MIDIConnectionEvent);
  };
  vi.mocked(output.open).mockImplementation(async () => {
    Object.assign(output, { connection: 'open' });
    emit(output); // Real open() produces another state-change event.
    return output;
  });
  const request = vi.fn().mockResolvedValue(access);
  vi.stubGlobal('navigator', { requestMIDIAccess: request });
  return { input, output, access, request, emit };
}

describe('MixTrack connection SysEx', () => {
  let midi: DJMidiService;
  beforeEach(() => {
    midi = new DJMidiService();
    vi.spyOn(console, 'log').mockImplementation(() => {});
    vi.spyOn(console, 'warn').mockImplementation(() => {});
  });
  afterEach(async () => {
    midi.destroy();
    await Promise.resolve();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it('routes repeated Browse pushes and Load 1/2 to the correct library actions', async () => {
    const { input } = fixture();
    await midi.init();
    const handler = vi.fn();
    midi.setActionHandler(handler);
    for (const note of [0x07, 0x07, 0x02, 0x03]) {
      input.onmidimessage?.({ data: new Uint8Array([0x9F, note, 127]) } as MIDIMessageEvent);
      input.onmidimessage?.({ data: new Uint8Array([0x8F, note, 0]) } as MIDIMessageEvent);
    }
    expect(handler.mock.calls.map(call => call.slice(0, 3))).toEqual([
      ['library.open', 1, 'press'], ['library.open', 1, 'press'],
      ['deckA.load', 1, 'press'], ['deckB.load', 1, 'press'],
    ]);
  });

  it('requests SysEx permission, awaits output open, and sends the exact six bytes once', async () => {
    const { output, request, emit } = fixture();
    let finishOpen!: (port: MIDIPort) => void;
    vi.mocked(output.open).mockImplementation(() => new Promise(resolve => { finishOpen = resolve; }));
    const init = midi.init();
    await vi.waitFor(() => expect(output.open).toHaveBeenCalledOnce());
    expect(output.send).not.toHaveBeenCalled();
    emit(output);
    finishOpen(output);
    expect(await init).toBe(true);
    expect(request).toHaveBeenCalledWith({ sysex: true });
    expect(vi.mocked(output.send).mock.calls.map(([bytes]) => Array.from(bytes))).toEqual([
      [0xF0, 0x7E, 0x00, 0x06, 0x01, 0xF7],
      [0x94, 2, 0], [0x94, 3, 0], [0x94, 4, 0], [0x94, 1, 127],
      [0x95, 2, 0], [0x95, 3, 0], [0x95, 4, 0], [0x95, 1, 127],
    ]);
    emit(output);
    await midi.init();
    expect(output.send).toHaveBeenCalledTimes(9);
  });

  it('handles an output arriving after its input and sends again after reconnect', async () => {
    const { output, access, emit } = fixture();
    (access.outputs as unknown as Map<string, MIDIOutput>).clear();
    await midi.init();
    expect(output.send).not.toHaveBeenCalled();
    (access.outputs as unknown as Map<string, MIDIOutput>).set(output.id, output);
    emit(output);
    await vi.waitFor(() => expect(output.send).toHaveBeenCalledTimes(9));
    emit(output, 'disconnected');
    emit(output, 'connected');
    await vi.waitFor(() => expect(output.send).toHaveBeenCalledTimes(18));
  });

  it('handles input-only reconnect without relying on a MIDIInput constructor', async () => {
    const { input, output, emit } = fixture();
    await midi.init();
    emit(input, 'disconnected');
    emit(input, 'connected');
    await vi.waitFor(() => expect(output.send).toHaveBeenCalledTimes(18));
  });

  it.each(['unrelated', 'manufacturer mismatch', 'ambiguous input', 'ambiguous output'])('does not send to %s ports', async kind => {
    const { input, output, access } = fixture();
    if (kind === 'unrelated') Object.assign(output, { name: 'Other Keyboard' });
    if (kind === 'manufacturer mismatch') Object.assign(output, { manufacturer: 'Other' });
    if (kind === 'ambiguous input') {
      (access.inputs as unknown as Map<string, MIDIInput>).set('input-2', { ...input, id: 'input-2' });
    }
    if (kind === 'ambiguous output') {
      (access.outputs as unknown as Map<string, MIDIOutput>).set('output-2', { ...output, id: 'output-2' });
    }
    expect(await midi.init()).toBe(true);
    expect(output.open).not.toHaveBeenCalled();
    expect(output.send).not.toHaveBeenCalled();
  });

  it.each(['open', 'send'])('preserves inbound mappings if output %s fails', async operation => {
    const { input, output } = fixture();
    if (operation === 'open') vi.mocked(output.open).mockRejectedValue(new Error('busy'));
    else vi.mocked(output.send).mockImplementation(() => { throw new Error('denied'); });
    expect(await midi.init()).toBe(true);
    expect(midi.isEnabled()).toBe(true);
    const handler = vi.fn();
    midi.setActionHandler(handler);
    midi.addMapping({ id: 'play', deviceId: input.id, type: 'noteon', channel: 0, note: 60, action: 'deckA.play', valueMode: 'toggle' });
    input.onmidimessage?.({ data: new Uint8Array([0x90, 60, 127]) } as MIDIMessageEvent);
    expect(handler).toHaveBeenCalledWith('deckA.play', 1, 'press', expect.anything());
  });

  it('falls back to input-only access when SysEx permission is denied', async () => {
    const { request, access, output, input } = fixture();
    Object.assign(access, { sysexEnabled: false });
    request.mockRejectedValueOnce(new Error('SysEx denied'));
    expect(await midi.init()).toBe(true);
    expect(request.mock.calls).toEqual([[{ sysex: true }], [{ sysex: false }]]);
    expect(input.onmidimessage).toBeTypeOf('function');
    expect(output.send).not.toHaveBeenCalled();
  });

  it('cancels a pending send and closes its output on destroy', async () => {
    const { output, access } = fixture();
    let finishOpen!: (port: MIDIPort) => void;
    vi.mocked(output.open).mockImplementation(() => new Promise(resolve => { finishOpen = resolve; }));
    const init = midi.init();
    await vi.waitFor(() => expect(output.open).toHaveBeenCalledOnce());
    midi.destroy();
    finishOpen(output);
    expect(await init).toBe(false);
    expect(output.send).not.toHaveBeenCalled();
    await vi.waitFor(() => expect(output.close).toHaveBeenCalledOnce());
    expect(access.onstatechange).toBeNull();
  });

  it.each(['noteon', 'cc'] as const)('routes both %s edges for pad actions, including wildcard input identity', async type => {
    const { input } = fixture();
    await midi.init();
    const handler = vi.fn();
    midi.setActionHandler(handler);
    midi.addMapping({ id: 'mode', deviceId: '*', type, channel: 0, note: 42, action: 'deckA.padMode', valueMode: 'toggle' });
    const status = type === 'cc' ? 0xB0 : 0x90;
    for (const velocity of [127, 0]) {
      input.onmidimessage?.({ data: new Uint8Array([status, 42, velocity]) } as MIDIMessageEvent);
    }
    expect(handler.mock.calls.map(call => call.slice(0, 3))).toEqual([
      ['deckA.padMode', 1, 'press'], ['deckA.padMode', 0, 'release'],
    ]);
    expect(handler.mock.calls[0][3].deviceId).toBe(input.id);
  });

  it.each([0, 1, 2, 3] as const)('clears inactive indicators and lights mode %i on the requested deck', async mode => {
    const { input, output } = fixture();
    await midi.init();
    vi.mocked(output.send).mockClear();
    midi.sendPadModeFeedback(input.id, 'B', mode);
    await vi.waitFor(() => expect(output.send).toHaveBeenCalledTimes(4));
    const messages = vi.mocked(output.send).mock.calls.map(([bytes]) => Array.from(bytes));
    expect(messages).toEqual([
      ...[1, 2, 3, 4].filter(note => note !== mode + 1).map(note => [0x95, note, 0]),
      [0x95, mode + 1, 127],
    ]);
  });

  it('restores live software modes after reconnect and resynchronizes an already-open output', async () => {
    const { output, emit } = fixture();
    const modes = { A: 0, B: 0 } as Record<'A' | 'B', 0 | 1 | 2 | 3>;
    await midi.init();
    modes.A = 2;
    modes.B = 3;
    vi.mocked(output.send).mockClear();
    midi.setPadModeProvider(deck => modes[deck]);
    await vi.waitFor(() => expect(output.send).toHaveBeenCalledTimes(8));
    expect(output.send).toHaveBeenCalledWith(new Uint8Array([0x94, 3, 127]));
    expect(output.send).toHaveBeenCalledWith(new Uint8Array([0x95, 4, 127]));
    vi.mocked(output.send).mockClear();
    emit(output, 'disconnected'); emit(output, 'connected');
    await vi.waitFor(() => expect(output.send).toHaveBeenCalledTimes(9));
    expect(output.send).toHaveBeenCalledWith(new Uint8Array([0x94, 3, 127]));
    expect(output.send).toHaveBeenCalledWith(new Uint8Array([0x95, 4, 127]));
  });

  it('does not send feedback to another controller', async () => {
    const { output } = fixture();
    await midi.init();
    vi.mocked(output.send).mockClear();
    midi.sendPadModeFeedback('other-input', 'A', 1);
    await Promise.resolve();
    expect(output.send).not.toHaveBeenCalled();
  });

  it('replays control LEDs after reconnect, suppresses duplicates and clears on teardown', async () => {
    const { output, emit } = fixture();
    let value = 127;
    midi.setFeedbackProvider(() => [[0x90, 0, value], [0x94, 0x14, 1], [0x90, 0x46, 127], [0x91, 0x48, 127]]);
    await midi.init();
    expect(output.send).toHaveBeenCalledWith(new Uint8Array([0x90, 0, 127]));
    vi.mocked(output.send).mockClear();
    midi.refreshFeedback(); midi.refreshFeedback();
    await Promise.resolve();
    expect(output.send).not.toHaveBeenCalled();
    value = 1;
    midi.refreshFeedback();
    await vi.waitFor(() => expect(output.send).toHaveBeenCalledExactlyOnceWith(new Uint8Array([0x90, 0, 1])));
    vi.mocked(output.send).mockClear();
    emit(output, 'disconnected'); emit(output, 'connected');
    await vi.waitFor(() => expect(output.send).toHaveBeenCalledTimes(13));
    expect(output.send).toHaveBeenCalledWith(new Uint8Array([0x94, 0x14, 1]));
    expect(output.send).toHaveBeenCalledWith(new Uint8Array([0x90, 0x46, 127]));
    expect(output.send).toHaveBeenCalledWith(new Uint8Array([0x91, 0x48, 127]));
    vi.mocked(output.send).mockClear();
    midi.setFeedbackProvider(null);
    expect(output.send).toHaveBeenCalledWith(new Uint8Array([0x90, 0, 0]));
    expect(output.send).toHaveBeenCalledWith(new Uint8Array([0x94, 0x14, 0]));
    expect(output.send).toHaveBeenCalledWith(new Uint8Array([0x90, 0x46, 0]));
    expect(output.send).toHaveBeenCalledWith(new Uint8Array([0x91, 0x48, 0]));
  });

  it.each(['cue', 'sync'] as const)('delivers %s release even with a toggle mapping, so feedback cannot stick', async action => {
    const { input } = fixture();
    await midi.init();
    const handler = vi.fn();
    midi.setActionHandler(handler);
    midi.addMapping({ id: action, deviceId: input.id, type: 'noteon', channel: 0, note: 42, action: `deckA.${action}`, valueMode: 'toggle' });
    input.onmidimessage?.({ data: new Uint8Array([0x90, 42, 127]) } as MIDIMessageEvent);
    input.onmidimessage?.({ data: new Uint8Array([0x80, 42, 0]) } as MIDIMessageEvent);
    expect(handler.mock.calls.map(call => call[2])).toEqual(['press', 'release']);
  });

  it('automatically maps a recognized controller and its pad banks without MIDI Learn', async () => {
    const { input } = fixture();
    await midi.init();
    const handler = vi.fn(); midi.setActionHandler(handler);
    const modes = { A: 0, B: 0 } as Record<'A' | 'B', 0 | 1 | 2 | 3>;
    midi.setPadModeProvider(deck => modes[deck]);
    expect(midi.getMappings().filter(mapping => mapping.builtIn && mapping.action === 'deckA.performancePad1').map(mapping => mapping.padMode)).toEqual([0, 1, 2, 3]);
    for (const mode of [0, 1, 2, 3] as const) {
      modes.A = mode;
      input.onmidimessage?.({ data: new Uint8Array([0x94, 0x14, 127]) } as MIDIMessageEvent);
      input.onmidimessage?.({ data: new Uint8Array([0x84, 0x14, 0]) } as MIDIMessageEvent);
    }
    expect(handler.mock.calls.filter(call => call[2] === 'press').map(call => [call[0], call[3].padMode])).toEqual([
      ['deckA.performancePad1', 0], ['deckA.performancePad1', 1], ['deckA.performancePad1', 2], ['deckA.performancePad1', 3],
    ]);
    handler.mockClear();
    input.onmidimessage?.({ data: new Uint8Array([0xBF, 0x0A, 64]) } as MIDIMessageEvent);
    expect(handler).toHaveBeenCalledWith('masterVolume', 64 / 127, 'value', expect.objectContaining({ builtIn: true }));
  });

  it('only routes the active custom pad bank and suppresses the matching default action', async () => {
    const { input } = fixture(); await midi.init();
    const handler = vi.fn(); midi.setActionHandler(handler);
    let mode: 0 | 1 | 2 | 3 = 0;
    midi.setPadModeProvider(() => mode);
    const actions = ['deckA.hotCue2', 'deckA.loopToggle', 'sampler.pad3', 'deckA.stemAcapella'] as const;
    for (const bank of [0, 1, 2, 3] as const) midi.addMapping({ id: `custom-${bank}`, deviceId: input.id, type: 'noteon', channel: 4,
      note: 0x14, valueMode: 'momentary', action: actions[bank], deck: 'A', padMode: bank });
    for (const bank of [0, 1, 2, 3] as const) {
      mode = bank;
      input.onmidimessage?.({ data: new Uint8Array([0x94, 0x14, 127]) } as MIDIMessageEvent);
      input.onmidimessage?.({ data: new Uint8Array([0x84, 0x14, 0]) } as MIDIMessageEvent);
    }
    expect(handler.mock.calls.filter(call => call[2] === 'press').map(call => call[0])).toEqual(actions);
  });

  it('releases a held scoped mapping on mode change and waits for physical release before reusing the pad', async () => {
    const { input } = fixture(); await midi.init();
    const handler = vi.fn(); midi.setActionHandler(handler);
    midi.addMapping({ id: 'gate', deviceId: input.id, type: 'noteon', channel: 4, note: 0x14,
      action: 'sampler.pad1', valueMode: 'momentary', deck: 'A', padMode: 0 });
    input.onmidimessage?.({ data: new Uint8Array([0x94, 0x14, 127]) } as MIDIMessageEvent);
    midi.sendPadModeFeedback(input.id, 'A', 1);
    expect(handler.mock.calls.map(call => call.slice(0, 3))).toEqual([['sampler.pad1', 1, 'press'], ['sampler.pad1', 0, 'release']]);
    input.onmidimessage?.({ data: new Uint8Array([0x94, 0x14, 127]) } as MIDIMessageEvent);
    input.onmidimessage?.({ data: new Uint8Array([0x84, 0x14, 0]) } as MIDIMessageEvent);
    expect(handler).toHaveBeenCalledTimes(2);
    input.onmidimessage?.({ data: new Uint8Array([0x94, 0x14, 127]) } as MIDIMessageEvent);
    expect(handler).toHaveBeenLastCalledWith('deckA.performancePad1', 1, 'press', expect.objectContaining({ padMode: 1 }));
  });

  it('preserves unscoped user overrides and combines both bytes of the tempo control', async () => {
    const { input } = fixture(); await midi.init();
    const handler = vi.fn(); midi.setActionHandler(handler);
    midi.addMapping({ id: 'my-play', deviceId: '*', type: 'noteon', channel: 0, note: 0, action: 'deckB.play', valueMode: 'toggle' });
    input.onmidimessage?.({ data: new Uint8Array([0x90, 0, 127]) } as MIDIMessageEvent);
    expect(handler).toHaveBeenCalledExactlyOnceWith('deckB.play', 1, 'press', expect.objectContaining({ builtIn: false }));
    handler.mockClear();
    input.onmidimessage?.({ data: new Uint8Array([0xB0, 0x29, 127]) } as MIDIMessageEvent);
    expect(handler).not.toHaveBeenCalled();
    input.onmidimessage?.({ data: new Uint8Array([0xB0, 0x09, 64]) } as MIDIMessageEvent);
    expect(handler).toHaveBeenLastCalledWith('deckA.tempo', (64 * 128 + 127) / 16383, 'value', expect.anything());
    input.onmidimessage?.({ data: new Uint8Array([0xB0, 0x29, 0]) } as MIDIMessageEvent);
    expect(handler).toHaveBeenLastCalledWith('deckA.tempo', 8192 / 16383, 'value', expect.anything());
  });

  it('persists custom bank overrides and disabled defaults, with a preset restore path', async () => {
    const storage = new Map<string, string>();
    vi.stubGlobal('localStorage', { getItem: (key: string) => storage.get(key) ?? null, setItem: (key: string, value: string) => storage.set(key, value) });
    const { input } = fixture(); await midi.init();
    const preset = midi.getMappings().find(mapping => mapping.action === 'deckA.play')!;
    midi.removeMapping(preset.id);
    midi.addMapping({ id: 'saved', deviceId: input.id, type: 'noteon', channel: 4, note: 0x14, action: 'sampler.pad2', valueMode: 'momentary', deck: 'A', padMode: 2 });
    midi.saveMappings(); midi.loadMappings();
    expect(midi.getMappings().find(mapping => mapping.id === 'saved')).toMatchObject({ deck: 'A', padMode: 2 });
    expect(midi.getMappings().some(mapping => mapping.id === preset.id)).toBe(false);
    expect(JSON.parse(storage.get('viib-dj-midi-mappings')!)).toHaveLength(1);
    midi.restoreDefaultMappings();
    expect(midi.getMappings().some(mapping => mapping.id === preset.id)).toBe(true);
    expect(midi.getMappings().some(mapping => mapping.id === 'saved')).toBe(false);
  });

  it('migrates old learned hot-cue pads into the Hot Cue bank without stealing other modes', async () => {
    const saved = [{ id: 'old-cue', deviceId: 'input-1', type: 'noteon', channel: 4, note: 0x14, action: 'deckA.hotCue3', valueMode: 'trigger' }];
    vi.stubGlobal('localStorage', { getItem: (key: string) => key === 'viib-dj-midi-mappings' ? JSON.stringify(saved) : null, setItem: vi.fn() });
    const { input } = fixture(); await midi.init();
    expect(midi.getMappings().find(mapping => mapping.id === 'old-cue')).toMatchObject({ deck: 'A', padMode: 0 });
    const handler = vi.fn(); midi.setActionHandler(handler);
    midi.sendPadModeFeedback(input.id, 'A', 2);
    input.onmidimessage?.({ data: new Uint8Array([0x94, 0x14, 127]) } as MIDIMessageEvent);
    expect(handler).toHaveBeenCalledExactlyOnceWith('deckA.performancePad1', 1, 'press', expect.objectContaining({ padMode: 2 }));
  });

  it('does not install a preset for unrelated hardware', async () => {
    const { input } = fixture(); Object.assign(input, { name: 'Other MIDI keyboard' });
    await midi.init();
    expect(midi.getMappings()).toEqual([]);
  });

  it('releases a custom gated pad when the DJ action handler unmounts', async () => {
    const { input } = fixture(); await midi.init();
    const handler = vi.fn(); midi.setActionHandler(handler);
    midi.addMapping({ id: 'gate-cleanup', deviceId: input.id, type: 'noteon', channel: 4, note: 0x14,
      action: 'sampler.pad1', valueMode: 'momentary', deck: 'A', padMode: 0 });
    input.onmidimessage?.({ data: new Uint8Array([0x94, 0x14, 127]) } as MIDIMessageEvent);
    midi.setActionHandler(null);
    expect(handler.mock.calls.map(call => call.slice(0, 3))).toEqual([['sampler.pad1', 1, 'press'], ['sampler.pad1', 0, 'release']]);
  });
});
