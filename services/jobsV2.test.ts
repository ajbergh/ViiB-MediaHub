import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import { ANALYSIS_FOREGROUND_PRIORITY, jobsV2 } from './jobsV2';

// The shared transport schedules its timeout through window; the node test
// environment has no window, so provide the two timer functions it uses.
beforeAll(() => {
  (globalThis as unknown as { window: unknown }).window = {
    setTimeout: globalThis.setTimeout.bind(globalThis),
    clearTimeout: globalThis.clearTimeout.bind(globalThis),
  };
});

const accepted = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 202, headers: { 'Content-Type': 'application/json' } });

// A Response body can only be read once, so a mock reused across calls must
// build a fresh one each time.
const acceptedAlways = (body: unknown) => () => Promise.resolve(accepted(body));

const requestBodyOf = (call: unknown[]) => JSON.parse((call[1] as RequestInit).body as string);

describe('jobsV2 analysis client', () => {
  afterEach(() => vi.restoreAllMocks());

  it('records the selection in the job parameters so a resumed job re-expands it', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(accepted({ id: 'job-1', type: 'analyze_tracks', status: 'queued' }));

    await expect(jobsV2.analyze({ mode: 'missing' })).resolves.toMatchObject({ type: 'analyze_tracks' });

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/v2/jobs/');
    expect((init as RequestInit).method).toBe('POST');
    expect(requestBodyOf(fetchMock.mock.calls[0])).toEqual({ type: 'analyze_tracks', parameters: { mode: 'missing' } });
  });

  it('carries song IDs and playlist selections verbatim', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation(acceptedAlways({ id: 'job-2', type: 'analyze_tracks', status: 'queued' }));

    await jobsV2.analyze({ mode: 'ids', songIds: ['a', 'b'] });
    await jobsV2.analyze({ mode: 'playlist', playlistId: 'list-1' });

    expect(requestBodyOf(fetchMock.mock.calls[0]).parameters).toEqual({ mode: 'ids', songIds: ['a', 'b'] });
    expect(requestBodyOf(fetchMock.mock.calls[1]).parameters).toEqual({ mode: 'playlist', playlistId: 'list-1' });
  });

  it('omits priority unless one is requested, so the backend default applies', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation(acceptedAlways({ id: 'job-3', type: 'analyze_tracks', status: 'queued' }));

    await jobsV2.analyze({ mode: 'all' });
    await jobsV2.analyze({ mode: 'all' }, ANALYSIS_FOREGROUND_PRIORITY);

    expect(requestBodyOf(fetchMock.mock.calls[0])).not.toHaveProperty('priority');
    expect(requestBodyOf(fetchMock.mock.calls[1]).priority).toBe(ANALYSIS_FOREGROUND_PRIORITY);
  });

  it('pauses and resumes the durable queue rather than a single job', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(accepted({ paused: 3 }))
      .mockResolvedValueOnce(accepted({ resumed: 3 }));

    await expect(jobsV2.pauseQueue()).resolves.toEqual({ paused: 3 });
    await expect(jobsV2.resumeQueue()).resolves.toEqual({ resumed: 3 });

    expect(fetchMock.mock.calls[0][0]).toBe('/api/v2/jobs/pause');
    expect(fetchMock.mock.calls[1][0]).toBe('/api/v2/jobs/resume');
  });

  it('reports playback pressure with an explicit TTL so a closed tab cannot park the queue', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(accepted({ active: true, remainingSeconds: 30 }));

    await expect(jobsV2.reportPlaybackPressure(true, 30)).resolves.toEqual({ active: true, remainingSeconds: 30 });

    expect(fetchMock.mock.calls[0][0]).toBe('/api/v2/jobs/analysis-pressure');
    expect(requestBodyOf(fetchMock.mock.calls[0])).toEqual({ active: true, ttlSeconds: 30 });
  });

  it('clears playback pressure without a TTL', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(accepted({ active: false, remainingSeconds: 0 }));

    await jobsV2.reportPlaybackPressure(false);

    expect(requestBodyOf(fetchMock.mock.calls[0])).toEqual({ active: false });
  });

  it('surfaces the structured backend error for an unexpandable selection', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({
      error: { code: 'invalid_analysis_selection', message: 'analysis selection "ids" requires song IDs', retryable: false },
    }), { status: 400, headers: { 'Content-Type': 'application/json' } }));

    await expect(jobsV2.analyze({ mode: 'ids' })).rejects.toThrow('analysis selection "ids" requires song IDs');
  });

  it('never retries an analysis submission, because a retry means a second job', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({
      error: { code: 'job_create_failed', message: 'Unable to create the operation job', retryable: true },
    }), { status: 500, headers: { 'Content-Type': 'application/json' } }));

    await expect(jobsV2.analyze({ mode: 'all' })).rejects.toThrow('Unable to create the operation job');
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});
