import { describe, expect, it } from 'vitest';
import { compareAnalysisReadiness, getAnalysisReadiness } from './analysisReadiness';

describe('analysis readiness', () => {
  it('maps every analysis state to explicit readiness text', () => {
    expect(getAnalysisReadiness().label).toBe('Not analyzed');
    expect(getAnalysisReadiness('pending').label).toBe('Queued');
    expect(getAnalysisReadiness('running').label).toBe('Analyzing');
    expect(getAnalysisReadiness('complete').label).toBe('Ready');
    expect(getAnalysisReadiness('partial').label).toBe('Partial');
    expect(getAnalysisReadiness('failed').label).toBe('Failed');
    expect(getAnalysisReadiness('unsupported').label).toBe('Unsupported');
    for (const status of ['pending', 'running', 'complete', 'partial', 'failed', 'unsupported'] as const) {
      expect(getAnalysisReadiness(status).description.toLowerCase()).toContain('analysis');
    }
  });

  it('distinguishes a missing record from analysis-list load and API failures', () => {
    expect(getAnalysisReadiness(undefined, 'loading').label).toBe('Checking');
    expect(getAnalysisReadiness(undefined, 'failed').label).toBe('Unavailable');
    expect(getAnalysisReadiness(undefined, 'loaded').label).toBe('Not analyzed');
  });

  it('sorts from complete through unsupported with missing rows called out', () => {
    const rows = [
      { id: 'unsupported', status: 'unsupported' }, { id: 'failed', status: 'failed' }, { id: 'missing', status: undefined },
      { id: 'pending', status: 'pending' }, { id: 'running', status: 'running' },
      { id: 'partial', status: 'partial' }, { id: 'complete', status: 'complete' },
    ] as const;
    const sorted = [...rows].sort((left, right) => compareAnalysisReadiness(left.status, right.status));
    expect(sorted.map(row => row.id)).toEqual(['complete', 'partial', 'running', 'pending', 'missing', 'failed', 'unsupported']);
    expect(compareAnalysisReadiness('complete', 'partial')).toBeLessThan(0);
    expect(compareAnalysisReadiness('unsupported', 'failed')).toBeGreaterThan(0);
  });
});
