/** Typed deferred fixture; Promise executors synchronously install both callbacks. */
export function deferred<T>() {
  let resolve: ((value: T | PromiseLike<T>) => void) | undefined;
  let reject: ((reason?: unknown) => void) | undefined;
  const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail; });
  if (!resolve || !reject) throw new Error('Promise executor did not install callbacks');
  return { promise, resolve, reject };
}
