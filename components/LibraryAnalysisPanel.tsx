/**
 * LibraryAnalysisPanel is the Phase 4 control surface for durable track
 * analysis: start a run over missing, outdated, or all tracks, watch its
 * progress, pause or cancel it, and see how many tracks could not be analyzed.
 *
 * It deliberately shows no BPM or key values. Those are Phase 5 and are gated
 * on the accuracy work; presenting a measured tempo here would make a claim
 * the analyzers have not yet earned.
 */
import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { AlertTriangle, Gauge, Pause, Play, RefreshCw, Square } from 'lucide-react';
import {
  AnalysisJobResult,
  AnalysisSelectionMode,
  OperationJob,
  SETTING_AUTO_ANALYZE_NEW_TRACKS,
  jobsV2,
} from '../services/jobsV2';
import { api } from '../services/api';

const actionClass = 'inline-flex items-center gap-2 rounded-lg bg-brand px-4 py-2 text-sm font-semibold text-black hover:bg-brand-hover disabled:cursor-not-allowed disabled:opacity-50';
const secondaryClass = 'inline-flex items-center gap-2 rounded-lg bg-surface-2 px-4 py-2 text-sm font-semibold text-text-main hover:bg-surface-3 disabled:cursor-not-allowed disabled:opacity-50';

/** Statuses in which a job still represents outstanding work. */
const ACTIVE_STATUSES = new Set(['queued', 'running', 'paused', 'canceling']);

const SELECTIONS: { mode: AnalysisSelectionMode; label: string; hint: string }[] = [
  { mode: 'missing', label: 'Analyze missing', hint: 'Tracks that have never been analyzed.' },
  { mode: 'stale', label: 'Analyze outdated', hint: 'Tracks analyzed by an older algorithm version.' },
  { mode: 'all', label: 'Re-analyze all', hint: 'Every local track. Already-current results are skipped, not repeated.' },
];

const isEnabled = (value: string) => ['1', 'true', 'yes', 'on', 'enabled'].includes(value.trim().toLowerCase());

/** analysisResultOf reads the aggregate counts a finished run reports. */
const analysisResultOf = (job: OperationJob | null): AnalysisJobResult | null => {
  if (!job?.result) return null;
  return job.result as AnalysisJobResult;
};

export const LibraryAnalysisPanel: React.FC = () => {
  const [jobs, setJobs] = useState<OperationJob[]>([]);
  const [autoAnalyze, setAutoAnalyze] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [message, setMessage] = useState('');
  const [error, setError] = useState('');

  // Only analysis jobs belong in this panel; the same durable queue also
  // carries scans and aggregate refreshes.
  const analysisJobs = useMemo(() => jobs.filter(job => job.type === 'analyze_tracks'), [jobs]);
  const activeJob = useMemo(() => analysisJobs.find(job => ACTIVE_STATUSES.has(job.status)) ?? null, [analysisJobs]);
  const lastFinished = useMemo(() => analysisJobs.find(job => !ACTIVE_STATUSES.has(job.status)) ?? null, [analysisJobs]);
  const lastResult = analysisResultOf(lastFinished);

  const refresh = useCallback(async (signal?: AbortSignal) => {
    try {
      const listed = await jobsV2.list('', signal);
      setJobs(listed.jobs);
    } catch (listError) {
      if (!signal?.aborted) setError(listError instanceof Error ? listError.message : 'Unable to load analysis jobs');
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    void refresh(controller.signal);
    // The jobs SSE stream already carries progress for the whole queue, so this
    // panel does not open a second stream.
    const unsubscribe = jobsV2.subscribe(setJobs);
    return () => { controller.abort(); unsubscribe(); };
  }, [refresh]);

  useEffect(() => {
    let canceled = false;
    void (async () => {
      try {
        const value = await api.getSetting(SETTING_AUTO_ANALYZE_NEW_TRACKS);
        if (!canceled) setAutoAnalyze(isEnabled(value));
      } catch {
        // An unreadable setting simply leaves the toggle off; it is not an
        // error worth interrupting the panel for.
      }
    })();
    return () => { canceled = true; };
  }, []);

  const run = async (label: string, operation: () => Promise<string>) => {
    setBusy(label); setError(''); setMessage('');
    try { setMessage(await operation()); }
    catch (operationError) { setError(operationError instanceof Error ? operationError.message : `${label} failed`); }
    finally { setBusy(null); await refresh(); }
  };

  const startAnalysis = (mode: AnalysisSelectionMode) => run(`start-${mode}`, async () => {
    const job = await jobsV2.analyze({ mode });
    return `Analysis queued (${mode}). Job ${job.id.slice(0, 8)}.`;
  });

  const pauseQueue = () => run('pause', async () => {
    const { paused } = await jobsV2.pauseQueue();
    return paused > 0
      ? `Paused ${paused} queued job${paused === 1 ? '' : 's'}. Work already in progress finishes first.`
      : 'No queued work to pause. A job that is already running finishes its current track.';
  });

  const resumeQueue = () => run('resume', async () => {
    const { resumed } = await jobsV2.resumeQueue();
    return resumed > 0 ? `Resumed ${resumed} job${resumed === 1 ? '' : 's'}.` : 'No paused work to resume.';
  });

  const cancelActive = () => run('cancel', async () => {
    if (!activeJob) return 'No analysis run to cancel.';
    await jobsV2.cancel(activeJob.id);
    // Cancellation is cooperative at track granularity, and finished tracks
    // stay finished, so restarting the same selection resumes rather than
    // repeating.
    return 'Cancellation requested. Tracks already analyzed are kept.';
  });

  const toggleAutoAnalyze = () => run('auto-analyze', async () => {
    const next = !autoAnalyze;
    await api.setSetting(SETTING_AUTO_ANALYZE_NEW_TRACKS, next ? 'true' : 'false');
    setAutoAnalyze(next);
    return next
      ? 'New tracks found by a scan will be analyzed in the background.'
      : 'Scans will no longer queue analysis automatically.';
  });

  const progressPercent = activeJob && activeJob.progressTotal > 0
    ? Math.min(100, Math.round((activeJob.progressCurrent / activeJob.progressTotal) * 100))
    : 0;

  return (
    <section className="rounded-xl border border-surface-highlight bg-surface-1 p-5">
      <div className="mb-4 flex items-center gap-3">
        <Gauge className="text-brand" />
        <h2 className="text-xl font-semibold">Track Analysis</h2>
      </div>
      <p className="mb-4 max-w-3xl text-sm text-text-secondary">
        Measure tempo and musical key for your local library so DJ features have durable, versioned values to work with.
        Results are stored per track and survive restarts: a run that is interrupted resumes where it stopped instead of starting over.
      </p>

      {(message || error) && (
        <div className={`mb-4 rounded-lg border p-3 text-sm ${error ? 'border-error/40 bg-error/10 text-error' : 'border-accent-green/30 bg-accent-green/10 text-text-main'}`} role="status">
          {error || message}
        </div>
      )}

      <div className="mb-4 flex flex-wrap gap-3">
        {SELECTIONS.map(selection => (
          <button
            key={selection.mode}
            className={selection.mode === 'missing' ? actionClass : secondaryClass}
            disabled={busy !== null || activeJob !== null}
            title={selection.hint}
            onClick={() => startAnalysis(selection.mode)}
          >
            <RefreshCw size={16} />{selection.label}
          </button>
        ))}
      </div>

      {activeJob ? (
        <div className="rounded-lg bg-surface-2 p-4">
          <div className="mb-2 flex flex-wrap items-center justify-between gap-3">
            <span className="text-sm font-semibold">{activeJob.message || 'Analyzing'}</span>
            <span className="text-xs text-text-secondary">
              {activeJob.progressTotal > 0 ? `${activeJob.progressCurrent} / ${activeJob.progressTotal}` : activeJob.status}
            </span>
          </div>
          <div className="h-2 overflow-hidden rounded-full bg-surface-3" role="progressbar" aria-valuenow={progressPercent} aria-valuemin={0} aria-valuemax={100}>
            <div className="h-full bg-brand transition-[width] duration-300" style={{ width: `${progressPercent}%` }} />
          </div>
          <div className="mt-3 flex flex-wrap gap-2">
            {activeJob.status === 'paused' ? (
              <button className={secondaryClass} disabled={busy !== null} onClick={resumeQueue}><Play size={15} />Resume</button>
            ) : (
              <button className={secondaryClass} disabled={busy !== null} onClick={pauseQueue}><Pause size={15} />Pause</button>
            )}
            <button className={secondaryClass} disabled={busy !== null} onClick={cancelActive}><Square size={15} />Cancel</button>
          </div>
        </div>
      ) : (
        <p className="text-sm text-text-secondary">No analysis run is in progress.</p>
      )}

      {lastResult && (
        <dl className="mt-4 grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
          <div className="rounded-lg bg-surface-2 p-3"><dt className="text-text-secondary">Selected</dt><dd className="mt-1 font-semibold">{lastResult.total ?? 0}</dd></div>
          <div className="rounded-lg bg-surface-2 p-3"><dt className="text-text-secondary">Analyzed</dt><dd className="mt-1 font-semibold">{lastResult.analyzed ?? 0}</dd></div>
          <div className="rounded-lg bg-surface-2 p-3"><dt className="text-text-secondary">Already current</dt><dd className="mt-1 font-semibold">{lastResult.skipped ?? 0}</dd></div>
          <div className="rounded-lg bg-surface-2 p-3">
            <dt className="text-text-secondary">Could not analyze</dt>
            <dd className={`mt-1 font-semibold ${(lastResult.failed ?? 0) > 0 ? 'text-error' : ''}`}>{lastResult.failed ?? 0}</dd>
          </div>
        </dl>
      )}

      {(lastResult?.failed ?? 0) > 0 && (
        <p className="mt-3 flex items-start gap-2 text-xs text-text-secondary">
          <AlertTriangle size={14} className="mt-0.5 shrink-0 text-error" />
          Tracks that could not be analyzed are recorded with the reason and are not retried on every run. Unsupported formats and unreadable files are the usual causes.
        </p>
      )}

      <label className="mt-5 flex items-start gap-3 text-sm">
        <input
          type="checkbox"
          className="mt-0.5 h-4 w-4 accent-brand"
          checked={autoAnalyze}
          disabled={busy !== null}
          onChange={toggleAutoAnalyze}
        />
        <span>
          <span className="font-semibold">Analyze new tracks automatically</span>
          <span className="block text-text-secondary">Queue a background analysis run after a scan finds new tracks. It runs below anything you start yourself and yields while DJ playback is active.</span>
        </span>
      </label>
    </section>
  );
};
