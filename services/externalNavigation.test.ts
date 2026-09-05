import { describe, expect, it } from 'vitest';
import { shouldUseSystemBrowser } from './externalNavigation';

describe('external navigation', () => {
  it('uses the system browser from the native Wails origin', () => {
    expect(shouldUseSystemBrowser({ hostname: 'wails.localhost' })).toBe(true);
  });

  it('keeps normal popup behavior for browser builds', () => {
    expect(shouldUseSystemBrowser({ hostname: 'localhost' })).toBe(false);
  });
});
