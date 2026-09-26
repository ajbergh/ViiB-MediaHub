/**
 * Module-scoped shims evaluated ahead of the Signalsmith Stretch package inside
 * the AudioWorklet global scope. The package would normally register its own
 * AudioWorkletNode; these bindings instead capture its WASM processor class,
 * built on an inert base, so the stem transport can own an instance and drive
 * it from the transport's buffered frames and clock.
 *
 * This depends on package internals (wasmReady, wasmModule, configure(),
 * buffersIn/buffersOut, bufferLength), so the dependency is pinned exactly and
 * stemTransport.worklet.test.ts exercises it against the real package.
 */
export const STRETCH_PRELUDE = `const AudioWorkletProcessor = class {
  constructor() { this.port = { onmessage: null, postMessage() {} }; }
};
let registerProcessor = (name, processor) => {
  if (name === 'signalsmith-stretch') globalThis.__viibStretchCore = processor;
};
`;

let moduleUrl: Promise<string> | null = null;

/** Object URL for the stretch core module, created once per page. Loaded on demand (~115 kB). */
export function stretchModuleUrl(): Promise<string> {
  moduleUrl ??= import('signalsmith-stretch?raw').then(({ default: source }) =>
    URL.createObjectURL(new Blob([STRETCH_PRELUDE, source], { type: 'text/javascript' })));
  moduleUrl.catch(() => { moduleUrl = null; });
  return moduleUrl;
}
