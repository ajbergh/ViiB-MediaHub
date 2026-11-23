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

    const match = container.querySelector<HTMLElement>('[aria-label="Transcript search results"] button');
    expect(match?.textContent).toContain('0:09');
    expect(match?.textContent).toContain('Yes, that happened last week.');
    expect(container.textContent).not.toContain('Have you ever tried');

    act(() => {
      match?.click();
    });
    expect(onSeek).toHaveBeenCalledWith(9);
  });

  it('freezes transcript during sponsor break and shifts timing on Resume', () => {
    Element.prototype.scrollTo = vi.fn();
    localStorage.clear();
    const onSeek = vi.fn();

    act(() => {
      root.render(
        <PodcastTranscriptLyrics
          episodeId="ep-break-1"
          cues={cues}
          currentTime={6}
          onSeek={onSeek}
          isVisible
        />,
      );
    });

    const pauseBtn = Array.from(container.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Pause for break'),
    );
    expect(pauseBtn).toBeDefined();

    act(() => {
      pauseBtn?.click();
    });

    // Audio advances 30s through sponsor break (6s -> 36s), transcript stays frozen at 6s (cue[1]).
    act(() => {
      root.render(
        <PodcastTranscriptLyrics
          episodeId="ep-break-1"
          cues={cues}
          currentTime={36}
          onSeek={onSeek}
          isVisible
        />,
      );
    });

    const cueLinesPaused = Array.from(container.querySelectorAll<HTMLElement>('[role="button"]'));
    expect(cueLinesPaused[1].getAttribute('aria-current')).toBe('true');

    const resumeBtn = Array.from(container.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Resume (+0:30)'),
    );
    expect(resumeBtn).toBeDefined();

    act(() => {
      resumeBtn?.click();
    });

    // Offset is now +30s; at 40s audio time, effective transcript time is 10s (cue[2]).
    act(() => {
      root.render(
        <PodcastTranscriptLyrics
          episodeId="ep-break-1"
          cues={cues}
          currentTime={40}
          onSeek={onSeek}
          isVisible
        />,
      );
    });

    const cueLinesAfter = Array.from(container.querySelectorAll<HTMLElement>('[role="button"]'));
    expect(cueLinesAfter[2].getAttribute('aria-current')).toBe('true');
    expect(container.textContent).toContain('+30s');
    expect(onSeek).not.toHaveBeenCalled();
  });

  it('aligns any line to current audio time without seeking and surfaces Skip break for toc=false chapters', () => {
    Element.prototype.scrollTo = vi.fn();
    localStorage.clear();
    const onSeek = vi.fn();

    act(() => {
      root.render(
        <PodcastTranscriptLyrics
          episodeId="ep-align-1"
          cues={cues}
          chapters={[{ startTime: 20, endTime: 50, title: 'Sponsor', toc: false }]}
          currentTime={24}
          onSeek={onSeek}
          isVisible
        />,
      );
    });

    const skipBreakBtn = Array.from(container.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Skip break'),
    );
    expect(skipBreakBtn).toBeDefined();
    act(() => {
      skipBreakBtn?.click();
    });
    expect(onSeek).toHaveBeenCalledWith(50);

    onSeek.mockClear();
    const alignBtn = Array.from(container.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Align line'),
    );
    act(() => {
      alignBtn?.click();
    });

    // Align cue[1] (4s) to currentTime=24s => +20s offset, without seeking.
    const cueLines = Array.from(container.querySelectorAll<HTMLElement>('[role="button"]'));
    act(() => {
      cueLines[1].click();
    });
    expect(onSeek).not.toHaveBeenCalled();
    expect(cueLines[1].getAttribute('aria-current')).toBe('true');
    expect(container.textContent).toContain('+20s');
  });
});
