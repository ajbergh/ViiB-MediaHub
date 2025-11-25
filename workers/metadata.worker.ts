import { parseSongFile } from '../lib/parsers';

self.onmessage = async (e: MessageEvent) => {
    const { id, file, path, relevantImages } = e.data;
    
    try {
        // Delegate to the shared parser logic
        const song = await parseSongFile(file, path, relevantImages || []);
        self.postMessage({ id, song });
    } catch (error: any) {
        console.error('Worker parsing error:', error);
        self.postMessage({ id, error: error.message || 'Unknown worker error' });
    }
};