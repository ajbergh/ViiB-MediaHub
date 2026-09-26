import React, { useEffect, useState } from 'react';
import { Gauge } from 'lucide-react';
import { jobsV2, type OperationJob } from '../services/jobsV2';

const isRunningAnalysis = (job: OperationJob): boolean =>
  job.type === 'analyze_tracks' && job.status === 'running';

/** A persistent, route-independent progress notice for automatic track analysis. */
export const AnalysisProgressToast: React.FC = () => {
  const [jobs, setJobs] = useState<OperationJob[]>([]);

  useEffect(() => {
    const controller = new AbortController();
    void jobsV2.list('', controller.signal)
      .then(({ jobs: listed }) => setJobs(listed))
      .catch(() => {
        // The job stream may be unavailable in browser-only use; ordinary app
        // behavior remains unaffected when the backend cannot be reached.
      });
    const unsubscribe = jobsV2.subscribe(setJobs);
    return () => {
      controller.abort();
      unsubscribe();
    };
  }, []);

  const activeJob = jobs.find(isRunningAnalysis);
  if (!activeJob) return null;

  const total = activeJob.progressTotal;
  const current = Math.min(activeJob.progressCurrent, total);
  const message = total > 0
    ? `Analyzing ${current} of ${total}`
    : 'Preparing track analysis';
  const percent = total > 0 ? Math.min(100, (current / total) * 100) : 0;

  return (
    <div
      className="fixed bottom-28 left-4 z-[200] w-[min(24rem,calc(100vw-2rem))] rounded-lg border border-brand/30 bg-surface-2 px-4 py-3 shadow-xl backdrop-blur-sm"
      role="status"
      aria-live="polite"
      aria-label={message}
    >
      <div className="flex items-center gap-3">
        <div className="rounded-full bg-brand p-1 text-surface-0">
          <Gauge size={16} aria-hidden="true" />
        </div>
        <span className="flex-1 text-sm font-medium text-text-main">{message}</span>
      </div>
      <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-surface-3" aria-hidden="true">
        <div className="h-full bg-brand transition-[width] duration-300" style={{ width: `${percent}%` }} />
      </div>
    </div>
  );
};
