import { describe, expect, it } from 'vitest';
import { describeTestMixPhaseEvidence, inspectTestMixDownbeatEvidence, type TestMixDownbeatEvidence } from './testMixPhaseReadiness';

const reviewed: TestMixDownbeatEvidence = {
  beats: [0.1, 0.6, 1.1, 1.6, 2.1, 2.6, 3.1, 3.6],
  downbeatIndices: [0, 4],
  locked: true,
  source: 'manual',
};

describe('Test Mix phrase/bar phase evidence readiness', () => {
  it('requires a reviewed, locked grid with explicit in-range downbeat indices', () => {
    expect(inspectTestMixDownbeatEvidence(reviewed)).toMatchObject({ ready: true });
    expect(inspectTestMixDownbeatEvidence({ ...reviewed, source: 'measured' })).toMatchObject({ ready: false, reason: 'grid is not manually reviewed' });
    expect(inspectTestMixDownbeatEvidence({ ...reviewed, locked: false })).toMatchObject({ ready: false, reason: 'grid is not locked' });
    expect(inspectTestMixDownbeatEvidence({ ...reviewed, downbeatIndices: [] })).toMatchObject({ ready: false, reason: 'explicit downbeat indices are missing' });
    expect(inspectTestMixDownbeatEvidence({ ...reviewed, downbeatIndices: [0, 0] })).toMatchObject({ ready: false, reason: 'downbeat indices are invalid' });
    expect(inspectTestMixDownbeatEvidence({ ...reviewed, downbeatIndices: [0, 8] })).toMatchObject({ ready: false, reason: 'downbeat indices are invalid' });
  });

  it('never describes evidence availability as an applied or sample-accurate phase lock', () => {
    expect(describeTestMixPhaseEvidence(reviewed, reviewed)).toContain('phase lock is unavailable');
    expect(describeTestMixPhaseEvidence({ ...reviewed, source: 'inferred-from-meter' }, reviewed))
      .toContain('Downbeat evidence unavailable: reference grid is not manually reviewed.');
    expect(describeTestMixPhaseEvidence(reviewed, null))
      .toContain('Downbeat evidence unavailable: candidate grid is unavailable.');
  });
});
