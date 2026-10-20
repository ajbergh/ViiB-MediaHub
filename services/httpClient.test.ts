// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest';
import { APIError, requestJSON } from './httpClient';

const token = '12345678-1234-4123-8123-123456789abc';
afterEach(() => vi.unstubAllGlobals());

it('retains the body diagnostic reference independently of legacy request ID and header', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: {
    code: 'fixture_failure', message: 'Maintenance unavailable', retryable: false,
    requestId: 'private-host-request', diagnosticToken: token,
  } }), { status: 503, headers: { 'X-ViiB-Diagnostic-Token': '87654321-1234-4123-8123-123456789abc' } })));
  await expect(requestJSON('/fixture', { retry: false })).rejects.toMatchObject({
    message: 'Maintenance unavailable', requestId: 'private-host-request', diagnosticToken: token,
  });
});

it('uses a valid diagnostic header for non-JSON failures without reusing request IDs', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('unavailable', { status: 503,
    headers: { 'X-ViiB-Diagnostic-Token': token, 'X-Request-ID': 'private-host-request' } })));
  await expect(requestJSON('/fixture', { retry: false })).rejects.toMatchObject({ diagnosticToken: token });
});

it.each(['private-path/title', '12345678-1234-1123-8123-123456789abc', token + ' private', ''])('rejects malformed diagnostic references: %s', async candidate => {
  const error = new APIError(503, { code: 'failure', message: 'Unavailable', retryable: false, requestId: token, diagnosticToken: candidate });
  expect(error.diagnosticToken).toBeUndefined();
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('bad response', { status: 503, headers: { 'X-Request-ID': token } })));
  await expect(requestJSON('/fixture', { retry: false })).rejects.toMatchObject({ diagnosticToken: undefined });
});
