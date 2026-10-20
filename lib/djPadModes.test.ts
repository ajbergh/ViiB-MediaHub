import { describe, expect, it, vi } from 'vitest';
import { DJPadModeController, type PadModeActions, type PerformancePad } from './djPadModes';

function fixture() {
  const actions: PadModeActions = {
    hotCue: vi.fn(), autoLoop: vi.fn(), stems: vi.fn(), sampler: vi.fn(), modeLED: vi.fn(),
  };
  const controller = new DJPadModeController(actions);
  const cycle = () => {
    controller.handleButton('A', 'mode', true, 'controller');
    controller.handleButton('A', 'mode', false, 'controller');
  };
  return { actions, controller, cycle };
}

describe('software pad modes', () => {
  it('cycles A through 0–3 back to 0 without changing B, and emits LED state', () => {
    const { actions, controller, cycle } = fixture();
    expect(controller.getMode('A')).toBe(0);
    for (const expected of [1, 2, 3, 0]) {
      cycle();
      expect(controller.getMode('A')).toBe(expected);
      expect(controller.getMode('B')).toBe(0);
      expect(actions.modeLED).toHaveBeenLastCalledWith('A', expected, 'controller');
    }
  });

  it('ignores release and repeated down events while Mode is held', () => {
    const { actions, controller } = fixture();
    controller.handleButton('B', 'mode', false, 'controller');
    controller.handleButton('B', 'mode', true, 'controller');
    controller.handleButton('B', 'mode', true, 'controller');
    expect(controller.getMode('B')).toBe(1);
    expect(actions.modeLED).toHaveBeenCalledOnce();
  });

  it.each([1, 2, 3, 4] as PerformancePad[])('dispatches pad %i according to its current deck mode', pad => {
    const { actions, controller, cycle } = fixture();
    const press = () => {
      controller.handleButton('A', pad, true, 'controller');
      controller.handleButton('A', pad, false, 'controller');
    };
    press();
    expect(actions.hotCue).toHaveBeenCalledExactlyOnceWith('A', pad);
    cycle();
    press();
    expect(actions.autoLoop).toHaveBeenCalledExactlyOnceWith('A', [1, 2, 4, 8][pad - 1]);
    cycle();
    press();
    expect(actions.sampler).toHaveBeenCalledExactlyOnceWith('A', pad);
    cycle();
    press();
    expect(actions.stems).toHaveBeenCalledExactlyOnceWith('A', pad);
    controller.handleButton('B', pad, true, 'other-controller');
    expect(actions.hotCue).toHaveBeenLastCalledWith('B', pad);
  });

  it('releases a gated sample on mode change and does not retrigger the held pad', () => {
    const { actions, controller, cycle } = fixture();
    const stop = vi.fn();
    vi.mocked(actions.sampler).mockReturnValue(stop);
    cycle(); cycle();
    controller.handleButton('A', 1, true, 'controller');
    cycle();
    expect(stop).toHaveBeenCalledOnce();
    controller.handleButton('A', 1, true, 'controller');
    expect(actions.stems).not.toHaveBeenCalled();
    controller.handleButton('A', 1, false, 'controller');
    expect(stop).toHaveBeenCalledOnce();
  });

  it('stops held gated samples when the controller disconnects', () => {
    const { actions, controller, cycle } = fixture();
    const restore = vi.fn();
    vi.mocked(actions.sampler).mockReturnValue(restore);
    cycle(); cycle();
    controller.handleButton('A', 1, true, 'controller');
    controller.handleButton('A', 2, true, 'controller');
    expect(restore).not.toHaveBeenCalled();
    controller.disconnectDevice('controller');
    expect(restore).toHaveBeenCalledTimes(2);
    controller.handleButton('A', 2, false, 'controller');
    expect(restore).toHaveBeenCalledTimes(2);
  });

  it('cleans up both decks on dispose', () => {
    const { actions, controller } = fixture();
    const stop = vi.fn();
    vi.mocked(actions.sampler).mockReturnValue(stop);
    for (const deck of ['A', 'B'] as const) {
      for (let n = 0; n < 2; n++) {
        controller.handleButton(deck, 'mode', true, 'controller');
        controller.handleButton(deck, 'mode', false, 'controller');
      }
      controller.handleButton(deck, 1, true, 'controller');
    }
    controller.dispose();
    expect(stop).toHaveBeenCalledTimes(2);
  });
});
