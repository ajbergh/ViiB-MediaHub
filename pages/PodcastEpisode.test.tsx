// @vitest-environment jsdom

import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  episodeId: 'ep-1',
  storeState: {} as any,
  getEpisodeDetail: vi.fn(),
  getFeed: vi.fn(),
  listBookmarks: vi.fn(),
  getEpisodeChapters: vi.fn(),
  getEpisodeTranscript: vi.fn(),
  patchEpisodeState: vi.fn(),
  addToQueue: vi.fn(),
  queueDownload: vi.fn(),
  cancelDownload: vi.fn(),
  removeDownload: vi.fn(),
  createBookmark: vi.fn(),
  deleteBookmark: vi.fn(),
}));

vi.mock('react-router', () => ({
  useParams: () => ({ id: mocks.episodeId }),
  Link: ({ to, children, ...rest }: any) => (
    <a href={to} {...rest}>
      {children}
    </a>
  ),
}));

vi.mock('../store', () => {
  const useStore = (selector: (s: any) => any) => selector(mocks.storeState);
  useStore.getState = () => mocks.storeState;
  return { useStore };
});

vi.mock('../services/podcasts', async () => {
  const actual = await vi.importActual<typeof import('../services/podcasts')>('../services/podcasts');
  return {
    ...actual,
    podcastsAPI: {
      ...actual.podcastsAPI,
      getEpisodeDetail: mocks.getEpisodeDetail,
      getFeed: mocks.getFeed,
      listBookmarks: mocks.listBookmarks,
      getEpisodeChapters: mocks.getEpisodeChapters,
      getEpisodeTranscript: mocks.getEpisodeTranscript,
      patchEpisodeState: mocks.patchEpisodeState,
      addToQueue: mocks.addToQueue,
      queueDownload: mocks.queueDownload,
      cancelDownload: mocks.cancelDownload,
      removeDownload: mocks.removeDownload,
      createBookmark: mocks.createBookmark,
      deleteBookmark: mocks.deleteBookmark,
    },
  };
});

import { PodcastAPIError } from '../services/podcasts';
import { PodcastEpisodePage } from './PodcastEpisode';

function makeDetail(overrides: Record<string, unknown> = {}) {
  return {
    id: 'ep-1',
    feedId: 'feed-1',
    guid: 'guid-1',
    title: 'Deep Dive into Distributed Audio',
    description: 'Plain fallback description',
    richDescription:
      '<p>Hello <strong>world</strong></p><script>alert(1)</script><a href="javascript:evil()">bad</a><a href="https://example.com/notes">notes</a>',
    hasAudio: true,
    enclosureType: 'audio/mpeg',
    enclosureBytes: 15_728_640,
    publishedAt: 1700000000000,
    durationMs: 1800000,
    isPlayed: false,
    positionMs: 120000,
    listenedMs: 90000,
    progressRevision: 4,
    hasChapters: false,
    hasTranscript: false,
    websiteUrl: 'https://example.com/episode-1',
    seasonNumber: 2,
    episodeNumber: 14,
    starred: false,
    archived: false,
    feedTitle: 'Systems Talk',
    feedAuthor: 'Jane Host',
    bookmarkCount: 1,
    downloadStatus: '',
    defaultSpeed: 1.25,
    skipIntroSeconds: 10,
    skipOutroSeconds: 15,
    ...overrides,
  };
}

function makeFeed(overrides: Record<string, unknown> = {}) {
  return {
    id: 'feed-1',
    url: 'https://example.com/feed.xml',
    title: 'Systems Talk',
    author: 'Jane Host',
    description: 'Show desc',
    imageUrl: 'https://example.com/art.png',
    language: 'en',
    episodeCount: 25,
    defaultSpeed: 1.25,
    skipIntroSeconds: 10,
    skipOutroSeconds: 15,
    ...overrides,
  };
}

async function flush() {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
  });
}

describe('PodcastEpisodePage', () => {
  let container: HTMLDivElement;
  let root: ReturnType<typeof createRoot>;

  beforeEach(() => {
    (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
    mocks.episodeId = 'ep-1';
    mocks.storeState = {
      currentSong: null,
      isPlaying: false,
      playSong: vi.fn().mockResolvedValue(undefined),
      togglePlay: vi.fn(),
      playNext: vi.fn(),
      addToQueue: vi.fn(),
      showToast: vi.fn(),
    };

    mocks.getEpisodeDetail.mockReset().mockResolvedValue({ episode: makeDetail() });
    mocks.getFeed.mockReset().mockResolvedValue({ feed: makeFeed() });
    mocks.listBookmarks.mockReset().mockResolvedValue({
      bookmarks: [
        { id: 'bm-1', episodeId: 'ep-1', positionMs: 65000, note: 'Key insight', createdAt: 1700000000000 },
      ],
    });
    mocks.getEpisodeChapters.mockReset().mockResolvedValue({ chapters: [], source: 'none' });
    mocks.getEpisodeTranscript.mockReset().mockResolvedValue({ cues: [], format: 'none' });
    mocks.patchEpisodeState.mockReset();
    mocks.addToQueue.mockReset().mockResolvedValue({ queue: [] });
    mocks.queueDownload.mockReset().mockResolvedValue({ download: { status: 'queued' } });
    mocks.cancelDownload.mockReset().mockResolvedValue({ download: { status: 'canceled' } });
    mocks.removeDownload.mockReset().mockResolvedValue(undefined);
    mocks.createBookmark.mockReset();
    mocks.deleteBookmark.mockReset().mockResolvedValue(undefined);

    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => {
      root.unmount();
    });
    container.remove();
  });

  it('renders loading skeleton then episode hero details and sanitized show notes', async () => {
    let resolveDetail!: (value: any) => void;
    mocks.getEpisodeDetail.mockReturnValueOnce(
      new Promise(resolve => {
        resolveDetail = resolve;
      })
    );

    await act(async () => {
      root.render(<PodcastEpisodePage />);
    });
    expect(container.querySelector('[aria-busy="true"]')).not.toBeNull();

    await act(async () => {
      resolveDetail({ episode: makeDetail() });
    });
    await flush();

    expect(container.querySelector('h1')?.textContent).toBe('Deep Dive into Distributed Audio');
    expect(container.textContent).toContain('Season 2');
    expect(container.textContent).toContain('Episode 14');
    expect(container.textContent).toContain('30:00');
    expect(container.textContent).toContain('15.0 MB');

    const backLinks = Array.from(container.querySelectorAll('a')).map(a => a.getAttribute('href'));
    expect(backLinks).toContain('/podcasts/show/feed-1');
    expect(backLinks).toContain('/podcasts');

    // XSS script removed, javascript: link neutered, https link preserved with noopener
    expect(container.querySelector('script')).toBeNull();
    expect(container.querySelector('a[href^="javascript:"]')).toBeNull();
    const notesLink = container.querySelector('a[href="https://example.com/notes"]');
    expect(notesLink?.getAttribute('target')).toBe('_blank');
    expect(notesLink?.getAttribute('rel')).toBe('noopener noreferrer');

    const extLink = container.querySelector('a[href="https://example.com/episode-1"]');
    expect(extLink?.getAttribute('target')).toBe('_blank');
    expect(extLink?.getAttribute('rel')).toBe('noopener noreferrer');
  });

  it('renders 404 state when episode is not found', async () => {
    mocks.getEpisodeDetail.mockRejectedValueOnce(
      new PodcastAPIError(404, 'podcast_episode_not_found', 'Podcast episode not found')
    );

    await act(async () => {
      root.render(<PodcastEpisodePage />);
    });
    await flush();

    expect(container.querySelector('h1')?.textContent).toBe('Episode not found');
    expect(container.textContent).toContain('Podcast episode not found');
    expect(container.querySelector('a[href="/podcasts"]')).not.toBeNull();
  });

  it('handles Listen/Resume, queue actions, state toggles, and download lifecycle', async () => {
    mocks.patchEpisodeState
      .mockResolvedValueOnce({
        state: { episodeId: 'ep-1', positionMs: 120000, listenedMs: 90000, isPlayed: false, revision: 4, completedAt: 0, lastPlayedAt: 0, starred: true, archived: false },
      })
      .mockResolvedValueOnce({
        state: { episodeId: 'ep-1', positionMs: 120000, listenedMs: 90000, isPlayed: false, revision: 4, completedAt: 0, lastPlayedAt: 0, starred: true, archived: true },
      })
      .mockResolvedValueOnce({
        state: { episodeId: 'ep-1', positionMs: 120000, listenedMs: 90000, isPlayed: true, revision: 5, completedAt: 1, lastPlayedAt: 1, starred: true, archived: true },
      });

    await act(async () => {
      root.render(<PodcastEpisodePage />);
    });
    await flush();

    const clickButton = async (labelIncludes: string) => {
      const btn = Array.from(container.querySelectorAll('button')).find(
        b => b.getAttribute('aria-label')?.includes(labelIncludes) || b.textContent?.includes(labelIncludes)
      );
      expect(btn).toBeTruthy();
      await act(async () => {
        btn!.click();
      });
      await flush();
    };

    await clickButton('Resume Deep Dive into Distributed Audio');
    expect(mocks.storeState.playSong).toHaveBeenCalledWith(
      expect.objectContaining({
        podcastEpisodeId: 'ep-1',
        podcastFeedId: 'feed-1',
        podcastResumeMs: 120000,
      }),
      [expect.objectContaining({ podcastEpisodeId: 'ep-1' })],
      'podcast',
      0
    );

    await clickButton('Play Deep Dive into Distributed Audio next');
    expect(mocks.addToQueue).toHaveBeenCalledWith('ep-1', 'next');

    await clickButton('Add Deep Dive into Distributed Audio to queue');
    expect(mocks.addToQueue).toHaveBeenCalledWith('ep-1', 'end');

    await clickButton('Star Deep Dive into Distributed Audio');
    expect(mocks.patchEpisodeState).toHaveBeenNthCalledWith(1, 'ep-1', { starred: true });

    await clickButton('Archive Deep Dive into Distributed Audio');
    expect(mocks.patchEpisodeState).toHaveBeenNthCalledWith(2, 'ep-1', { archived: true });

    await clickButton('Mark Deep Dive into Distributed Audio played');
    expect(mocks.patchEpisodeState).toHaveBeenNthCalledWith(3, 'ep-1', {
      isPlayed: true,
      expectedRevision: 4,
    });

    await clickButton('Download Deep Dive into Distributed Audio');
    expect(mocks.queueDownload).toHaveBeenCalledWith('ep-1');
    expect(container.textContent).toContain('Download queued');

    await clickButton('Cancel Download Deep Dive into Distributed Audio');
    expect(mocks.cancelDownload).toHaveBeenCalledWith('ep-1');
  });

  it('fetches chapters lazily, shows active chapter, and dispatches viib:podcast-seek or starts playback', async () => {
    mocks.getEpisodeDetail.mockResolvedValue({
      episode: makeDetail({ hasChapters: true, positionMs: 130000 }),
    });
    mocks.getEpisodeChapters.mockResolvedValue({
      source: 'podcast2',
      chapters: [
        { startTime: 0, title: 'Intro' },
        { startTime: 120, title: 'Architecture', url: 'https://example.com/arch' },
      ],
    });

    // First render when NOT currently playing
    await act(async () => {
      root.render(<PodcastEpisodePage />);
    });
    await flush();

    expect(mocks.getEpisodeChapters).toHaveBeenCalledWith('ep-1', expect.any(AbortSignal));

    const chaptersTab = container.querySelector('#tab-chapters') as HTMLButtonElement;
    expect(chaptersTab.textContent).toContain('2');
    await act(async () => {
      chaptersTab.click();
    });
    await flush();

    const archBtn = Array.from(container.querySelectorAll('button')).find(b =>
      b.getAttribute('aria-label')?.includes('Seek to chapter Architecture')
    )!;
    await act(async () => {
      archBtn.click();
    });
    expect(mocks.storeState.playSong).toHaveBeenCalledWith(
      expect.objectContaining({ podcastEpisodeId: 'ep-1', podcastResumeMs: 120000 }),
      [expect.objectContaining({ podcastEpisodeId: 'ep-1', podcastResumeMs: 120000 })],
      'podcast',
      0
    );

    // Now simulate episode currently playing in shared player
    mocks.storeState.currentSong = { id: 'podcast_ep-1', source: 'podcast', podcastEpisodeId: 'ep-1' };
    mocks.storeState.isPlaying = true;
    const seekEvents: Array<{ episodeId: string; seconds: number }> = [];
    const listener = (e: Event) => seekEvents.push((e as CustomEvent).detail);
    window.addEventListener('viib:podcast-seek', listener);

    await act(async () => {
      root.render(<PodcastEpisodePage />);
    });
    await flush();

    expect(container.textContent).toContain('Playing');
    const introBtn = Array.from(container.querySelectorAll('button')).find(b =>
      b.getAttribute('aria-label')?.includes('Seek to chapter Intro')
    )!;
    await act(async () => {
      introBtn.click();
    });
    window.removeEventListener('viib:podcast-seek', listener);

    expect(seekEvents).toEqual([{ episodeId: 'ep-1', seconds: 0 }]);
  });

  it('filters transcript cues and seeks on click', async () => {
    mocks.getEpisodeDetail.mockResolvedValue({
      episode: makeDetail({ hasTranscript: true }),
    });
    mocks.getEpisodeTranscript.mockResolvedValue({
      format: 'json',
      cues: [
        { startMs: 10000, endMs: 15000, speaker: 'Alice', text: 'Welcome to the show' },
        { startMs: 45000, endMs: 52000, speaker: 'Bob', text: 'SQLite WAL mode rocks' },
      ],
    });

    await act(async () => {
      root.render(<PodcastEpisodePage />);
    });
    await flush();

    const transcriptTab = container.querySelector('#tab-transcript') as HTMLButtonElement;
    await act(async () => {
      transcriptTab.click();
    });
    await flush();

    expect(container.textContent).toContain('Welcome to the show');
    expect(container.textContent).toContain('SQLite WAL mode rocks');

    const filterInput = container.querySelector('input[aria-label="Filter transcript"]') as HTMLInputElement;
    await act(async () => {
      const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!;
      setter.call(filterInput, 'SQLite');
      filterInput.dispatchEvent(new Event('input', { bubbles: true }));
    });

    expect(container.textContent).not.toContain('Welcome to the show');
    expect(container.textContent).toContain('SQLite WAL mode rocks');

    const cueBtn = Array.from(container.querySelectorAll('button')).find(b =>
      b.getAttribute('aria-label')?.includes('Seek to 0:45 Bob')
    )!;
    await act(async () => {
      cueBtn.click();
    });
    expect(mocks.storeState.playSong).toHaveBeenCalledWith(
      expect.objectContaining({ podcastResumeMs: 45000 }),
      expect.any(Array),
      'podcast',
      0
    );
  });

  it('creates, seeks, and deletes bookmarks', async () => {
    mocks.createBookmark.mockResolvedValue({
      bookmark: { id: 'bm-2', episodeId: 'ep-1', positionMs: 120000, note: 'Great point', createdAt: 1700000001000 },
    });

    await act(async () => {
      root.render(<PodcastEpisodePage />);
    });
    await flush();

    const bookmarksTab = container.querySelector('#tab-bookmarks') as HTMLButtonElement;
    expect(bookmarksTab.textContent).toContain('1');
    await act(async () => {
      bookmarksTab.click();
    });

    const timeInput = container.querySelector('input[aria-label="Bookmark timestamp"]') as HTMLInputElement;
    expect(timeInput.value).toBe('2:00'); // detail.positionMs = 120000ms

    const noteInput = container.querySelector('input[aria-label="Bookmark note"]') as HTMLInputElement;
    await act(async () => {
      const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!;
      setter.call(noteInput, 'Great point');
      noteInput.dispatchEvent(new Event('input', { bubbles: true }));
    });

    const form = container.querySelector('form')!;
    await act(async () => {
      form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
    });
    await flush();

    expect(mocks.createBookmark).toHaveBeenCalledWith('ep-1', 120000, 'Great point');
    expect(container.textContent).toContain('Great point');
    expect(bookmarksTab.textContent).toContain('2');

    const delBtn = container.querySelector('button[aria-label="Delete bookmark at 1:05"]') as HTMLButtonElement;
    await act(async () => {
      delBtn.click();
    });
    await flush();

    expect(mocks.deleteBookmark).toHaveBeenCalledWith('ep-1', 'bm-1');
    expect(container.textContent).not.toContain('Key insight');
    expect(bookmarksTab.textContent).toContain('1');
  });
});
