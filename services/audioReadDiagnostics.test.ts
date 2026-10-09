import { beforeEach, expect, it, vi } from 'vitest';
const warn = vi.hoisted(() => vi.fn());
vi.mock('./loggerService', () => ({ createLogger: () => ({ warn }) }));
beforeEach(() => { vi.resetModules(); warn.mockReset(); vi.useRealTimers(); });

it('records only allowlisted categories and HTTP status without private error details', async () => {
 const { reportAudioReadFailure: report } = await import('./audioReadDiagnostics');
 report('audio_metadata', Object.assign(new Error('C:/private/song.mp3 token=secret'), { status: 503, stack: 'secret stack', url: 'private-url' }));
 report('local_bands', Object.assign(new Error('private payload'), { category: 'invalid_payload' }));
 report('deck_analysis', new TypeError('private request URL'));
 report('audio_metadata', new SyntaxError('private body'));
 expect(warn.mock.calls).toEqual([
  ['Audio read failed', {operation:'audio_metadata',category:'http',status:503}],
  ['Audio read failed', {operation:'local_bands',category:'invalid_payload'}],
  ['Audio read failed', {operation:'deck_analysis',category:'type_error'}],
  ['Audio read failed', {operation:'audio_metadata',category:'invalid_json'}],
 ]);
 expect(JSON.stringify(warn.mock.calls)).not.toMatch(/secret|private|song.mp3/);
});
it('ignores expected fallback and cancellation, coalesces shared failures, and bounds logging', async () => {
 vi.useFakeTimers();vi.setSystemTime(100_000);
 const { reportAudioReadFailure: report } = await import('./audioReadDiagnostics');
 for(const status of [404,412]) report('local_bands',{status});
 report('audio_metadata',{name:'AbortError'});
 const shared=new TypeError('network');report('local_bands',shared);report('local_bands',shared);
 expect(warn).toHaveBeenCalledTimes(1);
 for(let i=0;i<30;i++) report('audio_metadata',new Error('failure'));
 expect(warn).toHaveBeenCalledTimes(20);
 vi.setSystemTime(160_000);report('audio_metadata',new Error('failure'));
 expect(warn).toHaveBeenCalledTimes(21);
 vi.useRealTimers();
});
it('never lets malformed errors or logger failures break recovery', async () => {
 const { reportAudioReadFailure: report } = await import('./audioReadDiagnostics');
 const broken={get status(){throw new Error('getter');}};
 expect(()=>report('local_bands',broken)).not.toThrow();
 warn.mockImplementation(()=>{throw new Error('logger');});
 expect(()=>report('audio_metadata',new Error('failed'))).not.toThrow();
});
it('coalesces only matching in-flight metadata reads and releases them after settlement', async () => {
 const { withSharedAudioMetadataRead: readShared } = await import('./audioReadDiagnostics');
 let resolveFirst!: (value: { sourceFingerprint: string }) => void;
 const read = vi.fn(() => new Promise<{ sourceFingerprint: string }>(resolve => { resolveFirst = resolve; }));
 const ownerA = vi.fn(() => true);
 const ownerB = vi.fn(() => true);
 const first = readShared('["song","source-a",1]', read, ownerA);
 const second = readShared('["song","source-a",1]', read, ownerB);
 expect(read).toHaveBeenCalledTimes(1);
 resolveFirst({ sourceFingerprint: 'source-a' });
 await expect(Promise.all([first, second])).resolves.toEqual([{ sourceFingerprint: 'source-a' }, { sourceFingerprint: 'source-a' }]);

 const otherSourceRead = vi.fn().mockResolvedValue({ sourceFingerprint: 'source-b' });
 await expect(readShared('["song","source-b",1]', otherSourceRead)).resolves.toEqual({ sourceFingerprint: 'source-b' });
 expect(otherSourceRead).toHaveBeenCalledTimes(1);
 const otherSessionRead = vi.fn().mockResolvedValue({ sourceFingerprint: 'source-a' });
 await expect(readShared('["song","source-a",2]', otherSessionRead)).resolves.toEqual({ sourceFingerprint: 'source-a' });
 expect(otherSessionRead).toHaveBeenCalledTimes(1);
 const afterSettlementRead = vi.fn().mockResolvedValue({ sourceFingerprint: 'source-a' });
 await expect(readShared('["song","source-a",1]', afterSettlementRead)).resolves.toEqual({ sourceFingerprint: 'source-a' });
 expect(afterSettlementRead).toHaveBeenCalledTimes(1);
});
