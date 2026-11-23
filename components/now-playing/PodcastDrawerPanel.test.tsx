// @vitest-environment jsdom

import React, { act } from 'react';
import { createRoot, Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PodcastTranscriptLyrics } from './PodcastDrawerPanel';
import type { PodcastTranscriptCue } from '../../services/podcasts';

const cues: PodcastTranscriptCue[] = [
  { startMs: 0, endMs: 4000, speaker: 'JACK', text: 'Have you ever tried installing an app' },
  { startMs: 4000, endMs: 9000, speaker: 'JACK', text: 'on your computer and got something else?' },
  { startMs: 9000, endMs: 14000, speaker: 'GUEST', text: 'Yes, that happened last week.' },
];

describe('PodcastTranscriptLyrics', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => {
      root.unmount();
    });
    container.remove();
  });

  it('renders cues in scrolling-lyrics style, highlights the active cue, and auto-centers', () => {
    const scrollTo = vi.fn();
    Element.prototype.scrollTo = scrollTo;
    const onSeek = vi.fn();

    act(() => {
      root.render(
        <PodcastTranscriptLyrics
          cues={cues}
          currentTime={6}
          onSeek={onSeek}
          isVisible
        />,
      );
    });

    const buttons = Array.from(container.querySelectorAll<HTMLElement>('[role="button"]'));
    expect(buttons).toHaveLength(3);

    const active = buttons[1];
    expect(active.getAttribute('aria-current')).toBe('true');
    expect(active.className).toContain('scale-105');

    const inactive = buttons[0];
    expect(inactive.getAttribute('aria-current')).toBeNull();
    expect(inactive.className).toContain('opacity-30');

    expect(scrollTo).toHaveBeenCalled();

    act(() => {
      buttons[2].click();
    });
    expect(onSeek).toHaveBeenCalledWith(9);
  });

  it('switches to searchable timestamped results when a query is entered', () => {
    const onSeek = vi.fn();

    act(() => {
      root.render(
        <PodcastTranscriptLyrics
          cues={cues}
          currentTime={6}
          onSeek={onSeek}
          isVisible
          query="last week"
        />,
      );
    });

    const match = container.querySelector('button');
    expect(match?.textContent).toContain('0:09');
    expect(match?.textContent).toContain('Yes, that happened last week.');
    expect(container.textContent).not.toContain('Have you ever tried');

    act(() => {
      match?.click();
    });
    expect(onSeek).toHaveBeenCalledWith(9);
  });
});
