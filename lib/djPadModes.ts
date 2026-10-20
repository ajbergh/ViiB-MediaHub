/** Software-owned performance-pad modes; hardware only supplies button events. */
import type { DeckId } from '../slices/djMixerSlice';

export type PadMode = 0 | 1 | 2 | 3;
export type PerformancePad = 1 | 2 | 3 | 4;
export const PAD_MODE_NAMES = ['Hot Cue', 'Loops', 'Sampler', 'Stems'] as const;
export const PAD_LOOP_BEATS = [1, 2, 4, 8] as const;

export interface PadModeActions {
  hotCue: (deck: DeckId, slot: PerformancePad) => void;
  autoLoop: (deck: DeckId, beats: number) => void;
  /** Return cleanup for gate-mode samples; one-shots need no cleanup. */
  sampler: (deck: DeckId, pad: PerformancePad) => (() => void) | void;
  stems: (deck: DeckId, pad: PerformancePad) => void;
  modeLED: (deck: DeckId, mode: PadMode, deviceId: string) => void;
}

export class DJPadModeController {
  private modes: Record<DeckId, PadMode> = { A: 0, B: 0 };
  private held = new Map<string, { deck: DeckId; deviceId: string; release?: () => void }>();

  constructor(private actions: PadModeActions) {}

  getMode(deck: DeckId): PadMode { return this.modes[deck]; }

  /** Called with raw Note/CC button-down state, including releases. */
  handleButton(deck: DeckId, control: 'mode' | PerformancePad, down: boolean, deviceId: string): void {
    const key = `${deviceId}:${deck}:${control}`;
    if (!down) {
      const held = this.held.get(key);
      this.held.delete(key);
      held?.release?.();
      return;
    }
    if (this.held.has(key)) return; // Repeated Note On / positive CC is not another press.
    this.held.set(key, { deck, deviceId });

    if (control === 'mode') {
      // Release momentary effects before changing their meaning. Keep physical
      // buttons latched until release so a held pad cannot retrigger in a new mode.
      this.releaseDeck(deck);
      this.modes[deck] = ((this.modes[deck] + 1) % 4) as PadMode;
      this.actions.modeLED(deck, this.modes[deck], deviceId);
      return;
    }

    let release: (() => void) | void = undefined;
    switch (this.modes[deck]) {
      case 0:
        this.actions.hotCue(deck, control);
        break;
      case 1:
        this.actions.autoLoop(deck, PAD_LOOP_BEATS[control - 1]);
        break;
      case 2:
        release = this.actions.sampler(deck, control);
        break;
      case 3:
        this.actions.stems(deck, control);
        break;
    }
    const held = this.held.get(key);
    if (held && release) held.release = release;
  }

  releaseDeck(deck: DeckId): void {
    for (const held of this.held.values()) {
      if (held.deck !== deck) continue;
      const release = held.release;
      held.release = undefined;
      release?.();
    }
  }

  disconnectDevice(deviceId: string): void {
    for (const [key, held] of this.held) {
      if (held.deviceId !== deviceId) continue;
      this.held.delete(key);
      held.release?.();
    }
  }

  dispose(): void {
    this.releaseDeck('A');
    this.releaseDeck('B');
    this.held.clear();
  }
}
