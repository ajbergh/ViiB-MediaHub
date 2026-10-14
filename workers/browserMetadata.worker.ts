/** Parses browser-imported audio using bundled dependencies, without network imports. */
import { parseBrowserSongMetadata } from '../lib/browserMetadata';

self.onmessage = async (event: MessageEvent) => {
  const { id, file, path, relevantImages } = event.data;
  try {
    const metadata = await parseBrowserSongMetadata(file, path, relevantImages || []);
    self.postMessage({ id, metadata });
  } catch (error) {
    self.postMessage({ id, error: error instanceof Error ? error.message : 'Unknown worker error' });
  }
};
