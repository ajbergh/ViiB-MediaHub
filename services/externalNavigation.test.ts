import { describe, expect, it } from 'vitest';
import { shouldUseSystemBrowser } from './externalNavigation';

describe('external navigation', () => {
  it('uses the system browser from the native Wails origin', () => {
    expect(shouldUseSystemBrowser({ hostname: 'wails.localhost' })).toBe(true);
  });

  it('uses the system browser from the macOS Wails custom-scheme origin', () => {
    expect(shouldUseSystemBrowser({ hostname: 'wails', protocol: 'wails:' })).toBe(true);
  });

  it('uses the system browser when the Wails bridge is injected', () => {
    expect(shouldUseSystemBrowser({ hostname: '', hasWailsBridge: true })).toBe(true);
  });

  it('keeps normal popup behavior for browser builds', () => {
    expect(shouldUseSystemBrowser({ hostname: 'localhost', protocol: 'http:' })).toBe(false);
  });
});
