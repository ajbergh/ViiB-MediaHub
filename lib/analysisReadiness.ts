import type { TrackAnalysisFeature } from '../services/api';

export interface AnalysisReadiness {
  label: string;
  rank: number;
  symbol: string;
  className: string;
  description: string;
}

export type AnalysisListLoadState = 'loading' | 'loaded' | 'failed';

const STATUS_READINESS: Record<TrackAnalysisFeature['status'], AnalysisReadiness> = {
  complete: {
    label: 'Ready', rank: 0, symbol: '●', className: 'text-emerald-300',
    description: 'Analysis completed. Readiness does not rate audio quality.',
  },
  partial: {
    label: 'Partial', rank: 1, symbol: '◒', className: 'text-amber-300',
    description: 'Some analysis completed; one or more dimensions may be unavailable.',
  },
  running: {
    label: 'Analyzing', rank: 2, symbol: '◷', className: 'text-blue-300',
    description: 'Analysis is currently running.',
  },
  pending: {
    label: 'Queued', rank: 3, symbol: '…', className: 'text-blue-300',
    description: 'Analysis is queued and has not started yet.',
  },
  failed: {
    label: 'Failed', rank: 5, symbol: '×', className: 'text-red-300',
    description: 'Analysis failed. This status does not describe audio quality.',
  },
  unsupported: {
    label: 'Unsupported', rank: 6, symbol: '—', className: 'text-neutral-500',
    description: 'Analysis is unsupported for this track or format.',
  },
};

const NOT_ANALYZED: AnalysisReadiness = {
  label: 'Not analyzed', rank: 4, symbol: '○', className: 'text-neutral-500',
  description: 'No analysis record exists. Readiness does not rate audio quality.',
};

const CHECKING_ANALYSIS: AnalysisReadiness = {
  label: 'Checking', rank: 7, symbol: '◷', className: 'text-neutral-400',
  description: 'The library is loading analysis readiness.',
};

const ANALYSIS_UNAVAILABLE: AnalysisReadiness = {
  label: 'Unavailable', rank: 8, symbol: '?', className: 'text-neutral-500',
  description: 'Analysis readiness could not be loaded. This does not rate audio quality.',
};

export function getAnalysisReadiness(
  status?: TrackAnalysisFeature['status'],
  listState: AnalysisListLoadState = 'loaded',
): AnalysisReadiness {
  if (listState === 'loading') return CHECKING_ANALYSIS;
  if (listState === 'failed') return ANALYSIS_UNAVAILABLE;
  if (!status) return NOT_ANALYZED;
  return STATUS_READINESS[status] ?? NOT_ANALYZED;
}

export function compareAnalysisReadiness(
  left?: TrackAnalysisFeature['status'],
  right?: TrackAnalysisFeature['status'],
  listState: AnalysisListLoadState = 'loaded',
): number {
  return getAnalysisReadiness(left, listState).rank - getAnalysisReadiness(right, listState).rank;
}
