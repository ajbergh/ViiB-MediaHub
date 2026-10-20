/** Keeps controller browsing tied to the mounted browser's filtered/sorted list. */
import type { DeckId } from '../slices/djMixerSlice';

interface LibraryController {
  browse: (steps: number) => void;
  load: (deck: DeckId) => void;
}
let controller: LibraryController | null = null;
let openLibrary: (() => void) | null = null;
let toggleLibrary: (() => void) | null = null;
let pendingSteps = 0;
let pendingLoad: DeckId | null = null;
export function registerDJMidiLibraryOpener(open: () => void, toggle: () => void = open): () => void {
  openLibrary = open;
  toggleLibrary = toggle;
  return () => { if (openLibrary === open) { openLibrary = null; toggleLibrary = null; pendingSteps = 0; pendingLoad = null; } };
}
export function openDJMidiLibrary(): void { openLibrary?.(); }
export function toggleDJMidiLibrary(): void { toggleLibrary?.(); }
export function registerDJMidiLibrary(next: LibraryController): () => void {
  controller = next;
  if (pendingSteps) { next.browse(pendingSteps); pendingSteps = 0; }
  if (pendingLoad) { next.load(pendingLoad); pendingLoad = null; }
  return () => { if (controller === next) controller = null; };
}
export function browseDJMidiLibrary(steps: number): void {
  openLibrary?.();
  if (controller) controller.browse(steps); else if (openLibrary) pendingSteps += steps;
}
export function loadDJMidiLibrary(deck: DeckId): void {
  openLibrary?.();
  if (controller) controller.load(deck); else if (openLibrary) pendingLoad = deck;
}
