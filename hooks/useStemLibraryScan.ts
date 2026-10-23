import { useEffect, useRef, useState } from 'react';
import { api } from '../services/api';
import { jobsV2, type OperationJob } from '../services/jobsV2';

const terminal = (job: OperationJob) => ['succeeded', 'failed', 'canceled', 'interrupted'].includes(job.status);

export function useStemLibraryScan(backendAvailable: boolean) {
  const [job, setJob] = useState<OperationJob | null>(null);
  const [starting, setStarting] = useState(false);
  const [canceling, setCanceling] = useState(false);
  const [error, setError] = useState('');
  const [progressError, setProgressError] = useState('');
  const action = useRef<AbortController | null>(null);
  const admitting = useRef(false);
  const jobID = job?.id;
  const active = Boolean(job && !terminal(job));
  const canPromote = Boolean(job && ['queued', 'running'].includes(job.status) && job.priority !== undefined && job.priority < 50);

  useEffect(() => () => action.current?.abort(), []);

  // Restore startup scans and scans started before Settings was opened.
  useEffect(() => {
    if (!backendAvailable || active) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    const restore = async () => {
      try {
        const { jobs } = await jobsV2.list('', controller.signal);
        if (controller.signal.aborted) return;
        const latest = jobs.filter(item => item.type === 'stem_library_scan')
          .sort((a, b) => b.createdAt - a.createdAt);
        const current = latest.find(item => !terminal(item)) || latest[0];
        if (current) setJob(previous => !previous || current.createdAt > previous.createdAt ||
          (current.id === previous.id && current.updatedAt > previous.updatedAt) ? current : previous);
        setProgressError('');
      } catch {
        if (!controller.signal.aborted) setProgressError('Unable to load Stem Library scan status. Retrying automatically.');
      }
      // Startup can enqueue its scan after Settings has already opened.
      if (!controller.signal.aborted) timer = setTimeout(() => void restore(), 5000);
    };
    void restore();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [backendAvailable, active]);

  // Poll only after the previous request finishes; abort requests on navigation
  // and never let an older response overwrite another scan's state.
  useEffect(() => {
    if (!backendAvailable || !jobID || !active) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    const poll = async () => {
      try {
        const next = await jobsV2.get(jobID, controller.signal);
        if (controller.signal.aborted) return;
        setJob(previous => previous?.id === jobID && next.updatedAt >= previous.updatedAt ?
          { ...next, priority: Math.max(next.priority ?? 0, previous.priority ?? 0) } : previous);
        setProgressError('');
      } catch {
        if (!controller.signal.aborted) {
          setProgressError('Unable to refresh scan progress. Retrying automatically; the scan may still be running.');
        }
      }
      // The effect stops polling once an accepted terminal snapshot renders.
      // A stale terminal response must not strand the newer active snapshot.
      if (!controller.signal.aborted) timer = setTimeout(() => void poll(), 1200);
    };
    void poll();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [backendAvailable, jobID, active]);

  const start = async () => {
    if (admitting.current) return;
    admitting.current = true;
    setStarting(true);
    setError('');
    setProgressError('');
    action.current?.abort();
    const controller = new AbortController();
    action.current = controller;
    try {
      const accepted = await api.scanStemLibraries(controller.signal);
      if (controller.signal.aborted) return;
      // Keep the accepted ID even if the first status request fails.
      setJob(previous => previous?.id === accepted.jobId ? { ...previous, priority: 50 } :
        { id: accepted.jobId, type: 'stem_library_scan', status: 'queued', priority: 50,
        progressCurrent: 0, progressTotal: 0, message: 'Queued Stem Library scan',
        attempts: 0, createdAt: Date.now(), updatedAt: 0 });
    } catch (cause) {
      if (!controller.signal.aborted) setError(cause instanceof Error ? cause.message : 'Unable to start Stem Library scan.');
    } finally {
      admitting.current = false;
      setStarting(false);
    }
  };

  const cancel = async () => {
    if (!job) return;
    setCanceling(true);
    setError('');
    action.current?.abort();
    const controller = new AbortController();
    action.current = controller;
    try {
      const next = await jobsV2.cancel(job.id, controller.signal);
      if (!controller.signal.aborted) setJob(previous => previous?.id === next.id ? next : previous);
    } catch (cause) {
      if (!controller.signal.aborted) setError(cause instanceof Error ? cause.message : 'Unable to cancel Stem Library scan.');
    } finally {
      setCanceling(false);
    }
  };

  return { job, active, canPromote, starting, canceling, error, progressError, start, cancel };
}
