import type { PersistStorage, StorageValue } from 'zustand/middleware';

/** Skip serialization and storage I/O when only non-persisted state changed.
 * Persisted fields must use immutable updates, as the Zustand slices do. */
export function deduplicatingPersistStorage<T extends object>(storage: PersistStorage<T> | undefined): PersistStorage<T> | undefined {
  if (!storage) return undefined;
  const written = new Map<string, StorageValue<T>>();
  return {
    getItem: name => storage.getItem(name),
    removeItem: name => {
      written.delete(name);
      return storage.removeItem(name);
    },
    setItem: (name, value) => {
      const previous = written.get(name);
      const keys = Object.keys(value.state) as (keyof T)[];
      if (previous && previous.version === value.version
        && keys.length === Object.keys(previous.state).length
        && keys.every(key => Object.is(value.state[key], previous.state[key]))) return;
      const result = storage.setItem(name, value);
      // Failed writes must remain retryable. This app uses synchronous localStorage,
      // but also retain the storage contract for asynchronous implementations.
      if (result instanceof Promise) return result.then(() => { written.set(name, value); });
      written.set(name, value);
      return result;
    },
  };
}
